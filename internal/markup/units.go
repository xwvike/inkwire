package markup

import (
	"fmt"
	stdimage "image"
	"math"
	"strconv"
	"strings"
)

// relativeUnits is what the font- and viewport-relative units come to for one
// element: em is a font size, rem the root element's, and the viewport is the
// page, which the root element states in pixels.
type relativeUnits struct {
	em, rem  float64
	viewport stdimage.Point
}

// resolve writes every em, rem, vw, vh, vmin and vmax in a declaration's value
// as the pixels it comes to, and every pt, pc, in, cm, mm and Q as well, so the rest of the renderer, which measures in
// pixels, reads it as though the author had written those. Quoted text and
// url() are left as they are: a number in a font name or an address is not a
// length.
//
// A viewport unit on a page with no stated size has nothing to be a share of,
// and so does one in the root element's own size; that is said through report
// and the declaration is dropped.
func (u relativeUnits) resolve(value string, report func(string)) (string, bool) {
	var out strings.Builder
	for index := 0; index < len(value); {
		character := value[index]
		switch {
		case character == '"' || character == '\'':
			end := strings.IndexByte(value[index+1:], character)
			if end < 0 {
				out.WriteString(value[index:])
				return out.String(), true
			}
			out.WriteString(value[index : index+end+2])
			index += end + 2
			continue
		case hasPrefixFold(value[index:], "url(") && !identifierBefore(value, index):
			end := strings.IndexByte(value[index:], ')')
			if end < 0 {
				out.WriteString(value[index:])
				return out.String(), true
			}
			out.WriteString(value[index : index+end+1])
			index += end + 1
			continue
		}
		number, unit, next := relativeLength(value, index)
		if unit == "" {
			out.WriteByte(character)
			index++
			continue
		}
		pixels, problem := u.pixels(number, unit)
		if problem != "" {
			if report != nil {
				report(problem)
			}
			return "", false
		}
		// Rounded well below a pixel, so 0.29 of 100 is 29 and not the
		// 28.999… that a whole-pixel property would cut down to 28.
		out.WriteString(strconv.FormatFloat(math.Round(float64(pixels*1000))/1000, 'f', -1, 64))
		out.WriteString("px")
		index = next
	}
	return out.String(), true
}

func (u relativeUnits) pixels(number float64, unit string) (float64, string) {
	width, height := float64(u.viewport.X), float64(u.viewport.Y)
	switch unit {
	case "em":
		return float64(number * u.em), ""
	case "rem":
		return float64(number * u.rem), ""
	}
	if perInch, ok := absoluteUnits[unit]; ok {
		return number * 96 / perInch, ""
	}
	if width <= 0 || height <= 0 {
		return 0, fmt.Sprintf("%s is a share of the page, and the page's size is not known here: "+
			"give the root element a width and height in px, and use %s only inside it", unit, unit)
	}
	switch unit {
	case "vw":
		return number * width / 100, ""
	case "vh":
		return number * height / 100, ""
	case "vmin":
		return number * min(width, height) / 100, ""
	}
	return number * max(width, height) / 100, ""
}

// absoluteUnits are the physical units, as how many of each make an inch. CSS
// fixes an inch at 96px whatever the screen, so these are exact.
var absoluteUnits = map[string]float64{"pt": 72, "pc": 6, "in": 1, "cm": 2.54, "mm": 25.4, "q": 101.6}

// relativeUnitNames are the units resolve rewrites, longest first, so that
// "rem" and "vmin" are not read as "em" and "vm…".
var relativeUnitNames = []string{"vmin", "vmax", "rem", "em", "vw", "vh", "pt", "pc", "in", "cm", "mm", "q"}

// relativeLength reads a number with one of the relative units at index,
// returning the unit as "" when there is none there. The number must start a
// token, so the 1 in "h1em" or "#1em" is not one, and the unit must end one,
// so "1emx" is not a length either.
func relativeLength(value string, index int) (float64, string, int) {
	if identifierBefore(value, index) || (index > 0 && (value[index-1] == '#' || value[index-1] == '.')) {
		return 0, "", index
	}
	end := index
	if end < len(value) && (value[end] == '+' || value[end] == '-') {
		end++
	}
	digits := false
	for end < len(value) && isDigit(value[end]) {
		end++
		digits = true
	}
	if end < len(value) && value[end] == '.' {
		end++
		for end < len(value) && isDigit(value[end]) {
			end++
			digits = true
		}
	}
	if !digits {
		return 0, "", index
	}
	for _, unit := range relativeUnitNames {
		if !hasPrefixFold(value[end:], unit) {
			continue
		}
		after := end + len(unit)
		if after < len(value) && isIdentifierByte(value[after]) {
			return 0, "", index
		}
		number, err := parseFinite(value[index:end])
		if err != nil {
			return 0, "", index
		}
		return number, unit, after
	}
	return 0, "", index
}

func identifierBefore(value string, index int) bool {
	return index > 0 && (isIdentifierByte(value[index-1]) || isDigit(value[index-1]))
}

func isDigit(b byte) bool { return '0' <= b && b <= '9' }

func isIdentifierByte(b byte) bool {
	return b == '-' || b == '_' || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || b >= 0x80 || isDigit(b)
}

func hasPrefixFold(value, prefix string) bool {
	return len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)
}
