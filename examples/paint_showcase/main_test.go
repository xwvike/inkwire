package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// The dash, clip and pattern behaviour this example illustrates is verified
// properly in internal/display; what this test adds is that the illustration
// itself still renders and still matches what was checked by eye.
func TestRenderPaintShowcase(t *testing.T) {
	frame, err := renderPaintShowcase()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width() != display.GiciskyWidth || frame.Height() != display.GiciskyHeight {
		t.Fatalf("paint showcase dimensions = %dx%d", frame.Width(), frame.Height())
	}
	payload, err := display.EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("paint showcase payload = %d bytes", len(payload))
	}
	assertMatchesReferencePNG(t, "paint_showcase.png", frame)
}

// assertMatchesReferencePNG compares decoded pixels instead of encoded bytes so
// that a change in the standard library's PNG encoder cannot fail the test, and
// so that a real rendering change reports the first differing coordinate.
func assertMatchesReferencePNG(t *testing.T, path string, frame *display.Frame) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reference, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if reference.Bounds() != frame.Bounds() {
		t.Fatalf("reference %s is %v, frame is %v", path, reference.Bounds(), frame.Bounds())
	}
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			want := color.NRGBAModel.Convert(reference.At(x, y))
			if got := color.NRGBAModel.Convert(frame.At(x, y)); got != want {
				t.Fatalf("pixel (%d,%d) = %v, reference %s has %v", x, y, got, path, want)
			}
		}
	}
}
