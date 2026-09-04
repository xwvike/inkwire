package markup

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// drawing compiles a page whose whole content is one drawing, and hands back
// what the drawing compiled to.
func drawing(t *testing.T, content string) (string, Document) {
	t.Helper()
	page, err := Compile(
		`<div class="page"><svg width="60" height="40">`+content+`</svg></div>`,
		`.page { display: flex; width: 60px; height: 40px; background: white; }
		 svg { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(page.JSON)), ""), page
}

// The six elements whose meaning the schema already has, each with the fields
// SVG states them in. Nothing here is a translation anybody had to invent: a
// rect is a rectangle and a polyline is a polyline.
func TestTheShapesSVGAndTheSchemaBothHave(t *testing.T) {
	tests := []struct{ name, content, want string }{
		{"rect", `<rect x="4" y="6" width="20" height="10" fill="black"/>`,
			`"bounds":{"x":4,"y":6,"width":20,"height":10},"node":{"type":"rectangle","fill":"black"`},
		{"a rounded rect", `<rect width="20" height="10" rx="3" fill="red"/>`,
			`"type":"rectangle","radius":3,"fill":"red"`},
		{"circle", `<circle cx="30" cy="20" r="8" fill="black"/>`,
			`"type":"circle","radius":8,"fill":"black","center":{"x":30,"y":20}`},
		// The box is a pixel wider and taller than twice the radius: the
		// drawing model measures a radius as half of one less than the span,
		// so an ellipse touches the last pixel inside its box rather than the
		// edge of it. Twice the radius draws one a pixel small on each axis,
		// which is what this used to do and what only showed when the same
		// page was drawn both ways and the two were compared.
		{"ellipse", `<ellipse cx="30" cy="20" rx="10" ry="5" fill="red"/>`,
			`"bounds":{"x":20,"y":15,"width":21,"height":11},"node":{"type":"ellipse","fill":"red"}`},
		{"line", `<line x1="0" y1="0" x2="60" y2="40" stroke="black"/>`,
			`"type":"line","stroke":{"ink":"black","width":1,"cap":"butt","join":"miter"},"from":{"x":0,"y":0},"to":{"x":60,"y":40}`},
		{"polyline", `<polyline points="0,0 10,20 20,0" fill="none" stroke="red"/>`,
			`"type":"polyline","stroke":{"ink":"red","width":1,"cap":"butt","join":"miter"},"points":[{"x":0,"y":0},{"x":10,"y":20},{"x":20,"y":0}]`},
		{"polygon", `<polygon points="0,0 20,0 10,20" fill="black"/>`,
			`"type":"polygon","fill":"black","points":[`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flat, page := drawing(t, test.content)
			for _, warning := range page.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			if !strings.Contains(flat, strings.ReplaceAll(test.want, " ", "")) {
				t.Errorf("wanted %s in\n%s", test.want, page.JSON)
			}
		})
	}
}

// A viewport clips. A shape hanging off the edge of one is cut by it rather
// than drawn over whatever the page put beside it.
func TestADrawingIsAnAbsoluteBoxThatClips(t *testing.T) {
	flat, _ := drawing(t, `<rect width="10" height="10" fill="black"/>`)
	if !strings.Contains(flat, `"type":"absolute","clip":true`) {
		t.Errorf("the drawing is not a clipping box:\n%s", flat)
	}
}

// The two ways of writing a point are both what SVG accepts, because both are
// what a tool exports.
func TestPointsAreReadHoweverTheyAreSeparated(t *testing.T) {
	for _, written := range []string{"0,0 10,20 20,0", "0 0 10 20 20 0", "0,0\n10,20\n20,0"} {
		flat, page := drawing(t, `<polyline points="`+written+`" stroke="black"/>`)
		for _, warning := range page.Warnings {
			t.Errorf("%q: warning: %s", written, warning.Message)
		}
		if !strings.Contains(flat, `{"x":10,"y":20}`) {
			t.Errorf("%q did not read as three points:\n%s", written, flat)
		}
	}
}

// SVG's own defaults, not the panel's: a shape says nothing and is filled
// black, and a stroke has to be asked for. An author who drew this anywhere
// else expects the shape they drew.
func TestSVGDefaultsAreSVGs(t *testing.T) {
	flat, page := drawing(t, `<rect width="10" height="10"/>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(flat, `"fill":"black"`) {
		t.Errorf("a rect that said nothing was not filled black:\n%s", flat)
	}
	if strings.Contains(flat, `"stroke"`) {
		t.Errorf("a rect that asked for no stroke got one:\n%s", flat)
	}
}

// Nothing in a drawing stops the page being drawn either, and what was not
// drawn is named. These are the ones SVG makes easy to write by accident.
func TestADrawingSaysWhatItCouldNotDraw(t *testing.T) {
	tests := map[string]struct{ content, want string }{
		"a shape with no paint at all":     {`<rect width="10" height="10" fill="none"/>`, "neither a fill nor a stroke"},
		"a line with no stroke":            {`<line x1="0" y1="0" x2="9" y2="9"/>`, "no stroke"},
		"a rect with no size":              {`<rect fill="black"/>`, "no width or height"},
		"a circle with no radius":          {`<circle cx="5" cy="5" fill="black"/>`, "no radius"},
		"an ink the panel has not":         {`<rect width="9" height="9" fill="blue"/>`, "blue"},
		"an element this build has not":    {`<foreignObject width="9" height="9"/>`, "foreignObject"},
		"a stroke thinner than a pixel":    {`<line x1="0" y1="0" x2="9" y2="9" stroke="black" stroke-width="0.2"/>`, "less than a pixel"},
		"points that do not pair up":       {`<polyline points="0,0 10" stroke="black"/>`, "not a run of pairs"},
		"a polygon without three corners":  {`<polygon points="0,0 10,10" fill="black"/>`, "at least three points"},
		"a transform this build cannot do": {`<g transform="skewX(10)"><rect width="9" height="9"/></g>`, "skewX"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, page := drawing(t, test.content)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if !strings.Contains(said, test.want) {
				t.Errorf("nothing said %q; it said %q", test.want, said)
			}
		})
	}
}

// A group carries its children without placing them, which is what makes an
// exported drawing's nesting harmless.
func TestAGroupIsWalkedThrough(t *testing.T) {
	flat, page := drawing(t, `<g><g><rect width="10" height="10" fill="black"/></g></g>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(flat, `"type":"rectangle"`) {
		t.Errorf("the rect inside two groups was not drawn:\n%s", flat)
	}
}

// The elements a drawing carries that are not part of it.
func TestTitleAndDefsAreNotDrawn(t *testing.T) {
	flat, page := drawing(t,
		`<title>a drawing</title><desc>about something</desc><rect width="9" height="9" fill="black"/>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if strings.Contains(flat, "drawing") || strings.Contains(flat, "something") {
		t.Errorf("a title or a description was drawn as words:\n%s", flat)
	}
}

// Both spellings of a dash, since this is where a drawing tool writes them.
func TestADashInADrawing(t *testing.T) {
	flat, page := drawing(t,
		`<line x1="0" y1="0" x2="60" y2="0" stroke="black" stroke-dasharray="6, 3" stroke-dashoffset="2"/>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(flat, `"dash":[6,3],"dashOffset":2`) {
		t.Errorf("the dash was not carried:\n%s", flat)
	}
}

// pathOf reads one d attribute the way a drawing does.
func pathOf(t *testing.T, data string) ([]command, []Warning) {
	t.Helper()
	c := &compiler{}
	return c.parsePathData(data, rootFrame(), "d"), c.warnings
}

// The letters, each in both cases, and what each one leaves out.
func TestPathDataIsReadCommandByCommand(t *testing.T) {
	tests := []struct {
		name, d string
		want    []string
	}{
		{"moveto and lineto", "M 1 2 L 3 4",
			[]string{`{"op":"move","to":{"x":1,"y":2}}`, `{"op":"line","to":{"x":3,"y":4}}`}},
		{"a moveto's second pair is a lineto", "M 1 2 3 4 5 6",
			[]string{`{"op":"move","to":{"x":1,"y":2}}`, `{"op":"line","to":{"x":3,"y":4}}`, `{"op":"line","to":{"x":5,"y":6}}`}},
		{"relative lineto", "M 10 10 l 5 5",
			[]string{`{"op":"line","to":{"x":15,"y":15}}`}},
		{"horizontal and vertical", "M 10 10 H 30 V 40 h -5 v -5",
			[]string{`{"op":"line","to":{"x":30,"y":10}}`, `{"op":"line","to":{"x":30,"y":40}}`,
				`{"op":"line","to":{"x":25,"y":40}}`, `{"op":"line","to":{"x":25,"y":35}}`}},
		{"cubic", "M 0 0 C 1 2 3 4 5 6",
			[]string{`{"op":"cubic","to":{"x":5,"y":6},"control1":{"x":1,"y":2},"control2":{"x":3,"y":4}}`}},
		{"quadratic", "M 0 0 Q 1 2 3 4",
			[]string{`{"op":"quadratic","to":{"x":3,"y":4},"control":{"x":1,"y":2}}`}},
		{"close", "M 0 0 L 5 5 Z",
			[]string{`{"op":"close"}`}},
		{"a command repeats without being written again", "M 0 0 L 1 1 2 2 3 3",
			[]string{`{"op":"line","to":{"x":1,"y":1}}`, `{"op":"line","to":{"x":2,"y":2}}`, `{"op":"line","to":{"x":3,"y":3}}`}},
		{"numbers that abut", "M0,0L10-20 5.5.5",
			[]string{`{"op":"line","to":{"x":10,"y":-20}}`, `{"op":"line","to":{"x":6,"y":1}}`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands, warnings := pathOf(t, test.d)
			for _, warning := range warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			written, err := json.Marshal(commands)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range test.want {
				if !strings.Contains(string(written), want) {
					t.Errorf("wanted %s in %s", want, written)
				}
			}
		})
	}
}

// A smooth curve leaves out the control point that mirrors the last one, and
// after anything but a curve of its own kind there is nothing to mirror — the
// specification says to use the pen's own position, and getting that wrong
// puts a kink in every path a drawing tool exports.
func TestASmoothCurveMirrorsTheOneBeforeIt(t *testing.T) {
	commands, _ := pathOf(t, "M 0 0 C 10 0 10 20 20 20 S 30 40 40 40")
	written, _ := json.Marshal(commands)
	// The second cubic's first control point is the reflection of 10,20
	// about 20,20, which is 30,20.
	if !strings.Contains(string(written), `"control1":{"x":30,"y":20}`) {
		t.Errorf("the smooth cubic did not mirror its predecessor: %s", written)
	}

	// A smooth curve with nothing to mirror starts from the pen.
	alone, _ := pathOf(t, "M 5 5 S 30 40 40 40")
	writtenAlone, _ := json.Marshal(alone)
	if !strings.Contains(string(writtenAlone), `"control1":{"x":5,"y":5}`) {
		t.Errorf("a smooth cubic with nothing before it did not start from the pen: %s", writtenAlone)
	}
}

// An arc is the one command whose two spellings are genuinely different: SVG
// says where it ends, the schema says the box it is inscribed in. Each of
// these is a case whose answer can be worked out by hand.
func TestAnArcIsConvertedToTheBoxItIsInscribedIn(t *testing.T) {
	tests := []struct {
		name, d             string
		x, y, width, height int
		start, sweep        float64
	}{
		{"a semicircle clockwise", "M 0 10 A 10 10 0 0 1 20 10", 0, 0, 21, 21, 180, 180},
		{"the same one the other way", "M 0 10 A 10 10 0 0 0 20 10", 0, 0, 21, 21, 180, -180},
		{"an ellipse's half", "M 10 0 A 10 5 0 0 1 10 10", 0, 0, 21, 11, -90, 180},
		{"a radius too small is grown until it reaches", "M 0 0 A 1 1 0 0 1 10 0", 0, -5, 11, 11, 180, 180},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands, warnings := pathOf(t, test.d)
			for _, warning := range warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			if len(commands) != 2 || commands[1].Op != "arc" {
				t.Fatalf("did not read as a move and an arc: %+v", commands)
			}
			arc := commands[1]
			want := rect{X: test.x, Y: test.y, Width: test.width, Height: test.height}
			if *arc.Bounds != want {
				t.Errorf("bounds = %+v, want %+v", *arc.Bounds, want)
			}
			if arc.Start != test.start || arc.Sweep != test.sweep {
				t.Errorf("start = %g sweep = %g, want %g and %g", arc.Start, arc.Sweep, test.start, test.sweep)
			}
		})
	}
}

// An arc with a radius of zero is a straight line, which the specification
// says outright and a drawing tool writes when a curve was flattened.
func TestAnArcWithNoRadiusIsALine(t *testing.T) {
	commands, warnings := pathOf(t, "M 0 0 A 0 0 0 0 1 10 10")
	for _, warning := range warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if len(commands) != 2 || commands[1].Op != "line" {
		t.Fatalf("did not read as a move and a line: %+v", commands)
	}
}

// A path with something unreadable in it draws as far as it got, which on a
// panel is a partial picture rather than none, and says where it stopped.
func TestAPathDrawsAsFarAsItGot(t *testing.T) {
	commands, warnings := pathOf(t, "M 0 0 L 10 10 X 20 20")
	if len(commands) != 2 {
		t.Errorf("the readable part was not kept: %+v", commands)
	}
	var said string
	for _, warning := range warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "X") {
		t.Errorf("where it stopped was not named: %q", said)
	}
}

// An arc's third number is the tilt of its own ellipse, and two arcs in one
// path may state it differently — so it belongs to the arc rather than to any
// transform around it. SVG carries it for that reason and the schema carries
// it for the same one; it used to be thrown away with a warning, which was a
// standard parameter not working.
func TestAnArcKeepsItsOwnTilt(t *testing.T) {
	commands, warnings := pathOf(t, "M 0 10 A 10 5 45 0 1 20 10")
	for _, warning := range warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if len(commands) != 2 || commands[1].Op != "arc" {
		t.Fatalf("did not read as a move and an arc: %+v", commands)
	}
	if commands[1].Rotation != 45 {
		t.Errorf("the arc's tilt came back as %g, want 45", commands[1].Rotation)
	}

	// Two arcs in one path, tilted differently, which is the case a transform
	// around them cannot produce.
	pair, _ := pathOf(t, "M 0 0 A 10 5 45 0 1 20 0 A 10 5 -30 0 1 40 0")
	if len(pair) != 3 {
		t.Fatalf("did not read as a move and two arcs: %+v", pair)
	}
	if pair[1].Rotation != 45 || pair[2].Rotation != -30 {
		t.Errorf("the two tilts came back as %g and %g, want 45 and -30", pair[1].Rotation, pair[2].Rotation)
	}
}

// A rotate in a transform attribute becomes a turn of its own rather than
// being folded into the coordinates, because a rect folded through a turn
// stops being a rect and an ellipse stops being an ellipse.
func TestAGroupsRotateBecomesATurn(t *testing.T) {
	flat, page := drawing(t, `<g transform="rotate(37)"><rect width="10" height="10" fill="black"/></g>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(flat, `"type":"rotated","degrees":37`) {
		t.Errorf("the group's rotate did not become a turn:\n%s", page.JSON)
	}
	// The shape inside is still the shape it was written as.
	if !strings.Contains(flat, `"type":"rectangle"`) {
		t.Errorf("the rect stopped being a rect:\n%s", page.JSON)
	}

	// SVG turns about the drawing's origin unless a point is named, which is
	// not what a node defaults to, so it has to be said.
	if !strings.Contains(flat, `"origin":{"x":0,"y":0}`) {
		t.Errorf("a rotate with no point named did not turn about the origin:\n%s", page.JSON)
	}
	about, _ := drawing(t, `<g transform="rotate(37, 20, 30)"><rect width="10" height="10" fill="black"/></g>`)
	if !strings.Contains(about, `"origin":{"x":20,"y":30}`) {
		t.Errorf("the named point was not used:\n%s", about)
	}
}

// A move after a turn would run along turned axes, which the coordinates this
// folds a move into cannot carry. It is reported rather than drawn wrong.
func TestAMoveAfterATurnIsReported(t *testing.T) {
	_, page := drawing(t, `<g transform="rotate(30) translate(10,10)"><rect width="9" height="9"/></g>`)
	var said string
	for _, warning := range page.Warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "turned axes") {
		t.Errorf("nothing said it: %q", said)
	}
}

// A group's paint is its children's, which is what the element is for and how
// every drawing tool writes one: fill and stroke go on the g and the shapes
// inside say only where they are.
func TestAGroupHandsItsPaintDown(t *testing.T) {
	flat, page := drawing(t,
		`<g fill="none" stroke="black" stroke-width="2"><circle cx="20" cy="20" r="8"/></g>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	// The circle itself, not the page around it: the background is a filled
	// rectangle and matching on "fill" anywhere would find that instead.
	if !strings.Contains(flat, `"type":"circle","radius":8,"stroke":{"ink":"black","width":2,"cap":"butt","join":"miter"}`) {
		t.Errorf("the group's paint did not reach the circle as stated:\n%s", flat)
	}
}

// A shape says the part it disagrees with and inherits the rest.
func TestAShapeOverridesOnlyWhatItStates(t *testing.T) {
	flat, _ := drawing(t,
		`<g fill="black" stroke="black" stroke-width="3"><rect width="9" height="9" fill="red"/></g>`)
	if !strings.Contains(flat, `"fill":"red"`) {
		t.Errorf("the shape's own fill did not win:\n%s", flat)
	}
	if !strings.Contains(flat, `"width":3`) || !strings.Contains(flat, `"cap":"butt"`) {
		t.Errorf("the group's stroke width was not inherited:\n%s", flat)
	}
}

// An attribute this build does not act on is named, the same way a CSS
// property it does not implement is. A drawing that quietly lost its blur is a
// drawing whose author has no way of finding out.
func TestADrawingNamesTheAttributesItDidNotAct0n(t *testing.T) {
	tests := map[string]string{
		"opacity":    `<rect width="9" height="9" opacity="0.5"/>`,
		"filter":     `<rect width="9" height="9" filter="url(#blur)"/>`,
		"mask":       `<rect width="9" height="9" mask="url(#m)"/>`,
		"on a group": `<g opacity="0.5"><rect width="9" height="9"/></g>`,
	}
	for want, content := range tests {
		t.Run(want, func(t *testing.T) {
			_, page := drawing(t, content)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if !strings.Contains(said, strings.TrimSuffix(want, " on a group")) &&
				!strings.Contains(said, "opacity") {
				t.Errorf("nothing named it: %q", said)
			}
		})
	}
}

// A drawing somebody else made names its colours in terms of their palette,
// not the panel's. Restating them in a stylesheet is what makes it usable, and
// it works because a presentation attribute is a rule of no specificity: any
// selector at all beats it, which is what CSS says and what a browser does.
func TestAStylesheetOutranksAPresentationAttribute(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="40" height="40"><rect width="20" height="20" fill="#d97757"/></svg></div>`,
		`.page { display: flex; width: 40px; height: 40px; background: white; }
		 svg rect { fill: black; }`)
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	if !strings.Contains(flat, `"type":"rectangle","fill":"black"`) {
		t.Errorf("the stylesheet did not win over the attribute:\n%s", page.JSON)
	}
}

// A custom property in an attribute is what a design system exports, and the
// colour it falls back to is the one worth reading.
func TestACustomPropertyInAnAttributeIsResolved(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="40" height="40"><rect width="20" height="20" fill="var(--brand, black)"/></svg></div>`,
		`.page { display: flex; width: 40px; height: 40px; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(page.JSON)), ""), `"fill":"black"`) {
		t.Errorf("the fallback was not used:\n%s", page.JSON)
	}

	// And the declared value wins over the fallback, as it does in CSS.
	declared, err := Compile(
		`<div class="page"><svg width="40" height="40"><rect width="20" height="20" fill="var(--brand, black)"/></svg></div>`,
		`:root { --brand: red; } .page { display: flex; width: 40px; height: 40px; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(declared.JSON)), ""), `"fill":"red"`) {
		t.Errorf("the declared value did not win over the fallback:\n%s", declared.JSON)
	}
}

// A viewBox says what a drawing's coordinates mean and the box says how large
// it is on the page. Every finite scale is valid, including a shrink, and the
// browser's default preserveAspectRatio keeps it centred without distortion.
func TestAViewBoxMapsIntoItsViewport(t *testing.T) {
	compile := func(t *testing.T, css string) Document {
		t.Helper()
		page, err := Compile(
			`<div class="page"><svg viewBox="0 0 50 50"><rect width="10" height="10" fill="black"/></svg></div>`,
			`.page { display: flex; width: 200px; height: 200px; } `+css)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	same := compile(t, `svg { display: block; width: 50px; height: 50px; }`)
	for _, warning := range same.Warnings {
		t.Errorf("a drawing at its own size was reported: %s", warning.Message)
	}
	if strings.Contains(string(same.JSON), "transformed") {
		t.Errorf("a drawing at its own size was magnified:\n%s", same.JSON)
	}

	doubled := compile(t, `svg { display: block; width: 100px; height: 100px; }`)
	for _, warning := range doubled.Warnings {
		t.Errorf("an exact magnification was reported: %s", warning.Message)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(doubled.JSON)), ""), `"bounds":{"x":0,"y":0,"width":20,"height":20}`) {
		t.Errorf("twice the size was not drawn twice the size:\n%s", doubled.JSON)
	}

	awkward := compile(t, `svg { display: block; width: 84px; height: 84px; }`)
	for _, warning := range awkward.Warnings {
		t.Errorf("a non-integer scale was reported: %s", warning.Message)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(awkward.JSON)), ""), `"bounds":{"x":0,"y":0,"width":17,"height":17}`) {
		t.Errorf("a non-integer scale was not applied:\n%s", awkward.JSON)
	}

	shrunken := compile(t, `svg { display: block; width: 25px; height: 25px; }`)
	for _, warning := range shrunken.Warnings {
		t.Errorf("a shrink was reported: %s", warning.Message)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(shrunken.JSON)), ""), `"bounds":{"x":0,"y":0,"width":5,"height":5}`) {
		t.Errorf("a viewBox was not shrunk into its viewport:\n%s", shrunken.JSON)
	}
}

func TestAViewBoxShiftsItsOriginAndMeetsInTheViewport(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg viewBox="-10 -20 50 50"><rect x="-10" y="-20" width="10" height="10" fill="black"/></svg></div>`,
		`.page { display: flex; width: 100px; height: 80px; } svg { display: block; width: 100px; height: 80px; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	// meet chooses 1.6 for a 50x50 viewBox in a 100x80 viewport, leaving a
	// 10px horizontal letterbox. The shifted origin therefore starts at 10,0.
	if !strings.Contains(flat, `"bounds":{"x":10,"y":0,"width":16,"height":16}`) {
		t.Errorf("the viewBox was not shifted and centred:\n%s", page.JSON)
	}
}

// A transform is folded into the numbers as they are read rather than becoming
// a node, which is exact: a translate is an offset and a scale is a multiplier,
// and neither leaves anything to round twice.
func TestATransformIsFoldedIntoTheCoordinates(t *testing.T) {
	tests := []struct{ name, content, want string }{
		{"translate", `<g transform="translate(10,20)"><rect x="1" y="2" width="4" height="4"/></g>`,
			`"bounds":{"x":11,"y":22,"width":4,"height":4}`},
		{"one that names only x", `<g transform="translate(10)"><rect x="1" y="2" width="4" height="4"/></g>`,
			`"bounds":{"x":11,"y":2,"width":4,"height":4}`},
		{"nested translates add up", `<g transform="translate(10,0)"><g transform="translate(0,5)"><rect x="1" y="1" width="4" height="4"/></g></g>`,
			`"bounds":{"x":11,"y":6,"width":4,"height":4}`},
		{"a scale multiplies both the place and the size", `<g transform="translate(4,4) scale(2)"><rect x="1" y="1" width="5" height="5"/></g>`,
			`"bounds":{"x":6,"y":6,"width":10,"height":10}`},
		{"a scale reaches a circle's radius", `<g transform="scale(3)"><circle cx="2" cy="2" r="3" fill="black"/></g>`,
			`"type":"circle","radius":9,"fill":"black","center":{"x":6,"y":6}`},
		{"and a path's points", `<g transform="translate(10,10)"><path d="M0 0 L4 4" stroke="black"/></g>`,
			`{"op":"move","to":{"x":10,"y":10}},{"op":"line","to":{"x":14,"y":14}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flat, page := drawing(t, test.content)
			for _, warning := range page.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			if !strings.Contains(flat, test.want) {
				t.Errorf("wanted %s in\n%s", test.want, page.JSON)
			}
		})
	}
}

func TestASignedAndNonUniformScaleIsFoldedIntoTheCoordinates(t *testing.T) {
	flat, page := drawing(t,
		`<g transform="translate(0,40) scale(.5,-.5)"><rect x="0" y="0" width="20" height="10"/></g>`)
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(flat, `"bounds":{"x":0,"y":35,"width":10,"height":5}`) {
		t.Errorf("the signed scale was not folded into the rectangle:\n%s", page.JSON)
	}
}

// What this cannot do is drawn as written and said. A turn is no longer on the
// list — it became a node of its own — so what is left is shearing, an
// arbitrary matrix, and a zero or malformed scale.
func TestATransformThatCannotBeFoldedIsReported(t *testing.T) {
	for _, transform := range []string{"skewX(10)", "matrix(1,0,0,1,5,5)", "scale(0)", "scale(foo)"} {
		t.Run(transform, func(t *testing.T) {
			_, page := drawing(t, `<g transform="`+transform+`"><rect width="9" height="9"/></g>`)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message
			}
			if !strings.Contains(said, transform[:strings.IndexByte(transform, '(')]) {
				t.Errorf("nothing named it: %q", said)
			}
		})
	}
}

// A clipPath is the one thing in a drawing named where it is written and used
// somewhere else, and its shape is stated in the drawing's coordinates rather
// than the clipped element's — so the clip is given the whole drawing to
// resolve against and the element is placed inside it.
func TestAShapeIsClippedToTheClipPathItNames(t *testing.T) {
	tests := []struct{ name, defined, want string }{
		{"a rect", `<rect x="2" y="4" width="10" height="6"/>`,
			`"type":"clipRect","rect":{"x":2,"y":4,"width":10,"height":6}`},
		{"a circle", `<circle cx="20" cy="20" r="8"/>`,
			`"type":"clipShape","shape":{"kind":"circle","radius":8,"center":{"x":20,"y":20}}`},
		{"an ellipse", `<ellipse cx="20" cy="20" rx="10" ry="5"/>`,
			`"kind":"ellipse","radiusX":10,"radiusY":5`},
		{"a polygon", `<polygon points="0,0 20,0 10,20"/>`,
			`"kind":"polygon","points":[{"x":0,"y":0}`},
		{"a path", `<path d="M0 0 L10 0 L10 10 Z"/>`,
			`"type":"clipPath","path":{"commands":[{"op":"move"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flat, page := drawing(t,
				`<defs><clipPath id="c">`+test.defined+`</clipPath></defs>`+
					`<rect width="40" height="40" fill="black" clip-path="url(#c)"/>`)
			for _, warning := range page.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			if !strings.Contains(flat, test.want) {
				t.Errorf("wanted %s in\n%s", test.want, page.JSON)
			}
			// The clipped shape is inside the clip rather than beside it.
			if !strings.Contains(flat, `"child":{"type":"absolute","children":[{"bounds"`) {
				t.Errorf("the shape was not placed inside its clip:\n%s", page.JSON)
			}
		})
	}
}

// A clipPath that is not there, or holds something this cannot clip to, leaves
// the shape unclipped and says so. A shape that vanished because its clip was
// missing would be a hole nobody could account for.
func TestAClipThatCannotBeUsedLeavesTheShapeAlone(t *testing.T) {
	tests := map[string]string{
		"defines no clipPath by that name": `<rect width="9" height="9" clip-path="url(#gone)"/>`,
		"is not a reference":               `<rect width="9" height="9" clip-path="circle(50%)"/>`,
		"with no shape in it":              `<defs><clipPath id="c"></clipPath></defs><rect width="9" height="9" clip-path="url(#c)"/>`,
		"holding a":                        `<defs><clipPath id="c"><line x1="0" y1="0" x2="9" y2="9"/></clipPath></defs><rect width="9" height="9" clip-path="url(#c)"/>`,
	}
	for want, content := range tests {
		t.Run(want, func(t *testing.T) {
			flat, page := drawing(t, content)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if !strings.Contains(said, want) {
				t.Errorf("nothing said %q; it said %q", want, said)
			}
			if !strings.Contains(flat, `"type":"rectangle"`) {
				t.Errorf("the shape was lost along with its clip:\n%s", flat)
			}
		})
	}
}

// A pattern is the same picture in both formats, written differently: SVG
// holds shapes, the schema holds a grid of characters and what ink each means.
// Getting from one to the other is filling integer cells from integer
// rectangles, which is arithmetic rather than drawing.
func TestAPatternBecomesTheGridTheSchemaTiles(t *testing.T) {
	tests := []struct {
		name, defined string
		rows          []string
		inks          map[string]string
	}{
		{"a diagonal of single cells",
			`<rect x="0" y="0" width="1" height="1" fill="black"/><rect x="1" y="1" width="1" height="1" fill="black"/>`,
			[]string{"x.", ".x"}, map[string]string{"x": "black"}},
		{"blocks of two inks",
			`<rect x="0" y="0" width="1" height="1" fill="black"/><rect x="1" y="1" width="1" height="1" fill="red"/>`,
			[]string{"x.", ".r"}, map[string]string{"x": "black", "r": "red"}},
		{"a rect with no fill is black, as SVG says",
			`<rect x="0" y="0" width="1" height="1"/>`,
			[]string{"x.", ".."}, map[string]string{"x": "black"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, page := drawing(t,
				`<defs><pattern id="p" width="2" height="2" patternUnits="userSpaceOnUse">`+
					test.defined+`</pattern></defs>`+
					`<rect width="20" height="20" fill="url(#p)"/>`)
			for _, warning := range page.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			var found map[string]any
			var walk func(any)
			walk = func(node any) {
				switch value := node.(type) {
				case map[string]any:
					if value["type"] == "pattern" {
						found = value
					}
					for _, inner := range value {
						walk(inner)
					}
				case []any:
					for _, inner := range value {
						walk(inner)
					}
				}
			}
			var document any
			if err := json.Unmarshal(page.JSON, &document); err != nil {
				t.Fatal(err)
			}
			walk(document)
			if found == nil {
				t.Fatalf("no pattern node:\n%s", page.JSON)
			}
			var rows []string
			for _, row := range found["rows"].([]any) {
				rows = append(rows, row.(string))
			}
			if !slices.Equal(rows, test.rows) {
				t.Errorf("rows = %q, want %q", rows, test.rows)
			}
			for letter, ink := range test.inks {
				if found["inks"].(map[string]any)[letter] != ink {
					t.Errorf("inks = %v, want %s to be %s", found["inks"], letter, ink)
				}
			}
		})
	}
}

// What a pattern cannot be read as, said rather than drawn as something else.
func TestAPatternThisCannotReadIsReported(t *testing.T) {
	tests := map[string]string{
		"defines no pattern by that name": `<rect width="9" height="9" fill="url(#gone)"/>`,
		"states no tile size":             `<defs><pattern id="p"><rect width="1" height="1"/></pattern></defs><rect width="9" height="9" fill="url(#p)"/>`,
		"is a picture":                    `<defs><pattern id="p" width="200" height="200"><rect width="1" height="1"/></pattern></defs><rect width="9" height="9" fill="url(#p)"/>`,
		"holds a circle":                  `<defs><pattern id="p" width="4" height="4"><circle cx="2" cy="2" r="1"/></pattern></defs><rect width="9" height="9" fill="url(#p)"/>`,
		"measured in":                     `<defs><pattern id="p" width="4" height="4" patternUnits="objectBoundingBox"><rect width="1" height="1"/></pattern></defs><rect width="9" height="9" fill="url(#p)"/>`,
		"a clip-path shapes it":           `<defs><pattern id="p" width="2" height="2"><rect width="1" height="1"/></pattern></defs><circle cx="9" cy="9" r="5" fill="url(#p)"/>`,
	}
	for want, content := range tests {
		t.Run(want, func(t *testing.T) {
			_, page := drawing(t, content)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if !strings.Contains(said, want) {
				t.Errorf("nothing said %q; it said %q", want, said)
			}
		})
	}
}

// SVG writes stroke-width without a unit, and a stylesheet that paints a shape
// is usually one somebody moved out of the shape's own attributes. Refusing
// the bare number made every rule copied across a file stop working with a
// message about pixels, which is the shape of a front end that is technically
// right and useless.
func TestAUnitlessStrokeWidthIsPixels(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="60" height="40"><rect x="4" y="6" width="20" height="10"/></svg></div>`,
		`.page { display: flex; width: 60px; height: 40px; }
		 svg { display: block; flex-grow: 1; }
		 rect { fill: none; stroke: black; stroke-width: 3; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("stroke-width: 3 was reported: %s", warning.Message)
	}
	if written := strings.Join(strings.Fields(string(page.JSON)), ""); !strings.Contains(written, `"width":3`) {
		t.Errorf("the stroke is not three pixels:\n%s", page.JSON)
	}
}

func TestAViewBoxScalesStrokeLengths(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="240" height="240" viewBox="0 0 24 24">
			<line x1="2" y1="12" x2="22" y2="12" stroke="black" stroke-width="1" stroke-dasharray="2 1"/>
		</svg></div>`,
		`.page { display: flex; width: 240px; height: 240px; }
			svg { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	if !strings.Contains(flat, `"stroke":{"ink":"black","width":10,"dash":[20,10]`) {
		t.Errorf("viewBox stroke lengths were not scaled: %s", page.JSON)
	}
}

func TestARoundCappedSVGPointIsAStrokeDot(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="20" height="20" fill="none" stroke="black" stroke-linecap="round"><path d="M 10 10 h .01"/></svg></div>`,
		`.page { display: flex; width: 20px; height: 20px; }
			svg { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	if !strings.Contains(flat, `"type":"path"`) || !strings.Contains(flat, `"cap":"round"`) {
		t.Errorf("round-capped point did not remain a styled path: %s", page.JSON)
	}
}

func TestSVGStrokeCapAndJoinCascadeIntoTheScene(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="20" height="20"><style>line { stroke: black; stroke-linecap: round; stroke-linejoin: bevel; }</style><line x1="2" y1="10" x2="18" y2="10"/></svg></div>`,
		`.page { display: flex; width: 20px; height: 20px; } svg { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	if !strings.Contains(flat, `"cap":"round","join":"bevel"`) {
		t.Fatalf("styled SVG stroke did not reach the scene: %s", page.JSON)
	}
}
