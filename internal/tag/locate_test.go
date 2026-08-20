package tag

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"tinygo.org/x/bluetooth"
)

func at(t *testing.T, seed byte) bluetooth.Address {
	t.Helper()
	var address bluetooth.Address
	for _, literal := range []string{
		fmt.Sprintf("0000f00d-0000-1000-8000-00805f9b%04x", uint16(seed)),
		fmt.Sprintf("AA:BB:CC:DD:EE:%02X", seed),
	} {
		address.Set(literal)
		var zero bluetooth.Address
		if address.String() != zero.String() {
			return address
		}
	}
	t.Fatal("no address literal this platform accepts")
	return address
}

// Which family a tag belongs to is discovered, not guessed from the shape of a
// name. These are the ways that can go, including the ones that used to end in
// a write to whichever family was the default.
func TestChoosingWhichTagToWriteTo(t *testing.T) {
	// Deliberately not the address this build used to default to. A tag that
	// the hardcoded default happens to match cannot show whether an empty
	// target was handled or quietly turned back into that default.
	ours := func(t *testing.T) gicisky.FoundDevice {
		return gicisky.FoundDevice{Address: at(t, 0x01), Name: "NEMRAABBCCDD", RSSI: -50}
	}
	theirs := func(t *testing.T) nrfepd.FoundDevice {
		return nrfepd.FoundDevice{Address: at(t, 0x02), Name: "NRF_EPD_C1F8", RSSI: -60}
	}

	t.Run("a named Gicisky tag", func(t *testing.T) {
		got, err := Choose([]gicisky.FoundDevice{ours(t)}, []nrfepd.FoundDevice{theirs(t)}, "NEMRAABBCCDD", "")
		if err != nil || got.Family != Gicisky {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("a named EPD-nRF5 tag", func(t *testing.T) {
		got, err := Choose([]gicisky.FoundDevice{ours(t)}, []nrfepd.FoundDevice{theirs(t)}, "NRF_EPD_C1F8", "")
		if err != nil || got.Family != NRFEPD {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	// Which tag is not worked out. One tag in range today is two the day
	// somebody brings another into the room, and the write that used to land
	// on the right one lands wherever.
	t.Run("no target is refused even when only one tag is there", func(t *testing.T) {
		for _, devices := range []struct {
			tags   []gicisky.FoundDevice
			others []nrfepd.FoundDevice
		}{
			{tags: []gicisky.FoundDevice{ours(t)}},
			{others: []nrfepd.FoundDevice{theirs(t)}},
			{tags: []gicisky.FoundDevice{ours(t)}, others: []nrfepd.FoundDevice{theirs(t)}},
		} {
			if _, err := Choose(devices.tags, devices.others, "", ""); !errors.Is(err, ErrNoDevice) {
				t.Errorf("an unnamed target was answered: %v", err)
			}
		}
	})

	// Being told what is there beats being told what is not.
	t.Run("a target that matches nothing says what was", func(t *testing.T) {
		_, err := Choose([]gicisky.FoundDevice{ours(t)}, []nrfepd.FoundDevice{theirs(t)}, "NEMR000000FF", "")
		if err == nil {
			t.Fatal("an unmatched target was accepted")
		}
		for _, want := range []string{"NEMR000000FF", "NEMRAABBCCDD", "NRF_EPD_C1F8"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %q: %v", want, err)
			}
		}
	})

	t.Run("nothing in range", func(t *testing.T) {
		if _, err := Choose(nil, nil, "NEMRAABBCCDD", ""); !errors.Is(err, ErrNoneInRange) {
			t.Fatalf("error = %v", err)
		}
	})

	// -family is obeyed and then checked. Writing one family's bytes to the
	// other is not a polite failure, so a target in the wrong family is named
	// rather than attempted.
	t.Run("an asserted family that the tag does not belong to", func(t *testing.T) {
		_, err := Choose([]gicisky.FoundDevice{ours(t)}, []nrfepd.FoundDevice{theirs(t)}, "NRF_EPD_C1F8", Gicisky)
		if err == nil {
			t.Fatal("a tag of the wrong family was accepted")
		}
		for _, want := range []string{"NRF_EPD_C1F8", "is a nrfepd tag", "the family asked for is gicisky"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %q: %v", want, err)
			}
		}
	})

	t.Run("an asserted family does not stand in for a target", func(t *testing.T) {
		if _, err := Choose([]gicisky.FoundDevice{ours(t)}, []nrfepd.FoundDevice{theirs(t)}, "", NRFEPD); !errors.Is(err, ErrNoDevice) {
			t.Errorf("-family was taken as naming a tag: %v", err)
		}
	})
}

func TestAnUnknownAssertedFamilyIsRefused(t *testing.T) {
	if _, err := assertedFamily("nrf"); err == nil {
		t.Fatal("a misspelled family was accepted")
	}
	for _, ok := range []string{"", "auto", Gicisky, NRFEPD} {
		if _, err := assertedFamily(ok); err != nil {
			t.Errorf("family %q was refused: %v", ok, err)
		}
	}
}

// Stopping the scan early is only safe once the match is complete. A Gicisky
// tag sends its name and its model in separate advertisements, so a scan that
// stopped at the name would come back holding a tag whose panel is unknown.
func TestAScanOnlyStopsForAMatchItCanUse(t *testing.T) {
	named := gicisky.FoundDevice{Address: at(t, 0x01), Name: "NEMRAABBCCDD", RSSI: -50}
	identified := named
	identified.HasAdvertised = true
	identified.Advertised = gicisky.Advertisement{ID: 0x0033}
	identified.Profile, identified.Identified = gicisky.LookupProfile(0x0033, 0)

	if usable(Choose([]gicisky.FoundDevice{named}, nil, "NEMRAABBCCDD", "")) {
		t.Error("the scan would have stopped on the name packet, before the model arrived")
	}
	if !usable(Choose([]gicisky.FoundDevice{identified}, nil, "NEMRAABBCCDD", "")) {
		t.Error("the scan kept listening after both halves had arrived")
	}
	// The other family puts everything knowable in one advertisement, so its
	// first match is already complete.
	other := nrfepd.FoundDevice{Address: at(t, 0x02), Name: "NRF_EPD_C1F8", RSSI: -60}
	if !usable(Choose(nil, []nrfepd.FoundDevice{other}, "NRF_EPD_C1F8", "")) {
		t.Error("an EPD-nRF5 match was treated as incomplete")
	}
	if usable(Choose(nil, nil, "NEMRAABBCCDD", "")) {
		t.Error("a scan with nothing in it was treated as a match")
	}
}

// Every scan is looking for a named tag now, so every scan can stop as soon as
// it has one. Nothing enumerates, so nothing waits out the window.
func TestTheScanStopsAsSoonAsTheNamedTagHasAnswered(t *testing.T) {
	identified := gicisky.FoundDevice{Address: at(t, 0x01), Name: "NEMRAABBCCDD", RSSI: -50}
	identified.HasAdvertised = true
	identified.Advertised = gicisky.Advertisement{ID: 0x0033}
	identified.Profile, identified.Identified = gicisky.LookupProfile(0x0033, 0)

	empty := stopFor("NEMRAABBCCDD", "",
		func() []gicisky.FoundDevice { return nil },
		func() []nrfepd.FoundDevice { return nil })
	if empty() {
		t.Error("the scan stopped before the tag had answered")
	}
	answered := stopFor("NEMRAABBCCDD", "",
		func() []gicisky.FoundDevice { return []gicisky.FoundDevice{identified} },
		func() []nrfepd.FoundDevice { return nil })
	if !answered() {
		t.Error("the scan kept listening after the tag had answered")
	}
}
