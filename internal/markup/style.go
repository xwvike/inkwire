package markup

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

type displayMode uint8

const (
	displayBlock displayMode = iota
	displayFlex
	displayGrid
	// displayContents draws no box of its own; its children take its place
	// in the parent, which is how a wrapper element stays out of a grid.
	displayContents
	displayNone
)

type axis uint8

const (
	axisRow axis = iota
	axisColumn
)

// length distinguishes an absent value from a zero one, and a percentage from
// a pixel count, because "width: 0" and no width at all mean different things.
type length struct {
	set     bool
	percent float64
	pixels  float64
}

func (l length) px() int { return int(l.pixels) }

// fixed reports a length that is already a number of pixels, which is what the
// few places that cannot wait for the layout need.
func (l length) fixed() bool { return l.set && l.percent == 0 }

// style is the computed value of every property this package understands. A
// field left at its zero value was not specified, except where a pointer or a
// length's set flag records the difference explicitly.
type style struct {
	display    displayMode
	direction  axis
	basis      length
	grow       int
	gap        int
	padding    compose.Insets
	margin     compose.Insets
	autoLeft   bool
	autoTop    bool
	autoRight  bool
	autoBottom bool
	width      length
	height     length

	background *display.Ink
	color      display.Ink
	border     *border

	fontFamily string
	fontSize   int
	textAlign  display.HorizontalAlign
	textVAlign display.VerticalAlign
	lineHeight int
	wrap       display.WrapMode
	preserve   bool
	clip       bool
	clipShape  compose.Shape
	hidden     bool
	dashed     bool
	absolute   bool
	minSize    [2]length // width, height
	maxSize    [2]length
	inset      [4]length // top, right, bottom, left
	layer      int
	transform  display.Transform
	ratio      float64
	columns    []compose.Track
	rows       []compose.Track
	rowGap     int
	columnGap  int
	gapSet     bool
	cellColumn [2]int // line, span
	cellRow    [2]int
	justify    compose.MainAlignment
	// alignSelf overrides the container's align-items for one item. The
	// pointer distinguishes "not stated" from "stretch".
	alignSelf   *compose.CrossAlignment
	justifySelf *compose.CrossAlignment
	objectFit   *display.ImageFit
	spaceEvenly bool
	alignItems  compose.CrossAlignment

	// inline starts from the element's own default and is cleared by any
	// display declaration, because a span told to be a block is a block.
	inline bool
}

type border struct {
	width  int
	ink    display.Ink
	radius int
}

// inherited is the subset CSS passes down. Everything else starts fresh on
// each element.
func (s style) inherited() style {
	return style{
		color:      s.color,
		fontFamily: s.fontFamily,
		fontSize:   s.fontSize,
		textAlign:  s.textAlign,
		textVAlign: s.textVAlign,
		lineHeight: s.lineHeight,
		wrap:       s.wrap,
		preserve:   s.preserve,
	}
}

func rootStyle() style {
	return style{
		color:      display.InkBlack,
		fontFamily: display.DefaultFontFamily,
		fontSize:   display.DefaultFontSize,
		// CSS wraps by default. Not wrapping is what white-space: nowrap is
		// for, and a page that silently refused to wrap would lose text with
		// nothing but a clipping warning to show for it.
		wrap: display.WrapRunes,
	}
}

// apply folds one declaration into the style, reporting anything it cannot
// honour rather than dropping it.
func (s *style) apply(property, value string, inherited style, report func(string)) {
	value = strings.TrimSpace(value)
	// inherit takes the parent's value, which is not the same as leaving the
	// field alone: an earlier declaration on this element may already have
	// changed it. initial and unset return the property to its own default.
	switch value {
	case "inherit":
		s.inheritOne(property, inherited)
		return
	case "initial", "unset", "revert":
		s.reset(property)
		return
	}
	switch property {
	case "display":
		switch value {
		case "block":
			s.display, s.inline = displayBlock, false
		case "flex":
			s.display, s.inline = displayFlex, false
		case "grid":
			s.display, s.inline = displayGrid, false
		case "inline-grid":
			s.display, s.inline = displayGrid, false
		case "contents":
			s.display, s.inline = displayContents, false
		case "none":
			s.display = displayNone
		case "inline":
			s.display, s.inline = displayBlock, true
		case "inline-block":
			// Nothing here lays boxes out along a line of text, so an
			// inline-block is a block that happens to sit in a flex line.
			s.display, s.inline = displayBlock, false
		case "inline-flex":
			s.display, s.inline = displayFlex, false
		default:
			report(fmt.Sprintf("display: %s is not supported; use block, flex, grid or none", value))
		}
	case "flex-direction":
		switch value {
		case "row":
			s.direction = axisRow
		case "column":
			s.direction = axisColumn
		default:
			report(fmt.Sprintf("flex-direction: %s is not supported; use row or column", value))
		}
	case "flex-basis":
		s.basis = parseLength(value, property, report)
	case "flex-grow":
		s.grow = parseNumber(value, property, report)
	case "flex":
		// Only the single-number form, which sets grow.
		s.grow = parseNumber(value, property, report)
	case "gap":
		fields := strings.Fields(value)
		s.rowGap = parseLength(fields[0], property, report).px()
		s.columnGap = s.rowGap
		if len(fields) > 1 {
			s.columnGap = parseLength(fields[1], property, report).px()
		}
		s.gap = s.rowGap
		s.gapSet = true
	case "row-gap":
		s.rowGap = parseLength(value, property, report).px()
		s.gap = s.rowGap
		s.gapSet = true
	case "column-gap":
		s.columnGap = parseLength(value, property, report).px()
		s.gapSet = true
	case "grid-template-columns":
		s.columns = parseTracks(value, property, report)
	case "grid-template-rows":
		s.rows = parseTracks(value, property, report)
	case "grid-column":
		s.cellColumn = parseGridLine(value, property, report)
	case "grid-row":
		s.cellRow = parseGridLine(value, property, report)
	case "justify-content":
		switch value {
		case "flex-start", "start", "normal":
			s.justify = compose.MainStart
		case "center":
			s.justify = compose.MainCenter
		case "flex-end", "end":
			s.justify = compose.MainEnd
		case "space-between":
			// Nothing in compose says this, but a growing spacer between each
			// pair of items says exactly it.
			s.spaceEvenly = true
		default:
			report(fmt.Sprintf("justify-content: %s is not supported", value))
		}
	case "line-height":
		s.lineHeight = parseLength(value, property, report).px()
	case "white-space":
		// The two questions white-space answers are whether runs of spaces
		// survive and whether a long line wraps, and the values are the four
		// combinations of those.
		switch value {
		case "normal":
			s.wrap, s.preserve = display.WrapRunes, false
		case "nowrap":
			s.wrap, s.preserve = display.NoWrap, false
		case "pre":
			s.wrap, s.preserve = display.NoWrap, true
		case "pre-wrap":
			s.wrap, s.preserve = display.WrapRunes, true
		default:
			report(fmt.Sprintf(
				"white-space: %s is not supported; use normal, nowrap, pre or pre-wrap", value))
		}
	case "clip-path":
		s.clipShape = parseClipPath(value, property, report)
	case "overflow":
		switch value {
		case "hidden", "clip":
			s.clip = true
		case "visible":
			s.clip = false
		default:
			report(fmt.Sprintf("overflow: %s is not supported; nothing here can scroll", value))
		}
	case "object-fit":
		switch value {
		case "fill":
			s.objectFit = fitOf(display.FitStretch)
		case "contain":
			s.objectFit = fitOf(display.FitContain)
		case "cover":
			s.objectFit = fitOf(display.FitCover)
		default:
			report(fmt.Sprintf("object-fit: %s is not supported; use fill, contain or cover", value))
		}
	case "justify-items":
		if cross, ok := parseCross(value, property, report); ok {
			s.justify = mainOfCross(cross)
		}
	case "justify-self":
		if value == "auto" {
			return
		}
		if cross, ok := parseCross(value, property, report); ok {
			s.justifySelf = &cross
		}
	case "align-self":
		if value == "auto" {
			return
		}
		if cross, ok := parseCross(value, "align-self", report); ok {
			s.alignSelf = &cross
		}
	case "border-width":
		s.borderStyle().width = parseLength(value, property, report).px()
	case "border-style":
		switch value {
		case "solid":
			s.borderStyle()
		case "dashed":
			s.borderStyle()
			s.dashed = true
		default:
			report(fmt.Sprintf("border-style: %s is not supported; use solid or dashed", value))
		}
	case "box-sizing":
		// This is already how every box here behaves: a width is the width of
		// the border box, padding and border included. Saying so is accepted;
		// asking for the CSS default is not, because it is not what happens.
		if value != "border-box" {
			report(fmt.Sprintf(
				"box-sizing: %s is not supported; a width here always includes padding and border", value))
		}
	case "scale":
		// Only a whole number: half a pixel has no meaning on a panel with
		// nothing between set and unset.
		fields := strings.Fields(value)
		factor, err := strconv.Atoi(fields[0])
		if err != nil || factor < 1 {
			report(fmt.Sprintf(
				"scale: %s must be a whole number of at least 1; anything else would have to resample", value))
			return
		}
		if len(fields) > 1 && fields[1] != fields[0] {
			report("scale: the two axes must scale together; this panel cannot stretch one and not the other")
			return
		}
		s.transform.Scale = factor
	case "rotate":
		turns, ok := quarterTurns(value)
		if !ok {
			report(fmt.Sprintf(
				"rotate: %s must be a whole number of quarter turns; anything else would have to resample", value))
			return
		}
		s.transform.Turns = turns
	case "transform":
		// The function form is what most stylesheets say, and it composes:
		// "rotate(90deg) scale(2)" is both, applied together.
		for _, call := range strings.Fields(value) {
			name, argument, ok := strings.Cut(strings.TrimSuffix(call, ")"), "(")
			if !ok {
				report(fmt.Sprintf("transform: %s is not a function call", call))
				return
			}
			switch name {
			case "scale":
				factor, err := strconv.Atoi(argument)
				if err != nil || factor < 1 {
					report(fmt.Sprintf(
						"transform: scale(%s) must be a whole number of at least 1", argument))
					return
				}
				s.transform.Scale = factor
			case "rotate":
				turns, ok := quarterTurns(argument)
				if !ok {
					report(fmt.Sprintf(
						"transform: rotate(%s) must be a whole number of quarter turns", argument))
					return
				}
				s.transform.Turns = turns
			case "none":
				s.transform = display.Transform{}
			default:
				report(fmt.Sprintf(
					"transform: %s() is not supported; only scale and rotate move every pixel onto another pixel",
					name))
				return
			}
		}
	case "aspect-ratio":
		width, height, ok := strings.Cut(value, "/")
		if !ok {
			height = "1"
		}
		numerator, errWidth := strconv.ParseFloat(strings.TrimSpace(width), 64)
		denominator, errHeight := strconv.ParseFloat(strings.TrimSpace(height), 64)
		if errWidth != nil || errHeight != nil || numerator <= 0 || denominator <= 0 {
			report(fmt.Sprintf("aspect-ratio: %s must be a positive ratio such as 16 / 9", value))
			return
		}
		s.ratio = numerator / denominator
	case "position":
		switch value {
		case "static":
			s.absolute = false
		case "absolute":
			s.absolute = true
		case "relative":
			// Nothing here is offset from its flow position, and a relative
			// box with no offsets is indistinguishable from a static one, so
			// this is only accepted for the containing-block role it plays.
		default:
			report(fmt.Sprintf("position: %s is not supported; use static or absolute", value))
		}
	case "top":
		s.inset[0] = parseLength(value, property, report)
	case "right":
		s.inset[1] = parseLength(value, property, report)
	case "bottom":
		s.inset[2] = parseLength(value, property, report)
	case "left":
		s.inset[3] = parseLength(value, property, report)
	case "inset":
		sides := parseInsets(value, report)
		s.inset = [4]length{
			{set: true, pixels: float64(sides.Top)},
			{set: true, pixels: float64(sides.Right)},
			{set: true, pixels: float64(sides.Bottom)},
			{set: true, pixels: float64(sides.Left)},
		}
	case "z-index":
		// Nothing here overlaps except boxes taken out of the flow, and for
		// those the layer is simply the order they are painted in.
		s.layer = parseNumber(value, property, report)
	case "visibility":
		switch value {
		case "visible":
			s.hidden = false
		case "hidden":
			s.hidden = true
		default:
			report(fmt.Sprintf("visibility: %s is not supported; use visible or hidden", value))
		}
	case "border-color":
		if ink, ok := parseInk(value, property, report); ok {
			s.borderStyle().ink = ink
		}
	case "align-items":
		switch value {
		default:
			if cross, ok := parseCross(value, "align-items", report); ok {
				s.alignItems = cross
			}
		}
	case "padding":
		s.padding = parseInsets(value, report)
	case "padding-top":
		s.padding.Top = parseLength(value, property, report).px()
	case "padding-right":
		s.padding.Right = parseLength(value, property, report).px()
	case "padding-bottom":
		s.padding.Bottom = parseLength(value, property, report).px()
	case "padding-left":
		s.padding.Left = parseLength(value, property, report).px()
	case "margin":
		// The shorthand cannot express auto alignment, which is what the
		// longhands are for; this is spacing only.
		s.margin = parseInsets(value, report)
	case "margin-left":
		if value == "auto" {
			s.autoLeft = true
			return
		}
		s.margin.Left = parseLength(value, property, report).px()
	case "margin-right":
		if value == "auto" {
			s.autoRight = true
			return
		}
		s.margin.Right = parseLength(value, property, report).px()
	case "margin-top":
		if value == "auto" {
			s.autoTop = true
			return
		}
		s.margin.Top = parseLength(value, property, report).px()
	case "margin-bottom":
		if value == "auto" {
			s.autoBottom = true
			return
		}
		s.margin.Bottom = parseLength(value, property, report).px()
	case "min-width":
		s.minSize[0] = parseLength(value, property, report)
	case "max-width":
		s.maxSize[0] = parseLength(value, property, report)
	case "min-height":
		s.minSize[1] = parseLength(value, property, report)
	case "max-height":
		s.maxSize[1] = parseLength(value, property, report)
	case "width":
		s.width = parseLength(value, property, report)
	case "height":
		s.height = parseLength(value, property, report)
	case "background", "background-color":
		if ink, ok := parseInk(value, property, report); ok {
			s.background = &ink
		}
	case "color":
		if ink, ok := parseInk(value, property, report); ok {
			s.color = ink
		}
	case "border":
		s.border = parseBorder(value, s.border, report)
	case "border-radius":
		radius := parseLength(value, property, report).px()
		if s.border == nil {
			s.border = &border{}
		}
		s.border.radius = radius
	case "font-family":
		s.fontFamily = strings.Trim(strings.Fields(value)[0], `"'`)
	case "font-size":
		s.fontSize = parseLength(value, property, report).px()
	case "vertical-align":
		// CSS gives this meaning inside a table cell: where the content sits
		// in a box taller than itself. A fixed-height row here is the same
		// situation, and it is the property an author reaches for.
		switch value {
		case "top":
			s.textVAlign = display.AlignTop
		case "middle":
			s.textVAlign = display.AlignMiddle
		case "bottom":
			s.textVAlign = display.AlignBottom
		default:
			report(fmt.Sprintf("vertical-align: %s is not supported; use top, middle or bottom", value))
		}
	case "text-align":
		switch value {
		case "left", "start":
			s.textAlign = display.AlignStart
		case "center":
			s.textAlign = display.AlignCenter
		case "right", "end":
			s.textAlign = display.AlignEnd
		default:
			report(fmt.Sprintf("text-align: %s is not supported", value))
		}
	default:
		report(fmt.Sprintf("%s is not a property this renderer implements", property))
	}
}

// borderStyle returns the border being built, creating it with the CSS
// defaults for anything not yet stated.
func (s *style) borderStyle() *border {
	if s.border == nil {
		s.border = &border{width: 1, ink: display.InkBlack}
	}
	return s.border
}

func fitOf(fit display.ImageFit) *display.ImageFit { return &fit }

// A grid places children along its rows with the same vocabulary a flex line
// uses across them, so the two alignments convert between each other.
func mainOfCross(cross compose.CrossAlignment) compose.MainAlignment {
	switch cross {
	case compose.CrossCenter:
		return compose.MainCenter
	case compose.CrossEnd:
		return compose.MainEnd
	}
	return compose.MainStart
}

func crossOfMain(main compose.MainAlignment) compose.CrossAlignment {
	switch main {
	case compose.MainCenter:
		return compose.CrossCenter
	case compose.MainEnd:
		return compose.CrossEnd
	}
	return compose.CrossStretch
}

// quarterTurns accepts the angles that move every pixel onto another pixel.
// Between them a rotation has to decide which of two pixels a sample belongs
// to, and either answer thins some strokes and thickens others.
func quarterTurns(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "none" || trimmed == "0" {
		return 0, true
	}
	degrees, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "deg"), 64)
	if err != nil {
		return 0, false
	}
	if degrees != float64(int(degrees)) || int(degrees)%90 != 0 {
		return 0, false
	}
	return ((int(degrees)/90)%4 + 4) % 4, true
}

// inheritOne copies one property from the value the parent passed down.
func (s *style) inheritOne(property string, inherited style) {
	switch property {
	case "color":
		s.color = inherited.color
	case "font-family":
		s.fontFamily = inherited.fontFamily
	case "font-size":
		s.fontSize = inherited.fontSize
	case "text-align":
		s.textAlign = inherited.textAlign
	case "vertical-align":
		s.textVAlign = inherited.textVAlign
	case "line-height":
		s.lineHeight = inherited.lineHeight
	case "white-space":
		s.wrap, s.preserve = inherited.wrap, inherited.preserve
	}
}

// reset returns one property to the value it would have had with no
// declaration at all.
func (s *style) reset(property string) {
	fresh := style{}
	switch property {
	case "color":
		s.color = display.InkBlack
	case "background", "background-color":
		s.background = nil
	case "border", "border-width", "border-style", "border-color", "border-radius":
		s.border, s.dashed = nil, false
	case "padding":
		s.padding = fresh.padding
	case "margin":
		s.margin = fresh.margin
	case "width":
		s.width = fresh.width
	case "height":
		s.height = fresh.height
	case "font-size":
		s.fontSize = display.DefaultFontSize
	case "font-family":
		s.fontFamily = display.DefaultFontFamily
	case "display":
		s.display = displayBlock
	}
}

// parseTracks reads a track list: lengths, fr units, auto, and repeat().
func parseTracks(value, property string, report func(string)) []compose.Track {
	var tracks []compose.Track
	for _, field := range expandRepeats(value, property, report) {
		switch {
		case field == "auto" || field == "min-content" || field == "max-content":
			tracks = append(tracks, compose.Track{})
		case strings.HasSuffix(field, "fr"):
			count, err := strconv.ParseFloat(strings.TrimSuffix(field, "fr"), 64)
			if err != nil || count <= 0 {
				report(fmt.Sprintf("%s: %s is not a positive number of fr", property, field))
				return nil
			}
			tracks = append(tracks, compose.Track{Fraction: int(count)})
		default:
			size := parseLength(field, property, report)
			if !size.set {
				return nil
			}
			tracks = append(tracks, compose.Track{
				Size: compose.Calc(int(size.percent*10), int(size.pixels)),
			})
		}
	}
	return tracks
}

// expandRepeats turns repeat(3, 1fr) into three fields, which is the only
// function in a track list worth having: the rest describe intrinsic sizes
// this vocabulary does not distinguish.
func expandRepeats(value, property string, report func(string)) []string {
	var fields []string
	for rest := strings.TrimSpace(value); rest != ""; {
		if !strings.HasPrefix(rest, "repeat(") {
			field, remainder, _ := strings.Cut(rest, " ")
			if field != "" {
				fields = append(fields, field)
			}
			rest = strings.TrimSpace(remainder)
			continue
		}
		inner, remainder, ok := strings.Cut(strings.TrimPrefix(rest, "repeat("), ")")
		if !ok {
			report(fmt.Sprintf("%s: repeat( is never closed", property))
			return nil
		}
		countText, pattern, ok := strings.Cut(inner, ",")
		count, err := strconv.Atoi(strings.TrimSpace(countText))
		if !ok || err != nil || count < 1 {
			report(fmt.Sprintf("%s: repeat() needs a whole number and a track list", property))
			return nil
		}
		for i := 0; i < count; i++ {
			fields = append(fields, strings.Fields(pattern)...)
		}
		rest = strings.TrimSpace(remainder)
	}
	return fields
}

// parseGridLine reads "3", "span 2" or "2 / 4" into a line and a span.
func parseGridLine(value, property string, report func(string)) [2]int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "auto" {
		return [2]int{}
	}
	if start, end, ok := strings.Cut(trimmed, "/"); ok {
		from, errFrom := strconv.Atoi(strings.TrimSpace(start))
		to, errTo := strconv.Atoi(strings.TrimSpace(end))
		if errFrom != nil || errTo != nil || from < 1 || to <= from {
			report(fmt.Sprintf("%s: %s must be two increasing line numbers", property, value))
			return [2]int{}
		}
		return [2]int{from, to - from}
	}
	if rest, ok := strings.CutPrefix(trimmed, "span "); ok {
		count, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || count < 1 {
			report(fmt.Sprintf("%s: span needs a whole number, got %s", property, rest))
			return [2]int{}
		}
		return [2]int{0, count}
	}
	line, err := strconv.Atoi(trimmed)
	if err != nil || line < 1 {
		report(fmt.Sprintf("%s: %s is not a line number", property, value))
		return [2]int{}
	}
	return [2]int{line, 1}
}

// parseClipPath reads the basic shapes. They are the ones that describe an
// outline rather than an image, which is what this panel can clip to: the
// clip is a mask of set and unset pixels, and a shape says exactly which.
func parseClipPath(value, property string, report func(string)) compose.Shape {
	trimmed := strings.TrimSpace(value)
	if trimmed == "none" {
		return compose.Shape{}
	}
	name, argument, ok := strings.Cut(strings.TrimSuffix(trimmed, ")"), "(")
	if !ok {
		report(fmt.Sprintf("%s: %s is not a shape function", property, value))
		return compose.Shape{}
	}
	lengths := func(fields []string) []compose.Length {
		result := make([]compose.Length, 0, len(fields))
		for _, field := range fields {
			size := parseLength(field, property, report)
			if !size.set {
				return nil
			}
			result = append(result, lengthOf(size))
		}
		return result
	}
	switch name {
	case "inset":
		body, corner, rounded := strings.Cut(argument, " round ")
		sides := lengths(strings.Fields(body))
		if len(sides) == 0 || len(sides) > 4 {
			report(fmt.Sprintf("%s: inset() needs one to four lengths", property))
			return compose.Shape{}
		}
		shape := compose.Shape{Kind: compose.ShapeInset}
		shape.Insets = [4]compose.Length{sides[0], sides[0], sides[0], sides[0]}
		if len(sides) > 1 {
			shape.Insets[1], shape.Insets[3] = sides[1], sides[1]
		}
		if len(sides) > 2 {
			shape.Insets[2] = sides[2]
		}
		if len(sides) > 3 {
			shape.Insets[3] = sides[3]
		}
		if rounded {
			radii := lengths(strings.Fields(corner))
			if len(radii) != 1 {
				report(fmt.Sprintf("%s: inset() rounds by a single radius", property))
				return compose.Shape{}
			}
			shape.Corner = radii[0]
		}
		return shape
	case "circle", "ellipse":
		body, centre, positioned := strings.Cut(argument, " at ")
		radii := lengths(strings.Fields(body))
		shape := compose.Shape{Kind: compose.ShapeCircle}
		if name == "ellipse" {
			shape.Kind = compose.ShapeEllipse
			if len(radii) != 2 {
				report(fmt.Sprintf("%s: ellipse() needs two radii", property))
				return compose.Shape{}
			}
			shape.RadiusX, shape.RadiusY = radii[0], radii[1]
		} else {
			if len(radii) > 1 {
				report(fmt.Sprintf("%s: circle() takes one radius", property))
				return compose.Shape{}
			}
			if len(radii) == 1 {
				shape.Radius = radii[0]
			}
		}
		if positioned {
			at := lengths(strings.Fields(centre))
			if len(at) != 2 {
				report(fmt.Sprintf("%s: at needs an x and a y", property))
				return compose.Shape{}
			}
			shape.Centre = [2]compose.Length{at[0], at[1]}
		}
		return shape
	case "polygon":
		shape := compose.Shape{Kind: compose.ShapePolygon}
		for _, pair := range strings.Split(argument, ",") {
			at := lengths(strings.Fields(pair))
			if len(at) != 2 {
				report(fmt.Sprintf("%s: each polygon corner needs an x and a y", property))
				return compose.Shape{}
			}
			shape.Points = append(shape.Points, [2]compose.Length{at[0], at[1]})
		}
		if len(shape.Points) < 3 {
			report(fmt.Sprintf("%s: a polygon needs at least three corners", property))
			return compose.Shape{}
		}
		return shape
	}
	report(fmt.Sprintf(
		"%s: %s() is not supported; use inset, circle, ellipse or polygon", property, name))
	return compose.Shape{}
}

func parseCross(value, property string, report func(string)) (compose.CrossAlignment, bool) {
	switch value {
	case "stretch":
		return compose.CrossStretch, true
	case "flex-start", "start":
		return compose.CrossStart, true
	case "center":
		return compose.CrossCenter, true
	case "flex-end", "end":
		return compose.CrossEnd, true
	}
	report(fmt.Sprintf("%s: %s is not supported", property, value))
	return 0, false
}

func parseLength(value, property string, report func(string)) length {
	switch {
	case value == "auto":
		return length{}
	case strings.HasPrefix(value, "calc("):
		return parseCalc(value, property, report)
	case strings.HasSuffix(value, "%"):
		number, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			report(fmt.Sprintf("%s: %s is not a percentage", property, value))
			return length{}
		}
		return length{set: true, percent: number}
	case strings.HasSuffix(value, "px"):
		number, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 64)
		if err != nil {
			report(fmt.Sprintf("%s: %s is not a pixel length", property, value))
			return length{}
		}
		return length{set: true, pixels: number}
	case value == "0":
		return length{set: true}
	}
	// Every other unit describes a physical or relative size this panel has no
	// way to resolve: there is one device, one density, and no viewport.
	report(fmt.Sprintf("%s: %s must be given in px or %%", property, value))
	return length{}
}

// parseCalc reads the sums of lengths that calc() is used for here. Only
// addition and subtraction of percentages and pixels are accepted: the rest of
// the grammar multiplies and divides by unitless numbers, which nothing in
// this vocabulary needs and which would invite expressions no panel can
// answer, such as a length divided by a length.
func parseCalc(value, property string, report func(string)) length {
	inner := strings.TrimSuffix(strings.TrimPrefix(value, "calc("), ")")
	fields := strings.Fields(inner)
	if len(fields) == 0 {
		report(fmt.Sprintf("%s: calc() is empty", property))
		return length{}
	}
	result := length{set: true}
	sign := 1.0
	expectTerm := true
	for _, field := range fields {
		if !expectTerm {
			switch field {
			case "+":
				sign = 1
			case "-":
				sign = -1
			default:
				report(fmt.Sprintf(
					"%s: calc() understands + and - between lengths, not %q", property, field))
				return length{}
			}
			expectTerm = true
			continue
		}
		term := parseLength(field, property, report)
		if !term.set {
			return length{}
		}
		result.percent += sign * term.percent
		result.pixels += sign * term.pixels
		expectTerm = false
	}
	if expectTerm {
		report(fmt.Sprintf("%s: calc() ends with an operator and no length after it", property))
		return length{}
	}
	return result
}

func parseNumber(value, property string, report func(string)) int {
	number, err := strconv.ParseFloat(strings.Fields(value)[0], 64)
	if err != nil {
		report(fmt.Sprintf("%s: %s is not a number", property, value))
		return 0
	}
	return int(number)
}

func parseInsets(value string, report func(string)) compose.Insets {
	fields := strings.Fields(value)
	sides := make([]int, 0, 4)
	for _, field := range fields {
		sides = append(sides, parseLength(field, "padding", report).px())
	}
	switch len(sides) {
	case 1:
		return compose.Insets{Top: sides[0], Right: sides[0], Bottom: sides[0], Left: sides[0]}
	case 2:
		return compose.Insets{Top: sides[0], Right: sides[1], Bottom: sides[0], Left: sides[1]}
	case 3:
		return compose.Insets{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[1]}
	case 4:
		return compose.Insets{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}
	}
	report(fmt.Sprintf("padding: %q needs one to four lengths", value))
	return compose.Insets{}
}

func parseInk(value, property string, report func(string)) (display.Ink, bool) {
	switch strings.ToLower(value) {
	case "black", "#000", "#000000":
		return display.InkBlack, true
	case "white", "#fff", "#ffffff":
		return display.InkWhite, true
	case "red", "#f00", "#ff0000":
		return display.InkRed, true
	case "transparent", "none":
		return 0, false
	}
	// Anything else would have to be approximated, and approximating a colour
	// on a panel with three of them is how a design silently stops matching.
	report(fmt.Sprintf("%s: %s is not one of the panel's inks (black, white, red)", property, value))
	return 0, false
}

func parseBorder(value string, existing *border, report func(string)) *border {
	fields := strings.Fields(value)
	if len(fields) == 0 || fields[0] == "none" {
		return existing
	}
	result := &border{ink: display.InkBlack, width: 1}
	if existing != nil {
		result.radius = existing.radius
	}
	for _, field := range fields {
		switch {
		case field == "solid":
		case strings.HasSuffix(field, "px"):
			result.width = parseLength(field, "border", report).px()
		default:
			if ink, ok := parseInk(field, "border", report); ok {
				result.ink = ink
			}
		}
	}
	return result
}
