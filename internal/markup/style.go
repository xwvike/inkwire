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
	clip       bool
	hidden     bool
	dashed     bool
	absolute   bool
	minSize    [2]length // width, height
	maxSize    [2]length
	inset      [4]length // top, right, bottom, left
	layer      int
	transform  display.Transform
	ratio      float64
	justify    compose.MainAlignment
	// alignSelf overrides the container's align-items for one item. The
	// pointer distinguishes "not stated" from "stretch".
	alignSelf   *compose.CrossAlignment
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
			report(fmt.Sprintf("display: %s is not supported; use block, flex or none", value))
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
		s.gap = parseLength(value, property, report).px()
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
		switch value {
		case "normal":
			s.wrap = display.WrapRunes
		case "nowrap":
			s.wrap = display.NoWrap
		default:
			// pre and its relatives keep the source's own spacing, which this
			// collapses on the way in and cannot get back.
			report(fmt.Sprintf("white-space: %s is not supported; use normal or nowrap", value))
		}
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
		s.wrap = inherited.wrap
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
