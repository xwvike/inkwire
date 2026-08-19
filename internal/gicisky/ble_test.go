package gicisky

import (
	"strings"
	"testing"
	"time"
)

func TestDriverMatchesTarget(t *testing.T) {
	// A second tag, so that "does this match" can be distinguished from
	// "does this match anything".
	const otherMAC, otherName = "FF:FF:AA:BB:CC:DD", "NEMRAABBCCDD"

	tests := []struct {
		name       string
		target     string
		deviceName string
		address    string
		want       bool
	}{
		{
			name:    "address as advertised on a host that exposes MACs",
			target:  TargetAddress,
			address: TargetAddress,
			want:    true,
		},
		{
			// The whole reason the derivation exists: on macOS the address
			// is a per-host UUID, so this is the only way a MAC target ever
			// reaches its tag.
			name:       "MAC target matches the name that MAC implies",
			target:     TargetAddress,
			deviceName: "NEMR92943861",
			address:    "e2ada7d1-187a-caea-21e4-d895f8240b62",
			want:       true,
		},
		{
			name:       "MAC target of a second tag matches that tag",
			target:     otherMAC,
			deviceName: otherName,
			address:    "some-other-uuid",
			want:       true,
		},
		{
			// This is the bug the derivation fixes: previously only the one
			// hardcoded default MAC had any name fallback at all, so a
			// second tag addressed by MAC was unreachable on macOS.
			name:       "MAC target does not match a different tag",
			target:     otherMAC,
			deviceName: "NEMR92943861",
			address:    "e2ada7d1-187a-caea-21e4-d895f8240b62",
			want:       false,
		},
		{
			// Every tag advertises PICKSMART while powering up. Honouring it
			// for a MAC target would send a write to whichever tag happened
			// to be booting.
			name:       "MAC target does not match a tag that is still powering up",
			target:     TargetAddress,
			deviceName: TargetName,
			want:       false,
		},
		{
			name:       "PICKSMART is reachable when it is asked for by name",
			target:     TargetName,
			deviceName: TargetName,
			want:       true,
		},
		{
			// What makes a target a MAC is twelve hex digits, not where the
			// separators fall, so an oddly punctuated one still resolves.
			name:       "MAC target with unusual separator placement",
			target:     "ffff-aa-bb-cc-dd",
			deviceName: otherName,
			want:       true,
		},
		{
			name:       "eleven hex digits is not a MAC",
			target:     "FFFFAABBCCD",
			deviceName: otherName,
			want:       false,
		},
		{
			// A truncated address is all hex and far shorter than a MAC.
			// Deriving a name from it would index past the end of it.
			name:       "a truncated address is not a MAC",
			target:     "FF",
			deviceName: otherName,
			want:       false,
		},
		{
			name:       "separators alone are not a MAC",
			target:     "::",
			deviceName: otherName,
			want:       false,
		},
		{
			// A twelve-character label is the right length for a MAC without
			// being one. Skipping the hex check would rewrite it into a name
			// and match a tag it has nothing to do with.
			name:       "a twelve-character label is a name, not a MAC",
			target:     "SHELF01BACK1",
			deviceName: "NEMRF01BACK1",
			want:       false,
		},
		{
			name:       "twelve characters that are not all hex is not a MAC",
			target:     "FFFFAABBCCDZ",
			deviceName: otherName,
			want:       false,
		},
		{
			name:       "MAC target without separators",
			target:     "FFFFAABBCCDD",
			deviceName: otherName,
			want:       true,
		},
		{
			name:       "MAC target in lower case with dashes",
			target:     "ff-ff-aa-bb-cc-dd",
			deviceName: "nemraabbccdd",
			want:       true,
		},
		{
			name:       "explicit name",
			target:     "OTHER-TAG",
			deviceName: "other-tag",
			want:       true,
		},
		{
			name:       "explicit name rejects the power-up name",
			target:     "OTHER-TAG",
			deviceName: TargetName,
			want:       false,
		},
		{
			name:       "explicit name rejects an unrelated tag",
			target:     "OTHER-TAG",
			deviceName: otherName,
			want:       false,
		},
		{
			name:    "explicit UUID",
			target:  "798242F1-4013-F052-96BE-FC5CD9CC8042",
			address: "798242f1-4013-f052-96be-fc5cd9cc8042",
			want:    true,
		},
		{
			// A UUID has thirty-two hex digits, so it must not be mistaken
			// for a MAC and turned into a name.
			name:       "explicit UUID is not read as a MAC",
			target:     "798242F1-4013-F052-96BE-FC5CD9CC8042",
			deviceName: "NEMR92943861",
			address:    "another-device",
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MatchesTarget(test.target, test.deviceName, test.address); got != test.want {
				t.Fatalf("MatchesTarget(%q, %q, %q) = %v, want %v",
					test.target, test.deviceName, test.address, got, test.want)
			}
		})
	}
}

func TestSelectIdentifiedRequiresAMatchedKnownAdvertisement(t *testing.T) {
	advertised, _ := ParseAdvertisement([]byte{0x33, 0x1E, 0x81, 0x01, 0x40})
	knownProfile, _ := LookupProfile(advertised.ID, advertised.Firmware)
	known := FoundDevice{
		Name:          "NEMR92943861",
		Advertised:    advertised,
		HasAdvertised: true,
		Profile:       knownProfile,
		Identified:    true,
	}
	selected, err := SelectIdentified([]FoundDevice{known}, TargetAddress)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Profile.Width != 296 || selected.Profile.Height != 128 {
		t.Fatalf("selected profile = %+v", selected.Profile)
	}

	_, err = SelectIdentified([]FoundDevice{{Name: TargetName}}, TargetName)
	if err == nil || !strings.Contains(err.Error(), "no model advertisement") {
		t.Fatalf("missing advertisement error = %v", err)
	}

	unknown := known
	unknown.Advertised.ID = 0x3FFE
	unknown.Identified = false
	_, err = SelectIdentified([]FoundDevice{unknown}, TargetAddress)
	if err == nil || !strings.Contains(err.Error(), "0x3FFE") {
		t.Fatalf("unknown profile error = %v", err)
	}
}

// These timings were measured against the tag on 2026-08-13, and the margins
// between them are the only reason a healthy write is not cut short. The test
// exists so that shortening a timeout has to argue with the measurement.
func TestTimeoutsClearTheMeasuredDevice(t *testing.T) {
	const (
		slowestScan       = 11530 * time.Millisecond
		slowestConnect    = 7790 * time.Millisecond
		slowestResponse   = 110 * time.Millisecond
		slowestHealthyRun = 20521 * time.Millisecond
	)

	if DefaultScanTimeout <= slowestScan {
		t.Errorf("scan timeout %s does not clear the slowest measured scan %s", DefaultScanTimeout, slowestScan)
	}
	// The response timeout also has to cover the first exchange, which the
	// tag only answers once it has connected and discovered services.
	if DefaultResponseTimeout <= slowestResponse {
		t.Errorf("response timeout %s does not clear the slowest measured reply %s", DefaultResponseTimeout, slowestResponse)
	}
	// One attempt must be able to complete a whole healthy write, or every
	// push would burn its retries on a tag that was answering all along.
	budget := DefaultScanTimeout + DefaultNotifyReadyDelay + slowestConnect
	if budget <= slowestHealthyRun {
		t.Errorf("one attempt allows %s, less than the slowest healthy write %s", budget, slowestHealthyRun)
	}
	if DefaultAttempts < 2 {
		t.Errorf("attempts = %d leaves no retry for a scan that misses the advertising window", DefaultAttempts)
	}
}
