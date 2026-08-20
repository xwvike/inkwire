package scene

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

func TestRenderForSizeUsesTargetSizeWhenDocumentIsImplicit(t *testing.T) {
	result, err := RenderForSize(compose.Document{}, image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Frame.Bounds().Size(), image.Pt(400, 300); got != want {
		t.Fatalf("frame size = %v, want %v", got, want)
	}
}

func TestRenderForSizeMapsPortraitToLogicalPage(t *testing.T) {
	result, err := RenderForSize(compose.Document{Orientation: display.OrientationPortraitClockwise}, image.Pt(400, 300))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Frame.Bounds().Size(), image.Pt(300, 400); got != want {
		t.Fatalf("portrait frame size = %v, want %v", got, want)
	}
	if result.Orientation != display.OrientationPortraitClockwise {
		t.Fatalf("orientation = %v, want portrait clockwise", result.Orientation)
	}
}

// A page written for one panel and sent to another used to be refused. The
// caller has a tag in front of them and a page they want on it, so it is laid
// out again for the panel that is really there and the difference is reported.
func TestRenderForSizeLaysAMismatchedPageOutAgainAndSaysSo(t *testing.T) {
	result, err := RenderForSize(compose.Document{Size: image.Pt(296, 128)}, image.Pt(400, 300))
	if err != nil {
		t.Fatalf("a mismatched page was refused: %v", err)
	}
	if got, want := result.Frame.Bounds().Size(), image.Pt(400, 300); got != want {
		t.Fatalf("frame = %v, want the panel's %v rather than the scene's", got, want)
	}
	var warning *compose.Warning
	for i, w := range result.Report.Warnings {
		if w.Code == "size-mismatch" {
			warning = &result.Report.Warnings[i]
		}
	}
	if warning == nil {
		t.Fatalf("nothing warned about the mismatch: %+v", result.Report.Warnings)
	}
	for _, want := range []string{"296x128", "400x300", "clipped"} {
		if !strings.Contains(warning.Message, want) {
			t.Errorf("warning does not mention %q: %s", want, warning.Message)
		}
	}
}

// A page that says nothing about its size is not a mismatch, and a page that
// agrees with the panel is not one either. Warning about those would train
// everybody to ignore the warning.
func TestRenderForSizeIsQuietWhenThereIsNothingToSay(t *testing.T) {
	for _, document := range []compose.Document{
		{},
		{Size: image.Pt(400, 300)},
	} {
		result, err := RenderForSize(document, image.Pt(400, 300))
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range result.Report.Warnings {
			if w.Code == "size-mismatch" {
				t.Errorf("document %+v warned about a size that matches: %s", document, w.Message)
			}
		}
	}
}

func TestDecoderRenderForSize(t *testing.T) {
	result, err := (Decoder{}).RenderForSize(strings.NewReader(`{"version":1}`), image.Pt(250, 132))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Frame.Bounds().Size(), image.Pt(250, 132); got != want {
		t.Fatalf("frame size = %v, want %v", got, want)
	}
}
