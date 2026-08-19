package display

import "fmt"

type Orientation uint8

const (
	OrientationLandscape Orientation = iota
	OrientationPortraitClockwise
	OrientationPortraitCounterClockwise
)

func NewPage(orientation Orientation, background Ink) (*Frame, error) {
	switch orientation {
	case OrientationLandscape:
		return NewFrame(GiciskyWidth, GiciskyHeight, background)
	case OrientationPortraitClockwise, OrientationPortraitCounterClockwise:
		return NewFrame(GiciskyHeight, GiciskyWidth, background)
	default:
		return nil, fmt.Errorf("invalid orientation %d", orientation)
	}
}
