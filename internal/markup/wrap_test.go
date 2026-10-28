package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestItemsThatDoNotFitWrapOntoTheNextLine(t *testing.T) {
	body := `<i class="a"></i><i class="b"></i><i class="c"></i>`
	const items = `.page { flex-wrap: wrap; }
		.a { width: 40px; height: 10px; background: black; }
		.b { width: 40px; height: 10px; background: red; }
		.c { width: 40px; height: 10px; background: yellow; } `
	tests := []struct {
		name, css          string
		black, red, yellow image.Rectangle
	}{
		{"lines stretch into the height", `.page { align-items: flex-start; }`,
			image.Rect(0, 0, 40, 10), image.Rect(40, 0, 80, 10), image.Rect(0, 25, 40, 35)},
		{"and so do the items in them", `.a, .b, .c { height: auto; }`,
			image.Rect(0, 0, 40, 25), image.Rect(40, 0, 80, 25), image.Rect(0, 25, 40, 50)},
		{"packed at the start", `.page { align-content: flex-start; }`,
			image.Rect(0, 0, 40, 10), image.Rect(40, 0, 80, 10), image.Rect(0, 10, 40, 20)},
		{"a row gap between lines", `.page { align-content: flex-start; row-gap: 5px; }`,
			image.Rect(0, 0, 40, 10), image.Rect(40, 0, 80, 10), image.Rect(0, 15, 40, 25)},
		{"a column gap along them", `.page { align-content: flex-start; column-gap: 15px; }`,
			image.Rect(0, 0, 40, 10), image.Rect(55, 0, 95, 10), image.Rect(0, 10, 40, 20)},
		{"a gap too wide for two", `.page { align-content: flex-start; column-gap: 25px; }`,
			image.Rect(0, 0, 40, 10), image.Rect(0, 10, 40, 20), image.Rect(0, 20, 40, 30)},
		{"each line justified on its own", `.page { align-content: flex-start; justify-content: center; }`,
			image.Rect(10, 0, 50, 10), image.Rect(50, 0, 90, 10), image.Rect(30, 10, 70, 20)},
		{"lines centred", `.page { align-content: center; }`,
			image.Rect(0, 15, 40, 25), image.Rect(40, 15, 80, 25), image.Rect(0, 25, 40, 35)},
		{"lines spaced", `.page { align-content: space-between; }`,
			image.Rect(0, 0, 40, 10), image.Rect(40, 0, 80, 10), image.Rect(0, 40, 40, 50)},
		{"a column wraps across", `.page { flex-direction: column; align-items: flex-start; align-content: flex-start; column-gap: 5px; }
			.a, .b, .c { height: 20px; width: 30px; }`,
			image.Rect(0, 0, 30, 20), image.Rect(0, 20, 30, 40), image.Rect(35, 0, 65, 20)},
		{"what does not divide goes to the first lines", `.page { align-items: flex-start; column-gap: 25px; }`,
			image.Rect(0, 0, 40, 10), image.Rect(0, 17, 40, 27), image.Rect(0, 34, 40, 44)},
		{"the flex-flow shorthand", `.page { flex-flow: column wrap; align-items: flex-start; align-content: flex-start; }
			.a, .b, .c { height: 20px; width: 30px; }`,
			image.Rect(0, 0, 30, 20), image.Rect(0, 20, 30, 40), image.Rect(30, 0, 60, 20)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, body, items+test.css)
			expect(t, got, display.InkBlack, test.black, "the first item")
			expect(t, got, display.InkRed, test.red, "the second item")
			expect(t, got, display.InkYellow, test.yellow, "the third item")
		})
	}
}

func TestEachWrappedLineIsFlexedOnItsOwn(t *testing.T) {
	// Three of four 30px bases fit in 100px, and grow into the 10 left over;
	// the fourth has a line to itself and grows to fill it.
	body := `<i class="a"></i><i class="a"></i><i class="b"></i><i class="c"></i>`
	got := boxes(t, body, `.page { flex-wrap: wrap; align-content: flex-start; }
		.a, .b, .c { flex: 1 1 30px; height: 10px; } .a { background: black; } .b { background: red; } .c { background: yellow; }`)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 67, 10), "the first two")
	expect(t, got, display.InkRed, image.Rect(67, 0, 100, 10), "the third")
	expect(t, got, display.InkYellow, image.Rect(0, 10, 100, 20), "the fourth")

	// Items with no basis wrap where what they hold stops fitting side by
	// side, which for these is 60px each in a 100px row.
	held := `<i class="f"><b class="x"></b></i><i class="f"><b class="y"></b></i>`
	const contents = `.page { flex-wrap: wrap; align-content: flex-start; align-items: flex-start; }
		.f { flex: 1; display: block; } .x, .y { display: block; width: 60px; height: 10px; }
		.x { background: black; } .y { background: red; } `
	got = boxes(t, held, contents)
	expect(t, got, display.InkBlack, image.Rect(0, 0, 60, 10), "the first item's content")
	expect(t, got, display.InkRed, image.Rect(0, 10, 60, 20), "the second item's content")
	// A width the item states caps how far its content holds it up, so these
	// fit side by side and share the row.
	got = boxes(t, held, contents+`.f { width: 30px; }`)
	expect(t, got, display.InkRed, image.Rect(50, 0, 100, 10), "the second item's content, beside the first")
}

func TestAWrappedRowIsAsTallAsItsLines(t *testing.T) {
	for width, lines := range map[string]int{"100px": 2, "50px": 3} {
		got := boxes(t, `<div class="w"><i class="a"></i><i class="a"></i><i class="a"></i></div><i class="after"></i>`,
			`.page { flex-direction: column; align-items: flex-start; }
			.w { display: flex; flex-wrap: wrap; width: `+width+`; row-gap: 4px; }
			.a { width: 40px; height: 10px; background: black; }
			.after { width: 10px; height: 5px; background: red; }`)
		height := lines*10 + (lines-1)*4
		expect(t, got, display.InkRed, image.Rect(0, height, 10, height+5), "the box after a "+width+" wrapped row")
	}
	// flex-flow sets both, and resets the one it leaves out.
	body := `<div class="page"><i>a</i><i>b</i></div>`
	shorthand, err := Compile(body, `.page { display: flex; width: 50px; height: 20px; flex-direction: column; flex-flow: wrap; }`)
	if err != nil {
		t.Fatal(err)
	}
	longhand, err := Compile(body, `.page { display: flex; width: 50px; height: 20px; flex-wrap: wrap; }`)
	if err != nil {
		t.Fatal(err)
	}
	if string(shorthand.JSON) != string(longhand.JSON) {
		t.Errorf("flex-flow: wrap left the direction a column:\n%s", shorthand.JSON)
	}
}

func TestWrappingSaysWhatItCannotDo(t *testing.T) {
	for css, want := range map[string]string{
		`.page { flex-wrap: wrap-reverse; }`:    "wrap-reverse is drawn as wrap",
		`.page { flex-wrap: sometimes; }`:       "is not one of nowrap, wrap",
		`.page { flex-flow: row wrap column; }`: "takes a direction and a wrap",
		`.page { flex-flow: diagonal; }`:        "neither a direction nor a wrap",
	} {
		if said := warningsFor(t, `<i>x</i>`, css); !strings.Contains(said, want) {
			t.Errorf("%s said %q; want %q", css, said, want)
		}
	}
	// An item too wide for any line has one to itself, and running past it
	// is said.
	_, said := renderProbe(t, `<i class="a"></i><i class="b"></i>`,
		`.page { flex-wrap: wrap; } .a { width: 80px; height: 5px; flex-shrink: 0; } .b { width: 10px; height: 5px; }`)
	if !strings.Contains(said, "only 60 are available") {
		t.Errorf("an item wider than the row said %q", said)
	}
	// So is running past the box across the lines.
	_, said = renderProbe(t, `<i class="a"></i><i class="a"></i><i class="a"></i>`,
		`.page { flex-wrap: wrap; } .a { width: 40px; height: 15px; flex-shrink: 0; }`)
	if !strings.Contains(said, "wrapped lines need 45 pixels across, only 40 are available") {
		t.Errorf("lines taller than the box said %q", said)
	}
}
