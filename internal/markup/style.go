package markup

import (
	"fmt"
	"math"
	"slices"
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
	display       displayMode
	direction     axis
	basis         length
	grow          float64
	shrink        float64
	padding       compose.Insets
	paddingLength [4]length
	// borderBox is box-sizing. CSS starts at content-box, where a stated width
	// is the width of the content and the padding and border go outside it;
	// border-box is the one nearly every stylesheet opts into, where the width
	// is the whole box and the content is what is left.
	borderBox    bool
	margin       compose.Insets
	marginLength [4]length
	autoLeft     bool
	autoTop      bool
	autoRight    bool
	autoBottom   bool
	width        length
	height       length

	background *display.Ink
	color      display.Ink
	border     *border
	borderSide [4]*borderSide

	fontFamily   string
	fontSize     int
	textAlign    display.HorizontalAlign
	inlineVAlign compose.InlineVerticalAlign
	lineHeight   int
	// lineHeightMultiple is a line-height written as a bare number. CSS keeps
	// the number rather than the length it comes to, because it is resolved
	// against each element's own font size — so it survives here as the number
	// and is worked out once the declarations are all in. Written as pixels
	// straight away, "line-height: 1.5; font-size: 20px" and the same two the
	// other way round gave different lines, and a child inherited its parent's
	// pixels rather than its parent's ratio.
	lineHeightMultiple float64
	// A percentage is a multiple that is resolved on this element and inherits
	// as the length it came to, which is where CSS parts company with the bare
	// number.
	lineHeightResolvesHere bool
	wrap                   display.WrapMode
	preserve               bool
	clip                   bool
	clipShape              compose.Shape
	hidden                 bool
	// line is border-style. stroke-dasharray states a pattern directly, which
	// is SVG's spelling of the same thing and overrides the one a style implies.
	line borderLine
	// dash is the pattern a stylesheet stated. Empty means the one a dashed
	// border gets when nobody says: a mark three times the border's width
	// and a gap twice it, which reads as dashes at any width.
	dash       []int
	dashOffset int
	absolute   bool
	positioned bool
	minSize    [2]length // width, height
	maxSize    [2]length
	inset      [4]length // top, right, bottom, left
	// insetFromShorthand records, per edge, that it came from inset rather
	// than from its own name, so a message about it can say where it is from.
	// An author who wrote "inset: 0" never typed the word right — and one who
	// wrote "inset: 0; left: 10px" did type left, so it is theirs by then.
	insetFromShorthand [4]bool
	layer              int
	transform          display.Transform
	// rotate is an angle in degrees, held apart from transform because the two
	// are no longer the same kind of thing: a magnification redraws a subtree
	// onto a larger surface, and a turn puts a turn into the state everything
	// under it works its own geometry out through. One is exact at whole
	// numbers, the other at any angle at all.
	rotate          float64
	rotateOrigin    *[2]length
	ratio           float64
	columns         []compose.Track
	rows            []compose.Track
	rowGap          int
	columnGap       int
	rowGapLength    length
	columnGapLength length
	gapSet          bool
	// The three properties a drawing is painted with. SVG states them as
	// attributes and CSS states them as properties, and CSS wins — a
	// presentation attribute is a rule of no specificity, so any selector
	// beats it. Held as pointers because not stating one and stating it as
	// none are different answers, and both have to survive being inherited.
	fill        *paint
	stroke      *paint
	strokeWidth *int
	cellColumn  [2]int // line, span
	cellRow     [2]int
	justify     compose.MainAlignment
	// alignSelf overrides the container's align-items for one item. The
	// pointer distinguishes "not stated" from "stretch".
	alignSelf   *compose.CrossAlignment
	justifySelf *compose.CrossAlignment
	objectFit   *display.ImageFit
	spaceEvenly bool
	alignItems  compose.CrossAlignment

	// inline starts from the element's own default and is cleared by any
	// display declaration, because a span told to be a block is a block.
	inline       bool
	inlineAtomic bool
	// blockified records the outer display adjustment imposed on a flex/grid
	// item. Its inner formatting mode remains available for descendants, but
	// inline-only properties do not apply to the item box itself.
	blockified bool
}

type border struct {
	width  int
	ink    display.Ink
	radius int
}

type borderSide struct {
	width      int
	ink        display.Ink
	line       borderLine
	dash       []int
	dashOffset int
}

// borderLine is border-style. CSS starts it at none, so a border with a width
// and no style draws nothing — which is what "border: 1px" means, and what a
// style this cannot draw falls back to rather than being drawn as something
// else.
type borderLine uint8

const (
	borderNone borderLine = iota
	borderSolid
	borderDashed
	borderDotted
)

// dashOf is the pattern a line style is drawn with, in multiples of the
// border's own width. CSS makes a dot as wide as the border and spaces it by
// the same; a dash is longer than it is wide. Solid has no pattern.
func (l borderLine) dashOf(width int) []int {
	switch l {
	case borderDashed:
		return []int{width * 3, width * 2}
	case borderDotted:
		return []int{width, width}
	}
	return nil
}

// parseBorderLine reads a border-style keyword. The ones this cannot draw are
// told apart from the ones that are not styles at all, because an author who
// wrote "dotted" wants to know it is a style rather than a colour, and one who
// wrote "double" wants to know it is a style this does not have.
func parseBorderLine(value, property string, report func(string)) (borderLine, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "solid":
		return borderSolid, true
	case "dashed":
		return borderDashed, true
	case "dotted":
		return borderDotted, true
	case "none", "hidden":
		return borderNone, true
	case "double", "groove", "ridge", "inset", "outset":
		report(fmt.Sprintf(
			"%s: %s needs more than one line or more than one shade, so it is not drawn; "+
				"use solid, dashed, dotted or none", property, value))
		return borderNone, true
	}
	return borderNone, false
}

// inherited is the subset CSS passes down. Everything else starts fresh on
// each element.
// paint is an ink, or the absence of one. fill: none and no fill at all are
// not the same: the first says do not paint, the second says ask the parent.
type paint struct {
	ink  display.Ink
	none bool
}

func (s style) inherited() style {
	return style{
		fill:        s.fill,
		stroke:      s.stroke,
		strokeWidth: s.strokeWidth,
		color:       s.color,
		fontFamily:  s.fontFamily,
		fontSize:    s.fontSize,
		textAlign:   s.textAlign,
		// vertical-align is not inherited, as in CSS: it says where one
		// inline box sits on its own line and means nothing to a descendant.
		lineHeight: s.lineHeight,
		// The ratio travels, not the pixels it came to: a child with its own
		// font size gets its own line from the same ratio.
		lineHeightMultiple: s.lineHeightMultiple,
		wrap:               s.wrap,
		preserve:           s.preserve,
		hidden:             s.hidden,
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
func (s *style) apply(property, value string, parent style, report func(string)) {
	value = strings.TrimSpace(value)
	// CSS matches keywords and units without regard to case, so the keywords
	// below are compared against this rather than against what was typed. The
	// value itself is kept as written for the things that are not keywords: a
	// font family is a name, and an author who asked for "Helvetica Neue"
	// wants it named back the way they wrote it.
	keyword := strings.ToLower(value)
	// A declaration with nothing after the colon says nothing, and no property
	// here has a meaning for it. Refusing it once is also what keeps the
	// parsers below from having to: several of them read the first word of the
	// value, and "gap:;" reached them as no words at all.
	if value == "" {
		report(fmt.Sprintf("%s: there is no value after the colon", property))
		return
	}
	// inherit takes the parent's value, which is not the same as leaving the
	// field alone: an earlier declaration on this element may already have
	// changed it. initial returns the property's own default; unset inherits
	// when CSS defines the property as inherited and otherwise resets it.
	switch keyword {
	case "inherit":
		s.inheritOne(property, parent, report)
		return
	case "initial", "revert":
		s.reset(property, report)
		return
	case "unset":
		if isInheritedProperty(property) {
			s.inheritOne(property, parent, report)
		} else {
			s.reset(property, report)
		}
		return
	}
	switch property {
	case "display":
		switch keyword {
		case "block":
			s.display, s.inline, s.inlineAtomic = displayBlock, false, false
		case "flex":
			s.display, s.inline, s.inlineAtomic = displayFlex, false, false
		case "grid":
			s.display, s.inline, s.inlineAtomic = displayGrid, false, false
		case "inline-grid":
			s.display, s.inline, s.inlineAtomic = displayGrid, false, true
		case "contents":
			s.display, s.inline, s.inlineAtomic = displayContents, false, false
		case "none":
			s.display = displayNone
		case "inline":
			s.display, s.inline, s.inlineAtomic = displayBlock, true, false
		case "inline-block":
			s.display, s.inline, s.inlineAtomic = displayBlock, false, true
		case "inline-flex":
			s.display, s.inline, s.inlineAtomic = displayFlex, false, true
		default:
			report(fmt.Sprintf("display: %s is not supported; use block, flex, grid or none", value))
		}
	case "flex-direction":
		switch keyword {
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
		s.grow = parseFlexFactor(value, property, report)
	case "flex-shrink":
		s.shrink = parseFlexFactor(value, property, report)
	case "flex":
		grow, shrink, basis, ok := parseFlexShorthand(value, property, report)
		if !ok {
			return
		}
		s.grow, s.shrink, s.basis = grow, shrink, basis
	case "gap":
		fields := splitSides(value)
		if len(fields) == 0 {
			report(fmt.Sprintf("gap: %q is not a length", value))
			return
		}
		rowLength := parseLength(fields[0], property, report)
		if !rowLength.set || negativeLength(rowLength) {
			if rowLength.set && negativeLength(rowLength) {
				report(fmt.Sprintf("%s: %s must not be negative", property, fields[0]))
			}
			return
		}
		s.rowGapLength, s.columnGapLength = rowLength, rowLength
		s.rowGap, s.columnGap = int(rowLength.pixels), int(rowLength.pixels)
		if len(fields) > 1 {
			columnLength := parseLength(fields[1], property, report)
			if !columnLength.set || negativeLength(columnLength) {
				if columnLength.set && negativeLength(columnLength) {
					report(fmt.Sprintf("%s: %s must not be negative", property, fields[1]))
				}
				return
			}
			s.columnGapLength, s.columnGap = columnLength, int(columnLength.pixels)
		}
		s.gapSet = true
	case "row-gap":
		rowLength := parseLength(value, property, report)
		if !rowLength.set || negativeLength(rowLength) {
			if rowLength.set && negativeLength(rowLength) {
				report(fmt.Sprintf("%s: %s must not be negative", property, value))
			}
			return
		}
		s.rowGapLength, s.rowGap = rowLength, int(rowLength.pixels)
		s.gapSet = true
	case "column-gap":
		columnLength := parseLength(value, property, report)
		if !columnLength.set || negativeLength(columnLength) {
			if columnLength.set && negativeLength(columnLength) {
				report(fmt.Sprintf("%s: %s must not be negative", property, value))
			}
			return
		}
		s.columnGapLength, s.columnGap = columnLength, int(columnLength.pixels)
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
		switch keyword {
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
		// A bare number is a multiple of the font size, which is how CSS
		// recommends line-height be written and therefore how it usually is.
		// Refusing it sent an author looking for a unit that the property
		// they copied did not have.
		if multiple, err := parseFinite(strings.TrimSpace(value)); err == nil {
			if multiple <= 0 {
				report(fmt.Sprintf("line-height: %s must be more than zero", value))
				return
			}
			s.lineHeightMultiple, s.lineHeightResolvesHere = multiple, false
			return
		}
		// A percentage is the same ratio said differently, and is the one
		// spelling of it that stops being a ratio on the element it is on.
		if size := parseLength(value, property, report); size.set && size.percent != 0 {
			if size.pixels != 0 {
				report(fmt.Sprintf("line-height: %s must be a share or a length, not both", value))
				return
			}
			if size.percent <= 0 {
				report(fmt.Sprintf("line-height: %s must be more than zero", value))
				return
			}
			s.lineHeightMultiple, s.lineHeightResolvesHere = size.percent/100, true
			return
		}
		if pixels, ok := wholeLength(value, property, report); ok {
			s.lineHeight, s.lineHeightMultiple = pixels, 0
		}
	case "white-space":
		// The two questions white-space answers are whether runs of spaces
		// survive and whether a long line wraps, and the values are the four
		// combinations of those.
		switch keyword {
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
		switch keyword {
		case "hidden", "clip":
			s.clip = true
		case "visible":
			s.clip = false
		default:
			report(fmt.Sprintf("overflow: %s is not supported; nothing here can scroll", value))
		}
	case "object-fit":
		switch keyword {
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
		if keyword == "auto" {
			return
		}
		if cross, ok := parseCross(value, property, report); ok {
			s.justifySelf = &cross
		}
	case "align-self":
		if keyword == "auto" {
			return
		}
		if cross, ok := parseCross(value, "align-self", report); ok {
			s.alignSelf = &cross
		}
	case "border-width":
		if width, ok := wholeLength(value, property, report); ok {
			s.borderStyle().width = width
			for index := range s.borderSide {
				if s.borderSide[index] != nil {
					s.borderSide[index].width = width
				}
			}
		}
	case "border-style":
		line, ok := parseBorderLine(value, property, report)
		if !ok {
			report(fmt.Sprintf("border-style: %s is not a border style; use solid, dashed, dotted or none", value))
			return
		}
		s.borderStyle()
		s.line = line
		for index := range s.borderSide {
			if s.borderSide[index] != nil {
				s.borderSide[index].line = line
				s.borderSide[index].dash, s.borderSide[index].dashOffset = nil, 0
			}
		}
		if line == borderNone {
			s.dash, s.dashOffset = nil, 0
		}
	// SVG's two names for the same thing a border does here. They are used
	// rather than something of this package's own invention because they are
	// what a dashed line is called everywhere else, and a subset that renames
	// what it borrows is a dialect.
	case "fill", "stroke":
		// A drawing's two paints. They inherit, which is what lets a group
		// state them once for everything inside it.
		value = strings.TrimSpace(value)
		if keyword == "none" || keyword == "transparent" {
			s.setPaint(property, &paint{none: true})
			return
		}
		ink, ok := parseInk(value, property, report)
		if !ok {
			return
		}
		s.setPaint(property, &paint{ink: ink})
	case "stroke-width":
		// SVG writes this one without a unit, and a stylesheet that names a
		// shape is usually a stylesheet somebody moved out of the shape's own
		// attributes. A bare number means pixels here as it does there.
		width := parseLength(bareNumberInPixels(value), property, report)
		if !width.fixed() {
			return
		}
		pixels := width.px()
		if pixels < 1 {
			report(fmt.Sprintf("stroke-width: %s is less than a pixel; drawn at 1", value))
			pixels = 1
		}
		s.strokeWidth = &pixels
	case "transform-origin":
		// The point a turn happens about, written the way CSS writes it: one
		// or two values, each a keyword, a share of the box or a distance
		// into it. Two keywords may be written either way round, which is why
		// they are sorted out before anything is measured.
		origin, ok := parseTransformOrigin(value, property, report)
		if !ok {
			return
		}
		s.rotateOrigin = origin
	case "stroke-dasharray":
		fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
		pattern := make([]int, 0, len(fields))
		for _, field := range fields {
			pixels, ok := wholePixels(field)
			if !ok || pixels < 0 {
				report(fmt.Sprintf("stroke-dasharray: %s is a run of whole pixels, such as \"4 2\"", value))
				return
			}
			pattern = append(pattern, pixels)
		}
		if len(pattern) == 0 {
			report("stroke-dasharray: give at least one length, such as \"4 2\"")
			return
		}
		s.borderStyle()
		s.line, s.dash = borderDashed, pattern
	case "stroke-dashoffset":
		pixels, ok := wholePixels(value)
		if !ok {
			report(fmt.Sprintf("stroke-dashoffset: %s is a whole number of pixels", value))
			return
		}
		s.borderStyle()
		s.dashOffset = pixels
	case "box-sizing":
		switch keyword {
		case "border-box":
			s.borderBox = true
		case "content-box":
			s.borderBox = false
		default:
			report(fmt.Sprintf("box-sizing: %s is not supported; use content-box or border-box", value))
		}
	case "scale":
		// Only a whole number: half a pixel has no meaning on a panel with
		// nothing between set and unset.
		fields := strings.Fields(value)
		if len(fields) == 0 {
			report(fmt.Sprintf("scale: %q is not a number", value))
			return
		}
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
		degrees, ok := parseAngle(value)
		if !ok {
			report(fmt.Sprintf("rotate: %s is not an angle; write it in degrees, such as 37deg", value))
			return
		}
		s.rotate = degrees
	case "transform":
		// none is the initial value and clears whatever was set.
		if keyword == "none" {
			s.transform, s.rotate, s.rotateOrigin = display.Transform{}, 0, nil
			return
		}
		// The function form is what most stylesheets say, and it composes:
		// "rotate(90deg) scale(2)" is both, applied together.
		for _, call := range strings.Fields(value) {
			name, argument, ok := strings.Cut(strings.TrimSuffix(call, ")"), "(")
			if !ok {
				report(fmt.Sprintf("transform: %s is not a function call", call))
				return
			}
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "scale":
				factor, err := strconv.Atoi(argument)
				if err != nil || factor < 1 {
					report(fmt.Sprintf(
						"transform: scale(%s) must be a whole number of at least 1", argument))
					return
				}
				s.transform.Scale = factor
			case "rotate":
				degrees, ok := parseAngle(argument)
				if !ok {
					report(fmt.Sprintf("transform: rotate(%s) is not an angle; write it in degrees", argument))
					return
				}
				s.rotate = degrees
			case "none":
				s.transform, s.rotate, s.rotateOrigin = display.Transform{}, 0, nil
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
		numerator, errWidth := parseFinite(strings.TrimSpace(width))
		denominator, errHeight := parseFinite(strings.TrimSpace(height))
		if errWidth != nil || errHeight != nil || numerator <= 0 || denominator <= 0 {
			report(fmt.Sprintf("aspect-ratio: %s must be a positive ratio such as 16 / 9", value))
			return
		}
		s.ratio = numerator / denominator
	case "position":
		switch keyword {
		case "static":
			s.absolute, s.positioned = false, false
		case "absolute":
			s.absolute, s.positioned = true, true
		case "relative":
			// A relative box stays in flow, establishes the containing block for
			// descendants, and is wrapped with its offsets when the scene is emitted.
			s.absolute, s.positioned = false, true
		default:
			report(fmt.Sprintf("position: %s is not supported; use static, relative or absolute", value))
		}
	case "top":
		s.inset[0], s.insetFromShorthand[0] = parseLength(value, property, report), false
	case "right":
		s.inset[1], s.insetFromShorthand[1] = parseLength(value, property, report), false
	case "bottom":
		s.inset[2], s.insetFromShorthand[2] = parseLength(value, property, report), false
	case "left":
		s.inset[3], s.insetFromShorthand[3] = parseLength(value, property, report), false
	case "inset":
		s.inset = parseInsetLengths(value, property, report)
		s.insetFromShorthand = [4]bool{true, true, true, true}
	case "z-index":
		// Nothing here overlaps except boxes taken out of the flow, and for
		// those the layer is simply the order they are painted in.
		s.layer = parseNumber(value, property, report)
	case "visibility":
		switch keyword {
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
			for index := range s.borderSide {
				if s.borderSide[index] != nil {
					s.borderSide[index].ink = ink
				}
			}
		}
	case "align-items":
		switch keyword {
		default:
			if cross, ok := parseCross(value, "align-items", report); ok {
				s.alignItems = cross
			}
		}
	case "padding":
		s.padding, s.paddingLength = parseStyleInsets(value, property, report)
	case "padding-top":
		parsed := parseLength(value, property, report)
		if parsed.set && !negativeLength(parsed) {
			s.paddingLength[0], s.padding.Top = parsed, int(parsed.pixels)
		} else if parsed.set {
			report(fmt.Sprintf("%s: %s must not be negative", property, value))
		}
	case "padding-right":
		parsed := parseLength(value, property, report)
		if parsed.set && !negativeLength(parsed) {
			s.paddingLength[1], s.padding.Right = parsed, int(parsed.pixels)
		} else if parsed.set {
			report(fmt.Sprintf("%s: %s must not be negative", property, value))
		}
	case "padding-bottom":
		parsed := parseLength(value, property, report)
		if parsed.set && !negativeLength(parsed) {
			s.paddingLength[2], s.padding.Bottom = parsed, int(parsed.pixels)
		} else if parsed.set {
			report(fmt.Sprintf("%s: %s must not be negative", property, value))
		}
	case "padding-left":
		parsed := parseLength(value, property, report)
		if parsed.set && !negativeLength(parsed) {
			s.paddingLength[3], s.padding.Left = parsed, int(parsed.pixels)
		} else if parsed.set {
			report(fmt.Sprintf("%s: %s must not be negative", property, value))
		}
	case "margin":
		s.margin, s.marginLength, s.autoTop, s.autoRight, s.autoBottom, s.autoLeft = parseMarginInsets(value, property, report)
	case "margin-left":
		if keyword == "auto" {
			s.autoLeft, s.marginLength[3], s.margin.Left = true, length{}, 0
			return
		}
		if value := parseLength(value, property, report); value.set {
			s.autoLeft, s.marginLength[3], s.margin.Left = false, value, int(value.pixels)
		}
	case "margin-right":
		if keyword == "auto" {
			s.autoRight, s.marginLength[1], s.margin.Right = true, length{}, 0
			return
		}
		if value := parseLength(value, property, report); value.set {
			s.autoRight, s.marginLength[1], s.margin.Right = false, value, int(value.pixels)
		}
	case "margin-top":
		if keyword == "auto" {
			s.autoTop, s.marginLength[0], s.margin.Top = true, length{}, 0
			return
		}
		if value := parseLength(value, property, report); value.set {
			s.autoTop, s.marginLength[0], s.margin.Top = false, value, int(value.pixels)
		}
	case "margin-bottom":
		if keyword == "auto" {
			s.autoBottom, s.marginLength[2], s.margin.Bottom = true, length{}, 0
			return
		}
		if value := parseLength(value, property, report); value.set {
			s.autoBottom, s.marginLength[2], s.margin.Bottom = false, value, int(value.pixels)
		}
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
		s.parseBorderShorthand(value, report)
	case "border-top", "border-right", "border-bottom", "border-left":
		s.parseBorderSideShorthand(borderSideIndex(property), value, report)
	case "border-top-width", "border-right-width", "border-bottom-width", "border-left-width":
		index := borderSideIndex(property)
		if width, ok := wholeLength(value, property, report); ok {
			s.ensureBorderSide(index).width = width
		}
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		index := borderSideIndex(property)
		line, ok := parseBorderLine(value, property, report)
		if ok {
			side := s.ensureBorderSide(index)
			side.line = line
			if line == borderNone {
				side.dash, side.dashOffset = nil, 0
			}
		}
	case "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		index := borderSideIndex(property)
		if ink, ok := parseInk(value, property, report); ok {
			s.ensureBorderSide(index).ink = ink
		}
	case "border-radius":
		radius, ok := wholeLength(value, property, report)
		if !ok {
			return
		}
		if s.border == nil {
			s.border = &border{}
		}
		s.border.radius = radius
	case "font":
		// The shorthand, which packs a style, a weight, a size, a line height
		// and a family into one line. Two of those five mean something here,
		// and they are the two that decide what the text looks like — so the
		// line is taken apart for them rather than refused whole. Refusing it
		// dropped a size and a family this renderer has, on account of a
		// weight it does not.
		s.applyFontShorthand(value, property, parent, report)
	case "font-family":
		// A stack, as CSS writes one, and the first name this build has wins
		// — which is what a browser does with it. An author writing
		// "Helvetica Neue", Arial, sans-serif has said what they want and
		// what they will settle for, and refusing the whole declaration
		// because the first name is not here would throw both away.
		// A family name is matched without regard to case, as CSS matches
		// one, and reported back the way the author wrote it.
		chosen, ok := "", false
		for _, name := range strings.Split(value, ",") {
			name = strings.Trim(strings.TrimSpace(name), `"'`)
			for _, have := range display.BuiltinFontFamilies() {
				if strings.EqualFold(name, have) {
					chosen, ok = have, true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			report(fmt.Sprintf("font-family: %s names no font this build has; drawn in %s, which it does. It has %s",
				value, display.DefaultFontFamily, strings.Join(display.BuiltinFontFamilies(), ", ")))
			return
		}
		s.fontFamily = chosen
	case "font-size":
		if pixels, ok := wholeLength(value, property, report); ok {
			s.fontSize = pixels
		}
	case "vertical-align":
		// An inline-level box only, as in CSS. Where it applies is checked
		// once the display is settled, because a declaration can change it.
		switch keyword {
		case "baseline":
			s.inlineVAlign = compose.InlineBaseline
		case "top":
			s.inlineVAlign = compose.InlineTop
		case "middle":
			s.inlineVAlign = compose.InlineMiddle
		case "bottom":
			s.inlineVAlign = compose.InlineBottom
		default:
			report(fmt.Sprintf("vertical-align: %s is not supported; use baseline, top, middle or bottom", value))
		}
	case "text-align":
		switch keyword {
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

// applyFontShorthand takes the size and the family out of a font declaration
// and reports what it left behind.
//
// The grammar puts the size before the family and everything optional before
// the size, so the first field that reads as a length is the size and whatever
// follows it is the family. A line height may be attached to the size with a
// slash, which is where the one in "13px/1.4" is.
func (s *style) applyFontShorthand(value, property string, parent style, report func(string)) {
	fields := strings.Fields(value)
	for index, field := range fields {
		size, height, _ := strings.Cut(field, "/")
		parsed := parseLength(size, property, func(string) {})
		if !parsed.fixed() {
			continue
		}
		if index+1 >= len(fields) {
			report(fmt.Sprintf("font: %s names a size and no family; write font-size on its own", value))
			return
		}
		if dropped := strings.Join(fields[:index], " "); dropped != "" {
			report(fmt.Sprintf("font: %s was ignored; this renderer has one weight and one slant", dropped))
		}
		s.apply("font-size", size, parent, report)
		if height != "" {
			s.apply("line-height", height, parent, report)
		} else {
			// A shorthand resets omitted subproperties. Otherwise an earlier
			// line-height declaration on this element leaks through the shorthand.
			s.reset("line-height", report)
		}
		s.apply("font-family", strings.Join(fields[index+1:], " "), parent, report)
		return
	}
	report(fmt.Sprintf("font: %s states no size; a size and a family are the whole of what this reads", value))
}

// setPaint writes whichever of the two paints was named.
func (s *style) setPaint(property string, value *paint) {
	if property == "fill" {
		s.fill = value
		return
	}
	s.stroke = value
}

// parseAngle reads an angle. CSS writes one with a unit and SVG writes one
// without, and both turn up in a stylesheet for a panel.
func parseAngle(value string) (float64, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	for _, unit := range []string{"deg", "grad", "rad", "turn"} {
		body, found := strings.CutSuffix(trimmed, unit)
		if !found {
			continue
		}
		number, err := parseFinite(strings.TrimSpace(body))
		if err != nil {
			return 0, false
		}
		switch unit {
		case "deg":
			return number, true
		case "grad":
			return number * 360 / 400, true
		case "rad":
			return number * 180 / math.Pi, true
		case "turn":
			return number * 360, true
		}
	}
	number, err := parseFinite(trimmed)
	if err != nil {
		return 0, false
	}
	return number, true
}

// originKeywords are the words CSS lets a transform-origin be written with,
// each of them the share of the box it names.
var originKeywords = map[string]struct {
	share float64
	axis  int // 0 for the horizontal, 1 for the vertical, -1 for either
}{
	"left": {0, 0}, "right": {1, 0},
	"top": {0, 1}, "bottom": {1, 1},
	"center": {0.5, -1},
}

// parseTransformOrigin reads one or two values into a pair of lengths.
func parseTransformOrigin(value, property string, report func(string)) (*[2]length, bool) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(fields) == 0 || len(fields) > 2 {
		report(fmt.Sprintf("%s: %s is one or two values", property, value))
		return nil, false
	}
	// A third value is a distance along the axis out of the page, which a
	// panel has none of.
	origin := [2]length{}
	stated := [2]bool{}
	for index, field := range fields {
		axis := index
		if keyword, known := originKeywords[field]; known {
			if keyword.axis >= 0 {
				axis = keyword.axis
			}
			if stated[axis] {
				report(fmt.Sprintf("%s: %s names the same axis twice", property, value))
				return nil, false
			}
			origin[axis] = length{set: true, percent: keyword.share * 100}
			stated[axis] = true
			continue
		}
		size := parseLength(field, property, report)
		if !size.set {
			return nil, false
		}
		origin[axis], stated[axis] = size, true
	}
	// One value states the horizontal and leaves the vertical in the middle,
	// which is what CSS does with it.
	for axis := range origin {
		if !stated[axis] {
			origin[axis] = length{set: true, percent: 50}
		}
	}
	return &origin, true
}

// wholePixels reads a dash length, which SVG writes without a unit and CSS
// writes with one. Both are accepted because both are what an author will
// have in front of them: the property is SVG's and the file is a stylesheet.
func wholePixels(field string) (int, bool) {
	trimmed := strings.TrimSpace(field)
	if hasUnit(trimmed, "px") {
		trimmed = trimmed[:len(trimmed)-2]
	}
	pixels, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return pixels, true
}

// bareNumberInPixels spells a unitless number as pixels, so that the SVG
// geometry properties keep accepting what SVG writes them as. Anything else
// is handed on untouched for the length parser to accept or refuse.
func bareNumberInPixels(value string) string {
	trimmed := strings.TrimSpace(value)
	if _, err := parseFinite(trimmed); err != nil {
		return value
	}
	return trimmed + "px"
}

// edges are what a box puts between its own outside and its content: the
// border, and the padding inside it. CSS counts both, and this used to count
// neither for the border — a border was painted on the box and took no room,
// so the content of a bordered box sat under its own frame and an absolutely
// positioned child was placed against the outside of it.
func (s style) edges() compose.Insets {
	top := s.effectiveBorderWidth(0)
	right := s.effectiveBorderWidth(1)
	bottom := s.effectiveBorderWidth(2)
	left := s.effectiveBorderWidth(3)
	return compose.Insets{
		Top:    top + s.padding.Top,
		Right:  right + s.padding.Right,
		Bottom: bottom + s.padding.Bottom,
		Left:   left + s.padding.Left,
	}
}

func (s style) edgesLengths() [4]compose.Length {
	result := [4]compose.Length{}
	if !s.hasBoxEdges() {
		return result
	}
	for index, padding := range s.paddingLength {
		var value compose.Length
		if padding.set {
			value = compose.Calc(int(padding.percent*10), int(padding.pixels))
		} else {
			value = compose.Pixels(s.paddingEdge(index))
		}
		percent, pixels := value.Parts()
		result[index] = compose.Calc(percent, pixels+s.effectiveBorderWidth(index))
	}
	return result
}

func (s style) hasBoxEdges() bool {
	if s.padding != (compose.Insets{}) || hasBorder(s) {
		return true
	}
	for _, length := range s.paddingLength {
		if length.set {
			return true
		}
	}
	return false
}

func (s style) paddingEdge(index int) int {
	switch index {
	case 0:
		return s.padding.Top
	case 1:
		return s.padding.Right
	case 2:
		return s.padding.Bottom
	default:
		return s.padding.Left
	}
}

// borderEdges are the border alone, which is what an absolutely positioned
// child is placed against: CSS resolves it in the padding box, so the border
// insets it and the padding does not.
func (s style) borderEdges() compose.Insets {
	return compose.Insets{
		Top: s.effectiveBorderWidth(0), Right: s.effectiveBorderWidth(1),
		Bottom: s.effectiveBorderWidth(2), Left: s.effectiveBorderWidth(3),
	}
}

func (s style) effectiveBorderWidth(index int) int {
	side := s.effectiveBorderSide(index)
	if side.line == borderNone {
		return 0
	}
	return side.width
}

// outerSize grows a stated size by the edges around it when the box is sized
// the way CSS sizes one by default, where a width is the width of the content.
func (s style) outerSize(size length, horizontal bool) length {
	if !size.set || s.borderBox {
		return size
	}
	edges := s.edgesLengths()
	if horizontal {
		leftPercent, leftPixels := edges[3].Parts()
		rightPercent, rightPixels := edges[1].Parts()
		size.percent += float64(leftPercent+rightPercent) / 10
		size.pixels += float64(leftPixels + rightPixels)
	} else {
		topPercent, topPixels := edges[0].Parts()
		bottomPercent, bottomPixels := edges[2].Parts()
		size.percent += float64(topPercent+bottomPercent) / 10
		size.pixels += float64(topPixels + bottomPixels)
	}
	return size
}

// borderStyle returns the border being built, creating it with the CSS
// defaults for anything not yet stated.
func (s *style) borderStyle() *border {
	if s.border == nil {
		s.border = &border{width: 1, ink: display.InkBlack}
	}
	return s.border
}

func borderSideIndex(property string) int {
	switch strings.TrimPrefix(property, "border-") {
	case "top", "top-width", "top-style", "top-color":
		return 0
	case "right", "right-width", "right-style", "right-color":
		return 1
	case "bottom", "bottom-width", "bottom-style", "bottom-color":
		return 2
	case "left", "left-width", "left-style", "left-color":
		return 3
	}
	return 0
}

func (s *style) ensureBorderSide(index int) *borderSide {
	if s.borderSide[index] == nil {
		effective := s.effectiveBorderSide(index)
		s.borderSide[index] = &effective
	}
	return s.borderSide[index]
}

func (s style) effectiveBorderSide(index int) borderSide {
	result := borderSide{}
	if s.border != nil {
		result.width, result.ink = s.border.width, s.border.ink
	}
	result.line, result.dash, result.dashOffset = s.line, slices.Clone(s.dash), s.dashOffset
	if side := s.borderSide[index]; side != nil {
		result = *side
		result.dash = slices.Clone(side.dash)
	}
	return result
}

func cloneBorder(value *border) *border {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
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

func isInheritedProperty(property string) bool {
	switch property {
	case "color", "fill", "stroke", "stroke-width", "stroke-dasharray", "stroke-dashoffset",
		"font", "font-family", "font-size", "line-height", "text-align",
		"white-space", "visibility":
		return true
	default:
		return false
	}
}

// inheritOne copies one property from the value the parent passed down.
// inheritOne takes one property's value from the parent, which is what the
// inherit keyword means for any property and not only the ones CSS passes down
// on their own. It reads the parent's whole computed style for that reason:
// display: inherit has to be able to see a display.
//
// Every property the switch in apply implements is here, and anything else
// reports. Left as a short list of the inheriting ones, this did nothing at
// all for the rest — silently, which is the one thing this package must never
// do with a declaration.
func (s *style) inheritOne(property string, parent style, report func(string)) {
	switch property {
	case "display":
		s.display, s.inline, s.inlineAtomic = parent.display, parent.inline, parent.inlineAtomic
	case "flex-direction":
		s.direction = parent.direction
	case "flex-basis":
		s.basis = parent.basis
	case "flex":
		s.grow, s.shrink, s.basis = parent.grow, parent.shrink, parent.basis
	case "flex-grow":
		s.grow = parent.grow
	case "flex-shrink":
		s.shrink = parent.shrink
	case "gap":
		s.rowGap, s.columnGap, s.rowGapLength, s.columnGapLength, s.gapSet = parent.rowGap, parent.columnGap, parent.rowGapLength, parent.columnGapLength, parent.gapSet
	case "row-gap":
		s.rowGap, s.rowGapLength, s.gapSet = parent.rowGap, parent.rowGapLength, parent.gapSet
	case "column-gap":
		s.columnGap, s.columnGapLength, s.gapSet = parent.columnGap, parent.columnGapLength, parent.gapSet
	case "grid-template-columns":
		s.columns = parent.columns
	case "grid-template-rows":
		s.rows = parent.rows
	case "grid-column":
		s.cellColumn = parent.cellColumn
	case "grid-row":
		s.cellRow = parent.cellRow
	case "justify-content":
		s.justify, s.spaceEvenly = parent.justify, parent.spaceEvenly
	case "justify-items":
		s.justify = parent.justify
	case "justify-self":
		s.justifySelf = parent.justifySelf
	case "align-items":
		s.alignItems = parent.alignItems
	case "align-self":
		s.alignSelf = parent.alignSelf
	case "padding":
		s.padding, s.paddingLength = parent.padding, parent.paddingLength
	case "padding-top":
		s.padding.Top, s.paddingLength[0] = parent.padding.Top, parent.paddingLength[0]
	case "padding-right":
		s.padding.Right, s.paddingLength[1] = parent.padding.Right, parent.paddingLength[1]
	case "padding-bottom":
		s.padding.Bottom, s.paddingLength[2] = parent.padding.Bottom, parent.paddingLength[2]
	case "padding-left":
		s.padding.Left, s.paddingLength[3] = parent.padding.Left, parent.paddingLength[3]
	case "margin":
		s.margin, s.marginLength = parent.margin, parent.marginLength
		s.autoTop, s.autoRight, s.autoBottom, s.autoLeft =
			parent.autoTop, parent.autoRight, parent.autoBottom, parent.autoLeft
	case "margin-top":
		s.margin.Top, s.marginLength[0], s.autoTop = parent.margin.Top, parent.marginLength[0], parent.autoTop
	case "margin-right":
		s.margin.Right, s.marginLength[1], s.autoRight = parent.margin.Right, parent.marginLength[1], parent.autoRight
	case "margin-bottom":
		s.margin.Bottom, s.marginLength[2], s.autoBottom = parent.margin.Bottom, parent.marginLength[2], parent.autoBottom
	case "margin-left":
		s.margin.Left, s.marginLength[3], s.autoLeft = parent.margin.Left, parent.marginLength[3], parent.autoLeft
	case "width":
		s.width = parent.width
	case "height":
		s.height = parent.height
	case "min-width":
		s.minSize[0] = parent.minSize[0]
	case "max-width":
		s.maxSize[0] = parent.maxSize[0]
	case "min-height":
		s.minSize[1] = parent.minSize[1]
	case "max-height":
		s.maxSize[1] = parent.maxSize[1]
	case "aspect-ratio":
		s.ratio = parent.ratio
	case "box-sizing":
		s.borderBox = parent.borderBox
	case "position":
		s.absolute, s.positioned = parent.absolute, parent.positioned
	case "top":
		s.inset[0], s.insetFromShorthand[0] = parent.inset[0], parent.insetFromShorthand[0]
	case "right":
		s.inset[1], s.insetFromShorthand[1] = parent.inset[1], parent.insetFromShorthand[1]
	case "bottom":
		s.inset[2], s.insetFromShorthand[2] = parent.inset[2], parent.insetFromShorthand[2]
	case "left":
		s.inset[3], s.insetFromShorthand[3] = parent.inset[3], parent.insetFromShorthand[3]
	case "inset":
		s.inset, s.insetFromShorthand = parent.inset, parent.insetFromShorthand
	case "z-index":
		s.layer = parent.layer
	case "background", "background-color":
		s.background = parent.background
	case "color":
		s.color = parent.color
	case "border":
		s.border = cloneBorder(parent.border)
		for index, side := range parent.borderSide {
			if side == nil {
				continue
			}
			copy := *side
			copy.dash = slices.Clone(side.dash)
			s.borderSide[index] = &copy
		}
		s.line, s.dash, s.dashOffset = parent.line, slices.Clone(parent.dash), parent.dashOffset
	case "border-width":
		if parent.border != nil {
			s.borderStyle().width = parent.border.width
		} else if s.border != nil {
			s.border.width = 1
		}
	case "border-style":
		s.line, s.dash, s.dashOffset = parent.line, slices.Clone(parent.dash), parent.dashOffset
		if parent.border == nil && s.border != nil {
			s.border.width = 0
		}
	case "border-color":
		if s.border != nil {
			if parent.border != nil {
				s.border.ink = parent.border.ink
			} else {
				s.border.ink = display.InkBlack
			}
		}
	case "border-radius":
		if s.border != nil {
			if parent.border != nil {
				s.border.radius = parent.border.radius
			} else {
				s.border.radius = 0
			}
		}
	case "border-top", "border-right", "border-bottom", "border-left":
		index := borderSideIndex(property)
		if side := parent.borderSide[index]; side != nil {
			copy := *side
			copy.dash = slices.Clone(side.dash)
			s.borderSide[index] = &copy
		} else {
			effective := parent.effectiveBorderSide(index)
			s.borderSide[index] = &effective
		}
	case "border-top-width", "border-right-width", "border-bottom-width", "border-left-width":
		index := borderSideIndex(property)
		effective := parent.effectiveBorderSide(index)
		s.borderSide[index] = &effective
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		index := borderSideIndex(property)
		effective := parent.effectiveBorderSide(index)
		s.borderSide[index] = &effective
	case "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		index := borderSideIndex(property)
		effective := parent.effectiveBorderSide(index)
		s.borderSide[index] = &effective
	case "visibility":
		s.hidden = parent.hidden
	case "overflow":
		s.clip = parent.clip
	case "clip-path":
		s.clipShape = parent.clipShape
	case "transform":
		s.transform, s.rotate, s.rotateOrigin = parent.transform, parent.rotate, parent.rotateOrigin
	case "rotate":
		s.rotate = parent.rotate
	case "transform-origin":
		s.rotateOrigin = parent.rotateOrigin
	case "scale":
		s.transform = parent.transform
	case "fill":
		s.fill = parent.fill
	case "stroke":
		s.stroke = parent.stroke
	case "stroke-width":
		s.strokeWidth = parent.strokeWidth
	case "stroke-dasharray":
		s.line, s.dash = parent.line, slices.Clone(parent.dash)
	case "stroke-dashoffset":
		s.dashOffset = parent.dashOffset
	case "font":
		s.fontFamily, s.fontSize = parent.fontFamily, parent.fontSize
		s.lineHeight, s.lineHeightMultiple = parent.lineHeight, parent.lineHeightMultiple
	case "font-family":
		s.fontFamily = parent.fontFamily
	case "font-size":
		s.fontSize = parent.fontSize
	case "line-height":
		s.lineHeight, s.lineHeightMultiple = parent.lineHeight, parent.lineHeightMultiple
	case "text-align":
		s.textAlign = parent.textAlign
	case "vertical-align":
		s.inlineVAlign = parent.inlineVAlign
	case "white-space":
		s.wrap, s.preserve = parent.wrap, parent.preserve
	case "object-fit":
		s.objectFit = parent.objectFit
	default:
		report(fmt.Sprintf("%s: inherit has no meaning for a property this renderer does not implement", property))
	}
}

// reset returns one property to the value it would have had with no
// declaration at all.
// reset returns one property to its initial value, which is what initial and
// revert ask for here, and what unset asks for on a non-inherited property.
// Most of them are the zero value of a fresh style; the handful that are not
// are the ones with a default the panel chose rather than the struct.
//
// Every property apply implements is here, for the same reason as inheritOne:
// the ones that were missing did nothing and said nothing.
func (s *style) reset(property string, report func(string)) {
	fresh := style{}
	switch property {
	case "display":
		// CSS's initial value, which is inline rather than the element's own
		// default: revert is the one that would mean the element's default,
		// and it is treated as initial here.
		s.display, s.inline, s.inlineAtomic = displayBlock, true, false
	case "flex-direction":
		s.direction = fresh.direction
	case "flex-basis":
		s.basis = fresh.basis
	case "flex":
		s.grow, s.shrink, s.basis = fresh.grow, 1, fresh.basis
	case "flex-grow":
		s.grow = fresh.grow
	case "flex-shrink":
		s.shrink = 1
	case "gap":
		s.rowGap, s.columnGap, s.rowGapLength, s.columnGapLength, s.gapSet = 0, 0, length{}, length{}, false
	case "row-gap":
		s.rowGap, s.rowGapLength, s.gapSet = 0, length{}, false
	case "column-gap":
		s.columnGap, s.columnGapLength, s.gapSet = 0, length{}, false
	case "grid-template-columns":
		s.columns = nil
	case "grid-template-rows":
		s.rows = nil
	case "grid-column":
		s.cellColumn = fresh.cellColumn
	case "grid-row":
		s.cellRow = fresh.cellRow
	case "justify-content":
		s.justify, s.spaceEvenly = fresh.justify, false
	case "justify-items":
		s.justify = fresh.justify
	case "justify-self":
		s.justifySelf = nil
	case "align-items":
		s.alignItems = fresh.alignItems
	case "align-self":
		s.alignSelf = nil
	case "padding":
		s.padding, s.paddingLength = fresh.padding, [4]length{}
	case "padding-top":
		s.padding.Top, s.paddingLength[0] = 0, length{}
	case "padding-right":
		s.padding.Right, s.paddingLength[1] = 0, length{}
	case "padding-bottom":
		s.padding.Bottom, s.paddingLength[2] = 0, length{}
	case "padding-left":
		s.padding.Left, s.paddingLength[3] = 0, length{}
	case "margin":
		s.margin, s.marginLength = fresh.margin, [4]length{}
		s.autoTop, s.autoRight, s.autoBottom, s.autoLeft = false, false, false, false
	case "margin-top":
		s.margin.Top, s.marginLength[0], s.autoTop = 0, length{}, false
	case "margin-right":
		s.margin.Right, s.marginLength[1], s.autoRight = 0, length{}, false
	case "margin-bottom":
		s.margin.Bottom, s.marginLength[2], s.autoBottom = 0, length{}, false
	case "margin-left":
		s.margin.Left, s.marginLength[3], s.autoLeft = 0, length{}, false
	case "width":
		s.width = fresh.width
	case "height":
		s.height = fresh.height
	case "min-width":
		s.minSize[0] = fresh.minSize[0]
	case "max-width":
		s.maxSize[0] = fresh.maxSize[0]
	case "min-height":
		s.minSize[1] = fresh.minSize[1]
	case "max-height":
		s.maxSize[1] = fresh.maxSize[1]
	case "aspect-ratio":
		s.ratio = 0
	case "box-sizing":
		s.borderBox = false
	case "position":
		s.absolute, s.positioned = false, false
	case "top":
		s.inset[0], s.insetFromShorthand[0] = fresh.inset[0], false
	case "right":
		s.inset[1], s.insetFromShorthand[1] = fresh.inset[1], false
	case "bottom":
		s.inset[2], s.insetFromShorthand[2] = fresh.inset[2], false
	case "left":
		s.inset[3], s.insetFromShorthand[3] = fresh.inset[3], false
	case "inset":
		s.inset, s.insetFromShorthand = fresh.inset, [4]bool{}
	case "z-index":
		s.layer = 0
	case "background", "background-color":
		s.background = nil
	case "color":
		s.color = display.InkBlack
	case "border":
		s.border, s.borderSide, s.line, s.dash, s.dashOffset = nil, [4]*borderSide{}, borderNone, nil, 0
	case "border-width":
		if s.border != nil {
			// The implementation's medium border is one pixel. Keeping the
			// border object preserves color and radius set by other longhands.
			s.border.width = 1
			for index := range s.borderSide {
				if s.borderSide[index] != nil {
					s.borderSide[index].width = 1
				}
			}
		}
	case "border-style":
		s.line, s.dash, s.dashOffset = borderNone, nil, 0
		if s.border != nil {
			// There is no separate style field in the scene schema; zero width
			// is the representation of CSS's initial border-style: none.
			s.border.width = 0
			for index := range s.borderSide {
				if s.borderSide[index] != nil {
					s.borderSide[index].line = borderNone
				}
			}
		}
	case "border-color":
		if s.border != nil {
			s.border.ink = display.InkBlack
			for index := range s.borderSide {
				if s.borderSide[index] != nil {
					s.borderSide[index].ink = display.InkBlack
				}
			}
		}
	case "border-radius":
		if s.border != nil {
			s.border.radius = 0
		}
	case "border-top", "border-right", "border-bottom", "border-left":
		s.borderSide[borderSideIndex(property)] = nil
	case "border-top-width", "border-right-width", "border-bottom-width", "border-left-width":
		s.ensureBorderSide(borderSideIndex(property)).width = 1
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		s.ensureBorderSide(borderSideIndex(property)).line = borderNone
	case "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		s.ensureBorderSide(borderSideIndex(property)).ink = display.InkBlack
	case "visibility":
		s.hidden = false
	case "overflow":
		s.clip = false
	case "clip-path":
		s.clipShape = compose.Shape{}
	case "transform":
		s.transform, s.rotate, s.rotateOrigin = display.Transform{}, 0, nil
	case "rotate":
		s.rotate = 0
	case "transform-origin":
		s.rotateOrigin = nil
	case "scale":
		s.transform = display.Transform{}
	case "fill":
		s.fill = nil
	case "stroke":
		s.stroke = nil
	case "stroke-width":
		s.strokeWidth = nil
	case "stroke-dasharray":
		s.line, s.dash = borderNone, nil
	case "stroke-dashoffset":
		s.dashOffset = 0
	case "font":
		s.fontFamily, s.fontSize = display.DefaultFontFamily, display.DefaultFontSize
		s.lineHeight, s.lineHeightMultiple, s.lineHeightResolvesHere = 0, 0, false
	case "font-family":
		s.fontFamily = display.DefaultFontFamily
	case "font-size":
		s.fontSize = display.DefaultFontSize
	case "line-height":
		s.lineHeight, s.lineHeightMultiple, s.lineHeightResolvesHere = 0, 0, false
	case "text-align":
		s.textAlign = display.AlignStart
	case "vertical-align":
		s.inlineVAlign = compose.InlineBaseline
	case "white-space":
		// CSS wraps by default, and WrapRunes is not the zero value, so this
		// is one of the few that cannot be taken from a fresh style.
		s.wrap, s.preserve = display.WrapRunes, false
	case "object-fit":
		s.objectFit = nil
	default:
		report(fmt.Sprintf("%s: initial has no meaning for a property this renderer does not implement", property))
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
			count, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(field, "fr")))
			if err != nil || count <= 0 {
				report(fmt.Sprintf("%s: %s is not a positive number of fr", property, field))
				return nil
			}
			tracks = append(tracks, compose.Track{Fraction: count})
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
// maxTracks is as many grid tracks as the widest panel this drives has pixels.
// Past that a track cannot be a pixel wide, so the number is a mistake rather
// than a layout, and expanding it is how a stylesheet stops a program.
const maxTracks = 400

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
		// A count is a number somebody typed and this walks it. The largest
		// panel here is four hundred pixels across, so a grid of more tracks
		// than that has a track narrower than a pixel; a million of them is
		// a typo, and a billion is the request that never came back.
		if count > maxTracks {
			report(fmt.Sprintf("%s: repeat(%d, …) asks for more tracks than a panel has pixels; the most is %d",
				property, count, maxTracks))
			return nil
		}
		for i := 0; i < count; i++ {
			fields = append(fields, strings.Fields(pattern)...)
		}
		rest = strings.TrimSpace(remainder)
	}
	return fields
}

// parseGridLine reads "3", "span 2", "2 / 4" or "2 / span 2" into a line and a
// span. The last two say the same thing and an author uses whichever they were
// thinking in — where the cell ends, or how many tracks it covers.
func parseGridLine(value, property string, report func(string)) [2]int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "auto" {
		return [2]int{}
	}
	if start, end, ok := strings.Cut(trimmed, "/"); ok {
		from, errFrom := strconv.Atoi(strings.TrimSpace(start))
		if counted, spans := cutKeyword(strings.TrimSpace(end), "span"); spans {
			count, errCount := strconv.Atoi(strings.TrimSpace(counted))
			if errFrom != nil || errCount != nil || from < 1 || count < 1 {
				report(fmt.Sprintf("%s: %s needs a line number and a span of at least one", property, value))
				return [2]int{}
			}
			return [2]int{from, count}
		}
		to, errTo := strconv.Atoi(strings.TrimSpace(end))
		if errFrom != nil || errTo != nil || from < 1 || to <= from {
			report(fmt.Sprintf("%s: %s must be two increasing line numbers", property, value))
			return [2]int{}
		}
		return [2]int{from, to - from}
	}
	if rest, ok := cutKeyword(trimmed, "span"); ok {
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
	name = strings.ToLower(strings.TrimSpace(name))
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
	switch strings.ToLower(strings.TrimSpace(value)) {
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
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	switch {
	case lower == "auto":
		return length{}
	case strings.HasPrefix(lower, "calc("):
		return parseCalc(value, property, report)
	case strings.HasSuffix(value, "%"):
		number, err := parseFinite(strings.TrimSuffix(value, "%"))
		if err != nil {
			report(fmt.Sprintf("%s: %s is not a percentage", property, value))
			return length{}
		}
		return length{set: true, percent: number}
	case hasUnit(value, "px"):
		number, err := parseFinite(value[:len(value)-2])
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

func negativeLength(value length) bool {
	return value.set && (value.percent < 0 || (value.percent == 0 && value.pixels < 0))
}

// parseCalc reads the sums of lengths that calc() is used for here. Only
// addition and subtraction of percentages and pixels are accepted: the rest of
// the grammar multiplies and divides by unitless numbers, which nothing in
// this vocabulary needs and which would invite expressions no panel can
// answer, such as a length divided by a length.
func parseCalc(value, property string, report func(string)) length {
	inner := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "calc("), ")")
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

// parseFinite reads a number the way CSS spells one, which is to say without
// NaN or an infinity. strconv accepts all three by name, and none of them is a
// length, an angle or a count. NaN reached json.Marshal, which refuses it, so
// one unreadable declaration stopped the whole page compiling — the opposite
// of what this package promises. A figure too large to hold comes back as an
// infinity with an error and is refused here for the same reason.
func parseFinite(value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("%s is not a finite number", value)
	}
	return number, nil
}

func parseNumber(value, property string, report func(string)) int {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		report(fmt.Sprintf("%s: %q is not a number", property, value))
		return 0
	}
	number, err := parseFinite(fields[0])
	if err != nil {
		report(fmt.Sprintf("%s: %s is not a number", property, value))
		return 0
	}
	return int(number)
}

func parseFlexFactor(value, property string, report func(string)) float64 {
	fields := strings.Fields(value)
	if len(fields) != 1 {
		report(fmt.Sprintf("%s: %s is not a single non-negative number", property, value))
		return 0
	}
	number, err := parseFinite(fields[0])
	if err != nil || number < 0 {
		report(fmt.Sprintf("%s: %s is not a single non-negative number", property, value))
		return 0
	}
	return number
}

// parseFlexShorthand keeps all three CSS flex components. In particular,
// flex: 1 expands to 1 1 0%, while flex: 0 0 40px retains its fixed basis
// and zero shrink factor instead of silently reducing to flex-grow alone.
func parseFlexShorthand(value, property string, report func(string)) (float64, float64, length, bool) {
	fields := splitSides(value)
	if len(fields) == 1 {
		switch strings.ToLower(fields[0]) {
		case "none":
			return 0, 0, length{}, true
		case "auto":
			return 1, 1, length{}, true
		}
		grow, ok := flexFactor(fields[0])
		if !ok {
			report(fmt.Sprintf("%s: %s is not a valid flex shorthand", property, value))
			return 0, 0, length{}, false
		}
		return grow, 1, length{set: true}, true // 0%: resolve from the flex container
	}
	if len(fields) < 2 || len(fields) > 3 {
		report(fmt.Sprintf("%s: %s needs one to three components", property, value))
		return 0, 0, length{}, false
	}
	grow, ok := flexFactor(fields[0])
	if !ok {
		report(fmt.Sprintf("%s: %s has an invalid grow factor", property, value))
		return 0, 0, length{}, false
	}
	if shrink, numeric := flexFactor(fields[1]); numeric {
		if len(fields) == 2 {
			return grow, shrink, length{set: true}, true
		}
		basis := parseLength(fields[2], property, report)
		if !basis.set && !strings.EqualFold(fields[2], "auto") {
			return 0, 0, length{}, false
		}
		return grow, shrink, basis, true
	}
	if len(fields) != 2 {
		report(fmt.Sprintf("%s: %s has an invalid shrink factor", property, value))
		return 0, 0, length{}, false
	}
	basis := parseLength(fields[1], property, report)
	if !basis.set && !strings.EqualFold(fields[1], "auto") {
		return 0, 0, length{}, false
	}
	return grow, 1, basis, true
}

func flexFactor(value string) (float64, bool) {
	number, err := parseFinite(value)
	return number, err == nil && number >= 0
}

// wholeLength reads a length for one of the fields that is counted in whole
// pixels and cannot hold anything else: a gap, an inset, a border, a strike.
//
// A percentage there is not a smaller number, it is a number that cannot be
// worked out until the layout runs, and the layout takes an int. So it is
// refused by name. It used to be taken as its pixel half, which made
// "padding: 50%" mean padding: 0 with nothing said about it.
func wholeLength(value, property string, report func(string)) (int, bool) {
	size := parseLength(value, property, report)
	if !size.set {
		return 0, false
	}
	if size.percent != 0 {
		report(fmt.Sprintf(
			"%s: %s cannot be a share of the box here; %s is counted in whole pixels",
			property, value, property))
		return 0, false
	}
	if size.pixels < 0 {
		report(fmt.Sprintf("%s: %s must not be negative", property, value))
		return 0, false
	}
	return int(size.pixels), true
}

// splitSides splits a shorthand into its one to four values. It splits on the
// spaces between them and not on the spaces inside them, because calc() is
// written with spaces around its operator and "padding: calc(10px + 2px)" came
// apart into three sides, one of which was a plus sign.
func splitSides(value string) []string {
	var sides []string
	depth, start := 0, -1
	for index, letter := range value {
		switch {
		case letter == '(':
			depth++
		case letter == ')':
			depth--
		}
		space := depth == 0 && (letter == ' ' || letter == '\t' || letter == '\n')
		if space {
			if start >= 0 {
				sides = append(sides, value[start:index])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = index
		}
	}
	if start >= 0 {
		sides = append(sides, value[start:])
	}
	return sides
}

func parseStyleInsets(value, property string, report func(string)) (compose.Insets, [4]length) {
	fields := splitSides(value)
	sides := make([]length, 0, 4)
	for _, field := range fields {
		parsed := parseLength(field, property, report)
		if !parsed.set {
			report(fmt.Sprintf("%s: %s must be a non-negative length", property, field))
			return compose.Insets{}, [4]length{}
		}
		if property == "padding" && (parsed.percent < 0 || parsed.pixels < 0) {
			report(fmt.Sprintf("%s: %s must be a non-negative length", property, field))
			return compose.Insets{}, [4]length{}
		}
		sides = append(sides, parsed)
	}
	if len(sides) == 0 || len(sides) > 4 {
		report(fmt.Sprintf("%s: %q needs one to four lengths", property, value))
		return compose.Insets{}, [4]length{}
	}
	result := [4]length{sides[0], sides[0], sides[0], sides[0]}
	if len(sides) > 1 {
		result[1], result[3] = sides[1], sides[1]
	}
	if len(sides) > 2 {
		result[2] = sides[2]
	}
	if len(sides) > 3 {
		result[3] = sides[3]
	}
	return compose.Insets{Top: int(result[0].pixels), Right: int(result[1].pixels), Bottom: int(result[2].pixels), Left: int(result[3].pixels)}, result
}

// parseMarginInsets is the margin counterpart to parseStyleInsets. Unlike
// padding, CSS margins accept auto, which is represented by a separate flag so
// flex flow can distribute the free space instead of treating it as zero.
func parseMarginInsets(value, property string, report func(string)) (compose.Insets, [4]length, bool, bool, bool, bool) {
	fields := splitSides(value)
	if len(fields) == 0 || len(fields) > 4 {
		report(fmt.Sprintf("%s: %q needs one to four lengths", property, value))
		return compose.Insets{}, [4]length{}, false, false, false, false
	}
	sides := make([]length, len(fields))
	auto := make([]bool, len(fields))
	for index, field := range fields {
		if strings.EqualFold(field, "auto") {
			auto[index] = true
			continue
		}
		parsed := parseLength(field, property, report)
		if !parsed.set {
			report(fmt.Sprintf("%s: %s must be a length or auto", property, field))
			return compose.Insets{}, [4]length{}, false, false, false, false
		}
		sides[index] = parsed
	}
	values := [4]length{sides[0], sides[0], sides[0], sides[0]}
	autos := [4]bool{auto[0], auto[0], auto[0], auto[0]}
	if len(sides) > 1 {
		values[1], values[3] = sides[1], sides[1]
		autos[1], autos[3] = auto[1], auto[1]
	}
	if len(sides) > 2 {
		values[2], autos[2] = sides[2], auto[2]
	}
	if len(sides) > 3 {
		values[3], autos[3] = sides[3], auto[3]
	}
	return compose.Insets{Top: int(values[0].pixels), Right: int(values[1].pixels), Bottom: int(values[2].pixels), Left: int(values[3].pixels)}, values,
		autos[0], autos[1], autos[2], autos[3]
}

func parseInsetLengths(value, property string, report func(string)) [4]length {
	fields := splitSides(value)
	sides := make([]length, 0, 4)
	for _, field := range fields {
		sides = append(sides, parseLength(field, property, report))
		if !sides[len(sides)-1].set && field != "auto" {
			return [4]length{}
		}
	}
	if len(sides) == 0 || len(sides) > 4 {
		report(fmt.Sprintf("%s: %q needs one to four lengths", property, value))
		return [4]length{}
	}
	result := [4]length{sides[0], sides[0], sides[0], sides[0]}
	if len(sides) > 1 {
		result[1], result[3] = sides[1], sides[1]
	}
	if len(sides) > 2 {
		result[2] = sides[2]
	}
	if len(sides) > 3 {
		result[3] = sides[3]
	}
	return result
}

func parseInk(value, property string, report func(string)) (display.Ink, bool) {
	switch strings.ToLower(value) {
	case "black", "#000", "#000000":
		return display.InkBlack, true
	case "white", "#fff", "#ffffff":
		return display.InkWhite, true
	case "red", "#f00", "#ff0000":
		return display.InkRed, true
	case "yellow", "#ff0", "#ffff00":
		// The fourth ink. Three of the panels this drives are BWRY parts, and
		// leaving yellow out of the stylesheet meant the only way to reach a
		// colour the hardware has was to write the scene document by hand.
		return display.InkYellow, true
	case "transparent", "none":
		return 0, false
	}
	// Anything else would have to be approximated, and approximating a colour
	// on a panel with four of them is how a design silently stops matching.
	report(fmt.Sprintf(
		"%s: %s is not one of the panel's inks (black, white, red, yellow)", property, value))
	return 0, false
}

// parseBorderShorthand reads "1px dashed black" in any order, which is how the
// shorthand is written.
//
// Every field is matched by what it is rather than by elimination: a width is
// a length, a style is one of the style keywords, and only what is left over
// is offered to the colour parser. Falling through to colour sent "dashed" to
// it, which named it as an ink the panel has not got and pointed the author at
// a colour they had not written wrong.
//
// The style a shorthand leaves out is none, as in CSS, so "border: 1px" draws
// nothing. Keywords and units are matched without regard to case.
func (s *style) parseBorderShorthand(value string, report func(string)) {
	result, line, dash, dashOffset, ok := parseBorderValue(value, "border", s.border, report)
	if !ok {
		return
	}
	s.borderSide = [4]*borderSide{}
	s.border, s.line, s.dash, s.dashOffset = result, line, dash, dashOffset
}

func (s *style) parseBorderSideShorthand(index int, value string, report func(string)) {
	base := s.border
	result, line, dash, dashOffset, ok := parseBorderValue(value, "border-"+[]string{"top", "right", "bottom", "left"}[index], base, report)
	if !ok {
		return
	}
	s.borderSide[index] = &borderSide{width: result.width, ink: result.ink, line: line, dash: dash, dashOffset: dashOffset}
}

func parseBorderValue(value, property string, previous *border, report func(string)) (*border, borderLine, []int, int, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil, borderNone, nil, 0, false
	}
	result := &border{ink: display.InkBlack, width: 1}
	if previous != nil {
		result.radius = previous.radius
	}
	line := borderNone
	for _, field := range fields {
		lower := strings.ToLower(field)
		switch {
		case isBorderStyleKeyword(lower):
			// Reported here rather than silently: a style this cannot draw is
			// left as none, so the border is not drawn as some other style.
			line, _ = parseBorderLine(lower, "border", report)
		case isLength(lower):
			width, ok := wholeLength(lower, property, report)
			if !ok {
				return nil, borderNone, nil, 0, false
			}
			result.width = width
		default:
			if ink, ok := parseInk(field, property, report); ok {
				result.ink = ink
			}
		}
	}
	var dash []int
	if line != borderDashed {
		dash = nil
	}
	return result, line, dash, 0, true
}

// isBorderStyleKeyword names every CSS border style, drawable here or not, so
// that each is read as a style rather than handed on to the colour parser and
// reported as a colour nobody wrote.
func isBorderStyleKeyword(lower string) bool {
	switch lower {
	case "solid", "dashed", "dotted", "none", "hidden",
		"double", "groove", "ridge", "inset", "outset":
		return true
	}
	return false
}

// isLength reports a border width: a number, with or without the unit.
func isLength(lower string) bool {
	_, err := parseFinite(strings.TrimSuffix(lower, "px"))
	return err == nil
}

// cutKeyword strips a leading keyword and the space after it, matching the
// keyword the way CSS matches one: without regard to case.
func cutKeyword(value, keyword string) (string, bool) {
	if len(value) <= len(keyword) || !strings.EqualFold(value[:len(keyword)], keyword) {
		return value, false
	}
	rest := value[len(keyword):]
	if strings.TrimLeft(rest, " \t") == rest {
		return value, false
	}
	return strings.TrimLeft(rest, " \t"), true
}

// hasUnit reports a length written with the given unit, whatever case it is
// written in: CSS matches a unit the way it matches a keyword.
func hasUnit(value, unit string) bool {
	return len(value) > len(unit) && strings.EqualFold(value[len(value)-len(unit):], unit)
}

// isNumber reports a bare number, which is what a width written without px is.
func isNumber(field string) bool {
	_, err := parseFinite(field)
	return err == nil
}
