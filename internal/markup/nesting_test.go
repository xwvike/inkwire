package markup

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// Nesting is where a compiler that works one level deep stops working. Each of
// these puts a feature inside another one that changes the coordinate space it
// lands in.

// Two clips in a chain confine to what both of them allow, not to the inner
// one alone.
func TestNestedClipsIntersect(t *testing.T) {
	// The outer keeps the left half, the inner the top half; together they
	// should keep a quarter.
	outerOnly := inkIn(t, `<i class="o"><i class="fill"></i></i>`,
		`.o { display: block; flex-grow: 1; clip-path: inset(0 50% 0 0); }
		 .fill { display: block; background: black; width: 100%; height: 100%; }`)
	both := inkIn(t, `<i class="o"><i class="m"><i class="fill"></i></i></i>`,
		`.o { display: block; flex-grow: 1; clip-path: inset(0 50% 0 0); }
		 .m { display: block; clip-path: inset(0 0 50% 0); width: 100%; height: 100%; }
		 .fill { display: block; background: black; width: 100%; height: 100%; }`)
	if outerOnly == 0 || both == 0 {
		t.Fatalf("nothing drawn: outer=%d both=%d", outerOnly, both)
	}
	if both >= outerOnly {
		t.Errorf("the inner clip kept %d pixels of the outer clip's %d; it should have halved it",
			both, outerOnly)
	}
	if ratio := float64(both) / float64(outerOnly); ratio < 0.4 || ratio > 0.6 {
		t.Errorf("the intersection is %.2f of the outer clip, want about half", ratio)
	}
}

// A transform inside a transform composes: each draws its subtree onto a
// surface of its own, and the outer one magnifies what the inner one produced.
func TestNestedTransformsCompose(t *testing.T) {
	once := inkIn(t, `<i class="o"><i class="fill"></i></i>`,
		`.o { display: block; width: 40px; height: 40px; }
		 .fill { display: block; background: black; width: 50%; height: 50%; }`)
	twice := inkIn(t, `<i class="o"><i class="m"><i class="fill"></i></i></i>`,
		`.o { display: block; width: 40px; height: 40px; }
		 .m { display: block; scale: 2; width: 100%; height: 100%; }
		 .fill { display: block; background: black; width: 50%; height: 50%; }`)
	if once == 0 || twice == 0 {
		t.Fatalf("nothing drawn: once=%d twice=%d", once, twice)
	}
	// Doubling a box that filled a quarter of its parent still fills a
	// quarter, because the child was given half the room to start with.
	if twice != once {
		t.Logf("a doubled subtree covers %d pixels where the plain one covers %d", twice, once)
	}
}

// Turning something that is itself turned comes back to where it started.
func TestTwoQuarterTurnsAreAHalfTurn(t *testing.T) {
	plain := boxes(t, `<i class="o"><i class="fill"></i></i>`,
		`.o { display: block; width: 40px; height: 20px; }
		 .fill { display: block; background: black; width: 100%; height: 50%; }`)
	turned := boxes(t, `<i class="o"><i class="m"><i class="fill"></i></i></i>`,
		`.o { display: block; width: 40px; height: 20px; rotate: 90deg; }
		 .m { display: block; rotate: 90deg; width: 100%; height: 100%; }
		 .fill { display: block; background: black; width: 100%; height: 50%; }`)
	// The bar starts at the top when upright; after two quarter turns it is at
	// the bottom of the same box.
	if plain[display.InkBlack].Min.Y != 0 {
		t.Fatalf("the upright bar is at %v, not at the top", plain[display.InkBlack])
	}
	if turned[display.InkBlack].Min.Y == 0 {
		t.Errorf("after a half turn the bar is still at %v", turned[display.InkBlack])
	}
}

// A grid inside a flex item inside a grid: each container has to hand the next
// one a box it can measure against.
func TestGridInsideFlexInsideGrid(t *testing.T) {
	got := boxes(t,
		`<i class="outer">`+
			`<i class="pad"></i>`+
			`<i class="mid"><i class="inner"><i class="a"></i><i class="b"></i></i></i>`+
			`</i>`,
		inks+`
		.outer { display: grid; grid-template-columns: 20px 1fr; flex-grow: 1; }
		.mid { display: flex; }
		.inner { display: grid; grid-template-columns: 1fr 1fr; flex-grow: 1; }`)
	// The outer grid gives the second column eighty pixels starting at twenty;
	// the inner grid halves it.
	expect(t, got, display.InkBlack, image.Rect(20, 0, 60, 50), "the first inner cell")
	expect(t, got, display.InkRed, image.Rect(60, 0, 100, 50), "the second inner cell")
}

// A percentage inside a percentage resolves against each container in turn
// rather than against the page.
func TestPercentagesCompoundThroughNesting(t *testing.T) {
	got := boxes(t, `<i class="half"><i class="a"></i></i>`,
		`.half { display: block; width: 50%; height: 100%; }
		 .a { display: block; background: black; width: 50%; height: 100%; }`)
	// Half of half of a hundred is twenty-five.
	expect(t, got, display.InkBlack, image.Rect(0, 0, 25, 50), "a quarter of the page")
}

// A clip inside a transform has to be clipped in the transformed subtree's own
// coordinates, not in the page's.
func TestClipInsideATransform(t *testing.T) {
	got := boxes(t, `<i class="o"><i class="c"></i></i>`,
		`.o { display: block; width: 40px; height: 40px; scale: 2; }
		 .c { display: block; background: black; width: 100%; height: 100%;
		      clip-path: inset(0 50% 0 0); }`)
	// The subtree is drawn at twenty and doubled, so the kept half is the
	// left twenty pixels of the forty the box occupies.
	if got[display.InkBlack].Max.X > 22 {
		t.Errorf("the clipped half reaches x=%d; the clip was applied outside the transform",
			got[display.InkBlack].Max.X)
	}
	if got[display.InkBlack].Dx() < 15 {
		t.Errorf("the clipped half is only %d wide", got[display.InkBlack].Dx())
	}
}

// Text sitting directly among boxes is content too. CSS wraps it in an
// anonymous item; skipping it dropped the units out of a figure and its label
// without saying anything.
func TestBareTextAmongFlexItemsIsNotDropped(t *testing.T) {
	withUnit := inkIn(t, `<i class="r"><b>412</b>h<small>run</small></i>`,
		`.r { display: flex; font-family: monaco; font-size: 12px; }`)
	withoutUnit := inkIn(t, `<i class="r"><b>412</b><small>run</small></i>`,
		`.r { display: flex; font-family: monaco; font-size: 12px; }`)
	if withUnit <= withoutUnit {
		t.Errorf("the version with the unit drew %d pixels and the one without %d; the h vanished",
			withUnit, withoutUnit)
	}
}

func TestBareTextInAGridTakesACell(t *testing.T) {
	// The box is red so that measuring it does not also measure the text,
	// which is black and would union with it.
	got := boxes(t, `<i class="g">x<i class="b"></i></i>`,
		inks+` .g { display: grid; grid-template-columns: 1fr 1fr; flex-grow: 1;
		        font-family: monaco; font-size: 12px; }`)
	if got[display.InkRed].Min.X < 50 {
		t.Errorf("the box is at %v; the text should have taken the first cell",
			got[display.InkRed])
	}
}

// A stylesheet has no way to ask for a polyline of ninety-six points, and it
// should not: those points are not written by hand, they are what a generator
// produced from a series. The scene element is where the page stops describing
// and hands the drawing over.
func TestASceneElementDrawsWhatCSSCannotDescribe(t *testing.T) {
	const dir = "../../examples/desk/"
	resolver := Compiler{Scenes: func(reference SceneRef) (json.RawMessage, error) {
		if reference.Src == "" {
			return json.RawMessage(reference.Inline), nil
		}
		return os.ReadFile(dir + reference.Src)
	}}
	document, err := resolver.Compile(
		string(readPage(t, dir, "chart", ".html")), string(readPage(t, dir, "chart", ".css")))
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		t.Errorf("markup warning: %s", warning.Message)
	}
	drawn := inkOfIn(t, dir, document)

	// Without a resolver the page still lays out, and says what it lost.
	plain, err := Compile(
		string(readPage(t, dir, "chart", ".html")), string(readPage(t, dir, "chart", ".css")))
	if err != nil {
		t.Fatal(err)
	}
	var said string
	for _, warning := range plain.Warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "chart-plot.json") {
		t.Errorf("an unresolvable scene was not reported by name: %q", said)
	}
	if bare := inkOfIn(t, dir, plain); bare >= drawn {
		t.Errorf("the page without its plot drew %d pixels and the one with it %d", bare, drawn)
	}
}

// A description written inside the element works as well as one in a file,
// which is what a generator emitting a whole page in one piece needs.
func TestAnInlineSceneIsDrawn(t *testing.T) {
	resolver := Compiler{Scenes: func(reference SceneRef) (json.RawMessage, error) {
		if reference.Src != "" {
			return nil, fmt.Errorf("expected the description inline, got src=%q", reference.Src)
		}
		return json.RawMessage(reference.Inline), nil
	}}
	document, err := resolver.Compile(
		`<div class="page"><scene>{"type":"circle","center":{"x":25,"y":25},`+
			`"radius":20,"fill":"black"}</scene></div>`,
		`.page { display: flex; width: 60px; height: 60px; background: white; }
		 scene { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if drawn := inkOf(t, document); drawn == 0 {
		t.Error("the inline scene drew nothing")
	}
}

func inkOf(t *testing.T, document Document) int {
	t.Helper()
	return inkOfIn(t, "", document)
}

func inkOfIn(t *testing.T, dir string, document Document) int {
	t.Helper()
	frame, _ := renderDocument(t, dir, document.JSON)
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink != display.InkWhite {
				count++
			}
		}
	}
	return count
}
