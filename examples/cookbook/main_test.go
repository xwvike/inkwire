package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/testscene"
)

func sections(t *testing.T) []section {
	t.Helper()
	return []section{
		{"fills", fills},
		{"strokes", strokes},
		{"paths", paths},
		{"text", texts},
		{"state", state},
		{"images", images},
		{"displaylist", displayLists},
	}
}

// The cookbook is documentation that runs, so the thing worth testing is that
// it still runs: every section draws without error and produces a payload the
// panel would accept.
func TestEverySectionRendersAPanel(t *testing.T) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sections(t) {
		t.Run(s.name, func(t *testing.T) {
			frame, err := renderPanel(s, fonts)
			if err != nil {
				t.Fatal(err)
			}
			testscene.AssertEncodesFor(t, 0x0033, frame, display.OrientationLandscape)
			// A panel that came out blank would still pass everything above.
			if countInk(frame, display.InkWhite) == frame.Width()*frame.Height() {
				t.Fatal("the panel is blank")
			}
		})
	}
}

// notes refuses to write a line that would be clipped, which is the only thing
// keeping the explanations readable. Check the guard rather than trusting it.
func TestNotesRefusesToBeClipped(t *testing.T) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := display.NewFrame(panelWidth, panelHeight, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	canvas := display.NewCanvas(frame)

	if err := notes(canvas, fonts, "this line is well within the forty-seven"); err != nil {
		t.Fatalf("a line that fits was refused: %v", err)
	}
	if err := notes(canvas, fonts, "this line is far too long to fit across the panel and must be refused"); err == nil {
		t.Error("an over-long line was accepted and would have been clipped")
	}
	if err := notes(canvas, fonts, "one", "two", "three", "four"); err == nil {
		t.Error("a fourth line was accepted and would have fallen off the box")
	}
}

func TestRenderContactSheet(t *testing.T) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var frames []*display.Frame
	for _, s := range sections(t) {
		frame, err := renderPanel(s, fonts)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	sheet, err := stack(frames)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesReferencePNG(t, "cookbook.png", sheet)
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
