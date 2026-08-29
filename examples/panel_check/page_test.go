// Package panel_check holds the two pages to push at a panel nobody here has
// driven before.
//
// A model table says a panel is 400x300 and shows black, white and red. It
// does not say what that panel can actually be read at, and the two are not
// the same question. These pages answer the second one, which has to be
// answered by looking at the panel rather than by anything in this repository.
//
// primitives draws every shape the schema has, with every stroke at two pixels
// and one cell putting one pixel beside two so the difference has somewhere to
// show. polarity is the controlled one: the same font at the same size in both
// directions, light on dark beside dark on light, because on the panel this was
// written against those two are not equally legible and the gap is two whole
// size steps.
//
// What that panel came back with is in the README, under the model table. The
// numbers there are for one panel and are not claimed for any other, which is
// the reason these pages are kept rather than deleted once they had been read:
// the next panel is a different question with the same two pages.
//
// # These pages are 400x300
//
// That is the 4.2" part, so they render at a size the 2.9" Gicisky tags cannot
// take. There is no payload assertion below for that reason. They reach a panel
// through the EPD-nRF5 driver, which asks the panel what it is and builds the
// page for the answer:
//
//	./inkwire push -device NRF_EPD_C1F8 examples/panel_check/polarity.html
package panel_check

import (
	"image"
	"os"
	"regexp"
	"testing"

	"github.com/xwvike/inkwire/internal/testscene"
)

func pages() []string { return []string{"primitives", "polarity"} }

func TestPagesMatchTheirReferences(t *testing.T) {
	for _, name := range pages() {
		t.Run(name, func(t *testing.T) {
			result := testscene.RenderPage(t, ".", name)
			if size := result.Frame.Bounds().Size(); size != image.Pt(400, 300) {
				t.Fatalf("frame is %v, want 400x300", size)
			}
			// A page whose job is to show what a panel can draw must not be
			// clipping anything itself, or what it shows is this repository's
			// mistake rather than the panel's limit.
			if len(result.Report.Warnings) != 0 {
				t.Errorf("warnings: %v", result.Report.Warnings)
			}
			if len(result.Report.MissingRunes) != 0 {
				t.Errorf("missing runes: %v", result.Report.MissingRunes)
			}
			testscene.AssertMatchesPNG(t, name+".png", result.Frame)
		})
	}
}

// Every stroke on the primitives page is two pixels, which is the width the
// panel it was written for needs. A one pixel stroke slipping back in would
// make the page quietly stop testing the thing it exists to test, and the one
// place a single pixel belongs is the cell that compares the two.
//
// The arc is the third width, at three, because a curve a pixel thinner than
// the straight edges beside it reads as thinner still.
func TestEveryStrokeIsTwoPixelsExceptWhereTheComparisonNeedsOne(t *testing.T) {
	page, err := os.ReadFile("primitives.html")
	if err != nil {
		t.Fatal(err)
	}
	widths := regexp.MustCompile(`stroke-width="(\d+)"`).FindAllStringSubmatch(string(page), -1)
	if len(widths) < 10 {
		t.Fatalf("found %d strokes, which cannot be right", len(widths))
	}
	ones := 0
	for _, match := range widths {
		switch match[1] {
		case "1":
			ones++
		case "0":
			t.Errorf("a stroke of width %s", match[1])
		}
	}
	// Exactly the two lines in the comparison cell, one black and one red.
	if ones != 2 {
		t.Errorf("%d strokes are one pixel wide, want the 2 in the comparison cell", ones)
	}
}
