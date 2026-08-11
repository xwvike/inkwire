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

func TestLandscapeFrameRotatesPortraitInRequestedDirection(t *testing.T) {
	portrait, err := NewPage(OrientationPortraitClockwise, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	portrait.Set(0, 0, InkBlack)
	portrait.Set(127, 0, InkRed)

	clockwise, err := LandscapeFrame(portrait, OrientationPortraitClockwise)
	if err != nil {
		t.Fatal(err)
	}
	assertInk(t, clockwise, 295, 0, InkBlack)
	assertInk(t, clockwise, 295, 127, InkRed)

	counterClockwise, err := LandscapeFrame(portrait, OrientationPortraitCounterClockwise)
	if err != nil {
		t.Fatal(err)
	}
	assertInk(t, counterClockwise, 0, 127, InkBlack)
	assertInk(t, counterClockwise, 0, 0, InkRed)
}

func TestEncodeGiciskyOrientedAcceptsPortraitPage(t *testing.T) {
	portrait, err := NewPage(OrientationPortraitClockwise, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeGiciskyOriented(portrait, OrientationPortraitClockwise)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != GiciskyPayloadSize {
		t.Fatalf("payload size = %d, want %d", len(payload), GiciskyPayloadSize)
	}
}
