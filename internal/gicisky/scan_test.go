package gicisky

import (
	"fmt"
	"testing"

	"tinygo.org/x/bluetooth"
)

// zeroAddress is what an Address stringifies to before anything has been set,
// which is also what it stays as when Set is given a literal of the wrong kind.
var zeroAddress = func() string { var address bluetooth.Address; return address.String() }()

// at builds a distinct Address for a test device.
//
// An Address is a CoreBluetooth UUID on macOS and a MAC on Linux, and Set
// silently ignores a string of the other kind. Both forms are tried and the
// result is checked, because the failure mode otherwise is that every test
// device shares the zero address, collapses onto one map key, and the test
// passes while exercising nothing.
func at(t *testing.T, seed byte) bluetooth.Address {
	t.Helper()
	var address bluetooth.Address
	for _, literal := range []string{
		fmt.Sprintf("0000f00d-0000-1000-8000-00805f9b%04x", uint16(seed)),
		fmt.Sprintf("AA:BB:CC:DD:EE:%02X", seed),
	} {
		address.Set(literal)
		if address.String() != zeroAddress {
			return address
		}
	}
	t.Fatalf("no address literal this test knows is accepted on this platform")
	return address
}

// The advertisement this tag actually sends, and the one a second tag would.
var (
	oursRaw   = []byte{0x33, 0x1E, 0x81, 0x01, 0x40} // 2.9" BWR
	otherRaw  = []byte{0x4B, 0x1E, 0x81, 0x01, 0x40} // 4.2" BWR
	unknownID = []byte{0xFE, 0x1E, 0x81, 0x01, 0x3F} // not in the table
)

func advertisement(t *testing.T, raw []byte) Advertisement {
	t.Helper()
	parsed, ok := ParseAdvertisement(raw)
	if !ok {
		t.Fatalf("% X was rejected", raw)
	}
	return parsed
}

// A tag puts its name and its manufacturer data in different advertisements,
// so the two have to converge on one entry or every tag would be listed twice,
// once anonymous and once unidentified.
func TestPacketsFromOneTagMergeIntoOneEntry(t *testing.T) {
	set := newDeviceSet()
	// Named packet first, carrying no manufacturer data.
	set.observe(at(t, 0x01), "NEMRAABBCCDD", -50, Advertisement{}, false)
	// Then the identifying packet, which carries no name.
	set.observe(at(t, 0x01), "", -47, advertisement(t, oursRaw), true)

	devices := set.sorted()
	if len(devices) != 1 {
		t.Fatalf("one tag produced %d entries", len(devices))
	}
	device := devices[0]
	if device.Name != "NEMRAABBCCDD" {
		t.Errorf("name = %q; the nameless packet erased what the named one taught", device.Name)
	}
	if !device.Identified || device.Profile.Width != 296 {
		t.Errorf("profile = %+v, want the 2.9 inch panel", device.Profile)
	}
	if device.RSSI != -47 {
		t.Errorf("RSSI = %d, want the most recent reading", device.RSSI)
	}
}

func TestTwoTagsStayTwoTags(t *testing.T) {
	set := newDeviceSet()
	set.observe(at(t, 0x01), "NEMRAABBCCDD", -50, advertisement(t, oursRaw), true)
	set.observe(at(t, 0x02), "NEMR11223344", -60, advertisement(t, otherRaw), true)

	devices := set.sorted()
	if len(devices) != 2 {
		t.Fatalf("two tags produced %d entries", len(devices))
	}
	// Strongest first, so the nearer tag leads.
	if devices[0].RSSI != -50 || devices[1].RSSI != -60 {
		t.Fatalf("order = %d then %d, want the stronger signal first", devices[0].RSSI, devices[1].RSSI)
	}
	if devices[0].Profile.Width != 296 || devices[1].Profile.Width != 400 {
		t.Errorf("panels = %dx%d and %dx%d; each tag must keep its own profile",
			devices[0].Profile.Width, devices[0].Profile.Height,
			devices[1].Profile.Width, devices[1].Profile.Height)
	}
}

// Equal signal must not leave the order up to map iteration, or two scans of
// the same room would print the same tags in different orders.
func TestEqualSignalIsBrokenByAddress(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		set := newDeviceSet()
		set.observe(at(t, 0x03), "", -55, advertisement(t, oursRaw), true)
		set.observe(at(t, 0x01), "", -55, advertisement(t, oursRaw), true)
		set.observe(at(t, 0x02), "", -55, advertisement(t, oursRaw), true)
		devices := set.sorted()
		for i := 1; i < len(devices); i++ {
			if devices[i-1].Address.String() >= devices[i].Address.String() {
				t.Fatalf("attempt %d ordered %s before %s",
					attempt, devices[i-1].Address.String(), devices[i].Address.String())
			}
		}
	}
}

// A tag whose id is not in the table is still a tag. Dropping it would make it
// look absent, which is worse than saying it cannot be driven.
func TestUnrecognisedTagsAreStillListed(t *testing.T) {
	set := newDeviceSet()
	if !set.observe(at(t, 0x09), "", -66, advertisement(t, unknownID), true) {
		t.Fatal("a tag advertising an unknown id was discarded")
	}
	devices := set.sorted()
	if len(devices) != 1 {
		t.Fatalf("got %d entries", len(devices))
	}
	if devices[0].Identified {
		t.Error("an id absent from the table was reported as identified")
	}
	if !devices[0].HasAdvertised || devices[0].Advertised.ID == 0 {
		t.Error("the advertised id was lost, so it cannot be reported upstream")
	}
}

func TestOtherPeoplesDevicesAreIgnored(t *testing.T) {
	set := newDeviceSet()
	for _, name := range []string{"", "MacBook Pro", "AirPods", "Some Beacon"} {
		if set.observe(at(t, 0x77), name, -40, Advertisement{}, false) {
			t.Errorf("a device named %q with no manufacturer data was kept", name)
		}
	}
	if devices := set.sorted(); len(devices) != 0 {
		t.Fatalf("kept %d unrelated devices", len(devices))
	}
}

// A tag still powering up advertises PICKSMART and no manufacturer data. It is
// worth listing so it can be seen at all, even though it cannot be identified.
func TestATagStillPoweringUpIsListed(t *testing.T) {
	set := newDeviceSet()
	if !set.observe(at(t, 0x07), TargetName, -44, Advertisement{}, false) {
		t.Fatal("a tag advertising its power-up name was discarded")
	}
	devices := set.sorted()
	if len(devices) != 1 || devices[0].Identified || devices[0].HasAdvertised {
		t.Fatalf("got %+v, want one listed but unidentified tag", devices)
	}
}

// The two scans stop on different questions, and getting that wrong is not
// visible in a payload or a picture — only in how long a write takes, or in a
// write to a tag whose panel was never established.
//
// Identify must not stop on the named packet. The name says which tag; the
// model is in a packet that has no name in it, so stopping at the first match
// would leave the panel unknown and the page unrenderable.
func TestIdentifyWaitsForBothHalvesAndFindDoesNot(t *testing.T) {
	const target = "NEMRAABBCCDD"
	findStop, identifyStop := matchedBy(target), identifiedBy(target)

	set := newDeviceSet()
	if findStop(set) || identifyStop(set) {
		t.Fatal("an empty scan satisfied something")
	}

	// A different tag's packets must not satisfy either question.
	set.observe(at(t, 0x02), "NEMR000000FF", -60, advertisement(t, otherRaw), true)
	if findStop(set) || identifyStop(set) {
		t.Fatal("somebody else's tag satisfied a question about ours")
	}

	// Our tag, named, with no manufacturer data yet.
	set.observe(at(t, 0x01), target, -50, Advertisement{}, false)
	if !findStop(set) {
		t.Error("find should stop here: the tag it was asked for has answered")
	}
	if identifyStop(set) {
		t.Fatal("identify stopped before the panel was known; the name packet carries no model")
	}

	// The identifying packet, which carries no name of its own.
	set.observe(at(t, 0x01), "", -47, advertisement(t, oursRaw), true)
	if !identifyStop(set) {
		t.Fatal("identify did not stop once both halves had arrived")
	}
	device, _ := set.match(target)
	if !device.Identified || device.Profile.Width != 296 {
		t.Errorf("profile = %+v, want the 2.9 inch panel", device.Profile)
	}
}
