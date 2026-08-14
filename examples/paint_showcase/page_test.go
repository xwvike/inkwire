// Package paint_showcase draws clipping, patterns and dashes, and is also
// where the layout nodes added later had to prove they change nothing.
//
// The page was written before grid, anchored, clip and clipShape existed, so
// it said the same things the long way round: three panels placed at rectangles
// somebody worked out by hand, each repeating its own size to draw its border,
// and a rounded window cut with four arcs whose coordinates were counted out
// one corner at a time. All of that is now what the nodes are for, and none of
// it moved a pixel, which is the point of rewriting this page rather than
// writing a new one. A node that cannot reproduce what the long way round
// produced is not a shorthand for it.
//
// The reference image is untouched. Off-by-one changes to the column gap, the
// rounded corner, the circle's radius and either anchor inset were each checked
// to fail against it.
//
// transformed is the one new node that is not here. Nothing on this page is a
// turned or magnified copy of anything, and putting one in at scale one to say
// it had been used would be a lie in the shape of an example.
package paint_showcase

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
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
	payload, err := result.Payload()
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("payload = %d bytes", len(payload))
	}
	testscene.AssertMatchesPNG(t, "paint_showcase.png", result.Frame)
}
