package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestVisibilityCanBeOverriddenByAnInlineDescendant(t *testing.T) {
	got := boxes(t, `<i class="parent"><i class="child" style="visibility: visible"></i></i>`,
		`.parent { display: block; flex-grow: 1; visibility: hidden; }
		 .child { display: block; width: 20px; height: 20px; background: red; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 20, 20), "a visible child inside a hidden parent")
}

func TestVisibilityHiddenStillHidesAnOrdinaryDescendant(t *testing.T) {
	got := boxes(t, `<i class="parent">hidden text<i class="child"></i></i>`,
		`.parent { display: block; flex-grow: 1; visibility: hidden; }
		 .child { display: block; width: 20px; height: 20px; background: red; }`)
	if _, drawn := got[display.InkRed]; drawn {
		t.Errorf("a child inherited visible from a hidden parent: %v", got[display.InkRed])
	}
}

func TestSVGVisibilityCanBeOverriddenInsideAHiddenGroup(t *testing.T) {
	page, err := Compile(
		`<div class="page"><svg width="60" height="40"><g class="hidden"><rect width="10" height="10" fill="black"/><rect class="show" x="20" width="10" height="10" fill="red"/></g></svg></div>`,
		`.page { display: flex; width: 60px; height: 40px; }
		 svg { display: block; flex-grow: 1; }
		 svg g.hidden { visibility: hidden; }
		 svg g.hidden .show { visibility: visible; }`)
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(page.JSON)), "")
	if !strings.Contains(flat, `"fill":"red"`) {
		t.Fatalf("visible descendant of hidden group was lost:\n%s", page.JSON)
	}
	if strings.Contains(flat, `"fill":"black"`) {
		t.Fatalf("hidden shape in group was painted:\n%s", page.JSON)
	}
}

func TestHiddenSVGKeepsItsLayoutSpace(t *testing.T) {
	got := boxes(t, `<svg class="hidden" width="20" height="50"><rect width="20" height="50"/></svg><i class="sibling"></i>`,
		`.hidden { display: block; width: 20px; height: 50px; visibility: hidden; }
		 .sibling { display: block; flex-grow: 1; background: red; }`)
	expect(t, got, display.InkRed, image.Rect(20, 0, 100, 50), "the sibling after a hidden SVG")
}

func TestDisplayContentsKeepsTheParentLayoutAndCarriesPaint(t *testing.T) {
	got := boxes(t, `<i class="outer"><i class="contents"><i class="a"></i><i class="b"></i></i></i>`,
		`.outer { display: flex; flex-grow: 1; }
		 .contents { display: contents; color: red; }
		 .a { display: block; flex-basis: 30px; background: red; }
		 .b { display: block; flex-basis: 40px; background: black; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 30, 50), "the first contents child")
	expect(t, got, display.InkBlack, image.Rect(30, 0, 70, 50), "the second contents child")
	document, err := Compile(`<div class="page"><i class="outer"><span class="contents"><span>x</span></span></i></div>`,
		`.page { display: flex; width: 100px; height: 50px; }
		 .outer { display: flex; flex-grow: 1; }
		 .contents { display: contents; color: red; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(document.JSON)), ""), `"ink":"red"`) {
		t.Fatalf("display:contents did not carry color to an exposed text child:\n%s", document.JSON)
	}
}

func TestNestedAbsolutePositionUsesTheNearestPositionedAncestor(t *testing.T) {
	got := boxes(t, `<i class="frame"><i class="wrapper"><i class="pin"></i></i></i>`,
		`.frame { display: block; flex-grow: 1; position: relative; }
		 .wrapper { display: block; width: 50px; height: 20px; margin: 10px 0 0 30px; }
		 .pin { display: block; position: absolute; top: 5px; left: 7px; width: 10px; height: 10px; background: red; }`)
	expect(t, got, display.InkRed, image.Rect(7, 5, 17, 15), "the pin anchored to frame rather than wrapper")
}

// Text sits at the top of its box, as it does in CSS, and centring is
// something a page asks for.
//
// It was defaulted to middle for two days. That is a divergence a web author
// would not expect, and it had a cost this could not have: a line of Chinese
// and a line of mixed Chinese and Latin have ink of different heights, so
// centring settled them a pixel apart whenever the leftover was odd. At the
// top there is nothing to halve.
func TestTextSitsAtTheTopOfItsBoxUnlessAsked(t *testing.T) {
	document, err := Compile(`<div class="page"><span class="label">x</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; }
		 .label { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	if flat := strings.Join(strings.Fields(string(document.JSON)), ""); strings.Contains(flat, `"verticalAlign"`) {
		t.Fatalf("text that said nothing about alignment was given one:\n%s", document.JSON)
	}

	centred, err := Compile(`<div class="page"><span class="label">x</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; }
		 .label { display: block; flex-grow: 1; vertical-align: middle; }`)
	if err != nil {
		t.Fatal(err)
	}
	if flat := strings.Join(strings.Fields(string(centred.JSON)), ""); !strings.Contains(flat, `"verticalAlign":"middle"`) {
		t.Fatalf("asking for middle did not produce it:\n%s", centred.JSON)
	}
}

func TestFlexGapUsesTheGapForTheActiveAxis(t *testing.T) {
	row := boxes(t, `<i class="a"></i><i class="b"></i>`,
		`.a { display: block; width: 20px; background: black; }
		 .b { display: block; width: 20px; background: red; }
		 .page { column-gap: 10px; row-gap: 3px; }`)
	expect(t, row, display.InkRed, image.Rect(30, 0, 50, 50), "the row column-gap")

	column := boxes(t, `<i class="column"><i class="a"></i><i class="b"></i></i>`,
		`.column { display: flex; flex-direction: column; flex-grow: 1; }
		 .column { row-gap: 5px; }
		 .a { display: block; height: 10px; background: black; }
		 .b { display: block; height: 10px; background: red; }`)
	expect(t, column, display.InkRed, image.Rect(0, 15, 100, 25), "the column row-gap")
}

func TestInsetShorthandKeepsPercentages(t *testing.T) {
	got := boxes(t, `<i class="pin"></i>`,
		`.pin { display: block; position: absolute; inset: 10% auto auto 20%; width: 20px; height: 10px; background: red; }`)
	expect(t, got, display.InkRed, image.Rect(20, 5, 40, 15), "percentage inset shorthand")
}

func TestFractionalGridTrackIsRejectedInsteadOfTruncated(t *testing.T) {
	said := warningsFor(t, `<i class="a"></i>`,
		`.a { display: grid; grid-template-columns: 0.5fr 1fr; }`)
	if !strings.Contains(said, "positive number of fr") {
		t.Errorf("fractional fr was silently accepted: %q", said)
	}
}

func TestUnsetInheritsOnlyInheritedProperties(t *testing.T) {
	parent := style{color: display.InkRed, hidden: true, background: inkPointer(display.InkWhite)}
	child := style{color: display.InkBlack, hidden: false, background: inkPointer(display.InkRed)}
	child.apply("color", "unset", parent, func(string) { t.Fatal("color: unset reported") })
	child.apply("visibility", "unset", parent, func(string) { t.Fatal("visibility: unset reported") })
	child.apply("background", "unset", parent, func(string) { t.Fatal("background: unset reported") })
	if child.color != display.InkRed || !child.hidden || child.background != nil {
		t.Fatalf("unset produced color=%v hidden=%v background=%v", child.color, child.hidden, child.background)
	}
}

func TestInheritedBorderIsCopiedPerField(t *testing.T) {
	parent := style{
		border:     &border{width: 3, ink: display.InkRed, radius: 4},
		line:       borderDashed,
		dash:       []int{6, 2},
		dashOffset: 1,
	}
	child := style{border: &border{width: 9, ink: display.InkWhite, radius: 8}}
	child.inheritOne("border-color", parent, func(string) { t.Fatal("border-color: inherit reported") })
	if child.border.width != 9 || child.border.ink != display.InkRed || child.border.radius != 8 {
		t.Fatalf("border-color inherit changed unrelated fields: %+v", *child.border)
	}
	child.inheritOne("border", parent, func(string) { t.Fatal("border: inherit reported") })
	if child.border == parent.border || &child.dash[0] == &parent.dash[0] {
		t.Fatal("border inherit aliased mutable parent state")
	}
	child.border.width = 7
	child.dash[0] = 99
	if parent.border.width != 3 || parent.dash[0] != 6 {
		t.Fatal("mutating an inherited border changed the parent")
	}
}

func TestBorderShorthandResetsAnEarlierDashedStyle(t *testing.T) {
	var current style
	current.apply("border-style", "dashed", style{}, func(string) { t.Fatal("border-style reported") })
	current.apply("border", "1px solid red", style{}, func(string) { t.Fatal("border reported") })
	if current.line != borderSolid || len(current.dash) != 0 || current.border == nil || current.border.ink != display.InkRed {
		t.Fatalf("border shorthand left dashed state behind: %+v line=%v dash=%v", *current.border, current.line, current.dash)
	}
	current.apply("border", "none", style{}, func(string) { t.Fatal("border:none reported") })
	if current.border == nil || current.line != borderNone || len(current.dash) != 0 {
		t.Fatalf("border:none left a line behind: border=%v line=%v dash=%v", current.border, current.line, current.dash)
	}
	current.apply("border", "1px solid red", style{}, func(string) { t.Fatal("border reported") })
	current.apply("border-style", "none", style{}, func(string) { t.Fatal("border-style:none reported") })
	if current.line != borderNone {
		t.Fatalf("border-style:none did not suppress the line: %v", current.line)
	}
}

func inkPointer(ink display.Ink) *display.Ink { return &ink }

// The border shorthand is written in any order and the style keyword belongs
// in it. Reading only solid sent "dashed" to the colour parser, which named it
// as an ink the panel has not got and pointed the author at a colour they had
// not written wrong.
func TestTheBorderShorthandTakesAStyleKeyword(t *testing.T) {
	for _, shorthand := range []string{"1px dashed black", "dashed 1px black", "black dashed 1px"} {
		t.Run(shorthand, func(t *testing.T) {
			document, err := Compile(`<div class="page"><i class="a"></i></div>`,
				`.page { display: flex; width: 60px; height: 30px; }
				 .a { flex-grow: 1; border: `+shorthand+`; }`)
			if err != nil {
				t.Fatal(err)
			}
			for _, warning := range document.Warnings {
				t.Errorf("%q was reported: %s", shorthand, warning.Message)
			}
			if flat := strings.Join(strings.Fields(string(document.JSON)), ""); !strings.Contains(flat, `"dash":`) {
				t.Errorf("%q did not produce a dashed border:\n%s", shorthand, document.JSON)
			}
		})
	}
}

// A stylesheet that reaches a page by two of its three routes at once is read
// once. A link naming the file that also sits beside the page is the way this
// happens, and applying it twice said everything it had to say twice.
//
// It is recognised by name and never by content, because two different files
// that happen to hold the same rules are two stylesheets and the second still
// wins. Deduplicating by content dropped the link and left the style element
// that the link was written to override.
func TestAStylesheetThatArrivesTwiceIsReadOnce(t *testing.T) {
	const sheet = `.page { display: flex; width: 60px; height: 30px; } .a { letter-spacing: 1px; }`
	document, err := Compiler{
		Stylesheets:    func(string) ([]byte, error) { return []byte(sheet), nil },
		StylesheetName: "page.css",
	}.Compile(`<link rel="stylesheet" href="page.css"><div class="page"><i class="a"></i></div>`, sheet)
	if err != nil {
		t.Fatal(err)
	}
	unsupported, duplicate := 0, 0
	for _, warning := range document.Warnings {
		switch warning.Code {
		case "unsupported-declaration":
			unsupported++
		case "duplicate-stylesheet":
			duplicate++
		}
	}
	if unsupported != 1 {
		t.Errorf("the same declaration was reported %d times, want once", unsupported)
	}
	if duplicate != 1 {
		t.Errorf("the duplicate was not named once: %d", duplicate)
	}
}

// Both edges and a size on one axis is more than can hold. CSS says the end
// edge gives; the schema refuses all three, so a page that says it used to be
// refused outright — the one thing in the pipeline that stopped rather than
// warned. The message names inset when that is where the edges came from.
func TestAnOverConstrainedAnchorDropsTheEndEdgeAndSaysSo(t *testing.T) {
	document, err := Compile(`<div class="page"><i class="pin"></i></div>`,
		`.page { position: relative; width: 200px; height: 100px; }
		 .pin { position: absolute; inset: 0; width: 50px; height: 50px; background: red; }`)
	if err != nil {
		t.Fatalf("the page was refused rather than warned about: %v", err)
	}
	var said string
	for _, warning := range document.Warnings {
		if warning.Code == "over-constrained" {
			said += warning.Message
		}
	}
	if !strings.Contains(said, "inset shorthand") {
		t.Errorf("the message did not say the edges came from inset: %q", said)
	}
	// And what it compiled to is accepted, which is the point of dropping one.
	if _, err := drawn(t, `<div class="page"><i class="pin"></i></div>`,
		`.page { position: relative; width: 200px; height: 100px; }
		 .pin { position: absolute; inset: 0; width: 50px; height: 50px; background: red; }`); err != nil {
		t.Errorf("the document it produced was refused: %v", err)
	}
}

// Two different stylesheets that say the same thing are two stylesheets, and
// the cascade is the order the page names them in. A page whose sibling and
// whose link hold identical rules, with a style element between them saying
// something else, ends up with what the link says — because the link is last.
func TestIdenticalStylesheetsFromDifferentFilesBothApply(t *testing.T) {
	const wins = `.x { color: red; }`
	document, err := Compiler{
		Stylesheets:    func(string) ([]byte, error) { return []byte(wins), nil },
		StylesheetName: "page.css",
	}.Compile(
		`<div class="page"><span class="x">X</span></div>`+
			`<style>.x { color: white; }</style>`+
			`<link rel="stylesheet" href="other.css">`,
		`.page { display: flex; width: 60px; height: 20px; }`+wins)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		if warning.Code == "duplicate-stylesheet" {
			t.Errorf("two different files were treated as one: %s", warning.Message)
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(string(document.JSON)), ""), `"ink":"red"`) {
		t.Errorf("the last stylesheet named did not win:\n%s", document.JSON)
	}
}

// The shorthand matches every field by what it is, so a style keyword is read
// as a style rather than handed to the colour parser and reported as an ink
// nobody wrote. A style this cannot draw is not drawn at all: solid instead
// would put a line on the page that the author asked for in another shape.
//
// dotted is drawn, because a dot the width of the border spaced by the same is
// a dash pattern and the stroke already takes one.
func TestTheBorderShorthandNamesWhatItCannotDraw(t *testing.T) {
	for _, style := range []string{"dotted", "double", "groove", "ridge", "outset"} {
		t.Run(style, func(t *testing.T) {
			document, err := Compile(`<div class="page"><i class="a"></i></div>`,
				`.page { display: flex; width: 60px; height: 30px; }
				 .a { flex-grow: 1; border: 1px `+style+` black; }`)
			if err != nil {
				t.Fatal(err)
			}
			var said string
			for _, warning := range document.Warnings {
				said += warning.Message
			}
			if strings.Contains(said, "ink") {
				t.Errorf("%s was offered to the colour parser: %q", style, said)
			}
			drawn := strings.Contains(strings.Join(strings.Fields(string(document.JSON)), ""), `"stroke":`)
			if style == "dotted" {
				if said != "" {
					t.Errorf("dotted was reported: %q", said)
				}
				if !drawn {
					t.Errorf("dotted was not drawn:\n%s", document.JSON)
				}
				return
			}
			if !strings.Contains(said, "not drawn") {
				t.Errorf("%s was not reported as a style that is not drawn: %q", style, said)
			}
			if drawn {
				t.Errorf("%s was reported and then drawn anyway:\n%s", style, document.JSON)
			}
		})
	}

	for _, shorthand := range []string{"1PX DASHED BLACK", "2Px Solid Red", "1px DOTTED black"} {
		t.Run(shorthand, func(t *testing.T) {
			said := warningsFor(t, `<i class="a"></i>`, ` .a { display: block; border: `+shorthand+`; }`)
			if said != "" {
				t.Errorf("%q was reported; CSS matches keywords and units without regard to case: %q",
					shorthand, said)
			}
		})
	}
}

// A border with a width and no style draws nothing, because CSS starts
// border-style at none. It used to draw a solid line nobody asked for.
func TestABorderWithNoStyleIsNotDrawn(t *testing.T) {
	for _, declaration := range []string{"border: 1px", "border: 2px black", "border-width: 3px"} {
		t.Run(declaration, func(t *testing.T) {
			document, err := Compile(`<div class="page"><i class="a"></i></div>`,
				`.page { display: flex; width: 60px; height: 30px; }
				 .a { flex-grow: 1; `+declaration+`; }`)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Join(strings.Fields(string(document.JSON)), ""), `"stroke":`) {
				t.Errorf("%q drew a line:\n%s", declaration, document.JSON)
			}
		})
	}
}

// Dots scale with the border they are drawn in, as they do in CSS: a dot is as
// wide as the border and is spaced by the same.
func TestADottedBorderIsDotsTheWidthOfTheBorder(t *testing.T) {
	for width, want := range map[string]string{"1px": `"dash":[1,1]`, "3px": `"dash":[3,3]`} {
		document, err := Compile(`<div class="page"><i class="a"></i></div>`,
			`.page { display: flex; width: 60px; height: 30px; }
			 .a { flex-grow: 1; border: `+width+` dotted black; }`)
		if err != nil {
			t.Fatal(err)
		}
		if flat := strings.Join(strings.Fields(string(document.JSON)), ""); !strings.Contains(flat, want) {
			t.Errorf("a %s dotted border is not %s:\n%s", width, want, document.JSON)
		}
	}
}

// The message about an over-constrained anchor names inset only for the edges
// that actually came from it. One longhand after the shorthand is the author's
// own, and telling them about a shorthand they have overridden is worse than
// saying nothing about where it came from.
func TestTheOverConstrainedMessageNamesOnlyTheEdgesInsetGave(t *testing.T) {
	said := warningsFor(t, `<i class="pin"></i>`,
		` .pin { position: absolute; inset: 0; left: 10px; width: 50px; }`)
	if !strings.Contains(said, "right comes from the inset shorthand") {
		t.Errorf("the message did not name right alone: %q", said)
	}
	if strings.Contains(said, "left and right come from") {
		t.Errorf("the message still claims left came from inset: %q", said)
	}

	// And with no shorthand in sight it names neither.
	own := warningsFor(t, `<i class="pin"></i>`,
		` .pin { position: absolute; left: 0; right: 0; width: 50px; }`)
	if strings.Contains(own, "inset shorthand") {
		t.Errorf("inset was named for edges written by hand: %q", own)
	}
}

// vertical-align is part of the inline formatting context. It is accepted on
// inline elements and participates in line-box placement.
func TestVerticalAlignOnAnInlineElementIsApplied(t *testing.T) {
	said := warningsFor(t, `<p>big <span class="a">X</span></p>`,
		` .a { vertical-align: middle; }`)
	if strings.Contains(said, "vertical-align") {
		t.Errorf("vertical-align on a span was reported: %q", said)
	}

	// On the box that holds the text it is what the property is for.
	if said := warningsFor(t, `<i class="a">x</i>`,
		` .a { display: block; height: 20px; vertical-align: middle; }`); said != "" {
		t.Errorf("vertical-align on a box was reported: %q", said)
	}
}

// A grid item is blockified, as a flex item is and as CSS says. Only flex was
// doing it, so a span in a grid cell stayed inline and every property that
// only means something on a box quietly did nothing to it — which is how
// examples/desk/disk.html came to carry four dead vertical-align declarations
// nobody could see were dead.
func TestAGridItemIsBlockifiedLikeAFlexItem(t *testing.T) {
	for _, container := range []string{"flex", "grid"} {
		t.Run(container, func(t *testing.T) {
			document, err := Compile(
				`<div class="page"><div class="box"><span class="cell">x</span></div></div>`,
				`.page { display: flex; width: 60px; height: 40px; }
				 .box { display: `+container+`; flex-grow: 1; }
				 .cell { height: 30px; vertical-align: middle; }`)
			if err != nil {
				t.Fatal(err)
			}
			for _, warning := range document.Warnings {
				t.Errorf("a %s item was still inline: %s", container, warning.Message)
			}
			if flat := strings.Join(strings.Fields(string(document.JSON)), ""); !strings.Contains(flat, `"verticalAlign":"middle"`) {
				t.Errorf("the %s item did not take the alignment:\n%s", container, document.JSON)
			}
		})
	}
}

// CSS matches keywords, units and function names without regard to case, and a
// stylesheet copied from anywhere may be written in any of them. Only the
// things that are names rather than keywords keep their case, and a font
// family is named back the way it was written.
func TestKeywordsAndUnitsAreMatchedWithoutRegardToCase(t *testing.T) {
	for _, declaration := range []string{
		"box-sizing: BORDER-BOX", "display: FLEX", "position: ABSOLUTE", "overflow: HIDDEN",
		"text-align: CENTER", "vertical-align: MIDDLE", "white-space: NOWRAP",
		"visibility: HIDDEN", "border-style: DOTTED", "object-fit: COVER",
		"width: 50PX", "padding: 5PX", "margin-left: AUTO", "flex-direction: COLUMN",
		"color: RED", "background: WHITE", "align-items: CENTER", "justify-content: SPACE-BETWEEN",
		"clip-path: CIRCLE(50%)", "transform: ROTATE(90DEG)", "transform: NONE", "rotate: 90DEG",
		"transform-origin: TOP LEFT", "font-family: MONACO", "grid-column: 1 / SPAN 2",
		"grid-template-columns: AUTO 1FR", "display: INHERIT", "color: INITIAL",
	} {
		t.Run(declaration, func(t *testing.T) {
			said := warningsFor(t, `<i class="a">x</i>`,
				` .a { display: block; width: 20px; height: 20px; `+declaration+`; }`)
			if said != "" {
				t.Errorf("%q was reported: %q", declaration, said)
			}
		})
	}

	// The family is reported the way it was written, not the way it was matched.
	said := warningsFor(t, `<i class="a">x</i>`, ` .a { font-family: "Helvetica Neue"; }`)
	if !strings.Contains(said, "Helvetica Neue") {
		t.Errorf("the family was not named as the author wrote it: %q", said)
	}
}
