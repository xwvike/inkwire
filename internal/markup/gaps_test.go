package markup

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
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
func TestImageIsDrawnThroughTheImageNode(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			shade := uint8(0xFF)
			if (x+y)%2 == 0 {
				shade = 0
			}
			source.SetNRGBA(x, y, color.NRGBA{R: shade, G: shade, B: shade, A: 0xFF})
		}
	}
	compiler := Compiler{Images: func(src string) (image.Image, error) {
		if src != "photo.png" {
			return nil, fmt.Errorf("unexpected src %q", src)
		}
		return source, nil
	}}
	document, err := compiler.Compile(
		`<div class="page"><img src="photo.png" class="p"></div>`,
		`.page { display: flex; width: 40px; height: 40px; } .p { flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	composed, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, _, err := composed.Compile(compose.Document{Size: image.Pt(40, 40), Root: document.Root})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
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
func TestImageWithoutAResolverIsReported(t *testing.T) {
	said := warningsFor(t, `<img src="photo.png" class="a">`, ` .a { flex-grow: 1; }`)
	if !strings.Contains(said, "photo.png") {
		t.Errorf("an unresolvable img was not reported by name: %q", said)
	}
}

func TestObjectFitReachesTheImageNode(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 4, 8))
	compiler := Compiler{Images: func(string) (image.Image, error) { return source, nil }}
	for _, fit := range []string{"fill", "contain", "cover"} {
		t.Run(fit, func(t *testing.T) {
			document, err := compiler.Compile(
				`<div class="page"><img src="p.png" class="p"></div>`,
				`.page { display: flex; width: 40px; height: 40px; } .p { flex-grow: 1; object-fit: `+fit+`; }`)
			if err != nil {
				t.Fatal(err)
			}
			for _, warning := range document.Warnings {
				t.Errorf("warning: %s", warning.Message)
			}
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
