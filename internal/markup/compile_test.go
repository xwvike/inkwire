package markup

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

func readPage(t *testing.T, dir, name, extension string) []byte {
	t.Helper()
	source, err := os.ReadFile(dir + name + extension)
	if os.IsNotExist(err) && extension == ".css" {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func render(t *testing.T, markupSource, cssSource string) (*display.Frame, []Warning, compose.Report) {
	t.Helper()
	return renderIn(t, "", markupSource, cssSource)
}

// renderIn compiles a page and renders it the way the command line does:
// through the decoder every scene document goes through. Nothing here reaches
// into the compiled tree, because after this package has run there is no tree
// — there is a document, and the only way to draw one is to decode it.
func renderIn(t *testing.T, dir, markupSource, cssSource string) (*display.Frame, []Warning, compose.Report) {
	t.Helper()
	pages := Compiler{
		Stylesheets: func(href string) ([]byte, error) { return os.ReadFile(dir + href) },
		Drawings:    func(src string) ([]byte, error) { return os.ReadFile(dir + src) },
	}
	page, err := pages.Compile(markupSource, cssSource)
	if err != nil {
		t.Fatal(err)
	}
	frame, report := renderDocument(t, dir, page.JSON)
	return frame, page.Warnings, report
}

func TestRemoteSVGImageIsFetchedByItsURL(t *testing.T) {
	previous := remoteDrawingClient
	remoteDrawingClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`<svg width="20" height="20"><rect width="20" height="20" fill="black"/></svg>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	defer func() { remoteDrawingClient = previous }()

	page, err := Compile(
		`<div class="page"><img src="https://example.com/image.svg"></div>`,
		`.page { display: flex; width: 20px; height: 20px; background: white; }
		 img { display: block; flex-grow: 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	if got, _ := frame.InkAt(0, 0); got != display.InkBlack {
		t.Fatalf("remote drawing pixel = %v, want black", got)
	}
}

func TestExternalSVGImageDoesNotInheritPagePaint(t *testing.T) {
	resolver := Compiler{Drawings: func(src string) ([]byte, error) {
		return []byte(`<svg width="20" height="20"><rect width="20" height="20" fill="black"/></svg>`), nil
	}}
	page, err := resolver.Compile(
		`<div class="page"><img src="badge.svg"></div>`,
		`.page { display: flex; width: 20px; height: 20px; background: white; fill: red; }
			rect { fill: red; } img { display: block; width: 20px; height: 20px; }`)
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	if got, _ := frame.InkAt(5, 5); got != display.InkBlack {
		t.Fatalf("external SVG image inherited page paint: got %v, want black", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// renderDocument decodes and draws what a page compiled to. A failure here is
// a document this package wrote that the schema will not accept, which is the
// one kind of mistake the pixel comparisons cannot see: they need two pictures
// and this produces none.
func renderDocument(t *testing.T, dir string, written []byte) (*display.Frame, compose.Report) {
	t.Helper()
	document, err := (scene.Decoder{BaseDir: dir}).Decode(bytes.NewReader(written))
	if err != nil {
		t.Fatalf("the document this page compiled to was refused: %v\n%s", err, written)
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, report, err := compiler.Compile(document)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	return frame, report
}

// frameOf draws what a page compiled to, at the size the page states. A test
// that used to hand compose a size alongside the tree now has to say it in the
// stylesheet, which is where a page says everything else.
func frameOf(t *testing.T, page Document) *display.Frame {
	t.Helper()
	frame, _ := renderDocument(t, "", page.JSON)
	return frame
}

// A subset that quietly ignores what it does not implement is worse than a
// small subset that rejects it, so every unhandled declaration has to come
// back named.
func TestUnsupportedDeclarationsAreReported(t *testing.T) {
	const markupSource = `<div class="a"><span>x</span></div>`
	tests := []struct {
		name string
		css  string
		want string
	}{
		{"unknown property", `.a { box-shadow: 0 0 4px black; }`, "box-shadow"},
		{"colour off the palette", `.a { color: #3366ff; }`, "#3366ff"},
		{"unit with no meaning here", `.a { padding: 2em; }`, "2em"},
		{"unsupported display", `.a { display: table; }`, "table"},
		{"weight the strikes do not have", `.a { font-weight: bold; }`, "font-weight"},
		{"overflow handling", `.a { text-overflow: ellipsis; }`, "text-overflow"},
		{"skew", `.a { transform: skew(10deg); }`, "skew"},
		{"filter", `.a { filter: blur(2px); }`, "filter"},
		{"at-rule", `@media (min-width: 100px) { .a { color: red; } }`, "@media"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := Compile(markupSource, test.css)
			if err != nil {
				t.Fatal(err)
			}
			var joined string
			for _, warning := range document.Warnings {
				joined += warning.Message + "\n"
			}
			if !bytes.Contains([]byte(joined), []byte(test.want)) {
				t.Errorf("no warning naming %q; got %q", test.want, joined)
			}
		})
	}
}

// A font size with no strike is drawn at the nearest one there is, and said.
//
// This used to fail outright, on the reasoning that rounding would draw a size
// the author did not ask for and say nothing. The reasoning was half right:
// the fault was in saying nothing. An author writing 13px — which is what an
// author writes — got no picture at all and a message about strikes, and had
// to learn which nine sizes exist before they could write a page. That is a
// thing to have to remember, and having to remember is the whole cost this
// front end exists to remove.
//
// So it rounds, warns, and writes the size it settled on into the document,
// where it can be read.
func TestAFontSizeWithNoStrikeIsDrawnAtTheNearestOne(t *testing.T) {
	const page = `.page { display: flex; width: 60px; height: 20px; background: white; }`
	document, err := Compile(`<div class="page"><span class="a">x</span></div>`,
		page+` .a { font-family: monaco; font-size: 13px; }`)
	if err != nil {
		t.Fatal(err)
	}
	var said string
	substitutions := 0
	for _, warning := range document.Warnings {
		said += warning.Code + " " + warning.Message
		if warning.Code == "substituted-font-size" {
			substitutions++
		}
	}
	if !strings.Contains(said, "substituted-font-size") || !strings.Contains(said, "13px") {
		t.Errorf("the substitution was not reported: %q", said)
	}
	if substitutions != 1 {
		t.Errorf("font-size substitution was reported %d times, want once", substitutions)
	}
	if !strings.Contains(string(document.JSON), `"size": 12`) {
		t.Errorf("the document does not say the size it settled on:\n%s", document.JSON)
	}
	if _, err := drawn(t, `<div class="page"><span class="a">x</span></div>`,
		page+` .a { font-family: monaco; font-size: 13px; }`); err != nil {
		t.Errorf("the page did not draw: %v", err)
	}

	// A size the family does have is unaffected, and says nothing.
	for _, size := range []int{12, 16, 48} {
		document, err := Compile(`<div class="page"><span class="a">x</span></div>`,
			page+fmt.Sprintf(` .a { font-family: monaco; font-size: %dpx; }`, size))
		if err != nil {
			t.Fatal(err)
		}
		for _, warning := range document.Warnings {
			t.Errorf("monaco %dpx was reported: %s", size, warning.Message)
		}
	}
}

// A family this build does not have is the same situation and gets the same
// answer. It is the more common one: a stylesheet copied from anywhere names
// fonts by the names browsers know them by.
func TestAnUnknownFontFamilyFallsBackAndIsSaid(t *testing.T) {
	document, err := Compile(`<div class="page"><span class="a">x</span></div>`,
		`.page { display: flex; width: 60px; height: 20px; }
		 .a { font-family: "Helvetica Neue", Arial, sans-serif; }`)
	if err != nil {
		t.Fatal(err)
	}
	var said string
	for _, warning := range document.Warnings {
		said += warning.Message
	}
	if !strings.Contains(said, "Helvetica Neue") || !strings.Contains(said, "ui") {
		t.Errorf("the fallback was not reported: %q", said)
	}

	// A stack whose first name is not here but whose second is takes the
	// second, which is what the stack is for.
	stacked, err := Compile(`<div class="page"><span class="a">x</span></div>`,
		`.page { display: flex; width: 60px; height: 20px; }
		 .a { font-family: Helvetica, monaco, hzk, sans-serif; font-size: 10px; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range stacked.Warnings {
		t.Errorf("a stack naming monaco was reported: %s", warning.Message)
	}
	if !strings.Contains(string(stacked.JSON), `"font": "monaco"`) {
		t.Errorf("the stack did not settle on monaco:\n%s", stacked.JSON)
	}
	if !strings.Contains(string(stacked.JSON), `"fallback"`) || !strings.Contains(string(stacked.JSON), `"hzk"`) {
		t.Errorf("the stack did not preserve hzk as a fallback:\n%s", stacked.JSON)
	}
}

func TestFontFamilyFallbackIsResolvedPerGlyph(t *testing.T) {
	frame, _, report := render(t, `<div class="page"><span class="a">A中</span></div>`,
		`.page { display: flex; width: 40px; height: 20px; background: white; }
		 .a { font-family: monaco, hzk; }`)
	if len(report.MissingRunes) != 0 {
		t.Fatalf("font-family fallback left runes missing: %q", string(report.MissingRunes))
	}
	got := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				got++
			}
		}
	}
	if got == 0 {
		t.Fatal("font-family fallback drew no glyphs")
	}
}

// drawn compiles and draws a page, handing back whatever refused it. The
// refusal is the subject here, so unlike render it does not end the test.
func drawn(t *testing.T, markupSource, cssSource string) (*display.Frame, error) {
	t.Helper()
	page, err := Compile(markupSource, cssSource)
	if err != nil {
		return nil, err
	}
	document, err := (scene.Decoder{}).Decode(bytes.NewReader(page.JSON))
	if err != nil {
		return nil, err
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, _, err := compiler.Compile(document)
	if err != nil {
		return nil, err
	}
	return compiled.Render()
}
