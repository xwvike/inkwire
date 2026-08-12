package display

import (
	"image"
	"image/color"
	"testing"
)

func ringPath(outer, inner image.Rectangle) Path {
	var path Path
	for _, rect := range []image.Rectangle{outer, inner} {
		path.MoveTo(rect.Min)
		path.LineTo(image.Pt(rect.Max.X-1, rect.Min.Y))
		path.LineTo(rect.Max.Sub(image.Pt(1, 1)))
		path.LineTo(image.Pt(rect.Min.X, rect.Max.Y-1))
		path.Close()
	}
	return path
}

func TestClipPathRestrictsDrawingToTheRegion(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)

	var circle Path
	circle.Arc(image.Rect(8, 8, 32, 32), 0, 360)
	canvas.ClipPath(circle)
	canvas.FillRect(frame.Bounds(), InkBlack)

	assertInk(t, frame, 20, 20, InkBlack) // centre of the circle
	assertInk(t, frame, 1, 1, InkWhite)   // outside its bounding box
	assertInk(t, frame, 9, 9, InkWhite)   // inside the box, outside the circle
}

// Nested contours are resolved with the even-odd rule, so a clip can be a ring.
func TestClipPathCutsHolesWithEvenOdd(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)
	canvas.ClipPath(ringPath(image.Rect(6, 6, 34, 34), image.Rect(14, 14, 26, 26)))
	canvas.FillRect(frame.Bounds(), InkRed)

	assertInk(t, frame, 8, 20, InkRed)    // in the ring
	assertInk(t, frame, 20, 20, InkWhite) // in the hole
	assertInk(t, frame, 2, 20, InkWhite)  // outside the ring
}

// Same contract as ClipRect: the region is pinned in frame coordinates, so a
// later translation moves what is drawn, not where drawing is allowed.
func TestClipPathIsFixedInFrameCoordinates(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)

	var square Path
	square.MoveTo(image.Pt(4, 4))
	square.LineTo(image.Pt(19, 4))
	square.LineTo(image.Pt(19, 19))
	square.LineTo(image.Pt(4, 19))
	square.Close()
	canvas.ClipPath(square)
	canvas.Translate(image.Pt(10, 10))
	canvas.FillRect(image.Rect(0, 0, 30, 30), InkBlack)

	assertInk(t, frame, 15, 15, InkBlack) // inside both the clip and the shifted rect
	assertInk(t, frame, 25, 25, InkWhite) // shifted rect reaches here, the clip does not
	assertInk(t, frame, 5, 5, InkWhite)   // clip allows it, the shifted rect misses it
}

func TestClipPathComposesWithRectAndSurvivesRestore(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)

	var circle Path
	circle.Arc(image.Rect(4, 4, 36, 36), 0, 360)

	canvas.Save()
	canvas.ClipPath(circle)
	canvas.ClipRect(image.Rect(0, 0, 20, 40))
	canvas.FillRect(frame.Bounds(), InkBlack)
	if !canvas.Restore() {
		t.Fatal("Restore did not pop the clip")
	}
	canvas.FillRect(image.Rect(30, 0, 34, 4), InkRed)

	assertInk(t, frame, 10, 20, InkBlack) // left half of the circle
	assertInk(t, frame, 28, 20, InkWhite) // right half, cut by the rectangle
	assertInk(t, frame, 31, 1, InkRed)    // restored canvas draws outside the circle again
}

func TestClipPathOnChildCanvasDoesNotReachTheParent(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	parent := NewCanvas(frame)

	var circle Path
	circle.Arc(image.Rect(4, 4, 36, 36), 0, 360)
	parent.ClipPath(circle)

	// The wedge has to reach into the circle for the case to mean anything:
	// the circle is centred near (19.5,19.5) with a radius of 15.5.
	child := parent.Clip(image.Rect(0, 0, 20, 20))
	var wedge Path
	wedge.MoveTo(image.Pt(0, 0))
	wedge.LineTo(image.Pt(19, 0))
	wedge.LineTo(image.Pt(0, 19))
	wedge.Close()
	child.ClipPath(wedge)
	child.FillRect(frame.Bounds(), InkBlack)

	// The parent still sees only the circle, not the child's wedge.
	parent.FillRect(image.Rect(24, 24, 30, 30), InkRed)
	assertInk(t, frame, 26, 26, InkRed)
	assertInk(t, frame, 9, 9, InkBlack)   // inside the circle, the wedge and the child rect
	assertInk(t, frame, 18, 18, InkWhite) // inside circle and child rect, outside the wedge
	assertInk(t, frame, 1, 1, InkWhite)   // inside the wedge, outside the circle the parent set
}

// Everything reaches the frame through Canvas.Set, so images and text obey a
// path clip without either of them knowing it exists.
func TestClipPathAppliesToImagesAndText(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)

	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			source.SetNRGBA(x, y, color.NRGBA{A: 0xff})
		}
	}

	var circle Path
	circle.Arc(image.Rect(4, 4, 36, 36), 0, 360)
	canvas.ClipPath(circle)
	if err := canvas.DrawImage(source, frame.Bounds(), ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := canvas.DrawTextBox(builtinRegistry(t), TextBox{
		Bounds: image.Rect(0, 0, 40, 16),
		Runs:   []TextRun{{Text: "AB", Style: TextStyle{Font: "monaco", Size: 10, Ink: InkRed}}},
	}); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 1, 1, InkWhite)   // the image covered the whole frame, the clip did not
	assertInk(t, frame, 20, 20, InkBlack) // image inside the circle
	assertInk(t, frame, 0, 5, InkWhite)   // text starts outside the circle and is cut
}

func TestClipPathWithNoContoursDrawsNothing(t *testing.T) {
	frame := newTestFrame(t, 16, 16)
	canvas := NewCanvas(frame)
	canvas.ClipPath(Path{})
	canvas.FillRect(frame.Bounds(), InkBlack)
	if got := countInk(frame, InkBlack); got != 0 {
		t.Fatalf("an empty clip path painted %d pixels", got)
	}
}

func TestDisplayListReplaysClipPath(t *testing.T) {
	const size = 40
	ring := ringPath(image.Rect(6, 6, 34, 34), image.Rect(14, 14, 26, 26))

	direct := newTestFrame(t, size, size)
	canvas := NewCanvas(direct)
	canvas.Save()
	canvas.Translate(image.Pt(2, 1))
	canvas.ClipPath(ring)
	canvas.FillRect(image.Rect(-10, -10, 50, 50), InkBlack)
	canvas.Restore()

	list := &DisplayList{}
	list.Save()
	list.Translate(image.Pt(2, 1))
	list.ClipPath(ring)
	list.FillRect(image.Rect(-10, -10, 50, 50), InkBlack)
	list.Restore()

	replayed := newTestFrame(t, size, size)
	if err := list.Replay(NewCanvas(replayed)); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a, _ := direct.InkAt(x, y)
			b, _ := replayed.InkAt(x, y)
			if a != b {
				t.Fatalf("pixel (%d,%d): replay = %d, canvas = %d", x, y, b, a)
			}
		}
	}
	if countInk(replayed, InkBlack) == 0 {
		t.Fatal("the clipped fill painted nothing")
	}
	bounds := list.Bounds()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if ink, _ := replayed.InkAt(x, y); ink != InkWhite && !image.Pt(x, y).In(bounds) {
				t.Fatalf("painted pixel (%d,%d) falls outside recorded bounds %v", x, y, bounds)
			}
		}
	}
}

// Recording a clip must snapshot the path, like every other mutable input.
func TestDisplayListSnapshotsClipPath(t *testing.T) {
	ring := ringPath(image.Rect(4, 4, 28, 28), image.Rect(12, 12, 20, 20))
	list := &DisplayList{}
	list.ClipPath(ring)
	list.FillRect(image.Rect(0, 0, 32, 32), InkBlack)
	ring.Reset()

	frame := newTestFrame(t, 32, 32)
	if err := list.Replay(NewCanvas(frame)); err != nil {
		t.Fatal(err)
	}
	assertInk(t, frame, 6, 16, InkBlack)
	assertInk(t, frame, 16, 16, InkWhite)
}
