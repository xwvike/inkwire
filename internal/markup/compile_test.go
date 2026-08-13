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
// The pages that are not here cannot be: they exist to show arcs, circles,
// polygons, patterns and paths, and CSS has no way to ask for those. That is
// the same boundary chart.json sits on, and it is a boundary rather than a gap.
func rewritten() [][2]string {
	return [][2]string{
		{"../../examples/desk/", "disk"},
		{"../../examples/desk/", "claude"},
		{"../../examples/desk/", "tasks"},
		{"../../examples/desk/", "btc"},
		{"../../examples/text_showcase/", "page"},
		{"../../examples/schema_quickstart/", "page"},
	}
}

func readPage(t *testing.T, dir, name, extension string) []byte {
	t.Helper()
	source, err := os.ReadFile(dir + name + extension)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func render(t *testing.T, markupSource, cssSource string) (*display.Frame, []Warning, compose.Report) {
	t.Helper()
	document, err := Compile(markupSource, cssSource)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, report, err := compiler.Compile(compose.Document{Root: document.Root})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	return frame, document.Warnings, report
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
	fromMarkup, warnings, report := render(t,
		string(readPage(t, dir, name, ".html")), string(readPage(t, dir, name, ".css")))
	for _, warning := range warnings {
		t.Errorf("markup warning %s: %s", warning.Code, warning.Message)
	}
	for _, warning := range report.Warnings {
		t.Errorf("compose warning %s at %s: %s", warning.Code, warning.Path, warning.Message)
	}

	sceneSource := readPage(t, dir, name, ".json")
	result, err := (scene.Decoder{}).Render(bytes.NewReader(sceneSource))
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

// A font size with no strike cannot be approximated, because the sizes are a
// set and not a range. Rounding to the nearest would draw a size the author
// did not ask for and say nothing, so this fails outright, before anything
// reaches the panel. CSS authors will write 13px without thinking; they should
// find out here rather than by looking at the tag.
func TestAFontSizeWithNoStrikeIsRefused(t *testing.T) {
	document, err := Compile(`<div class="a">x</div>`, `.a { font-family: monaco; font-size: 13px; }`)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = compiler.Compile(compose.Document{Root: document.Root})
	if err == nil {
		t.Fatal("13px compiled, but monaco has no 13px strike")
	}
	if !strings.Contains(err.Error(), "13px") {
		t.Errorf("the error does not name the size that was refused: %v", err)
	}

	// A size the family does have is unaffected, including the enlargements.
	for _, size := range []int{12, 16, 48} {
		if _, _, err := compiler.Compile(compose.Document{Root: mustCompile(t,
			`<div class="a">x</div>`,
			fmt.Sprintf(`.a { font-family: monaco; font-size: %dpx; }`, size))}); err != nil {
			t.Errorf("monaco %dpx was refused: %v", size, err)
		}
	}
}

func mustCompile(t *testing.T, markupSource, cssSource string) compose.Node {
	t.Helper()
	document, err := Compile(markupSource, cssSource)
	if err != nil {
		t.Fatal(err)
	}
	return document.Root
}
