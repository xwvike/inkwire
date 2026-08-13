package gicisky

import "testing"

// The upstream table records one real advertisement per panel family. Decoding
// those bytes back to the right profile checks the parser against samples that
// were captured, not against the parser's own arithmetic.
func TestSampleAdvertisementsDecodeToTheirPanels(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		id    uint16
		model string
		width int
	}{
		// The upstream comment labels this only "2.1", and two 2.1" BWR rows
		// exist. The high byte picks the 250x128 one, not the 212x104 one.
		{"2.1 inch", []byte{0x0B, 0x1D, 0x81, 0x01, 0x41}, 0x010B, `EPD 2.1" BWR`, 250},
		{"2.9 inch", []byte{0x33, 0x1D, 0x81, 0x01, 0x40}, 0x0033, `EPD 2.9" BWR`, 296},
		{"3.7 inch", []byte{0x2B, 0x1E, 0x81, 0x01, 0x02}, 0x022B, `EPD 3.7" BWR`, 240},
		{"4.2 inch", []byte{0x4B, 0x1E, 0x81, 0x01, 0x40}, 0x004B, `EPD 4.2" BWR`, 400},
		{"7.5 inch", []byte{0x2B, 0x1E, 0x01, 0x01, 0x01}, 0x012B, `EPD 7.5" BWR`, 800},
		{"10.2 inch", []byte{0x8B, 0x1F, 0x01, 0x01, 0x00}, 0x008B, `EPD 10.2" BWR`, 960},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			advertised, ok := ParseAdvertisement(test.bytes)
			if !ok {
				t.Fatalf("% X was rejected", test.bytes)
			}
			if advertised.ID != test.id {
				t.Fatalf("id = 0x%04X, want 0x%04X", advertised.ID, test.id)
			}
			profile, known := LookupProfile(advertised.ID, advertised.Firmware)
			if !known {
				t.Fatalf("id 0x%04X is not in the table", advertised.ID)
			}
			if profile.Model != test.model || profile.Width != test.width {
				t.Errorf("profile = %q %dx%d, want %q width %d",
					profile.Model, profile.Width, profile.Height, test.model, test.width)
			}
		})
	}
}

// 0x2B appears as the low byte of both the 3.7 inch and the 7.5 inch panels.
// They are only told apart by the high byte, so a parser that ignored it would
// drive a 240x416 page onto an 800x480 panel.
func TestTheHighByteSeparatesPanelsThatShareALowByte(t *testing.T) {
	small, _ := ParseAdvertisement([]byte{0x2B, 0x1E, 0x81, 0x01, 0x02})
	large, _ := ParseAdvertisement([]byte{0x2B, 0x1E, 0x01, 0x01, 0x01})
	if small.ID == large.ID {
		t.Fatalf("both decoded to 0x%04X", small.ID)
	}
	smallProfile, _ := LookupProfile(small.ID, small.Firmware)
	largeProfile, _ := LookupProfile(large.ID, large.Firmware)
	if smallProfile.Pixels() >= largeProfile.Pixels() {
		t.Errorf("3.7 inch %dx%d is not smaller than 7.5 inch %dx%d",
			smallProfile.Width, smallProfile.Height, largeProfile.Width, largeProfile.Height)
	}
}

// The id occupies fourteen bits; the top two vary between panel families and
// are not understood, so they must be masked rather than carried into lookups.
func TestTheTopTwoBitsAreNotPartOfTheIdentity(t *testing.T) {
	// This is the advertisement captured from the tag on this desk.
	advertised, ok := ParseAdvertisement([]byte{0x33, 0x1E, 0x81, 0x01, 0x40})
	if !ok {
		t.Fatal("the captured advertisement was rejected")
	}
	if advertised.Hardware != 0x4033 {
		t.Errorf("hardware = 0x%04X, want 0x4033", advertised.Hardware)
	}
	if advertised.ID != 0x0033 {
		t.Errorf("id = 0x%04X, want 0x0033 once the top bits are masked", advertised.ID)
	}
	profile, known := LookupProfile(advertised.ID, advertised.Firmware)
	if !known || profile.Width != 296 || profile.Height != 128 || profile.Palette != PaletteBWR {
		t.Errorf("profile = %+v, want the 296x128 BWR panel this was captured from", profile)
	}
	if !profile.Verified {
		t.Error("the panel that was checked against hardware is not marked verified")
	}
}

func TestAdvertisementsOfTheWrongLengthAreRefused(t *testing.T) {
	// Longer is refused as firmly as shorter: five bytes is the format this
	// parser knows, and a sixth means it is looking at something else.
	for _, data := range [][]byte{nil, {}, {0x33}, {0x33, 0x1E, 0x81, 0x01}, {0x33, 0x1E, 0x81, 0x01, 0x40, 0x00}} {
		if _, ok := ParseAdvertisement(data); ok {
			t.Errorf("% X was accepted despite being %d bytes", data, len(data))
		}
	}
}

// The firmware version selects the packing for one panel, so its byte order
// has to be pinned through the parser and not just handed to LookupProfile.
func TestFirmwareIsReadBigEndianFromTheMiddleBytes(t *testing.T) {
	advertised, ok := ParseAdvertisement([]byte{0x2B, 0x1E, 0x81, 0x01, 0x01})
	if !ok {
		t.Fatal("the advertisement was rejected")
	}
	if advertised.Firmware != 0x8101 {
		t.Fatalf("firmware = 0x%04X, want 0x8101 from bytes 2 and 3 in that order", advertised.Firmware)
	}
	// Reading those two bytes the other way round would give 0x0181, miss the
	// correction, and pack a 7.5" panel the way its newer firmware expects.
	profile, known := LookupProfile(advertised.ID, advertised.Firmware)
	if !known || profile.Width != 800 {
		t.Fatalf("profile = %+v, want the 7.5 inch panel", profile)
	}
	if !profile.Compression || profile.Compression2 {
		t.Errorf("firmware 0x%04X did not select the earlier compression: %+v", advertised.Firmware, profile)
	}
}

// Battery is the one byte that says something about the tag rather than the
// model, which makes it the only per-tag fact a scan can report.
func TestBatteryVoltageIsDecodedFromTheSecondByte(t *testing.T) {
	// Captured from the tag on this desk: 0x1E is 30 tenths of a volt.
	advertised, ok := ParseAdvertisement([]byte{0x33, 0x1E, 0x81, 0x01, 0x40})
	if !ok {
		t.Fatal("the captured advertisement was rejected")
	}
	if advertised.Battery != 0x1E {
		t.Errorf("battery byte = 0x%02X, want 0x1E", advertised.Battery)
	}
	if advertised.Voltage() != 3.0 {
		t.Errorf("voltage = %v, want 3.0", advertised.Voltage())
	}
}

// A firmware revision changes how one panel is packed. Losing that would send
// the wrong compression to a 7.5 inch tag running older firmware.
func TestOlderSevenFiveFirmwareSelectsTheEarlierCompression(t *testing.T) {
	current, _ := LookupProfile(0x012B, 0x0101)
	older, _ := LookupProfile(0x012B, 0x8101)
	if !current.Compression2 || current.Compression {
		t.Errorf("current firmware profile = %+v, want the later compression", current)
	}
	if !older.Compression || older.Compression2 {
		t.Errorf("older firmware profile = %+v, want the earlier compression", older)
	}
}

func TestUnknownIdsAreNotGuessedAt(t *testing.T) {
	if profile, known := LookupProfile(0x3FFF, 0); known {
		t.Errorf("an unknown id resolved to %+v", profile)
	}
}

func TestKnownProfilesAreOrderedAndComplete(t *testing.T) {
	known := KnownProfiles()
	if len(known) != len(profiles) {
		t.Fatalf("listed %d profiles, table has %d", len(known), len(profiles))
	}
	for i := 1; i < len(known); i++ {
		if known[i-1].ID >= known[i].ID {
			t.Fatalf("profiles are not ordered: 0x%04X before 0x%04X", known[i-1].ID, known[i].ID)
		}
	}
	verified := 0
	for _, profile := range known {
		if profile.Width <= 0 || profile.Height <= 0 || profile.Model == "" {
			t.Errorf("profile 0x%04X is incomplete: %+v", profile.ID, profile)
		}
		if profile.Verified {
			verified++
		}
	}
	// Only the panel on this desk has been checked. If that ever changes it
	// should be a deliberate edit, not a quiet one.
	if verified != 1 {
		t.Errorf("%d profiles claim to be hardware-verified, expected exactly 1", verified)
	}
}
