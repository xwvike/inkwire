package markup

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Nothing a stylesheet says may stop the compiler coming back.
//
// Three things used to. repeat() walks its count, so a count somebody typed
// wrong walked a billion times; and a custom property declared as itself, or
// two declared as each other, substituted forever. On a service each of those
// is one request that wedges a goroutine and takes the memory with it, which
// is a worse failure than a wrong picture.
//
// The timeout is what makes this a test rather than a hope: a loop that does
// not terminate cannot be caught by looking at the answer.
func TestNoStylesheetCanHangTheCompiler(t *testing.T) {
	const frame = `.page { display: flex; width: 100px; height: 40px; }`
	tests := map[string]string{
		"a repeat count nobody meant": `.page { display: grid;
			grid-template-columns: repeat(1000000000, 1px); width: 100px; height: 40px; }`,
		"a property declared as itself":   `:root { --a: var(--a); } ` + frame + ` i { width: var(--a); }`,
		"two declared as each other":      `:root { --a: var(--b); --b: var(--a); } ` + frame + ` i { width: var(--a); }`,
		"a var nested inside another var": `:root { --a: var(--b); --b: 20px; } ` + frame + ` i { width: var(--a); }`,
		"a repeat inside a track list":    `.page { display: grid; grid-template-columns: 10px repeat(4, 1fr) 10px; width: 100px; height: 40px; }`,
	}
	for name, css := range tests {
		t.Run(name, func(t *testing.T) {
			done := make(chan Document, 1)
			go func() {
				page, err := Compile(`<div class="page"><i></i></div>`, css)
				if err != nil {
					t.Errorf("the page did not compile: %v", err)
				}
				done <- page
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the compiler did not come back")
			}
		})
	}
}

// A stylesheet copied from a real site carries selectors no subset has, and
// usually more than one. Losing the page over the first of them means losing
// the parts that had nothing to do with the panel.
func TestASelectorThisBuildCannotReadIsSkipped(t *testing.T) {
	const frame = `.page { display: flex; width: 100px; height: 40px; background: white; }
	               .a { display: block; flex-grow: 1; background: black; }`
	tests := map[string]string{
		"a vendor's pseudo-element": `input::-webkit-inner-spin-button { color: red; }`,
		"::placeholder":             `input::placeholder { color: red; }`,
		":is()":                     `:is(h1, h2) { color: red; }`,
		":has()":                    `div:has(> span) { color: red; }`,
		"something not a selector":  `>>> ((( { color: red; }`,
	}
	for name, bad := range tests {
		t.Run(name, func(t *testing.T) {
			page, err := Compile(`<div class="page"><i class="a"></i></div>`, bad+frame)
			if err != nil {
				t.Fatalf("the page did not compile: %v", err)
			}
			if !warned(page, "unsupported-selector") {
				t.Error("the selector was not reported")
			}
			// The rules around it still apply, which is the whole point.
			if !strings.Contains(string(page.JSON), `"width": 100`) {
				t.Errorf("the rest of the stylesheet was lost:\n%s", page.JSON)
			}
		})
	}
}

// A browser skips the rule it cannot read and carries on. One missing brace
// near the bottom of a stylesheet does not blank the page, and it should not
// here either.
func TestAStylesheetWithASyntaxErrorKeepsWhatParsed(t *testing.T) {
	const good = `.page { display: flex; width: 100px; height: 40px; background: white; }`
	tests := map[string]string{
		"a stray semicolon":     `.a { color: ; ; }`,
		"a stray closing brace": `}}}`,
		"a rule never closed":   `.b { color: red;`,
		"no property name":      `.c { : red; }`,
	}
	for name, bad := range tests {
		t.Run(name, func(t *testing.T) {
			page, err := Compile(`<div class="page">x</div>`, good+"\n"+bad)
			if err != nil {
				t.Fatalf("the page did not compile: %v", err)
			}
			if !strings.Contains(string(page.JSON), `"width": 100`) {
				t.Errorf("the readable rule was lost as well:\n%s", page.JSON)
			}
		})
	}
}

// The refusals that are right. A document with nothing in it has nothing to
// draw, and saying so is the answer — quietly returning an empty page would
// send an author looking at their stylesheet.
func TestADocumentWithNothingInItIsRefused(t *testing.T) {
	for name, markup := range map[string]string{
		"nothing":        "",
		"only spaces":    "   \n  ",
		"only a doctype": "<!DOCTYPE html>",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(markup, ""); err == nil {
				t.Error("a document with nothing in it compiled")
			}
		})
	}
}

// Things that are merely extreme, as against malformed, come out the far end.
func TestExtremeButWellFormedInputIsCompiled(t *testing.T) {
	const frame = `.page { display: flex; width: 100px; height: 40px; }`
	tests := map[string]struct{ markup, css string }{
		"two hundred thousand characters of text": {
			`<div class="page">` + strings.Repeat("字", 200000) + `</div>`, frame},
		"a size larger than any panel": {
			`<div class="page"><i></i></div>`, `.page { display: flex; width: 99999999px; height: 99999999px; }`},
		"a negative size": {
			`<div class="page"><i></i></div>`, `.page { display: flex; width: -50px; height: -50px; }`},
		"calc inside calc": {
			`<div class="page"><i></i></div>`, frame + ` i { width: calc(calc(50% - 1px) + calc(2px)); }`},
		"bytes that are not UTF-8": {
			"<div class=\"page\">\xff\xfe</div>", frame},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(test.markup, test.css); err != nil {
				t.Errorf("did not compile: %v", err)
			}
		})
	}
}

// Nothing an author can write in a declaration may bring the process down.
// This matters more here than in most parsers: the server compiles a page it
// was handed over HTTP, so a declaration is untrusted input and a panic is a
// way to stop the machine from a stylesheet.
//
// "gap:;" was that. Several properties read the first word of their value, and
// a value with no words at all reached them as an empty slice. The fix is a
// single refusal at the top of apply, and this is what says so stays true for
// whatever is added to the switch next.
func TestNoDeclarationCanPanic(t *testing.T) {
	properties := strings.Fields(`display width height min-width max-width min-height max-height
		aspect-ratio box-sizing padding padding-top padding-right padding-bottom padding-left
		margin margin-top margin-right margin-bottom margin-left flex flex-direction flex-basis
		flex-grow flex-shrink gap row-gap column-gap align-items align-self justify-content justify-items
		justify-self grid-template-columns grid-template-rows grid-column grid-row position top
		right bottom left inset z-index background background-color color border border-width
		border-style border-color border-top border-right border-bottom border-left
		border-top-width border-right-width border-bottom-width border-left-width
		border-top-style border-right-style border-bottom-style border-left-style
		border-top-color border-right-color border-bottom-color border-left-color border-radius visibility overflow clip-path transform rotate
		transform-origin scale fill stroke stroke-width stroke-dasharray stroke-dashoffset font
		font-family font-size line-height text-align vertical-align white-space object-fit`)
	values := []string{"", " ", "/", "(", ")", "()", "calc(", "calc()", "calc(1px +", "-",
		"0", "1", "-1", "px", "1px 2px 3px 4px 5px", ",", "a,b", "url(", "url(#)", "var(--x)",
		"inset(", "circle(", "polygon(1px)", "rotate(", "repeat(", "repeat(1)", "span",
		"span 0", "1 /", "/ 2", "1 / span", "auto auto auto auto auto", "\t\n", "'", "\"",
		"1e999", "99999999999999999999", "NaN", "Inf", "-Inf", "+Inf", "infinity", "nan",
		"NaNpx", "Infdeg", "NaNfr", "NaN / 1", "1 / NaN", "calc(NaN + 1px)",
		"#", "#0", "#00000000000"}
	for _, property := range properties {
		for _, value := range values {
			func() {
				defer func() {
					if problem := recover(); problem != nil {
						t.Errorf("%s: %q panicked: %v", property, value, problem)
					}
				}()
				_, _ = Compile(`<div class="page"><span class="a">x</span></div>`,
					fmt.Sprintf(`.page { display: flex; width: 40px; height: 20px; }
					 .a { %s: %s }`, property, value))
			}()
		}
	}
}

// An empty value is refused by name rather than ignored, so an author who left
// a value out is told which property they left it out of.
func TestAnEmptyValueIsReportedByName(t *testing.T) {
	document, err := Compile(`<div class="page"><span>x</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; gap: ; }`)
	if err != nil {
		t.Fatal(err)
	}
	var said string
	for _, warning := range document.Warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "gap") || !strings.Contains(said, "no value") {
		t.Errorf("the empty gap was not reported: %q", said)
	}
}

// A number CSS cannot spell must not reach the document. strconv reads NaN and
// Inf by name; json.Marshal refuses both, so a single unreadable declaration
// stopped the whole page compiling — which is the opposite of the promise that
// a bad declaration costs you that declaration and nothing else.
func TestANonFiniteNumberCostsOnlyItsOwnDeclaration(t *testing.T) {
	for _, declaration := range []string{
		"rotate: NaNdeg", "rotate: Infdeg", "aspect-ratio: NaN", "aspect-ratio: 1 / Inf",
		"flex-grow: Inf", "z-index: NaN", "width: NaNpx", "height: Infpx",
		"line-height: Inf", "scale: Inf", "flex-basis: 1e999px",
		"grid-template-columns: NaNfr", "padding: NaNpx", "font-size: Infpx",
	} {
		t.Run(declaration, func(t *testing.T) {
			document, err := Compile(`<div class="page"><i class="a">x</i></div>`,
				`.page { display: flex; width: 60px; height: 40px; }
				 .a { flex-grow: 1; background: red; `+declaration+`; }`)
			if err != nil {
				t.Fatalf("the page did not compile: %v", err)
			}
			var said string
			for _, warning := range document.Warnings {
				said += warning.Message
			}
			if said == "" {
				t.Errorf("%q was taken silently", declaration)
			}
			for _, spelling := range []string{"NaN", "Inf", "+Inf", "-Inf"} {
				if strings.Contains(string(document.JSON), spelling) {
					t.Errorf("%s reached the document:\n%s", spelling, document.JSON)
				}
			}
		})
	}
}
