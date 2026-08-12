package display

import (
	"image"
	"testing"
)

// DisplayList derives recorded bounds from its own geometry helpers rather than
// from the rasterizer, so the two can drift apart. TestDisplayListReplayMatches
// ImmediateCanvas already pins replay fidelity; these cases exist to vary the
// parameters the bounds math actually depends on: brush width, dash pattern and
// sweep direction.
func boundsCases() []struct {
	name   string
	record func(*DisplayList)
} {
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

	return []struct {
		name   string
		record func(*DisplayList)
	}{
		{"StrokeRectThick", func(d *DisplayList) { d.StrokeRect(image.Rect(5, 5, 60, 40), thick) }},
		{"StrokeRectDashed", func(d *DisplayList) { d.StrokeRect(image.Rect(5, 5, 60, 40), dashed) }},
		{"DrawLineThick", func(d *DisplayList) { d.DrawLine(image.Pt(10, 10), image.Pt(70, 50), thick) }},
		{"DrawLineOddDash", func(d *DisplayList) { d.DrawLine(image.Pt(10, 70), image.Pt(80, 20), odd) }},
		{"DrawLineNegativeOffset", func(d *DisplayList) { d.DrawLine(image.Pt(5, 5), image.Pt(85, 85), negative) }},
		{"DrawPolylineThick", func(d *DisplayList) { d.DrawPolyline(line, thick) }},
		{"StrokePolygonThick", func(d *DisplayList) { d.StrokePolygon(poly, thick) }},
		{"FillPolygon", func(d *DisplayList) { d.FillPolygon(poly, InkRed) }},
		{"StrokeCircleThick", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 30, thick) }},
		{"StrokeCircleDashed", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 30, dashed) }},
		{"StrokeCircleWidthOverRadius", func(d *DisplayList) { d.StrokeCircle(image.Pt(45, 45), 3, thick) }},
		{"StrokeEllipseDashed", func(d *DisplayList) { d.StrokeEllipse(image.Rect(10, 20, 80, 55), dashed) }},
		{"StrokeRoundRectDashed", func(d *DisplayList) { d.StrokeRoundRect(image.Rect(8, 8, 70, 50), 12, dashed) }},
		{"DrawArcThick", func(d *DisplayList) { d.DrawArc(image.Rect(10, 10, 80, 70), 200, 300, thick) }},
		{"DrawArcNegativeSweep", func(d *DisplayList) { d.DrawArc(image.Rect(10, 10, 80, 70), 40, -190, thin) }},
		{"FillPie", func(d *DisplayList) { d.FillPie(image.Rect(10, 10, 80, 70), -90, 125, InkRed) }},
		{"FillChord", func(d *DisplayList) { d.FillChord(image.Rect(10, 10, 80, 70), 20, 210, InkBlack) }},
		{"StrokePathCurve", func(d *DisplayList) { d.StrokePath(curve, thick) }},
		{"FillPathHole", func(d *DisplayList) { d.FillPath(holed, InkBlack) }},
	}
}

func TestDisplayListBoundsContainEveryPaintedPixel(t *testing.T) {
	for _, test := range boundsCases() {
		t.Run(test.name, func(t *testing.T) {
			list := &DisplayList{}
			test.record(list)
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
				t.Fatal("this case painted nothing, so it proves nothing")
			}
			if len(outside) != 0 {
				t.Fatalf("%d of %d painted pixels fall outside Bounds()=%v, first %v",
					len(outside), painted, bounds, outside[:min(5, len(outside))])
			}
		})
	}
}

// A display list must not drop a draw the canvas would perform. Recording
// guards that duplicate a primitive's internal early-return go stale the moment
// that primitive changes: a dashed round rectangle wider than its own box
// became a silent no-op on replay while the canvas still painted it, because
// the guard still described a stroker the canvas no longer used.
func TestDisplayListRecordsWhateverTheCanvasPaints(t *testing.T) {
	const size = 24
	rect := image.Rect(4, 4, 16, 16)
	wide := StrokeStyle{Ink: InkBlack, Width: 20, Dash: []int{3, 2}}
	huge := StrokeStyle{Ink: InkBlack, Width: 40}

	for _, test := range []struct {
		name   string
		draw   func(*Canvas)
		record func(*DisplayList)
	}{
		{"round rect, dash wider than the box",
			func(c *Canvas) { c.StrokeRoundRect(rect, 4, wide) },
			func(d *DisplayList) { d.StrokeRoundRect(rect, 4, wide) }},
		{"round rect, stroke wider than the box",
			func(c *Canvas) { c.StrokeRoundRect(rect, 4, huge) },
			func(d *DisplayList) { d.StrokeRoundRect(rect, 4, huge) }},
		{"ellipse, dash wider than the box",
			func(c *Canvas) { c.StrokeEllipse(rect, wide) },
			func(d *DisplayList) { d.StrokeEllipse(rect, wide) }},
		{"rect, dash wider than the box",
			func(c *Canvas) { c.StrokeRect(rect, wide) },
			func(d *DisplayList) { d.StrokeRect(rect, wide) }},
		{"circle, dash wider than the radius",
			func(c *Canvas) { c.StrokeCircle(image.Pt(12, 12), 5, wide) },
			func(d *DisplayList) { d.StrokeCircle(image.Pt(12, 12), 5, wide) }},
		{"full arc, dash wider than the box",
			func(c *Canvas) { c.DrawArc(rect, 0, 360, wide) },
			func(d *DisplayList) { d.DrawArc(rect, 0, 360, wide) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			direct := newTestFrame(t, size, size)
			test.draw(NewCanvas(direct))

			list := &DisplayList{}
			test.record(list)
			replayed := newTestFrame(t, size, size)
			if err := list.Replay(NewCanvas(replayed)); err != nil {
				t.Fatal(err)
			}
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					want, _ := direct.InkAt(x, y)
					got, _ := replayed.InkAt(x, y)
					if got != want {
						t.Fatalf("pixel (%d,%d): replay = %d, canvas = %d (recorded %d commands)",
							x, y, got, want, list.Len())
					}
				}
			}
		})
	}
}
