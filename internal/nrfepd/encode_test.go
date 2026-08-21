package nrfepd

import (
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// The two panels these tests encode for. Encode takes a model rather than a
// flag now, so the ink set comes from the table the firmware also reads.
var (
	colourPanel = Model{Name: "test-bwr", Width: 8, Height: 1, Palette: PaletteBWR, Packing: PackingPlanes}
	monoPanel   = Model{Name: "test-bw", Width: 8, Height: 1, Palette: PaletteBW, Packing: PackingPlanes}
)

// The packing is stated here as pictures rather than as byte constants, because
// the thing that goes wrong with a plane format is never the arithmetic. It is
// which way the bits run and which value means ink, and a row written out as
// eight pixels says that where 0x7f does not.
func TestEncodePacksRowsLeftToRightWithASetBitForWhite(t *testing.T) {
	frame, err := display.NewFrame(8, 1, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, display.InkBlack)

	black, colour, err := Encode(frame, colourPanel)
	if err != nil {
		t.Fatal(err)
	}
	// The leftmost pixel is the most significant bit, and it is the black one,
	// so it is the only bit clear.
	if got, want := black[0], byte(0x7f); got != want {
		t.Errorf("black plane = %08b, want %08b", got, want)
	}
	// Nothing is red, and a clear bit is red, so the colour plane is solid.
	if got, want := colour[0], byte(0xff); got != want {
		t.Errorf("colour plane = %08b, want %08b", got, want)
	}
}

// Red is the one value that is not simply itself in one plane: it reads as ink
// in the black plane as well, so that a panel combining the two either way
// shows something rather than paper.
func TestEncodeWritesRedIntoBothPlanes(t *testing.T) {
	frame, err := display.NewFrame(8, 1, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(1, 0, display.InkRed)

	black, colour, err := Encode(frame, colourPanel)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := colour[0], byte(0xbf); got != want {
		t.Errorf("colour plane = %08b, want %08b (bit clear where red is)", got, want)
	}
	if got, want := black[0], byte(0xbf); got != want {
		t.Errorf("black plane = %08b, want %08b (red reads as ink here too)", got, want)
	}
}

// A row starts on a byte boundary, so a width off the multiple leaves bits at
// the end of each row that no pixel owns. They have to be white: a clear bit is
// ink, and a strip of black down the right edge of every row is exactly the
// kind of thing that looks like a hardware fault rather than a packing bug.
func TestEncodePadsAPartialRowWithWhite(t *testing.T) {
	frame, err := display.NewFrame(12, 2, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	black, _, err := Encode(frame, colourPanel)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(black), 2*2; got != want {
		t.Fatalf("plane length = %d, want %d (two bytes a row)", got, want)
	}
	for index, value := range black {
		if value != 0xff {
			t.Errorf("byte %d = %08b, want all white", index, value)
		}
	}
}

// Losing a colour is not something the picture shows you afterwards: a red run
// flattened to black looks like a page that was drawn in black. So it is
// refused, and the refusal names where, because on a 400x300 page "there is red
// somewhere" is not a thing anybody can act on.
func TestEncodeRefusesRedOnABlackAndWhitePanel(t *testing.T) {
	frame, err := display.NewFrame(8, 4, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(3, 2, display.InkRed)

	_, _, err = Encode(frame, monoPanel)
	if err == nil {
		t.Fatal("red was accepted onto a black and white panel")
	}
	if !strings.Contains(err.Error(), "(3,2)") {
		t.Errorf("the error does not say where the red is: %v", err)
	}

	// The same frame on a colour panel is fine, and no colour plane is built
	// for a panel that has none.
	if _, colour, err := Encode(frame, colourPanel); err != nil || colour == nil {
		t.Errorf("colour panel: colour = %v, err = %v", colour != nil, err)
	}
	black, colour, err := Encode(mustFrame(t, 8, 1), monoPanel)
	if err != nil {
		t.Fatal(err)
	}
	if colour != nil {
		t.Error("a black and white panel was given a colour plane")
	}
	if len(black) != 1 {
		t.Errorf("black plane = %d bytes, want 1", len(black))
	}
}

func mustFrame(t *testing.T, width, height int) *display.Frame {
	t.Helper()
	frame, err := display.NewFrame(width, height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}
