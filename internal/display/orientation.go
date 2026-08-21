package display

type Orientation uint8

const (
	OrientationLandscape Orientation = iota
	OrientationPortraitClockwise
	OrientationPortraitCounterClockwise
)

// Valid reports whether an orientation is one of the three.
//
// It used to be checked by NewPage, which every page went through on its way
// to being drawn. Nothing goes through NewPage any more — a page is made at a
// stated size — so the check has to be somewhere a compiler can ask for it.
func (o Orientation) Valid() bool {
	switch o {
	case OrientationLandscape, OrientationPortraitClockwise, OrientationPortraitCounterClockwise:
		return true
	}
	return false
}
