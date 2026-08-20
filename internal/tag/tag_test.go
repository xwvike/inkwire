package tag

import "testing"

// Picking the wrong family is not a failure that announces itself. The two
// speak different services and pack pixels differently, so a page sent to the
// wrong driver either finds nothing to talk to or writes bytes that mean
// something else into a panel's RAM.
func TestTheFamilyIsChosenFromTheTargetOrStatedOutright(t *testing.T) {
	tests := []struct {
		requested, target, want string
		refused                 bool
	}{
		// The name is the one thing an EPD-nRF5 tag says about itself.
		{requested: "auto", target: "NRF_EPD_C1F8", want: NRFEPD},
		{requested: "auto", target: "nrf_epd_c1f8", want: NRFEPD},
		// Everything else keeps the family that has always been the default.
		{requested: "auto", target: "PICKSMART", want: Gicisky},
		{requested: "auto", target: "FF:FF:92:94:38:61", want: Gicisky},
		// An address says nothing about the family, so saying so is how an
		// EPD-nRF5 tag is reached by address at all.
		{requested: NRFEPD, target: "FF:FF:92:94:38:61", want: NRFEPD},
		{requested: Gicisky, target: "NRF_EPD_C1F8", want: Gicisky},
		// A query parameter that was never sent and a flag left at its
		// default are the same question, and used to be answered by two
		// copies of this rule that disagreed about it.
		{requested: "", target: "NRF_EPD_C1F8", want: NRFEPD},
		{requested: "", target: "FF:FF:92:94:38:61", want: Gicisky},
		// A misspelling is refused rather than quietly treated as auto.
		{requested: "nrf", target: "NRF_EPD_C1F8", refused: true},
	}
	for _, test := range tests {
		got, err := Resolve(test.requested, test.target)
		if test.refused {
			if err == nil {
				t.Errorf("family %q was accepted", test.requested)
			}
			// A refusal has to come back empty as well as failing. A caller
			// that reads the family before the error would otherwise route a
			// misspelling to whichever family this happened to name.
			if got != "" {
				t.Errorf("family %q was refused but still answered %q", test.requested, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("family %q target %q: %v", test.requested, test.target, err)
			continue
		}
		if got != test.want {
			t.Errorf("family %q target %q chose %s, want %s", test.requested, test.target, got, test.want)
		}
	}
}
