package display

import (
	"bytes"
	"image/png"
	"testing"
)

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
