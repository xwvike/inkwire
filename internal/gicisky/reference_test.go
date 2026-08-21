package gicisky

import (
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// The 2.9" BWR panel, the one model in this table whose output has been checked
// against hardware here.
const (
	verified29Width       = 296
	verified29Height      = 128
	verified29PlaneSize   = verified29Width * verified29Height / 8
	verified29PayloadSize = verified29PlaneSize * 2
)

// encodeVerified29 packs a frame for that panel and only that panel, written
// out longhand: a 90-degree counter-clockwise transform, row-major packing,
// MSB first.
//
// It exists to be disagreed with. Encode derives every panel's layout from the
// profile table, and a table-driven encoder can be wrong in a way that agreeing
// with itself would never reveal, so the one model anybody has held in their
// hand is also written the other way and the two are compared. Keeping them
// separate implementations is the point; keeping them in separate packages
// never was, and this one lived in display, where it made the drawing layer
// know the name of a tag family.
//
// It is a test file because that is all it ever was. Nothing in the program
// called it.
func encodeVerified29(frame *display.Frame) []byte {
	payload := make([]byte, verified29PayloadSize)
	bw := payload[:verified29PlaneSize]
	red := payload[verified29PlaneSize:]

	// Rotated output coordinates are (x=sourceY, y=width-1-sourceX).
	for sourceY := 0; sourceY < verified29Height; sourceY++ {
		for sourceX := 0; sourceX < verified29Width; sourceX++ {
			rotatedX := sourceY
			rotatedY := verified29Width - 1 - sourceX
			pixel := rotatedY*verified29Height + rotatedX
			index := pixel / 8
			mask := byte(0x80 >> uint(pixel%8))
			ink, _ := frame.InkAt(sourceX, sourceY)
			switch ink {
			case display.InkWhite:
				bw[index] |= mask
			case display.InkRed:
				red[index] |= mask
			}
		}
	}
	return payload
}

// The oracle has to be right before it is worth comparing anything to it, so
// its planes and its rotation are checked against bytes written down by hand.
func TestTheVerifiedEncoderPacksItsPlanesAndRotation(t *testing.T) {
	frame, err := display.NewFrame(verified29Width, verified29Height, display.InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(295, 0, display.InkBlack)
	frame.Set(294, 1, display.InkRed)

	payload := encodeVerified29(frame)
	if got, want := len(payload), verified29PayloadSize; got != want {
		t.Fatalf("payload length = %d, want %d", got, want)
	}
	bw := payload[:verified29PlaneSize]
	red := payload[verified29PlaneSize:]
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
