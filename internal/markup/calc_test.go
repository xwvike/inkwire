package markup

import (
	"strings"
	"testing"
)

func TestMathIsWorkedOutAsCSSWorksItOut(t *testing.T) {
	tests := map[string]length{
		"calc(10px + 20px)":           {set: true, pixels: 30},
		"calc(100% - 10px)":           {set: true, percent: 100, pixels: -10},
		"calc(100%-10px)":             {set: true, percent: 100, pixels: -10},
		"calc(3px * 2)":               {set: true, pixels: 6},
		"calc(2 * 3px)":               {set: true, pixels: 6},
		"calc(10px / 4)":              {set: true, pixels: 2.5},
		"calc(50% / 2 + 4px)":         {set: true, percent: 25, pixels: 4},
		"calc((100% - 20px) / 2)":     {set: true, percent: 50, pixels: -10},
		"calc(2px + 3px * 2)":         {set: true, pixels: 8},
		"calc(-2px + 5px)":            {set: true, pixels: 3},
		"calc(10px - -2px)":           {set: true, pixels: 12},
		"calc(calc(4px + 4px) * 2)":   {set: true, pixels: 16},
		"calc(0 + 5px)":               {set: true, pixels: 5},
		"min(10px, 4px, 8px)":         {set: true, pixels: 4},
		"max(10px, 4px, 8px)":         {set: true, pixels: 10},
		"clamp(12px, 20px, 16px)":     {set: true, pixels: 16},
		"clamp(12px, 8px, 16px)":      {set: true, pixels: 12},
		"clamp(12px, 14px, 16px)":     {set: true, pixels: 14},
		"calc(min(10px, 20px) + 50%)": {set: true, percent: 50, pixels: 10},
		"CALC(1PX + 1PX)":             {set: true, pixels: 2},
		"calc( 1px  +  2px )":         {set: true, pixels: 3},
		"max(calc(2px * 3), 5px)":     {set: true, pixels: 6},
		"calc(1.5 * (4px + 2px))":     {set: true, pixels: 9},
	}
	for value, want := range tests {
		var said string
		got := parseMath(value, "width", func(message string) { said += message })
		if got != want || said != "" {
			t.Errorf("%s came to %+v (said %q); want %+v", value, got, said, want)
		}
	}
	refused := map[string]string{
		"calc(2px * 3px)": "not by another length",
		"calc(4px / 2px)": "not by a length",
		"calc(4px / 0)":   "divides by zero",
		"calc(4px + 2)":   "cannot add a number to a length",
		"calc(4px +)":     "an operator with no length after it",
		"calc(4px + 2px":  "a bracket is not closed",
		"calc(4px 2px)":   "no operator between them",
		"calc(4px) 2px":   "is not where it can be read",
		"min(50%, 10px)":  "cannot be worked out until the box",
		"clamp(1px, 2px)": "takes a least, a preferred and a most",
		"calc(2 * 3)":     "works out to a number",
		"calc(2ch + 1px)": "must be given in px",
		"min(2px, 3)":     "between a number and a length",
	}
	for value, want := range refused {
		var said string
		got := parseMath(value, "width", func(message string) { said += message + "\n" })
		if got.set || !strings.Contains(said, want) || strings.Count(said, "\n") != 1 {
			t.Errorf("%s came to %+v and said %q; want it refused once, saying %q", value, got, said, want)
		}
	}
}

// What the units and the variables come to is worked out before the sum, so
// the sums a design system writes come out as they would in a browser.
func TestMathTakesUnitsAndVariables(t *testing.T) {
	body := `<div class="page"><div class="a">x</div></div>`
	const page = `.page { width: 200px; height: 80px; font-size: 16px; --space: 4px; } `
	for relative, pixels := range map[string]string{
		`.a { padding: calc(var(--space) * 2); }`:      `.a { padding: 8px; }`,
		`.a { width: clamp(10px, 50vw, 60px); }`:       `.a { width: 60px; }`,
		`.a { width: calc(100% - 2 * var(--space)); }`: `.a { width: calc(100% - 8px); }`,
		`.a { margin-left: calc(1em / 2); }`:           `.a { margin-left: 8px; }`,
	} {
		got, err := Compile(body, page+relative)
		if err != nil {
			t.Fatal(err)
		}
		want, err := Compile(body, page+pixels)
		if err != nil {
			t.Fatal(err)
		}
		if string(got.JSON) != string(want.JSON) || len(got.Warnings) != 0 {
			t.Errorf("%s did not draw as %s: %v", relative, pixels, got.Warnings)
		}
	}
}

// A sum can come out below zero once the box it is a share of is known, and a
// size that does is held at zero, as a browser holds it, without a word.
func TestASumBelowZeroIsNotRefused(t *testing.T) {
	for _, css := range []string{`.a { width: calc(10% - 40px); }`, `.a { width: min(-5px, 10px); }`} {
		if said := warningsFor(t, `<div class="a">x</div>`, css); said != "" {
			t.Errorf("%s said %q", css, said)
		}
	}
}

// A custom property is a name the author made up, so --Big and --big are two.
func TestCustomPropertyNamesKeepTheirCase(t *testing.T) {
	body := `<div class="page"><div class="a">x</div></div>`
	got, err := Compile(body, `.page { width: 100px; height: 40px; --Gap: 7px; --gap: 3px; } .a { width: var(--Gap); }`)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Compile(body, `.page { width: 100px; height: 40px; } .a { width: 7px; }`)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.JSON) != string(want.JSON) || len(got.Warnings) != 0 {
		t.Errorf("var(--Gap) did not take --Gap: %v", got.Warnings)
	}
}
