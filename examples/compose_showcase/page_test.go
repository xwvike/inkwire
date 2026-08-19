// Package compose_showcase is one page carrying an image through the automatic
// adaptations a document does not ask for: the tone decision, the contrast
// enhancement, the dither. Its test holds the report to exactly one image
// decision, so an adaptation that silently stops happening is a failure rather
// than a slightly different picture.
package compose_showcase

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

//go:embed page.json
var pageJSON []byte

func TestPageMatchesReference(t *testing.T) {
	result, err := (scene.Decoder{BaseDir: "."}).Render(bytes.NewReader(pageJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Report.MissingRunes) != 0 || len(result.Report.Warnings) != 0 {
		t.Fatalf("report: missing=%q warnings=%v", string(result.Report.MissingRunes), result.Report.Warnings)
	}
	if len(result.Report.Images) != 1 {
		t.Fatalf("image decisions = %d, want one", len(result.Report.Images))
	}
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "compose_showcase.png", result.Frame)
}
