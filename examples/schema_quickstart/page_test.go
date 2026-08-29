// Package schema_quickstart is the one example kept in both formats.
//
// page.html is what somebody writing a page today would write. page.json is
// what it compiles to, kept beside it because the schema is still the thing
// the panel is driven by and a reader following the documentation needs one
// document to read. internal/markup renders both and holds them to the same
// pixels, which is what makes the pair a translation rather than two pages
// that happen to look alike.
package schema_quickstart

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
	testscene.AssertMatchesPNG(t, "schema_quickstart.png", result.Frame)
}
