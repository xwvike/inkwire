package markup

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

// warningsFor compiles and returns everything the compiler said, so a test can
// assert either that something worked or that it was refused by name.
func warningsFor(t *testing.T, body, css string) string {
	t.Helper()
	document, err := Compile(`<div class="page">`+body+`</div>`,
		`.page { display: flex; width: 100px; height: 50px; }`+css)
	var joined string
	for _, warning := range document.Warnings {
		joined += warning.Message + "\n"
	}
	if err != nil {
		joined += err.Error()
	}
	return joined
}

// Things an author would write without a second thought. Each either works or
// says why not; what none of them may do is nothing at all.
func TestCommonThingsAreEitherDoneOrRefused(t *testing.T) {
	tests := []struct {
		name string
		body string
		css  string
	}{
		{"an image", `<img src="photo.png">`, ``},
		{"a line break", `<span>one<br>two</span>`, ``},
		{"line-height", `<span>x</span>`, ` span { line-height: 20px; }`},
		{"white-space nowrap", `<span>x</span>`, ` span { white-space: nowrap; }`},
		{"min-width", `<span>x</span>`, ` span { min-width: 10px; }`},
		{"max-width", `<span>x</span>`, ` span { max-width: 10px; }`},
		{"overflow hidden", `<span>x</span>`, ` span { overflow: hidden; }`},
		{"rgb colour", `<span>x</span>`, ` span { color: rgb(255, 0, 0); }`},
		{"font shorthand", `<span>x</span>`, ` span { font: 12px monaco; }`},
		{"letter-spacing", `<span>x</span>`, ` span { letter-spacing: 1px; }`},
		{"text-transform", `<span>x</span>`, ` span { text-transform: uppercase; }`},
		{"per-side border", `<span>x</span>`, ` span { border-bottom: 1px solid black; }`},
		{"per-corner radius", `<span>x</span>`, ` span { border-top-left-radius: 2px; }`},
		{"a background image", `<span>x</span>`, ` span { background-image: url(photo.png); }`},
		{"opacity", `<span>x</span>`, ` span { opacity: 0.5; }`},
		{"a table", `<table><tr><td>x</td></tr></table>`, ``},
		{"a list", `<ul><li>x</li></ul>`, ``},
		{"a heading", `<h1>x</h1>`, ``},
		{"a paragraph", `<p>x</p>`, ``},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			said := warningsFor(t, test.body, test.css)
			if said == "" {
				t.Logf("SILENT — accepted with no effect and no warning")
				return
			}
			t.Logf("said: %s", strings.TrimSpace(said))
		})
	}
}

// The capabilities added after the probe above, each checked for doing the
// thing rather than merely not complaining about it.

func TestTextWrapsByDefault(t *testing.T) {
	// Twelve pixel Monaco advances seven, so this cannot be one line in 100px.
	const sentence = "wrapping happens when a line is longer than its box"
	got := boxes(t, `<span class="a">`+sentence+`</span>`,
		`.a { display: block; flex-grow: 1; font-family: monaco; font-size: 12px; }`)
	if height := got[display.InkBlack].Dy(); height < 30 {
		t.Errorf("the text occupied %d pixels of height; it cannot have wrapped", height)
	}
}

func TestNowrapKeepsOneLine(t *testing.T) {
	const sentence = "wrapping happens when a line is longer than its box"
	got := boxes(t, `<span class="a">`+sentence+`</span>`,
		`.a { display: block; flex-grow: 1; font-family: monaco; font-size: 12px; white-space: nowrap; }`)
	if height := got[display.InkBlack].Dy(); height > 16 {
		t.Errorf("nowrap text occupied %d pixels of height; it wrapped anyway", height)
	}
}

func TestBreakStartsANewLine(t *testing.T) {
	got := boxes(t, `<span class="a">one<br>two</span>`,
		`.a { display: block; flex-grow: 1; font-family: monaco; font-size: 12px; white-space: nowrap; }`)
	if height := got[display.InkBlack].Dy(); height < 20 {
		t.Errorf("a break produced %d pixels of height; both words are on one line", height)
	}
}

func TestOverflowHiddenClipsToTheBox(t *testing.T) {
	got := boxes(t, `<i class="outer"><i class="b"></i></i>`,
		inks+` .outer { display: block; flex-basis: 30px; overflow: hidden; }
		.b { width: 200px; height: 20px; }`)
	if right := got[display.InkRed].Max.X; right > 30 {
		t.Errorf("the child reached x=%d, past its parent's 30 pixel box", right)
	}
}

func TestLineHeightSetsTheLine(t *testing.T) {
	got := boxes(t, `<span class="a">one<br>two</span>`,
		`.a { display: block; flex-grow: 1; font-family: monaco; font-size: 12px; line-height: 20px; }`)
	if height := got[display.InkBlack].Dy(); height < 26 {
		t.Errorf("two lines at a 20 pixel line height covered only %d pixels", height)
	}
}

// An img reaches the same image node a scene document uses, so the measuring
// and dithering that node does apply to a page as well.
//
// It reaches it by naming the file, not by carrying a decoded picture. This
// package compiles a page to a document and the document is read by the
// decoder, which is what opens the file — so there is one loader, with one
// pixel limit and one list of formats it accepts, and no second one behind
// the stylesheet holding a page to less.
func TestImageIsDrawnThroughTheImageNode(t *testing.T) {
	directory := probeDirectory(t)
	if err := os.Rename(filepath.Join(directory, "p.png"), filepath.Join(directory, "photo.png")); err != nil {
		t.Fatal(err)
	}
	document, err := Compile(
		`<div class="page"><img src="photo.png" class="p"></div>`,
		`.page { display: flex; width: 40px; height: 40px; } .p { flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	frame, _ := renderDocument(t, directory+string(filepath.Separator), document.JSON)
	dark := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				dark++
			}
		}
	}
	if dark == 0 {
		t.Fatal("the image drew nothing")
	}
	if dark == 1600 {
		t.Fatal("the image drew a solid block; the checkerboard did not survive")
	}
}

// Without a resolver an img is reported, never silently missing.
// A picture that is not there is reported by the decoder now rather than by
// this package, because this package no longer looks. That is the change
// working rather than a check being lost: the message names the file, and it
// is the same message a scene document naming the same missing file gets.
func TestAnImageThatIsNotThereIsReportedByName(t *testing.T) {
	page, err := Compile(`<div class="page"><img src="photo.png" class="a"></div>`,
		`.page { display: flex; width: 40px; height: 40px; } .a { flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (scene.Decoder{BaseDir: t.TempDir()}).Decode(bytes.NewReader(page.JSON))
	if err == nil {
		t.Fatal("a page naming a picture that is not there compiled and decoded")
	}
	if !strings.Contains(err.Error(), "photo.png") {
		t.Errorf("the missing picture was not named: %v", err)
	}
}

func TestObjectFitReachesTheImageNode(t *testing.T) {
	directory := probeDirectory(t)
	for css, want := range map[string]string{"fill": "stretch", "contain": "contain", "cover": "cover"} {
		t.Run(css, func(t *testing.T) {
			document, err := Compile(
				`<div class="page"><img src="p.png" class="p"></div>`,
				`.page { display: flex; width: 40px; height: 40px; } .p { flex-grow: 1; object-fit: `+css+`; }`)
			if err != nil {
				t.Fatal(err)
			}
			for _, warning := range document.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
			// The fit is checked in the document rather than in the picture,
			// because a fit is a word the schema has and the two spellings
			// are not the same: CSS says fill where a scene says stretch.
			if !strings.Contains(string(document.JSON), `"fit": "`+want+`"`) {
				t.Errorf("object-fit: %s did not become %q:\n%s", css, want, document.JSON)
			}
			renderDocument(t, directory+string(filepath.Separator), document.JSON)
		})
	}
}

func TestVisibilityHiddenKeepsTheSpace(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-basis: 20px; visibility: hidden; } .b { flex-grow: 1; }`)
	if _, drawn := got[display.InkBlack]; drawn {
		t.Errorf("a hidden box was painted at %v", got[display.InkBlack])
	}
	// Unlike display:none, the space it occupied is still occupied.
	expect(t, got, display.InkRed, image.Rect(20, 0, 100, 50), "b after the hidden box's space")
}

func TestDashedBorder(t *testing.T) {
	solid := boxes(t, `<i class="a"></i>`,
		`.a { display: block; flex-grow: 1; border: 1px solid black; }`)
	dashed := boxes(t, `<i class="a"></i>`,
		`.a { display: block; flex-grow: 1; border: 1px solid black; border-style: dashed; }`)
	if solid[display.InkBlack] != dashed[display.InkBlack] {
		t.Fatalf("the dashed border covers %v, the solid one %v; both should trace the box",
			dashed[display.InkBlack], solid[display.InkBlack])
	}
	// The dashes have to actually leave gaps, or nothing was dashed.
	solidInk := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; }`)
	dashedInk := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; border-style: dashed; }`)
	if dashedInk >= solidInk {
		t.Errorf("the dashed border used %d pixels and the solid one %d", dashedInk, solidInk)
	}
}

func countInk(t *testing.T, css string) int {
	t.Helper()
	frame, _ := renderProbe(t, `<i class="a"></i>`, css)
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				count++
			}
		}
	}
	return count
}

func TestBoxSizingBorderBoxIsAccepted(t *testing.T) {
	if said := warningsFor(t, `<i class="a"></i>`,
		` .a { display: block; flex-grow: 1; background: black; box-sizing: border-box; }`); said != "" {
		t.Errorf("border-box was reported even though it is what happens: %s", said)
	}
	// content-box is not what happens, so it says so.
	if said := warningsFor(t, `<i class="a"></i>`,
		` .a { display: block; flex-grow: 1; background: black; box-sizing: content-box; }`); said == "" {
		t.Error("content-box was accepted silently")
	}
}

func TestAbsolutePositioning(t *testing.T) {
	got := boxes(t, `<i class="a"></i><i class="b"></i>`,
		inks+` .a { flex-grow: 1; } .b { position: absolute; top: 10px; left: 20px; width: 15px; height: 15px; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 100, 50), "the flow child still fills the line")
	expect(t, got, display.InkRed, image.Rect(20, 10, 35, 25), "the placed child at its inset")
}

func TestCSSWideKeywords(t *testing.T) {
	// inherit keeps the parent's colour rather than being an unknown value.
	got := boxes(t, `<i class="a"><i class="b"></i></i>`,
		`.a { display: block; flex-grow: 1; color: red; }
		 .b { display: block; background: black; height: 10px; }
		 .b { color: inherit; }`)
	if _, drawn := got[display.InkBlack]; !drawn {
		t.Error("inherit was treated as an error and the box vanished")
	}
	if said := warningsFor(t, `<i class="a">x</i>`, ` .a { color: inherit; }`); said != "" {
		t.Errorf("inherit was reported: %s", said)
	}
	if said := warningsFor(t, `<i class="a">x</i>`, ` .a { color: initial; }`); said != "" {
		t.Errorf("initial was reported: %s", said)
	}
}

func TestMinAndMaxOnTheContainersAxis(t *testing.T) {
	wide := boxes(t, twoBoxes, inks+` .a { flex-basis: 10px; min-width: 40px; } .b { flex-grow: 1; }`)
	expect(t, wide, display.InkBlack, image.Rect(0, 0, 40, 50), "a raised to its minimum")

	narrow := boxes(t, twoBoxes, inks+` .a { flex-basis: 80px; max-width: 30px; } .b { flex-grow: 1; }`)
	expect(t, narrow, display.InkBlack, image.Rect(0, 0, 30, 50), "a held to its maximum")
}

// A minimum across the container's axis used to be unresolvable, because
// nothing knew that size before the layout ran. It is resolved there now.
func TestMinAcrossTheAxisApplies(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		` .a { display: block; background: black; flex-grow: 1; height: 5px; min-height: 20px; }`)
	if height := got[display.InkBlack].Dy(); height != 20 {
		t.Errorf("the box is %d pixels tall, want its 20 pixel minimum", height)
	}
}

func TestCustomProperties(t *testing.T) {
	got := boxes(t, `<i class="a"></i><i class="b"></i>`,
		`.page { --edge: 25px; --accent: red; }
		 .a { display: block; flex-basis: var(--edge); background: var(--accent); }
		 .b { display: block; flex-grow: 1; background: black; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 25, 50), "a sized and coloured from variables")
	expect(t, got, display.InkBlack, image.Rect(25, 0, 100, 50), "b after it")
}

func TestCustomPropertyFallbackAndMissing(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-basis: var(--nope, 30px); }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 30, 50), "the fallback")

	said := warningsFor(t, `<i class="a"></i>`,
		` .a { display: block; background: black; flex-basis: var(--nope); }`)
	if !strings.Contains(said, "--nope") {
		t.Errorf("an undeclared variable was not named: %q", said)
	}
}

// A maximum caps an item however it was sized, including one that would
// otherwise grow to fill the line.
func TestMaximumCapsAGrowingItem(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-grow: 1; max-width: 30px; } .b { flex-grow: 1; }`)
	if width := got[display.InkBlack].Dx(); width > 30 {
		t.Errorf("a grew to %d pixels past its 30 pixel maximum", width)
	}
}

// inherit takes the parent's value. Leaving the field alone is not the same
// thing: an earlier declaration on the same element may already have set it.
func TestInheritTakesTheParentsValueNotTheCurrentOne(t *testing.T) {
	got := boxes(t, `<i class="outer"><i class="a">x</i></i>`,
		`.outer { display: block; flex-grow: 1; color: red; }
		 .a { display: block; color: black; }
		 .a { color: inherit; }`)
	if _, red := got[display.InkRed]; !red {
		t.Errorf("inherit kept the element's own black instead of taking the parent's red; got %v", got)
	}
}

// The container's rectangle is known when the layout runs, so every form of
// absolute positioning resolves: from either edge, from both, and with the
// size left to the insets.
func TestAbsoluteFromEveryEdge(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want image.Rectangle
	}{
		{"top left with a size", `top: 5px; left: 10px; width: 20px; height: 15px;`,
			image.Rect(10, 5, 30, 20)},
		{"bottom right with a size", `bottom: 5px; right: 10px; width: 20px; height: 15px;`,
			image.Rect(70, 30, 90, 45)},
		{"stretched between opposite edges", `top: 5px; bottom: 5px; left: 10px; right: 10px;`,
			image.Rect(10, 5, 90, 45)},
		{"one edge, sized", `right: 0; top: 0; width: 25px; height: 10px;`,
			image.Rect(75, 0, 100, 10)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, `<i class="b"></i>`,
				`.b { display: block; background: red; position: absolute; `+test.css+` }`)
			expect(t, got, display.InkRed, test.want, test.name)
		})
	}
}

// z-index decides which of two overlapping boxes is on top, which for a
// renderer that paints once is simply which is painted last.
func TestZIndexOrdersOverlappingBoxes(t *testing.T) {
	const both = `<i class="a"></i><i class="b"></i>`
	const place = `.a { display: block; background: black; position: absolute;
		 top: 0; left: 0; width: 40px; height: 40px; }
		 .b { display: block; background: red; position: absolute;
		 top: 0; left: 0; width: 40px; height: 40px; }`

	// Document order alone puts b on top.
	got := boxes(t, both, place)
	if _, black := got[display.InkBlack]; black {
		t.Error("the later box did not cover the earlier one")
	}

	// Raising a puts it back over b.
	got = boxes(t, both, place+` .a { z-index: 1; }`)
	if _, red := got[display.InkRed]; red {
		t.Error("z-index did not raise the earlier box over the later one")
	}
	expect(t, got, display.InkBlack, image.Rect(0, 0, 40, 40), "the raised box")
}

// Magnifying draws the subtree onto a surface of its own and copies it over
// enlarged, so a bordered box becomes a bordered box with thicker lines rather
// than a larger box with the same one-pixel border.
func TestScaleMagnifiesTheWholeSubtree(t *testing.T) {
	plain := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; }`)
	doubled := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; scale: 2; }`)
	if doubled <= plain {
		t.Errorf("doubling used %d pixels of ink and the original %d", doubled, plain)
	}

	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-basis: 20px; height: 10px; scale: 2; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 20, 10), "a doubled box still fills the box it was given")
}

// A quarter turn is a transposition, so a wide box becomes a tall one.
func TestRotateTurnsTheSubtree(t *testing.T) {
	upright := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { display: block; flex-grow: 1; } .b { width: 100%; height: 20%; }`)
	turned := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { display: block; flex-grow: 1; rotate: 90deg; } .b { width: 100%; height: 20%; }`)
	if upright[display.InkRed].Dx() <= upright[display.InkRed].Dy() {
		t.Fatalf("the upright bar is %v, which is not wider than it is tall", upright[display.InkRed])
	}
	if turned[display.InkRed].Dy() <= turned[display.InkRed].Dx() {
		t.Errorf("after a quarter turn the bar is %v, still not taller than it is wide", turned[display.InkRed])
	}
}

// A magnification that would have to resample is refused rather than
// approximated. A turn is not: a turn is not a magnification, and everything
// under one works out where its own geometry goes rather than being redrawn
// at a new size, so any angle at all is exact for anything that is not
// already pixels.
func TestOnlyExactMagnificationsAreAccepted(t *testing.T) {
	for _, css := range []string{
		`.a { scale: 1.5; }`,
		`.a { scale: 2 3; }`,
	} {
		said := warningsFor(t, `<i class="a"></i>`, ` .a { display: block; background: black; flex-grow: 1; }`+css)
		if said == "" {
			t.Errorf("%s was accepted, but it cannot be done without resampling", css)
		}
	}
	// A whole magnification and an angle of any size are not refused.
	for _, css := range []string{
		`.a { scale: 3; }`, `.a { rotate: 180deg; }`, `.a { rotate: -90deg; }`,
		`.a { rotate: 45deg; }`, `.a { rotate: 37.5deg; }`, `.a { rotate: 0.2turn; }`,
		`.a { rotate: 30deg; transform-origin: top left; }`,
	} {
		said := warningsFor(t, `<i class="a"></i>`, ` .a { display: block; background: black; flex-grow: 1; }`+css)
		if said != "" {
			t.Errorf("%s was refused: %s", css, said)
		}
	}
}

// calc() is a share of the container and an adjustment to it, which is the
// form it is nearly always written in.
func TestCalcMixesPercentAndPixels(t *testing.T) {
	tests := []struct {
		css  string
		want image.Rectangle
	}{
		{`width: calc(100% - 20px);`, image.Rect(0, 0, 80, 50)},
		{`width: calc(50% + 5px);`, image.Rect(0, 0, 55, 50)},
		{`width: calc(30px + 10px);`, image.Rect(0, 0, 40, 50)},
		{`width: calc(25%);`, image.Rect(0, 0, 25, 50)},
	}
	for _, test := range tests {
		t.Run(test.css, func(t *testing.T) {
			got := boxes(t, `<i class="a"></i>`,
				`.a { display: block; background: black; `+test.css+` }`)
			expect(t, got, display.InkBlack, test.want, test.css)
		})
	}
}

// A calc that resolves below zero is clamped rather than inverted, and one
// written with something other than a length is refused by name.
func TestCalcEdges(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; height: 50px; width: calc(10% - 40px); }`)
	if _, drawn := got[display.InkBlack]; drawn {
		t.Errorf("a calc resolving below zero drew %v", got[display.InkBlack])
	}
	for _, css := range []string{
		`.a { width: calc(100% * 2); }`,
		`.a { width: calc(100% -); }`,
		`.a { width: calc(2em + 4px); }`,
	} {
		if said := warningsFor(t, `<i class="a"></i>`,
			` .a { display: block; background: black; flex-grow: 1; }`+css); said == "" {
			t.Errorf("%s was accepted silently", css)
		}
	}
}

// An inset given as a percentage resolves against the container, like every
// other length now does.
func TestPercentageInsets(t *testing.T) {
	got := boxes(t, `<i class="b"></i>`,
		`.b { display: block; background: red; position: absolute;
		 top: 20%; left: 25%; width: 50%; height: 40%; }`)
	expect(t, got, display.InkRed, image.Rect(25, 10, 75, 30), "an anchored box in percentages")
}

// The function form of transform is what most stylesheets say, and it composes.
func TestTransformFunctionForm(t *testing.T) {
	byProperty := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; scale: 2; }`)
	byFunction := countInk(t, `.a { display: block; flex-grow: 1; border: 1px solid black; transform: scale(2); }`)
	if byProperty != byFunction {
		t.Errorf("scale: 2 drew %d pixels and transform: scale(2) drew %d", byProperty, byFunction)
	}
	if said := warningsFor(t, `<i class="a"></i>`,
		` .a { display: block; background: black; flex-grow: 1; transform: rotate(90deg) scale(2); }`); said != "" {
		t.Errorf("a composed transform was refused: %s", said)
	}
	// Functions this build has no way to do are named, not lumped together.
	for _, css := range []string{`transform: skew(10deg)`, `transform: translate(4px, 4px)`, `transform: scale(1.5)`} {
		if said := warningsFor(t, `<i class="a"></i>`,
			` .a { display: block; background: black; flex-grow: 1; `+css+`; }`); said == "" {
			t.Errorf("%s was accepted", css)
		}
	}
}

// aspect-ratio ties the axes together, so a box that states one size takes the
// other from it.
func TestAspectRatio(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-basis: 40px; aspect-ratio: 2 / 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 40, 20), "a two-to-one box forty wide")

	square := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-basis: 30px; aspect-ratio: 1; }`)
	expect(t, square, display.InkBlack, image.Rect(0, 0, 30, 30), "a square")

	if said := warningsFor(t, `<i class="a"></i>`,
		` .a { display: block; background: black; flex-grow: 1; aspect-ratio: 0 / 3; }`); said == "" {
		t.Error("a ratio with a zero in it was accepted")
	}
}

// The reason grid was worth building: a column measured once across rows that
// know nothing about each other, so labels of different lengths still line up.
func TestGridColumnsLineUpAcrossRows(t *testing.T) {
	got := boxes(t,
		`<i class="g">`+
			`<span class="l">/</span><i class="bar"></i>`+
			`<span class="l">/backup</span><i class="bar"></i>`+
			`</i>`,
		`.g { display: grid; grid-template-columns: auto 1fr; flex-grow: 1;
		      font-family: monaco; font-size: 12px; }
		 .bar { display: block; background: red; }`)
	// Seven characters of Monaco 12 advance seven pixels each.
	if got[display.InkRed].Min.X != 49 {
		t.Errorf("the bars begin at x=%d, want 49: the widest label sets the column",
			got[display.InkRed].Min.X)
	}
	// Both bars share that edge, which a row of rows could not manage.
	if got[display.InkRed].Dy() < 20 {
		t.Errorf("the bars cover %v; both rows should have one", got[display.InkRed])
	}
}

func TestGridTracksAndPlacement(t *testing.T) {
	tests := []struct {
		name string
		css  string
		body string
		want image.Rectangle
	}{
		{"fr shares", `grid-template-columns: 1fr 3fr;`,
			`<i class="a"></i><i class="b"></i>`, image.Rect(25, 0, 100, 50)},
		{"repeat", `grid-template-columns: repeat(4, 1fr);`,
			`<i class="a"></i><i class="b"></i>`, image.Rect(25, 0, 50, 50)},
		{"fixed and fr", `grid-template-columns: 20px 1fr;`,
			`<i class="a"></i><i class="b"></i>`, image.Rect(20, 0, 100, 50)},
		{"explicit line", `grid-template-columns: repeat(3, 1fr);`,
			`<i class="a"></i><i class="b" style="grid-column: 3"></i>`, image.Rect(67, 0, 100, 50)},
		{"span", `grid-template-columns: repeat(4, 1fr);`,
			`<i class="a"></i><i class="b" style="grid-column: span 2"></i>`, image.Rect(25, 0, 75, 50)},
		{"column gap", `grid-template-columns: 1fr 1fr; column-gap: 10px;`,
			`<i class="a"></i><i class="b"></i>`, image.Rect(55, 0, 100, 50)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, `<i class="g">`+test.body+`</i>`,
				inks+` .g { display: grid; flex-grow: 1; `+test.css+` }`)
			expect(t, got, display.InkRed, test.want, test.name)
		})
	}
}

// Rows wrap when the columns run out, and an implicit row is as tall as it
// needs to be.
func TestGridWrapsOntoImplicitRows(t *testing.T) {
	got := boxes(t, `<i class="g"><i class="a"></i><i class="a"></i><i class="b"></i></i>`,
		inks+` .g { display: grid; flex-grow: 1; grid-template-columns: 1fr 1fr; }`)
	expect(t, got, display.InkRed, image.Rect(0, 25, 50, 50), "the third cell on a second row")
}

// The panel has clipped to an arbitrary path since early on, and CSS could not
// reach it: overflow only ever clips to the box. clip-path is the property
// that says the shape.
func TestClipPathShapes(t *testing.T) {
	const filled = `.a { display: block; background: black; flex-grow: 1; }`
	full := countInk(t, filled)

	tests := []struct {
		name string
		css  string
	}{
		{"inset", `clip-path: inset(10px);`},
		{"inset in percent", `clip-path: inset(25%);`},
		{"rounded inset", `clip-path: inset(0 round 8px);`},
		{"circle", `clip-path: circle(40%);`},
		{"circle placed", `clip-path: circle(10px at 20px 20px);`},
		{"ellipse", `clip-path: ellipse(30% 45%);`},
		{"polygon", `clip-path: polygon(0 0, 100% 0, 50% 100%);`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clipped := countInk(t, filled+` .a { `+test.css+` }`)
			if clipped == 0 {
				t.Fatalf("%s clipped everything away", test.css)
			}
			if clipped >= full {
				t.Errorf("%s covered %d pixels and the unclipped box covers %d", test.css, clipped, full)
			}
		})
	}
}

// A triangle keeps its top edge and loses its bottom corners, which a
// rectangle-shaped clip could not do.
func TestPolygonClipIsNotJustARectangle(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-grow: 1;
		      clip-path: polygon(50% 0, 100% 100%, 0 100%); }`)
	// The apex is a single column at the top; the base spans the full width.
	frame, _ := renderProbe(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-grow: 1;
		      clip-path: polygon(50% 0, 100% 100%, 0 100%); }`)
	top, bottom := rowWidth(frame, 1), rowWidth(frame, frame.Height()-1)
	if top >= bottom {
		t.Errorf("the shape is %d wide at the top and %d at the bottom; that is not a triangle", top, bottom)
	}
	if got[display.InkBlack].Dx() < 50 {
		t.Errorf("the base only covers %d pixels", got[display.InkBlack].Dx())
	}
}

func rowWidth(frame *display.Frame, y int) int {
	count := 0
	for x := 0; x < frame.Width(); x++ {
		if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
			count++
		}
	}
	return count
}

// overflow:hidden on a rounded box clips to the rounded box, not to the
// rectangle around it.
func TestOverflowFollowsTheBorderRadius(t *testing.T) {
	const body = `<i class="o"><i class="fill"></i></i>`
	const base = `.o { display: block; flex-grow: 1; overflow: hidden; }
		 .fill { display: block; background: black; width: 100%; height: 100%; }`
	square := inkIn(t, body, base)
	rounded := inkIn(t, body, base+` .o { border-radius: 12px; }`)
	if rounded == 0 || square == 0 {
		t.Fatalf("nothing was drawn: square=%d rounded=%d", square, rounded)
	}
	if rounded >= square {
		t.Errorf("the rounded clip kept %d pixels and the square one %d; corners should be gone",
			rounded, square)
	}
}

func inkIn(t *testing.T, body, css string) int {
	t.Helper()
	frame, said := renderProbe(t, body, css)
	if frame == nil {
		t.Fatalf("nothing rendered: %s", said)
	}
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				count++
			}
		}
	}
	return count
}

// An image is content rather than a container, and it was taking a shortcut
// out of the compiler that skipped clipping and transforming. A circular
// portrait came out square and said nothing about it.
func TestAnImageIsClippedAndTransformedLikeAnythingElse(t *testing.T) {
	// A solid block rather than the checkerboard the other probes use.
	// Automatic processing profiles the picture and chooses a treatment from
	// what it finds, so a checkerboard and a filled square are not fitted the
	// same way — and this test is about the clip, not about the profiler.
	directory := t.TempDir()
	block := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			block.Set(x, y, image.Black)
		}
	}
	file, err := os.Create(filepath.Join(directory, "p.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, block); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	inkFor := func(t *testing.T, css string) int {
		t.Helper()
		document, err := Compile(
			`<div class="page"><img src="p.png" class="a"></div>`,
			`.page { display: flex; width: 60px; height: 60px; background: white; } `+css)
		if err != nil {
			t.Fatal(err)
		}
		for _, warning := range document.Warnings {
			t.Errorf("warning: %s", warning.Message)
		}
		frame, _ := renderDocument(t, directory+string(filepath.Separator), document.JSON)
		count := 0
		for y := 0; y < frame.Height(); y++ {
			for x := 0; x < frame.Width(); x++ {
				if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
					count++
				}
			}
		}
		return count
	}

	square := inkFor(t, `.a { flex-grow: 1; }`)
	circle := inkFor(t, `.a { flex-grow: 1; clip-path: circle(50%); }`)
	if square == 0 {
		t.Fatal("the image drew nothing")
	}
	if circle >= square {
		t.Errorf("clipping the image to a circle kept %d pixels of %d", circle, square)
	}
	// A circle inscribed in a square covers about π/4 of it.
	if ratio := float64(circle) / float64(square); ratio < 0.6 || ratio > 0.9 {
		t.Errorf("the circle covers %.2f of the square, which is not a circle", ratio)
	}
}
