// Package compose_showcase is one page carrying an image through the automatic
// adaptations a document does not ask for: the tone decision, the fit, the
// dither. Its test holds the report to exactly one image decision, so an
// adaptation that silently stops happening is a failure rather than a slightly
// different picture.
//
// The page used to ask for a local contrast pass as well, which it no longer
// can: a stylesheet has no word for it. The page keeps layout in CSS and leaves
// the pixel-level decision to the renderer, which reports it explicitly.
package compose_showcase

import (
	"testing"

	"github.com/xwvike/inkwire/internal/testscene"
)

func TestPageMatchesReference(t *testing.T) {
	result := testscene.RenderPage(t, ".", "page")
	if len(result.Report.MissingRunes) != 0 || len(result.Report.Warnings) != 0 {
		t.Fatalf("report: missing=%q warnings=%v", string(result.Report.MissingRunes), result.Report.Warnings)
	}
	if len(result.Report.Images) != 1 {
		t.Fatalf("image decisions = %d, want one", len(result.Report.Images))
	}
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "compose_showcase.png", result.Frame)
}
