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
	percent bool
	value   float64
}

func (l length) pixels() int { return int(l.value) }

// style is the computed value of every property this package understands. A
// field left at its zero value was not specified, except where a pointer or a
// length's set flag records the difference explicitly.
type style struct {
	display   displayMode
	direction axis
	basis     length
	grow      int
	gap       int
	padding   compose.Insets
	margin    compose.Insets
	autoLeft  bool
	autoTop   bool
	width     length
	height    length

	background *display.Ink
	color      display.Ink
	border     *border

	fontFamily string
	fontSize   int
	textAlign  display.HorizontalAlign
	textVAlign display.VerticalAlign
	alignItems compose.CrossAlignment

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
	}
}

func rootStyle() style {
	return style{
		color:      display.InkBlack,
		fontFamily: display.DefaultFontFamily,
		fontSize:   display.DefaultFontSize,
	}
}

// apply folds one declaration into the style, reporting anything it cannot
// honour rather than dropping it.
func (s *style) apply(property, value string, report func(string)) {
	value = strings.TrimSpace(value)
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
		s.gap = parseLength(value, property, report).pixels()
	case "align-items":
		switch value {
		case "stretch":
			s.alignItems = compose.CrossStretch
		case "flex-start", "start":
			s.alignItems = compose.CrossStart
		case "center":
			s.alignItems = compose.CrossCenter
		case "flex-end", "end":
			s.alignItems = compose.CrossEnd
		default:
			report(fmt.Sprintf("align-items: %s is not supported", value))
		}
	case "padding":
		s.padding = parseInsets(value, report)
	case "padding-top":
		s.padding.Top = parseLength(value, property, report).pixels()
	case "padding-right":
		s.padding.Right = parseLength(value, property, report).pixels()
	case "padding-bottom":
		s.padding.Bottom = parseLength(value, property, report).pixels()
	case "padding-left":
		s.padding.Left = parseLength(value, property, report).pixels()
	case "margin":
		// The shorthand cannot express auto alignment, which is what the
		// longhands are for; this is spacing only.
		s.margin = parseInsets(value, report)
	case "margin-left":
		if value == "auto" {
			s.autoLeft = true
			return
		}
		s.margin.Left = parseLength(value, property, report).pixels()
	case "margin-right":
		s.margin.Right = parseLength(value, property, report).pixels()
	case "margin-top":
		if value == "auto" {
			s.autoTop = true
			return
		}
		s.margin.Top = parseLength(value, property, report).pixels()
	case "margin-bottom":
		s.margin.Bottom = parseLength(value, property, report).pixels()
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
		radius := parseLength(value, property, report).pixels()
		if s.border == nil {
			s.border = &border{}
		}
		s.border.radius = radius
	case "font-family":
		s.fontFamily = strings.Trim(strings.Fields(value)[0], `"'`)
	case "font-size":
		s.fontSize = parseLength(value, property, report).pixels()
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

func parseLength(value, property string, report func(string)) length {
	switch {
	case value == "auto":
		return length{}
	case strings.HasSuffix(value, "%"):
		number, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			report(fmt.Sprintf("%s: %s is not a percentage", property, value))
			return length{}
		}
		return length{set: true, percent: true, value: number}
	case strings.HasSuffix(value, "px"):
		number, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 64)
		if err != nil {
			report(fmt.Sprintf("%s: %s is not a pixel length", property, value))
			return length{}
		}
		return length{set: true, value: number}
	case value == "0":
		return length{set: true}
	}
	// Every other unit describes a physical or relative size this panel has no
	// way to resolve: there is one device, one density, and no viewport.
	report(fmt.Sprintf("%s: %s must be given in px or %%", property, value))
	return length{}
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
		sides = append(sides, parseLength(field, "padding", report).pixels())
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
			result.width = parseLength(field, "border", report).pixels()
		default:
			if ink, ok := parseInk(field, "border", report); ok {
				result.ink = ink
			}
		}
	}
	return result
}
