package display

import (
	"image"
	"image/color"
	"slices"
	"testing"
)

func TestDisplayListReplayMatchesImmediateCanvas(t *testing.T) {
	directFrame := newTestFrame(t, 96, 64)
	replayFrame := newTestFrame(t, 96, 64)
	direct := NewCanvas(directFrame)
	list := &DisplayList{}
	registry := builtinRegistry(t)
	source := image.NewNRGBA(image.Rect(2, 3, 5, 5))
	source.SetNRGBA(2, 3, color.NRGBA{R: 0xff, A: 0xff})
	source.SetNRGBA(3, 3, color.NRGBA{A: 0xff})
	source.SetNRGBA(4, 3, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

	var path Path
	path.MoveTo(image.Pt(49, 5))
	path.QuadraticTo(image.Pt(57, 0), image.Pt(64, 7))
	path.LineTo(image.Pt(57, 14))
	path.Close()
	stroke := StrokeStyle{Ink: InkBlack, Width: 2, Dash: []int{3, 2}, DashOffset: 1}
	textBox := TextBox{
		Bounds: image.Rect(48, 26, 91, 42),
		Runs: []TextRun{
			{Text: "中", Style: TextStyle{Font: "ui", Size: 12, Ink: InkBlack}},
			{Text: "23", Style: TextStyle{Font: "monaco", Size: 10, Ink: InkRed}},
		},
		LineHeight: 14,
	}

	direct.Save()
	direct.ClipRect(image.Rect(2, 2, 93, 61))
	direct.Translate(image.Pt(2, 1))
	direct.StrokeRect(image.Rect(1, 1, 18, 13), stroke)
	direct.FillRoundRect(image.Rect(20, 1, 34, 13), 3, InkRed)
	direct.StrokeCircle(image.Pt(42, 7), 6, StrokeStyle{Ink: InkBlack, Width: 3})
	direct.FillEllipse(Upright(image.Rect(2, 17, 17, 29)), InkBlack)
	direct.DrawArc(Upright(image.Rect(20, 17, 35, 31)), 30, 230, StrokeStyle{Ink: InkRed, Width: 1})
	direct.FillPie(Upright(image.Rect(37, 17, 50, 30)), -90, 130, InkRed)
	direct.FillChord(Upright(image.Rect(52, 17, 66, 30)), 10, 220, InkBlack)
	direct.StrokePath(path, StrokeStyle{Ink: InkRed, Width: 1})
	direct.FillPolygon([]image.Point{image.Pt(69, 3), image.Pt(82, 5), image.Pt(74, 14)}, InkBlack)
	if err := direct.DrawImage(source, image.Rect(68, 17, 88, 29), ImageOptions{Fit: FitContain}); err != nil {
		t.Fatal(err)
	}
	if _, err := direct.DrawTextBox(registry, textBox); err != nil {
		t.Fatal(err)
	}
	direct.FillRect(image.Rect(1, 44, 7, 50), InkRed)
	direct.DrawLine(image.Pt(9, 44), image.Pt(16, 50), StrokeStyle{Ink: InkBlack, Width: 2})
	direct.DrawPolyline([]image.Point{image.Pt(19, 50), image.Pt(23, 44), image.Pt(27, 50)}, StrokeStyle{Ink: InkRed, Width: 1})
	direct.StrokePolygon([]image.Point{image.Pt(30, 50), image.Pt(34, 44), image.Pt(39, 50)}, StrokeStyle{Ink: InkBlack, Width: 1})
	direct.FillCircle(image.Pt(45, 47), 3, InkRed)
	direct.StrokeEllipse(Upright(image.Rect(51, 43, 61, 52)), StrokeStyle{Ink: InkBlack, Width: 2})
	direct.StrokeRoundRect(image.Rect(64, 43, 76, 53), 3, StrokeStyle{Ink: InkRed, Width: 1})
	var filledPath Path
	filledPath.MoveTo(image.Pt(80, 43))
	filledPath.LineTo(image.Pt(90, 43))
	filledPath.LineTo(image.Pt(85, 52))
	filledPath.Close()
	direct.FillPath(filledPath, InkBlack)
	direct.Restore()
	direct.Set(0, 0, InkRed)

	list.Save()
	list.ClipRect(image.Rect(2, 2, 93, 61))
	list.Translate(image.Pt(2, 1))
	list.StrokeRect(image.Rect(1, 1, 18, 13), stroke)
	list.FillRoundRect(image.Rect(20, 1, 34, 13), 3, InkRed)
	list.StrokeCircle(image.Pt(42, 7), 6, StrokeStyle{Ink: InkBlack, Width: 3})
	list.FillEllipse(Upright(image.Rect(2, 17, 17, 29)), InkBlack)
	list.DrawArc(Upright(image.Rect(20, 17, 35, 31)), 30, 230, StrokeStyle{Ink: InkRed, Width: 1})
	list.FillPie(Upright(image.Rect(37, 17, 50, 30)), -90, 130, InkRed)
	list.FillChord(Upright(image.Rect(52, 17, 66, 30)), 10, 220, InkBlack)
	list.StrokePath(path, StrokeStyle{Ink: InkRed, Width: 1})
	list.FillPolygon([]image.Point{image.Pt(69, 3), image.Pt(82, 5), image.Pt(74, 14)}, InkBlack)
	if err := list.DrawImage(source, image.Rect(68, 17, 88, 29), ImageOptions{Fit: FitContain}); err != nil {
		t.Fatal(err)
	}
	if _, err := list.DrawTextBox(registry, textBox); err != nil {
		t.Fatal(err)
	}
	list.FillRect(image.Rect(1, 44, 7, 50), InkRed)
	list.DrawLine(image.Pt(9, 44), image.Pt(16, 50), StrokeStyle{Ink: InkBlack, Width: 2})
	list.DrawPolyline([]image.Point{image.Pt(19, 50), image.Pt(23, 44), image.Pt(27, 50)}, StrokeStyle{Ink: InkRed, Width: 1})
	list.StrokePolygon([]image.Point{image.Pt(30, 50), image.Pt(34, 44), image.Pt(39, 50)}, StrokeStyle{Ink: InkBlack, Width: 1})
	list.FillCircle(image.Pt(45, 47), 3, InkRed)
	list.StrokeEllipse(Upright(image.Rect(51, 43, 61, 52)), StrokeStyle{Ink: InkBlack, Width: 2})
	list.StrokeRoundRect(image.Rect(64, 43, 76, 53), 3, StrokeStyle{Ink: InkRed, Width: 1})
	list.FillPath(filledPath, InkBlack)
	if !list.Restore() {
		t.Fatal("display list Restore did not match Save")
	}
	list.Set(0, 0, InkRed)

	if err := list.Replay(NewCanvas(replayFrame)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(directFrame.pixels, replayFrame.pixels) {
		t.Fatal("display list replay differs from immediate rendering")
	}
}

func TestDisplayListSnapshotsMutableInputs(t *testing.T) {
	list := &DisplayList{}
	points := []image.Point{image.Pt(1, 1), image.Pt(7, 1), image.Pt(4, 6)}
	dash := []int{2, 2}
	stroke := StrokeStyle{Ink: InkBlack, Width: 1, Dash: dash}
	list.StrokePolygon(points, stroke)

	var path Path
	path.MoveTo(image.Pt(10, 1))
	path.LineTo(image.Pt(15, 1))
	path.LineTo(image.Pt(12, 6))
	path.Close()
	list.FillPath(path, InkRed)

	source := image.NewNRGBA(image.Rect(4, 6, 5, 7))
	source.SetNRGBA(4, 6, color.NRGBA{R: 0xff, A: 0xff})
	if err := list.DrawImage(source, image.Rect(17, 1, 19, 3), ImageOptions{}); err != nil {
		t.Fatal(err)
	}

	points[0] = image.Pt(30, 30)
	dash[0] = 100
	path.Reset()
	source.SetNRGBA(4, 6, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

	frame := newTestFrame(t, 22, 9)
	if err := list.Replay(NewCanvas(frame)); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 12, 3, InkRed)
	assertInk(t, frame, 17, 1, InkRed)
}

func TestDisplayListCloneAndResetAreIndependent(t *testing.T) {
	list := &DisplayList{}
	list.FillRect(image.Rect(1, 1, 4, 4), InkBlack)
	clone := list.Clone()
	list.Reset()
	list.FillRect(image.Rect(5, 1, 7, 3), InkRed)

	if got, want := clone.Len(), 1; got != want {
		t.Fatalf("clone length = %d, want %d", got, want)
	}
	if got, want := list.Bounds(), image.Rect(5, 1, 7, 3); got != want {
		t.Fatalf("reset list bounds = %v, want %v", got, want)
	}
	frame := newTestFrame(t, 8, 5)
	if err := clone.Replay(NewCanvas(frame)); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 5, 1, InkWhite)
}

func TestDisplayListBoundsIncludeTransformClipAndStroke(t *testing.T) {
	list := &DisplayList{}
	list.ClipRect(image.Rect(0, 0, 10, 10))
	list.Translate(image.Pt(5, 4))
	list.DrawLine(image.Pt(-5, -4), image.Pt(8, 0), StrokeStyle{Ink: InkBlack, Width: 3})

	if got, want := list.Bounds(), image.Rect(0, 0, 10, 6); got != want {
		t.Fatalf("display list bounds = %v, want %v", got, want)
	}
	if got, want := list.Len(), 3; got != want {
		t.Fatalf("display list length = %d, want %d", got, want)
	}
}

func TestDisplayListNestedStateRestoresRecordingBounds(t *testing.T) {
	list := &DisplayList{}
	list.Save()
	list.Translate(image.Pt(10, 0))
	list.ClipRect(image.Rect(0, 0, 2, 2))
	list.FillRect(image.Rect(-5, -5, 5, 5), InkBlack)
	if !list.Restore() {
		t.Fatal("Restore did not pop saved display list state")
	}
	list.Set(1, 4, InkRed)

	if got, want := list.Bounds(), image.Rect(1, 0, 12, 5); got != want {
		t.Fatalf("restored display list bounds = %v, want %v", got, want)
	}
}

func TestDisplayListEmptyAndRestoreUnderflow(t *testing.T) {
	list := &DisplayList{}
	if list.Restore() {
		t.Fatal("empty display list restored past its initial state")
	}
	if list.Len() != 0 || !list.Bounds().Empty() {
		t.Fatalf("empty display list has length %d and bounds %v", list.Len(), list.Bounds())
	}
	if err := list.Replay(NewCanvas(newTestFrame(t, 2, 2))); err != nil {
		t.Fatal(err)
	}
}

func TestDisplayListRejectsInvalidResourcesWithoutRecording(t *testing.T) {
	list := &DisplayList{}
	if err := list.DrawTextLayout(nil); err == nil {
		t.Fatal("DrawTextLayout accepted a nil layout")
	}
	if err := list.DrawImage(nil, image.Rect(0, 0, 1, 1), ImageOptions{}); err == nil {
		t.Fatal("DrawImage accepted a nil source")
	}
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if err := list.DrawImage(source, image.Rect(0, 0, 1, 1), ImageOptions{Fit: ImageFit(99)}); err == nil {
		t.Fatal("DrawImage accepted an invalid fit mode")
	}
	if list.Len() != 0 {
		t.Fatalf("invalid resources recorded %d commands", list.Len())
	}
	if err := list.Replay(nil); err == nil {
		t.Fatal("Replay accepted a nil canvas")
	}
}

func TestDisplayListReplayDoesNotChangeTargetCanvasState(t *testing.T) {
	list := &DisplayList{}
	list.Save()
	list.Translate(image.Pt(5, 0))
	list.FillRect(image.Rect(0, 0, 1, 1), InkBlack)

	frame := newTestFrame(t, 10, 5)
	canvas := NewCanvas(frame)
	canvas.Translate(image.Pt(1, 2))
	canvas.ClipRect(image.Rect(0, 0, 3, 2))
	if err := list.Replay(canvas); err != nil {
		t.Fatal(err)
	}
	canvas.Set(0, 0, InkRed)
	if canvas.Restore() {
		t.Fatal("Replay leaked its save stack into the target canvas")
	}
	assertInk(t, frame, 1, 2, InkRed)
}
