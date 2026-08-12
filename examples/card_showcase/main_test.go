package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestRenderCardShowcase(t *testing.T) {
	frame, err := renderCardShowcase()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width() != display.GiciskyWidth || frame.Height() != display.GiciskyHeight {
		t.Fatalf("card showcase dimensions = %dx%d", frame.Width(), frame.Height())
	}
	payload, err := display.EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("card showcase payload = %d bytes", len(payload))
	}
	assertMatchesReferencePNG(t, "card_showcase.png", frame)
}

// The card is the only example that puts all three inks on the panel, and the
// red comes out of the artwork rather than being drawn: the mascot's mouth is
// warm enough to pass the red test while its outlines and face do not.
func TestCardUsesAllThreeInks(t *testing.T) {
	frame, err := renderCardShowcase()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[display.Ink]int{}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			counts[ink]++
		}
	}
	for ink, name := range map[display.Ink]string{
		display.InkBlack: "black", display.InkWhite: "white", display.InkRed: "red",
	} {
		if counts[ink] < 100 {
			t.Errorf("%s covers only %d pixels, the card should use all three inks", name, counts[ink])
		}
	}

	// Red inside the portrait can only have come from the image itself.
	portrait := 0
	for y := 40; y < 95; y++ {
		for x := 25; x < 80; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkRed {
				portrait++
			}
		}
	}
	if portrait == 0 {
		t.Error("no red inside the portrait: the mouth should reach the red plane")
	}
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
