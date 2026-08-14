package nrfepd

import "testing"

// The firmware picks a panel by ID and stores it, so a wrong ID here is not a
// lookup that fails. It is a different panel driven confidently.
func TestEveryModelHasItsOwnID(t *testing.T) {
	seen := map[uint8]string{}
	for _, model := range models {
		if other, taken := seen[model.ID]; taken {
			t.Errorf("id 0x%02x is both %s and %s", model.ID, other, model.Name)
		}
		seen[model.ID] = model.Name
	}
	// The reference enum runs from 0x01 to 0x11 with nothing missing, and a
	// gap here would mean a panel was dropped in transcription.
	for id := uint8(0x01); id <= 0x11; id++ {
		if _, ok := LookupModel(id); !ok {
			t.Errorf("id 0x%02x is in the firmware enum and not in this table", id)
		}
	}
	if len(models) != 0x11 {
		t.Errorf("table has %d panels, the firmware enum has %d", len(models), 0x11)
	}
}

// The packing does not follow from the size or the palette; it follows from the
// driver chip, and only the UC8159 parts differ. Tying the two together here is
// what stops a new entry being added with the majority packing because that is
// what the entry above it had.
func TestOnlyTheUC8159PartsArePackedInNibbles(t *testing.T) {
	for _, model := range models {
		nibbles := model.Packing == PackingNibbles
		if uc8159 := model.Driver == "UC8159"; nibbles != uc8159 {
			t.Errorf("%s is driven by %s and packed as %v", model.Name, model.Driver, model.Packing)
		}
	}
}

// A plane's length is the one number that has to be right before anything is
// sent, because it is what the panel reads back out as a picture.
func TestPlaneSizeFollowsThePacking(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		// 400 is fifty whole bytes a row, times three hundred rows.
		{"UC8176_420_BW", 50 * 300},
		// 880 is a hundred and ten, times five hundred and twenty-eight.
		{"SSD1677_750_HD_BWR", 110 * 528},
		// Nibbles are two pixels a byte over the whole page, not per row.
		{"UC8159_750_LOW_BWR", 640 * 384 / 2},
	}
	for _, test := range tests {
		model, ok := LookupModelName(test.name)
		if !ok {
			t.Fatalf("%s is not in the table", test.name)
		}
		if got := model.PlaneSize(); got != test.want {
			t.Errorf("%s plane = %d bytes, want %d", test.name, got, test.want)
		}
	}
}

// Exactly one entry has been held up against the panel it describes. Naming
// that one here, rather than counting them, means a panel becomes verified by
// somebody deciding it has been rather than by the claim drifting true.
//
// The other sixteen are transcriptions. They are very likely right and that is
// not the same as known: the size is the thing they would be wrong about, and a
// wrong size is not a page that looks off, it is a panel filled with bytes that
// mean something else.
func TestOnlyPanelsHeldUpAgainstTheirOwnHardwareAreVerified(t *testing.T) {
	verified := map[string]bool{"UC8176_420_BWR": true}
	for _, model := range models {
		if model.Verified != verified[model.Name] {
			t.Errorf("%s says verified=%v; if a panel has been driven and seen, add it here",
				model.Name, model.Verified)
		}
	}
}
