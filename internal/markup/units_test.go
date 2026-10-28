package markup

import (
	"bytes"
	stdimage "image"
	"strings"
	"testing"
)

func TestRelativeUnitsAreWrittenAsThePixelsTheyComeTo(t *testing.T) {
	units := relativeUnits{em: 16, rem: 12, viewport: stdimage.Pt(400, 300)}
	tests := map[string]string{
		"2em":                          "32px",
		"1.5rem":                       "18px",
		".5em":                         "8px",
		"-1em":                         "-16px",
		"+1em":                         "16px",
		"1EM":                          "16px",
		"50vw 10vh":                    "200px 30px",
		"10vmin 10vmax":                "30px 40px",
		"12pt":                         "16px",
		"1in 2.54cm 25.4mm 6pc 101.6q": "96px 96px 96px 96px 96px",
		"calc(100% - 1em)":             "calc(100% - 16px)",
		"0 1em 2px":                    "0 16px 2px",
		"1px solid red":                "1px solid red",
		"h1em":                         "h1em",
		"#1em":                         "#1em",
		"1emx":                         "1emx",
		"2ch":                          "2ch",
		`"Font 2em", serif`:            `"Font 2em", serif`,
		"url(a1em.png)":                "url(a1em.png)",
		"url(2em.png) 1em":             "url(2em.png) 16px",
		"0.29em":                       "4.64px",
	}
	for value, want := range tests {
		got, ok := units.resolve(value, nil)
		if !ok || got != want {
			t.Errorf("%q came to %q (%v); want %q", value, got, ok, want)
		}
	}
	// A whole-pixel property cuts what it is given down to a pixel, so a share
	// that floating point lands just under one must not lose it.
	if got, _ := (relativeUnits{viewport: stdimage.Pt(100, 100)}).resolve("29vw", nil); got != "29px" {
		t.Errorf("29vw of 100px came to %s", got)
	}
}

// Each pair draws the same page: one written in the units, the other in the
// pixels they come to.
func TestRelativeUnitsMeasureWhatCSSMeasures(t *testing.T) {
	const page = `.page { width: 200px; height: 80px; font-size: 16px; } `
	tests := []struct {
		name, relative, pixels string
	}{
		{"em is the element's own font size",
			`.a { width: 2em; font-size: 24px; }`, `.a { width: 48px; font-size: 24px; }`},
		{"em in font-size is the parent's",
			`.a { font-size: 1.5em; width: 1em; }`, `.a { font-size: 24px; width: 24px; }`},
		{"rem is the root's",
			`.b { font-size: 24px; } .a { padding: 1rem; }`, `.b { font-size: 24px; } .a { padding: 16px; }`},
		{"vw and vh are the page's",
			`.a { width: 50vw; height: 25vh; }`, `.a { width: 100px; height: 20px; }`},
		{"a variable is measured where it is used",
			`.page { --gap: 1em; } .a { font-size: 24px; margin-left: var(--gap); }`,
			`.a { font-size: 24px; margin-left: 24px; }`},
		{"calc mixes them",
			`.a { width: calc(100% - 1em); }`, `.a { width: calc(100% - 16px); }`},
		{"the font shorthand's line height is the element's",
			`.a { font: 1.5em/1em ui; }`, `.a { font: 24px/24px ui; }`},
		{"a percentage font size is the parent's",
			`.b { font-size: 24px; } .a { font-size: 50%; width: 1em; }`, `.b { font-size: 24px; } .a { font-size: 12px; width: 12px; }`},
		{"and so it is in the shorthand",
			`.a { font: 150%/1 ui; }`, `.a { font: 24px/1 ui; }`},
		{"points are 4/3 of a pixel",
			`.a { border: 1.5pt solid black; }`, `.a { border: 2px solid black; }`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `<div class="page"><div class="b"><div class="a">x</div></div></div>`
			relative, err := Compile(body, page+test.relative)
			if err != nil {
				t.Fatal(err)
			}
			pixels, err := Compile(body, page+test.pixels)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(relative.JSON, pixels.JSON) {
				t.Errorf("%s compiled to\n%s\nbut %s to\n%s", test.relative, relative.JSON, test.pixels, pixels.JSON)
			}
			if len(relative.Warnings) != len(pixels.Warnings) {
				t.Errorf("%s said %v, %s said %v", test.relative, relative.Warnings, test.pixels, pixels.Warnings)
			}
		})
	}
}

func TestAViewportUnitWithNoPageIsSaid(t *testing.T) {
	for name, css := range map[string]string{
		"in the root's own size": `.page { width: 100vw; height: 80px; }`,
		"on a page of no size":   `.page { display: flex; } .a { width: 50vw; }`,
	} {
		t.Run(name, func(t *testing.T) {
			page, err := Compile(`<div class="page"><div class="a">x</div></div>`, css)
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if err != nil {
				said += err.Error()
			}
			if !strings.Contains(said, "give the root element a width and height in px") {
				t.Errorf("nothing said the page has no size; it said %q", said)
			}
		})
	}
}
