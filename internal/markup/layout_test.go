package markup

import (
	"fmt"
	"image"
	"testing"

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
	rendered := frameOf(t, document)
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

func TestFlexShorthandKeepsTheBasisAndShrinkFactor(t *testing.T) {
	got := boxes(t, twoBoxes, inks+` .page { width: 100px; }`+
		` .a { flex: 0 0 40px; } .b { flex: 1; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 40, 50), "a keeps the fixed basis")
	expect(t, got, display.InkRed, image.Rect(40, 0, 100, 50), "b takes the remaining space")

	got = boxes(t, twoBoxes, inks+` .page { width: 40px; }`+
		` .a { flex: 0 1 30px; } .b { flex: 0 1 30px; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 20, 50), "a shrinks by its weighted basis")
	expect(t, got, display.InkRed, image.Rect(20, 0, 40, 50), "b shrinks by its weighted basis")
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
		inks+` .a { display: flex; flex-grow: 1; padding: 5px 10px; } .b { flex-grow: 1; }`)
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

// A margin on a child of a block container has to be split along the axis the
// children actually run down, which for a block is always the page. Read off
// the container's flex direction instead, a block container answered "row",
// and a left margin became a gap above the box rather than beside it.
//
// The example that found it wrote "margin: 1px 0 0 2px" on a caption and got
// three pixels of space above it and none at the side.
func TestAMarginOnABlockChildGoesTheWayTheChildrenStack(t *testing.T) {
	got := boxes(t, `<div class="outer"><div class="mark"></div></div>`,
		`.outer { display: block; flex-grow: 1; }
		 .mark { width: 10px; height: 10px; background: red; margin: 4px 0 0 20px; }`)
	mark := got[display.InkRed]
	if mark.Min.X != 20 {
		t.Errorf("the left margin put the box at x=%d, want 20", mark.Min.X)
	}
	if mark.Min.Y != 4 {
		t.Errorf("the top margin put the box at y=%d, want 4", mark.Min.Y)
	}
}

// Taking an element out of the flow blockifies it, as it does in CSS. Left
// inline, an absolutely positioned span was read by its parent as a line of
// text: the box was never built, and its position, its size and its turn went
// with it. The strip down the side of examples/layout_showcase is exactly
// that element, and it came out as unturned text lying across the page.
func TestAnAbsolutelyPositionedInlineElementStillGetsItsBox(t *testing.T) {
	got := boxes(t, `<div class="frame"><span class="pin"></span></div>`,
		`.frame { display: block; flex-grow: 1; position: relative; }
		 .pin { position: absolute; top: 6px; left: 40px; width: 12px; height: 8px; background: red; }`)
	if pin := got[display.InkRed]; pin != image.Rect(40, 6, 52, 14) {
		t.Errorf("the pin is at %v, want (40,6)-(52,14)", pin)
	}
}

// An absolutely positioned child is placed against its container's padding
// box, so the container's padding does not push it inwards. Applying the
// padding around the anchored layer as well moved the badge in
// examples/layout_showcase by the whole of a padding-right that was there to
// keep the grid clear of it.
func TestPaddingDoesNotInsetAnAbsolutelyPositionedChild(t *testing.T) {
	got := boxes(t, `<div class="frame"><span class="pin"></span><b class="in"></b></div>`,
		`.frame { display: block; flex-grow: 1; position: relative; padding: 0 30px 0 10px; }
		 .pin { position: absolute; top: 0; right: 4px; width: 6px; height: 6px; background: red; }
		 .in { display: block; width: 6px; height: 6px; background: black; }`)
	// The page is 100 wide, so four from its right edge is x 90.
	if pin := got[display.InkRed]; pin.Min.X != 90 {
		t.Errorf("the pin starts at x=%d, want 90: the padding moved it", pin.Min.X)
	}
	// The child that is in the flow does get inset, which is what padding is.
	if in := got[display.InkBlack]; in.Min.X != 10 {
		t.Errorf("the ordinary child starts at x=%d, want 10", in.Min.X)
	}
}

// An auto-sized absolute child uses its content box when one edge is stated,
// as CSS does. Without this, right:5px made a badge stretch from the left edge
// of its containing block all the way to x=95.
func TestAnAbsolutelyPositionedAutoBoxShrinksToItsContent(t *testing.T) {
	got := boxes(t, `<div class="frame"><span class="pin">NEW</span></div>`,
		`.frame { display: block; flex-grow: 1; position: relative; }
		 .pin { position: absolute; top: 5px; right: 5px; background: red;
			 font-family: monaco; font-size: 10px; white-space: nowrap; }`)
	if pin := got[display.InkRed]; pin != image.Rect(77, 5, 95, 19) {
		t.Errorf("the auto-sized badge is at %v, want (77,5)-(95,19)", pin)
	}
}

// grid-column: 1 / span 3 and grid-column: 1 / 4 say the same thing, and an
// author writes whichever they were thinking in — where the cell ends, or how
// many tracks it covers. Only the first was read.
func TestAGridCellMaySayItsSpanAfterTheSlash(t *testing.T) {
	const sheet = `.grid { display: grid; flex-grow: 1;
		   grid-template-columns: 20px 20px 20px; grid-template-rows: 10px; }
		 .wide { background: red; }`
	spanned := boxes(t, `<div class="grid"><div class="wide"></div></div>`,
		sheet+` .wide { grid-column: 1 / span 3; }`)
	counted := boxes(t, `<div class="grid"><div class="wide"></div></div>`,
		sheet+` .wide { grid-column: 1 / 4; }`)
	if spanned[display.InkRed] != counted[display.InkRed] {
		t.Errorf("span 3 covered %v, 1 / 4 covered %v", spanned[display.InkRed], counted[display.InkRed])
	}
	if width := spanned[display.InkRed].Dx(); width != 60 {
		t.Errorf("the cell is %d wide, want the 60 of three tracks", width)
	}
}

// The box model CSS has: a border takes room, a stated width is the content's
// unless box-sizing says otherwise, and an absolutely positioned child is
// placed against the padding box — inside the border, outside the padding.
func TestTheBoxModelIsTheOneCSSHas(t *testing.T) {
	t.Run("content-box grows the box by its padding and border", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`,
			`.a { display: block; width: 20px; height: 10px; padding: 4px;
			      border: 3px solid black; background: red; }`)
		// 20 + 4 + 4 + 3 + 3 across, 10 + 14 down.
		if box := got[display.InkBlack]; box.Dx() != 34 || box.Dy() != 24 {
			t.Errorf("the border box is %dx%d, want 34x24", box.Dx(), box.Dy())
		}
	})

	t.Run("border-box states the box itself", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`,
			`.a { display: block; box-sizing: border-box; width: 34px; height: 24px;
			      padding: 4px; border: 3px solid black; background: red; }`)
		if box := got[display.InkBlack]; box.Dx() != 34 || box.Dy() != 24 {
			t.Errorf("the border box is %dx%d, want 34x24", box.Dx(), box.Dy())
		}
	})

	t.Run("a border takes room from the content", func(t *testing.T) {
		got := boxes(t, `<i class="a"><i class="in"></i></i>`,
			`.a { display: block; box-sizing: border-box; width: 40px; height: 40px;
			      border: 5px solid black; }
			 .in { display: block; width: 4px; height: 4px; background: red; }`)
		if in := got[display.InkRed]; in.Min.X != 5 || in.Min.Y != 5 {
			t.Errorf("the content starts at %v, want (5,5) inside the border", in.Min)
		}
	})

	t.Run("an absolute child is placed against the padding box", func(t *testing.T) {
		got := boxes(t, `<i class="host"><i class="pin"></i></i>`,
			`.host { position: relative; display: block; box-sizing: border-box;
			         width: 60px; height: 40px; border: 10px solid black; padding: 7px; }
			 .pin { position: absolute; top: 0; left: 0; width: 6px; height: 6px; background: red; }`)
		// Inside the border, outside the padding.
		if pin := got[display.InkRed]; pin.Min != image.Pt(10, 10) {
			t.Errorf("the pin is at %v, want (10,10): inside the border, not inset by the padding", pin.Min)
		}
	})
}

// The box model applies to every property that states a size, not only width
// and height, and to the awkward values as well as the ordinary ones.
func TestBoxSizingReachesEverySizingProperty(t *testing.T) {
	const frame = `.a { display: block; background: red; padding: 4px; border: 3px solid black; }`

	t.Run("min-width grows by the edges under content-box", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`, frame+` .a { min-width: 20px; height: 10px; }`)
		// 20 of content, 4 and 4 of padding, 3 and 3 of border.
		if box := got[display.InkBlack]; box.Dx() != 34 {
			t.Errorf("the box is %d wide, want 34", box.Dx())
		}
	})

	t.Run("max-height is the content's under content-box", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`,
			frame+` .a { width: 20px; height: 40px; max-height: 10px; }`)
		if box := got[display.InkBlack]; box.Dy() != 24 {
			t.Errorf("the box is %d tall, want 24", box.Dy())
		}
	})

	t.Run("flex-basis is sized the way a width is", func(t *testing.T) {
		content := boxes(t, `<i class="a"></i><i class="rest"></i>`,
			frame+` .a { flex-basis: 20px; } .rest { display: block; flex-grow: 1; background: white; }`)
		border := boxes(t, `<i class="a"></i><i class="rest"></i>`,
			frame+` .a { flex-basis: 34px; box-sizing: border-box; }
			 .rest { display: block; flex-grow: 1; background: white; }`)
		if content[display.InkBlack].Dx() != border[display.InkBlack].Dx() {
			t.Errorf("flex-basis 20 content-box covered %d and 34 border-box covered %d; they are the same box",
				content[display.InkBlack].Dx(), border[display.InkBlack].Dx())
		}
	})

	t.Run("a percentage width takes the edges on top of it", func(t *testing.T) {
		// Half of the hundred pixel page, and the edges outside that.
		got := boxes(t, `<i class="a"></i>`, frame+` .a { width: 50%; height: 10px; }`)
		if box := got[display.InkBlack]; box.Dx() != 64 {
			t.Errorf("the box is %d wide, want 50 of content and 14 of edges", box.Dx())
		}
	})

	t.Run("a zero width still draws its edges", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`, frame+` .a { width: 0; height: 10px; }`)
		if box := got[display.InkBlack]; box.Dx() != 14 {
			t.Errorf("the box is %d wide, want the 14 its padding and border take", box.Dx())
		}
	})

	t.Run("border-box smaller than its own edges does not invert", func(t *testing.T) {
		got := boxes(t, `<i class="a"></i>`,
			frame+` .a { box-sizing: border-box; width: 8px; height: 8px; }`)
		if box := got[display.InkBlack]; box.Dx() != 8 || box.Dy() != 8 {
			t.Errorf("the box is %dx%d, want the 8x8 it asked for", box.Dx(), box.Dy())
		}
	})
}

// An absolutely positioned child resolves against the padding box of the
// nearest positioned ancestor, however many unpositioned boxes are between
// them and whatever padding those carry.
func TestTheContainingBlockIsTheNearestPositionedPaddingBox(t *testing.T) {
	got := boxes(t, `<i class="host"><i class="mid"><i class="pin"></i></i></i>`,
		`.host { position: relative; display: block; box-sizing: border-box;
		         margin: 6px; width: 80px; height: 40px;
		         border: 5px solid black; padding: 9px; }
		 .mid { display: block; padding: 4px; }
		 .pin { position: absolute; top: 0; left: 0; width: 6px; height: 6px; background: red; }`)
	// The host's border box starts at the margin, 6; its padding box starts a
	// border in from that, 11. Neither its own padding nor the wrapper's
	// moves a placed child.
	if pin := got[display.InkRed]; pin.Min != image.Pt(11, 11) {
		t.Errorf("the pin is at %v, want (11,11)", pin.Min)
	}
}
