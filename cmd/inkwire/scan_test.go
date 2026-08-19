package main

import (
	"flag"
	"fmt"
	"io"
	"testing"

	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"tinygo.org/x/bluetooth"
)

// advertisement is enough of a payload for the two collectors to sort by.
// The real one is platform-specific and unexported, and the only fields either
// family reads are the local name and the manufacturer data.
type advertisement struct {
	name         string
	manufacturer []bluetooth.ManufacturerDataElement
}

func (a advertisement) LocalName() string                           { return a.name }
func (a advertisement) HasServiceUUID(bluetooth.UUID) bool          { return false }
func (a advertisement) ServiceUUIDs() []bluetooth.UUID              { return nil }
func (a advertisement) Bytes() []byte                               { return nil }
func (a advertisement) ServiceData() []bluetooth.ServiceDataElement { return nil }
func (a advertisement) ManufacturerData() []bluetooth.ManufacturerDataElement {
	return a.manufacturer
}

func packet(t *testing.T, seed byte, rssi int16, payload advertisement) bluetooth.ScanResult {
	t.Helper()
	var address bluetooth.Address
	for _, literal := range []string{
		fmt.Sprintf("0000f00d-0000-1000-8000-00805f9b%04x", uint16(seed)),
		fmt.Sprintf("AA:BB:CC:DD:EE:%02X", seed),
	} {
		address.Set(literal)
		var zero bluetooth.Address
		if address.String() != zero.String() {
			break
		}
	}
	return bluetooth.ScanResult{Address: address, RSSI: rssi, AdvertisementPayload: payload}
}

// Both families now come out of one pass of the radio, so an advertisement is
// offered to both collectors. Each tag has to land in exactly one list: a
// packet counted twice would list one tag as two, and one counted by neither
// would lose it entirely.
func TestOneScanSortsEachTagIntoExactlyOneFamily(t *testing.T) {
	tests := []struct {
		name            string
		result          bluetooth.ScanResult
		gicisky, nrfepd int
	}{
		{
			name: "a Gicisky tag, named and identified",
			result: packet(t, 0x01, -50, advertisement{
				name: "NEMR92943861",
				manufacturer: []bluetooth.ManufacturerDataElement{
					{CompanyID: gicisky.ManufacturerCompanyID, Data: []byte{0x33, 0x1E, 0x81, 0x01, 0x40}},
				},
			}),
			gicisky: 1, nrfepd: 0,
		},
		{
			// The identifying packet carries no name of its own.
			name: "a Gicisky advertisement with no name",
			result: packet(t, 0x02, -55, advertisement{
				manufacturer: []bluetooth.ManufacturerDataElement{
					{CompanyID: gicisky.ManufacturerCompanyID, Data: []byte{0x33, 0x1E, 0x81, 0x01, 0x40}},
				},
			}),
			gicisky: 1, nrfepd: 0,
		},
		{
			name:    "an EPD-nRF5 tag, which advertises a name and nothing else",
			result:  packet(t, 0x03, -60, advertisement{name: "NRF_EPD_C1F8"}),
			gicisky: 0, nrfepd: 1,
		},
		{
			name: "somebody else's device",
			result: packet(t, 0x04, -40, advertisement{
				name:         "Wireless Keyboard",
				manufacturer: []bluetooth.ManufacturerDataElement{{CompanyID: 0x004C, Data: []byte{0x10, 0x06}}},
			}),
			gicisky: 0, nrfepd: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags, others := gicisky.NewCollector(), nrfepd.NewCollector()
			tags.Observe(test.result)
			others.Observe(test.result)
			if got := len(tags.Devices()); got != test.gicisky {
				t.Errorf("gicisky devices = %d, want %d", got, test.gicisky)
			}
			if got := len(others.Devices()); got != test.nrfepd {
				t.Errorf("nrfepd devices = %d, want %d", got, test.nrfepd)
			}
		})
	}

	// The whole point of one pass: every family's tags come out of the same
	// stream, still separated.
	tags, others := gicisky.NewCollector(), nrfepd.NewCollector()
	for _, test := range tests {
		tags.Observe(test.result)
		others.Observe(test.result)
	}
	if got := len(tags.Devices()); got != 2 {
		t.Errorf("one pass produced %d Gicisky tags, want 2", got)
	}
	if got := len(others.Devices()); got != 1 {
		t.Errorf("one pass produced %d EPD-nRF5 tags, want 1", got)
	}
}

// The default device is a Gicisky address, and handing it to the other family
// is a scan that cannot succeed. `inkwire push -family nrfepd page.json` spent
// three full scans and 49 seconds reporting "no NRF_EPD tag found (target
// FF:FF:92:94:38:61)" with the tag advertising the whole time.
func TestTheOtherFamilyDoesNotInheritThisOnesDefaultDevice(t *testing.T) {
	const giciskyDefault = "FF:FF:92:94:38:61"
	tests := []struct {
		name     string
		family   string
		target   string
		explicit bool
		want     string
	}{
		{
			name:   "nrfepd with no device takes the first tag of its kind",
			family: familyNRFEPD, target: giciskyDefault, want: "",
		},
		{
			name:   "a device the caller typed is kept, whatever family",
			family: familyNRFEPD, target: "NRF_EPD_C1F8", explicit: true, want: "NRF_EPD_C1F8",
		},
		{
			// Somebody who types the Gicisky default at an EPD-nRF5 tag has
			// asked for something that will not work, and is told so rather
			// than quietly given a different tag.
			name:   "even this family's own default, if it was typed",
			family: familyNRFEPD, target: giciskyDefault, explicit: true, want: giciskyDefault,
		},
		{
			name:   "gicisky keeps its default",
			family: familyGicisky, target: giciskyDefault, want: giciskyDefault,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := targetFor(test.family, test.target, test.explicit); got != test.want {
				t.Errorf("targetFor(%q, %q, %v) = %q, want %q",
					test.family, test.target, test.explicit, got, test.want)
			}
		})
	}
}

// Whether a flag was given cannot be read off its value: -device with the
// default address typed out is still the caller asking for that address, and
// comparing against the default would silently take it away again.
func TestWasSetTellsAGivenFlagFromItsDefault(t *testing.T) {
	const address = "FF:FF:92:94:38:61"
	newFlags := func() *flag.FlagSet {
		flags := flag.NewFlagSet("push", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.String("device", address, "")
		return flags
	}

	silent := newFlags()
	if err := silent.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if wasSet(silent, "device") {
		t.Error("a flag nobody gave was reported as given")
	}

	typed := newFlags()
	if err := typed.Parse([]string{"-device", address}); err != nil {
		t.Fatal(err)
	}
	if !wasSet(typed, "device") {
		t.Error("the default address, typed out, was mistaken for silence")
	}

	other := newFlags()
	if err := other.Parse([]string{"-device", "NRF_EPD_C1F8"}); err != nil {
		t.Fatal(err)
	}
	if !wasSet(other, "device") {
		t.Error("a flag that was given was reported as absent")
	}
}
