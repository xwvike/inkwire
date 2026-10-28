package markup

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// replacedFrame draws a page whose img is a solid black picture, so that
// black on the panel is the picture and nothing else.
func replacedFrame(t *testing.T, css string) *display.Frame {
	t.Helper()
	directory := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			source.Set(x, y, color.Black)
		}
	}
	file, err := os.Create(filepath.Join(directory, "p.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	file.Close()
	page, err := Compile(`<div class="page"><img src="p.png"></div>`,
		`.page { width: 40px; height: 40px; background: white; }
		 img { display: block; width: 20px; height: 20px; object-fit: fill; `+css+` }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Logf("  warning: %s", warning.Message)
	}
	frame, _ := renderDocument(t, directory+string(filepath.Separator), page.JSON)
	return frame
}

func whereInk(frame *display.Frame, want display.Ink) image.Rectangle {
	var found image.Rectangle
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == want {
				found = found.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return found
}

// A picture's box is drawn as any other: its border round it, and its
// padding between the two, rather than the picture stretched over both.
func TestAPictureIsDrawnInsideItsPaddingAndBorder(t *testing.T) {
	frame := replacedFrame(t, `border: 2px solid red; padding: 3px; box-sizing: content-box;`)
	if got, want := whereInk(frame, display.InkRed), image.Rect(0, 0, 30, 30); got != want {
		t.Errorf("the border is at %v; want %v", got, want)
	}
	if got, want := whereInk(frame, display.InkBlack), image.Rect(5, 5, 25, 25); got != want {
		t.Errorf("the picture is at %v; want %v, inside the border and padding", got, want)
	}
	// And its background shows where the picture does not reach.
	frame = replacedFrame(t, `padding: 2px; background: red; box-sizing: content-box;`)
	if got, want := whereInk(frame, display.InkRed), image.Rect(0, 0, 24, 24); got != want {
		t.Errorf("the background is at %v; want %v", got, want)
	}
}

// Rounded corners round the picture too, whatever overflow says: that is how
// border-radius: 50% makes a round portrait.
func TestARoundedPictureIsCutToItsCorners(t *testing.T) {
	frame := replacedFrame(t, `border-radius: 50%;`)
	if ink, _ := frame.InkAt(0, 0); ink != display.InkWhite {
		t.Errorf("the corner of a round picture is %v; want it cut away", ink)
	}
	if ink, _ := frame.InkAt(10, 10); ink != display.InkBlack {
		t.Errorf("the middle of a round picture is %v; want the picture", ink)
	}
}
