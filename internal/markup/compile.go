package markup

import (
	"fmt"
	stdimage "image"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"golang.org/x/net/html"
)

// Warning names an element and says what this renderer could not do with it.
type Warning struct {
	Path    string
	Code    string
	Message string
}

// Document is a compiled page: the root node and everything the compiler could
// not honour on the way there.
type Document struct {
	Root     compose.Node
	Warnings []Warning
}

// inlineByDefault are the elements that join the text around them instead of
// starting a box of their own, unless a stylesheet says otherwise.
var inlineByDefault = map[string]bool{
	"span": true, "b": true, "i": true, "small": true, "em": true,
	"strong": true, "a": true, "label": true, "code": true,
}

type compiler struct {
	sheet    *stylesheet
	warnings []Warning
	// computedFor caches each element's style. The tree is visited more than
	// once, and without this an element's unsupported declarations would be
	// reported once per visit.
	computedFor map[*html.Node]style
	images      func(string) (stdimage.Image, error)
}

// Compiler holds what compiling a page needs beyond the page itself.
type Compiler struct {
	// Images resolves an img element's src attribute. Without one an img is
	// reported rather than quietly dropped, because a missing picture is the
	// kind of absence nobody notices until the tag is on the wall.
	Images func(src string) (stdimage.Image, error)
}

// Compile turns one HTML document and one stylesheet into compose nodes,
// resolving no images.
func Compile(markup, css string) (Document, error) {
	return Compiler{}.Compile(markup, css)
}

func (options Compiler) Compile(markup, css string) (Document, error) {
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return Document{}, fmt.Errorf("parse markup: %w", err)
	}
	sheet, err := parseStylesheet(css)
	if err != nil {
		return Document{}, err
	}
	c := &compiler{sheet: sheet, computedFor: map[*html.Node]style{}, images: options.Images}
	for _, name := range atRules(css) {
		c.warn("stylesheet", "unsupported-at-rule",
			fmt.Sprintf("%s is not implemented; there is one panel and one frame, so there is nothing for it to select on", name))
	}

	body := findBody(root)
	if body == nil {
		return Document{Warnings: c.warnings}, fmt.Errorf("markup has no body")
	}
	element := firstElement(body)
	if element == nil {
		return Document{Warnings: c.warnings}, fmt.Errorf("body has no element to render")
	}
	node, _ := c.element(element, rootStyle(), "root")
	if node == nil {
		return Document{Warnings: c.warnings},
			fmt.Errorf("the root element resolved to nothing to draw")
	}
	return Document{Root: node, Warnings: c.warnings}, nil
}

func (c *compiler) warn(path, code, message string) {
	c.warnings = append(c.warnings, Warning{Path: path, Code: code, Message: message})
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
	// Children of a flex container are blockified: being a flex item settles
	// the question before the element's own default gets a say.
	if parent.display == displayFlex {
		current.inline = false
	}
	// Custom properties are resolved against the chain this element sits in,
	// which is how they inherit: an ancestor's declaration is in scope here
	// because that ancestor matched too.
	declared := c.variablesFor(node)
	for _, applied := range c.sheet.declarationsFor(node) {
		property, value := applied.property, applied.value
		if strings.Contains(value, "var(") {
			substituted, problem := c.sheet.substitute(value, declared)
			if problem != "" {
				c.warn(path, "unsupported-declaration",
					fmt.Sprintf("%s: %s: %s", describe(node), property, problem))
				continue
			}
			value = substituted
		}
		current.apply(property, value, inherited, func(message string) {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("%s: %s", describe(node), message))
		})
	}
	c.computedFor[node] = current
	return current
}

// variablesFor gathers the custom properties in scope for one element: those
// declared on it, and those declared on anything it descends from.
func (c *compiler) variablesFor(node *html.Node) map[string]string {
	declared := map[string]string{}
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type != html.ElementNode {
			continue
		}
		for _, candidate := range c.sheet.variables {
			if !candidate.selector.Match(ancestor) {
				continue
			}
			for _, applied := range candidate.declaration {
				// The nearest declaration wins, and the walk goes outward, so
				// only fill what is still empty.
				if _, taken := declared[applied.property]; !taken {
					declared[applied.property] = applied.value
				}
			}
		}
	}
	return declared
}

// element compiles one element and reports the flex weights its parent should
// give it.
func (c *compiler) element(node *html.Node, parent style, path string) (compose.Node, style) {
	current := c.computed(node, parent, path)
	if current.display == displayNone {
		return nil, current
	}
	if current.hidden {
		// visibility keeps the box and its space; only the paint goes.
		return sized(compose.Spacer{}, current), current
	}

	if node.Data == "img" {
		return sized(c.image(node, current, path), current), current
	}
	inner := c.children(node, current, path)
	if inner == nil && current.background == nil && current.border == nil {
		return nil, current
	}

	// Padding wraps whatever the children produced.
	if inner != nil && current.padding != (compose.Insets{}) {
		inner = compose.Padding{Insets: current.padding, Child: inner}
	}

	// A background and a border are layers under the content, which is what a
	// stack is for. An element with neither is just its content.
	var layers []compose.Node
	if current.background != nil {
		layers = append(layers, compose.Rectangle{Fill: current.background, Radius: radiusOf(current)})
	}
	if current.border != nil && current.border.width > 0 {
		stroke := &display.StrokeStyle{Ink: current.border.ink, Width: current.border.width}
		if current.dashed {
			stroke.Dash = []int{current.border.width * 3, current.border.width * 2}
		}
		layers = append(layers, compose.Rectangle{Radius: current.border.radius, Stroke: stroke})
	}
	if current.clip && inner != nil {
		inner = compose.Clip{Child: inner}
	}
	if len(layers) == 0 {
		return sized(inner, current), current
	}
	if inner != nil {
		layers = append(layers, inner)
	}
	return sized(compose.Stack{Children: layers}, current), current
}

func radiusOf(s style) int {
	if s.border == nil {
		return 0
	}
	return s.border.radius
}

// sized applies an explicit width or height, which compose expresses as a
// node's preferred size.
func sized(node compose.Node, s style) compose.Node {
	if node == nil {
		return nil
	}
	if !s.width.set && !s.height.set {
		return node
	}
	size := stdimage.Point{}
	if s.width.set && !s.width.percent {
		size.X = s.width.pixels()
	}
	if s.height.set && !s.height.percent {
		size.Y = s.height.pixels()
	}
	if size == (stdimage.Point{}) {
		return node
	}
	return compose.Stack{Size: size, Children: []compose.Node{node}}
}

// image resolves an img element. Its fit comes from object-fit, and the
// pixel reduction is left to the image node, which measures the source and
// chooses a treatment for it.
func (c *compiler) image(node *html.Node, current style, path string) compose.Node {
	source := attribute(node, "src")
	if source == "" {
		c.warn(path, "unsupported-declaration", "img has no src")
		return nil
	}
	if c.images == nil {
		c.warn(path, "unresolved-image", fmt.Sprintf(
			"img src=%q was not drawn: this compiler was given no way to resolve images", source))
		return nil
	}
	decoded, err := c.images(source)
	if err != nil {
		c.warn(path, "unresolved-image", fmt.Sprintf("img src=%q: %v", source, err))
		return nil
	}
	picture := compose.Image{Source: decoded, Processing: compose.ImageAuto}
	if current.objectFit != nil {
		picture.Overrides.Fit = current.objectFit
	}
	return picture
}

// children lays out an element's children, choosing between a run of text and
// a flex line by what the children are.
func (c *compiler) children(node *html.Node, current style, path string) compose.Node {
	// A flex container lays out boxes, never a line of text, so the inline
	// path is not even attempted for one.
	if current.display != displayFlex {
		if runs := c.textRuns(node, current, path); len(runs) > 0 {
			// Text sits at the top of its box, as it does in CSS. Centring it
			// vertically is the container's job, through align-items, not a
			// property of the text itself.
			// Text sits at the top of its box unless vertical-align says
			// otherwise, which is what CSS does everywhere except a table
			// cell, and a fixed-height row here is the same situation.
			return compose.Text{Runs: runs, Align: current.textAlign,
				VerticalAlign: current.textVAlign, Wrap: current.wrap, LineHeight: current.lineHeight}
		}
	}

	var items []compose.LayoutChild
	var placed []compose.Anchor
	index := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		childPath := fmt.Sprintf("%s>%s[%d]", path, child.Data, index)
		index++
		compiled, childStyle := c.element(child, current, childPath)
		if compiled == nil {
			continue
		}
		// An absolutely positioned child is taken out of the line and placed
		// against the container's own box, which is what compose's Absolute
		// does with an explicit rectangle.
		if childStyle.absolute {
			placed = append(placed, anchorFor(childStyle, compiled))
			continue
		}
		// margin-left:auto on a row, or margin-top:auto on a column, pushes
		// this child and everything after it to the far end. A spacer that
		// takes all the slack is how compose says the same thing.
		if (current.direction == axisRow && childStyle.autoLeft) ||
			(current.direction == axisColumn && childStyle.autoTop) {
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Grow: 1})
		}
		before, after, across := marginsAlong(childStyle, current.direction)
		if across != (compose.Insets{}) {
			compiled = compose.Padding{Insets: across, Child: compiled}
		}
		if before > 0 {
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Basis: compose.Pixels(before)})
		}
		items = append(items, c.layoutChild(compiled, childStyle, current, childPath))
		if after > 0 {
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Basis: compose.Pixels(after)})
		}
		if (current.direction == axisRow && childStyle.autoRight) ||
			(current.direction == axisColumn && childStyle.autoBottom) {
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Grow: 1})
		}
	}
	if current.display != displayFlex && current.gap > 0 {
		c.warn(path, "unsupported-declaration",
			"gap has no effect on a block container; it spaces flex items")
	}
	if len(items) == 0 && len(placed) == 0 {
		return nil
	}
	flow := func(line compose.Node) compose.Node {
		if len(placed) == 0 {
			return line
		}
		layered := compose.Anchored{Children: placed}
		if line == nil {
			return layered
		}
		return compose.Stack{Children: []compose.Node{line, layered}}
	}
	if current.display == displayFlex {
		if current.spaceEvenly {
			items = spread(items)
		}
		cross := current.alignItems
		if current.direction == axisColumn {
			return flow(compose.Column{Gap: current.gap, MainAlign: current.justify,
				CrossAlign: cross, Children: items})
		}
		return flow(compose.Row{Gap: current.gap, MainAlign: current.justify,
			CrossAlign: cross, Children: items})
	}
	// Block children stack down the page, which is a column that neither
	// grows nor gaps.
	if len(items) == 0 {
		return flow(nil)
	}
	return flow(compose.Column{CrossAlign: compose.CrossStretch, Children: items})
}

// anchorFor describes where a positioned child sits without deciding it. The
// container's rectangle is only known once the layout runs, so the insets go
// through as they were written and are resolved there.
func anchorFor(child style, node compose.Node) compose.Anchor {
	anchor := compose.Anchor{Node: node, Layer: child.layer}
	for index, edge := range []**int{&anchor.Top, &anchor.Right, &anchor.Bottom, &anchor.Left} {
		if value := child.inset[index]; value.set && !value.percent {
			pixels := value.pixels()
			*edge = &pixels
		}
	}
	if child.width.set && !child.width.percent {
		anchor.Size.X = child.width.pixels()
	}
	if child.height.set && !child.height.percent {
		anchor.Size.Y = child.height.pixels()
	}
	return anchor
}

// spread puts a growing spacer between each pair of items, which is what
// space-between describes: the slack goes into the gaps rather than the ends.
func spread(items []compose.LayoutChild) []compose.LayoutChild {
	if len(items) < 2 {
		return items
	}
	spaced := make([]compose.LayoutChild, 0, len(items)*2-1)
	for index, item := range items {
		if index > 0 {
			spaced = append(spaced, compose.LayoutChild{Node: compose.Spacer{}, Grow: 1})
		}
		spaced = append(spaced, item)
	}
	return spaced
}

// marginsAlong splits a flex item's margins by axis, because the two need
// different treatment. Along the container's axis a margin is a fixed gap
// beside the item, which a spacer expresses exactly. Across it, the item is
// inset instead, which is what wrapping the built node in padding does.
func marginsAlong(child style, direction axis) (before, after int, across compose.Insets) {
	if direction == axisColumn {
		return child.margin.Top, child.margin.Bottom,
			compose.Insets{Left: child.margin.Left, Right: child.margin.Right}
	}
	return child.margin.Left, child.margin.Right,
		compose.Insets{Top: child.margin.Top, Bottom: child.margin.Bottom}
}

// layoutChild hands the child's stated sizes to the layout as lengths, which
// is where they can finally be resolved. Nothing here decides a pixel.
func (c *compiler) layoutChild(node compose.Node, child, parent style, path string) compose.LayoutChild {
	along := parent.direction
	if parent.display != displayFlex {
		// Block children stack down the page whatever the container says.
		along = axisColumn
	}
	item := compose.LayoutChild{Node: node, Grow: child.grow, AlignSelf: child.alignSelf}

	mainSize, crossSize := child.width, child.height
	minMain, minCross := child.minSize[0], child.minSize[1]
	maxMain, maxCross := child.maxSize[0], child.maxSize[1]
	if along == axisColumn {
		mainSize, crossSize = crossSize, mainSize
		minMain, minCross = minCross, minMain
		maxMain, maxCross = maxCross, maxMain
	}
	item.Basis = lengthOf(mainSize)
	if child.basis.set {
		item.Basis = lengthOf(child.basis)
	}
	item.Cross = lengthOf(crossSize)
	item.MinMain, item.MaxMain = lengthOf(minMain), lengthOf(maxMain)
	item.MinCross, item.MaxCross = lengthOf(minCross), lengthOf(maxCross)
	return item
}

// lengthOf carries a stated size through unresolved. Percentages are kept in
// tenths so that a figure like 87.3% survives the trip.
func lengthOf(size length) compose.Length {
	switch {
	case !size.set:
		return compose.Auto()
	case size.percent:
		return compose.Tenths(int(size.value * 10))
	}
	return compose.Pixels(size.pixels())
}

// textRuns collects the inline content of an element. It returns nothing when
// the element has element children that are not inline, which is how a box
// with boxes inside it is told from a box with words inside it.
func (c *compiler) textRuns(node *html.Node, current style, path string) []display.TextRun {
	started := false
	return c.collectRuns(node, current, path, &started)
}

// collectRuns walks the inline content of an element. started is shared across
// the whole line, because collapsing whitespace is a property of the line and
// not of any one text node: the space between </b> and the word after it is
// real, and only the space at the very beginning of the line is dropped.
func (c *compiler) collectRuns(node *html.Node, current style, path string, started *bool) []display.TextRun {
	var runs []display.TextRun
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			text := collapse(child.Data, *started)
			if text == "" {
				continue
			}
			*started = true
			runs = append(runs, display.TextRun{Text: text, Style: textStyle(current)})
		case html.ElementNode:
			if child.Data == "br" {
				runs = append(runs, display.TextRun{Text: "\n", Style: textStyle(current)})
				*started = false
				continue
			}
			childStyle := c.computed(child, current, path)
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

func textStyle(s style) display.TextStyle {
	return display.TextStyle{Font: s.fontFamily, Size: s.fontSize, Ink: s.color}
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
