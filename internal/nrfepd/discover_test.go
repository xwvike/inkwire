package nrfepd

import (
	"fmt"
	"testing"

	"tinygo.org/x/bluetooth"
)

// advertisement is enough of a payload to be sorted by. The real one is
// platform-specific and unexported, and the only parts either question reads
// are the local name and the service UUIDs.
type advertisement struct {
	name     string
	services []bluetooth.UUID
}

func (a advertisement) LocalName() string { return a.name }
func (a advertisement) HasServiceUUID(u bluetooth.UUID) bool {
	for _, s := range a.services {
		if s == u {
			return true
		}
	}
	return false
}
func (a advertisement) ServiceUUIDs() []bluetooth.UUID                        { return a.services }
func (a advertisement) Bytes() []byte                                         { return nil }
func (a advertisement) ManufacturerData() []bluetooth.ManufacturerDataElement { return nil }
func (a advertisement) ServiceData() []bluetooth.ServiceDataElement           { return nil }

func packet(t *testing.T, seed byte, payload advertisement) bluetooth.ScanResult {
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
	return bluetooth.ScanResult{Address: address, RSSI: -60, AdvertisementPayload: payload}
}

// The service UUID is what the firmware serves; the name is what somebody set
// DEVICE_NAME to. Identifying by the name alone meant a renamed tag vanished
// from a scan that could plainly see it advertising 62750001.
func TestATagIsRecognisedByItsServiceNotOnlyItsName(t *testing.T) {
	tests := []struct {
		name    string
		payload advertisement
		want    bool
	}{
		{
			name:    "the service, advertised without a name",
			payload: advertisement{services: []bluetooth.UUID{serviceUUID}},
			want:    true,
		},
		{
			name:    "a tag whose firmware was renamed still advertises the service",
			payload: advertisement{name: "kitchen", services: []bluetooth.UUID{serviceUUID}},
			want:    true,
		},
		{
			name:    "the name alone, for a build that does not list the service",
			payload: advertisement{name: "NRF_EPD_C1F8"},
			want:    true,
		},
		{
			name:    "somebody else's device",
			payload: advertisement{name: "Wireless Keyboard"},
			want:    false,
		},
		{
			name:    "somebody else's service",
			payload: advertisement{services: []bluetooth.UUID{bluetooth.ServiceUUIDHeartRate}},
			want:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Advertises(packet(t, 0x01, test.payload)); got != test.want {
				t.Errorf("Advertises = %v, want %v", got, test.want)
			}
		})
	}
}

// A packet carrying the service need not carry the name, so the two have to
// converge on one entry. Overwriting would let the nameless one win and leave
// a tag that cannot be addressed by the name it advertises.
func TestTheNamelessServicePacketDoesNotEraseTheName(t *testing.T) {
	set := newDeviceSet()
	set.observe(packet(t, 0x01, advertisement{name: "NRF_EPD_C1F8", services: []bluetooth.UUID{serviceUUID}}))
	set.observe(packet(t, 0x01, advertisement{services: []bluetooth.UUID{serviceUUID}}))

	devices := set.sorted()
	if len(devices) != 1 {
		t.Fatalf("one tag produced %d entries", len(devices))
	}
	if devices[0].Name != "NRF_EPD_C1F8" {
		t.Errorf("name = %q; the nameless packet erased what the named one taught", devices[0].Name)
	}
}
