package compose

import (
	"image"
	"image/color"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func newTestFrame(t *testing.T, width, height int) *display.Frame {
	t.Helper()
	frame, err := display.NewFrame(width, height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func countInk(frame *display.Frame, target display.Ink) int {
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == target {
				count++
			}
		}
	}
	return count
}

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
	if got := artProfile.SuggestOptions().Dither; got != display.DitherThreshold {
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
	if got := gradientProfile.SuggestOptions().Dither; got != display.DitherFloydSteinberg {
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

	ink := func(options display.ImageOptions) int {
		frame := newTestFrame(t, 48, 48)
		if err := display.NewCanvas(frame).DrawImage(light, frame.Bounds(), options); err != nil {
			t.Fatal(err)
		}
		return countInk(frame, display.InkBlack)
	}
	if got := ink(display.ImageOptions{Dither: display.DitherThreshold}); got != 0 {
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
	if err := display.NewCanvas(before).DrawImage(source, before.Bounds(), display.ImageOptions{Dither: display.DitherThreshold}); err != nil {
		t.Fatal(err)
	}
	if countInk(before, display.InkWhite) != 0 {
		t.Fatal("the unenhanced source already separates; the case proves nothing")
	}

	enhanced, err := EnhanceContrast(source, 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	after := newTestFrame(t, size, size)
	if err := display.NewCanvas(after).DrawImage(enhanced, after.Bounds(), display.ImageOptions{Dither: display.DitherThreshold}); err != nil {
		t.Fatal(err)
	}
	if countInk(after, display.InkWhite) == 0 {
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
	if err := display.NewCanvas(frame).DrawImage(enhanced, frame.Bounds(), display.ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if countInk(frame, display.InkRed) != 0 {
		t.Fatal("red survived the contrast pass, so its documented limit is wrong")
	}
}

// The failure this exists for: an icon whose shapes differ in hue and whose
// brightness straddles the cut, so some shapes land on the paper side and
// disappear. Measured against the paper instead, they are all ink.
func TestColourCarriesStructureIsDetectedAndAnswered(t *testing.T) {
	source := solidImage(60, 60, func(x, y int) color.NRGBA {
		switch {
		case y < 16:
			return color.NRGBA{R: 0xd8, G: 0x50, B: 0x40, A: 0xff} // vermilion
		case y < 32:
			return color.NRGBA{R: 0xf2, G: 0xe1, B: 0x4f, A: 0xff} // yellow
		case y < 48:
			return color.NRGBA{R: 0x5f, G: 0xe0, B: 0xe8, A: 0xff} // cyan
		default:
			return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // paper
		}
	})

	profile, err := ProfileImage(source)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Photographic {
		t.Fatal("flat wedges read as a photograph")
	}
	if !profile.ColourCarriesStructure {
		t.Fatalf("colour structure not detected; only %.1f%% counted as lost", 100*profile.LostToLuminance)
	}

	painted := func(img image.Image, options display.ImageOptions) int {
		frame := newTestFrame(t, 60, 60)
		if err := display.NewCanvas(frame).DrawImage(img, frame.Bounds(), options); err != nil {
			t.Fatal(err)
		}
		return 60*60 - countInk(frame, display.InkWhite)
	}
	byBrightness := painted(source, profile.SuggestOptions())

	toned, err := ToneByColourDistance(source)
	if err != nil {
		t.Fatal(err)
	}
	options, err := profile.SuggestOptionsFor(toned)
	if err != nil {
		t.Fatal(err)
	}
	byDistance := painted(toned, options)

	if byDistance <= byBrightness {
		t.Fatalf("colour distance painted %d pixels against brightness %d; it is meant to recover shapes, not lose them",
			byDistance, byBrightness)
	}
}

// A drawing that is dark where it is inked has nothing to gain here, and must
// not be dragged onto the other path.
func TestPlainArtworkDoesNotNeedColourDistance(t *testing.T) {
	source := solidImage(40, 40, func(x, y int) color.NRGBA {
		if x >= 10 && x < 30 {
			return color.NRGBA{A: 0xff}
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	})
	profile, err := ProfileImage(source)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ColourCarriesStructure {
		t.Errorf("black on white was sent down the colour path, losing %.1f%%", 100*profile.LostToLuminance)
	}
}

// Rewriting tone has emptied the red plane every other time; this pass keeps
// it, and keeps the original colour rather than substituting a saturated red,
// which would drag the luminance of those pixels to 54 and skew any cut
// measured from the result.
func TestToneByColourDistanceKeepsRed(t *testing.T) {
	// Paper has to be present for "the yellow becomes ink" to mean anything:
	// with only two colours the cut simply lands between them.
	source := solidImage(30, 30, func(x, y int) color.NRGBA {
		switch {
		case x < 10:
			return color.NRGBA{R: 0xd8, G: 0x50, B: 0x40, A: 0xff} // convincingly red
		case x < 20:
			return color.NRGBA{R: 0xf2, G: 0xe1, B: 0x4f, A: 0xff} // light yellow
		default:
			return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // paper
		}
	})
	profile, err := ProfileImage(source)
	if err != nil {
		t.Fatal(err)
	}
	toned, err := ToneByColourDistance(source)
	if err != nil {
		t.Fatal(err)
	}
	options, err := profile.SuggestOptionsFor(toned)
	if err != nil {
		t.Fatal(err)
	}
	frame := newTestFrame(t, 30, 30)
	if err := display.NewCanvas(frame).DrawImage(toned, frame.Bounds(), options); err != nil {
		t.Fatal(err)
	}
	if countInk(frame, display.InkRed) == 0 {
		t.Error("red was flattened away by the tone pass")
	}
	if countInk(frame, display.InkBlack) == 0 {
		t.Error("the light yellow half did not become ink")
	}
}

func TestToneByColourDistanceRejectsBadInput(t *testing.T) {
	if _, err := ToneByColourDistance(nil); err == nil {
		t.Fatal("ToneByColourDistance accepted a nil image")
	}
	if _, err := ToneByColourDistance(image.NewNRGBA(image.Rectangle{})); err == nil {
		t.Fatal("ToneByColourDistance accepted an empty image")
	}
}
