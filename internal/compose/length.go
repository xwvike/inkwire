package compose

import "fmt"

// Length is a size that may not be knowable when a document is written.
//
// A number of pixels is settled from the start. A percentage is not: it is a
// share of a box whose size only exists once the layout has run. Carrying the
// difference through to that point, rather than deciding it earlier, is what
// lets a percentage mean the same thing wherever it appears, and lets a
// minimum or maximum apply on the axis a container measures rather than only
// on the one it divides.
// A length holds both parts because CSS lets them be added: calc(100% - 10px)
// is a share of the container and a fixed adjustment to it, and neither half
// can be dropped without changing what was asked for.
type Length struct {
	set bool
	// tenths of a percent, so that 87.3% survives
	percent int
	pixels  int
}

// Auto is the absent length: the box takes whatever size it measures.
func Auto() Length { return Length{} }

func Pixels(value int) Length { return Length{set: true, pixels: value} }

// Tenths builds a percentage from tenths of a percent, so 873 is 87.3%.
func Tenths(tenths int) Length { return Length{set: true, percent: tenths} }

// Calc is a share of the container plus a fixed number of pixels, which is
// what calc() is for once everything but lengths has been ruled out.
func Calc(tenths, pixels int) Length {
	return Length{set: true, percent: tenths, pixels: pixels}
}

func (l Length) IsSet() bool { return l.set }

func (l Length) String() string {
	switch {
	case !l.set:
		return "auto"
	case l.percent != 0 && l.pixels != 0:
		return fmt.Sprintf("calc(%.1f%% %+dpx)", float64(l.percent)/10, l.pixels)
	case l.percent != 0:
		return fmt.Sprintf("%.1f%%", float64(l.percent)/10)
	}
	return fmt.Sprintf("%dpx", l.pixels)
}

// Resolve turns the length into pixels against the space it is a length of.
// An absent length reports false, which is not the same as zero: a box with no
// stated size is measured, and a box stated as zero is not drawn.
func (l Length) Resolve(available int) (int, bool) {
	if !l.set {
		return 0, false
	}
	if available < 0 {
		available = 0
	}
	// Rounded rather than truncated, so that halves do not always fall the
	// same way and a row of percentages adds up to what it should.
	share := (available*l.percent + 500) / 1000
	total := share + l.pixels
	if total < 0 {
		total = 0
	}
	return total, true
}

// valid allows a negative pixel part, because calc(100% - 10px) is a perfectly
// ordinary thing to write; what it may not do is resolve below zero, which
// Resolve handles.
func (l Length) valid() bool { return !l.set || l.percent >= 0 }

// intrinsic reports the length's contribution to how large a box wants to be,
// before any container has been chosen. A percentage contributes nothing: it
// is a share of something that does not exist yet, and guessing at it here
// inflates the measurement and pushes the box's siblings off the line.
func (l Length) intrinsic() (int, bool) {
	if !l.set || l.percent != 0 {
		return 0, false
	}
	return l.pixels, true
}

// clamp applies a minimum and a maximum, either of which may be absent.
func clamp(value int, minimum, maximum Length, available int) int {
	if low, ok := minimum.Resolve(available); ok && value < low {
		value = low
	}
	if high, ok := maximum.Resolve(available); ok && value > high {
		value = high
	}
	return value
}
