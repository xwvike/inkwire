package display

import (
	"image"
	"math"
	"math/rand"
	"testing"
)

func near(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// The identity is the transform that does nothing, and every other property
// here is stated against it.
func TestIdentityMovesNothing(t *testing.T) {
	for _, point := range [][2]float64{{0, 0}, {3, -7}, {1e6, 0.5}} {
		x, y := Identity().Apply(point[0], point[1])
		near(t, x, point[0], "x")
		near(t, y, point[1], "y")
	}
}

// A transform followed by its inverse is the transform that does nothing. This
// is the one property the whole design rests on: every area primitive works by
// undoing the transform on a pixel and asking the question it always asked, so
// an inverse that is not exactly an inverse moves every filled shape.
func TestATransformAndItsInverseCancel(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	for range 500 {
		m := Rotate(random.Float64()*720-360, random.Float64()*100-50, random.Float64()*100-50).
			Then(Translate(image.Pt(random.Intn(200)-100, random.Intn(200)-100))).
			Then(Scale(random.Float64()*4+0.25, random.Float64()*4+0.25, 3, 5))
		inverse, ok := m.Invert()
		if !ok {
			t.Fatalf("%+v could not be inverted", m)
		}
		x, y := random.Float64()*100-50, random.Float64()*100-50
		there, alsoThere := m.Apply(x, y)
		back, alsoBack := inverse.Apply(there, alsoThere)
		if math.Abs(back-x) > 1e-6 || math.Abs(alsoBack-y) > 1e-6 {
			t.Fatalf("%+v took (%v,%v) to (%v,%v) and back to (%v,%v)", m, x, y, there, alsoThere, back, alsoBack)
		}
	}
}

// A transform that flattens the plane onto a line has nothing to undo it to,
// and saying so beats dividing by zero.
func TestAFlattenedTransformCannotBeInverted(t *testing.T) {
	for _, m := range []Matrix{
		{},
		Scale(0, 1, 0, 0),
		Scale(1, 0, 0, 0),
		{A: 2, B: 4, C: 1, D: 2},
	} {
		if _, ok := m.Invert(); ok {
			t.Errorf("%+v reported an inverse and has none", m)
		}
	}
}

// Composition has to be composition: turning and then turning again is turning
// by the sum, and doing it in the other order about the same point is the same.
func TestTurningTwiceIsTurningByTheSum(t *testing.T) {
	first := Rotate(30, 10, 10)
	second := Rotate(45, 10, 10)
	together := first.Then(second)
	once := Rotate(75, 10, 10)
	for _, point := range [][2]float64{{0, 0}, {20, 5}, {-13, 40}} {
		x, y := together.Apply(point[0], point[1])
		wantX, wantY := once.Apply(point[0], point[1])
		near(t, x, wantX, "x")
		near(t, y, wantY, "y")
	}
}

// Four quarter turns come back to where they started, exactly. Not nearly:
// a quarter turn's sine and cosine are exact, and a drawing turned four times
// that landed a pixel out would be a drawing nobody could turn twice.
func TestFourQuarterTurnsComeBackExactly(t *testing.T) {
	turn := Rotate(90, 8, 8)
	whole := turn.Then(turn).Then(turn).Then(turn)
	for _, point := range [][2]float64{{0, 0}, {16, 3}, {-5, 21}} {
		x, y := whole.Apply(point[0], point[1])
		near(t, x, point[0], "x")
		near(t, y, point[1], "y")
	}
}

// The order matters and has to be the one a nested transform is read in: the
// inner transform first, the outer one after.
func TestOrderIsInnerThenOuter(t *testing.T) {
	move := Translate(image.Pt(10, 0))
	turn := Rotate(90, 0, 0)

	// Moved then turned: (0,0) goes to (10,0), which a quarter turn about the
	// origin takes to (0,10).
	x, y := move.Then(turn).Apply(0, 0)
	near(t, x, 0, "x")
	near(t, y, 10, "y")

	// Turned then moved: (0,0) stays, and moving takes it to (10,0).
	x, y = turn.Then(move).Apply(0, 0)
	near(t, x, 10, "x")
	near(t, y, 0, "y")
}

// A rotation about a point leaves that point alone. Getting this wrong is the
// commonest way a rotation is written, and it shows up as everything sliding.
func TestARotationLeavesItsOwnCentreAlone(t *testing.T) {
	for _, angle := range []float64{0, 1, 37, 90, 180, -60, 359.5} {
		x, y := Rotate(angle, 12, -4).Apply(12, -4)
		near(t, x, 12, "x")
		near(t, y, -4, "y")
	}
}

// A rotation keeps distances, which is what makes it a rotation and not a
// stretch. Anything that scaled would show up here.
func TestARotationKeepsDistances(t *testing.T) {
	random := rand.New(rand.NewSource(2))
	for range 200 {
		m := Rotate(random.Float64()*360, random.Float64()*40, random.Float64()*40)
		x1, y1 := random.Float64()*50, random.Float64()*50
		x2, y2 := random.Float64()*50, random.Float64()*50
		before := math.Hypot(x2-x1, y2-y1)
		ax, ay := m.Apply(x1, y1)
		bx, by := m.Apply(x2, y2)
		near(t, math.Hypot(bx-ax, by-ay), before, "distance")
	}
}

// The fast path is also the promise. A whole number of pixels and nothing else
// has to be recognised, and anything else must not be — a fractional offset or
// a turn taking the fast path would draw the wrong thing very quietly.
func TestOnlyAWholePixelMoveIsAnOffset(t *testing.T) {
	offset, whole := Translate(image.Pt(4, -9)).Offset()
	if !whole || offset != image.Pt(4, -9) {
		t.Errorf("a whole move came back as %v, %v", offset, whole)
	}
	if _, whole := Identity().Offset(); !whole {
		t.Error("doing nothing is a move of nothing")
	}
	for _, m := range []Matrix{
		{A: 1, D: 1, E: 0.5},
		{A: 1, D: 1, F: -0.25},
		Rotate(90, 0, 0),
		Scale(2, 2, 0, 0),
		Rotate(0.0001, 0, 0),
	} {
		if _, whole := m.Offset(); whole {
			t.Errorf("%+v was taken for a whole-pixel move", m)
		}
	}
}

// A quarter turn moves every pixel onto another pixel, so a glyph or a picture
// can be turned by one without resampling. Recognising one that is not is how
// a page would silently come out blurred.
func TestQuartersRecognisesTheExactTurns(t *testing.T) {
	for turns, angle := range map[int]float64{0: 0, 1: 90, 2: 180, 3: 270} {
		got, exact := Rotate(angle, 7, 7).Then(Translate(image.Pt(3, 4))).Quarters()
		if !exact || got != turns {
			t.Errorf("%g degrees came back as %d quarters, exact=%v; want %d", angle, got, exact, turns)
		}
	}
	for _, angle := range []float64{1, 45, 89.9, 91, 200} {
		if _, exact := Rotate(angle, 0, 0).Quarters(); exact {
			t.Errorf("%g degrees was taken for a quarter turn", angle)
		}
	}
	if _, exact := Scale(2, 2, 0, 0).Quarters(); exact {
		t.Error("a magnification was taken for a quarter turn")
	}
}

// The box a transform maps another box to has to hold every point of it, or a
// drawing gets clipped by the loop that was meant to cover it.
func TestAMappedBoxHoldsEverythingInside(t *testing.T) {
	random := rand.New(rand.NewSource(3))
	box := image.Rect(-3, 5, 41, 29)
	for range 200 {
		m := Rotate(random.Float64()*360, random.Float64()*40, random.Float64()*40)
		mapped := m.MapRect(box)
		for _, corner := range [][2]int{
			{box.Min.X, box.Min.Y}, {box.Max.X - 1, box.Min.Y},
			{box.Min.X, box.Max.Y - 1}, {box.Max.X - 1, box.Max.Y - 1},
			{(box.Min.X + box.Max.X) / 2, (box.Min.Y + box.Max.Y) / 2},
		} {
			moved := m.ApplyPoint(image.Pt(corner[0], corner[1]))
			if !moved.In(mapped) {
				t.Fatalf("%v maps into %v, which does not hold %v", box, mapped, moved)
			}
		}
	}
}

// A whole-pixel move maps a box exactly, with none of the slack a turn needs.
func TestAMovedBoxIsExact(t *testing.T) {
	box := image.Rect(2, 3, 10, 20)
	if got := Translate(image.Pt(5, -1)).MapRect(box); got != box.Add(image.Pt(5, -1)) {
		t.Errorf("a moved box came back as %v", got)
	}
}

// A pixel is a square and its coordinate names a corner, so a point is moved
// from its centre. Moving the corner instead drifts a turned drawing half a
// pixel up and left, which is the mistake every implementation makes once.
func TestAPixelIsMovedFromItsCentre(t *testing.T) {
	// A quarter turn about the centre of a two-by-two square swaps its pixels
	// around the middle. Sampling corners would leave one of them outside.
	turn := Rotate(90, 1, 1)
	got := map[image.Point]bool{}
	for y := range 2 {
		for x := range 2 {
			got[turn.ApplyPoint(image.Pt(x, y))] = true
		}
	}
	if len(got) != 4 {
		t.Fatalf("a quarter turn of four pixels landed on %d places: %v", len(got), got)
	}
}
