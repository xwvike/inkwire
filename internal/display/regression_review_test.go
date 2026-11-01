package display

import (
	"image"
	"testing"
)

func TestEvenStrokeCoversHalfPixelSamples(t *testing.T) {
	// Compare the complete segment with its geometric footprint, including
	// shallow slopes where row culling can omit pixels before coverage is tested.
	for _, width := range []int{1, 2, 3, 4} {
		for dx := 1; dx <= 30; dx++ {
			for dy := -10; dy <= 10; dy++ {
				from, to := image.Pt(10, 15), image.Pt(10+dx, 15+dy)
				frame := newTestFrame(t, 55, 40)
				NewCanvas(frame).DrawPolyline([]image.Point{from, to}, StrokeStyle{Width: width, Ink: InkBlack, Cap: StrokeCapButt})
				bias := 0.0
				if width%2 == 0 {
					bias = 0.5
				}
				for y := 0; y < 40; y++ {
					for x := 0; x < 55; x++ {
						distance, _, projection := distanceToFloatSegment(float64(x)+bias, float64(y)+bias, from, to)
						want := distance <= float64(width)/2 && projection >= 0 && projection <= 1
						ink, _ := frame.InkAt(x, y)
						if (ink == InkBlack) != want {
							t.Fatalf("width %d segment %v -> %v pixel (%d,%d): ink=%s, covered=%v", width, from, to, x, y, ink, want)
						}
					}
				}
			}
		}
	}
}

func TestZeroRadiusCircleContainsItsCenter(t *testing.T) {
	frame := newTestFrame(t, 9, 9)
	NewCanvas(frame).FillCircle(image.Pt(4, 4), 0, InkBlack)
	assertInk(t, frame, 4, 4, InkBlack)
}

func TestCircleStrokeKeepsZeroRadiusHole(t *testing.T) {
	for radius := 1; radius <= 4; radius++ {
		frame := newTestFrame(t, 11, 11)
		NewCanvas(frame).StrokeCircle(image.Pt(5, 5), radius, StrokeStyle{Width: radius, Ink: InkBlack})
		assertInk(t, frame, 5, 5, InkWhite)
		assertInk(t, frame, 5+radius, 5, InkBlack)
	}
}
