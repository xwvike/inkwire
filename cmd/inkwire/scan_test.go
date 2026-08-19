package main

import (
	"fmt"
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
