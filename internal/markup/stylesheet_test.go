package markup

import (
	"fmt"
	"strings"
	"testing"
)

// A page written in one file is the ordinary case for anything generated, and
// it did not work: a style element was read by nobody and its text was walked
// as if it were words, so the page came out as a paragraph of CSS.
func TestAPageCarriesItsOwnStylesheet(t *testing.T) {
	page, err := Compile(`<style>.page { display: flex; width: 60px; height: 20px; background: white; }</style>
		<div class="page"><span>OK</span></div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(string(page.JSON), `"width": 60`) {
		t.Errorf("the style element was not applied:\n%s", page.JSON)
	}
	if strings.Contains(string(page.JSON), "display") {
		t.Errorf("the stylesheet was drawn as words:\n%s", page.JSON)
	}
}

// A style element is allowed in the body, and the parser leaves it there. Its
// text is a stylesheet wherever it sits, and never content.
func TestAStyleElementInTheBodyIsStillAStylesheet(t *testing.T) {
	page, err := Compile(
		`<div class="page"><style>.page { display: flex; width: 60px; height: 20px; }
		 span { font-family: monaco; font-size: 10px; }</style><span>OK</span></div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if !strings.Contains(string(page.JSON), `"width": 60`) {
		t.Errorf("the style element was not applied:\n%s", page.JSON)
	}
	if strings.Contains(string(page.JSON), "font-family") {
		t.Errorf("the stylesheet was drawn as words:\n%s", page.JSON)
	}
}

// The page's own style comes after the file's, which is the order a browser
// applies them in: a file says what pages share and the page overrides it.
func TestThePagesOwnStyleWinsOverTheFileBesideIt(t *testing.T) {
	const both = `<style>.page { width: 40px; }</style>
		<div class="page"><span>OK</span></div>`
	page, err := Compile(both, `.page { display: flex; width: 90px; height: 20px; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page.JSON), `"width": 40`) {
		t.Errorf("the file won over the page's own style:\n%s", page.JSON)
	}
}

func TestALinkIsReadThroughTheResolver(t *testing.T) {
	asked := ""
	compiler := Compiler{Stylesheets: func(href string) ([]byte, error) {
		asked = href
		return []byte(`.page { display: flex; width: 70px; height: 20px; }`), nil
	}}
	page, err := compiler.Compile(
		`<link rel="stylesheet" href="shared.css"><div class="page"><span>OK</span></div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Errorf("warning: %s", warning.Message)
	}
	if asked != "shared.css" {
		t.Errorf("the resolver was asked for %q", asked)
	}
	if !strings.Contains(string(page.JSON), `"width": 70`) {
		t.Errorf("the linked stylesheet was not applied:\n%s", page.JSON)
	}
}

// A link that could not be read is named. A page whose stylesheet did not
// arrive lays out as almost nothing, and without this the only symptom is a
// blank panel.
func TestALinkThatCannotBeReadIsNamed(t *testing.T) {
	tests := map[string]Compiler{
		"with no resolver": {},
		"that fails": {Stylesheets: func(string) ([]byte, error) {
			return nil, fmt.Errorf("no such file")
		}},
	}
	for name, compiler := range tests {
		t.Run(name, func(t *testing.T) {
			page, err := compiler.Compile(
				`<link rel="stylesheet" href="missing.css"><div class="page">x</div>`,
				`.page { display: flex; width: 20px; height: 20px; }`)
			if err != nil {
				t.Fatal(err)
			}
			var said string
			for _, warning := range page.Warnings {
				said += warning.Code + " " + warning.Message
			}
			if !strings.Contains(said, "unresolved-stylesheet") || !strings.Contains(said, "missing.css") {
				t.Errorf("the link was not reported by name: %q", said)
			}
		})
	}
}

// Said once the whole document has been looked at, because that is the only
// point at which the answer is known. Asking it from the caller's side put a
// warning on every page that carried its own style.
func TestOnlyAPageWithNoStyleAtAllIsWarnedAbout(t *testing.T) {
	bare, err := Compile(`<div>x</div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !warned(bare, "no-stylesheet") {
		t.Error("a page with no style anywhere was not warned about")
	}
	for _, source := range []string{
		`<style>.page { display: flex; width: 20px; height: 20px; }</style><div class="page">x</div>`,
		`<div class="page" style="display: flex; width: 20px; height: 20px;">x</div>`,
	} {
		page, err := Compile(source, "")
		if err != nil {
			t.Fatal(err)
		}
		if warned(page, "no-stylesheet") {
			t.Errorf("a page carrying its own style was told it had none: %s", source)
		}
	}
}

func warned(document Document, code string) bool {
	for _, warning := range document.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

// A dashed border had one pattern, worked out from its width, and the schema's
// offset could not be asked for at all. Both are stated now, under the names
// SVG gives them.
func TestADashCanBeStated(t *testing.T) {
	const page = `.page { display: flex; width: 40px; height: 20px; } .a { flex-grow: 1; `
	tests := []struct {
		css  string
		want string
	}{
		{`border: 1px solid black; }`, `"stroke": {"ink": "black","width": 1}`},
		{`border: 1px solid black; border-style: dashed; }`, `"dash": [3,2]`},
		{`border: 1px solid black; stroke-dasharray: 6 2; }`, `"dash": [6,2]`},
		{`border: 1px solid black; stroke-dasharray: 6, 2; }`, `"dash": [6,2]`},
		{`border: 1px solid black; stroke-dasharray: 6 2; stroke-dashoffset: 3; }`, `"dashOffset": 3`},
	}
	for _, test := range tests {
		document, err := Compile(`<div class="page"><i class="a"></i></div>`, page+test.css)
		if err != nil {
			t.Fatal(err)
		}
		for _, warning := range document.Warnings {
			t.Errorf("%s: warning %s", test.css, warning.Message)
		}
		flat := strings.Join(strings.Fields(string(document.JSON)), "")
		if !strings.Contains(flat, strings.ReplaceAll(test.want, " ", "")) {
			t.Errorf("%s\n  wanted %s in\n%s", test.css, test.want, document.JSON)
		}
	}
}

// A pattern that is not a run of whole pixels is refused by name rather than
// rounded into something the author did not write.
func TestANonsenseDashIsRefused(t *testing.T) {
	for _, css := range []string{`stroke-dasharray: none;`, `stroke-dasharray: 50%;`, `stroke-dashoffset: 10%;`} {
		document, err := Compile(`<div class="page"><i class="a"></i></div>`,
			`.page { display: flex; width: 40px; height: 20px; } .a { flex-grow: 1; border: 1px solid black; `+css+` }`)
		if err != nil {
			t.Fatal(err)
		}
		if !warned(document, "unsupported-declaration") {
			t.Errorf("%s was accepted", css)
		}
	}
}

// The font shorthand carries a size and a family, which are the two things
// about text this renderer has. Refusing the whole line on account of a weight
// it does not have threw those away too.
func TestTheFontShorthandIsTakenApart(t *testing.T) {
	tests := []struct {
		css, font string
		size      int
	}{
		{`font: bold 14px monaco;`, "monaco", 14},
		{`font: 16px/24px monaco;`, "monaco", 16},
		{`font: italic 700 16px "Helvetica Neue", monaco;`, "monaco", 16},
	}
	for _, test := range tests {
		document, err := Compile(`<div class="page"><span class="a">x</span></div>`,
			`.page { display: flex; width: 80px; height: 30px; } .a { `+test.css+` }`)
		if err != nil {
			t.Fatal(err)
		}
		flat := strings.Join(strings.Fields(string(document.JSON)), "")
		if !strings.Contains(flat, fmt.Sprintf(`"font":%q,"size":%d`, test.font, test.size)) {
			t.Errorf("%s did not become %s at %d:\n%s", test.css, test.font, test.size, document.JSON)
		}
	}
}

// What it drops it names, so that a weight nobody can have is not silently
// nothing.
func TestTheFontShorthandSaysWhatItDropped(t *testing.T) {
	document, err := Compile(`<div class="page"><span class="a">x</span></div>`,
		`.page { display: flex; width: 80px; height: 30px; } .a { font: bold 14px monaco; }`)
	if err != nil {
		t.Fatal(err)
	}
	var said string
	for _, warning := range document.Warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "bold") {
		t.Errorf("the weight it dropped was not named: %q", said)
	}
}
