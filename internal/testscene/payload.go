package testscene

import (
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
)

// AssertEncodesFor checks that a page still packs for the panel it was drawn
// for, by running the encoder that model actually uses.
//
// An example page is written for one tag, and the two ways it can stop suiting
// that tag are both invisible in a reference PNG: the page can be the wrong
// size, or it can use an ink the panel cannot show. Encoding is what notices
// either, which is why examples that write to nothing still encode.
//
// The id is the one `inkwire scan` prints and `inkwire encode -profile-id`
// takes, so a failure names the model rather than a byte count.
func AssertEncodesFor(t *testing.T, id uint16, frame *display.Frame, orientation display.Orientation) []byte {
	t.Helper()
	profile, known := gicisky.LookupProfile(id, 0)
	if !known {
		t.Fatalf("no Gicisky profile 0x%04X", id)
	}
	payload, err := gicisky.EncodeOriented(frame, orientation, profile)
	if err != nil {
		t.Fatalf("encode for %s (0x%04X): %v", profile.Model, id, err)
	}
	return payload
}
