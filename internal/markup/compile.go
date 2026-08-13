package markup

import (
	"fmt"
	"image"
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
}

// Compile turns one HTML document and one stylesheet into compose nodes.
func Compile(markup, css string) (Document, error) {
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return Document{}, fmt.Errorf("parse markup: %w", err)
	}
	sheet, err := parseStylesheet(css)
	if err != nil {
		return Document{}, err
	}
	c := &compiler{sheet: sheet, computedFor: map[*html.Node]style{}}
	for _, name := range atRules(css) {
		c.warn("stylesheet", "unsupported-at-rule",
			fmt.Sprintf("%s is not implemented; there is one panel and one frame, so there is nothing for it to select on", name))
	}

	body := findBody(root)
	if body == nil {
		return Document{}, fmt.Errorf("markup has no body")
	}
	element := firstElement(body)
	if element == nil {
		return Document{}, fmt.Errorf("body has no element to render")
	}
	node, _ := c.element(element, rootStyle(), "root")
	if node == nil {
		return Document{}, fmt.Errorf("the root element resolved to nothing to draw")
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
	current := parent.inherited()
	current.display = displayBlock
	current.inline = inlineByDefault[node.Data]
	// Children of a flex container are blockified: being a flex item settles
	// the question before the element's own default gets a say.
	if parent.display == displayFlex {
		current.inline = false
	}
	for _, applied := range c.sheet.declarationsFor(node) {
		property, value := applied.property, applied.value
		current.apply(property, value, func(message string) {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("%s: %s", describe(node), message))
		})
	}
	c.computedFor[node] = current
	return current
}

// element compiles one element and reports the flex weights its parent should
// give it.
func (c *compiler) element(node *html.Node, parent style, path string) (compose.Node, style) {
	current := c.computed(node, parent, path)
	if current.display == displayNone {
		return nil, current
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
		layers = append(layers, compose.Rectangle{
			Radius: current.border.radius,
			Stroke: &display.StrokeStyle{Ink: current.border.ink, Width: current.border.width},
		})
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
	size := image.Point{}
	if s.width.set && !s.width.percent {
		size.X = s.width.pixels()
	}
	if s.height.set && !s.height.percent {
		size.Y = s.height.pixels()
	}
	if size == (image.Point{}) {
		return node
	}
	return compose.Stack{Size: size, Children: []compose.Node{node}}
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
			return compose.Text{Runs: runs, Align: current.textAlign, VerticalAlign: current.textVAlign}
		}
	}

	var items []compose.LayoutChild
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
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Basis: before})
		}
		items = append(items, c.layoutChild(compiled, childStyle, current))
		if after > 0 {
			items = append(items, compose.LayoutChild{Node: compose.Spacer{}, Basis: after})
		}
	}
	if len(items) == 0 {
		return nil
	}
	if current.display == displayFlex && current.direction == axisColumn {
		return compose.Column{Gap: current.gap, CrossAlign: current.alignItems, Children: items}
	}
	if current.display == displayFlex {
		return compose.Row{Gap: current.gap, CrossAlign: current.alignItems, Children: items}
	}
	// Block children stack down the page, which is a column that neither
	// grows nor gaps.
	if len(items) == 1 {
		return items[0].Node
	}
	return compose.Column{CrossAlign: compose.CrossStretch, Children: items}
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

// layoutChild turns a child's own style into the weights its parent needs.
func (c *compiler) layoutChild(node compose.Node, child, parent style) compose.LayoutChild {
	item := compose.LayoutChild{Node: node, Grow: child.grow}
	switch {
	case child.basis.set && !child.basis.percent:
		item.Basis = child.basis.pixels()
	case parent.direction == axisRow && child.width.set && !child.width.percent:
		item.Basis = child.width.pixels()
	case parent.direction == axisColumn && child.height.set && !child.height.percent:
		item.Basis = child.height.pixels()
	}
	// A percentage width on a block child has no container size to resolve
	// against until layout runs, so it becomes the ratio it describes: the
	// element takes that share of the line and a spacer takes the rest.
	if child.width.percent && parent.display != displayFlex {
		share := int(child.width.value * 10)
		item.Node = compose.Row{Children: []compose.LayoutChild{
			{Node: node, Grow: max(1, share)},
			{Node: compose.Spacer{}, Grow: max(1, 1000-share)},
		}}
	}
	return item
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
