package gicisky

import (
	"testing"
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
