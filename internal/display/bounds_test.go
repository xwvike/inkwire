package display

import (
	"image"
	"math/rand"
	"testing"
	"time"
)

// bresenhamStep says where drawBresenham is after any number of steps, in
// every direction and at every slope, without taking them.
func TestABresenhamStepIsWhereTheWalkGets(t *testing.T) {
	random := rand.New(rand.NewSource(3))
	for trial := 0; trial < 4000; trial++ {
		span := []int{3, 40, 5000}[trial%3]
		from := image.Pt(random.Intn(2*span)-span, random.Intn(2*span)-span)
		to := image.Pt(random.Intn(2*span)-span, random.Intn(2*span)-span)
		step := 0
		drawBresenham(from, to, func(point image.Point) {
			if got := bresenhamStep(from, to, step); got != point {
				t.Fatalf("%v to %v: step %d is %v, the walk is at %v", from, to, step, got, point)
			}
			step++
		})
	}
}

// Geometry far larger than the frame costs what can be seen of it. Every one
// of these used to take minutes or exhaust memory; each is a shape a page can
// ask for by writing a large number.
func TestHugeGeometryCostsWhatIsSeen(t *testing.T) {
	const huge = 100_000_000
	dashed := StrokeStyle{Ink: InkBlack, Width: 2, Dash: []int{3, 2}}
	round := StrokeStyle{Ink: InkBlack, Width: 3, Dash: []int{3, 2}, Cap: StrokeCapRound, Join: StrokeJoinRound}
	triangle := []image.Point{{0, 0}, {huge, 0}, {0, huge}}
	cases := map[string]func(*Canvas){
		"dashed rect":   func(c *Canvas) { c.StrokeRect(image.Rect(0, 0, huge, huge), dashed) },
		"dashed circle": func(c *Canvas) { c.StrokeCircle(image.Pt(0, 0), huge, dashed) },
		"turned ellipse": func(c *Canvas) {
			c.StrokeEllipse(Oval{Bounds: image.Rect(-huge, -1000, huge+1, 1001), Rotation: 10}, dashed)
		},
		"round rect":     func(c *Canvas) { c.StrokeRoundRect(image.Rect(0, 0, huge, huge), huge/10, dashed) },
		"far line":       func(c *Canvas) { c.DrawLine(image.Pt(-huge, -huge/3), image.Pt(huge, huge/3), dashed) },
		"round dashes":   func(c *Canvas) { c.DrawLine(image.Pt(0, 0), image.Pt(huge, huge/3), round) },
		"polygon dashes": func(c *Canvas) { c.StrokePolygon(triangle, dashed) },
		"polygon fill":   func(c *Canvas) { c.FillPolygon(triangle, InkRed) },
		"wide stroke": func(c *Canvas) {
			c.StrokePolygon(triangle, StrokeStyle{Ink: InkBlack, Width: huge, Join: StrokeJoinRound})
		},
		"wide line": func(c *Canvas) { c.DrawPolyline(triangle, StrokeStyle{Ink: InkBlack, Width: huge}) },
		"arc":       func(c *Canvas) { c.DrawArc(Upright(image.Rect(-huge, -huge, huge, huge)), 10, 90, round) },
		"clip circle": func(c *Canvas) {
			var path Path
			path.Arc(Upright(image.Rect(-huge, -huge, huge, huge)), 0, 360)
			path.Close()
			c.ClipPath(path)
			c.FillRect(image.Rect(0, 0, 400, 300), InkBlack)
		},
		"drawn small": func(c *Canvas) {
			c.Transform(Scale(1e-6, 1e-6, 0, 0))
			c.StrokePolygon(triangle, StrokeStyle{Ink: InkBlack, Width: huge / 100})
		},
		"scaled frame": func(c *Canvas) {
			source := newTestFrame(t, 1, 1)
			if err := c.DrawFrame(source, image.Pt(-5, -5), Transform{Scale: 20_000, Turns: 1}, nil); err != nil {
				t.Error(err)
			}
		},
		"drawn large": func(c *Canvas) {
			c.Transform(Rotate(37, 0, 0).Then(Scale(1e5, 1e5, 0, 0)))
			c.StrokeCircle(image.Pt(0, 0), 10, dashed)
		},
	}
	for name, draw := range cases {
		frame := newTestFrame(t, 400, 300)
		done := make(chan struct{})
		start := time.Now()
		go func() {
			draw(NewCanvas(frame))
			close(done)
		}()
		select {
		case <-done:
			if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
				t.Errorf("%s took %v", name, elapsed)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
}
