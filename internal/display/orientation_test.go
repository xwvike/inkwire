package display

import "testing"

// Orientation used to be validated on the way through NewPage, which every
// page went through. Nothing does any more — a page is made at a size somebody
// stated — so the check has to stand on its own, and something has to still be
// asking it.
func TestOrientationKnowsItsThreeValues(t *testing.T) {
	for _, valid := range []Orientation{
		OrientationLandscape,
		OrientationPortraitClockwise,
		OrientationPortraitCounterClockwise,
	} {
		if !valid.Valid() {
			t.Errorf("orientation %d reported invalid", valid)
		}
	}
	for _, invalid := range []Orientation{3, 9, 255} {
		if invalid.Valid() {
			t.Errorf("orientation %d reported valid", invalid)
		}
	}
}
