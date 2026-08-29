// Package showcase is the one page to look at first: a single 296x128 document
// that uses the ordinary nodes — rows, text, rectangles, an image — at the
// density a real tag page is written at.
package showcase

import (
	"testing"

	"github.com/xwvike/inkwire/internal/testscene"
)

func TestPageMatchesReference(t *testing.T) {
	result := testscene.RenderPage(t, ".", "page")
	if len(result.Report.MissingRunes) != 0 || len(result.Report.Warnings) != 0 {
		t.Fatalf("report: missing=%q warnings=%v", string(result.Report.MissingRunes), result.Report.Warnings)
	}
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "showcase.png", result.Frame)
}
