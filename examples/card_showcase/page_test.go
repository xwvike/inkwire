package card_showcase

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

//go:embed page.json
var pageJSON []byte

func TestPageMatchesReference(t *testing.T) {
	result := renderPage(t)
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "card_showcase.png", result.Frame)
}

func TestPageUsesAllThreeInksWithoutRedInPortrait(t *testing.T) {
	frame := renderPage(t).Frame
	counts := map[display.Ink]int{}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			counts[ink]++
		}
	}
	for ink, name := range map[display.Ink]string{display.InkBlack: "black", display.InkWhite: "white", display.InkRed: "red"} {
		if counts[ink] < 100 {
			t.Errorf("%s covers only %d pixels", name, counts[ink])
		}
	}
	red := 0
	for y := 45; y < 90; y++ {
		for x := 30; x < 74; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkRed {
				red++
			}
		}
	}
	if red != 0 {
		t.Errorf("%d red pixels inside portrait", red)
	}
}

func renderPage(t *testing.T) scene.Result {
	t.Helper()
	result, err := (scene.Decoder{BaseDir: "."}).Render(bytes.NewReader(pageJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Report.MissingRunes) != 0 || len(result.Report.Warnings) != 0 {
		t.Fatalf("report: missing=%q warnings=%v", string(result.Report.MissingRunes), result.Report.Warnings)
	}
	return result
}
