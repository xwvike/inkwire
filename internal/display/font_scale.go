package display

import "fmt"

// scaledFace presents an existing strike enlarged by a whole number, turning
// each source pixel into an n by n block.
//
// Nearest-neighbour enlargement is normally a poor way to resize type, because
// it destroys the greys that antialiasing put there. This panel has no greys:
// a glyph is already a bitmap of set and unset pixels, so multiplying it by a
// whole number reproduces exactly the shape the designer drew, at n times the
// size, with nothing lost and nothing invented. It is the one enlargement that
// is exact here.
//
// The factor must be a whole number for that to hold. Scaling by 1.5 would
// have to decide what to do with half a pixel, and every answer to that
// question either drops part of a stroke or thickens it unevenly, which at
// these sizes is the difference between a legible glyph and a smudge.
type scaledFace struct {
	base   Face
	factor int
	name   string
}

func newScaledFace(base Face, factor int) (Face, error) {
	if base == nil {
		return nil, fmt.Errorf("scaled face needs a base face")
	}
	if factor < 1 {
		return nil, fmt.Errorf("scale factor must be at least 1, got %d", factor)
	}
	if factor == 1 {
		return base, nil
	}
	return &scaledFace{base: base, factor: factor, name: fmt.Sprintf("%s@%dx", base.Name(), factor)}, nil
}

func (f *scaledFace) Name() string { return f.name }
func (f *scaledFace) Size() int    { return f.base.Size() * f.factor }

func (f *scaledFace) Metrics() FontMetrics {
	metrics := f.base.Metrics()
	metrics.Ascent *= f.factor
	metrics.Descent *= f.factor
	metrics.LineGap *= f.factor
	return metrics
}

func (f *scaledFace) Glyph(r rune) (Glyph, bool) {
	source, ok := f.base.Glyph(r)
	if !ok {
		return Glyph{}, false
	}
	width, height := source.Width*f.factor, source.Height*f.factor
	rowBytes := (width + 7) / 8
	scaled := Glyph{
		Width:    width,
		Height:   height,
		RowBytes: rowBytes,
		Advance:  source.Advance * f.factor,
		Data:     make([]byte, rowBytes*height),
	}
	for y := 0; y < source.Height; y++ {
		for x := 0; x < source.Width; x++ {
			if !source.On(x, y) {
				continue
			}
			for dy := 0; dy < f.factor; dy++ {
				row := (y*f.factor + dy) * rowBytes
				for dx := 0; dx < f.factor; dx++ {
					column := x*f.factor + dx
					scaled.Data[row+column/8] |= 0x80 >> uint(column%8)
				}
			}
		}
	}
	return scaled, true
}
