// Package text_showcase is the page to look at when choosing a size: every
// strike this build carries, drawn at the size it will be drawn at.
//
// Its test is a reference comparison and nothing else, because there is
// nothing here to assert about the layout. What would go wrong is a font
// changing underneath the page, and a changed font is a changed picture.
package text_showcase

import (
	"testing"

	"github.com/xwvike/inkwire/internal/testscene"
)

func TestPageMatchesReference(t *testing.T) {
	result := testscene.RenderPage(t, ".", "page")
	if len(result.Report.MissingRunes) != 0 {
		t.Errorf("the fonts have no glyph for %q", string(result.Report.MissingRunes))
	}
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "text_showcase.png", result.Frame)
}
