// Package state_showcase is a page about clipping and nesting rather than
// about content: four absolute frames, each clipped, with lines drawn past
// their edges on purpose. What it proves is that a clip is still a clip
// several levels down.
package state_showcase

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
	testscene.AssertMatchesPNG(t, "state_showcase.png", result.Frame)
}
