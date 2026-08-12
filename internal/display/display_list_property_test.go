package display

import (
	"image"
	"testing"
)

type rasterCase struct {
	name   string
	record func(*DisplayList)
	draw   func(*Canvas)
}

func rasterCases() []rasterCase {
	thin := StrokeStyle{Ink: InkBlack, Width: 1}
	thick := StrokeStyle{Ink: InkBlack, Width: 5}
	dashed := StrokeStyle{Ink: InkRed, Width: 3, Dash: []int{4, 3}, DashOffset: 2}
	odd := StrokeStyle{Ink: InkBlack, Width: 2, Dash: []int{5}}
	negative := StrokeStyle{Ink: InkBlack, Width: 2, Dash: []int{3, 2}, DashOffset: -7}

	poly := []image.Point{{X: 20, Y: 20}, {X: 60, Y: 25}, {X: 55, Y: 60}, {X: 25, Y: 55}}
	line := []image.Point{{X: 10, Y: 10}, {X: 70, Y: 40}, {X: 30, Y: 70}}

	var curve Path
	curve.MoveTo(image.Pt(10, 60))
	curve.CubicTo(image.Pt(30, 5), image.Pt(60, 95), image.Pt(80, 30))

	var holed Path
	holed.MoveTo(image.Pt(15, 15))
	holed.LineTo(image.Pt(75, 15))
	holed.LineTo(image.Pt(75, 75))
	holed.LineTo(image.Pt(15, 75))
	holed.Close()
	holed.MoveTo(image.Pt(35, 35))
	holed.LineTo(image.Pt(55, 35))
	holed.LineTo(image.Pt(55, 55))
	holed.LineTo(image.Pt(35, 55))
	holed.Close()

	return []rasterCase{
		{"FillRect", func(d *DisplayList) { d.FillRect(image.Rect(5, 5, 40, 30), InkBlack) },
			func(c *Canvas) { c.FillRect(image.Rect(5, 5, 40, 30), InkBlack) }},
		{"StrokeRectThick", func(d *DisplayList) { d.StrokeRect(image.Rect(5, 5, 60, 40), thick) },
			func(c *Canvas) { c.StrokeRect(image.Rect(5, 5, 60, 40), thick) }},
		{"StrokeRectDashed", func(d *DisplayList) { d.StrokeRect(image.Rect(5, 5, 60, 40), dashed) },
			func(c *Canvas) { c.StrokeRect(image.Rect(5, 5, 60, 40), dashed) }},
		{"DrawLineThick", func(d *DisplayList) { d.DrawLine(image.Pt(10, 10), image.Pt(70, 50), thick) },
			func(c *Canvas) { c.DrawLine(image.Pt(10, 10), image.Pt(70, 50), thick) }},
		{"DrawLineOddDash", func(d *DisplayList) { d.DrawLine(image.Pt(10, 70), image.Pt(80, 20), odd) },
			func(c *Canvas) { c.DrawLine(image.Pt(10, 70), image.Pt(80, 20), odd) }},
		{"DrawLineNegOffset", func(d *DisplayList) { d.DrawLine(image.Pt(5, 5), image.Pt(85, 85), negative) },
			func(c *Canvas) { c.DrawLine(image.Pt(5, 5), image.Pt(85, 85), negative) }},
		{"DrawPolyline", func(d *DisplayList) { d.DrawPolyline(line, thick) },
			func(c *Canvas) { c.DrawPolyline(line, thick) }},
		{"StrokePolygon", func(d *DisplayList) { d.StrokePolygon(poly, thick) },
			func(c *Canvas) { c.StrokePolygon(poly, thick) }},
		{"FillPolygon", func(d *DisplayList) { d.FillPolygon(poly, InkRed) },
			func(c *Canvas) { c.FillPolygon(poly, InkRed) }},
		{"FillCircle", func(d *DisplayList) { d.FillCircle(image.Pt(45, 45), 30, InkBlack) },
			func(c *Canvas) { c.FillCircle(image.Pt(45, 45), 30, InkBlack) }},
		{"StrokeCircleThick", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 30, thick) },
			func(c *Canvas) { c.StrokeCircle(image.Pt(45, 45), 30, thick) }},
		{"StrokeCircleDashed", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 30, dashed) },
			func(c *Canvas) { c.StrokeCircle(image.Pt(45, 45), 30, dashed) }},
		{"StrokeCircleWidthOverRadius", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 3, thick) },
			func(c *Canvas) { c.StrokeCircle(image.Pt(45, 45), 3, thick) }},
		{"FillEllipse", func(d *DisplayList) { d.FillEllipse(image.Rect(10, 20, 80, 55), InkRed) },
			func(c *Canvas) { c.FillEllipse(image.Rect(10, 20, 80, 55), InkRed) }},
		{"StrokeEllipseDashed", func(d *DisplayList) { d.StrokeEllipse(image.Rect(10, 20, 80, 55), dashed) },
			func(c *Canvas) { c.StrokeEllipse(image.Rect(10, 20, 80, 55), dashed) }},
		{"FillRoundRect", func(d *DisplayList) { d.FillRoundRect(image.Rect(8, 8, 70, 50), 12, InkBlack) },
			func(c *Canvas) { c.FillRoundRect(image.Rect(8, 8, 70, 50), 12, InkBlack) }},
		{"StrokeRoundRectDashed", func(d *DisplayList) { d.StrokeRoundRect(image.Rect(8, 8, 70, 50), 12, dashed) },
			func(c *Canvas) { c.StrokeRoundRect(image.Rect(8, 8, 70, 50), 12, dashed) }},
		{"DrawArcThick", func(d *DisplayList) { d.DrawArc(image.Rect(10, 10, 80, 70), 200, 300, thick) },
			func(c *Canvas) { c.DrawArc(image.Rect(10, 10, 80, 70), 200, 300, thick) }},
		{"DrawArcNegSweep", func(d *DisplayList) { d.DrawArc(image.Rect(10, 10, 80, 70), 40, -190, thin) },
			func(c *Canvas) { c.DrawArc(image.Rect(10, 10, 80, 70), 40, -190, thin) }},
		{"FillPie", func(d *DisplayList) { d.FillPie(image.Rect(10, 10, 80, 70), -90, 125, InkRed) },
			func(c *Canvas) { c.FillPie(image.Rect(10, 10, 80, 70), -90, 125, InkRed) }},
		{"FillChord", func(d *DisplayList) { d.FillChord(image.Rect(10, 10, 80, 70), 20, 210, InkBlack) },
			func(c *Canvas) { c.FillChord(image.Rect(10, 10, 80, 70), 20, 210, InkBlack) }},
		{"StrokePathCurve", func(d *DisplayList) { d.StrokePath(curve, thick) },
			func(c *Canvas) { c.StrokePath(curve, thick) }},
		{"FillPathHole", func(d *DisplayList) { d.FillPath(holed, InkBlack) },
			func(c *Canvas) { c.FillPath(holed, InkBlack) }},
	}
}

// Replaying a display list must paint exactly what the direct Canvas call paints.
func TestReplayMatchesDirectDrawAcrossPrimitives(t *testing.T) {
	for _, probe := range rasterCases() {
		t.Run(probe.name, func(t *testing.T) {
			direct := newTestFrame(t, 96, 96)
			probe.draw(NewCanvas(direct))

			list := &DisplayList{}
			probe.record(list)
			replayed := newTestFrame(t, 96, 96)
			if err := list.Replay(NewCanvas(replayed)); err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 96; y++ {
				for x := 0; x < 96; x++ {
					want, _ := direct.InkAt(x, y)
					got, _ := replayed.InkAt(x, y)
					if got != want {
						t.Fatalf("pixel (%d,%d): replay = %d, direct = %d", x, y, got, want)
					}
				}
			}
		})
	}
}

// DisplayList.Bounds is derived separately from the rasterizer, so it must be
// checked against the pixels replay actually paints.
func TestDisplayListBoundsContainEveryPaintedPixel(t *testing.T) {
	for _, probe := range rasterCases() {
		t.Run(probe.name, func(t *testing.T) {
			list := &DisplayList{}
			probe.record(list)
			bounds := list.Bounds()

			frame := newTestFrame(t, 96, 96)
			if err := list.Replay(NewCanvas(frame)); err != nil {
				t.Fatal(err)
			}
			painted := 0
			var outside []image.Point
			for y := 0; y < 96; y++ {
				for x := 0; x < 96; x++ {
					ink, _ := frame.InkAt(x, y)
					if ink == InkWhite {
						continue
					}
					painted++
					if !image.Pt(x, y).In(bounds) {
						outside = append(outside, image.Pt(x, y))
					}
				}
			}
			if painted == 0 {
				t.Fatalf("%s painted nothing, probe is vacuous", probe.name)
			}
			if len(outside) != 0 {
				t.Fatalf("%s: %d of %d painted pixels fall outside Bounds()=%v, first %v",
					probe.name, len(outside), painted, bounds, outside[:min(5, len(outside))])
			}
		})
	}
}
