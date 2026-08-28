package scene_test

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

// inkOf draws a document and reports which pixels it put ink on, which is the
// only thing worth comparing when what is being tested is where a shape went.
func inkOf(t *testing.T, root string) map[image.Point]bool {
	t.Helper()
	source := `{"version":1,"size":{"width":80,"height":80},"background":"white","root":` + root + `}`
	result, err := (scene.Decoder{}).Render(bytes.NewReader([]byte(source)))
	if err != nil {
		t.Fatalf("%s: %v", root, err)
	}
	set := map[image.Point]bool{}
	for y := 0; y < result.Frame.Height(); y++ {
		for x := 0; x < result.Frame.Width(); x++ {
			if ink, _ := result.Frame.InkAt(x, y); ink != display.InkWhite {
				set[image.Pt(x, y)] = true
			}
		}
	}
	return set
}

func same(a, b map[image.Point]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for point := range a {
		if !b[point] {
			return false
		}
	}
	return true
}

// turned wraps a node in a rotation about the middle of the page.
func turned(degrees float64, node string) string {
	return fmt.Sprintf(`{"type":"rotated","degrees":%g,"child":%s}`, degrees, node)
}

// Everything a document can draw turns, which is the point: a rotation is not
// a property some shapes have and others do not, it is what the drawing state
// says and every shape works out its own geometry through it.
func TestEverythingTurns(t *testing.T) {
	nodes := map[string]string{
		"a rectangle":   `{"type":"rectangle","size":{"width":50,"height":20},"fill":"black"}`,
		"a rounded one": `{"type":"rectangle","size":{"width":50,"height":20},"radius":6,"fill":"black"}`,
		"an ellipse":    `{"type":"ellipse","size":{"width":50,"height":20},"fill":"black"}`,
		"a circle":      `{"type":"circle","size":{"width":50,"height":50},"center":{"x":15,"y":15},"radius":10,"fill":"black"}`,
		"a polygon":     `{"type":"polygon","points":[{"x":0,"y":0},{"x":40,"y":0},{"x":20,"y":30}],"fill":"black"}`,
		"a polyline":    `{"type":"polyline","points":[{"x":0,"y":0},{"x":40,"y":10},{"x":10,"y":30}],"stroke":{"ink":"black","width":1}}`,
		"a line":        `{"type":"line","from":{"x":0,"y":0},"to":{"x":50,"y":30},"stroke":{"ink":"black","width":1}}`,
		"an arc":        `{"type":"arc","size":{"width":50,"height":30},"start":-90,"sweep":200,"stroke":{"ink":"black","width":1}}`,
		"a pie":         `{"type":"pie","size":{"width":50,"height":30},"start":-90,"sweep":120,"ink":"black"}`,
		"a chord":       `{"type":"chord","size":{"width":50,"height":30},"start":20,"sweep":220,"ink":"black"}`,
		"a path":        `{"type":"path","fill":"black","commands":[{"op":"move","to":{"x":0,"y":0}},{"op":"line","to":{"x":40,"y":6}},{"op":"cubic","control1":{"x":30,"y":30},"control2":{"x":10,"y":30},"to":{"x":0,"y":0}},{"op":"close"}]}`,
		"a pattern":     `{"type":"pattern","size":{"width":40,"height":40},"rows":["x.",".x"],"inks":{"x":"black"}}`,
		"a pixel":       `{"type":"pixel","at":{"x":10,"y":10},"ink":"black"}`,
		"text":          `{"type":"text","runs":[{"text":"Ag","font":"monaco","size":12}]}`,
	}
	for name, node := range nodes {
		t.Run(name, func(t *testing.T) {
			upright := inkOf(t, node)
			if len(upright) == 0 {
				t.Fatal("it drew nothing to begin with")
			}
			if same(upright, inkOf(t, turned(37, node))) {
				t.Error("turning it by 37 degrees drew exactly what leaving it alone drew")
			}
		})
	}
}

// A turn of nothing is nothing, and a whole turn comes back. Both have to hold
// for every kind of thing, because they are what says the arithmetic composes
// rather than merely producing a picture.
func TestATurnOfNothingAndAWholeTurn(t *testing.T) {
	const node = `{"type":"polygon","points":[{"x":4,"y":4},{"x":44,"y":4},{"x":24,"y":34}],"fill":"black"}`
	upright := inkOf(t, node)
	for _, degrees := range []float64{0, 360, -360, 720} {
		if !same(upright, inkOf(t, turned(degrees, node))) {
			t.Errorf("a turn of %g degrees moved it", degrees)
		}
	}
}

// Two turns are one turn of the sum. This is composition, and it is the
// property a rotation written as an angle instead of a matrix gets wrong.
func TestTurningTwiceIsTurningOnce(t *testing.T) {
	const node = `{"type":"rectangle","size":{"width":40,"height":16},"fill":"black"}`
	twice := inkOf(t, turned(20, turned(25, node)))
	once := inkOf(t, turned(45, node))
	if !same(twice, once) {
		t.Errorf("turning by 25 then 20 covered %d pixels and turning by 45 covered %d",
			len(twice), len(once))
	}
}

// Four quarter turns come back exactly, which is stronger than a whole turn in
// one go: each of the four rounds separately, and a rotation that drifted by a
// pixel a time would show here and nowhere else.
func TestFourQuarterTurnsComeBack(t *testing.T) {
	const node = `{"type":"path","fill":"black","commands":[{"op":"move","to":{"x":6,"y":6}},{"op":"line","to":{"x":40,"y":10}},{"op":"line","to":{"x":14,"y":34}},{"op":"close"}]}`
	if !same(inkOf(t, node), inkOf(t, turned(90, turned(90, turned(90, turned(90, node)))))) {
		t.Error("four quarter turns did not come back to where they started")
	}
}

// A rotation does not change how much room a thing takes, which is what CSS
// does with a transform and the only answer that composes.
func TestATurnDoesNotMoveWhatIsBesideIt(t *testing.T) {
	beside := func(inner string) string {
		return `{"type":"row","children":[` +
			`{"basis":30,"node":` + inner + `},` +
			`{"basis":20,"node":{"type":"rectangle","fill":"red"}}]}`
	}
	const box = `{"type":"rectangle","fill":"black"}`
	upright := inkOf(t, beside(box))
	rotated := inkOf(t, beside(turned(30, box)))
	red := 0
	for point := range upright {
		if point.X >= 30 {
			red++
		}
	}
	moved := 0
	for point := range rotated {
		if point.X >= 30 {
			moved++
		}
	}
	if red == 0 {
		t.Fatal("the box beside it drew nothing")
	}
	// The turned box reaches over, so the count can only grow; what must not
	// happen is the neighbour moving, which would show as its left edge
	// shifting. Its right edge is the page, so its area cannot shrink.
	if moved < red {
		t.Errorf("the neighbour lost %d pixels when the box beside it turned", red-moved)
	}
}

// The origin is settable, as it is in CSS and SVG, and it defaults to the
// centre. Turning about a corner and turning about the middle are different
// pictures, and getting the default wrong would look like everything sliding.
func TestTheOriginIsSettableAndDefaultsToTheCentre(t *testing.T) {
	const node = `{"type":"rectangle","size":{"width":40,"height":40},"fill":"black"}`
	middle := inkOf(t, `{"type":"rotated","degrees":30,"child":`+node+`}`)
	stated := inkOf(t, `{"type":"rotated","degrees":30,"origin":{"x":40,"y":40},"child":`+node+`}`)
	corner := inkOf(t, `{"type":"rotated","degrees":30,"origin":{"x":0,"y":0},"child":`+node+`}`)
	if !same(middle, stated) {
		t.Error("stating the centre drew something other than leaving it out")
	}
	if same(middle, corner) {
		t.Error("turning about a corner drew what turning about the middle drew")
	}
}
