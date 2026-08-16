// Package claude_status is a status page for the 4.2" panel: how much of a
// usage window is left, when it resets, and how the two shares are running.
//
// It is written in Chinese throughout, which is the reason it is worth having
// as an example rather than a private page. Every glyph on it has to come from
// a bitmap font this repository carries, at a size that font actually has, and
// a page that asks for a rune the font lacks does not fail — it draws a gap and
// says so in the report. So the missing-rune check below is the one that would
// catch a page that looked fine to write and comes out with holes in it.
package claude_status

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

func renderPage(t *testing.T) scene.Result {
	t.Helper()
	result, err := (scene.Decoder{BaseDir: "."}).RenderFile("page.json")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPageMatchesReference(t *testing.T) {
	result := renderPage(t)
	// The 4.2" panel, not the 2.9" one the Gicisky driver writes to. This page
	// reaches a tag through the EPD-nRF5 driver, which asks the panel what it
	// is and builds for the answer, so there is no payload assertion here.
	if size := result.Frame.Bounds().Size(); size != image.Pt(400, 300) {
		t.Fatalf("frame is %v, want 400x300", size)
	}
	if len(result.Report.Warnings) != 0 {
		t.Errorf("warnings: %v", result.Report.Warnings)
	}
	testscene.AssertMatchesPNG(t, "page.png", result.Frame)
}

// A missing rune is drawn as a gap rather than refused, so nothing about the
// render says the page came out wrong. On a page that is almost entirely CJK,
// that is the failure worth naming separately from the reference comparison:
// the reference would drift with it if it were ever updated without looking.
func TestEveryGlyphOnThisPageExistsInItsFont(t *testing.T) {
	if missing := renderPage(t).Report.MissingRunes; len(missing) != 0 {
		t.Errorf("the fonts have no glyph for %q", string(missing))
	}
}
