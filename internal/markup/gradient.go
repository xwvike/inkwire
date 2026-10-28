package markup

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/display"
)

// isGradient reports whether a background value is written as a gradient, so
// that it is read as one rather than offered to the colour parser, which would
// call it a colour the panel does not have.
func isGradient(value string) bool {
	return strings.Contains(strings.ToLower(value), "gradient(")
}

// parseLinearGradient reads a CSS linear-gradient into the fill the panel
// draws it with.
//
// What it can draw is a gradient between white and one ink — black and the
// greys, or red and its tints — running towards one side of the box, with
// stops placed in percentages. That covers the gradient most pages write. The
// rest is refused out loud, with what to write instead, because a gradient
// that quietly became something else would be a design nobody asked for.
func parseLinearGradient(value, property string, report func(string)) (*display.Gradient, bool) {
	refuse := func(format string, args ...any) (*display.Gradient, bool) {
		report(fmt.Sprintf("%s: %s", property, fmt.Sprintf(format, args...)))
		return nil, false
	}
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, other := range []string{"repeating-linear-gradient(", "radial-gradient(", "repeating-radial-gradient(", "conic-gradient(", "repeating-conic-gradient("} {
		if strings.HasPrefix(lower, other) {
			return refuse("%s is not supported; linear-gradient() is", strings.TrimSuffix(other, "("))
		}
	}
	if !strings.HasPrefix(lower, "linear-gradient(") || !strings.HasSuffix(trimmed, ")") {
		return refuse("%s is not a single linear-gradient(); a background holds one gradient or one colour", value)
	}
	arguments := splitTopLevel(trimmed[len("linear-gradient(") : len(trimmed)-1])

	gradient := &display.Gradient{Direction: display.GradientToBottom}
	if len(arguments) > 0 {
		if direction, isDirection, ok := parseGradientDirection(arguments[0]); isDirection {
			if !ok {
				return refuse("%s is not a direction this draws; use to top, to right, to bottom, to left, or 0deg, 90deg, 180deg, 270deg", strings.TrimSpace(arguments[0]))
			}
			gradient.Direction = direction
			arguments = arguments[1:]
		}
	}
	if len(arguments) < 2 {
		return refuse("a gradient needs at least two colours, %s has %d", value, len(arguments))
	}

	// Positions as written; nil where a stop leaves its place to be worked out.
	places := make([]*float64, len(arguments))
	for index, argument := range arguments {
		fields := splitSides(strings.TrimSpace(argument))
		if len(fields) == 0 {
			return refuse("%s has an empty colour stop", value)
		}
		if len(fields) > 2 {
			return refuse("%q has more than a colour and one position; two-position stops are not supported", strings.TrimSpace(argument))
		}
		if len(fields) == 1 && isStopPosition(fields[0]) {
			return refuse("%s is a position with no colour; colour hints are not supported", fields[0])
		}
		var said []string
		ink, ok := parseInk(fields[0], property, func(message string) { said = append(said, message) })
		if !ok {
			if len(said) == 0 {
				// transparent and none come back unreported, and a gradient
				// fading into the page behind it is not something a panel
				// with no transparency can draw.
				return refuse("%s cannot stand in a gradient; use white for the light end", fields[0])
			}
			for _, message := range said {
				report(message)
			}
			return nil, false
		}
		// A colour taken with something said about it, such as a tinted grey
		// drawn as a plain one, is still said.
		for _, message := range said {
			report(message)
		}
		gradient.Stops = append(gradient.Stops, display.GradientStop{Ink: ink})
		if len(fields) == 2 {
			place, ok := parseStopPosition(fields[1])
			if !ok {
				return refuse("%s is not a stop position this reads; positions are percentages", fields[1])
			}
			places[index] = &place
		}
	}
	placeStops(gradient.Stops, places)
	if err := gradient.Validate(); err != nil {
		return refuse("%s", err)
	}
	return gradient, true
}

// parseGradientDirection reads the optional first argument. isDirection says
// whether the argument is a direction at all — the alternative is that it is
// the first colour — and ok whether it is one this draws.
func parseGradientDirection(argument string) (direction display.GradientDirection, isDirection, ok bool) {
	words := strings.Fields(strings.ToLower(argument))
	if len(words) == 0 {
		return 0, false, false
	}
	if words[0] == "to" {
		if len(words) != 2 {
			return 0, true, false // a corner, such as to top right
		}
		switch words[1] {
		case "top":
			return display.GradientToTop, true, true
		case "right":
			return display.GradientToRight, true, true
		case "bottom":
			return display.GradientToBottom, true, true
		case "left":
			return display.GradientToLeft, true, true
		}
		return 0, true, false
	}
	if len(words) != 1 {
		return 0, false, false
	}
	var degrees float64
	number, unit, numeric := splitUnit(words[0])
	if !numeric {
		return 0, false, false
	}
	switch unit {
	case "deg":
		degrees = number
	case "turn":
		degrees = float64(number * 360)
	case "grad":
		degrees = float64(number * 0.9)
	case "rad":
		degrees = number * 180 / math.Pi
	default:
		return 0, false, false
	}
	// CSS measures the angle clockwise from pointing up, so a quarter turn is
	// to right. Only the four quarters are sides of a box.
	turned := math.Mod(math.Mod(degrees, 360)+360, 360)
	for _, side := range []struct {
		degrees   float64
		direction display.GradientDirection
	}{{0, display.GradientToTop}, {90, display.GradientToRight}, {180, display.GradientToBottom}, {270, display.GradientToLeft}} {
		if math.Abs(turned-side.degrees) < 1e-9 {
			return side.direction, true, true
		}
	}
	return 0, true, false
}

// splitUnit separates a number from the unit written after it, and says
// whether there was a number there at all.
func splitUnit(word string) (float64, string, bool) {
	end := len(word)
	for end > 0 && (word[end-1] < '0' || word[end-1] > '9') && word[end-1] != '.' {
		end--
	}
	number, err := strconv.ParseFloat(word[:end], 64)
	if err != nil {
		return 0, "", false
	}
	return number, word[end:], true
}

// isStopPosition reports whether a word is a length or percentage, which is
// what tells a colour hint — a position standing alone — from a colour.
func isStopPosition(word string) bool {
	_, unit, numeric := splitUnit(strings.ToLower(word))
	return numeric && (unit == "%" || unit == "px" || unit == "")
}

func parseStopPosition(word string) (float64, bool) {
	number, unit, numeric := splitUnit(strings.ToLower(word))
	switch {
	case !numeric:
		return 0, false
	case unit == "%":
		return number / 100, true
	case unit == "" && number == 0:
		return 0, true
	}
	return 0, false
}

// placeStops fills in the positions a page left out, the way CSS does: the
// first stop is at the start and the last at the end, a stop placed before one
// ahead of it is moved up to it, and stops in between with no place of their
// own are spread evenly between the ones that have one.
func placeStops(stops []display.GradientStop, places []*float64) {
	last := len(stops) - 1
	at := make([]float64, len(stops))
	known := make([]bool, len(stops))
	for index, place := range places {
		if place != nil {
			at[index], known[index] = *place, true
		}
	}
	if !known[0] {
		at[0], known[0] = 0, true
	}
	if !known[last] {
		at[last], known[last] = 1, true
	}
	highest := at[0]
	for index := range at {
		if known[index] {
			highest = max(highest, at[index])
			at[index] = highest
		}
	}
	for index := 1; index < last; index++ {
		if known[index] {
			continue
		}
		next := index
		for !known[next] {
			next++
		}
		from, to := at[index-1], at[next]
		steps := float64(next - index + 1)
		for fill := index; fill < next; fill++ {
			at[fill] = from + (to-from)*float64(fill-index+1)/steps
			known[fill] = true
		}
	}
	for index := range stops {
		stops[index].At = at[index]
	}
}

// splitTopLevel splits a function's arguments at the commas between them,
// leaving any comma inside a nested function where it is.
func splitTopLevel(text string) []string {
	var parts []string
	depth, start := 0, 0
	for index, r := range text {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, text[start:index])
				start = index + 1
			}
		}
	}
	return append(parts, text[start:])
}

// gradientJSON is a gradient as the scene document spells it.
type gradientJSON struct {
	To    string             `json:"to,omitempty"`
	Stops []gradientStopJSON `json:"stops"`
}

type gradientStopJSON struct {
	Ink string  `json:"ink"`
	At  float64 `json:"at"`
}

func gradientOf(gradient *display.Gradient) *gradientJSON {
	if gradient == nil {
		return nil
	}
	written := &gradientJSON{}
	if gradient.Direction != display.GradientToBottom {
		written.To = gradient.Direction.String()
	}
	for _, stop := range gradient.Stops {
		written.Stops = append(written.Stops, gradientStopJSON{Ink: inkName(stop.Ink), At: stop.At})
	}
	return written
}
