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

func TestTextDefaultsToMiddleAlignment(t *testing.T) {
	document, err := Compile(`<div class="page"><span class="label">x</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; }
		 .label { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(document.JSON)), "")
	if !strings.Contains(flat, `"verticalAlign":"middle"`) {
		t.Fatalf("default text alignment was not emitted as middle:\n%s", document.JSON)
	}

	top, err := Compile(`<div class="page"><span class="label">x</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; }
		 .label { display: block; flex-grow: 1; vertical-align: top; }`)
	if err != nil {
		t.Fatal(err)
	}
	topFlat := strings.Join(strings.Fields(string(top.JSON)), "")
	if strings.Contains(topFlat, `"verticalAlign":"middle"`) {
		t.Fatalf("explicit top alignment was replaced by middle:\n%s", top.JSON)
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
		dashed:     true,
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
	if current.dashed || len(current.dash) != 0 || current.border == nil || current.border.ink != display.InkRed {
		t.Fatalf("border shorthand left dashed state behind: %+v dashed=%v dash=%v", *current.border, current.dashed, current.dash)
	}
	current.apply("border", "none", style{}, func(string) { t.Fatal("border:none reported") })
	if current.border != nil || current.dashed || len(current.dash) != 0 {
		t.Fatalf("border:none left a border behind: border=%v dashed=%v dash=%v", current.border, current.dashed, current.dash)
	}
	current.apply("border", "1px solid red", style{}, func(string) { t.Fatal("border reported") })
	current.apply("border-style", "none", style{}, func(string) { t.Fatal("border-style:none reported") })
	if current.border == nil || current.border.width != 0 {
		t.Fatalf("border-style:none did not suppress the border: %+v", current.border)
	}
}

func inkPointer(ink display.Ink) *display.Ink { return &ink }
