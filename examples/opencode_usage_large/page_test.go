// Package opencode_usage_large is a 400x300 opencode usage board for an EPD-nRF5 tag
// with a UC8176_420_BWR panel.
package opencode_usage_large

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/testscene"
)

func TestPageMatchesReference(t *testing.T) {
	result := testscene.RenderPage(t, ".", "page")
	if size := result.Frame.Bounds().Size(); size != image.Pt(400, 300) {
		t.Fatalf("frame is %v, want 400x300", size)
	}
	if len(result.Report.Warnings) != 0 {
		t.Errorf("warnings: %v", result.Report.Warnings)
	}
	if len(result.Report.MissingRunes) != 0 {
		t.Errorf("missing runes: %q", string(result.Report.MissingRunes))
	}
	testscene.AssertEncodesForNRFEPD(t, "UC8176_420_BWR", result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "opencode_usage_large.png", result.Frame)
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
