package markup

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

// The pages live beside the scene documents they were rewritten from, so a
// reader comparing the two formats finds them side by side rather than one of
// them buried in a test directory.
const examples = "../../examples/desk/"

// rewritten names every page that exists in both formats, by the directory and
// base name that hold them. Pixel identity between the two is what keeps two
// authoring formats from becoming two renderers.
//
// chart belongs here even though CSS cannot describe a ninety-six point
// polyline, because the page does not ask it to: the layout is a stylesheet
// and the plot is a scene element handed over. Leaving it out is how its
// stylesheet came to draw a plot area of a different height from the one the
// scene document draws, with nothing to notice.
//
// The pages that are not here cannot be: they exist to show arcs, circles,
// polygons, patterns and paths on their own, and a page with nothing but
// geometry in it is a scene document rather than a page.
func rewritten() [][2]string {
	return [][2]string{
		{"../../examples/schema_quickstart/", "page"},
	}
}

func readPage(t *testing.T, dir, name, extension string) []byte {
	t.Helper()
	source, err := os.ReadFile(dir + name + extension)
	// An example carries its styles in a style element, so the sibling
	// stylesheet the compiler also accepts is usually not there. Its absence
	// is the ordinary case rather than a broken example.
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

// The whole point of the exercise: a page written as HTML and CSS has to reach
// the panel as the same drawing commands the scene document produces. Anything
// less and the two authoring formats would drift into two renderers.
func TestTheHTMLPagesMatchTheirSceneDocumentsExactly(t *testing.T) {
	for _, page := range rewritten() {
		dir, name := page[0], page[1]
		label := strings.TrimSuffix(strings.TrimPrefix(dir, "../../examples/"), "/") + "/" + name
		t.Run(label, func(t *testing.T) { assertSamePixels(t, dir, name) })
	}
}

func assertSamePixels(t *testing.T, dir, name string) {
	t.Helper()
	fromMarkup, warnings, report := renderIn(t, dir,
		string(readPage(t, dir, name, ".html")), string(readPage(t, dir, name, ".css")))
	for _, warning := range warnings {
		t.Errorf("markup warning %s: %s", warning.Code, warning.Message)
	}
	for _, warning := range report.Warnings {
		t.Errorf("compose warning %s at %s: %s", warning.Code, warning.Path, warning.Message)
	}

	// The same directory the markup side gets, because a page that names a
	// picture names it relative to itself and the scene document beside it
	// names the same one the same way.
	sceneSource := readPage(t, dir, name, ".json")
	result, err := (scene.Decoder{BaseDir: dir}).Render(bytes.NewReader(sceneSource))
	if err != nil {
		t.Fatal(err)
	}
	fromScene := result.Frame

	if fromMarkup.Bounds() != fromScene.Bounds() {
		t.Fatalf("markup rendered %v, the scene document renders %v", fromMarkup.Bounds(), fromScene.Bounds())
	}
	differing := 0
	firstX, firstY := -1, -1
	for y := 0; y < fromScene.Height(); y++ {
		for x := 0; x < fromScene.Width(); x++ {
			want := color.NRGBAModel.Convert(fromScene.At(x, y))
			if got := color.NRGBAModel.Convert(fromMarkup.At(x, y)); got != want {
				differing++
				if firstX < 0 {
					firstX, firstY = x, y
				}
			}
		}
	}
	if differing != 0 {
		t.Errorf("%d of %d pixels differ, first at (%d,%d)",
			differing, fromScene.Width()*fromScene.Height(), firstX, firstY)
	}
}

// A subset that quietly ignores what it does not implement is worse than a
// small schema that never accepted it, so every unhandled declaration has to
// come back named.
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
	for _, warning := range document.Warnings {
		said += warning.Code + " " + warning.Message
	}
	if !strings.Contains(said, "substituted-font-size") || !strings.Contains(said, "13px") {
		t.Errorf("the substitution was not reported: %q", said)
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
		 .a { font-family: Helvetica, monaco, sans-serif; font-size: 10px; }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range stacked.Warnings {
		t.Errorf("a stack naming monaco was reported: %s", warning.Message)
	}
	if !strings.Contains(string(stacked.JSON), `"font": "monaco"`) {
		t.Errorf("the stack did not settle on monaco:\n%s", stacked.JSON)
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
