package display

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestAPictureMadeSmallerIsTheAverageOfWhatIsBehindEachPixel(t *testing.T) {
	// One-pixel stripes, ten of them behind each pixel drawn: chosen at a
	// point they come out all black or all white, and averaged they are the
	// grey they look like from a distance.
	stripes := image.NewGray(image.Rect(0, 0, 100, 10))
	for x := 0; x < 100; x++ {
		for y := 0; y < 10; y++ {
			if x%2 == 0 {
				stripes.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	target := image.Rect(0, 0, 10, 1)
	sampler := newAreaSampler(stripes, fitImage(stripes.Bounds(), target, FitStretch), target)
	for x := 0; x < 10; x++ {
		if got := sampler.at(x, 0); math.Abs(got.r-127.5) > 0.01 || got.a != 1 {
			t.Errorf("pixel %d averaged %+v; want half white", x, got)
		}
	}
	// A step that is not a whole number of pixels takes the part of the
	// pixel at each end of the box that is behind it.
	ramp := image.NewGray(image.Rect(0, 0, 3, 1))
	for x, value := range []uint8{0, 90, 180} {
		ramp.SetGray(x, 0, color.Gray{Y: value})
	}
	target = image.Rect(0, 0, 2, 1)
	sampler = newAreaSampler(ramp, fitImage(ramp.Bounds(), target, FitStretch), target)
	for x, want := range []float64{30, 150} {
		if got := sampler.at(x, 0).r; math.Abs(got-want) > 0.01 {
			t.Errorf("pixel %d of the ramp averaged %.2f; want %.0f", x, got, want)
		}
	}
	// Along an axis the picture is made larger on, the box is a pixel wide,
	// which is the blend bilinear makes, and at the edge it is what of the
	// box is over the picture.
	rows := image.NewGray(image.Rect(0, 0, 100, 2))
	for x := 0; x < 100; x++ {
		rows.SetGray(x, 1, color.Gray{Y: 255})
	}
	target = image.Rect(0, 0, 10, 8)
	sampler = newAreaSampler(rows, fitImage(rows.Bounds(), target, FitStretch), target)
	for y, want := range map[int]float64{0: 0, 3: 255 * 0.375, 7: 255} {
		if got := sampler.at(0, y).r; math.Abs(got-want) > 0.01 {
			t.Errorf("row %d of two rows made eight averaged %.2f; want %.2f", y, got, want)
		}
	}
	// Only the part of the picture inside the crop is averaged.
	wide := image.NewGray(image.Rect(0, 0, 40, 10))
	for x := 0; x < 40; x++ {
		for y := 0; y < 10; y++ {
			if x < 15 || x >= 25 {
				wide.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	target = image.Rect(0, 0, 2, 2)
	sampler = newAreaSampler(wide, fitImage(wide.Bounds(), target, FitCover), target)
	for x := 0; x < 2; x++ {
		if got := sampler.at(x, 0).r; got != 0 {
			t.Errorf("the cover crop's pixel %d averaged %.2f from outside the crop; want black", x, got)
		}
	}
}

func TestAveragingIsNotTintedByWhatCannotBeSeen(t *testing.T) {
	// A transparent pixel's colour is not seen, so it does not tint the
	// opaque one beside it.
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	picture.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 0})
	picture.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	target := image.Rect(0, 0, 1, 1)
	got := newAreaSampler(picture, fitImage(picture.Bounds(), target, FitStretch), target).at(0, 0)
	if got.r != 255 || got.g != 255 || got.b != 255 || got.a != 0.5 {
		t.Errorf("white beside clear red averaged %+v; want white at half alpha", got)
	}
	// Nothing at all behind it is clear.
	clear := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	if got := newAreaSampler(clear, fitImage(clear.Bounds(), target, FitStretch), target).at(0, 0); got != (sampledColor{}) {
		t.Errorf("a clear picture averaged %+v", got)
	}
}

func TestEachLayoutIsReadAsAtReadsIt(t *testing.T) {
	bounds := image.Rect(1, 2, 9, 8)
	rgba, nrgba, gray := image.NewRGBA(bounds), image.NewNRGBA(bounds), image.NewGray(bounds)
	ycbcr := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	paletted := image.NewPaletted(bounds, color.Palette{color.Black, color.White, color.NRGBA{R: 200, A: 100}})
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			value := color.NRGBA{R: uint8(x * 30), G: uint8(y * 40), B: uint8(x * y), A: uint8(55 + x*20)}
			rgba.Set(x, y, value)
			nrgba.Set(x, y, value)
			gray.Set(x, y, value)
			paletted.SetColorIndex(x, y, uint8((x+y)%3))
			ycbcr.Y[ycbcr.YOffset(x, y)] = uint8(x * 25)
			ycbcr.Cb[ycbcr.COffset(x, y)] = uint8(100 + y*10)
			ycbcr.Cr[ycbcr.COffset(x, y)] = uint8(90 + x*12)
		}
	}
	for name, picture := range map[string]image.Image{"rgba": rgba, "nrgba": nrgba, "gray": gray, "ycbcr": ycbcr, "paletted": paletted} {
		read := premultipliedOf(picture)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := picture.At(x, y).RGBA()
				gr, gg, gb, ga := read(x, y)
				// NRGBA is premultiplied here in floating point, which is
				// what averaging wants; At rounds it to a 16-bit integer.
				for _, pair := range [][2]float64{{gr, float64(r) / 257}, {gg, float64(g) / 257}, {gb, float64(b) / 257}, {ga, float64(a) / 257}} {
					if math.Abs(pair[0]-pair[1]) > 1 {
						t.Fatalf("%s at %d,%d read %v %v %v %v; At reads %v %v %v %v", name, x, y, gr, gg, gb, ga, r>>8, g>>8, b>>8, a>>8)
					}
				}
			}
		}
	}
}

func TestSmoothSamplingAveragesWhenItMakesAPictureSmaller(t *testing.T) {
	// Two white pixels to every black one is a light grey, and cut at the
	// middle it is white; picked at points it is whichever stripe is hit.
	stripes := image.NewGray(image.Rect(0, 0, 300, 3))
	for x := 0; x < 300; x++ {
		for y := 0; y < 3; y++ {
			if x%3 != 0 {
				stripes.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	draw := func(sampling SamplingMode) int {
		frame, err := NewFrame(30, 1, InkWhite)
		if err != nil {
			t.Fatal(err)
		}
		if err := NewCanvas(frame).DrawImage(stripes, image.Rect(0, 0, 30, 1), ImageOptions{Sampling: sampling}); err != nil {
			t.Fatal(err)
		}
		return inkIn(frame, frame.Bounds())
	}
	if black := draw(SampleBilinear); black != 0 {
		t.Errorf("smoothed stripes drew %d black pixels; want the white their average is", black)
	}
	if black := draw(SampleNearest); black == 0 {
		t.Error("nearest sampling drew no black; it picks a pixel rather than averaging")
	}
}

func BenchmarkDrawingALargePhotoSmall(b *testing.B) {
	photo := image.NewYCbCr(image.Rect(0, 0, 4000, 3000), image.YCbCrSubsampleRatio420)
	for index := range photo.Y {
		photo.Y[index] = uint8(index * 7)
	}
	frame, err := NewFrame(296, 128, InkWhite)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := NewCanvas(frame).DrawImage(photo, frame.Bounds(), ImageOptions{Sampling: SampleBilinear, Fit: FitCover}); err != nil {
			b.Fatal(err)
		}
	}
}
