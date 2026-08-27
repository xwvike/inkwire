package markup

import (
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
