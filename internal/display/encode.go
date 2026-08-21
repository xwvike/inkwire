package display

import (
	"fmt"
	"image/png"
	"io"
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
