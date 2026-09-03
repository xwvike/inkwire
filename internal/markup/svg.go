package markup

import (
	"fmt"
	"math"
	"strings"

	"golang.org/x/net/html"
)

// An svg element is where a page stops describing boxes and describes shapes.
//
// CSS has no vocabulary for an arc, a polygon or a path, and giving it one
// would mean inventing a dialect that looks like CSS and is not. SVG is that
// vocabulary, already standard, already what every drawing tool exports, and
// almost exactly the one the schema has: a rect is a rectangle, a polyline is
// a polyline, and the two names for a dashed line are the same two names.
//
// The parser is the one this package already uses. HTML puts an svg element
// and its children in their own namespace and leaves attribute names in the
// case they were written in, so viewBox survives and an svg child is told
// from a div by asking.
//
// What SVG has and a panel does not — gradients, filters, masks, opacity,
// arbitrary rotation, markers, text on a path — is skipped and reported, the
// same rule the stylesheet is held to. An svg exported from a drawing tool
// will draw a simplified picture and say exactly what it simplified.

// svgNamespace is what the HTML parser marks these elements with.
const svgNamespace = "svg"

// svg compiles an svg element into the drawing it describes.
//
// The children are placed in an absolute box by their own coordinates, which
// is what an SVG viewport is. Clipping is on because a viewport clips: a shape
// hanging off the edge of one is cut by it rather than drawn over whatever the
// page put next to it.
func (c *compiler) svg(node *html.Node, current style, path string) *emitted {
	width, height, frame := c.viewport(node, current, path)
	if width <= 0 || height <= 0 {
		c.warn(path, "unresolved-drawing",
			"an svg element states no size: give it width and height, or a stylesheet that does")
		return nil
	}
	box := rect{Width: width, Height: height}
	c.clips, c.patterns = map[string]*html.Node{}, map[string]*html.Node{}
	clipPaths(node, c.clips)
	patterns(node, c.patterns)
	drawing := &emitted{Type: "absolute", Clip: true}
	var children []placed
	c.svgChildren(node, box, c.svgPaint(node, rootPaint(), current, path), current, frame, path, &children)
	if len(children) == 0 {
		if !current.hidden {
			c.warn(path, "unresolved-drawing", "an svg element with nothing in it that this build can draw")
		}
		return nil
	}
	drawing.Children = children
	return drawing
}

// viewport is how large the drawing is in the page and how its user
// coordinates map into that box.
//
// A stylesheet wins over the attributes, because that is what a presentation
// attribute is: a default that CSS overrides. A viewBox maps its user
// coordinates into the viewport, using the browser's xMidYMid meet default.
func (c *compiler) viewport(node *html.Node, current style, path string) (int, int, svgFrame) {
	width, height := 0, 0
	viewBoxMinX, viewBoxMinY := 0.0, 0.0
	viewBoxWidth, viewBoxHeight := 0.0, 0.0
	hasViewBox := false
	if box := strings.TrimSpace(attribute(node, "viewBox")); box != "" {
		fields := strings.FieldsFunc(box, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
		})
		if len(fields) != 4 {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("viewBox=%q needs four numbers", box))
		} else {
			var values [4]float64
			valid := true
			for index, field := range fields {
				var ok bool
				values[index], ok = svgNumber(field)
				valid = valid && ok
			}
			if !valid || values[2] <= 0 || values[3] <= 0 {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"viewBox=%q needs finite origin and positive width and height", box))
			} else {
				viewBoxMinX, viewBoxMinY = values[0], values[1]
				viewBoxWidth, viewBoxHeight = values[2], values[3]
				hasViewBox = true
				width, height = pixels(viewBoxWidth), pixels(viewBoxHeight)
			}
		}
	}
	if value, ok := svgNumber(attribute(node, "width")); ok {
		width = pixels(value)
	}
	if value, ok := svgNumber(attribute(node, "height")); ok {
		height = pixels(value)
	}
	// A stylesheet states the size the same way it does for any other box, and
	// it is the one that decides where the element sits on the page.
	if current.width.fixed() {
		width = current.width.px()
	}
	if current.height.fixed() {
		height = current.height.px()
	}
	if !hasViewBox || width <= 0 || height <= 0 {
		return width, height, rootFrame()
	}
	if stated := strings.TrimSpace(attribute(node, "preserveAspectRatio")); stated == "none" {
		return width, height, svgFrame{
			offsetX: -viewBoxMinX * float64(width) / viewBoxWidth,
			offsetY: -viewBoxMinY * float64(height) / viewBoxHeight,
			scaleX:  float64(width) / viewBoxWidth,
			scaleY:  float64(height) / viewBoxHeight,
		}
	} else if stated != "" && stated != "xMidYMid meet" {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"preserveAspectRatio=%q is not implemented; xMidYMid meet is used", stated))
	}
	scale := math.Min(float64(width)/viewBoxWidth, float64(height)/viewBoxHeight)
	return width, height, svgFrame{
		offsetX: (float64(width)-viewBoxWidth*scale)/2 - viewBoxMinX*scale,
		offsetY: (float64(height)-viewBoxHeight*scale)/2 - viewBoxMinY*scale,
		scaleX:  scale,
		scaleY:  scale,
	}
}

// svgChildren walks a drawing, placing what it can draw and naming what it
// cannot. It calls itself for a g element, which groups without placing.
func (c *compiler) svgChildren(node *html.Node, box rect, inherited svgPaint, inheritedStyle style, frame svgFrame, path string, into *[]placed) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Namespace != svgNamespace {
			continue
		}
		childPath := fmt.Sprintf("%s>%s", path, child.Data)
		childStyle := c.computed(child, inheritedStyle, childPath)
		// display:none removes a group and everything beneath it. Visibility is
		// inherited, so a hidden group still has to be walked for a descendant
		// that explicitly says visible; individual hidden shapes are skipped.
		if childStyle.display == displayNone {
			continue
		}
		switch child.Data {
		case "g":
			// A group carries style down to its children, which is what the
			// element is for. What it says and this cannot do is reported
			// here rather than once per child, since it was written once.
			c.reportUnread(child, childPath)
			paint := c.svgPaint(child, inherited, childStyle, childPath)
			degrees, about, turned := c.turnOf(child, frame, childPath)
			if !turned {
				c.svgChildren(child, box, paint, childStyle, c.transformed(child, frame, childPath), childPath, into)
				continue
			}
			// A turned group becomes a box of its own, turned. Its children
			// keep the coordinates they were written in, which is what makes
			// the turn the only thing that moved.
			var inner []placed
			c.svgChildren(child, box, paint, childStyle, c.transformed(child, frame, childPath), childPath, &inner)
			if len(inner) == 0 {
				continue
			}
			*into = append(*into, placed{Bounds: box, Node: &emitted{
				Type: "rotated", Degrees: degrees, Origin: about,
				Child: &emitted{Type: "absolute", Children: inner},
			}})
		case "title", "desc", "metadata", "style", "defs", "clipPath", "pattern":
			// Not drawn, by their own definition.
		default:
			if childStyle.hidden {
				continue
			}
			placement, ok := c.svgShape(child, box, inherited, childStyle, c.transformed(child, frame, childPath), childPath)
			if !ok {
				continue
			}
			placement = c.clipped(child, box, frame, placement, childPath)
			if degrees, about, turned := c.turnOf(child, frame, childPath); turned {
				placement = placed{Bounds: box, Node: &emitted{
					Type: "rotated", Degrees: degrees, Origin: about,
					Child: &emitted{Type: "absolute", Children: []placed{placement}},
				}}
			}
			*into = append(*into, placement)
		}
	}
}

// svgShape turns one element into the node that draws it and the box it draws
// in. The two kinds of shape need different boxes: one states its own
// coordinates and is placed across the whole viewport, and one fills whatever
// box it is given and so is placed in its own.
func (c *compiler) svgShape(node *html.Node, box rect, inherited svgPaint, current style, frame svgFrame, path string) (placed, bool) {
	c.reportUnread(node, path)
	// A fill that names a pattern is not a colour, so it is read before the
	// paint is: what comes back is the drawing that fills the shape rather
	// than an ink to fill it with.
	if tiled, ok := c.patternFill(c.svgAttribute(node, "fill", path), path); ok {
		return c.tiled(node, box, frame, tiled, path)
	}
	paint := c.svgPaint(node, inherited, current, path)
	switch node.Data {
	case "rect":
		x1, y1 := frame.place(svgLength(node, "x"), svgLength(node, "y"))
		x2, y2 := frame.place(svgLength(node, "x")+svgLength(node, "width"), svgLength(node, "y")+svgLength(node, "height"))
		x, y := math.Min(x1, x2), math.Min(y1, y2)
		width, height := frame.sizeX(svgLength(node, "width")), frame.sizeY(svgLength(node, "height"))
		if width <= 0 || height <= 0 {
			c.warn(path, "unsupported-declaration", "a rect with no width or height draws nothing")
			return placed{}, false
		}
		shape := &emitted{Type: "rectangle", Fill: paint.fill, Stroke: paint.edge()}
		// SVG rounds a rectangle by two radii and the drawing model by one, so
		// a rect rounded differently on each axis is drawn by the larger.
		radiusX, radiusY := frame.sizeX(svgLength(node, "rx")), frame.sizeY(svgLength(node, "ry"))
		shape.Radius = pixels(max(radiusX, radiusY))
		if radiusX != radiusY && radiusX > 0 && radiusY > 0 {
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"rx=%g and ry=%g differ; a corner here has one radius and %d was used", radiusX, radiusY, shape.Radius))
		}
		if !c.painted(shape, paint, path) {
			return placed{}, false
		}
		return placed{Bounds: rect{X: pixels(x), Y: pixels(y), Width: pixels(width), Height: pixels(height)}, Node: shape}, true

	case "circle":
		writtenRadius := svgLength(node, "r")
		radius := frame.size(writtenRadius)
		if radius <= 0 {
			c.warn(path, "unsupported-declaration", "a circle with no radius draws nothing")
			return placed{}, false
		}
		centerX, centerY := frame.place(svgLength(node, "cx"), svgLength(node, "cy"))
		center := at(centerX, centerY)
		if frame.sizeX(writtenRadius) != frame.sizeY(writtenRadius) {
			// A non-uniform transform turns a circle into an ellipse. The
			// drawing schema represents that with its bounding box, so retain
			// the geometry instead of silently choosing one radius.
			radiusX, radiusY := frame.sizeX(writtenRadius), frame.sizeY(writtenRadius)
			shape := &emitted{Type: "ellipse", Fill: paint.fill, Stroke: paint.edge()}
			if !c.painted(shape, paint, path) {
				return placed{}, false
			}
			return placed{Bounds: rect{
				X: pixels(centerX - radiusX), Y: pixels(centerY - radiusY),
				Width: pixels(radiusX*2) + 1, Height: pixels(radiusY*2) + 1}, Node: shape}, true
		}
		shape := &emitted{Type: "circle", Fill: paint.fill, Stroke: paint.edge(),
			Center: center, Radius: pixels(radius)}
		if !c.painted(shape, paint, path) {
			return placed{}, false
		}
		return placed{Bounds: box, Node: shape}, true

	case "ellipse":
		// An ellipse fills the box it is given, so its box is worked out from
		// the centre and the two radii it states.
		radiusX, radiusY := frame.sizeX(svgLength(node, "rx")), frame.sizeY(svgLength(node, "ry"))
		if radiusX <= 0 || radiusY <= 0 {
			c.warn(path, "unsupported-declaration", "an ellipse with no radii draws nothing")
			return placed{}, false
		}
		centreX, centreY := frame.place(svgLength(node, "cx"), svgLength(node, "cy"))
		shape := &emitted{Type: "ellipse", Fill: paint.fill, Stroke: paint.edge()}
		if !c.painted(shape, paint, path) {
			return placed{}, false
		}
		// The box is a pixel wider and taller than twice the radius, because
		// the drawing model measures a radius as half of one less than the
		// span: the ellipse touches the last pixel inside the box rather than
		// the edge of it. Twice the radius draws one a pixel small on each
		// axis, which is the kind of wrong that only shows when the same
		// picture is drawn both ways and compared.
		return placed{Bounds: rect{
			X: pixels(centreX - radiusX), Y: pixels(centreY - radiusY),
			Width: pixels(radiusX*2) + 1, Height: pixels(radiusY*2) + 1}, Node: shape}, true

	case "line":
		if paint.edge() == nil {
			c.warn(path, "unsupported-declaration", "a line with no stroke draws nothing")
			return placed{}, false
		}
		shape := &emitted{Type: "line", Stroke: paint.edge(),
			From: atFrame(frame, svgLength(node, "x1"), svgLength(node, "y1")),
			To:   atFrame(frame, svgLength(node, "x2"), svgLength(node, "y2"))}
		return placed{Bounds: box, Node: shape}, true

	case "polyline", "polygon":
		points, err := c.svgPoints(attribute(node, "points"), frame)
		if err != "" {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("points: %s", err))
			return placed{}, false
		}
		if node.Data == "polyline" {
			if paint.edge() == nil {
				c.warn(path, "unsupported-declaration", "a polyline with no stroke draws nothing")
				return placed{}, false
			}
			return placed{Bounds: box, Node: &emitted{Type: "polyline", Points: points, Stroke: paint.edge()}}, true
		}
		if len(points) < 3 {
			c.warn(path, "unsupported-declaration", "a polygon needs at least three points")
			return placed{}, false
		}
		shape := &emitted{Type: "polygon", Points: points, Fill: paint.fill, Stroke: paint.edge()}
		if !c.painted(shape, paint, path) {
			return placed{}, false
		}
		return placed{Bounds: box, Node: shape}, true
	case "path":
		data := strings.TrimSpace(attribute(node, "d"))
		if data == "" {
			c.warn(path, "unsupported-declaration", "a path with no d draws nothing")
			return placed{}, false
		}
		commands := c.parsePathData(data, frame, path)
		if len(commands) == 0 {
			return placed{}, false
		}
		shape := &emitted{Type: "path", Commands: commands, Fill: paint.fill, Stroke: paint.edge()}
		if !c.painted(shape, paint, path) {
			return placed{}, false
		}
		return placed{Bounds: box, Node: shape}, true
	}

	c.warn(path, "unsupported-declaration", fmt.Sprintf(
		"%s is not an element this build draws", node.Data))
	return placed{}, false
}

// tiled places a pattern where the shape that named it sits. Only a rectangle
// can be tiled: the schema fills a box, and clipping the tile to something else
// is what clip-path is for.
func (c *compiler) tiled(node *html.Node, box rect, frame svgFrame, tile *emitted, path string) (placed, bool) {
	if tile == nil {
		return placed{}, false
	}
	if node.Data != "rect" {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"a %s filled with a pattern is not drawn; a pattern fills a rect, and a clip-path shapes it", node.Data))
		return placed{}, false
	}
	x1, y1 := frame.place(svgLength(node, "x"), svgLength(node, "y"))
	x2, y2 := frame.place(svgLength(node, "x")+svgLength(node, "width"), svgLength(node, "y")+svgLength(node, "height"))
	x, y := math.Min(x1, x2), math.Min(y1, y2)
	width, height := frame.sizeX(svgLength(node, "width")), frame.sizeY(svgLength(node, "height"))
	if width <= 0 || height <= 0 {
		c.warn(path, "unsupported-declaration", "a rect with no width or height draws nothing")
		return placed{}, false
	}
	return placed{Bounds: rect{
		X: pixels(x), Y: pixels(y), Width: pixels(width), Height: pixels(height)}, Node: tile}, true
}

// painted reports whether a shape has anything to draw with, which is the one
// thing SVG's defaults make easy to get wrong: fill defaults to black, so a
// shape that says nothing is filled, and a shape that says fill=none and
// nothing else is invisible.
func (c *compiler) painted(shape *emitted, paint svgPaint, path string) bool {
	if shape.Fill == "" && shape.Stroke == nil {
		c.warn(path, "unsupported-declaration", "a shape with neither a fill nor a stroke draws nothing")
		return false
	}
	return true
}

// svgPaint is what a shape is drawn with, and what a group hands down.
//
// The parts are held separately rather than as a finished stroke because they
// are stated separately and inherited separately: a group may name the ink and
// a shape inside it the width, and neither has said anything about the other.
type svgPaint struct {
	fill        string
	strokeInk   string
	strokeWidth int
	dash        []int
	dashOffset  int
}

// edge is the stroke a shape ends up with, or nothing if no ink was ever named.
func (p svgPaint) edge() *stroke {
	if p.strokeInk == "" {
		return nil
	}
	width := p.strokeWidth
	if width < 1 {
		width = 1
	}
	return &stroke{Ink: p.strokeInk, Width: width, Dash: p.dash, DashOffset: p.dashOffset}
}

// svgPaint reads the presentation attributes that say how a shape is painted.
//
// SVG's defaults are followed rather than the panel's: fill is black unless
// something says otherwise, and stroke is nothing. An author who has drawn an
// SVG anywhere else expects the shape they drew.
func (c *compiler) svgPaint(node *html.Node, inherited svgPaint, current style, path string) svgPaint {
	report := func(message string) { c.warn(path, "unsupported-declaration", message) }
	paint := inherited
	if stated := c.svgAttribute(node, "fill", path); stated != "" {
		if ink, ok := parseInk(stated, "fill", report); ok {
			paint.fill = inkName(ink)
		} else {
			paint.fill = ""
		}
	}
	if stated := c.svgAttribute(node, "stroke", path); stated != "" {
		if ink, ok := parseInk(stated, "stroke", report); ok {
			paint.strokeInk = inkName(ink)
		} else {
			paint.strokeInk = ""
		}
	}
	if width, ok := svgNumber(c.svgAttribute(node, "stroke-width", path)); ok {
		if pixels(width) < 1 {
			// A stroke thinner than the thing it is drawn on is not a thinner
			// line, it is no line. Saying so beats a shape that quietly lost
			// its outline.
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"stroke-width=%g is less than a pixel; drawn at 1", width))
			width = 1
		}
		paint.strokeWidth = pixels(width)
	}
	if dash := c.svgAttribute(node, "stroke-dasharray", path); dash != "" {
		paint.dash = nil
		if dash != "none" {
			for _, field := range strings.FieldsFunc(dash, func(r rune) bool { return r == ',' || r == ' ' }) {
				length, ok := svgNumber(field)
				if !ok || length < 0 {
					c.warn(path, "unsupported-declaration", fmt.Sprintf(
						"stroke-dasharray=%q is a run of lengths, such as \"4 2\"", dash))
					paint.dash = nil
					break
				}
				paint.dash = append(paint.dash, pixels(length))
			}
		}
	}
	if offset, ok := svgNumber(c.svgAttribute(node, "stroke-dashoffset", path)); ok {
		paint.dashOffset = pixels(offset)
	}
	// A stylesheet outranks all of it. In CSS a presentation attribute is a
	// rule of no specificity, so any selector at all beats it — which is what
	// makes a drawing somebody else made usable here: the shapes are kept and
	// the colours are restated in terms the panel has.
	if current.fill != nil {
		paint.fill = ""
		if !current.fill.none {
			paint.fill = inkName(current.fill.ink)
		}
	}
	if current.stroke != nil {
		paint.strokeInk = ""
		if !current.stroke.none {
			paint.strokeInk = inkName(current.stroke.ink)
		}
	}
	if current.strokeWidth != nil {
		paint.strokeWidth = *current.strokeWidth
	}
	if current.line != borderNone || len(current.dash) > 0 {
		paint.dash = current.dash
		paint.dashOffset = current.dashOffset
	}
	return paint
}

// svgAttribute reads a presentation attribute, resolving any custom property
// in it. A drawing exported from a design system names its colours with var(),
// and the value it falls back to is the one worth reading.
func (c *compiler) svgAttribute(node *html.Node, name, path string) string {
	value := strings.TrimSpace(attribute(node, name))
	if !strings.Contains(value, "var(") {
		return value
	}
	substituted, problem := c.sheet.substitute(value, c.variablesFor(node))
	if problem != "" {
		c.warn(path, "unsupported-declaration", fmt.Sprintf("%s: %s: %s", name, value, problem))
		return ""
	}
	return strings.TrimSpace(substituted)
}

// rootPaint is what a drawing starts with: SVG's own defaults rather than the
// panel's. A shape that says nothing is filled black and has no outline, which
// is what an author who drew it anywhere else expects to see.
func rootPaint() svgPaint { return svgPaint{fill: "black", strokeWidth: 1} }

// readAttributes are the ones this build acts on, per element. Anything else
// on a shape is reported, for the same reason an unimplemented CSS property
// is: a drawing that quietly lost its blur is a drawing whose author has no
// way of finding out.
var readAttributes = map[string]bool{
	"fill": true, "stroke": true, "stroke-width": true,
	"stroke-dasharray": true, "stroke-dashoffset": true,
	"x": true, "y": true, "width": true, "height": true, "rx": true, "ry": true,
	"cx": true, "cy": true, "r": true,
	"x1": true, "y1": true, "x2": true, "y2": true,
	"points": true, "d": true, "viewBox": true, "transform": true, "clip-path": true,
	// Said elsewhere or meaning nothing here, and not worth a line each.
	"id": true, "class": true, "style": true, "xmlns": true, "version": true,
	"xml:space": true, "aria-hidden": true, "role": true, "focusable": true,
}

// reportUnread names the attributes on an element that this build does not act
// on, so that a drawing exported from a tool says what it lost.
func (c *compiler) reportUnread(node *html.Node, path string) {
	for _, attribute := range node.Attr {
		name := attribute.Key
		if attribute.Namespace != "" {
			name = attribute.Namespace + ":" + name
		}
		if readAttributes[name] || strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") {
			continue
		}
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"%s=%q is not something this build draws with", name, attribute.Val))
	}
}

// svgPoints reads the points attribute of a polyline or a polygon, which is a
// run of coordinate pairs separated by whatever the author felt like.
func (c *compiler) svgPoints(value string, frame svgFrame) ([]point, string) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
	})
	if len(fields) == 0 {
		return nil, "there are none"
	}
	if len(fields)%2 != 0 {
		return nil, fmt.Sprintf("%d numbers is not a run of pairs", len(fields))
	}
	points := make([]point, 0, len(fields)/2)
	for index := 0; index < len(fields); index += 2 {
		x, okX := svgNumber(fields[index])
		y, okY := svgNumber(fields[index+1])
		if !okX || !okY {
			return nil, fmt.Sprintf("%q and %q are not a pair of numbers", fields[index], fields[index+1])
		}
		points = append(points, *atFrame(frame, x, y))
	}
	return points, ""
}

// svgLength reads a coordinate attribute, which is absent as often as it is
// stated: every one of them defaults to zero.
func svgLength(node *html.Node, name string) float64 {
	value, _ := svgNumber(attribute(node, name))
	return value
}

// svgNumber reads a number as SVG writes one. A unit is accepted and dropped:
// there is one unit on a panel and it is the pixel, so px says what is already
// true and anything else is a length this cannot honour.
func svgNumber(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	trimmed = strings.TrimSuffix(trimmed, "px")
	number, err := parseFinite(trimmed)
	if err != nil {
		return 0, false
	}
	return number, true
}

// svgFrame is the coordinate space a shape is read in: where its origin sits
// and how much larger it is drawn than it is written.
//
// A transform is folded into this rather than becoming a node of its own. A
// translate is an offset and scale is a multiplier on each axis, and applying
// both to the numbers as they are read is exact — there is no rounding to
// accumulate and nothing to nest. What cannot be folded is reported, which is
// everything that turns or shears the page.
type svgFrame struct {
	offsetX, offsetY float64
	scaleX, scaleY   float64
}

func rootFrame() svgFrame { return svgFrame{scaleX: 1, scaleY: 1} }

// place puts a coordinate written in a group's own space into the drawing's.
func (f svgFrame) place(x, y float64) (float64, float64) {
	return f.offsetX + x*f.scaleX, f.offsetY + y*f.scaleY
}

// size scales a radius using the uniform component of the frame. Callers that
// preserve both axes (ellipses and rectangles) use sizeX and sizeY instead.
func (f svgFrame) size(value float64) float64 {
	return value * math.Min(math.Abs(f.scaleX), math.Abs(f.scaleY))
}

func (f svgFrame) sizeX(value float64) float64 { return value * math.Abs(f.scaleX) }
func (f svgFrame) sizeY(value float64) float64 { return value * math.Abs(f.scaleY) }

// transformed folds a transform attribute into the frame, reporting the parts
// of it that cannot be folded.
func (c *compiler) transformed(node *html.Node, outer svgFrame, path string) svgFrame {
	stated := strings.TrimSpace(attribute(node, "transform"))
	if stated == "" {
		return outer
	}
	frame := outer
	for _, function := range svgTransforms(stated) {
		name, arguments := function[0], function[1]
		numbers := readNumbers(arguments)
		switch name {
		case "translate":
			if len(numbers) == 0 || len(numbers) > 2 {
				c.warn(path, "unsupported-declaration", fmt.Sprintf("transform: translate%s takes one or two numbers", arguments))
				continue
			}
			y := 0.0
			if len(numbers) == 2 {
				y = numbers[1]
			}
			frame.offsetX += numbers[0] * frame.scaleX
			frame.offsetY += y * frame.scaleY
		case "scale":
			if len(numbers) != 1 && len(numbers) != 2 {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"transform: scale%s takes one or two numbers", arguments))
				continue
			}
			scaleX, scaleY := numbers[0], numbers[0]
			if len(numbers) == 2 {
				scaleY = numbers[1]
			}
			if scaleX == 0 || scaleY == 0 || math.IsNaN(scaleX) || math.IsNaN(scaleY) || math.IsInf(scaleX, 0) || math.IsInf(scaleY, 0) {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"transform: scale%s has a zero or non-finite factor", arguments))
				continue
			}
			frame.scaleX *= scaleX
			frame.scaleY *= scaleY
		case "rotate":
			// Handled by turnOf, which puts it on a node of its own rather
			// than into the coordinates: a turn folded into the numbers would
			// stop a rect being a rect and an ellipse being an ellipse.
		default:
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"transform: %s%s is not something this build can do; the group is drawn untransformed", name, arguments))
		}
	}
	return frame
}

// turnOf reads the rotation out of a transform attribute, in the drawing's own
// coordinates. The second return says whether there was one.
//
// A turn is not folded into the coordinates the way a move and a magnification
// are. Folding one would take a rect out of being a rect and an ellipse out of
// being an ellipse, since neither is square to the page once turned; a node
// keeps them what they are and lets the drawing state carry the angle.
//
// SVG applies the functions in a transform left to right, each inside the last.
// A move or a magnification after a turn would therefore be along turned axes,
// which the frame cannot carry — so it is reported rather than drawn wrong.
func (c *compiler) turnOf(node *html.Node, outer svgFrame, path string) (float64, *origin, bool) {
	stated := strings.TrimSpace(attribute(node, "transform"))
	if stated == "" {
		return 0, nil, false
	}
	frame := outer
	degrees, turned := 0.0, false
	var about *origin
	for _, function := range svgTransforms(stated) {
		name, arguments := function[0], function[1]
		numbers := readNumbers(arguments)
		switch name {
		case "rotate":
			if len(numbers) != 1 && len(numbers) != 3 {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"transform: rotate%s takes an angle, or an angle and a point", arguments))
				continue
			}
			if turned {
				c.warn(path, "unsupported-declaration",
					"transform: a second rotate on one element is not read; write them on nested elements")
				continue
			}
			degrees, turned = numbers[0], true
			if len(numbers) == 3 {
				x, y := frame.place(numbers[1], numbers[2])
				about = &origin{X: pixels(x), Y: pixels(y)}
			}
		case "translate", "scale":
			if turned {
				c.warn(path, "unsupported-declaration", fmt.Sprintf(
					"transform: %s%s comes after a rotate, so it would run along turned axes; it is not read",
					name, arguments))
				continue
			}
			frame = c.transformed(node, frame, path)
		}
	}
	if turned && about == nil {
		// SVG turns about the drawing's origin when no point is named, which
		// is not what CSS does and has to be said as a point rather than left
		// to the node's own default of the middle of its box.
		x, y := frame.place(0, 0)
		about = &origin{X: pixels(x), Y: pixels(y)}
	}
	return degrees, about, turned
}

// svgTransforms splits a transform attribute into the functions it names,
// each as its name and the text of its arguments.
func svgTransforms(value string) [][2]string {
	var functions [][2]string
	for rest := value; ; {
		open := strings.IndexByte(rest, '(')
		if open < 0 {
			return functions
		}
		close := strings.IndexByte(rest[open:], ')')
		if close < 0 {
			return functions
		}
		close += open
		name := strings.TrimSpace(strings.Trim(rest[:open], " ,"))
		functions = append(functions, [2]string{name, rest[open : close+1]})
		rest = rest[close+1:]
	}
}

// readNumbers reads the numbers inside a transform function's brackets.
func readNumbers(arguments string) []float64 {
	inner := strings.Trim(strings.TrimSpace(arguments), "()")
	var numbers []float64
	for _, field := range strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' }) {
		value, ok := svgNumber(field)
		if !ok {
			return nil
		}
		numbers = append(numbers, value)
	}
	return numbers
}

// A clipPath is named where it is defined and used where it is wanted, which
// is the one place a drawing refers to itself. Everything else here is read
// where it is written.
//
// The shape is stated in the drawing's own coordinates rather than the clipped
// element's, so the clip is given the whole viewport to resolve against and the
// element is placed inside it. That is more nodes than clipping the element's
// own box would be, and it is the only arrangement that puts the shape where
// the drawing said it goes.

// clipPaths indexes the clipPath elements a drawing defines, wherever they sit.
func clipPaths(node *html.Node, into map[string]*html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Namespace != svgNamespace {
			continue
		}
		if child.Data == "clipPath" {
			if id := strings.TrimSpace(attribute(child, "id")); id != "" {
				into[id] = child
			}
			continue
		}
		clipPaths(child, into)
	}
}

// clipped wraps a placed shape in the clip its element names.
func (c *compiler) clipped(node *html.Node, box rect, frame svgFrame, shape placed, path string) placed {
	reference := strings.TrimSpace(attribute(node, "clip-path"))
	if reference == "" || reference == "none" {
		return shape
	}
	name, ok := strings.CutPrefix(reference, "url(#")
	name, closed := strings.CutSuffix(name, ")")
	if !ok || !closed {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"clip-path=%q is not a reference to a clipPath in this drawing", reference))
		return shape
	}
	defined, known := c.clips[name]
	if !known {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"clip-path names %q and this drawing defines no clipPath by that name", name))
		return shape
	}
	clip := c.clipNode(defined, c.transformed(defined, frame, path), path)
	if clip == nil {
		return shape
	}
	// The clip resolves against the whole drawing, so it is given the whole
	// drawing's box and the shape is placed inside it.
	clip.Child = &emitted{Type: "absolute", Children: []placed{shape}}
	return placed{Bounds: box, Node: clip}
}

// clipNode reads the one shape a clipPath holds into the node that clips to it.
func (c *compiler) clipNode(defined *html.Node, frame svgFrame, path string) *emitted {
	var only *html.Node
	for child := defined.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Namespace != svgNamespace {
			continue
		}
		if only != nil {
			c.warn(path, "unsupported-declaration",
				"a clipPath here holds one shape; the first is used and the rest are not")
			break
		}
		only = child
	}
	if only == nil {
		c.warn(path, "unsupported-declaration", "a clipPath with no shape in it clips nothing")
		return nil
	}
	inner := c.transformed(only, frame, path)
	switch only.Data {
	case "rect":
		x1, y1 := inner.place(svgLength(only, "x"), svgLength(only, "y"))
		x2, y2 := inner.place(svgLength(only, "x")+svgLength(only, "width"), svgLength(only, "y")+svgLength(only, "height"))
		x, y := math.Min(x1, x2), math.Min(y1, y2)
		width, height := inner.sizeX(svgLength(only, "width")), inner.sizeY(svgLength(only, "height"))
		if width <= 0 || height <= 0 {
			c.warn(path, "unsupported-declaration", "a clipPath's rect has no width or height")
			return nil
		}
		return &emitted{Type: "clipRect", Rect: &rect{
			X: pixels(x), Y: pixels(y), Width: pixels(width), Height: pixels(height)}}
	case "circle":
		writtenRadius := svgLength(only, "r")
		radius := inner.size(writtenRadius)
		if radius <= 0 {
			c.warn(path, "unsupported-declaration", "a clipPath's circle has no radius")
			return nil
		}
		center := atFrame(inner, svgLength(only, "cx"), svgLength(only, "cy"))
		if inner.sizeX(writtenRadius) != inner.sizeY(writtenRadius) {
			return &emitted{Type: "clipShape", Shape: &shape{Kind: "ellipse",
				RadiusX: pixels(inner.sizeX(writtenRadius)), RadiusY: pixels(inner.sizeY(writtenRadius)), Center: center}}
		}
		return &emitted{Type: "clipShape", Shape: &shape{Kind: "circle",
			Radius: pixels(radius), Center: center}}
	case "ellipse":
		radiusX, radiusY := inner.sizeX(svgLength(only, "rx")), inner.sizeY(svgLength(only, "ry"))
		if radiusX <= 0 || radiusY <= 0 {
			c.warn(path, "unsupported-declaration", "a clipPath's ellipse has no radii")
			return nil
		}
		return &emitted{Type: "clipShape", Shape: &shape{Kind: "ellipse",
			RadiusX: pixels(radiusX), RadiusY: pixels(radiusY),
			Center: atFrame(inner, svgLength(only, "cx"), svgLength(only, "cy"))}}
	case "polygon":
		points, err := c.svgPoints(attribute(only, "points"), inner)
		if err != "" {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("a clipPath's polygon: %s", err))
			return nil
		}
		if len(points) < 3 {
			c.warn(path, "unsupported-declaration", "a clipPath's polygon needs at least three corners")
			return nil
		}
		return &emitted{Type: "clipShape", Shape: &shape{Kind: "polygon", Points: points}}
	case "path":
		commands := c.parsePathData(strings.TrimSpace(attribute(only, "d")), inner, path)
		if len(commands) == 0 {
			return nil
		}
		return &emitted{Type: "clipPath", Path: &pathValue{Commands: commands}}
	}
	c.warn(path, "unsupported-declaration", fmt.Sprintf(
		"a clipPath holding a %s is not something this build clips to", only.Data))
	return nil
}

// A pattern tiles a shape with something too small to be worth drawing one at
// a time, which on a panel with no greys is how a fill is made to read as a
// tone. SVG says it with a pattern element holding shapes; the schema says it
// with a grid of characters and what ink each one means.
//
// The two are the same picture written differently, and getting from one to
// the other is filling integer cells from integer rectangles. That is
// arithmetic rather than drawing — a rectangle covers the cells its corners
// say it covers — so it stays on this side of the line. A pattern holding
// anything but rectangles would need the tile actually drawn, and is reported.

// maxTile is as large a pattern tile as this will read. A tile is meant to be
// smaller than the thing it fills; past this it is a picture, and a picture
// belongs in an img.
const maxTile = 64

// patternFill turns a fill of url(#id) into the pattern it names, and reports
// what it could not read. The second return says whether the fill was one.
func (c *compiler) patternFill(value string, path string) (*emitted, bool) {
	name, ok := strings.CutPrefix(strings.TrimSpace(value), "url(#")
	name, closed := strings.CutSuffix(name, ")")
	if !ok || !closed {
		return nil, false
	}
	defined, known := c.patterns[name]
	if !known {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"fill names %q and this drawing defines no pattern by that name", name))
		return nil, true
	}
	width := pixels(svgLength(defined, "width"))
	height := pixels(svgLength(defined, "height"))
	if width < 1 || height < 1 {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"the pattern %q states no tile size", name))
		return nil, true
	}
	if width > maxTile || height > maxTile {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"the pattern %q tiles %dx%d, which is larger than the %d this reads; a tile that size is a picture", name, width, height, maxTile))
		return nil, true
	}
	if units := strings.TrimSpace(attribute(defined, "patternUnits")); units != "" && units != "userSpaceOnUse" {
		c.warn(path, "unsupported-declaration", fmt.Sprintf(
			"the pattern %q is measured in %s; this reads one measured in userSpaceOnUse", name, units))
		return nil, true
	}

	// One character per ink, in the order the inks were first used, so that a
	// tile of one colour reads as one letter rather than whichever the map
	// happened to hand back.
	grid := make([][]byte, height)
	for row := range grid {
		grid[row] = []byte(strings.Repeat(".", width))
	}
	letters := map[string]byte{}
	next := 0
	const alphabet = "xrywabcdefghijkmnopqstuvz"
	drawn := false
	for child := defined.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Namespace != svgNamespace {
			continue
		}
		if child.Data != "rect" {
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"the pattern %q holds a %s; this reads one made of rectangles", name, child.Data))
			continue
		}
		ink := "black"
		if stated := strings.TrimSpace(attribute(child, "fill")); stated != "" {
			parsed, painted := parseInk(stated, "fill", func(message string) {
				c.warn(path, "unsupported-declaration", fmt.Sprintf("the pattern %q: %s", name, message))
			})
			if !painted {
				continue
			}
			ink = inkName(parsed)
		}
		letter, seen := letters[ink]
		if !seen {
			if next >= len(alphabet) {
				continue
			}
			letter = alphabet[next]
			letters[ink] = letter
			next++
		}
		left, top := pixels(svgLength(child, "x")), pixels(svgLength(child, "y"))
		right := left + pixels(svgLength(child, "width"))
		bottom := top + pixels(svgLength(child, "height"))
		for row := max(top, 0); row < min(bottom, height); row++ {
			for column := max(left, 0); column < min(right, width); column++ {
				grid[row][column] = letter
				drawn = true
			}
		}
	}
	if !drawn {
		c.warn(path, "unsupported-declaration", fmt.Sprintf("the pattern %q covers no cell", name))
		return nil, true
	}
	rows := make([]any, height)
	for index, row := range grid {
		rows[index] = string(row)
	}
	inks := make(map[string]string, len(letters))
	for ink, letter := range letters {
		inks[string(letter)] = ink
	}
	return &emitted{Type: "pattern", Rows: rows, Inks: inks}, true
}

// patterns indexes the pattern elements a drawing defines, wherever they sit.
func patterns(node *html.Node, into map[string]*html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Namespace != svgNamespace {
			continue
		}
		if child.Data == "pattern" {
			if id := strings.TrimSpace(attribute(child, "id")); id != "" {
				into[id] = child
			}
			continue
		}
		patterns(child, into)
	}
}
