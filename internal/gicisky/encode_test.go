package gicisky

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestEncodeVerifiedProfileMatchesLegacyEncoder(t *testing.T) {
	profile, known := LookupProfile(0x0033, 0)
	if !known {
		t.Fatal("0x0033 profile is missing")
	}
	frame, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, display.InkBlack)
	frame.Set(295, 0, display.InkBlack)
	frame.Set(12, 18, display.InkRed)
	frame.Set(294, 1, display.InkRed)

	got, err := Encode(frame, profile)
	if err != nil {
		t.Fatal(err)
	}
	want, err := display.EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("profile encoder no longer matches the verified 2.9 inch encoder")
	}
}

func TestEncodeOrientedProfileAcceptsPortraitPage(t *testing.T) {
	profile, _ := LookupProfile(0x0033, 0)
	portrait, err := display.NewFrame(profile.Height, profile.Width, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	portrait.Set(0, 0, display.InkBlack)
	landscape, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	landscape.Set(profile.Width-1, 0, display.InkBlack)

	got, err := EncodeOriented(portrait, display.OrientationPortraitClockwise, profile)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Encode(landscape, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("portrait clockwise page was not mapped to the expected landscape pixels")
	}
}

func TestEncodeProfilesUseTheirPanelSizes(t *testing.T) {
	tests := []struct {
		name string
		id   uint16
		want int
	}{
		{"2.9 inch BW", 0x0028, 296 * 128 / 8},
		{"4.2 inch BWR", 0x004B, 400 * 300 / 8 * 2},
		{"TFT 2.1 inch BW", 0x00A0, 250 * 132 / 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, known := LookupProfile(test.id, 0)
			if !known {
				t.Fatalf("profile 0x%04X is missing", test.id)
			}
			frame, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := Encode(frame, profile)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) != test.want {
				t.Fatalf("payload length = %d, want %d", len(payload), test.want)
			}
		})
	}
}

func TestEncodeRejectsInkThePanelCannotShow(t *testing.T) {
	profile, _ := LookupProfile(0x0028, 0)
	frame, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, display.InkRed)
	if _, err := Encode(frame, profile); err == nil || !strings.Contains(err.Error(), "cannot show red") {
		t.Fatalf("BW red error = %v", err)
	}

	profile, _ = LookupProfile(0x0033, 0)
	frame, err = display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, display.InkYellow)
	if _, err := Encode(frame, profile); err == nil || !strings.Contains(err.Error(), "cannot show yellow") {
		t.Fatalf("BWR yellow error = %v", err)
	}
}

func TestEncodeFourColorPacksBWRYInTwoBitOrder(t *testing.T) {
	profile := Profile{Width: 4, Height: 1, Palette: PaletteBWRY, FourColor: true}
	frame, err := display.NewFrame(profile.Width, profile.Height, display.InkBlack)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(1, 0, display.InkWhite)
	frame.Set(2, 0, display.InkYellow)
	frame.Set(3, 0, display.InkRed)

	payload, err := Encode(frame, profile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := payload[0], byte(0x1b); got != want {
		t.Fatalf("first BWRY byte = %02x, want %02x", got, want)
	}
	if got, want := len(payload), profile.Width*profile.Height/4; got != want {
		t.Fatalf("payload length = %d, want %d", got, want)
	}
}

func TestEncodeColumnCompressionFormat(t *testing.T) {
	profile, _ := LookupProfile(0x022B, 0)
	frame, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := Encode(frame, profile)
	if err != nil {
		t.Fatal(err)
	}
	bytesPerLine := profile.Height / 8
	wantLength := 4 + profile.Width*2*(bytesPerLine+7)
	if got := int(binary.LittleEndian.Uint32(payload[:4])); got != wantLength {
		t.Fatalf("header length = %d, want %d", got, wantLength)
	}
	if len(payload) != wantLength {
		t.Fatalf("payload length = %d, want %d", len(payload), wantLength)
	}
	if payload[4] != 0x75 || payload[5] != byte(bytesPerLine+7) || payload[6] != byte(bytesPerLine) {
		t.Fatalf("first compressed-column header = % x", payload[4:11])
	}
}

func TestCompress2MatchesUpstreamVector(t *testing.T) {
	raw := append(bytes.Repeat([]byte{0xff}, 64), bytes.Repeat([]byte{0x00}, 64)...)
	want, err := hex.DecodeString("4000000075124010000080ffffffff000038ffffffff751240100000800000000000003800000000")
	if err != nil {
		t.Fatal(err)
	}
	if got := compress2(raw); !bytes.Equal(got, want) {
		t.Fatalf("compress2() = %x, want %x", got, want)
	}
}

func TestEncodeCompression2UsesQuickLZChunks(t *testing.T) {
	profile, _ := LookupProfile(0x012B, 0x0101)
	frame, err := display.NewFrame(profile.Width, profile.Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := Encode(frame, profile)
	if err != nil {
		t.Fatal(err)
	}
	rawLength := profile.Width * profile.Height / 8 * 2
	if got, want := int(binary.LittleEndian.Uint32(payload[:4])), rawLength/2; got != want {
		t.Fatalf("compression2 second half length = %d, want %d", got, want)
	}
	if payload[4] != 0x75 {
		t.Fatalf("first compression2 chunk marker = 0x%02x, want 0x75", payload[4])
	}
	if len(payload) >= rawLength/2 {
		t.Fatalf("compression2 payload length = %d, want less than half of raw %d", len(payload), rawLength)
	}
}
