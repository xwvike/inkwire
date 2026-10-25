package scene

import (
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// A tone is written as the ink and its coverage in sixteenths, the spelling
// the markup compiler writes and Ink.String prints, so a document read back
// says what it was written as.
func TestAToneIsReadInSixteenths(t *testing.T) {
	for value, want := range map[string]display.Ink{
		"black 8/16":   display.Tone(display.InkBlack, 8),
		"red 1/16":     display.Tone(display.InkRed, 1),
		"yellow 15/16": display.Tone(display.InkYellow, 15),
		"black 0/16":   display.InkWhite,
		"black 16/16":  display.InkBlack,
	} {
		got, err := parseInk(value)
		if err != nil {
			t.Errorf("%q: %v", value, err)
			continue
		}
		if got != want {
			t.Errorf("%q is %s, want %s", value, got, want)
		}
		if _, _, toned := want.Toned(); toned && got.String() != value {
			t.Errorf("%q prints back as %q", value, got.String())
		}
	}
	for _, value := range []string{
		"white 8/16", "black 8/15", "black 17/16", "black -1/16", "black8/16", "black 8", "grey", "black  8/16",
	} {
		if got, err := parseInk(value); err == nil {
			t.Errorf("%q was read as %s", value, got)
		}
	}
}
