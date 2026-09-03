// Package claude_usage is a 296x128 operational usage snapshot.
package claude_usage

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/testscene"
)

func TestPageMatchesReference(t *testing.T) {
	result := testscene.RenderPage(t, ".", "page")
	if size := result.Frame.Bounds().Size(); size != image.Pt(296, 128) {
		t.Fatalf("frame is %v, want 296x128", size)
	}
	if len(result.Report.Warnings) != 0 {
		t.Errorf("warnings: %v", result.Report.Warnings)
	}
	if len(result.Report.MissingRunes) != 0 {
		t.Errorf("missing runes: %q", string(result.Report.MissingRunes))
	}
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "claude_usage.png", result.Frame)
}

func TestPageUsesThePanelInks(t *testing.T) {
	frame := testscene.RenderPage(t, ".", "page").Frame
	counts := map[display.Ink]int{}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			counts[ink]++
		}
	}
	for ink, name := range map[display.Ink]string{
		display.InkBlack: "black",
		display.InkRed:   "red",
	} {
		if counts[ink] < 100 {
			t.Errorf("%s covers only %d pixels", name, counts[ink])
		}
	}
}
