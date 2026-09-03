package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// These are the things someone writing CSS without thinking about this
// renderer would reach for. Anything here that does not work is a place where
// they would have to start thinking about it.

func TestNestedFlexThreeDeep(t *testing.T) {
	got := boxes(t,
		`<i class="outer"><i class="mid"><i class="a"></i><i class="b"></i></i></i>`,
		inks+`
		.outer { display: flex; flex-direction: column; flex-grow: 1; padding: 5px; }
		.mid { display: flex; flex-grow: 1; gap: 10px; }
		.a { flex-grow: 1; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(5, 5, 45, 45), "a inside two nested flex boxes")
	expect(t, got, display.InkRed, image.Rect(55, 5, 95, 45), "b after the inner gap")
}

func TestMarginAutoOnBothSidesCentres(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-basis: 40px; margin-left: auto; margin-right: auto; }`)
	expect(t, got, display.InkBlack, image.Rect(30, 0, 70, 50), "an item centred by auto margins")
}

func TestSeveralAutoMarginsShareTheSlack(t *testing.T) {
	got := boxes(t, `<i class="a"></i><i class="b"></i>`,
		inks+` .a { flex-basis: 20px; margin-left: auto; } .b { flex-basis: 20px; margin-left: auto; }`)
	// Sixty pixels of slack, two auto margins, thirty each.
	expect(t, got, display.InkBlack, image.Rect(30, 0, 50, 50), "a after the first share")
	expect(t, got, display.InkRed, image.Rect(80, 0, 100, 50), "b after the second")
}

func TestJustifyContent(t *testing.T) {
	for _, test := range []struct {
		value string
		want  image.Rectangle
	}{
		{"flex-start", image.Rect(0, 0, 40, 50)},
		{"center", image.Rect(30, 0, 70, 50)},
		{"flex-end", image.Rect(60, 0, 100, 50)},
	} {
		t.Run(test.value, func(t *testing.T) {
			got := boxes(t, `<i class="a"></i>`,
				`.a { display: block; background: black; flex-basis: 40px; }`+
					` .page { justify-content: `+test.value+`; }`)
			expect(t, got, display.InkBlack, test.want, "justify-content: "+test.value)
		})
	}
}

func TestFlexShorthandForms(t *testing.T) {
	for _, form := range []string{"flex: 1", "flex: 1 1 0", "flex: 1 1 auto"} {
		t.Run(form, func(t *testing.T) {
			got := boxes(t, `<i class="a"></i><i class="b"></i>`,
				inks+` .a { flex-basis: 20px; } .b { `+form+`; }`)
			expect(t, got, display.InkRed, image.Rect(20, 0, 100, 50), form)
		})
	}
}

func TestAlignSelfOverridesTheContainer(t *testing.T) {
	got := boxes(t, `<i class="a"></i><i class="b"></i>`,
		inks+` .page { align-items: flex-start; } .a { flex-grow: 1; height: 10px; }
		.b { flex-grow: 1; height: 10px; align-self: flex-end; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 50, 10), "a at the container's alignment")
	expect(t, got, display.InkRed, image.Rect(50, 40, 100, 50), "b at its own")
}

func TestBorderLonghands(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; flex-grow: 1; border-width: 1px; border-style: solid; border-color: black; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 100, 50), "a border set through longhands")
}

func TestSingleSideBorderIsPaintedAndCountsInTheBox(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; flex-grow: 1; border-bottom: 1px solid black; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 49, 100, 50), "border-bottom paints the bottom edge")
}

// Specificity decides which rule wins, and getting it wrong shows up as a
// colour or a size quietly coming from the wrong place.
func TestSpecificity(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want display.Ink
	}{
		{"id beats class", `#x { background: red; } .a { background: black; }`, display.InkRed},
		{"two classes beat one", `.a.y { background: red; } .a { background: black; }`, display.InkRed},
		{"later of equal weight wins", `.a { background: black; } .y { background: red; }`, display.InkRed},
		{"element loses to class", `i { background: black; } .a { background: red; }`, display.InkRed},
		{"descendant counts", `.page .a { background: red; } .a { background: black; }`, display.InkRed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, `<i id="x" class="a y"></i>`,
				`.a { display: block; flex-grow: 1; } `+test.css)
			if _, drawn := got[test.want]; !drawn {
				t.Errorf("the winning rule did not paint; got %v", got)
			}
		})
	}
}

func TestNestedAndChildSelectorsMatch(t *testing.T) {
	const layout = `.outer { display: block; flex-grow: 1; }
		.middle { display: block; }
		.target { display: block; width: 10px; height: 10px; }`

	direct := boxes(t, `<div class="outer"><i class="target"></i></div>`,
		layout+` .outer > .target { background: red; }`)
	expect(t, direct, display.InkRed, image.Rect(0, 0, 10, 10), "a direct child selector")

	deep := boxes(t, `<div class="outer"><div class="middle"><i class="target"></i></div></div>`,
		layout+` .outer .target { background: red; }`)
	expect(t, deep, display.InkRed, image.Rect(0, 0, 10, 10), "a deep descendant selector")
}

func TestInlineStyleBeatsEverySelector(t *testing.T) {
	got := boxes(t, `<i id="x" class="a" style="background: red"></i>`,
		`#x { background: black; } .a { display: block; flex-grow: 1; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 100, 50), "the style attribute")
}

func TestImportantWins(t *testing.T) {
	got := boxes(t, `<i id="x" class="a"></i>`,
		`.a { display: block; flex-grow: 1; background: red !important; } #x { background: black; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 100, 50), "an important declaration")
}

func TestInlineImportantBeatsImportantStylesheet(t *testing.T) {
	got := boxes(t, `<i id="x" class="a" style="background: red !important"></i>`,
		`.a { display: block; flex-grow: 1; background: black !important; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 100, 50), "an inline important declaration")
}

// gap has no meaning outside a flex container, so it should say so rather than
// look as though it did something.
func TestGapOnABlockContainerIsReported(t *testing.T) {
	document, err := Compile(`<div class="page"><i class="a"></i></div>`,
		`.page { display: block; width: 100px; height: 50px; gap: 10px; }`+
			` .a { display: block; background: black; }`)
	if err != nil {
		t.Fatal(err)
	}
	var joined string
	for _, warning := range document.Warnings {
		joined += warning.Message
	}
	if !strings.Contains(joined, "gap") {
		t.Errorf("gap on a block container was accepted silently; warnings: %q", joined)
	}
}
