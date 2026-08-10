package gicisky

import "testing"

func TestDriverMatchesTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		deviceName string
		address    string
		want       bool
	}{
		{
			name:    "default address",
			target:  TargetAddress,
			address: TargetAddress,
			want:    true,
		},
		{
			name:       "default primary name fallback",
			target:     TargetAddress,
			deviceName: TargetName,
			want:       true,
		},
		{
			name:       "default secondary name fallback",
			target:     TargetAddress,
			deviceName: FallbackName,
			want:       true,
		},
		{
			name:       "explicit name",
			target:     "OTHER-TAG",
			deviceName: "other-tag",
			want:       true,
		},
		{
			name:       "explicit name rejects primary fallback",
			target:     "OTHER-TAG",
			deviceName: TargetName,
			want:       false,
		},
		{
			name:       "explicit name rejects secondary fallback",
			target:     "OTHER-TAG",
			deviceName: FallbackName,
			want:       false,
		},
		{
			name:    "explicit UUID",
			target:  "798242F1-4013-F052-96BE-FC5CD9CC8042",
			address: "798242f1-4013-f052-96be-fc5cd9cc8042",
			want:    true,
		},
		{
			name:       "explicit UUID rejects default name",
			target:     "798242F1-4013-F052-96BE-FC5CD9CC8042",
			deviceName: FallbackName,
			address:    "another-device",
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver := &Driver{Target: test.target}
			if got := driver.matches(test.deviceName, test.address); got != test.want {
				t.Fatalf("matches(%q, %q) = %v, want %v", test.deviceName, test.address, got, test.want)
			}
		})
	}
}
