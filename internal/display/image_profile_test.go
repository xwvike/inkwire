package display

import (
	"image"
	"image/color"
	"testing"
)

func solidImage(width, height int, shade func(x, y int) color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, shade(x, y))
		}
	}
	return img
}

func TestProfileImageRejectsBadInput(t *testing.T) {
	if _, err := ProfileImage(nil); err == nil {
		t.Fatal("ProfileImage accepted a nil image")
	}
	if _, err := ProfileImage(image.NewNRGBA(image.Rectangle{})); err == nil {
		t.Fatal("ProfileImage accepted an empty image")
	}
}

// Flat artwork has almost nothing between black and white; a gradient is
// almost nothing but.
func TestProfileSeparatesFlatArtworkFromContinuousTone(t *testing.T) {
	art := solidImage(64, 64, func(x, y int) color.NRGBA {
		if (x/8+y/8)%2 == 0 {
			return color.NRGBA{A: 0xff}
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	})
	artProfile, err := ProfileImage(art)
	if err != nil {
		t.Fatal(err)
	}
	if artProfile.Photographic {
		t.Errorf("flat artwork read as photographic at %.2f mid-tone", artProfile.MidToneFraction)
	}
	if got := artProfile.SuggestOptions().Dither; got != DitherThreshold {
		t.Errorf("flat artwork suggested dither %d, want threshold", got)
	}

	gradient := solidImage(64, 64, func(x, y int) color.NRGBA {
		level := uint8(48 + x*2)
		return color.NRGBA{R: level, G: level, B: level, A: 0xff}
	})
	gradientProfile, err := ProfileImage(gradient)
	if err != nil {
		t.Fatal(err)
	}
	if !gradientProfile.Photographic {
		t.Errorf("a gradient read as flat artwork at %.2f mid-tone", gradientProfile.MidToneFraction)
	}
	if got := gradientProfile.SuggestOptions().Dither; got != DitherFloydSteinberg {
		t.Errorf("continuous tone suggested dither %d, want Floyd-Steinberg", got)
	}
}

// The failure a fixed cut at 128 causes: artwork that lives entirely above it
// thresholds away to nothing. Otsu cuts where this image actually divides.
func TestOtsuRescuesArtworkLighterThanTheFixedCut(t *testing.T) {
	light := solidImage(48, 48, func(x, y int) color.NRGBA {
		if x >= 16 && x < 32 {
			return color.NRGBA{R: 180, G: 180, B: 180, A: 0xff} // the mark
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	})
	profile, err := ProfileImage(light)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Threshold <= 128 || profile.Threshold >= 255 {
		t.Fatalf("Otsu threshold = %d, want a cut between the mark and the paper", profile.Threshold)
	}

	ink := func(options ImageOptions) int {
		frame := newTestFrame(t, 48, 48)
		if err := NewCanvas(frame).DrawImage(light, frame.Bounds(), options); err != nil {
			t.Fatal(err)
		}
		return countInk(frame, InkBlack)
	}
	if got := ink(ImageOptions{Dither: DitherThreshold}); got != 0 {
		t.Fatalf("the fixed cut painted %d pixels; this case exists because it paints none", got)
	}
	if got := ink(profile.SuggestOptions()); got == 0 {
		t.Fatal("the suggested options also erased the mark")
	}
}

// Otsu settles on an end of the range for an already two-tone image, where a
// zero would collide with ImageOptions reading zero as "unset".
func TestOtsuNeverReturnsAThresholdThatMeansUnset(t *testing.T) {
	bilevel := solidImage(32, 32, func(x, y int) color.NRGBA {
		if x < 16 {
			return color.NRGBA{A: 0xff}
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	})
	profile, err := ProfileImage(bilevel)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Threshold < 1 || profile.Threshold > 254 {
		t.Fatalf("Otsu threshold = %d, want it clamped into [1,254]", profile.Threshold)
	}
}

// Transparent pixels are measured as the white they will be drawn over, not as
// whatever colour the file happens to store beneath them.
func TestProfileFlattensAlphaAgainstWhite(t *testing.T) {
	hidden := solidImage(32, 32, func(x, y int) color.NRGBA {
		if x < 16 {
			return color.NRGBA{A: 0xff} // opaque black
		}
		return color.NRGBA{A: 0} // transparent, stored as black
	})
	profile, err := ProfileImage(hidden)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Photographic {
		t.Error("half black and half transparent read as photographic")
	}
	if profile.Threshold < 1 || profile.Threshold > 254 {
		t.Fatalf("Otsu threshold = %d on a two-tone image", profile.Threshold)
	}
	// Measured as black over white, the two halves are as far apart as possible.
	if profile.MidToneFraction > 0.05 {
		t.Errorf("mid-tone fraction = %.2f, want near zero once alpha is flattened", profile.MidToneFraction)
	}
}

func TestDisableRedKeepsTheRedPlaneEmpty(t *testing.T) {
	source := solidImage(8, 8, func(x, y int) color.NRGBA {
		return color.NRGBA{R: 0xff, G: 0x20, B: 0x20, A: 0xff}
	})
	for _, test := range []struct {
		name    string
		options ImageOptions
		wantRed bool
	}{
		{"red allowed", ImageOptions{}, true},
		{"red disabled", ImageOptions{DisableRed: true}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			frame := newTestFrame(t, 8, 8)
			if err := NewCanvas(frame).DrawImage(source, frame.Bounds(), test.options); err != nil {
				t.Fatal(err)
			}
			if got := countInk(frame, InkRed) > 0; got != test.wantRed {
				t.Fatalf("red present = %v, want %v", got, test.wantRed)
			}
		})
	}
}

func TestEnhanceContrastRejectsBadInput(t *testing.T) {
	if _, err := EnhanceContrast(nil, 4, 1); err == nil {
		t.Fatal("EnhanceContrast accepted a nil image")
	}
	if _, err := EnhanceContrast(image.NewNRGBA(image.Rectangle{}), 4, 1); err == nil {
		t.Fatal("EnhanceContrast accepted an empty image")
	}
	if _, err := EnhanceContrast(image.NewNRGBA(image.Rect(0, 0, 4, 4)), -1, 1); err == nil {
		t.Fatal("EnhanceContrast accepted a negative radius")
	}
}

// The case it exists for: a feature that is mid-tone against a dark field
// carries too little difference to survive one bit until it is amplified.
func TestEnhanceContrastLiftsDetailOutOfADarkField(t *testing.T) {
	const size, field, feature = 64, 40.0, 95.0
	source := solidImage(size, size, func(x, y int) color.NRGBA {
		level := uint8(field)
		if x >= 26 && x < 38 && y >= 26 && y < 38 {
			level = uint8(feature)
		}
		return color.NRGBA{R: level, G: level, B: level, A: 0xff}
	})

	// Both are dark, so both threshold to black and the feature is lost.
	before := newTestFrame(t, size, size)
	if err := NewCanvas(before).DrawImage(source, before.Bounds(), ImageOptions{Dither: DitherThreshold}); err != nil {
		t.Fatal(err)
	}
	if countInk(before, InkWhite) != 0 {
		t.Fatal("the unenhanced source already separates; the case proves nothing")
	}

	enhanced, err := EnhanceContrast(source, 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	after := newTestFrame(t, size, size)
	if err := NewCanvas(after).DrawImage(enhanced, after.Bounds(), ImageOptions{Dither: DitherThreshold}); err != nil {
		t.Fatal(err)
	}
	if countInk(after, InkWhite) == 0 {
		t.Fatal("the feature is still lost after the contrast pass")
	}
}

// Flat tone must come through unchanged: only differences are amplified.
func TestEnhanceContrastLeavesFlatToneAlone(t *testing.T) {
	source := solidImage(32, 32, func(x, y int) color.NRGBA {
		return color.NRGBA{R: 90, G: 90, B: 90, A: 0xff}
	})
	enhanced, err := EnhanceContrast(source, 6, 2.5)
	if err != nil {
		t.Fatal(err)
	}
	bounds := enhanced.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(enhanced.At(x, y)).(color.NRGBA)
			if c.R < 89 || c.R > 91 {
				t.Fatalf("flat tone at (%d,%d) moved to %d", x, y, c.R)
			}
		}
	}
}

// Sharpening works on luminance, so whatever colour was there is gone; a
// caller that needs both has to be told rather than surprised.
func TestEnhanceContrastDiscardsColour(t *testing.T) {
	source := solidImage(16, 16, func(x, y int) color.NRGBA {
		if x < 8 {
			return color.NRGBA{R: 0xff, G: 0x20, B: 0x20, A: 0xff}
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	})
	enhanced, err := EnhanceContrast(source, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	frame := newTestFrame(t, 16, 16)
	if err := NewCanvas(frame).DrawImage(enhanced, frame.Bounds(), ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if countInk(frame, InkRed) != 0 {
		t.Fatal("red survived the contrast pass, so its documented limit is wrong")
	}
}
