package display

import (
	"fmt"
	"image"
	"math"
)

// GradientDirection is the edge a gradient runs towards, as CSS names it.
type GradientDirection uint8

const (
	// GradientToBottom is first because it is what CSS does when no direction
	// is given.
	GradientToBottom GradientDirection = iota
	GradientToTop
	GradientToRight
	GradientToLeft
)

func (d GradientDirection) String() string {
	switch d {
	case GradientToBottom:
		return "bottom"
	case GradientToTop:
		return "top"
	case GradientToRight:
		return "right"
	case GradientToLeft:
		return "left"
	}
	return fmt.Sprintf("GradientDirection(%d)", uint8(d))
}

// GradientStop is a colour at a place along a gradient: At runs from 0 at the
// start edge to 1 at the edge the gradient runs towards. A stop may sit beyond
// either edge, as a CSS stop at -20% or 120% does, which places the part of the
// gradient the box shows rather than stretching it.
type GradientStop struct {
	Ink Ink
	At  float64
}

// Gradient is a fill whose tone changes across a box.
//
// A panel has no in-between shades, so a gradient is drawn the way a tone is:
// each pixel works out how much of the ink it should carry, and the same
// ordered dither that draws a grey decides whether that pixel is the ink or
// white. The dither has seventeen coverages, so a gradient is seventeen bands
// at most, with the stops deciding where each falls.
//
// Every stop is white, an ink, or a tone of that same ink — black through the
// greys to white, or red through its tints to white. Two inks in one gradient
// would need a pixel to be partly each, and a pixel is one ink.
type Gradient struct {
	Direction GradientDirection
	Stops     []GradientStop
}

// Validate reports what about a gradient cannot be drawn.
func (g *Gradient) Validate() error {
	if g == nil {
		return fmt.Errorf("gradient is missing")
	}
	if g.Direction > GradientToLeft {
		return fmt.Errorf("invalid gradient direction %d", g.Direction)
	}
	if len(g.Stops) < 2 {
		return fmt.Errorf("a gradient needs at least two stops, has %d", len(g.Stops))
	}
	if _, err := g.ink(); err != nil {
		return err
	}
	previous := 0.0
	for index, stop := range g.Stops {
		if math.IsNaN(stop.At) || math.IsInf(stop.At, 0) {
			return fmt.Errorf("stop %d is at %v", index, stop.At)
		}
		if index > 0 && stop.At < previous {
			return fmt.Errorf("stop %d is at %v, before the stop ahead of it at %v", index, stop.At, previous)
		}
		previous = stop.At
	}
	return nil
}

// ink is the one ink the gradient lays over white.
func (g *Gradient) ink() (Ink, error) {
	ink := InkWhite
	for index, stop := range g.Stops {
		if !stop.Ink.valid() {
			return 0, fmt.Errorf("stop %d has invalid ink %d", index, stop.Ink)
		}
		base, _, _ := stop.Ink.Toned()
		if base == InkWhite {
			continue
		}
		if ink != InkWhite && base != ink {
			return 0, fmt.Errorf("stops %s and %s are different inks, and a gradient runs between one ink and white", ink, base)
		}
		ink = base
	}
	if ink == InkWhite {
		// White to white: still a gradient, of nothing. Black is as good an
		// ink as any to have none of.
		ink = InkBlack
	}
	return ink, nil
}

// coverage is how much of the ink a stop carries, from 0 for white to 1.
func coverage(ink Ink) float64 {
	if ink == InkWhite {
		return 0
	}
	if _, sixteenths, toned := ink.Toned(); toned {
		return float64(sixteenths) / ToneSteps
	}
	return 1
}

// at is the tone a gradient gives the pixel at x, y of rect.
func (g *Gradient) at(rect image.Rectangle, ink Ink, x, y int) Ink {
	var place float64
	switch g.Direction {
	case GradientToRight, GradientToLeft:
		place = (float64(x-rect.Min.X) + 0.5) / float64(rect.Dx())
	default:
		place = (float64(y-rect.Min.Y) + 0.5) / float64(rect.Dy())
	}
	if g.Direction == GradientToLeft || g.Direction == GradientToTop {
		place = 1 - place
	}

	stops := g.Stops
	share := coverage(stops[0].Ink)
	switch {
	case place <= stops[0].At:
	case place >= stops[len(stops)-1].At:
		share = coverage(stops[len(stops)-1].Ink)
	default:
		for index := 1; index < len(stops); index++ {
			from, to := stops[index-1], stops[index]
			if place > to.At {
				continue
			}
			share = coverage(to.Ink)
			if span := to.At - from.At; span > 0 {
				blend := (place - from.At) / span
				share = coverage(from.Ink) + float64((coverage(to.Ink)-coverage(from.Ink))*blend)
			}
			break
		}
	}
	return Tone(ink, int(math.Round(float64(share*ToneSteps))))
}

// FillGradient fills a rectangle, rounded when radius is positive, with a
// gradient running across it.
//
// The place along the gradient is measured in the box's own coordinates, so a
// turned box carries its gradient round with it; the dither each pixel is
// drawn with is the frame's, so two gradients side by side meet the way two
// greys do.
func (c *Canvas) FillGradient(rect image.Rectangle, radius int, gradient *Gradient) {
	if rect.Empty() || gradient.Validate() != nil {
		return
	}
	ink, _ := gradient.ink()
	radius = clampRadius(rect, radius)
	c.fillWhere(rect, func(x, y int) (Ink, bool) {
		if radius > 0 && !pointInRoundRect(x, y, rect, radius) {
			return 0, false
		}
		return gradient.at(rect, ink, x, y), true
	})
}

// FillGradient records a gradient fill. A Gradient is not changed once handed
// over, so it is recorded by reference, as a Pattern is.
func (d *DisplayList) FillGradient(rect image.Rectangle, radius int, gradient *Gradient) {
	if rect.Empty() || gradient.Validate() != nil {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillGradient, rect: rect, radius: radius, gradient: gradient}, rect)
}
