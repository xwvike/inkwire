package panel

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/tag"
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

// A panel without the ink a page asked for draws it black and says so. The
// refusal this replaced had a reason — losing a colour is not something the
// picture shows you afterwards — but that reason is about it happening
// silently, and a warning is what stops it being silent. The caller gets the
// page and the knowledge instead of neither.
//
// Both families answer the same way. They did not: gicisky refused yellow on a
// BWR panel and EPD-nRF5 quietly drew it black, so the same mistake was an
// error on one tag and invisible on the other.
func TestAnInkThePanelHasNoPlaceForIsDrawnBlackAndReported(t *testing.T) {
	for _, target := range []Panel{
		giciskyPanel(t, 0x0028),          // BW
		nrfepdPanel(t, "UC8176_420_BW"),  // BW
		giciskyPanel(t, 0x004B),          // BWR, no yellow
		nrfepdPanel(t, "UC8176_420_BWR"), // BWR, no yellow
	} {
		ink := display.InkYellow
		if target.palette() == "BW" {
			ink = display.InkRed
		}
		result, page, err := Render(filled(ink), target)
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		if len(page.Flattened) == 0 {
			t.Errorf("%s: %s went through unreported", target, ink)
		}
		var warned bool
		for _, warning := range result.Report.Warnings {
			if warning.Code == "unsupported-ink" && strings.Contains(warning.Message, ink.String()) {
				warned = true
			}
		}
		if !warned {
			t.Errorf("%s: warnings = %v, want one naming %s", target, result.Report.Warnings, ink)
		}
		// The preview has to show what the panel will show, not what the scene
		// asked for, or the picture and the tag disagree about the same page.
		if got, _ := result.Frame.InkAt(1, 1); got != display.InkBlack {
			t.Errorf("%s: preview pixel = %s, want black", target, got)
		}
	}
}

// A panel that has the ink is left alone, which is the half that says the
// flattening is looking at the palette rather than at every coloured pixel.
func TestAPanelThatHasTheInkIsLeftAlone(t *testing.T) {
	for _, target := range []Panel{
		giciskyPanel(t, 0x002E),          // BWRY
		nrfepdPanel(t, "UC8176_420_BWR"), // BWR
	} {
		ink := display.InkYellow
		if target.palette() == "BWR" {
			ink = display.InkRed
		}
		result, page, err := Render(filled(ink), target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if len(page.Flattened) != 0 {
			t.Errorf("%s: flattened %v, and it can show %s", target, page.Flattened, ink)
		}
		if len(result.Report.Warnings) != 0 {
			t.Errorf("%s: warnings = %v, want none", target, result.Report.Warnings)
		}
		if got, _ := result.Frame.InkAt(1, 1); got != ink {
			t.Errorf("%s: preview pixel = %s, want %s", target, got, ink)
		}
	}
}

// Below the seam the two encoders still refuse, and refuse alike. panel is what
// decides that a page is worth having anyway; an encoder handed an ink the
// panel has no plane for has been given something nobody checked, and drawing
// it as some other colour is the one thing it must not do quietly.
func TestBothEncodersRefuseAnInkTheyWereHandedDirectly(t *testing.T) {
	for _, target := range []Panel{
		giciskyPanel(t, 0x004B),
		nrfepdPanel(t, "UC8176_420_BWR"),
	} {
		size := target.Size()
		frame, err := display.NewFrame(size.X, size.Y, display.InkWhite)
		if err != nil {
			t.Fatal(err)
		}
		frame.Set(2, 2, display.InkYellow)
		if _, err := target.pack(frame, display.OrientationLandscape); err == nil {
			t.Errorf("%s: the encoder accepted yellow", target)
		} else if !strings.Contains(err.Error(), "yellow") {
			t.Errorf("%s: error = %v, want it to name the ink", target, err)
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

// The join has to reach both catalogues, which is the thing that was written
// out by hand at every place that wanted it and got one family or the other
// wrong. Counting is enough to catch a family dropped from the loop.
func TestTheCatalogueReachesBothFamilies(t *testing.T) {
	profiles, models := gicisky.KnownProfiles(), nrfepd.Models()
	if len(profiles) == 0 || len(models) == 0 {
		t.Fatalf("a family has no panels at all: %d Gicisky, %d EPD-nRF5", len(profiles), len(models))
	}
	all := All()
	if len(all) != len(profiles)+len(models) {
		t.Fatalf("All lists %d panels, want %d + %d", len(all), len(profiles), len(models))
	}
	seen := map[string]int{}
	for _, known := range all {
		seen[known.Family]++
		if known.Size().X <= 0 || known.Size().Y <= 0 {
			t.Errorf("%s (%s) has size %v", known, known.ID(), known.Size())
		}
	}
	if seen[tag.Gicisky] != len(profiles) || seen[tag.NRFEPD] != len(models) {
		t.Errorf("All is %v, want %d Gicisky and %d EPD-nRF5", seen, len(profiles), len(models))
	}
}

// Each family's id is written to its own width, because they are different
// numbers from different places and a byte padded to four digits reads as the
// other family's id. The READMEs' panel tables are matched against these
// strings, so the widths are load-bearing rather than cosmetic.
func TestAnIDIsWrittenToItsOwnFamilysWidth(t *testing.T) {
	if got := giciskyPanel(t, 0x0033).ID(); got != "0x0033" {
		t.Errorf("Gicisky ID = %q, want 0x0033", got)
	}
	model, known := nrfepd.LookupModelName("UC8176_420_BWR")
	if !known {
		t.Fatal("no EPD-nRF5 model UC8176_420_BWR")
	}
	if got, want := OfNRFEPD(model).ID(), fmt.Sprintf("0x%02x", model.ID); got != want {
		t.Errorf("EPD-nRF5 ID = %q, want %q", got, want)
	}
}

// The family has to be stated because the two catalogues are numbered
// independently: 0x0B is a panel to both firmwares and a different one to each.
// There is no shape to an id that says which family wrote it.
func TestAPanelAskedForByHandHasToNameItsFamily(t *testing.T) {
	if _, err := ByKey("0x0033"); err == nil {
		t.Fatal("an id with no family was accepted")
	}
	gicisky, err := ByKey("gicisky:0x0033")
	if err != nil {
		t.Fatalf("gicisky:0x0033: %v", err)
	}
	if gicisky.Size() != image.Pt(296, 128) {
		t.Errorf("gicisky:0x0033 = %v, want 296x128", gicisky.Size())
	}
	// The same digits, the other family, a different panel.
	if _, err := ByKey("nrfepd:0x0033"); err == nil {
		t.Error("0x0033 was accepted as an EPD-nRF5 id, which this build has no panel for")
	}
}

// An EPD-nRF5 panel can be asked for by the firmware's name because those
// names identify a panel. A Gicisky panel cannot, and this is why: two entries
// share a name and are different sizes, so accepting it would mean picking one
// of them for somebody without saying so.
func TestOnlyOneFamilysNamesIdentifyAPanel(t *testing.T) {
	byName, err := ByKey("nrfepd:UC8176_420_BWR")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	byID, err := ByKey("nrfepd:" + fmt.Sprintf("0x%02x", byName.NRFEPD.ID))
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if byName != byID {
		t.Errorf("name gave %s and id gave %s", byName, byID)
	}

	names := map[string][]string{}
	for _, known := range All() {
		if known.Family == tag.Gicisky {
			names[known.Name()] = append(names[known.Name()], known.ID())
		}
	}
	var shared bool
	for _, ids := range names {
		if len(ids) > 1 {
			shared = true
		}
	}
	if !shared {
		t.Skip("no Gicisky name is shared any more, so the id-only rule could be relaxed")
	}
	if _, err := ByKey(`gicisky:EPD 2.1" BWR`); err == nil {
		t.Error("a Gicisky name that two panels share was accepted")
	}
}

// A size is not a panel and must not quietly become one. Nothing about 400x300
// says which inks are available, so a page laid out at a size gets no ink check
// and should not look as though it had one.
func TestASizeIsReadWithoutBecomingAPanel(t *testing.T) {
	if got, err := ParseSize("400x300"); err != nil || got != image.Pt(400, 300) {
		t.Fatalf("ParseSize(400x300) = %v, %v", got, err)
	}
	if got, err := ParseSize(" 296 X 128 "); err != nil || got != image.Pt(296, 128) {
		t.Errorf("ParseSize with spaces and capitals = %v, %v", got, err)
	}
	for _, text := range []string{"", "400", "400x", "x300", "0x300", "400x-1", "four hundred"} {
		if _, err := ParseSize(text); err == nil {
			t.Errorf("ParseSize(%q) was accepted", text)
		}
	}
}

// The nibble-packed panels are refused before anything is drawn for them.
// The driver refuses them too, but only ever with a tag on the other end of a
// connection, and a page rendered for one offline would otherwise come out
// looking like a page.
func TestAPanelThisBuildCannotPackIsRefusedWithNoTagPresent(t *testing.T) {
	known, err := ByKey("nrfepd:UC8159_750_LOW_BW")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	_, _, err = Render(filled(display.InkBlack), known)
	if err == nil {
		t.Fatal("a nibble-packed panel was rendered for")
	}
	if !strings.Contains(err.Error(), "two pixels to a byte") {
		t.Errorf("error = %v, want it to say why the panel cannot be packed", err)
	}
}
