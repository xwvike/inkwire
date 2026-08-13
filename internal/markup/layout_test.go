package markup

import (
	"fmt"
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

// box renders one document on a 100x50 page and reports the rectangle each
// marked element actually covers. Testing geometry rather than appearance is
// the only way to tell a property that works from one that happens not to
// matter in the page it was written for.
func boxes(t *testing.T, body, css string) map[display.Ink]image.Rectangle {
	t.Helper()
	const page = `<div class="page">` + "%s" + `</div>`
	const frame = `.page { display: flex; width: 100px; height: 50px; background: white; }`
	document, err := Compile(fmt.Sprintf(page, body), frame+"\n"+css)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		t.Logf("  warning: %s", warning.Message)
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, _, err := compiler.Compile(compose.Document{
		Size: image.Pt(100, 50), Root: document.Root,
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	found := map[display.Ink]image.Rectangle{}
	for y := 0; y < rendered.Height(); y++ {
		for x := 0; x < rendered.Width(); x++ {
			ink, _ := rendered.InkAt(x, y)
			if ink == display.InkWhite {
				continue
			}
			pixel := image.Rect(x, y, x+1, y+1)
			if existing, ok := found[ink]; ok {
				found[ink] = existing.Union(pixel)
			} else {
				found[ink] = pixel
			}
		}
	}
	return found
}

func expect(t *testing.T, got map[display.Ink]image.Rectangle, ink display.Ink, want image.Rectangle, what string) {
	t.Helper()
	if got[ink] != want {
		t.Errorf("%s: %v, want %v", what, got[ink], want)
	}
}

const twoBoxes = `<i class="a"></i><i class="b"></i>`
const inks = `.a { display: block; background: black; } .b { display: block; background: red; }`

func TestFlexBasisSplitsTheLine(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-basis: 30px; } .b { flex-basis: 70px; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 30, 50), "a at basis 30")
	expect(t, got, display.InkRed, image.Rect(30, 0, 100, 50), "b at basis 70")
}

func TestFlexGrowSharesWhatIsLeft(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-basis: 20px; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 20, 50), "a at basis 20")
	expect(t, got, display.InkRed, image.Rect(20, 0, 100, 50), "b takes the rest")
}

func TestGrowWeightsAreProportional(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-grow: 1; } .b { flex-grow: 3; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 25, 50), "a takes one quarter")
	expect(t, got, display.InkRed, image.Rect(25, 0, 100, 50), "b takes three quarters")
}

func TestGapSeparatesItems(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .page { gap: 10px; } .a { flex-grow: 1; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 45, 50), "a before the gap")
	expect(t, got, display.InkRed, image.Rect(55, 0, 100, 50), "b after the gap")
}

func TestColumnStacksDown(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .page { flex-direction: column; } .a { flex-basis: 20px; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 100, 20), "a at the top")
	expect(t, got, display.InkRed, image.Rect(0, 20, 100, 50), "b below it")
}

// An item that states its cross size keeps it under every alignment, and
// stretch then behaves as flex-start. That is what CSS says, and it is not
// something a container-wide alignment can express by itself.
func TestAlignItemsWithADefiniteCrossSize(t *testing.T) {
	for _, test := range []struct {
		value string
		want  image.Rectangle
	}{
		{"stretch", image.Rect(0, 0, 100, 10)},
		{"flex-start", image.Rect(0, 0, 100, 10)},
		{"center", image.Rect(0, 20, 100, 30)},
		{"flex-end", image.Rect(0, 40, 100, 50)},
	} {
		t.Run(test.value, func(t *testing.T) {
			got := boxes(t, `<i class="a"></i>`,
				`.a { display: block; background: black; flex-grow: 1; height: 10px; }`+
					` .page { align-items: `+test.value+`; }`)
			expect(t, got, display.InkBlack, test.want, "align-items: "+test.value)
		})
	}
}

// Only an item whose cross size is auto is stretched to fill.
func TestStretchFillsAnItemWithNoCrossSize(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 100, 50), "an auto-height item under stretch")
}

func TestPaddingInsetsTheContent(t *testing.T) {
	got := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { flex-grow: 1; padding: 5px 10px; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkRed, image.Rect(10, 5, 90, 45), "b inside a's padding")
}

func TestMarginAlongTheAxis(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-basis: 20px; margin-right: 10px; } .b { flex-grow: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 20, 50), "a at its basis")
	expect(t, got, display.InkRed, image.Rect(30, 0, 100, 50), "b after a's margin")
}

func TestMarginAcrossTheAxis(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; flex-grow: 1; margin: 8px 0; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 8, 100, 42), "a inset by its cross-axis margin")
}

func TestMarginAutoPushesOver(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { flex-basis: 20px; } .b { flex-basis: 30px; margin-left: auto; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 20, 50), "a stays at the start")
	expect(t, got, display.InkRed, image.Rect(70, 0, 100, 50), "b is pushed to the end")
}

func TestWidthAndHeight(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; background: black; width: 40px; height: 20px; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 40, 20), "an explicit size")
}

func TestPercentWidthOfABlockParent(t *testing.T) {
	got := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { display: block; flex-grow: 1; } .b { width: 25%; height: 50px; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 25, 50), "b at a quarter of its parent")
}

func TestDisplayNoneRemovesTheBox(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .a { display: none; } .b { flex-grow: 1; }`)
	if _, drawn := got[display.InkBlack]; drawn {
		t.Errorf("display:none still drew something at %v", got[display.InkBlack])
	}
	expect(t, got, display.InkRed, image.Rect(0, 0, 100, 50), "b takes the whole line")
}

func TestBorderSitsAtTheEdge(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; flex-grow: 1; border: 1px solid black; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 100, 50), "the border traces the box")
}

// A percentage is resolved against the container once the container has a
// size, which is the point of carrying lengths unresolved into the layout.
// Before that it worked only for a block child taking a share of its line.
func TestPercentagesResolveOnEitherAxisAndEitherParent(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want image.Rectangle
	}{
		{"width on a flex item", `.a { width: 50%; }`, image.Rect(0, 0, 50, 50)},
		{"flex-basis in percent", `.a { flex-basis: 25%; }`, image.Rect(0, 0, 25, 50)},
		// A flex item with no size along the line has none, so this one is
		// given a width before its percentage height can be seen at all.
		{"height on a flex item", `.a { flex-grow: 1; height: 40%; }`, image.Rect(0, 0, 100, 20)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, `<i class="a"></i>`,
				`.a { display: block; background: black; }`+test.css)
			expect(t, got, display.InkBlack, test.want, test.name)
		})
	}
}

// A hundred percent means fill, which is what an unconstrained child already
// does, so it resolves on either kind of parent.
func TestFullPercentMeansFill(t *testing.T) {
	got := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { display: block; flex-grow: 1; padding: 5px; } .b { width: 100%; height: 100%; }`)
	expect(t, got, display.InkRed, image.Rect(5, 5, 95, 45), "b filling a inside its padding")
}

// A percentage height on a block child divides the column the same way a
// percentage width divides the row.
func TestPercentHeightOfABlockParent(t *testing.T) {
	got := boxes(t, `<i class="a"><i class="b"></i></i>`,
		inks+` .a { display: block; flex-grow: 1; } .b { height: 40%; }`)
	expect(t, got, display.InkRed, image.Rect(0, 0, 100, 20), "b at two fifths of its parent")
}
