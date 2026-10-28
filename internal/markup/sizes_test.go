package markup

import (
	stdimage "image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// inkBounds is the smallest rectangle holding every black pixel.
func inkBounds(t *testing.T, body, css string) stdimage.Rectangle {
	t.Helper()
	frame, said := renderProbe(t, body, `.page { width: 200px; height: 40px; } `+css)
	if frame == nil {
		t.Fatal(said)
	}
	var bounds stdimage.Rectangle
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				bounds = bounds.Union(stdimage.Rect(x, y, x+1, y+1))
			}
		}
	}
	return bounds
}

func TestAFitContentBoxIsAsWideAsWhatItHolds(t *testing.T) {
	// The box is 20px of padding round nothing, so it is 20px wide wherever
	// it is, and the page around it is 200px.
	const box = `.a { background: black; padding: 5px 10px; } `
	tests := []struct {
		name, css string
		left      int
	}{
		{"in block flow", `.page { display: block; } .a { width: fit-content; }`, 0},
		{"centred by its margins", `.page { display: block; } .a { width: fit-content; margin: 0 auto; }`, 90},
		{"in a column that stretches", `.page { flex-direction: column; } .a { width: max-content; }`, 0},
		{"in a column that centres", `.page { flex-direction: column; align-items: center; } .a { width: fit-content; }`, 90},
		{"in a grid", `.page { display: grid; grid-template-columns: 1fr; } .a { width: fit-content; }`, 0},
		{"in a grid that centres", `.page { display: grid; grid-template-columns: 1fr; justify-items: center; } .a { width: fit-content; }`, 90},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bounds := inkBounds(t, `<div class="a"></div>`, box+test.css)
			if bounds.Dx() != 20 || bounds.Min.X != test.left {
				t.Errorf("the box is drawn at %v; want 20px wide from x=%d", bounds, test.left)
			}
		})
	}
	// Across a row, fit-content height is the content's, not the row's.
	bounds := inkBounds(t, `<div class="a"></div>`, box+`.a { height: fit-content; }`)
	if bounds.Dy() != 10 {
		t.Errorf("a fit-content box in a row is %dpx tall; want 10", bounds.Dy())
	}
	bounds = inkBounds(t, `<div class="a"></div>`, box+`.page { display: grid; grid-template-rows: 40px; } .a { height: fit-content; }`)
	if bounds.Dy() != 10 {
		t.Errorf("a fit-content box in a grid row is %dpx tall; want 10", bounds.Dy())
	}
	// Written as a length again, the width is that length.
	bounds = inkBounds(t, `<div class="a"></div>`, box+`.page { display: block; } .a { width: fit-content; width: 50px; }`)
	if bounds.Dx() != 70 {
		t.Errorf("a width written after fit-content left the box %dpx wide; want 70", bounds.Dx())
	}
}

func TestSizeKeywordsAreReadOrSaid(t *testing.T) {
	quiet := []string{
		`.a { max-width: none; max-height: none; }`,
		`.a { min-width: min-content; }`,
		`.a { gap: normal; row-gap: normal; column-gap: normal; }`,
		`.a { line-height: normal; }`,
		`.a { width: fit-content; }`,
	}
	for _, css := range quiet {
		if said := warningsFor(t, `<div class="a">x</div>`, css); said != "" {
			t.Errorf("%s said %q", css, said)
		}
	}
	said := map[string]string{
		`.a { width: max-content; }`:          "max-content is drawn as fit-content",
		`.a { width: min-content; }`:          "min-content is not supported",
		`.a { max-width: auto; }`:             "none leaves the box unbounded",
		`.a { max-width: fit-content; }`:      "is not supported",
		`.a { gap: auto; }`:                   "auto is not a gap",
		`.a { gap: 1px 2px 3px; }`:            "needs one or two lengths",
		`.a { padding-top: auto; }`:           "auto is not a padding",
		`.a { padding: auto; }`:               "auto is not a padding",
		`.a { border-width: auto; }`:          "auto is not a value border-width takes",
		`.a { font-size: auto; }`:             "auto is not a value font-size takes",
		`.a { border-top-style: groovy; }`:    "groovy is not a border style",
		`.a { border: 1px 2px solid black; }`: "more than one width, style or colour",
		`.a { border: 1ch solid black; }`:     "1ch must be given in px",
		`.a { width: abc; }`:                  "abc is not a length",
		`.a { margin: 1ch; }`:                 "1ch must be given in px",
	}
	for css, want := range said {
		got := warningsFor(t, `<div class="a">x</div>`, css)
		if !strings.Contains(got, want) {
			t.Errorf("%s said %q; want it to say %q", css, got, want)
		}
		if strings.Count(strings.TrimSpace(got), "\n") > 0 {
			t.Errorf("%s said it more than once: %q", css, got)
		}
	}
}

func TestABorderWidthMayBeAKeyword(t *testing.T) {
	for keyword, width := range map[string]int{"thin": 1, "medium": 3, "thick": 5} {
		bounds := inkBounds(t, `<div class="a"></div>`,
			`.a { width: 20px; height: 20px; border: `+keyword+` solid black; box-sizing: border-box; }`)
		frame, _ := renderProbe(t, `<div class="a"></div>`,
			`.page { width: 200px; height: 40px; } .a { width: 20px; height: 20px; border: `+keyword+` solid black; box-sizing: border-box; }`)
		drawn := 0
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if ink, _ := frame.InkAt(x, bounds.Min.Y+10); ink == display.InkBlack {
				drawn++
			}
		}
		if drawn != 2*width {
			t.Errorf("border: %s solid drew %d columns across the middle; want %d", keyword, drawn, 2*width)
		}
	}
}
