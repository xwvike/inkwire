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
