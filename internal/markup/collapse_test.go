package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestMarginsThatMeetCollapse(t *testing.T) {
	// .blk is a flex item, so it keeps its children's margins inside it and
	// each case starts at its top.
	const frame = `.page { flex-direction: column; align-items: flex-start; } .blk { width: 60px; }
		.a { height: 10px; background: black; } .b { height: 10px; background: red; } `
	tests := []struct {
		name, body, css string
		black, red      image.Rectangle
	}{
		{"siblings take the larger", `<div class="a"></div><div class="b"></div>`,
			`.a { margin-bottom: 8px; } .b { margin-top: 5px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 18, 60, 28)},
		{"whichever is larger", `<div class="a"></div><div class="b"></div>`,
			`.a { margin-bottom: 3px; } .b { margin-top: 9px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 19, 60, 29)},
		{"a negative one is taken off", `<div class="a"></div><div class="b"></div>`,
			`.a { margin-bottom: 8px; } .b { margin-top: -3px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 15, 60, 25)},
		{"two negative ones are the more negative", `<div class="a"></div><div class="b"></div>`,
			`.a { margin-bottom: -3px; height: 20px; } .b { margin-top: -5px; }`,
			image.Rect(0, 0, 60, 15), image.Rect(0, 15, 60, 25)},
		{"a first child's goes outside its parent", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(30, 10, 60, 20)},
		{"and joins the parent's own", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; margin-top: 4px; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(30, 10, 60, 20)},
		{"a padding keeps it in", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; padding-top: 1px; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 11, 30, 21), image.Rect(0, 0, 60, 21)},
		{"so does a border", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; border-top: 1px solid red; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 11, 30, 21), image.Rect(0, 0, 60, 21)},
		{"and overflow: hidden", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; overflow: hidden; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(0, 0, 60, 20)},
		{"through two parents", `<div class="b"><div><div class="a"></div></div></div>`,
			`.b { height: auto; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(30, 10, 60, 20)},
		{"a last child's goes after its parent", `<div class="w"><div class="a"></div></div><div class="b"></div>`,
			`.a { margin-bottom: 10px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 20, 60, 30)},
		{"or has a padding below", `<div class="w"><div class="a"></div></div><div class="b"></div>`,
			`.w { padding-bottom: 1px; } .a { margin-bottom: 10px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 25, 60, 35)},
		{"or a least height", `<div class="w"><div class="a"></div></div><div class="b"></div>`,
			`.w { min-height: 5px; } .a { margin-bottom: 10px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 24, 60, 34)},
		{"a positioned box keeps it in", `<div class="b"><div class="a"></div></div>`,
			`.blk { position: relative; height: 40px; } .b { position: absolute; top: 0; left: 0; width: 60px; height: auto; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(0, 0, 60, 20)},
		{"so does an inline-block", `<div class="b"><div class="a"></div></div>`,
			`.b { display: inline-block; width: 60px; height: auto; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 10, 30, 20), image.Rect(0, 0, 60, 20)},
		{"through an empty box between", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
		{"through two", `<div class="a"></div><div class="e"></div><div class="e f"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin-top: 3px; } .f { margin-bottom: 9px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 19, 60, 29)},
		{"not through a box with a height", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; height: 1px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 23, 60, 33)},
		{"nor one with text in it", `<div class="a"></div><div class="e">x</div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; line-height: 5px; font-size: 12px; color: yellow; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 27, 60, 37)},
		{"nor one with a padding", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; padding-top: 1px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 23, 60, 33)},
		{"an empty box first", `<div class="e"></div><div class="a"></div><div class="b"></div>`,
			`.e { margin: 3px 0 7px; } .a { margin-top: 2px; }`,
			image.Rect(0, 7, 60, 17), image.Rect(0, 17, 60, 27)},
		{"an empty box last", `<div class="w"><div class="a"></div><div class="e"></div></div><div class="b"></div>`,
			`.a { margin-bottom: 2px; } .e { margin: 9px 0 3px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 19, 60, 29)},
		{"what empties out leaves the gap open", `<div class="a"></div><div class="e"><i class="x"></i></div><div class="b"></div>`,
			`.blk { position: relative; } .x { position: absolute; } .a { margin-bottom: 4px; } .e { margin: 6px 0; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
		{"a padding at the sides does not", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; padding: 0 2px; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(2, 10, 32, 20), image.Rect(0, 10, 60, 20)},
		{"a box with only an empty box in it", `<div class="a"></div><div class="w"><div class="e"></div></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin-top: 6px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
		{"out of a box with a height that holds only empty boxes", `<div class="a"></div><div class="w"><div class="e"></div></div><div class="b"></div>`,
			`.w { height: 5px; } .e { margin-top: 6px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 21, 60, 31)},
		{"or out of the bottom if the top is closed", `<div class="a"></div><div class="w"><div class="e"></div></div><div class="b"></div>`,
			`.w { padding-top: 1px; } .e { margin-top: 6px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 17, 60, 27)},
		{"a parent's percentage margin keeps its child's in", `<div class="b"><div class="a"></div></div>`,
			`.b { height: auto; margin-top: 10%; } .a { margin-top: 10px; width: 30px; }`,
			image.Rect(0, 16, 30, 26), image.Rect(0, 6, 60, 26)},
		{"but not a percentage", `<div class="a"></div><div class="w"><div class="e"></div></div><div class="b"></div>`,
			`.e { margin-top: 10%; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
		{"not through a box with a least height", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; min-height: 1px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 23, 60, 33)},
		{"nor one laid out on its own", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; overflow: hidden; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 22, 60, 32)},
		{"nor one with a padding below", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; padding-bottom: 1px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 23, 60, 33)},
		{"but a padding at its sides is nothing", `<div class="a"></div><div class="e"></div><div class="b"></div>`,
			`.a { margin-bottom: 4px; } .e { margin: 6px 0; padding: 0 3px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
		{"unless the parent states its height", `<div class="w"><div class="a"></div></div><div class="b"></div>`,
			`.w { height: 12px; } .a { margin-bottom: 10px; } .b { margin-top: 4px; }`,
			image.Rect(0, 0, 60, 10), image.Rect(0, 16, 60, 26)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, `<div class="blk">`+test.body+`</div>`, frame+test.css)
			expect(t, got, display.InkBlack, test.black, "the black box")
			expect(t, got, display.InkRed, test.red, "the red box")
		})
	}
}

func TestThePageKeepsItsChildrensMargins(t *testing.T) {
	got := boxes(t, `<div class="a"></div>`,
		`.page { display: block; } .a { width: 10px; height: 10px; margin-top: 10px; background: black; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 10, 10, 20), "the page's first child")
}

func TestMarginsInALineDoNotCollapse(t *testing.T) {
	// Flex items keep both margins, and a flex item keeps its children's
	// margins inside it, as in a browser.
	got := boxes(t, `<div class="a"></div><div class="b"><div class="c"></div></div>`,
		`.page { flex-direction: column; align-items: flex-start; }
		.a { width: 10px; height: 10px; margin-bottom: 8px; background: black; }
		.b { width: 10px; margin-top: 5px; background: red; }
		.c { height: 10px; margin-top: 3px; }`)
	expect(t, got, display.InkRed, image.Rect(0, 23, 10, 36), "the flex item after the first")
}

func TestAPercentageMarginIsNotCollapsedAndSaysSo(t *testing.T) {
	said := warningsFor(t, `<div class="blk"><div class="a"></div><div class="b"></div></div>`,
		`.blk { width: 60px; } .a { height: 5px; margin-bottom: 10%; } .b { height: 5px; margin-top: 4px; }`)
	if !strings.Contains(said, "not when one is a percentage") {
		t.Errorf("a percentage margin beside another said %q", said)
	}
	said = warningsFor(t, `<div class="blk"><div class="a"></div><div class="e"></div><div class="b"></div></div>`,
		`.blk { width: 60px; } .a { height: 5px; margin-bottom: 4px; } .e { margin-top: 10%; } .b { height: 5px; }`)
	if !strings.Contains(said, "not when one is a percentage") {
		t.Errorf("a percentage margin on an empty box said %q", said)
	}
	if said := warningsFor(t, `<div class="blk"><div class="w"><div class="e"></div></div></div>`,
		`.blk { width: 60px; } .e { margin-top: 10%; }`); said != "" {
		t.Errorf("a percentage margin meeting no other said %q", said)
	}
	if said := warningsFor(t, `<div class="blk"><div class="a"></div><div class="b"></div></div>`,
		`.blk { width: 60px; } .a { height: 5px; margin-bottom: 10%; } .b { height: 5px; }`); said != "" {
		t.Errorf("a percentage margin beside none said %q", said)
	}
}
