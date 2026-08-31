package testscene

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/markup"
	"github.com/xwvike/inkwire/internal/scene"
)

// RenderPage draws an example the way its author writes it.
//
// Every example is a page written as HTML, with its styles in a style element
// and its picture beside it. The fallback below keeps tests able to render an
// older encoded page when a fixture explicitly needs one.
func RenderPage(t *testing.T, dir, name string) scene.Result {
	t.Helper()
	page := filepath.Join(dir, name+".html")
	if _, err := os.Stat(page); err != nil {
		document, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := (scene.Decoder{BaseDir: dir}).Render(bytes.NewReader(document))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	markupSource, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	beside := func(named string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, filepath.Clean("/"+named)))
	}
	cssSource, err := os.ReadFile(filepath.Join(dir, name+".css"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	compiled, err := (markup.Compiler{Stylesheets: beside, Drawings: beside}).
		Compile(string(markupSource), string(cssSource))
	if err != nil {
		t.Fatal(err)
	}
	var said []string
	for _, warning := range compiled.Warnings {
		said = append(said, warning.Code+": "+warning.Message)
	}
	if len(said) != 0 {
		t.Errorf("compiling %s said: %s", page, strings.Join(said, "; "))
	}
	result, err := (scene.Decoder{BaseDir: dir}).Render(bytes.NewReader(compiled.JSON))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
