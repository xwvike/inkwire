package markup

import (
	"fmt"
	"strings"
)

// mathFunctions are the CSS functions that work a length out of others.
var mathFunctions = []string{"calc(", "min(", "max(", "clamp("}

// isMath reports a value that is one of mathFunctions.
func isMath(lower string) bool {
	for _, name := range mathFunctions {
		if strings.HasPrefix(lower, name) {
			return true
		}
	}
	return false
}

// parseMath works out calc(), min(), max() and clamp() as CSS does: sums and
// differences of lengths, products and quotients of a length and a number,
// brackets, and any of the four inside another.
//
// What comes out is a share of the containing block and a number of pixels,
// which is all a length here can hold until layout. min(), max() and clamp()
// choose between lengths, which can only be done before layout when none of
// them is a share of something; one that is is said and dropped.
func parseMath(value, property string, report func(string)) length {
	reader := &mathReader{tokens: mathTokens(value), property: property, report: report}
	result, ok := reader.sum()
	if ok && reader.at < len(reader.tokens) {
		reader.fail(fmt.Sprintf("%q is not where it can be read", reader.tokens[reader.at]))
		ok = false
	}
	if !ok {
		return length{}
	}
	if result.number {
		if result.value.pixels != 0 {
			report(fmt.Sprintf("%s: %s works out to a number, not a length; give it a unit", property, value))
			return length{}
		}
		return length{set: true}
	}
	return result.value
}

// quantity is a length, or a plain number that a length can be multiplied or
// divided by. A number is kept in value.pixels.
type quantity struct {
	value  length
	number bool
}

type mathReader struct {
	tokens   []string
	at       int
	property string
	report   func(string)
	failed   bool
}

func (r *mathReader) fail(message string) {
	if !r.failed {
		r.report(fmt.Sprintf("%s: %s", r.property, message))
	}
	r.failed = true
}

func (r *mathReader) peek() string {
	if r.at < len(r.tokens) {
		return r.tokens[r.at]
	}
	return ""
}

func (r *mathReader) next() string {
	token := r.peek()
	if token != "" {
		r.at++
	}
	return token
}

func (r *mathReader) sum() (quantity, bool) {
	total, ok := r.product()
	for ok {
		operator := r.peek()
		if operator != "+" && operator != "-" {
			break
		}
		r.next()
		var term quantity
		if term, ok = r.product(); !ok {
			break
		}
		// A bare zero is how a length of nothing is usually written, so it
		// is one here even though CSS would want 0px.
		if total.number != term.number && !(isZero(total) || isZero(term)) {
			r.fail("calc() cannot add a number to a length; give the number a unit")
			return quantity{}, false
		}
		sign := 1.0
		if operator == "-" {
			sign = -1
		}
		total = quantity{
			value: length{set: true, percent: total.value.percent + float64(sign*term.value.percent),
				pixels: total.value.pixels + float64(sign*term.value.pixels)},
			number: total.number && term.number,
		}
	}
	return total, ok
}

func isZero(q quantity) bool { return q.value.percent == 0 && q.value.pixels == 0 }

func (r *mathReader) product() (quantity, bool) {
	total, ok := r.unit()
	for ok {
		operator := r.peek()
		if operator != "*" && operator != "/" {
			break
		}
		r.next()
		var factor quantity
		if factor, ok = r.unit(); !ok {
			break
		}
		switch {
		case operator == "/" && !factor.number:
			r.fail("calc() can divide by a number, not by a length")
			return quantity{}, false
		case operator == "/" && factor.value.pixels == 0:
			r.fail("calc() divides by zero")
			return quantity{}, false
		case operator == "/":
			total.value.percent /= factor.value.pixels
			total.value.pixels /= factor.value.pixels
		case !total.number && !factor.number:
			r.fail("calc() can multiply a length by a number, not by another length")
			return quantity{}, false
		case total.number:
			scale := total.value.pixels
			total = factor
			total.value.percent *= scale
			total.value.pixels *= scale
		default:
			total.value.percent *= factor.value.pixels
			total.value.pixels *= factor.value.pixels
		}
	}
	return total, ok
}

func (r *mathReader) unit() (quantity, bool) {
	token := r.next()
	switch {
	case token == "" || token == ")" || token == "," || strings.ContainsAny(token, "+-*/") && len(token) == 1:
		r.fail("calc() has an operator with no length after it")
		return quantity{}, false
	case token == "(" || token == "calc(":
		inner, ok := r.sum()
		return inner, ok && r.close()
	case token == "min(" || token == "max(" || token == "clamp(":
		return r.choice(strings.TrimSuffix(token, "("))
	}
	if number, err := parseFinite(token); err == nil {
		return quantity{value: length{set: true, pixels: number}, number: true}, true
	}
	parsed := parseLength(token, r.property, func(message string) {
		r.fail(strings.TrimPrefix(message, r.property+": "))
	})
	if !parsed.set {
		r.fail(fmt.Sprintf("%s is not a length", token))
		return quantity{}, false
	}
	return quantity{value: parsed}, true
}

func (r *mathReader) close() bool {
	switch token := r.next(); token {
	case ")":
		return true
	case "":
		r.fail("a bracket is not closed")
	default:
		r.fail(fmt.Sprintf("%q follows a length with no operator between them", token))
	}
	return false
}

// choice works out min(), max() or clamp().
func (r *mathReader) choice(name string) (quantity, bool) {
	var arguments []quantity
	for {
		argument, ok := r.sum()
		if !ok {
			return quantity{}, false
		}
		arguments = append(arguments, argument)
		if r.peek() != "," {
			break
		}
		r.next()
	}
	if !r.close() {
		return quantity{}, false
	}
	if name == "clamp" && len(arguments) != 3 {
		r.fail("clamp() takes a least, a preferred and a most value")
		return quantity{}, false
	}
	for _, argument := range arguments[1:] {
		if argument.number != arguments[0].number {
			r.fail(name + "() cannot choose between a number and a length")
			return quantity{}, false
		}
	}
	for _, argument := range arguments {
		if argument.value.percent != 0 {
			r.fail(name + "() with a percentage cannot be worked out until the box it is a share of is laid out; " +
				"write the lengths in pixels, or use calc()")
			return quantity{}, false
		}
	}
	pick := arguments[0]
	switch name {
	case "min":
		for _, argument := range arguments[1:] {
			if argument.value.pixels < pick.value.pixels {
				pick = argument
			}
		}
	case "max":
		for _, argument := range arguments[1:] {
			if argument.value.pixels > pick.value.pixels {
				pick = argument
			}
		}
	default:
		least, preferred, most := arguments[0], arguments[1], arguments[2]
		pick = preferred
		if most.value.pixels < pick.value.pixels {
			pick = most
		}
		if least.value.pixels > pick.value.pixels {
			pick = least
		}
	}
	return pick, true
}

// mathTokens splits a value into numbers with their units, operators,
// brackets and the names of the functions they open. A sign is part of a
// number when it comes where a number is expected and is written against it,
// as in "-2px"; anywhere else it is an operator, so "100%-10px" reads as a
// difference, as it does in every calculator if not in CSS.
func mathTokens(value string) []string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var tokens []string
	operand := func() bool {
		if len(tokens) == 0 {
			return true
		}
		last := tokens[len(tokens)-1]
		return strings.HasSuffix(last, "(") || last == "," || last == "+" || last == "-" || last == "*" || last == "/"
	}
	for index := 0; index < len(lower); {
		character := lower[index]
		switch {
		case character == ' ' || character == '\t' || character == '\n':
			index++
		case strings.ContainsRune("()*/,", rune(character)):
			tokens = append(tokens, string(character))
			index++
		case (character == '+' || character == '-') && !(operand() && index+1 < len(lower) &&
			(isDigit(lower[index+1]) || lower[index+1] == '.')):
			tokens = append(tokens, string(character))
			index++
		default:
			// A token runs to the next space, bracket or operator. Nothing
			// here is a word with a minus inside it, so a minus always ends
			// one: "100%-10px" is a difference.
			end := index + 1
			for end < len(lower) && !strings.ContainsRune(" \t\n()*/,+-", rune(lower[end])) {
				end++
			}
			if end < len(lower) && lower[end] == '(' {
				end++
			}
			tokens = append(tokens, lower[index:end])
			index = end
		}
	}
	return tokens
}
