package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestInlineFormattingKeepsAtomicBoxInTextFlow(t *testing.T) {
	page, err := Compile(
		`<div class="flow">before <span class="badge">box</span> after</div>`,
		`.flow { display: block; width: 100px; height: 30px; font-family: monaco; font-size: 10px; line-height: 14px; }
		 .badge { display: inline-block; padding: 1px 2px; background: red; vertical-align: middle; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page.JSON), `"type": "inline"`) {
		t.Fatalf("mixed inline content did not emit an inline node: %s", page.JSON)
	}
	if len(page.Warnings) != 0 {
		t.Fatalf("warnings = %v", page.Warnings)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	var red image.Rectangle
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			if ink != display.InkRed {
				continue
			}
			pixel := image.Rect(x, y, x+1, y+1)
			if red.Empty() {
				red = pixel
			} else {
				red = red.Union(pixel)
			}
		}
	}
	if red.Empty() || red.Min.X == 0 || red.Dy() == 0 {
		t.Fatalf("inline atomic box bounds = %v; it was not painted after the leading text", red)
	}
}
