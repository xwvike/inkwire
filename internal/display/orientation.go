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

// LandscapeFrame maps a logical page onto the tag's physical landscape panel.
func LandscapeFrame(frame *Frame, orientation Orientation) (*Frame, error) {
	if frame == nil {
		return nil, fmt.Errorf("frame must not be nil")
	}
	switch orientation {
	case OrientationLandscape:
		if frame.Width() != GiciskyWidth || frame.Height() != GiciskyHeight {
			return nil, fmt.Errorf("landscape page must be %dx%d, got %dx%d", GiciskyWidth, GiciskyHeight, frame.Width(), frame.Height())
		}
		return frame, nil
	case OrientationPortraitClockwise, OrientationPortraitCounterClockwise:
		if frame.Width() != GiciskyHeight || frame.Height() != GiciskyWidth {
			return nil, fmt.Errorf("portrait page must be %dx%d, got %dx%d", GiciskyHeight, GiciskyWidth, frame.Width(), frame.Height())
		}
	default:
		return nil, fmt.Errorf("invalid orientation %d", orientation)
	}

	landscape, err := NewFrame(GiciskyWidth, GiciskyHeight, InkWhite)
	if err != nil {
		return nil, err
	}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			switch orientation {
			case OrientationPortraitClockwise:
				landscape.Set(frame.Height()-1-y, x, ink)
			case OrientationPortraitCounterClockwise:
				landscape.Set(y, frame.Width()-1-x, ink)
			}
		}
	}
	return landscape, nil
}

func EncodeGiciskyOriented(frame *Frame, orientation Orientation) ([]byte, error) {
	landscape, err := LandscapeFrame(frame, orientation)
	if err != nil {
		return nil, err
	}
	return EncodeGicisky(landscape)
}
