package state_showcase

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
	testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
	testscene.AssertMatchesPNG(t, "state_showcase.png", result.Frame)
}
