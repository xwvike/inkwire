package testscene

import (
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/panel"
)

// AssertEncodesFor checks that a page still packs for the panel it was drawn
// for, by running the encoder that model actually uses.
//
// An example page is written for one tag, and the two ways it can stop suiting
// that tag are both invisible in a reference PNG: the page can be the wrong
// size, or it can use an ink the panel cannot show. Encoding is what notices
// either, which is why examples that write to nothing still encode.
//
// The id is the one `inkwire scan` prints under MODEL, so a failure names the
// model rather than a byte count.
func AssertEncodesFor(t *testing.T, id uint16, frame *display.Frame, orientation display.Orientation) []byte {
	t.Helper()
	profile, found := gicisky.LookupProfile(id, 0)
	if !found {
		t.Fatalf("no Gicisky profile 0x%04X", id)
	}
	return assertEncodes(t, panel.OfGicisky(profile), frame, orientation).Bytes
}

// AssertEncodesForNRFEPD is AssertEncodesFor for an EPD-nRF5 tag, whose models
// are named by the firmware rather than numbered by an advertisement. The name
// is the one the tag reports once connected, such as UC8176_420_BWR.
func AssertEncodesForNRFEPD(t *testing.T, name string, frame *display.Frame, orientation display.Orientation) []byte {
	t.Helper()
	model, found := nrfepd.LookupModelName(name)
	if !found {
		t.Fatalf("no EPD-nRF5 model %s", name)
	}
	page := assertEncodes(t, panel.OfNRFEPD(model), frame, orientation)
	return append(append([]byte(nil), page.Black...), page.Colour...)
}

// assertEncodes packs a frame and fails on anything the packing had to change.
//
// Encoding no longer refuses an ink the panel cannot show: it redraws it as
// black and says so in Flattened, because a person pushing a page would rather
// have the page. An example is not that person. A page written for one panel
// that leans on an ink the panel lacks has stopped suiting it, and reading
// Flattened is the only way that is still noticed.
func assertEncodes(t *testing.T, target panel.Panel, frame *display.Frame, orientation display.Orientation) panel.Page {
	t.Helper()
	page, err := target.Encode(frame, orientation)
	if err != nil {
		t.Fatalf("encode for %s (%s): %v", target, target.ID(), err)
	}
	if len(page.Flattened) != 0 {
		t.Errorf("%s (%s) cannot show %v, which encoding redrew as black", target, target.ID(), page.Flattened)
	}
	return page
}
