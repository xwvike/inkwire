package markup

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/display"
	"golang.org/x/net/html"
)

// listTypes are the list-style-type keywords a marker is drawn for. disc is
// not here because it is the initial value, which is held as no type at all.
var listTypes = map[string]bool{
	"none": true, "circle": true, "square": true, "disclosure-open": true, "disclosure-closed": true,
	"decimal": true, "decimal-leading-zero": true,
	"lower-alpha": true, "lower-latin": true, "upper-alpha": true, "upper-latin": true,
	"lower-roman": true, "upper-roman": true, "lower-greek": true,
	"cjk-decimal": true, "simp-chinese-informal": true, "trad-chinese-informal": true, "cjk-ideographic": true,
}

// listStyleType reads one list-style-type: a keyword, or a string that is the
// marker itself. A name this has no counter style for is drawn as decimal,
// which is what a browser does with a name nothing defined.
func listStyleType(value string, report func(string)) (string, string) {
	value = strings.TrimSpace(value)
	if text, ok := quotedString(value); ok {
		return "string", text
	}
	keyword := strings.ToLower(value)
	switch {
	case keyword == "disc":
		return "", ""
	case listTypes[keyword]:
		return keyword, ""
	}
	report(fmt.Sprintf("list-style-type: %s is not a counter style this has; drawn as decimal, as a browser draws a name it does not know", value))
	return "decimal", ""
}

// applyListStyle reads the shorthand, whose parts may come in any order. none
// is the type when no other part names one, and the image when one does.
func (s *style) applyListStyle(value string, report func(string)) {
	listType, listText, inside := "", "", false
	typed, nones := false, 0
	for _, field := range listStyleFields(value) {
		keyword := strings.ToLower(field)
		switch {
		case keyword == "none":
			nones++
		case keyword == "inside" || keyword == "outside":
			inside = keyword == "inside"
		case strings.HasPrefix(keyword, "url(") || strings.Contains(keyword, "gradient("):
			report(fmt.Sprintf("list-style: %s is not drawn; the marker list-style-type names is drawn instead", field))
		default:
			listType, listText = listStyleType(field, report)
			typed = true
		}
	}
	if nones > 0 && !typed {
		listType = "none"
	}
	s.listType, s.listText, s.listInside = listType, listText, inside
}

// listStyleFields splits the shorthand at the spaces between its parts, and
// not at the ones inside a string or a url().
func listStyleFields(value string) []string {
	var fields []string
	var current strings.Builder
	depth, quote := 0, rune(0)
	for _, letter := range value {
		switch {
		case quote != 0:
			if letter == quote {
				quote = 0
			}
		case letter == '"' || letter == '\'':
			quote = letter
		case letter == '(':
			depth++
		case letter == ')':
			depth--
		case depth == 0 && (letter == ' ' || letter == '\t' || letter == '\n'):
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(letter)
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

// quotedString is the text of a CSS string, with its escaped quotes taken as
// the quotes they stand for.
func quotedString(value string) (string, bool) {
	if len(value) < 2 || (value[0] != '"' && value[0] != '\'') || value[len(value)-1] != value[0] {
		return "", false
	}
	inner := value[1 : len(value)-1]
	return strings.ReplaceAll(inner, `\`+string(value[0]), string(value[0])), true
}

// markerText is what a counted marker reads for an item's number, with what
// CSS puts after it: a full stop and a space for the alphabets and numbers, and
// the enumeration comma for Chinese ones. A number a style has no way to write
// — a Roman numeral for zero — is written in decimal, as CSS writes it.
func markerText(listType string, ordinal int) string {
	decimal := strconv.Itoa(ordinal) + ". "
	switch listType {
	case "decimal-leading-zero":
		if ordinal >= 0 && ordinal < 10 {
			return "0" + decimal
		}
		if ordinal < 0 && ordinal > -10 {
			return "-0" + strconv.Itoa(-ordinal) + ". "
		}
	case "lower-alpha", "lower-latin":
		if text, ok := alphabetic(ordinal, []rune("abcdefghijklmnopqrstuvwxyz")); ok {
			return text + ". "
		}
	case "upper-alpha", "upper-latin":
		if text, ok := alphabetic(ordinal, []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")); ok {
			return text + ". "
		}
	case "lower-greek":
		if text, ok := alphabetic(ordinal, []rune("αβγδεζηθικλμνξοπρστυφχψω")); ok {
			return text + ". "
		}
	case "lower-roman":
		if text, ok := roman(ordinal); ok {
			return strings.ToLower(text) + ". "
		}
	case "upper-roman":
		if text, ok := roman(ordinal); ok {
			return text + ". "
		}
	case "cjk-decimal":
		if ordinal >= 0 {
			digits := []rune("〇一二三四五六七八九")
			var text strings.Builder
			for _, digit := range strconv.Itoa(ordinal) {
				text.WriteRune(digits[digit-'0'])
			}
			return text.String() + "、"
		}
	case "simp-chinese-informal", "trad-chinese-informal", "cjk-ideographic":
		if text, ok := chineseInformal(ordinal); ok {
			return text + "、"
		}
	}
	return decimal
}

// alphabetic counts as a spreadsheet's columns do: a to z, then aa.
func alphabetic(ordinal int, letters []rune) (string, bool) {
	if ordinal < 1 {
		return "", false
	}
	var reversed []rune
	for ordinal > 0 {
		ordinal--
		reversed = append(reversed, letters[ordinal%len(letters)])
		ordinal /= len(letters)
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return string(reversed), true
}

func roman(ordinal int) (string, bool) {
	if ordinal < 1 || ordinal > 3999 {
		return "", false
	}
	var text strings.Builder
	for _, step := range []struct {
		value  int
		symbol string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"},
		{50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
		for ordinal >= step.value {
			text.WriteString(step.symbol)
			ordinal -= step.value
		}
	}
	return text.String(), true
}

// chineseInformal writes a number as it is read: 十一, 一百零五. Below ten
// thousand the simplified and traditional forms are the same characters.
func chineseInformal(ordinal int) (string, bool) {
	if ordinal < 0 || ordinal > 9999 {
		return "", false
	}
	digits := []rune("零一二三四五六七八九")
	if ordinal == 0 {
		return string(digits[0]), true
	}
	var text []rune
	zero := false
	for _, place := range []struct {
		value int
		unit  string
	}{{1000, "千"}, {100, "百"}, {10, "十"}, {1, ""}} {
		digit := ordinal / place.value % 10
		if digit == 0 {
			zero = len(text) > 0
			continue
		}
		if zero {
			text = append(text, digits[0])
			zero = false
		}
		// Ten to nineteen are read without the one: 十一, not 一十一.
		if !(place.value == 10 && digit == 1 && len(text) == 0) {
			text = append(text, digits[digit])
		}
		text = append(text, []rune(place.unit)...)
	}
	return string(text), true
}

// listMarker is the marker one list item is given: a shape for the bullets,
// or text for everything that counts or was written as a string.
type listMarker struct {
	shape *emitted
	text  string
	// gap is the room between the marker and the item's text, which for text
	// is the space it ends with, and written is that text as it was before
	// the space was taken off.
	gap     int
	written string
	// size and baseline place a shape against the item's first line.
	size, baseline, middle int
}

// listMarkerFor is the marker an item shows, or nil when it shows none.
func (c *compiler) listMarkerFor(node *html.Node, current style, path string) *listMarker {
	if !current.listItem || current.listType == "none" || current.display == displayNone {
		return nil
	}
	strike := c.strike(current.fontFamily, current.fontSize, path)
	metrics, space, ok := display.BuiltinFontLine(current.fontFamily, strike)
	if !ok {
		metrics, space = display.FontMetrics{Ascent: strike * 5 / 6, Descent: strike / 6}, strike/2
	}
	line := current.lineHeight
	if line <= 0 {
		line = metrics.Height()
	}
	baseline := max(0, line-metrics.Ascent-metrics.Descent)/2 + metrics.Ascent
	marker := &listMarker{gap: space, baseline: baseline}
	ink := inkName(current.color)
	switch current.listType {
	case "", "circle", "square", "disclosure-open", "disclosure-closed":
		// A bullet a third of the font's height, centred where a line
		// through the text would be drawn: the middle of its lower case.
		marker.size = max(3, (current.fontSize*2+3)/6)
		marker.middle = baseline - (metrics.Ascent*3+5)/10
		// The shape takes the size of whatever box it is put in, which is
		// the anchor's beside the item and its own on the line.
		switch current.listType {
		case "":
			marker.shape = &emitted{Type: "ellipse", Fill: ink}
		case "circle":
			marker.shape = &emitted{Type: "ellipse", Stroke: &stroke{Ink: ink, Width: 1}}
		case "square":
			marker.shape = &emitted{Type: "rectangle", Fill: ink}
		case "disclosure-closed":
			marker.shape = &emitted{Type: "polygon", Fill: ink,
				Points: []point{{0, 0}, {marker.size, marker.size / 2}, {0, marker.size}}}
		case "disclosure-open":
			marker.shape = &emitted{Type: "polygon", Fill: ink,
				Points: []point{{0, 0}, {marker.size, 0}, {marker.size / 2, marker.size}}}
		}
		return marker
	}
	text := current.listText
	if current.listType != "string" {
		text = markerText(current.listType, c.ordinal(node))
	}
	trimmed := strings.TrimRight(text, " ")
	marker.text, marker.written, marker.gap = trimmed, text, space*(len(text)-len(trimmed))
	if trimmed == "" {
		return nil
	}
	return marker
}

// ordinal is an item's number in its list: the one its value attribute
// states, or one on from the item before it, or where the list starts.
func (c *compiler) ordinal(node *html.Node) int {
	if known, ok := c.ordinals[node]; ok {
		return known
	}
	list := node.Parent
	reversed := list != nil && list.Data == "ol" && hasAttribute(list, "reversed")
	step := 1
	if reversed {
		step = -1
	}
	ordinal, stated := 0, false
	if node.Data == "li" {
		ordinal, stated = integerAttribute(node, "value")
	}
	if !stated {
		previous := node.PrevSibling
		for ; previous != nil; previous = previous.PrevSibling {
			// An item that is not drawn is not a list item: display: none
			// is the display it has instead.
			if computed, ok := c.computedFor[previous]; ok && computed.listItem {
				break
			}
		}
		switch {
		case previous != nil:
			ordinal = c.ordinal(previous) + step
		case list != nil && list.Data == "ol":
			ordinal, stated = integerAttribute(list, "start")
			if !stated {
				ordinal = 1
				if reversed {
					ordinal = countItems(list)
				}
			}
		default:
			ordinal = 1
		}
	}
	c.ordinals[node] = ordinal
	return ordinal
}

// countItems is how many items a reversed list counts down from.
func countItems(list *html.Node) int {
	count := 0
	for child := list.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "li" {
			count++
		}
	}
	return count
}

func hasAttribute(node *html.Node, name string) bool {
	for _, candidate := range node.Attr {
		if candidate.Key == name && candidate.Namespace == "" {
			return true
		}
	}
	return false
}

func integerAttribute(node *html.Node, name string) (int, bool) {
	if !hasAttribute(node, name) {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(attribute(node, name)))
	return value, err == nil
}

// outside hangs the marker in the margin beside the item's first line, which
// is where a browser puts it: outside the item's border, so that its padding
// moves the text away from the marker rather than the marker with it.
func (m *listMarker) outside(c *compiler, current style, path string) *emitted {
	_, top := current.edgesLengths()[0].Parts()
	var placed anchor
	if m.shape != nil {
		placed = anchor{Node: m.shape, Left: -(m.gap + m.size), Top: top + m.middle - m.size/2,
			Width: m.size, Height: m.size}
	} else {
		// The text is set against the right of a box that ends where the gap
		// begins, so that it ends there however wide it comes out.
		room := 10 * current.fontSize
		text := current
		text.decorated, text.decoration = 0, 0
		placed = anchor{Node: &emitted{Type: "text", Runs: plainRuns([]run{c.runOf(m.text, text, path)}),
			Align: "end", LineHeight: current.lineHeight},
			Left: -(m.gap + room), Top: top, Width: room}
	}
	return &emitted{Type: "anchored", Children: []anchor{placed}}
}

// inside is the marker as the first thing on the item's first line.
func (m *listMarker) inside(c *compiler, current style, path string) inlineItem {
	if m.shape != nil {
		shape := *m.shape
		shape.Size = &size{Width: m.size, Height: m.size}
		return inlineItem{Node: &shape, Margin: &insets{Right: m.gap}, VerticalAlign: "middle"}
	}
	text := current
	text.decorated, text.decoration = 0, 0
	return inlineItem{Runs: []run{c.runOf(m.written, text, path)}, LineHeight: current.lineHeight}
}
