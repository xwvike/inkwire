package display

import (
	"fmt"
	"image/png"
	"io"
)

const (
	GiciskyWidth       = 296
	GiciskyHeight      = 128
	GiciskyPlaneSize   = GiciskyWidth * GiciskyHeight / 8
	GiciskyPayloadSize = GiciskyPlaneSize * 2
)

func WritePNG(writer io.Writer, frame *Frame) error {
	if writer == nil {
		return fmt.Errorf("PNG writer must not be nil")
	}
	if frame == nil {
		return fmt.Errorf("frame must not be nil")
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(writer, frame); err != nil {
		return fmt.Errorf("encode PNG: %w", err)
	}
	return nil
}

// EncodeNRFEPD packs a frame into the planes the EPD-nRF5 firmware expects.
//
// The two families disagree about almost everything, which is why this is a
// second function rather than a parameter on the first. Gicisky rotates a
// quarter turn and sets a bit to mean ink; here the panel is written the way
// it is seen, row by row, and a set bit means white. The colour plane is
// inverted on top of that: a clear bit is red.
//
// A red pixel comes out clear in both planes. That is what the reference
// implementation does, and it is the reading that survives either way of
// combining the two: panels where the colour plane wins show red, and the
// black plane alone would have shown ink rather than paper.
//
// red says whether the panel has a colour plane at all. A frame with red on it
// bound for a black and white panel is refused rather than quietly flattened,
// because losing a colour is not something the picture shows you afterwards.
func EncodeNRFEPD(frame *Frame, red bool) (black, colour []byte, err error) {
	if frame == nil {
		return nil, nil, fmt.Errorf("frame must not be nil")
	}
	width, height := frame.Width(), frame.Height()
	if width <= 0 || height <= 0 {
		return nil, nil, fmt.Errorf("frame must have a positive size, got %dx%d", width, height)
	}
	// A row starts on a byte boundary, so a width that is not a multiple of
	// eight leaves spare bits at the end of every row that no pixel owns.
	// Both planes start white and ink is cleared into them, which is what
	// leaves those bits white: building them the other way up would run a
	// black stripe down the right edge of every row on any panel whose width
	// is not a multiple of eight.
	stride := (width + 7) / 8
	black = fillWhite(stride * height)
	if red {
		colour = fillWhite(stride * height)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*stride + x/8
			mask := byte(0x80 >> uint(x%8))
			ink, _ := frame.InkAt(x, y)
			switch {
			case ink == InkWhite:
			case ink == InkRed && !red:
				return nil, nil, fmt.Errorf("frame has red at (%d,%d) and this panel is black and white", x, y)
			case ink == InkRed:
				colour[index] &^= mask
				black[index] &^= mask
			default:
				black[index] &^= mask
			}
		}
	}
	return black, colour, nil
}

func fillWhite(size int) []byte {
	plane := make([]byte, size)
	for index := range plane {
		plane[index] = 0xff
	}
	return plane
}

// EncodeGicisky converts the visual 296x128 frame into the tag's two planes.
// The physical protocol requires a 90-degree counter-clockwise transform,
// row-major packing, and MSB-first bits.
func EncodeGicisky(frame *Frame) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("frame must not be nil")
	}
	if frame.Width() != GiciskyWidth || frame.Height() != GiciskyHeight {
		return nil, fmt.Errorf("Gicisky frame must be %dx%d, got %dx%d", GiciskyWidth, GiciskyHeight, frame.Width(), frame.Height())
	}
	payload := make([]byte, GiciskyPayloadSize)
	bw := payload[:GiciskyPlaneSize]
	red := payload[GiciskyPlaneSize:]

	// Rotated output coordinates are (x=sourceY, y=width-1-sourceX).
	for sourceY := 0; sourceY < GiciskyHeight; sourceY++ {
		for sourceX := 0; sourceX < GiciskyWidth; sourceX++ {
			rotatedX := sourceY
			rotatedY := GiciskyWidth - 1 - sourceX
			pixel := rotatedY*GiciskyHeight + rotatedX
			index := pixel / 8
			mask := byte(0x80 >> uint(pixel%8))
			ink, _ := frame.InkAt(sourceX, sourceY)
			switch ink {
			case InkWhite:
				bw[index] |= mask
			case InkRed:
				red[index] |= mask
			}
		}
	}
	return payload, nil
}
