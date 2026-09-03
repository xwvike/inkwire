// Package markup_capabilities is the executable Markup capability atlas.
package markup_capabilities

import (
	"testing"

	"github.com/xwvike/inkwire/internal/testscene"
)

var pages = []string{"layout", "inline", "paint", "svg", "resources", "cascade", "potrace"}

func TestCapabilityPagesMatchReferences(t *testing.T) {
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			result := testscene.RenderPage(t, ".", page)
			if len(result.Report.MissingRunes) != 0 {
				t.Fatalf("missing runes: %q", string(result.Report.MissingRunes))
			}
			if len(result.Report.Warnings) != 0 {
				t.Fatalf("warnings: %v", result.Report.Warnings)
			}
			testscene.AssertMatchesPNG(t, page+".png", result.Frame)
		})
	}
}
