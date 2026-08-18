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

func TestRenderForSizeRejectsExplicitSizeMismatch(t *testing.T) {
	_, err := RenderForSize(compose.Document{Size: image.Pt(296, 128)}, image.Pt(400, 300))
	if err == nil || !strings.Contains(err.Error(), "scene declares size 296x128 but target page is 400x300") {
		t.Fatalf("mismatch error = %v", err)
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
