package display

import "testing"

func TestNewPageUsesLogicalOrientationDimensions(t *testing.T) {
	landscape, err := NewPage(OrientationLandscape, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if landscape.Width() != 296 || landscape.Height() != 128 {
		t.Fatalf("landscape size = %dx%d", landscape.Width(), landscape.Height())
	}
	portrait, err := NewPage(OrientationPortraitClockwise, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if portrait.Width() != 128 || portrait.Height() != 296 {
		t.Fatalf("portrait size = %dx%d", portrait.Width(), portrait.Height())
	}
}
