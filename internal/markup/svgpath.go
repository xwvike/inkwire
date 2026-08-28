package markup

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A path's d attribute is the one place SVG is not simply another spelling of
// something the schema already has.
//
// The schema states a curve the way it is drawn: a cubic names both of its
// control points, an arc names the box it is inscribed in and the angles it
// runs between. SVG states a path the way it is written by hand and by a
// program: a letter, some numbers, and as much left out as can be worked out
// from what came before. A command repeats without being written again, a
// smooth curve leaves out the control point that mirrors the last one, and an
// arc names where it ends rather than where its centre is.
//
// So this is a translation rather than a mapping, and it is the one part of
// the drawing side with arithmetic in it.

// command is one instruction of a path, in the schema's spelling.
type command struct {
	Op       string  `json:"op"`
	To       *point  `json:"to,omitempty"`
	Control  *point  `json:"control,omitempty"`
	Control1 *point  `json:"control1,omitempty"`
	Control2 *point  `json:"control2,omitempty"`
	Bounds   *rect   `json:"bounds,omitempty"`
	Start    float64 `json:"start,omitempty"`
	Sweep    float64 `json:"sweep,omitempty"`
}

// pathPen carries what a command needs to know about the ones before it: where
// the pen is, where the contour it is drawing began, and the control point a
// smooth curve mirrors.
type pathPen struct {
	x, y         float64
	startX       float64
	startY       float64
	lastControlX float64
	lastControlY float64
	lastWasCubic bool
	lastWasQuad  bool
}

// parsePathData reads a d attribute into the commands that draw it.
//
// It reports rather than fails: a path with something unreadable in it draws
// as far as it got, which on a panel is a partial picture instead of none.
func (c *compiler) parsePathData(data string, frame svgFrame, path string) []command {
	scanner := &pathScanner{source: data}
	var commands []command
	var pen pathPen
	letter := byte(0)
	for {
		next, ok := scanner.command()
		if !ok {
			// A letter this does not know ends the path, and saying which one
			// is the whole difference between a picture that stops early and
			// a picture that stops early for no reason anybody can see.
			if scanner.failed != "" {
				c.warn(path, "unsupported-declaration", fmt.Sprintf("d: %s", scanner.failed))
			}
			break
		}
		if next != 0 {
			letter = next
		} else if letter == 0 {
			c.warn(path, "unsupported-declaration", "a path begins with a number rather than a command")
			return commands
		} else if letter == 'M' {
			// A moveto's second pair onwards is a lineto, which is how a
			// polygon written as a path has one letter and many points.
			letter = 'L'
		} else if letter == 'm' {
			letter = 'l'
		}
		emitted, err := c.pathCommand(letter, scanner, &pen, frame, path)
		if err != "" {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("d: %s", err))
			return commands
		}
		commands = append(commands, emitted...)
		if scanner.failed != "" {
			c.warn(path, "unsupported-declaration", fmt.Sprintf("d: %s", scanner.failed))
			return commands
		}
	}
	return commands
}

// pathCommand reads the arguments of one command and turns it into the schema's.
func (c *compiler) pathCommand(letter byte, scanner *pathScanner, pen *pathPen, frame svgFrame, path string) ([]command, string) {
	relative := letter >= 'a' && letter <= 'z'
	// The absolute form is the one the arithmetic is written in; a relative
	// one is the same command measured from where the pen already is.
	absolute := func(x, y float64) (float64, float64) {
		if relative {
			return pen.x + x, pen.y + y
		}
		return x, y
	}
	switch letter | 0x20 {
	case 'm':
		x, y, ok := scanner.pair()
		if !ok {
			return nil, "a moveto needs two numbers"
		}
		pen.x, pen.y = absolute(x, y)
		pen.startX, pen.startY = pen.x, pen.y
		pen.lastWasCubic, pen.lastWasQuad = false, false
		return []command{{Op: "move", To: atFrame(frame, pen.x, pen.y)}}, ""

	case 'l':
		x, y, ok := scanner.pair()
		if !ok {
			return nil, "a lineto needs two numbers"
		}
		pen.x, pen.y = absolute(x, y)
		pen.lastWasCubic, pen.lastWasQuad = false, false
		return []command{{Op: "line", To: atFrame(frame, pen.x, pen.y)}}, ""

	case 'h', 'v':
		value, ok := scanner.number()
		if !ok {
			return nil, "a horizontal or vertical lineto needs one number"
		}
		if letter|0x20 == 'h' {
			pen.x, _ = absolute(value, 0)
		} else {
			_, pen.y = absolute(0, value)
		}
		pen.lastWasCubic, pen.lastWasQuad = false, false
		return []command{{Op: "line", To: atFrame(frame, pen.x, pen.y)}}, ""

	case 'c', 's':
		var control1X, control1Y float64
		if letter|0x20 == 'c' {
			x, y, ok := scanner.pair()
			if !ok {
				return nil, "a cubic needs six numbers"
			}
			control1X, control1Y = absolute(x, y)
		} else {
			// The smooth form leaves out the first control point, which is
			// the reflection of the last one about where the pen is. A smooth
			// curve after anything but a cubic has nothing to reflect, and
			// the spec says to use the pen's own position.
			control1X, control1Y = pen.x, pen.y
			if pen.lastWasCubic {
				control1X = 2*pen.x - pen.lastControlX
				control1Y = 2*pen.y - pen.lastControlY
			}
		}
		x2, y2, ok := scanner.pair()
		if !ok {
			return nil, "a cubic needs its second control point"
		}
		control2X, control2Y := absolute(x2, y2)
		x, y, ok := scanner.pair()
		if !ok {
			return nil, "a cubic needs an end point"
		}
		endX, endY := absolute(x, y)
		pen.lastControlX, pen.lastControlY = control2X, control2Y
		pen.x, pen.y = endX, endY
		pen.lastWasCubic, pen.lastWasQuad = true, false
		return []command{{Op: "cubic",
			Control1: atFrame(frame, control1X, control1Y),
			Control2: atFrame(frame, control2X, control2Y),
			To:       atFrame(frame, endX, endY)}}, ""

	case 'q', 't':
		var controlX, controlY float64
		if letter|0x20 == 'q' {
			x, y, ok := scanner.pair()
			if !ok {
				return nil, "a quadratic needs four numbers"
			}
			controlX, controlY = absolute(x, y)
		} else {
			controlX, controlY = pen.x, pen.y
			if pen.lastWasQuad {
				controlX = 2*pen.x - pen.lastControlX
				controlY = 2*pen.y - pen.lastControlY
			}
		}
		x, y, ok := scanner.pair()
		if !ok {
			return nil, "a quadratic needs an end point"
		}
		endX, endY := absolute(x, y)
		pen.lastControlX, pen.lastControlY = controlX, controlY
		pen.x, pen.y = endX, endY
		pen.lastWasCubic, pen.lastWasQuad = false, true
		return []command{{Op: "quadratic", Control: atFrame(frame, controlX, controlY), To: atFrame(frame, endX, endY)}}, ""

	case 'a':
		radiusX, radiusY, ok := scanner.pair()
		if !ok {
			return nil, "an arc needs seven numbers"
		}
		rotation, ok := scanner.number()
		if !ok {
			return nil, "an arc needs its x-axis rotation"
		}
		largeArc, ok := scanner.flag()
		if !ok {
			return nil, "an arc needs its large-arc flag"
		}
		sweepFlag, ok := scanner.flag()
		if !ok {
			return nil, "an arc needs its sweep flag"
		}
		x, y, ok := scanner.pair()
		if !ok {
			return nil, "an arc needs an end point"
		}
		endX, endY := absolute(x, y)
		if rotation != 0 {
			c.warn(path, "unsupported-declaration", fmt.Sprintf(
				"d: an arc rotated by %g degrees is drawn unrotated; an arc here is square to the page", rotation))
		}
		bounds, start, sweep, drawn := arcFromEndpoints(frame,
			pen.x, pen.y, endX, endY, math.Abs(radiusX), math.Abs(radiusY), largeArc, sweepFlag)
		pen.x, pen.y = endX, endY
		pen.lastWasCubic, pen.lastWasQuad = false, false
		if !drawn {
			// Either radius is zero, which the spec says is a straight line.
			return []command{{Op: "line", To: atFrame(frame, endX, endY)}}, ""
		}
		return []command{{Op: "arc", Bounds: bounds, Start: start, Sweep: sweep}}, ""

	case 'z':
		pen.x, pen.y = pen.startX, pen.startY
		pen.lastWasCubic, pen.lastWasQuad = false, false
		return []command{{Op: "close"}}, ""
	}
	return nil, fmt.Sprintf("%q is not a path command", string(letter))
}

// arcFromEndpoints turns the arc SVG writes into the arc the schema draws.
//
// SVG names where an arc ends and which of the four arcs between two points is
// meant; the schema names the box the whole ellipse fits in and the angles the
// arc runs between. Getting from one to the other is the conversion in the SVG
// specification's implementation notes, and it is written out here because
// there is no way to guess it.
//
// The box is a pixel wider and taller than twice the radius, because the
// drawing model measures a radius as half of one less than the width — the
// ellipse touches the last pixel inside the box rather than the edge of it.
func arcFromEndpoints(frame svgFrame, x1, y1, x2, y2, radiusX, radiusY float64, largeArc, sweepFlag bool) (*rect, float64, float64, bool) {
	if radiusX == 0 || radiusY == 0 || (x1 == x2 && y1 == y2) {
		return nil, 0, 0, false
	}
	// Halfway between the two points, which is where the maths is easiest.
	halfX, halfY := (x1-x2)/2, (y1-y2)/2

	// A radius too small to reach between the points is scaled up until it
	// does, which the spec asks for rather than treating as an error.
	lambda := halfX*halfX/(radiusX*radiusX) + halfY*halfY/(radiusY*radiusY)
	if lambda > 1 {
		scale := math.Sqrt(lambda)
		radiusX, radiusY = radiusX*scale, radiusY*scale
	}

	numerator := radiusX*radiusX*radiusY*radiusY - radiusX*radiusX*halfY*halfY - radiusY*radiusY*halfX*halfX
	denominator := radiusX*radiusX*halfY*halfY + radiusY*radiusY*halfX*halfX
	factor := 0.0
	if denominator != 0 && numerator > 0 {
		factor = math.Sqrt(numerator / denominator)
	}
	if largeArc == sweepFlag {
		factor = -factor
	}
	offsetX := factor * radiusX * halfY / radiusY
	offsetY := -factor * radiusY * halfX / radiusX
	centreX := offsetX + (x1+x2)/2
	centreY := offsetY + (y1+y2)/2

	start := degrees(math.Atan2((halfY-offsetY)/radiusY, (halfX-offsetX)/radiusX))
	end := degrees(math.Atan2((-halfY-offsetY)/radiusY, (-halfX-offsetX)/radiusX))
	sweep := end - start
	// The sweep flag says which way round, and the two ways differ by a whole
	// turn. Which of the four arcs is meant is settled by this and largeArc
	// together, and both have already had their say by here.
	if sweepFlag && sweep < 0 {
		sweep += 360
	}
	if !sweepFlag && sweep > 0 {
		sweep -= 360
	}
	// The box is stated in the drawing's coordinates, which is where the
	// group's own offset and magnification are applied.
	cornerX, cornerY := frame.place(centreX-radiusX, centreY-radiusY)
	return &rect{
		X:      pixels(cornerX),
		Y:      pixels(cornerY),
		Width:  pixels(frame.size(radiusX*2)) + 1,
		Height: pixels(frame.size(radiusY*2)) + 1,
	}, start, sweep, true
}

func degrees(radians float64) float64 { return radians * 180 / math.Pi }

// pathScanner reads path data, where a command is a letter and its arguments
// are numbers that need not be separated by anything at all.
type pathScanner struct {
	source string
	at     int
	failed string
}

// command returns the next command letter, or zero when the next thing is a
// number — which means the command before it repeats.
func (s *pathScanner) command() (byte, bool) {
	s.skip()
	if s.at >= len(s.source) {
		return 0, false
	}
	character := s.source[s.at]
	if strings.IndexByte("MmLlHhVvCcSsQqTtAaZz", character) >= 0 {
		s.at++
		return character, true
	}
	if strings.IndexByte("0123456789.+-", character) >= 0 {
		return 0, true
	}
	s.failed = fmt.Sprintf("%q is not a path command", string(character))
	return 0, false
}

func (s *pathScanner) pair() (float64, float64, bool) {
	x, ok := s.number()
	if !ok {
		return 0, 0, false
	}
	y, ok := s.number()
	if !ok {
		return 0, 0, false
	}
	return x, y, true
}

// flag reads an arc's large-arc or sweep flag, which is a single character and
// may be written with nothing between it and the number after it.
func (s *pathScanner) flag() (bool, bool) {
	s.skip()
	if s.at >= len(s.source) {
		return false, false
	}
	switch s.source[s.at] {
	case '0':
		s.at++
		return false, true
	case '1':
		s.at++
		return true, true
	}
	return false, false
}

// number reads one number. Two may abut with nothing between them, since a
// minus sign separates as well as negates and a second decimal point starts a
// second number: "1.5.5" is two numbers and "10-20" is two more.
func (s *pathScanner) number() (float64, bool) {
	s.skip()
	start := s.at
	if s.at < len(s.source) && (s.source[s.at] == '+' || s.source[s.at] == '-') {
		s.at++
	}
	digits, dot := false, false
	for s.at < len(s.source) {
		character := s.source[s.at]
		switch {
		case character >= '0' && character <= '9':
			digits = true
		case character == '.' && !dot:
			dot = true
		case (character == 'e' || character == 'E') && digits:
			// An exponent, and the sign that may follow it.
			if s.at+1 < len(s.source) && (s.source[s.at+1] == '+' || s.source[s.at+1] == '-') {
				s.at++
			}
		default:
			goto done
		}
		s.at++
	}
done:
	if !digits {
		s.at = start
		return 0, false
	}
	value, err := strconv.ParseFloat(s.source[start:s.at], 64)
	if err != nil {
		s.at = start
		return 0, false
	}
	return value, true
}

func (s *pathScanner) skip() {
	for s.at < len(s.source) {
		switch s.source[s.at] {
		case ' ', ',', '\t', '\n', '\r', '\f':
			s.at++
		default:
			return
		}
	}
}
