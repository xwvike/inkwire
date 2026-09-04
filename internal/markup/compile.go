package markup

import (
	"encoding/json"
	"fmt"
	stdimage "image"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
	"golang.org/x/net/html"
)

// Warning names an element and says what this renderer could not do with it.
type Warning struct {
	Path    string
	Code    string
	Message string
}

// Document is what a page compiled to: a scene document, and everything the
// compiler could not honour on the way there.
//
// It is bytes rather than a tree because those bytes are the product. A page
// is a second way of writing a scene document, so what it compiles to is a
// scene document — one that can be printed, diffed, sent to a device, or read
// by the decoder every other document goes through. A tree here would be this
// package deciding it knew what a page means, and it does not need to.
type Document struct {
	JSON     []byte
	Warnings []Warning
}

// inlineByDefault are the elements that join the text around them instead of
// starting a box of their own, unless a stylesheet says otherwise.
var inlineByDefault = map[string]bool{
	"span": true, "b": true, "i": true, "small": true, "em": true,
	"strong": true, "a": true, "label": true, "code": true,
	"u": true, "s": true, "del": true, "ins": true, "mark": true,
	"sub": true, "sup": true, "abbr": true, "cite": true, "q": true,
	"time": true, "kbd": true, "samp": true, "var": true, "dfn": true,
	"bdi": true, "bdo": true, "data": true, "ruby": true, "rt": true,
	"rp": true, "wbr": true, "img": true, "svg": true,
}

type compiler struct {
	sheet    *stylesheet
	warnings []Warning
	// warningSink lets an isolated resource report through the compiler that
	// owns the page without sharing its stylesheet or computed-style cache.
	warningSink *[]Warning
	// computedFor caches each element's style. The tree is visited more than
	// once, and without this an element's unsupported declarations would be
	// reported once per visit.
	computedFor map[*html.Node]style
	// clips are the clipPath elements the drawing being compiled defines,
	// which is the one thing in a drawing that is named where it is written
	// and used somewhere else.
	clips map[string]*html.Node
	// patterns are the same, for the element that says what a fill tiles with.
	patterns map[string]*html.Node
	// elements are the named SVG elements a <use> may reference. They are
	// indexed per drawing, just like clip paths and patterns.
	elements map[string]*html.Node
	// useStack prevents a malformed cyclic chain of <use> references from
	// recursing until the compiler stack is exhausted.
	useStack map[string]bool
	drawings func(string) ([]byte, error)
}

// Compiler holds what compiling a page needs beyond the page itself.
type Compiler struct {
	// Stylesheets reads what a link element points at, for the same reason
	// and on the same terms as Drawings. Without one a link is reported rather
	// than ignored, because a page whose stylesheet did not arrive lays out
	// as almost nothing and the reason has to be findable.
	Stylesheets func(href string) ([]byte, error)
	// StylesheetName is what the stylesheet handed to Compile is called, when
	// it has a name. A link naming the same thing is that same stylesheet
	// arriving a second time, and only a name can tell that from two files
	// that happen to hold the same rules.
	StylesheetName string
	// Drawings reads the file an img element names when that file is an svg,
	// on the same terms as the other two.
	Drawings func(src string) ([]byte, error)
}

const maxRemoteDrawingBytes = 32 << 20

var remoteDrawingClient = &http.Client{Timeout: 15 * time.Second}

// Compile turns one HTML document and one stylesheet into a scene document.
func Compile(markup, css string) (Document, error) {
	return Compiler{}.Compile(markup, css)
}

func (options Compiler) Compile(markup, css string) (Document, error) {
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return Document{}, fmt.Errorf("parse markup: %w", err)
	}
	c := &compiler{computedFor: map[*html.Node]style{}, drawings: options.Drawings}
	css, styled := c.gatherStyles(root, css, options.StylesheetName, options.Stylesheets)
	if strings.TrimSpace(css) == "" && !styled {
		// Said here because here is where every source of style has been
		// looked for. A page with a style element of its own has a
		// stylesheet; whether a file sat beside it is not the question, and
		// asking it from the caller's side made every single-file page carry
		// a warning that it was missing something it was not missing.
		c.warn("stylesheet", "no-stylesheet",
			"this page has no style at all: no stylesheet was given, and it carries no style element, "+
				"no link and no style attribute")
	}
	sheet, err := parseStylesheet(css, func(code, message string) {
		c.warn("stylesheet", code, message)
	})
	if err != nil {
		return Document{}, err
	}
	c.sheet = sheet
	for _, name := range atRules(css) {
		c.warn("stylesheet", "unsupported-at-rule",
			fmt.Sprintf("@%s is not implemented; there is one panel and one frame, so there is nothing for it to select on", name))
	}

	body := findBody(root)
	if body == nil {
		return Document{Warnings: c.warnings}, fmt.Errorf("markup has no body")
	}
	element := firstElement(body)
	if element == nil {
		return Document{Warnings: c.warnings}, fmt.Errorf("body has no element to render")
	}
	tree, _ := c.element(element, rootStyle(), "root", nil)
	if tree == nil {
		return Document{Warnings: c.warnings},
			fmt.Errorf("the root element resolved to nothing to draw")
	}
	written, err := json.MarshalIndent(document{
		Version:     scene.Version,
		Orientation: c.orientation(element),
		Size:        sizeOf(c.pageSize(element)),
		Root:        tree,
	}, "", " ")
	if err != nil {
		return Document{Warnings: c.warnings}, fmt.Errorf("write the document: %w", err)
	}
	return Document{JSON: written, Warnings: c.warnings}, nil
}

// orientation is the one thing about a page that is not a style. It says how
// the panel is hung, which decides how the packed bytes are turned on their
// way out, and no stylesheet has a word for it.
//
// It stays absent unless the root element says so, and absent means the same
// here as in a scene document: landscape. Silence used to mean landscape too,
// but silently — this package produced a tree with no orientation on it and
// the zero value made the choice. Writing it into the document means a page
// hung the other way is a page that says so, and a page that says nothing
// says nothing in a file somebody can read.
func (c *compiler) orientation(element *html.Node) string {
	stated := strings.TrimSpace(attribute(element, "orientation"))
	switch stated {
	case "", "landscape":
		return ""
	case "portrait-cw", "portraitClockwise":
		return "portraitClockwise"
	case "portrait-ccw", "portraitCounterClockwise":
		return "portraitCounterClockwise"
	}
	c.warn("root", "unsupported-declaration", fmt.Sprintf(
		"orientation=%q is not one of landscape, portrait-cw or portrait-ccw", stated))
	return ""
}

// notContent are the elements that say something about the document rather
// than appearing in it. A style element's text is a stylesheet and a title's
// is a title, and neither is words on a panel — which is what they became
// when the only thing this walked for was boxes and text.
var notContent = map[string]bool{
	"style": true, "link": true, "script": true, "title": true,
	"meta": true, "head": true, "base": true, "template": true,
}

// gatherStyles collects the stylesheets the document carries and puts them
// after the one handed in.
//
// After, because that is the order a browser applies them in and the order an
// author expects: a file sets out what the pages share and the page overrides
// it. Two rules of equal specificity are settled by which came last, so this
// is not a detail.
//
// A page written in one file is the case this exists for. Until it did, a
// style element was read by nobody and its text was walked as if it were
// words, so the page came out as a paragraph of CSS with no stylesheet.
// The second return says whether the document carries style of its own beyond
// what is collected here: a style attribute is style, and a page written
// entirely in them has a stylesheet in every sense that matters even though
// there is no stylesheet to collect.
func (c *compiler) gatherStyles(root *html.Node, css, name string, resolve func(string) ([]byte, error)) (string, bool) {
	// Every source in the order the page states it, because that order is the
	// cascade: two rules of equal weight are settled by which came last.
	type source struct {
		id  string // "" for a style element, which has no identity to share
		css string
	}
	sources := []source{{id: stylesheetID(name), css: css}}
	styled := false
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if strings.TrimSpace(attribute(node, "style")) != "" {
				styled = true
			}
			switch node.Data {
			case "style":
				sources = append(sources, source{css: textContent(node)})
				return
			case "link":
				if linked := c.linkedStyles(node, resolve); strings.TrimSpace(linked) != "" {
					sources = append(sources,
						source{id: stylesheetID(attribute(node, "href")), css: linked})
				}
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	// One stylesheet can reach a page twice: the file beside it is read
	// whether or not the page asks, so a link naming that same file arrives
	// again. The link is the one to keep, at the position the page put it —
	// it is what a browser would see, and the sibling is a convention of this
	// tool that a page has no way to place. Dropping the later copy instead
	// would move a stylesheet earlier in the cascade than the page says.
	//
	// Identity is the name, never the content: two different files that happen
	// to say the same thing are two stylesheets, and the second still wins.
	claimed := map[string]int{}
	for index, entry := range sources {
		if entry.id != "" {
			claimed[entry.id] = index
		}
	}
	var collected strings.Builder
	for index, entry := range sources {
		if entry.id != "" && claimed[entry.id] != index {
			if index == 0 {
				c.warn("stylesheet", "duplicate-stylesheet", fmt.Sprintf(
					"the stylesheet beside this page is also linked as %q, so it was read once, where the link is",
					strings.TrimSpace(entry.id)))
			} else {
				c.warn("stylesheet", "duplicate-stylesheet", fmt.Sprintf(
					"link href=%q was already read, so it was read once", strings.TrimSpace(entry.id)))
			}
			continue
		}
		if collected.Len() > 0 {
			collected.WriteString("\n")
		}
		collected.WriteString(entry.css)
	}
	return collected.String(), styled
}

// stylesheetID names a stylesheet so that the same one arriving twice can be
// recognised. Two ways of writing one path are one stylesheet; two paths are
// two, whatever they contain.
func stylesheetID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.Contains(name, "://") {
		return name
	}
	return path.Clean(name)
}

// linkedStyles reads the stylesheet a link element names.
//
// The reading is the caller's, for the reason every other file this package
// refers to is: which files may be read is a question about where the page
// came from, and this package does not know that and should not.
func (c *compiler) linkedStyles(node *html.Node, resolve func(string) ([]byte, error)) string {
	if rel := strings.ToLower(strings.TrimSpace(attribute(node, "rel"))); rel != "stylesheet" {
		return ""
	}
	href := strings.TrimSpace(attribute(node, "href"))
	if href == "" {
		c.warn("stylesheet", "unresolved-stylesheet", "a link element with rel=stylesheet has no href")
		return ""
	}
	if resolve == nil {
		c.warn("stylesheet", "unresolved-stylesheet", fmt.Sprintf(
			"link href=%q was not read: this compiler was given no way to resolve one", href))
		return ""
	}
	linked, err := resolve(href)
	if err != nil {
		c.warn("stylesheet", "unresolved-stylesheet", fmt.Sprintf("link href=%q: %v", href, err))
		return ""
	}
	return string(linked)
}

// pageSize is the root element's own width and height, which is the page the
// document is written for. Anything but a whole number of pixels states no
// page: a percentage has nothing to be a percentage of at the root.
func (c *compiler) pageSize(element *html.Node) stdimage.Point {
	computed, ok := c.computedFor[element]
	if !ok {
		return stdimage.Point{}
	}
	width, height := computed.width, computed.height
	if !width.set || !height.set || width.percent != 0 || height.percent != 0 {
		return stdimage.Point{}
	}
	return stdimage.Pt(int(width.pixels), int(height.pixels))
}

func (c *compiler) warn(path, code, message string) {
	warning := Warning{Path: path, Code: code, Message: message}
	if c.warningSink != nil {
		appendWarning(c.warningSink, warning)
		return
	}
	appendWarning(&c.warnings, warning)
}

func appendWarning(target *[]Warning, warning Warning) {
	for _, existing := range *target {
		if existing == warning {
			return
		}
	}
	*target = append(*target, warning)
}

// computed resolves one element's style from its parent's, the stylesheet and
// its own style attribute.
func (c *compiler) computed(node *html.Node, parent style, path string) style {
	if cached, ok := c.computedFor[node]; ok {
		return cached
	}
	inherited := parent.inherited()
	current := inherited
	current.display = displayBlock
	current.inline = inlineByDefault[node.Data]
	current.shrink = 1 // flex-shrink's CSS initial value; it is not inherited.
	externalSVG := isExternalSVGImage(node)
	if externalSVG {
		// A replaced SVG image owns a separate document. Its page-level paint
		// properties neither cascade into that document nor affect the image
		// box, so do not let them enter the computed style in the first place.
		current.fill, current.stroke, current.strokeWidth = nil, nil, nil
		current.lineCap, current.lineJoin = "", ""
		current.line, current.dash, current.dashOffset = borderNone, nil, 0
	}
	// Children of a flex or grid container are blockified: being an item in
	// one settles the question before the element's own default gets a say.
	// Grid was missing, so a span in a grid cell was still inline and every
	// property that only means something on a box did nothing to it.
	if parent.display == displayFlex || parent.display == displayGrid {
		current.inline = false
		current.blockified = true
	}
	// Custom properties are resolved against the chain this element sits in,
	// which is how they inherit: an ancestor's declaration is in scope here
	// because that ancestor matched too.
	declared := c.variablesFor(node)
	// A style attribute the parser refuses loses everything the author wrote
	// in it, and saying nothing about that leaves them looking for a rule that
	// never ran.
	if _, err := inlineDeclarations(node); err != nil {
		c.warn(path, "unreadable-rule", fmt.Sprintf(
			"%s: the style attribute was skipped: %v. Everything the stylesheet says still applies",
			describe(node), err))
	}
	for _, applied := range c.sheet.declarationsFor(node) {
		property, value := applied.property, applied.value
		if externalSVG && externalSVGPaintProperties[property] {
			continue
		}
		if strings.Contains(value, "var(") {
			substituted, problem := c.sheet.substitute(value, declared)
			if problem != "" {
				c.warn(path, "unsupported-declaration",
					fmt.Sprintf("%s: %s: %s", describe(node), property, problem))
				continue
			}
			value = substituted
		}
		// The whole parent style, not the inheriting subset: inherit means
		// "take the parent's value" for any property, and display: inherit
		// has to be able to see a display.
		current.apply(property, value, parent, func(message string) {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("%s: %s", describe(node), message))
		})
	}
	// A line-height written as a number is a ratio, and the font size it is a
	// ratio of is only settled now that every declaration is in. Working it
	// out as each declaration arrived made the answer depend on the order the
	// two were written in.
	if current.lineHeightMultiple > 0 {
		current.lineHeight = int(float64(current.fontSize)*current.lineHeightMultiple + 0.5)
		if current.lineHeightResolvesHere {
			current.lineHeightMultiple, current.lineHeightResolvesHere = 0, false
		}
	}
	// Taking an element out of the flow blockifies it, as it does in CSS.
	// Without this an absolutely positioned span was still inline, so its
	// parent read it as a line of text and its position, size and turn went
	// with the box that was never built.
	if current.absolute {
		current.inline = false
	}
	// vertical-align is an inline-level property. On anything else CSS simply
	// does not apply it, and a declaration that does nothing has to say so —
	// this used to align the text inside the box instead, which is a thing
	// CSS has no way to ask for and a page written here could not carry to a
	// browser. Centring a box's contents is align-items on its container.
	if current.blockified || (!current.inline && !current.inlineAtomic) {
		for _, applied := range c.sheet.declarationsFor(node) {
			if applied.property != "vertical-align" {
				continue
			}
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"%s: vertical-align applies to inline-level boxes, so it has no effect here; "+
					"to place a box's contents use align-items on its container, or align-self on it",
				describe(node)))
			break
		}
	}
	c.computedFor[node] = current
	return current
}

// variablesFor gathers the custom properties in scope for one element: those
// declared on it, and those declared on anything it descends from.
//
// Two orders are at work and they are not the same one. Between elements the
// nearest wins outright, because that is what inheriting means, and the walk
// goes outward so a name already set is left alone. Within one element the
// ordinary cascade decides, which is what customFor runs: an id beats a class,
// a later rule beats an earlier one of equal weight, the style attribute beats
// every selector, and important beats all of it.
func (c *compiler) variablesFor(node *html.Node) map[string]string {
	declared := map[string]string{}
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type != html.ElementNode {
			continue
		}
		here := map[string]string{}
		for _, applied := range c.sheet.customFor(ancestor) {
			here[applied.property] = applied.value
		}
		for name, value := range here {
			if _, taken := declared[name]; !taken {
				declared[name] = value
			}
		}
	}
	return declared
}

// element compiles one element and reports the flex weights its parent should
// give it.
func (c *compiler) element(node *html.Node, parent style, path string, containing *childBoxes) (*emitted, style) {
	current := c.computed(node, parent, path)
	if current.display == displayNone {
		return nil, current
	}
	// An image has no descendants that could override visibility, so keeping its
	// box as a spacer is enough. Ordinary boxes and SVGs still have to walk their
	// children because a descendant may explicitly say visible.
	if current.hidden && node.Data == "img" {
		return sized(&emitted{Type: "spacer"}, current), current
	}

	if node.Namespace == svgNamespace && node.Data == "svg" {
		// A drawing is content rather than a container, and the element around
		// it still sizes, clips and transforms it.
		drawing := c.svg(node, current, path)
		if drawing == nil && current.hidden {
			// visibility:hidden preserves the SVG box even when every shape
			// underneath it is hidden.
			return sized(&emitted{Type: "spacer"}, current), current
		}
		return relatively(sized(transformed(shaped(drawing, current), current), current), current), current
	}
	if node.Data == "img" {
		// An image is content rather than a container, but it is still an
		// element: clipping and transforming apply to it exactly as they do
		// to anything else, and returning here without them made a circular
		// portrait come out square without a word about it.
		return relatively(sized(transformed(shaped(c.image(node, current, path), current), current), current), current), current
	}
	// The element's own paint goes even though its children are still walked,
	// because one of them may say visible.
	if current.hidden {
		current.background, current.border = nil, nil
	}
	inner, insetAlready := c.children(node, current, path, containing)
	if inner == nil && (current.background == nil && !hasBorder(current) || current.hasBoxEdges()) {
		// An element with nothing to paint still has a box. Dropping it
		// shifted everything after it along, which in a grid meant a spacer
		// element vanished and the cell it was holding was taken by the next
		// child. Removing the box is what display: none is for, and that is
		// handled above.
		inner = &emitted{Type: "spacer"}
	}

	// Padding wraps whatever the children produced, unless they wrapped it
	// themselves: a container with an absolutely positioned child insets the
	// line and leaves the child alone, which is what padded does there.
	if !insetAlready {
		inner = padded(inner, current)
	}

	// A background and a border are layers under the content, which is what a
	// stack is for. An element with neither is just its content.
	var layers []*emitted
	if current.background != nil {
		layers = append(layers, &emitted{Type: "rectangle",
			Fill: inkName(*current.background), Radius: radiusOf(current)})
	}
	// A border with no style is not drawn, which is what CSS's initial
	// border-style of none means and what a style this cannot draw falls back
	// to. Drawing it as solid instead is the one thing that must not happen:
	// the page would show a line the author asked not to have, or asked for in
	// a shape this has no way to make.
	if edge, sides := borderStrokes(current); edge != nil || sides != ([4]*stroke{}) {
		borderLayer := &emitted{Type: "rectangle", Radius: radiusOf(current), Stroke: edge,
			StrokeTop: sides[0], StrokeRight: sides[1], StrokeBottom: sides[2], StrokeLeft: sides[3]}
		layers = append(layers, borderLayer)
	}
	// overflow confines what is inside the element; the element's own
	// background and border are drawn regardless. clip-path is the other way
	// round: it shapes the element itself, so it is applied further out.
	if current.clip && inner != nil {
		if current.border != nil && current.border.radius > 0 {
			// Clipping to the box means clipping to the box as drawn, and a
			// rounded border draws a rounded box.
			inner = &emitted{Type: "clipShape", Child: inner, Shape: shapeOf(compose.Shape{
				Kind: compose.ShapeInset, Corner: compose.Pixels(current.border.radius),
			})}
		} else {
			inner = &emitted{Type: "clip", Child: inner}
		}
	}
	if len(layers) == 0 {
		return relatively(sized(transformed(shaped(inner, current), current), current), current), current
	}
	if inner != nil {
		layers = append(layers, inner)
	}
	return relatively(sized(transformed(shaped(&emitted{Type: "stack", Children: layers}, current), current), current), current), current
}

// shaped confines the whole element to its clip-path, background and border
// included, which is what clip-path means: the element is that shape.
func shaped(node *emitted, s style) *emitted {
	if node == nil || s.clipShape.Kind == compose.ShapeNone {
		return node
	}
	return &emitted{Type: "clipShape", Shape: shapeOf(s.clipShape), Child: node}
}

// transformed wraps a node when the style asks for a magnification or a turn.
// The wrapper draws the subtree onto a surface of its own, so everything under
// it moves together rather than each shape being redrawn at a new size.
// transformed wraps a node in whichever of the two transforms it asked for.
//
// They are separate nodes because they are separate things. A magnification
// redraws the subtree onto a larger surface, which is exact only in whole
// numbers. A turn puts a turn into the drawing state and everything under it
// works out where its own geometry goes, which is exact at any angle for
// anything drawn from geometry — and resampled, as it has to be, for a glyph
// or a picture.
//
// The turn goes outside the magnification, so a box magnified and turned is
// turned as it ends up rather than as it was written.
func transformed(node *emitted, s style) *emitted {
	if node == nil {
		return node
	}
	if (s.transform != display.Transform{}) {
		node = &emitted{Type: "transformed",
			Scale: s.transform.Scale, Turns: s.transform.Turns, Child: node}
	}
	if s.rotate == 0 {
		return node
	}
	turn := &emitted{Type: "rotated", Degrees: s.rotate, Child: node}
	if s.rotateOrigin != nil {
		turn.Origin = &origin{
			X: lengthValue(lengthOf(s.rotateOrigin[0])),
			Y: lengthValue(lengthOf(s.rotateOrigin[1])),
		}
	}
	return turn
}

// relatively keeps the element in its parent's flow while moving the whole
// painted subtree. The scene decoder resolves these lengths against the
// containing box once the layout has produced one.
func relatively(node *emitted, s style) *emitted {
	if node == nil || !s.positioned || s.absolute || !hasInset(s) {
		return node
	}
	return &emitted{
		Type:   "relative",
		Top:    lengthValue(lengthOf(s.inset[0])),
		Right:  lengthValue(lengthOf(s.inset[1])),
		Bottom: lengthValue(lengthOf(s.inset[2])),
		Left:   lengthValue(lengthOf(s.inset[3])),
		Child:  node,
	}
}

func radiusOf(s style) int {
	if s.border == nil {
		return 0
	}
	return s.border.radius
}

func hasBorder(s style) bool {
	for index := 0; index < 4; index++ {
		if s.effectiveBorderWidth(index) > 0 {
			return true
		}
	}
	return false
}

func borderStrokes(s style) (*stroke, [4]*stroke) {
	var sides [4]*stroke
	for index := range sides {
		side := s.effectiveBorderSide(index)
		if side.width <= 0 || side.line == borderNone {
			continue
		}
		value := &stroke{Ink: inkName(side.ink), Width: side.width,
			Dash: side.line.dashOf(side.width), DashOffset: side.dashOffset}
		if len(side.dash) > 0 {
			value.Dash = side.dash
		}
		sides[index] = value
	}
	if sides[0] != nil && strokeEqual(sides[0], sides[1]) &&
		strokeEqual(sides[0], sides[2]) && strokeEqual(sides[0], sides[3]) {
		return sides[0], [4]*stroke{}
	}
	return nil, sides
}

func strokeEqual(left, right *stroke) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Ink == right.Ink && left.Width == right.Width && left.DashOffset == right.DashOffset &&
		left.Cap == right.Cap && left.Join == right.Join && slices.Equal(left.Dash, right.Dash)
}

// sized applies an explicit width or height, which compose expresses as a
// node's preferred size.
func sized(node *emitted, s style) *emitted {
	if node == nil {
		return nil
	}
	if !s.width.set && !s.height.set {
		return node
	}
	if s.absolute {
		// An absolutely positioned box is sized by the anchor that places it,
		// from these same two lengths. Saying it twice was harmless while the
		// layout took the tree straight from here and ignored one of them;
		// the schema refuses a placed node that also states a size, and is
		// right to — two sizes on one box is a question about which wins.
		return node
	}
	// A stated size is the content's under CSS's default box-sizing, so the
	// box around it is that much larger. border-box states the box itself.
	width, height := s.outerSize(s.width, true), s.outerSize(s.height, false)
	size := stdimage.Point{}
	if width.fixed() {
		size.X = width.px()
	}
	if height.fixed() {
		size.Y = height.px()
	}
	if size == (stdimage.Point{}) {
		return node
	}
	// A stack that states no size of its own can carry this one, which spares
	// every page a wrapper around its root: an element with both a size and a
	// background already produced a stack for the background to sit under.
	if node.Type == "stack" && node.Size == nil && node.Raw == nil {
		node.Size = sizeOf(size)
		return node
	}
	return &emitted{Type: "stack", Size: sizeOf(size), Children: []*emitted{node}}
}

// textContent gathers an element's text without any of the white-space
// handling the layout needs, because a description is read rather than drawn.
func textContent(node *html.Node) string {
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			builder.WriteString(child.Data)
		case html.ElementNode:
			builder.WriteString(textContent(child))
		}
	}
	return builder.String()
}

// image writes an img element out as the picture it names.
//
// It does not open the file. Whoever reads the document this produces has a
// loader already, with a limit on how many pixels it will decode, a list of
// the two formats it accepts, and a rule about where a path may point. This
// package having a second loader meant a picture arriving through a
// stylesheet was held to none of it, which is the sort of difference that is
// invisible until it is a hole.
func (c *compiler) image(node *html.Node, current style, path string) *emitted {
	source := attribute(node, "src")
	if source == "" {
		c.warn(path, "unsupported-declaration", "img has no src")
		return nil
	}
	// An img naming a drawing is a drawing, which is what the tag means
	// everywhere else. It is compiled here rather than left for the decoder
	// because a drawing is markup and the decoder reads documents.
	if isExternalSVGImage(node) {
		c.warnExternalSVGPaint(node, path)
		return c.externalDrawing(source, current, path)
	}
	picture := &emitted{Type: "image", Source: source, Processing: "auto"}
	if current.objectFit != nil {
		picture.Overrides = &overrides{Fit: fitName(*current.objectFit)}
	}
	return picture
}

func isExternalSVGImage(node *html.Node) bool {
	return node.Data == "img" && strings.EqualFold(pathExtension(attribute(node, "src")), ".svg")
}

var externalSVGPaintProperties = map[string]bool{
	"fill":              true,
	"stroke":            true,
	"stroke-width":      true,
	"stroke-dasharray":  true,
	"stroke-dashoffset": true,
	"stroke-linecap":    true,
	"stroke-linejoin":   true,
}

// warnExternalSVGPaint makes the resource boundary visible. These properties
// are valid page CSS, but an external SVG in an img is a replaced document and
// page selectors do not cascade into it. declarationsFor is used instead of
// the computed style so a declaration that was accepted but later overridden
// is still not silently lost.
func (c *compiler) warnExternalSVGPaint(node *html.Node, path string) {
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type != html.ElementNode {
			continue
		}
		for _, applied := range c.sheet.declarationsFor(ancestor) {
			if !externalSVGPaintProperties[applied.property] {
				continue
			}
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"%s on an external SVG image has no effect; page CSS does not cascade into <img> documents",
				applied.property))
		}
	}
}

// externalDrawing compiles a drawing that sits in its own file.
//
// It is read through the same resolver a stylesheet is, and for the same
// reason: which files may be read is a question about where the page came
// from, and this package does not know that.
func (c *compiler) externalDrawing(source string, current style, path string) *emitted {
	var read []byte
	var err error
	if isRemoteSource(source) {
		read, err = fetchRemoteDrawing(source)
	} else if c.drawings == nil {
		c.warn(path, "unresolved-drawing", fmt.Sprintf(
			"img src=%q was not drawn: this compiler was given no way to read one", source))
		return nil
	} else {
		read, err = c.drawings(source)
	}
	if err != nil {
		c.warn(path, "unresolved-drawing", fmt.Sprintf("img src=%q: %v", source, err))
		return nil
	}
	root, err := html.Parse(strings.NewReader(string(read)))
	if err != nil {
		c.warn(path, "unresolved-drawing", fmt.Sprintf("img src=%q: %v", source, err))
		return nil
	}
	element := findSVG(root)
	if element == nil {
		c.warn(path, "unresolved-drawing", fmt.Sprintf("img src=%q holds no svg element", source))
		return nil
	}
	// An external SVG is the document of an image, not a subtree of this page.
	// Keep the SVG backend, but give it a fresh stylesheet and style cache so
	// page selectors and inherited paints cannot enter through the resource
	// boundary. Warnings still belong to the page compilation.
	isolated := *c
	isolated.sheet = &stylesheet{}
	isolated.computedFor = map[*html.Node]style{}
	isolated.warningSink = &c.warnings
	resourcePath := fmt.Sprintf("%s<%s>", path, source)
	resourceStyle := isolated.computed(element, rootStyle(), resourcePath)
	// The image's CSS dimensions are the outer replaced box, but the vector
	// backend still needs them to map the resource viewport into that box. Copy
	// only those dimensions; paints and all other page properties stay outside.
	if current.width.set {
		resourceStyle.width = current.width
	}
	if current.height.set {
		resourceStyle.height = current.height
	}
	return isolated.svg(element, resourceStyle, resourcePath)
}

func isRemoteSource(source string) bool {
	parsed, err := url.Parse(source)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func fetchRemoteDrawing(source string) ([]byte, error) {
	response, err := remoteDrawingClient.Get(source)
	if err != nil {
		return nil, fmt.Errorf("fetch drawing: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch drawing: HTTP %s", response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteDrawingBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read drawing response: %w", err)
	}
	if len(content) > maxRemoteDrawingBytes {
		return nil, fmt.Errorf("drawing response exceeds the %d byte limit", maxRemoteDrawingBytes)
	}
	return content, nil
}

// findSVG is the drawing inside a file that holds one, which the HTML parser
// will have put in the body whatever the file looked like.
func findSVG(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.Namespace == svgNamespace && node.Data == "svg" {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findSVG(child); found != nil {
			return found
		}
	}
	return nil
}

// pathExtension is the suffix of a src, without needing the file to exist or
// this package to know what a path is.
func pathExtension(source string) string {
	if parsed, err := url.Parse(source); err == nil && parsed.Path != "" {
		source = parsed.Path
	}
	dot := strings.LastIndexByte(source, '.')
	slash := strings.LastIndexAny(source, "/\\")
	if dot < 0 || dot < slash {
		return ""
	}
	return source[dot:]
}

// children lays out an element's children, choosing between a run of text and
// a flex line by what the children are.
// children also reports whether it has already applied the element's padding,
// which it does when it has laid an anchored layer over the content: an
// absolutely positioned child is placed against the padding box and not inside
// it, so the padding has to go round the line rather than round both.
func (c *compiler) children(node *html.Node, current style, path string, containing *childBoxes) (*emitted, bool) {
	// A flex container lays out boxes, never a line of text, so the inline
	// path is not even attempted for one.
	if current.display != displayFlex && c.inlineCompatible(node, current, path) {
		if items, ok, special := c.inlineItems(node, current, path, containing); ok && special {
			return &emitted{Type: "inline", Items: items,
				Align: horizontalAlignName(current.textAlign),
				Wrap:  wrapName(current.wrap), LineHeight: current.lineHeight}, false
		}
		if runs := c.textRuns(node, current, path); len(runs) > 0 {
			// Text sits at the top of its box, as it does in CSS. Centring it
			// vertically is what vertical-align: middle is for, and the
			// centring is where a half pixel has to go somewhere: a line of
			// Chinese and a line of mixed Chinese and Latin have ink of
			// different heights, so at an even line height the two settled a
			// pixel apart. At the top they cannot.
			return &emitted{Type: "text", Runs: runs,
				Align: horizontalAlignName(current.textAlign),
				Wrap:  wrapName(current.wrap), LineHeight: current.lineHeight}, false
		}
	}

	var boxes childBoxes
	childContaining := containing
	if childContaining == nil || current.positioned {
		childContaining = &boxes
	}
	c.appendChildren(node, current, path, &boxes, childContaining)
	items, cells, placed := boxes.items, boxes.cells, boxes.placed

	// gap only means something between the children of a flex line or a grid,
	// so a block container that declares one is asking for something it will
	// not get.
	if current.display == displayBlock && current.gapSet {
		c.warn(path, "unsupported-declaration",
			"gap has no effect on a block container; it spaces flex and grid children")
	}

	flow := func(line *emitted) (*emitted, bool) {
		line = padded(line, current)
		if len(placed) == 0 {
			return line, line != nil
		}
		layered := anchoredIn(placed, current)
		if line == nil {
			return layered, true
		}
		return &emitted{Type: "stack", Children: []*emitted{line, layered}}, true
	}

	if current.display == displayGrid {
		if len(cells) == 0 {
			return flow(nil)
		}
		return flow(&emitted{Type: "grid",
			Columns: tracksOf(current.columns), Rows: tracksOf(current.rows),
			ColumnGap: current.columnGap, RowGap: current.rowGap,
			ColumnGapLength: deferredLengthValue(lengthOf(current.columnGapLength)),
			RowGapLength:    deferredLengthValue(lengthOf(current.rowGapLength)),
			AlignItems:      crossAlignName(current.alignItems),
			JustifyItems:    crossAlignName(crossOfMain(current.justify)),
			Children:        cells,
		})
	}
	if current.display == displayFlex {
		if current.spaceEvenly {
			items = spread(items)
		}
		cross := current.alignItems
		if current.direction == axisColumn {
			return flow(&emitted{Type: "column", Gap: current.rowGap, GapLength: deferredLengthValue(lengthOf(current.rowGapLength)),
				MainAlign: mainAlignName(current.justify), CrossAlign: crossAlignName(cross),
				Children: items})
		}
		return flow(&emitted{Type: "row", Gap: current.columnGap, GapLength: deferredLengthValue(lengthOf(current.columnGapLength)),
			MainAlign: mainAlignName(current.justify), CrossAlign: crossAlignName(cross),
			Children: items})
	}
	// Block children stack down the page, which is a column that neither
	// grows nor gaps.
	if len(items) == 0 {
		return flow(nil)
	}
	return flow(&emitted{Type: "column", Children: items})
}

// inlineCompatible is a side-effect-free preflight. It prevents the collector
// from resolving an image or drawing and then resolving it a second time when
// a later sibling turns the parent back into ordinary block flow.
func (c *compiler) inlineCompatible(node *html.Node, current style, path string) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if notContent[child.Data] || child.Data == "br" {
			continue
		}
		childPath := fmt.Sprintf("%s>%s", path, child.Data)
		childStyle := c.computed(child, current, childPath)
		if childStyle.display == displayNone || childStyle.hidden {
			continue
		}
		if childStyle.absolute {
			return false
		}
		if childStyle.inlineAtomic ||
			(child.Data == "img" && childStyle.inline) ||
			(child.Namespace == svgNamespace && child.Data == "svg" && childStyle.inline) {
			continue
		}
		if childStyle.display == displayContents {
			if !c.inlineCompatible(child, contentsContext(current, childStyle), childPath) {
				return false
			}
			continue
		}
		if !childStyle.inline || childStyle.display == displayFlex || childStyle.display == displayGrid {
			return false
		}
		if !c.inlineCompatible(child, childStyle, childPath) {
			return false
		}
	}
	return true
}

// padded insets an element's content by everything between it and the outside
// of the box: the border and then the padding, which is what CSS counts.
func padded(inner *emitted, current style) *emitted {
	edges := current.edgesLengths()
	if inner == nil || !lengthsSet(edges) {
		return inner
	}
	if fixed, ok := fixedInsets(edges); ok {
		return &emitted{Type: "padding", Insets: insetsOf(fixed), Child: inner}
	}
	return &emitted{Type: "padding", LengthInsets: lengthInsetsOf(edges), Child: inner}
}

func fixedInsets(lengths [4]compose.Length) (compose.Insets, bool) {
	values := [4]int{}
	for index, length := range lengths {
		percent, pixels := length.Parts()
		if percent != 0 {
			return compose.Insets{}, false
		}
		values[index] = pixels
	}
	return compose.Insets{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}, true
}

// anchoredIn places the children that were taken out of the flow. They resolve
// against the padding box, so the border insets them and the padding does not.
func anchoredIn(placed []anchor, current style) *emitted {
	layered := &emitted{Type: "anchored", Children: placed}
	edges := current.borderEdges()
	if edges == (compose.Insets{}) {
		return layered
	}
	return &emitted{Type: "padding", Insets: insetsOf(edges), Child: layered}
}

// anchorFor describes where a positioned child sits without deciding it. The
// container's rectangle is only known once the layout runs, so the insets go
// through as they were written and are resolved there.
//
// Both edges and a size on one axis is more than can hold at once, and CSS
// says which one gives: the end edge, so a box that states left, right and
// width keeps the left and the width. The schema refuses all three, and is
// right to for a document somebody wrote by hand — but a stylesheet is not
// refused for saying something CSS has an answer for, so the answer is applied
// here and said.
func (c *compiler) anchorFor(child style, node *emitted, path string) anchor {
	placed := anchor{Node: node, Layer: child.layer}
	top, right, bottom, left := child.inset[0], child.inset[1], child.inset[2], child.inset[3]
	width := child.outerSize(child.width, true)
	height := child.outerSize(child.height, false)
	if left.set && right.set && width.set {
		c.warn(path, "over-constrained",
			overConstrained("left", "right", "width", child.insetFromShorthand[3], child.insetFromShorthand[1]))
		right = length{}
	}
	if top.set && bottom.set && height.set {
		c.warn(path, "over-constrained",
			overConstrained("top", "bottom", "height", child.insetFromShorthand[0], child.insetFromShorthand[2]))
		bottom = length{}
	}
	placed.Top = lengthValue(lengthOf(top))
	placed.Right = lengthValue(lengthOf(right))
	placed.Bottom = lengthValue(lengthOf(bottom))
	placed.Left = lengthValue(lengthOf(left))
	placed.Width = lengthValue(lengthOf(width))
	placed.Height = lengthValue(lengthOf(height))
	return placed
}

// overConstrained names the three that cannot all hold, and says which of them
// came from inset — an author who wrote "inset: 0" has not written the word
// "right" anywhere and would otherwise be told about a property they never
// used. One who then wrote "left: 10px" did write that one, so it is theirs.
func overConstrained(start, end, of string, startFromInset, endFromInset bool) string {
	from := ""
	switch {
	case startFromInset && endFromInset:
		from = fmt.Sprintf(" (%s and %s come from the inset shorthand)", start, end)
	case startFromInset:
		from = fmt.Sprintf(" (%s comes from the inset shorthand)", start)
	case endFromInset:
		from = fmt.Sprintf(" (%s comes from the inset shorthand)", end)
	}
	return fmt.Sprintf(
		"%s, %s and %s cannot all hold at once%s; %s was dropped, as CSS drops it",
		start, end, of, from, end)
}

// childBoxes is where one container's children accumulate. They are collected
// separately because a container places them differently: a flex line, a grid
// cell, or a box anchored to the container's edges.
type childBoxes struct {
	items  []layoutChild
	cells  []gridChild
	placed []anchor
	index  int
}

// contentsContext keeps the outer container's layout rules while carrying the
// computed values that display: contents would otherwise pass to its children.
// A contents element has no box of its own, but its inherited paint and text
// properties still apply to the nodes it exposes.
func contentsContext(container, contents style) style {
	next := container
	inherited := contents.inherited()
	next.fill, next.stroke, next.strokeWidth = inherited.fill, inherited.stroke, inherited.strokeWidth
	next.color = inherited.color
	next.fontFamily, next.fontFallback, next.fontSize = inherited.fontFamily, slices.Clone(inherited.fontFallback), inherited.fontSize
	next.textAlign = inherited.textAlign
	next.lineHeight, next.lineHeightMultiple = inherited.lineHeight, inherited.lineHeightMultiple
	next.wrap, next.preserve, next.hidden = inherited.wrap, inherited.preserve, inherited.hidden
	next.lineCap, next.lineJoin = inherited.lineCap, inherited.lineJoin
	return next
}

func hasInset(s style) bool {
	for _, edge := range s.inset {
		if edge.set {
			return true
		}
	}
	return false
}

// appendChildren walks one element's children into the collection. It calls
// itself for an element with display: contents, whose children belong to this
// container rather than to a box of its own.
func (c *compiler) appendChildren(node *html.Node, current style, path string, into, containing *childBoxes) {
	// The axis the children actually run along. direction only means anything
	// on a flex container; a block container stacks down the page whatever it
	// says, and reading direction straight off it left every block at the row
	// default, which sent a left margin into the gap above the box.
	flow := current.direction
	if current.display != displayFlex {
		flow = axisColumn
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			if current.hidden {
				continue
			}
			// Text sitting directly among boxes becomes a box of its own, as
			// CSS puts it in an anonymous item. Skipping it dropped the units
			// out of "<b>412</b>h<small>hours</small>" without a word.
			text := c.spacing(child.Data, current, false)
			if text == "" {
				continue
			}
			anonymous := &emitted{Type: "text",
				Runs:       plainRuns([]run{c.runOf(text, current, path)}),
				Wrap:       wrapName(current.wrap),
				LineHeight: current.lineHeight,
			}
			if current.display == displayGrid {
				into.cells = append(into.cells, gridChild{Node: anonymous})
				continue
			}
			into.items = append(into.items, layoutChild{Node: anonymous})
			continue
		}
		if child.Type != html.ElementNode || notContent[child.Data] {
			continue
		}
		childPath := fmt.Sprintf("%s>%s[%d]", path, child.Data, into.index)
		into.index++
		childStyle := c.computed(child, current, childPath)
		if childStyle.display == displayContents {
			c.appendChildren(child, contentsContext(current, childStyle), childPath, into, containing)
			continue
		}
		compiled, childStyle := c.element(child, current, childPath, containing)
		if compiled == nil {
			continue
		}
		// An absolutely positioned child is taken out of the line and placed
		// against the container's own box.
		if childStyle.absolute {
			target := containing
			if target == nil {
				target = into
			}
			target.placed = append(target.placed, c.anchorFor(childStyle, compiled, childPath))
			continue
		}
		if current.display == displayGrid {
			into.cells = append(into.cells, gridCell(compiled, childStyle))
			continue
		}
		// margin-left:auto on a row, or margin-top:auto on a column, pushes
		// this child and everything after it to the far end. A spacer that
		// takes all the slack is how compose says the same thing.
		if current.display == displayFlex && ((flow == axisRow && childStyle.autoLeft) ||
			(flow == axisColumn && childStyle.autoTop)) {
			into.items = append(into.items, grower())
		}
		into.items = append(into.items, c.layoutChild(compiled, childStyle, current, childPath))
		if current.display == displayFlex && ((flow == axisRow && childStyle.autoRight) ||
			(flow == axisColumn && childStyle.autoBottom)) {
			into.items = append(into.items, grower())
		}
	}
}

// grower is the spacer that takes whatever room is left, which is how a
// margin of auto and a space-between are both said.
func grower() layoutChild {
	return layoutChild{Node: &emitted{Type: "spacer"}, Grow: 1}
}

// gridCell carries a child's own placement into the grid, where a line number
// and a span are the whole of what a cell needs to know.
func gridCell(node *emitted, child style) gridChild {
	return gridChild{
		Node:        node,
		Column:      child.cellColumn[0],
		ColumnSpan:  child.cellColumn[1],
		Row:         child.cellRow[0],
		RowSpan:     child.cellRow[1],
		AlignSelf:   optionalCrossAlignName(child.alignSelf),
		JustifySelf: optionalCrossAlignName(child.justifySelf),
		Margin:      lengthInsetsOf(styleLengths(child.marginLength, child.margin)),
	}
}

// spread puts a growing spacer between each pair of items, which is what
// space-between describes: the slack goes into the gaps rather than the ends.
func spread(items []layoutChild) []layoutChild {
	if len(items) < 2 {
		return items
	}
	spaced := make([]layoutChild, 0, len(items)*2-1)
	for index, item := range items {
		if index > 0 {
			spaced = append(spaced, grower())
		}
		spaced = append(spaced, item)
	}
	return spaced
}

// layoutChild hands the child's stated sizes to the layout as lengths, which
// is where they can finally be resolved. Nothing here decides a pixel.
func (c *compiler) layoutChild(node *emitted, child, parent style, path string) layoutChild {
	along := parent.direction
	if parent.display != displayFlex {
		// Block children stack down the page whatever the container says.
		along = axisColumn
	}
	item := layoutChild{
		Node:      node,
		AlignSelf: optionalCrossAlignName(child.alignSelf), Ratio: child.ratio,
	}
	// Only flex items participate in flex factor distribution. Block flow is
	// represented by the same column node, but its children keep their measured
	// sizes and overflow just as ordinary block boxes do.
	if parent.display == displayFlex {
		item.Grow, item.Shrink = child.grow, child.shrink
	}
	item.Margin = lengthInsetsOf(styleLengths(child.marginLength, child.margin))

	// Every stated size is the content's unless the box says border-box, so
	// the item the line is given has to be the box around it. flex-basis is
	// sized the same way a width is, which is what CSS does with it.
	mainSize, crossSize := child.outerSize(child.width, true), child.outerSize(child.height, false)
	minMain, minCross := child.outerSize(child.minSize[0], true), child.outerSize(child.minSize[1], false)
	maxMain, maxCross := child.outerSize(child.maxSize[0], true), child.outerSize(child.maxSize[1], false)
	if along == axisColumn {
		mainSize, crossSize = crossSize, mainSize
		minMain, minCross = minCross, minMain
		maxMain, maxCross = maxCross, maxMain
	}
	item.Basis = lengthValue(lengthOf(mainSize))
	if child.basis.set {
		item.Basis = lengthValue(lengthOf(child.outerSize(child.basis, along == axisRow)))
	}
	item.Cross = lengthValue(lengthOf(crossSize))
	item.MinMain, item.MaxMain = lengthValue(lengthOf(minMain)), lengthValue(lengthOf(maxMain))
	item.MinCross, item.MaxCross = lengthValue(lengthOf(minCross)), lengthValue(lengthOf(maxCross))
	return item
}

func styleLengths(lengths [4]length, fallback compose.Insets) [4]compose.Length {
	result := [4]compose.Length{}
	values := [4]int{fallback.Top, fallback.Right, fallback.Bottom, fallback.Left}
	for index, value := range lengths {
		if value.set {
			result[index] = lengthOf(value)
		} else if values[index] != 0 {
			result[index] = compose.Pixels(values[index])
		}
	}
	return result
}

func styleLengthsSet(lengths [4]length) bool {
	for _, length := range lengths {
		if length.set {
			return true
		}
	}
	return false
}

// lengthOf carries a stated size through unresolved. Percentages are kept in
// tenths so that a figure like 87.3% survives the trip.
func lengthOf(size length) compose.Length {
	if !size.set {
		return compose.Length{}
	}
	return compose.Calc(int(size.percent*10), int(size.pixels))
}

// textRuns collects the inline content of an element. It returns nothing when
// the element has element children that are not inline, which is how a box
// with boxes inside it is told from a box with words inside it.
func (c *compiler) textRuns(node *html.Node, current style, path string) []run {
	started := false
	return plainRuns(c.collectRuns(node, current, path, &started))
}

// inlineItems is the real inline-formatting collector. It keeps the legacy
// text-run fast path available for ordinary text, but switches to fragment
// boxes as soon as HTML contains an atomic inline node or box-affecting style.
func (c *compiler) inlineItems(node *html.Node, current style, path string, containing *childBoxes) ([]inlineItem, bool, bool) {
	started := false
	items, ok, special := c.collectInline(node, current, path, containing, &started, false, nil)
	return items, ok, special
}

func (c *compiler) collectInline(node *html.Node, current style, path string, containing *childBoxes, started *bool, boxed bool, boxStyle *style) ([]inlineItem, bool, bool) {
	var items []inlineItem
	special := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if current.hidden {
				continue
			}
			text := c.spacing(child.Data, current, *started)
			if text == "" {
				continue
			}
			*started = true
			effectiveBox := current
			hasBox := boxed && inlineBoxStyle(current)
			if boxStyle != nil {
				effectiveBox = mergeInlineBox(*boxStyle, current)
				hasBox = true
			}
			item := c.inlineTextItem(text, current, effectiveBox, hasBox, path)
			items = append(items, item)
			if hasBox {
				special = true
			}
		case html.ElementNode:
			if notContent[child.Data] {
				continue
			}
			childPath := fmt.Sprintf("%s>%s", path, child.Data)
			if child.Data == "br" {
				items = append(items, inlineItem{Break: true})
				*started = false
				special = true
				continue
			}
			childStyle := c.computed(child, current, childPath)
			if childStyle.display == displayNone {
				continue
			}
			if childStyle.hidden {
				c.warn(childPath, "unsupported-declaration", fmt.Sprintf(
					"%s: visibility: hidden on text inside a line drops the words rather than keeping the space they took", describe(child)))
				continue
			}
			if childStyle.absolute {
				return nil, false, false
			}
			if childStyle.inlineAtomic ||
				(child.Data == "img" && childStyle.inline) ||
				(child.Namespace == svgNamespace && child.Data == "svg" && childStyle.inline) {
				compiled, _ := c.element(child, current, childPath, containing)
				if compiled == nil {
					continue
				}
				items = append(items, c.inlineNodeItem(compiled, childStyle))
				*started = true
				special = true
				continue
			}
			if childStyle.display == displayContents {
				nested, ok, nestedSpecial := c.collectInline(child, contentsContext(current, childStyle), childPath, containing, started, boxed, boxStyle)
				if !ok {
					return nil, false, false
				}
				items = append(items, nested...)
				special = special || nestedSpecial
				continue
			}
			if !childStyle.inline || childStyle.display == displayFlex || childStyle.display == displayGrid {
				return nil, false, false
			}
			nextBox := boxStyle
			if inlineBoxStyle(childStyle) {
				merged := childStyle
				if boxStyle != nil {
					merged = mergeInlineBox(*boxStyle, childStyle)
				}
				nextBox = &merged
			}
			nested, ok, nestedSpecial := c.collectInline(child, childStyle, childPath, containing, started, true, nextBox)
			if !ok {
				return nil, false, false
			}
			if len(nested) == 0 && inlineBoxStyle(childStyle) {
				// An empty inline with padding or a background still has a
				// drawable box. A zero-size spacer gives the compose node the
				// same content-box semantics without inventing text.
				empty := c.inlineBoxItem(childStyle)
				empty.Node = &emitted{Type: "spacer"}
				nested = append(nested, empty)
			}
			items = append(items, nested...)
			special = special || nestedSpecial || inlineBoxStyle(childStyle) ||
				childStyle.wrap != current.wrap || childStyle.lineHeight != current.lineHeight
		}
	}
	return items, true, special
}

func (c *compiler) inlineTextItem(text string, current, box style, boxed bool, path string) inlineItem {
	item := c.inlineBoxItem(box)
	item.LineHeight = current.lineHeight
	item.VerticalAlign = inlineVerticalAlignName(current.inlineVAlign)
	item.Wrap = wrapName(current.wrap)
	item.Runs = []run{c.runOf(text, current, path)}
	if !boxed {
		item.Padding = nil
		item.Margin = nil
		return item
	}
	return item
}

func mergeInlineBox(outer, inner style) style {
	merged := outer
	if inner.padding != (compose.Insets{}) {
		merged.padding = inner.padding
	}
	if styleLengthsSet(inner.paddingLength) {
		merged.paddingLength = inner.paddingLength
	}
	if inner.margin != (compose.Insets{}) {
		merged.margin = inner.margin
	}
	if styleLengthsSet(inner.marginLength) {
		merged.marginLength = inner.marginLength
	}
	if inner.background != nil {
		merged.background = inner.background
	}
	if inner.lineCap != "" {
		merged.lineCap = inner.lineCap
	}
	if inner.lineJoin != "" {
		merged.lineJoin = inner.lineJoin
	}
	if inner.border != nil {
		merged.border, merged.line, merged.dash, merged.dashOffset = inner.border, inner.line, inner.dash, inner.dashOffset
	}
	for index, side := range inner.borderSide {
		if side != nil {
			copy := *side
			copy.dash = slices.Clone(side.dash)
			merged.borderSide[index] = &copy
		}
	}
	if inner.positioned {
		merged.positioned, merged.inset = true, inner.inset
	}
	return merged
}

func (c *compiler) inlineBoxItem(current style) inlineItem {
	item := inlineItem{LineHeight: current.lineHeight,
		VerticalAlign: inlineVerticalAlignName(current.inlineVAlign), Wrap: wrapName(current.wrap)}
	item.Padding = insetsOf(current.padding)
	if styleLengthsSet(current.paddingLength) {
		// The dynamic representation includes any fixed calc adjustment and is
		// resolved by compose; do not validate its unresolved pixel part as a
		// standalone (possibly negative) inset.
		item.Padding = nil
	}
	item.Margin = insetsOf(current.margin)
	item.PaddingLength = deferredLengthInsetsOf(styleLengths(current.paddingLength, current.padding))
	item.MarginLength = deferredLengthInsetsOf(styleLengths(current.marginLength, current.margin))
	if current.background != nil {
		item.Background = inkName(*current.background)
	}
	if edge, sides := borderStrokes(current); edge != nil {
		item.Border = edge
	} else {
		item.BorderTop, item.BorderRight = sides[0], sides[1]
		item.BorderBottom, item.BorderLeft = sides[2], sides[3]
	}
	item.Radius = radiusOf(current)
	item.Top = lengthValue(lengthOf(current.inset[0]))
	item.Right = lengthValue(lengthOf(current.inset[1]))
	item.Bottom = lengthValue(lengthOf(current.inset[2]))
	item.Left = lengthValue(lengthOf(current.inset[3]))
	return item
}

func (c *compiler) inlineNodeItem(node *emitted, current style) inlineItem {
	item := inlineItem{Node: node, Margin: insetsOf(current.margin),
		MarginLength:  deferredLengthInsetsOf(styleLengths(current.marginLength, current.margin)),
		VerticalAlign: inlineVerticalAlignName(current.inlineVAlign)}
	return item
}

func inlineBoxStyle(s style) bool {
	return s.padding != (compose.Insets{}) || s.margin != (compose.Insets{}) ||
		styleLengthsSet(s.paddingLength) || styleLengthsSet(s.marginLength) ||
		s.background != nil || hasBorder(s) ||
		s.positioned ||
		s.inlineVAlign != compose.InlineBaseline
}

// collectRuns walks the inline content of an element. started is shared across
// the whole line, because collapsing whitespace is a property of the line and
// not of any one text node: the space between </b> and the word after it is
// real, and only the space at the very beginning of the line is dropped.
func (c *compiler) collectRuns(node *html.Node, current style, path string, started *bool) []run {
	var runs []run
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if current.hidden {
				continue
			}
			text := c.spacing(child.Data, current, *started)
			if text == "" {
				continue
			}
			*started = true
			runs = append(runs, c.runOf(text, current, path))
		case html.ElementNode:
			if notContent[child.Data] {
				continue
			}
			if child.Data == "br" {
				runs = append(runs, c.runOf("\n", current, path))
				*started = false
				continue
			}
			childStyle := c.computed(child, current, path)
			// display: none takes an element and its content out of the
			// document, and a line of text is content like any other. Without
			// this the words were laid into the run anyway and reached the
			// panel, which is the one failure worse than a wrong layout.
			if childStyle.display == displayNone {
				continue
			}
			// A hidden inline element keeps its space in CSS and loses its
			// paint, and a run has no way to be one without the other. The
			// words go, and that is said rather than done quietly.
			if childStyle.hidden {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"%s: visibility: hidden on text inside a line drops the words rather than "+
						"keeping the space they took", describe(child)))
				continue
			}
			if !childStyle.inline || childStyle.display == displayFlex {
				return nil
			}
			nested := c.collectRuns(child, childStyle, path, started)
			if nested == nil && child.FirstChild != nil {
				return nil
			}
			runs = append(runs, nested...)
		}
	}
	return runs
}

// spacing applies the white-space rule in force. Preserved text keeps the
// source's own runs of spaces, which is the only way a page that lines its
// columns up with them can be written at all.
func (c *compiler) spacing(text string, current style, started bool) string {
	if current.preserve {
		return strings.Trim(text, "\n")
	}
	return collapse(text, started)
}

// collapse applies the part of white-space processing that matters here: runs
// of spaces and newlines become one space, so indented markup does not become
// indented text. started says whether anything has been written on this line
// yet, which is the only thing that decides whether leading space survives.
func collapse(text string, started bool) string {
	var builder strings.Builder
	space := false
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space && (started || builder.Len() > 0) {
			builder.WriteByte(' ')
		}
		space = false
		builder.WriteRune(r)
	}
	if space && (started || builder.Len() > 0) {
		builder.WriteByte(' ')
	}
	return builder.String()
}

func describe(node *html.Node) string {
	name := node.Data
	if class := attribute(node, "class"); class != "" {
		name += "." + strings.ReplaceAll(class, " ", ".")
	}
	return name
}

func findBody(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.Data == "body" {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findBody(child); found != nil {
			return found
		}
	}
	return nil
}

func firstElement(node *html.Node) *html.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			return child
		}
	}
	return nil
}
