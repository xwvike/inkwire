package scene_test

import (
	"bytes"
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

// A field that decodes and reaches nothing is a field nobody can use, and the
// schema has had two of those before. These draw the same shape twice, once
// turned and once not, and require the ink to move.
func drawn(t *testing.T, node string) map[image.Point]bool {
	t.Helper()
	source := `{"version":1,"size":{"width":64,"height":64},"background":"white","root":` +
		`{"type":"absolute","children":[{"bounds":{"x":8,"y":20,"width":48,"height":24},"node":` + node + `}]}}`
	result, err := (scene.Decoder{}).Render(bytes.NewReader([]byte(source)))
	if err != nil {
		t.Fatalf("%s: %v", node, err)
	}
	set := map[image.Point]bool{}
	for y := 0; y < result.Frame.Height(); y++ {
		for x := 0; x < result.Frame.Width(); x++ {
			if ink, _ := result.Frame.InkAt(x, y); ink != display.InkWhite {
				set[image.Pt(x, y)] = true
			}
		}
	}
	if len(set) == 0 {
		t.Fatalf("%s drew nothing", node)
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

// Every node the rotation reaches, each drawn upright and turned. A shape that
// came out the same either did not read the field or did nothing with it.
func TestRotationReachesEveryShapeThatHasOne(t *testing.T) {
	tests := map[string][2]string{
		"ellipse": {
			`{"type":"ellipse","fill":"black"}`,
			`{"type":"ellipse","rotation":40,"fill":"black"}`},
		"a stroked ellipse": {
			`{"type":"ellipse","stroke":{"ink":"black","width":1}}`,
			`{"type":"ellipse","rotation":40,"stroke":{"ink":"black","width":1}}`},
		"arc": {
			`{"type":"arc","start":-90,"sweep":200,"stroke":{"ink":"black","width":1}}`,
			`{"type":"arc","start":-90,"sweep":200,"rotation":40,"stroke":{"ink":"black","width":1}}`},
		"pie": {
			`{"type":"pie","start":-90,"sweep":120,"ink":"black"}`,
			`{"type":"pie","start":-90,"sweep":120,"rotation":40,"ink":"black"}`},
		"chord": {
			`{"type":"chord","start":20,"sweep":220,"ink":"black"}`,
			`{"type":"chord","start":20,"sweep":220,"rotation":40,"ink":"black"}`},
		"an arc inside a path": {
			`{"type":"path","fill":"black","commands":[{"op":"move","to":{"x":0,"y":12}},` +
				`{"op":"arc","bounds":{"x":0,"y":0,"width":46,"height":24},"start":180,"sweep":180},{"op":"close"}]}`,
			`{"type":"path","fill":"black","commands":[{"op":"move","to":{"x":0,"y":12}},` +
				`{"op":"arc","bounds":{"x":0,"y":0,"width":46,"height":24},"start":180,"sweep":180,"rotation":40},{"op":"close"}]}`},
	}
	for name, pair := range tests {
		t.Run(name, func(t *testing.T) {
			if same(drawn(t, pair[0]), drawn(t, pair[1])) {
				t.Error("turning it drew exactly what leaving it alone drew")
			}
		})
	}
}

// Absent means upright, so every document written before there was a rotation
// draws what it always drew. Stating zero has to mean the same thing.
func TestNoRotationAndZeroAreTheSame(t *testing.T) {
	for name, pair := range map[string][2]string{
		"ellipse": {`{"type":"ellipse","fill":"black"}`, `{"type":"ellipse","rotation":0,"fill":"black"}`},
		"pie": {`{"type":"pie","start":-90,"sweep":120,"ink":"black"}`,
			`{"type":"pie","start":-90,"sweep":120,"rotation":0,"ink":"black"}`},
	} {
		t.Run(name, func(t *testing.T) {
			if !same(drawn(t, pair[0]), drawn(t, pair[1])) {
				t.Error("saying nothing and saying zero drew different pictures")
			}
		})
	}
}

// A whole turn is no turn, whichever way round and however many times, because
// an ellipse is symmetric about both of its axes.
func TestATurnThatComesBackToWhereItStarted(t *testing.T) {
	upright := drawn(t, `{"type":"ellipse","fill":"black"}`)
	for _, rotation := range []string{"180", "-180", "360", "-360", "540"} {
		if !same(upright, drawn(t, `{"type":"ellipse","rotation":`+rotation+`,"fill":"black"}`)) {
			t.Errorf("a rotation of %s moved an ellipse that has both axes symmetric", rotation)
		}
	}
}

// A rotation on a shape that has no ellipse behind it is a field that would do
// nothing, and the schema refuses a field it would ignore rather than letting
// an author believe it applied.
func TestRotationIsRefusedWhereItWouldMeanNothing(t *testing.T) {
	for _, node := range []string{
		`{"type":"rectangle","rotation":30,"fill":"black"}`,
		`{"type":"circle","center":{"x":10,"y":10},"radius":8,"rotation":30,"fill":"black"}`,
		`{"type":"polygon","rotation":30,"points":[{"x":0,"y":0},{"x":9,"y":0},{"x":4,"y":9}],"fill":"black"}`,
	} {
		source := `{"version":1,"size":{"width":64,"height":64},"root":` + node + `}`
		if _, err := (scene.Decoder{}).Decode(bytes.NewReader([]byte(source))); err == nil {
			t.Errorf("%s was accepted, and the rotation would have done nothing", node)
		}
	}
}
