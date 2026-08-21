package panel

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
)

func giciskyPanel(t *testing.T, id uint16) Panel {
	t.Helper()
	profile, known := gicisky.LookupProfile(id, 0)
	if !known {
		t.Fatalf("no Gicisky profile 0x%04X", id)
	}
	return OfGicisky(profile)
}

func nrfepdPanel(t *testing.T, name string) Panel {
	t.Helper()
	model, known := nrfepd.LookupModelName(name)
	if !known {
		t.Fatalf("no EPD-nRF5 model %q", name)
	}
	return OfNRFEPD(model)
}

// filled is a document that covers the whole panel in one ink, which is enough
// to reach the encoder and, when the ink is red, enough to be refused by a
// panel that has no colour plane.
func filled(ink display.Ink) compose.Document {
	return compose.Document{Root: compose.Rectangle{
		Size: image.Pt(4, 4),
		Fill: compose.Ink(ink),
	}}
}

// Each family answers in its own shape, and Len covers both without being told
// which. A caller that reads the wrong pair of fields gets nothing rather than
// something wrong, so the shape is worth pinning.
func TestEachFamilyPacksIntoItsOwnShape(t *testing.T) {
	result, page, err := Render(filled(display.InkBlack), giciskyPanel(t, 0x0033))
	if err != nil {
		t.Fatalf("Gicisky render: %v", err)
	}
	if len(page.Bytes) == 0 || page.Black != nil || page.Colour != nil {
		t.Fatalf("Gicisky page = %d bytes, black=%d colour=%d, want one buffer",
			len(page.Bytes), len(page.Black), len(page.Colour))
	}
	if page.Len() != len(page.Bytes) {
		t.Errorf("Len = %d, want %d", page.Len(), len(page.Bytes))
	}
	if result.Frame.Width() != 296 || result.Frame.Height() != 128 {
		t.Errorf("frame = %dx%d, want 296x128", result.Frame.Width(), result.Frame.Height())
	}

	result, page, err = Render(filled(display.InkBlack), nrfepdPanel(t, "UC8176_420_BWR"))
	if err != nil {
		t.Fatalf("EPD-nRF5 render: %v", err)
	}
	if page.Bytes != nil || len(page.Black) == 0 || len(page.Colour) == 0 {
		t.Fatalf("EPD-nRF5 page = bytes=%d black=%d colour=%d, want two planes",
			len(page.Bytes), len(page.Black), len(page.Colour))
	}
	if page.Len() != len(page.Black)+len(page.Colour) {
		t.Errorf("Len = %d, want %d", page.Len(), len(page.Black)+len(page.Colour))
	}
	if result.Frame.Width() != 400 || result.Frame.Height() != 300 {
		t.Errorf("frame = %dx%d, want 400x300", result.Frame.Width(), result.Frame.Height())
	}
}

// A black and white panel has no plane to put red in, and both families say so
// rather than dropping the colour. What matters here is not the refusal but
// what comes back with it: the page drew, so the report exists, and the caller
// needs it to explain a failure that the picture cannot show.
func TestAPageThatDrawsAndWillNotPackKeepsItsReport(t *testing.T) {
	for _, target := range []Panel{
		giciskyPanel(t, 0x0028),
		nrfepdPanel(t, "UC8176_420_BW"),
	} {
		result, page, err := Render(filled(display.InkRed), target)
		if err == nil {
			t.Fatalf("%s accepted red ink", target)
		}
		if result.Frame == nil {
			t.Errorf("%s: the page drew and the frame was dropped, so there is no report to send", target)
		}
		if result.Report.Bounds.Empty() {
			t.Errorf("%s: report has no bounds", target)
		}
		if page.Len() != 0 {
			t.Errorf("%s: returned %d bytes beside the error", target, page.Len())
		}
	}
}

// The other direction, which is the one that tells a caller there is nothing
// to report: a layout that fails leaves no frame behind.
func TestALayoutThatFailsLeavesNoFrame(t *testing.T) {
	document := compose.Document{Orientation: display.Orientation(9)}
	result, _, err := Render(document, giciskyPanel(t, 0x0033))
	if err == nil {
		t.Fatal("an invalid orientation was accepted")
	}
	if result.Frame != nil {
		t.Errorf("frame = %v, want nil so the caller knows there is no report", result.Frame.Bounds())
	}
}

// The panel decides the size, not the document. A page written for one panel
// and sent to another is laid out again and said so, and that warning is the
// only thing standing between a caller and a page silently clipped.
func TestThePanelDecidesTheSize(t *testing.T) {
	document := filled(display.InkBlack)
	document.Size = image.Pt(296, 128)
	result, _, err := Render(document, nrfepdPanel(t, "UC8176_420_BWR"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Frame.Width() != 400 || result.Frame.Height() != 300 {
		t.Fatalf("frame = %dx%d, want the panel's 400x300", result.Frame.Width(), result.Frame.Height())
	}
	var warned bool
	for _, warning := range result.Report.Warnings {
		if warning.Code == "size-mismatch" {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want size-mismatch", result.Report.Warnings)
	}
}

// A Panel nobody constructed has no family, and the accessors read the Gicisky
// half of an empty struct rather than guessing. That is only safe because it
// ends in a refusal: a zero panel is nought by nought, which is not a size
// anything will lay out for.
func TestAPanelNobodyNamedIsRefusedRatherThanGuessed(t *testing.T) {
	var empty Panel
	if size := empty.Size(); size != (image.Point{}) {
		t.Fatalf("Size = %v, want the zero point", size)
	}
	_, _, err := Render(filled(display.InkBlack), empty)
	if err == nil {
		t.Fatal("a panel with no family was rendered for")
	}
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("error = %v, want it to name the size it would not accept", err)
	}
}

// Both families name a panel the same way, including the qualifier that says
// the entry was transcribed rather than checked. Eleven of the twelve Gicisky
// entries are unverified, so a name printed without it is a size claimed with
// more confidence than anybody here has.
func TestAnUnverifiedPanelSaysSoWhicheverFamilyItIs(t *testing.T) {
	verified := giciskyPanel(t, 0x0033)
	if got := verified.String(); !strings.Contains(got, "296x128") || strings.Contains(got, "unverified") {
		t.Errorf("String = %q, want the size and no qualifier", got)
	}
	for _, target := range []Panel{giciskyPanel(t, 0x0028), nrfepdPanel(t, "UC8176_420_BW")} {
		if got := target.String(); !strings.Contains(got, "(unverified)") {
			t.Errorf("String = %q, want it to say the entry is unverified", got)
		}
		if got, want := target.String(), target.Name(); !strings.Contains(got, want) {
			t.Errorf("String = %q, want it to contain the name %q", got, want)
		}
	}
}
