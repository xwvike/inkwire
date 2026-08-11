package display

import (
	"image"
	"image/color"
	"testing"
)

func TestDrawImageContainPreservesAspectRatio(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	for x := 0; x < 2; x++ {
		source.SetNRGBA(x, 0, color.NRGBA{A: 0xff})
	}
	frame, err := NewFrame(4, 4, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCanvas(frame).DrawImage(source, frame.Bounds(), ImageOptions{Fit: FitContain}); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 1, 0, InkWhite)
	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 1, 2, InkBlack)
	assertInk(t, frame, 1, 3, InkWhite)
}

func TestDrawImageDetectsRedBeforeGrayscale(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	source.SetNRGBA(1, 0, color.NRGBA{R: 100, G: 100, B: 100, A: 0xff})
	source.SetNRGBA(2, 0, color.NRGBA{R: 220, G: 220, B: 220, A: 0xff})
	frame, err := NewFrame(3, 1, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCanvas(frame).DrawImage(source, frame.Bounds(), ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 0, 0, InkRed)
	assertInk(t, frame, 1, 0, InkBlack)
	assertInk(t, frame, 2, 0, InkWhite)
}

func TestDrawImageFloydSteinbergProducesBothTones(t *testing.T) {
	source := image.NewGray(image.Rect(0, 0, 16, 2))
	for i := range source.Pix {
		source.Pix[i] = 128
	}
	frame, err := NewFrame(16, 2, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCanvas(frame).DrawImage(source, frame.Bounds(), ImageOptions{Dither: DitherFloydSteinberg}); err != nil {
		t.Fatal(err)
	}
	if black, white := countInk(frame, InkBlack), countInk(frame, InkWhite); black == 0 || white == 0 {
		t.Fatalf("Floyd-Steinberg rendered black=%d white=%d", black, white)
	}
}

func TestDrawImageCoverCropsSourceCenter(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		source.SetNRGBA(0, y, color.NRGBA{R: 0xff, A: 0xff})
		source.SetNRGBA(1, y, color.NRGBA{A: 0xff})
		source.SetNRGBA(2, y, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
		source.SetNRGBA(3, y, color.NRGBA{R: 0xff, A: 0xff})
	}
	frame, err := NewFrame(2, 2, InkRed)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCanvas(frame).DrawImage(source, frame.Bounds(), ImageOptions{Fit: FitCover}); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 0, 0, InkBlack)
	assertInk(t, frame, 1, 0, InkWhite)
}
