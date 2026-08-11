package display

import (
	"bytes"
	"image/png"
	"testing"
)

func TestEncodeGiciskyPlanesAndRotation(t *testing.T) {
	frame, err := NewFrame(GiciskyWidth, GiciskyHeight, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(295, 0, InkBlack)
	frame.Set(294, 1, InkRed)

	payload, err := EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(payload), GiciskyPayloadSize; got != want {
		t.Fatalf("payload length = %d, want %d", got, want)
	}
	bw := payload[:GiciskyPlaneSize]
	red := payload[GiciskyPlaneSize:]
	if got, want := bw[0], byte(0x7f); got != want {
		t.Fatalf("first BW byte = %02x, want %02x", got, want)
	}
	if got, want := bw[16], byte(0xbf); got != want {
		t.Fatalf("rotated red BW byte = %02x, want %02x", got, want)
	}
	if got, want := red[16], byte(0x40); got != want {
		t.Fatalf("rotated red byte = %02x, want %02x", got, want)
	}
}

func TestWritePNGRoundTripPreservesSemanticColors(t *testing.T) {
	frame, err := NewFrame(3, 1, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, InkBlack)
	frame.Set(2, 0, InkRed)
	var buffer bytes.Buffer
	if err := WritePNG(&buffer, frame); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	for x, want := range []Ink{InkBlack, InkWhite, InkRed} {
		got := decoded.At(x, 0)
		wr, wg, wb, wa := want.RGBA().RGBA()
		gr, gg, gb, ga := got.RGBA()
		if gr != wr || gg != wg || gb != wb || ga != wa {
			t.Errorf("pixel %d = %v, want %v", x, got, want.RGBA())
		}
	}
}
