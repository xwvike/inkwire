package display

import (
	"image"
	"testing"
)

func TestMaskRasterizeAndAccess(t *testing.T) {
	bounds := image.Rect(3, 5, 9, 11)
	m := rasterizeMask(bounds, func(x, y int) bool { return x < 6 })
	if got, want := m.count(), 3*6; got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
	if !m.at(3, 5) || !m.at(5, 10) {
		t.Fatal("a pixel inside the predicate is not set")
	}
	if m.at(6, 5) {
		t.Fatal("a pixel outside the predicate is set")
	}
	for _, outside := range []image.Point{{X: 2, Y: 5}, {X: 9, Y: 5}, {X: 3, Y: 4}, {X: 3, Y: 11}} {
		if m.at(outside.X, outside.Y) {
			t.Errorf("%v is outside the mask but reads as set", outside)
		}
	}
	// Writing outside the bounds must not corrupt anything or panic.
	before := m.count()
	m.set(-40, -40, true)
	m.set(400, 400, true)
	if m.count() != before {
		t.Fatal("a write outside the bounds changed the mask")
	}
}

func TestMaskEmptyBoundsAreInert(t *testing.T) {
	m := newMask(image.Rectangle{})
	if !m.empty() || m.count() != 0 {
		t.Fatal("a zero-size mask is not empty")
	}
	if m.at(0, 0) {
		t.Fatal("a zero-size mask reports a set pixel")
	}
	if eroded := m.erode(3); !eroded.empty() {
		t.Fatal("eroding a zero-size mask produced pixels")
	}
	if band := m.innerBand(2); band.count() != 0 {
		t.Fatal("a zero-size mask produced an inner band")
	}
}

func TestMaskErodeInsetsARectangle(t *testing.T) {
	bounds := image.Rect(0, 0, 20, 20)
	m := rasterizeMask(bounds, func(x, y int) bool { return image.Pt(x, y).In(image.Rect(4, 4, 16, 16)) })
	for _, radius := range []int{1, 2, 3} {
		eroded := m.erode(radius)
		want := image.Rect(4+radius, 4+radius, 16-radius, 16-radius)
		if got := eroded.count(); got != want.Dx()*want.Dy() {
			t.Errorf("radius %d: eroded count = %d, want %d", radius, got, want.Dx()*want.Dy())
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if got, expected := eroded.at(x, y), image.Pt(x, y).In(want); got != expected {
					t.Fatalf("radius %d: pixel (%d,%d) = %v, want %v", radius, x, y, got, expected)
				}
			}
		}
	}
}

// The whole point of building a stroke by erosion: the band cannot escape the
// shape, because it is the shape minus a subset of itself.
func TestMaskInnerBandStaysInsideTheShape(t *testing.T) {
	bounds := image.Rect(0, 0, 64, 64)
	shapes := map[string]func(x, y int) bool{
		"circle":  func(x, y int) bool { return pointInCircle(x, y, image.Pt(32, 32), 25) },
		"ellipse": func(x, y int) bool { return Upright(image.Rect(4, 12, 60, 52)).contains(x, y) },
		"polygon": func(x, y int) bool {
			return pointInPolygon(image.Pt(x, y), []image.Point{{X: 8, Y: 8}, {X: 56, Y: 18}, {X: 44, Y: 58}, {X: 12, Y: 46}})
		},
	}
	for name, inside := range shapes {
		fill := rasterizeMask(bounds, inside)
		for _, width := range []int{1, 2, 3, 6} {
			band := fill.innerBand(width)
			if band.count() == 0 {
				t.Fatalf("%s width %d: the band is empty", name, width)
			}
			band.each(func(x, y int) {
				if !fill.at(x, y) {
					t.Fatalf("%s width %d: band pixel (%d,%d) is outside the fill", name, width, x, y)
				}
			})
		}
	}
}

// Rectangles are the case where the existing implicit stroker and an eroded
// band already agree, so the band can be trusted against a known-good result.
func TestMaskInnerBandMatchesStrokeRect(t *testing.T) {
	const size = 48
	rect := image.Rect(6, 9, 42, 39)
	for _, width := range []int{1, 2, 3, 5, 6} {
		frame := newTestFrame(t, size, size)
		NewCanvas(frame).StrokeRect(rect, StrokeStyle{Ink: InkBlack, Width: width})

		fill := rasterizeMask(image.Rect(0, 0, size, size), func(x, y int) bool { return image.Pt(x, y).In(rect) })
		band := fill.innerBand(width)

		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				ink, _ := frame.InkAt(x, y)
				if got, want := band.at(x, y), ink == InkBlack; got != want {
					t.Fatalf("width %d: pixel (%d,%d) band=%v StrokeRect=%v", width, x, y, got, want)
				}
			}
		}
	}
}

// A shape cut off by the clip must not grow a stroke along the cut. The mask
// has to follow the shape past the clip so erosion sees real geometry there.
func TestStrokeMaskBoundsFollowTheShapeNotTheClip(t *testing.T) {
	shape := image.Rect(-50, -50, 150, 150)
	clip := image.Rect(10, 10, 50, 50)
	const width = 3

	bounds := strokeMaskBounds(shape, clip, width)
	if bounds.Min.X >= clip.Min.X || bounds.Max.X <= clip.Max.X {
		t.Fatalf("mask bounds %v do not extend past the clip %v", bounds, clip)
	}
	if !bounds.In(shape) {
		t.Fatalf("mask bounds %v escaped the shape %v", bounds, shape)
	}

	fill := rasterizeMask(bounds, func(x, y int) bool { return image.Pt(x, y).In(shape) })
	band := fill.innerBand(width)
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		for x := clip.Min.X; x < clip.Max.X; x++ {
			if band.at(x, y) {
				t.Fatalf("pixel (%d,%d) was stroked along the clip edge, but the shape has no edge there", x, y)
			}
		}
	}

	// The same shape ending exactly at the clip does produce an edge there.
	ending := image.Rect(-50, -50, 30, 150)
	fill = rasterizeMask(strokeMaskBounds(ending, clip, width), func(x, y int) bool { return image.Pt(x, y).In(ending) })
	if !fill.innerBand(width).at(29, 20) {
		t.Fatal("a real edge inside the clip was not stroked")
	}
}
