package scene

import (
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// The nodes below were reachable from compose long before they were reachable
// from a document. These tests are what "reachable" has to mean: not that the
// decoder accepts the spelling, but that the drawing changes because of it.

func renderScene(t *testing.T, document string) Result {
	t.Helper()
	result, err := (Decoder{}).Render(strings.NewReader(document))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(result.Report.Warnings) != 0 {
		t.Fatalf("warnings: %v", result.Report.Warnings)
	}
	return result
}

func inkAt(t *testing.T, result Result, x, y int) display.Ink {
	t.Helper()
	ink, ok := result.Frame.InkAt(x, y)
	if !ok {
		t.Fatalf("(%d,%d) is outside the frame", x, y)
	}
	return ink
}

func assertInk(t *testing.T, result Result, x, y int, want display.Ink, why string) {
	t.Helper()
	if got := inkAt(t, result, x, y); got != want {
		t.Errorf("(%d,%d) = %v, want %v: %s", x, y, got, want, why)
	}
}

// An automatic track takes the width of what is in it and a fractional track
// takes what is left, which is the whole reason to reach for a grid.
func TestGridSizesAutomaticAndFractionalTracks(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "grid",
			"columns": ["auto", "1fr"],
			"rows": ["auto"],
			"columnGap": 6,
			"children": [
				{"node": {"type": "rectangle", "size": {"width": 10, "height": 8}, "fill": "black"}},
				{"node": {"type": "rectangle", "size": {"height": 8}, "fill": "red"}}
			]
		}
	}`)

	assertInk(t, result, 9, 0, display.InkBlack, "the automatic column is as wide as its content")
	assertInk(t, result, 10, 0, display.InkWhite, "the gap separates the two columns")
	assertInk(t, result, 15, 0, display.InkWhite, "the gap is six wide")
	assertInk(t, result, 16, 0, display.InkRed, "the fractional column starts after the gap")
	assertInk(t, result, display.GiciskyWidth-1, 0, display.InkRed, "and runs to the far edge")
}

// A stated span reaches across tracks it was not placed in.
func TestGridPlacesAndSpans(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "grid",
			"columns": [20, 20, 20],
			"rows": [10, 10],
			"children": [
				{"column": 2, "row": 1, "node": {"type": "rectangle", "fill": "black"}},
				{"column": 1, "row": 2, "columnSpan": 3, "node": {"type": "rectangle", "fill": "red"}}
			]
		}
	}`)

	assertInk(t, result, 0, 0, display.InkWhite, "the first cell was skipped over")
	assertInk(t, result, 25, 5, display.InkBlack, "the child landed in the column it named")
	assertInk(t, result, 45, 5, display.InkWhite, "and no further")
	assertInk(t, result, 0, 15, display.InkRed, "the span starts at the first column")
	assertInk(t, result, 59, 15, display.InkRed, "and covers all three")
}

// Magnifying is not the same as drawing something larger: the child draws at
// its own size and every pixel of it becomes a block.
func TestTransformedMagnifiesTheChildsDrawing(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 4, "height": 4},
				"node": {
					"type": "transformed", "scale": 2,
					"child": {"type": "pixel", "at": {"x": 0, "y": 0}, "size": {"width": 2, "height": 2}, "ink": "black"}
				}
			}]
		}
	}`)

	for _, at := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		assertInk(t, result, at[0], at[1], display.InkBlack, "one pixel became a two by two block")
	}
	assertInk(t, result, 2, 0, display.InkWhite, "the block is only two wide")
	assertInk(t, result, 0, 2, display.InkWhite, "and only two tall")
}

// Half a turn is the one rotation whose result does not depend on which way
// round the quarter turns go, so it checks the wiring without restating what
// display already tests about direction.
func TestTransformedTurnsTheChild(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 4, "height": 4},
				"node": {
					"type": "transformed", "turns": 2,
					"child": {"type": "pixel", "at": {"x": 0, "y": 0}, "size": {"width": 4, "height": 4}, "ink": "black"}
				}
			}]
		}
	}`)

	assertInk(t, result, 3, 3, display.InkBlack, "half a turn moves the corner pixel to the far corner")
	assertInk(t, result, 0, 0, display.InkWhite, "and away from where it was drawn")
}

func TestClipShapeConfinesToACircle(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 21, "height": 21},
				"node": {
					"type": "clipShape",
					"shape": {"kind": "circle", "radius": "50%"},
					"child": {"type": "rectangle", "fill": "black"}
				}
			}]
		}
	}`)

	assertInk(t, result, 10, 10, display.InkBlack, "the middle of the circle is filled")
	assertInk(t, result, 0, 0, display.InkWhite, "the corner is outside it")
	assertInk(t, result, 20, 20, display.InkWhite, "as is the opposite corner")
}

func TestClipShapeConfinesToAnInset(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 20, "height": 20},
				"node": {
					"type": "clipShape",
					"shape": {"kind": "inset", "insets": ["25%", "25%", "25%", "25%"]},
					"child": {"type": "rectangle", "fill": "black"}
				}
			}]
		}
	}`)

	assertInk(t, result, 10, 10, display.InkBlack, "the middle survives the inset")
	assertInk(t, result, 4, 10, display.InkWhite, "a quarter is taken off the left")
	assertInk(t, result, 10, 4, display.InkWhite, "and off the top")
	assertInk(t, result, 15, 10, display.InkWhite, "and off the right")
	assertInk(t, result, 14, 10, display.InkBlack, "which leaves exactly ten columns, not eleven")
}

// A corner is the branch where the outline is arcs rather than lines, and the
// thing worth checking is the same either way: it stays inside the box.
func TestClipShapeRoundsTheCornersOfAnInset(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 20, "height": 20},
				"node": {
					"type": "clipShape",
					"shape": {"kind": "inset", "insets": [0, 0, 0, 0], "corner": 6},
					"child": {"type": "rectangle", "fill": "black"}
				}
			}]
		}
	}`)

	assertInk(t, result, 10, 10, display.InkBlack, "the middle is untouched by rounding")
	assertInk(t, result, 0, 0, display.InkWhite, "the corner is cut away")
	assertInk(t, result, 19, 19, display.InkWhite, "as is the far corner")
	assertInk(t, result, 10, 19, display.InkBlack, "the last row is still reached between the corners")
	assertInk(t, result, 19, 10, display.InkBlack, "and so is the last column")
}

// An inset from the far edge is the case Absolute cannot express, because it
// is not a number until the container has been laid out.
func TestAnchoredResolvesInsetsAgainstItsContainer(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 100, "height": 40},
				"node": {
					"type": "anchored",
					"children": [
						{"left": "50%", "top": 0, "width": 10, "height": 10,
						 "node": {"type": "rectangle", "fill": "black"}},
						{"right": 0, "bottom": 0, "width": 10, "height": 10,
						 "node": {"type": "rectangle", "fill": "red"}}
					]
				}
			}]
		}
	}`)

	assertInk(t, result, 50, 0, display.InkBlack, "half of a hundred is fifty")
	assertInk(t, result, 49, 0, display.InkWhite, "and not a pixel before it")
	assertInk(t, result, 99, 39, display.InkRed, "an inset of zero from the far corner sits in it")
	assertInk(t, result, 90, 30, display.InkRed, "and the box is ten square")
	assertInk(t, result, 89, 30, display.InkWhite, "reaching no further left than that")
}

func TestAnchoredPaintsHigherLayersLast(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "anchored",
			"children": [
				{"left": 0, "top": 0, "width": 20, "height": 20, "layer": 5,
				 "node": {"type": "rectangle", "fill": "red"}},
				{"left": 0, "top": 0, "width": 20, "height": 20, "layer": 1,
				 "node": {"type": "rectangle", "fill": "black"}}
			]
		}
	}`)

	assertInk(t, result, 5, 5, display.InkRed, "the higher layer wins regardless of document order")
}

// Ratio derives the cross size from the main one, which is the direction
// compose supports: a child that states a cross size has already answered the
// question, so the ratio stands down rather than contradicting it.
func TestLayoutChildHonoursTheRatioBetweenTheAxes(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "row",
			"crossAlign": "start",
			"children": [
				{"basis": 20, "ratio": 2, "node": {"type": "rectangle", "fill": "black"}}
			]
		}
	}`)

	assertInk(t, result, 19, 9, display.InkBlack, "twenty wide at two to one is ten tall")
	assertInk(t, result, 20, 9, display.InkWhite, "and no wider than the basis")
	assertInk(t, result, 19, 10, display.InkWhite, "nor taller than the ratio allows")
}

// The four limits and an explicit cross size reach compose too, and a maximum
// that bites is the plainest way to see that they arrived.
func TestLayoutChildHonoursItsLimits(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "row",
			"crossAlign": "start",
			"children": [
				{"basis": 40, "maxMain": 12, "cross": 30, "minCross": 8, "maxCross": 10,
				 "node": {"type": "rectangle", "fill": "black"}}
			]
		}
	}`)

	assertInk(t, result, 11, 9, display.InkBlack, "the maximum won over the basis")
	assertInk(t, result, 12, 9, display.InkWhite, "so twelve is the last column")
	assertInk(t, result, 11, 10, display.InkWhite, "and the cross maximum capped the height at ten")
}

func TestSchemaRejectsWhatItCannotHonour(t *testing.T) {
	tests := []struct {
		name string
		root string
		want string
	}{
		{
			"a track spelled as something else",
			`{"type":"grid","columns":["fill"],"children":[]}`,
			"a track is",
		},
		{
			"a fraction that is not a whole number",
			`{"type":"grid","columns":["1.5fr"],"children":[]}`,
			"not a fraction",
		},
		{
			"a shape nobody can draw",
			`{"type":"clipShape","shape":{"kind":"squircle"},"child":{"type":"rectangle"}}`,
			"squircle",
		},
		{
			"an inset without four sides",
			`{"type":"clipShape","shape":{"kind":"inset","insets":[1,2]},"child":{"type":"rectangle"}}`,
			"four lengths",
		},
		{
			"a polygon that cannot enclose anything",
			`{"type":"clipShape","shape":{"kind":"polygon","points":[{"x":0,"y":0},{"x":1,"y":1}]},"child":{"type":"rectangle"}}`,
			"at least three corners",
		},
		{
			"a centre on a shape that has none",
			`{"type":"clipShape","shape":{"kind":"inset","insets":[0,0,0,0],"center":{"x":1,"y":1}},"child":{"type":"rectangle"}}`,
			"only a circle or an ellipse",
		},
		{
			"a scale below nothing",
			`{"type":"transformed","scale":-2,"child":{"type":"rectangle"}}`,
			"must not be negative",
		},
		{
			"an alignment that is not one",
			`{"type":"grid","children":[{"alignSelf":"middle","node":{"type":"rectangle"}}]}`,
			"middle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Decoder{}).Decode(strings.NewReader(`{"version":1,"root":` + test.root + `}`))
			if err == nil {
				t.Fatal("decoded without complaint")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

// calc is the one length that is neither a share of the container nor a fixed
// number of pixels but both, and compose has held it since the length model
// was written. Nothing could ask for it until now.
//
// The spaces are optional, and both spellings have to mean the same thing or
// one of them is a trap.
func TestCalcSubtractsPixelsFromAShare(t *testing.T) {
	for _, spelling := range []string{"calc(100% - 10px)", "calc(100%-10px)"} {
		t.Run(spelling, func(t *testing.T) {
			result := renderScene(t, `{
				"version": 1, "background": "white",
				"root": {
					"type": "absolute",
					"children": [{
						"bounds": {"x": 0, "y": 0, "width": 100, "height": 10},
						"node": {
							"type": "row",
							"crossAlign": "start",
							"children": [
								{"basis": "`+spelling+`", "cross": 10,
								 "node": {"type": "rectangle", "fill": "black"}}
							]
						}
					}]
				}
			}`)

			assertInk(t, result, 89, 5, display.InkBlack, "all of a hundred but ten is ninety")
			assertInk(t, result, 90, 5, display.InkWhite, "and not a pixel more")
		})
	}
}

func TestCalcAddsPixelsToAShareEitherWayRound(t *testing.T) {
	for _, spelling := range []string{"calc(50% + 8px)", "calc(8px + 50%)"} {
		t.Run(spelling, func(t *testing.T) {
			result := renderScene(t, `{
				"version": 1, "background": "white",
				"root": {
					"type": "absolute",
					"children": [{
						"bounds": {"x": 0, "y": 0, "width": 100, "height": 10},
						"node": {
							"type": "row",
							"crossAlign": "start",
							"children": [
								{"basis": "`+spelling+`", "cross": 10,
								 "node": {"type": "rectangle", "fill": "black"}}
							]
						}
					}]
				}
			}`)

			assertInk(t, result, 57, 5, display.InkBlack, "half of a hundred plus eight is fifty-eight")
			assertInk(t, result, 58, 5, display.InkWhite, "and no wider")
		})
	}
}

// A grid track is a length, so it gets calc without being told about it.
func TestATrackCanBeCalculatedToo(t *testing.T) {
	result := renderScene(t, `{
		"version": 1, "background": "white",
		"root": {
			"type": "absolute",
			"children": [{
				"bounds": {"x": 0, "y": 0, "width": 100, "height": 10},
				"node": {
					"type": "grid",
					"columns": ["calc(100% - 30px)", "1fr"],
					"rows": [10],
					"children": [
						{"node": {"type": "rectangle", "fill": "black"}},
						{"node": {"type": "rectangle", "fill": "red"}}
					]
				}
			}]
		}
	}`)

	assertInk(t, result, 69, 5, display.InkBlack, "the calculated track is seventy wide")
	assertInk(t, result, 70, 5, display.InkRed, "and the fraction takes the thirty that are left")
}

func TestCalcRefusesWhatALengthCannotHold(t *testing.T) {
	tests := []struct {
		name  string
		basis string
		want  string
	}{
		{"a sign that is not one", `"calc(100% * 2px)"`, "a + or -"},
		{"two percentages", `"calc(50% + 20%)"`, "not a number of pixels"},
		{"pixels first and subtracted", `"calc(30px - 50%)"`, "not a length here"},
		{"an unclosed bracket", `"calc(100% - 10px"`, "closing bracket"},
		{"a bare unit", `"10px"`, "not a length"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Decoder{}).Decode(strings.NewReader(
				`{"version":1,"root":{"type":"row","children":[{"basis":` + test.basis +
					`,"node":{"type":"rectangle"}}]}}`))
			if err == nil {
				t.Fatal("decoded without complaint")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

// A field that is read, accepted and then ignored is the worst kind, because
// it was written in good faith and the page comes out wrong somewhere else.
// These three used to be exactly that.
func TestSchemaRefusesFieldsItWouldOtherwiseIgnore(t *testing.T) {
	tests := []struct {
		name string
		root string
		want string
	}{
		{
			"a size where absolute gives the bounds",
			`{"type":"absolute","children":[{"bounds":{"x":0,"y":0,"width":40,"height":20},
			  "node":{"type":"rectangle","size":{"width":10,"height":10},"fill":"black"}}]}`,
			"leaves nothing for a size to do",
		},
		{
			"a size where anchored gives the insets",
			`{"type":"anchored","children":[{"left":0,"top":0,"width":40,"height":20,
			  "node":{"type":"rectangle","size":{"width":10,"height":10},"fill":"black"}}]}`,
			"leaves nothing for a size to do",
		},
		{
			"a ratio beside the cross size it would have worked out",
			`{"type":"row","children":[{"basis":20,"cross":10,"ratio":2,
			  "node":{"type":"rectangle"}}]}`,
			"give one or the other",
		},
		{
			"both horizontal edges and a width",
			`{"type":"anchored","children":[{"left":4,"right":4,"width":20,
			  "node":{"type":"rectangle"}}]}`,
			"cannot all hold at once",
		},
		{
			"both vertical edges and a height",
			`{"type":"anchored","children":[{"top":4,"bottom":4,"height":20,
			  "node":{"type":"rectangle"}}]}`,
			"cannot all hold at once",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Decoder{}).Decode(strings.NewReader(`{"version":1,"root":` + test.root + `}`))
			if err == nil {
				t.Fatal("decoded without complaint")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

// The point is to refuse only what would have been ignored. Each of these
// states one of the three and has to keep working.
func TestSchemaStillAcceptsEachOfThoseAlone(t *testing.T) {
	documents := []string{
		`{"type":"absolute","children":[{"bounds":{"x":0,"y":0,"width":40,"height":20},
		  "node":{"type":"rectangle","fill":"black"}}]}`,
		`{"type":"anchored","children":[{"left":0,"top":0,"width":40,"height":20,
		  "node":{"type":"rectangle","fill":"black"}}]}`,
		`{"type":"row","children":[{"basis":20,"ratio":2,"node":{"type":"rectangle"}}]}`,
		`{"type":"row","children":[{"basis":20,"cross":10,"node":{"type":"rectangle"}}]}`,
		`{"type":"anchored","children":[{"left":4,"right":4,"node":{"type":"rectangle"}}]}`,
		`{"type":"anchored","children":[{"left":4,"width":20,"node":{"type":"rectangle"}}]}`,
		`{"type":"row","children":[{"node":{"type":"rectangle","size":{"width":10,"height":10}}}]}`,
	}
	for _, document := range documents {
		if _, err := (Decoder{}).Decode(strings.NewReader(`{"version":1,"root":` + document + `}`)); err != nil {
			t.Errorf("refused a document it should accept: %v\n%s", err, document)
		}
	}
}
