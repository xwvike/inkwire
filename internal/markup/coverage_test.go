package markup

import (
	"image"
	"sort"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

// classify says what one declaration actually did. A declaration that changes
// nothing and says nothing is the failure this package exists to avoid: the
// author believes it applied.
func classify(t *testing.T, body, base, declaration string) string {
	t.Helper()
	without, _ := renderProbe(t, body, base)
	with, said := renderProbe(t, body, base+" "+declaration)
	// A refusal names the declaration; a consequence describes the layout it
	// produced. Treating every warning as a refusal reported four properties
	// as unsupported that had in fact worked and then overflowed the box they
	// were given, which is the renderer doing both of its jobs.
	refused := strings.Contains(said, "not a property") ||
		strings.Contains(said, "is not supported") ||
		strings.Contains(said, "must be") ||
		strings.Contains(said, "cannot be") ||
		strings.Contains(said, "not one of the panel")
	switch {
	case refused:
		return "REFUSED  " + strings.TrimSpace(strings.SplitN(said, "\n", 2)[0])
	case without == nil || with == nil:
		return "ERROR"
	case !samePixels(without, with):
		if said != "" {
			return "APPLIED  (and reported: " + strings.TrimSpace(strings.SplitN(said, "\n", 2)[0]) + ")"
		}
		return "APPLIED"
	}
	return "SILENT"
}

func renderProbe(t *testing.T, body, css string) (*display.Frame, string) {
	t.Helper()
	compiler := Compiler{Images: func(string) (image.Image, error) {
		source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if (x+y)%2 == 0 {
					source.Set(x, y, image.Black)
				} else {
					source.Set(x, y, image.White)
				}
			}
		}
		return source, nil
	}}
	document, err := compiler.Compile(`<div class="page">`+body+`</div>`,
		`.page { display: flex; width: 60px; height: 40px; background: white; }`+css)
	var said string
	for _, warning := range document.Warnings {
		said += warning.Message + "\n"
	}
	if err != nil {
		return nil, said + err.Error()
	}
	composed, err := compose.NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, report, err := composed.Compile(compose.Document{Size: image.Pt(60, 40), Root: document.Root})
	if err != nil {
		return nil, said + err.Error()
	}
	for _, warning := range report.Warnings {
		said += warning.Message + "\n"
	}
	frame, err := compiled.Render()
	if err != nil {
		return nil, said + err.Error()
	}
	return frame, said
}

func samePixels(a, b *display.Frame) bool {
	for y := 0; y < a.Height(); y++ {
		for x := 0; x < a.Width(); x++ {
			left, _ := a.InkAt(x, y)
			right, _ := b.InkAt(x, y)
			if left != right {
				return false
			}
		}
	}
	return true
}

func TestCoverageOfTheRequestedProperties(t *testing.T) {
	const twoItems = `<i class="a">text</i><i class="b">more</i>`
	const shapes = `.a { display: block; background: black; flex-grow: 1; color: white; }` +
		` .b { display: block; background: red; flex-grow: 1; }`
	const picture = `<img src="p.png" class="a"><i class="b"></i>`

	tests := []struct {
		property    string
		body        string
		base        string
		declaration string
	}{
		{"aspect-ratio", twoItems, shapes, `.a { aspect-ratio: 16 / 9; }`},
		{"box-sizing", twoItems, shapes, `.a { box-sizing: border-box; padding: 4px; }`},
		{"display:inline-block", twoItems, shapes, `.a { display: inline-block; }`},
		{"display:grid", twoItems, shapes, `.a { display: grid; }`},
		{"display:inline-flex", twoItems, shapes, `.a { display: inline-flex; }`},
		{"float", twoItems, shapes, `.a { float: left; }`},
		{"object-fit", picture, shapes, `.a { object-fit: cover; }`},
		{"object-position", picture, shapes, `.a { object-position: top left; }`},
		{"overflow", twoItems, shapes, `.a { overflow: hidden; }`},
		{"overflow-x", twoItems, shapes, `.a { overflow-x: hidden; }`},
		{"overscroll-behavior", twoItems, shapes, `.a { overscroll-behavior: contain; }`},
		{"position:relative", twoItems, shapes, `.a { position: relative; }`},
		{"position:absolute", twoItems, shapes, `.a { position: absolute; }`},
		{"top/left", twoItems, shapes, `.a { position: absolute; top: 5px; left: 5px; }`},
		{"inset", twoItems, shapes, `.a { inset: 5px; }`},
		{"visibility", twoItems, shapes, `.a { visibility: hidden; }`},
		{"z-index", twoItems, shapes, `.a { z-index: 5; }`},
		{"padding", twoItems, shapes, `.a { padding: 4px; }`},
		{"margin", twoItems, shapes, `.a { margin: 4px; }`},
		{"min-width", twoItems, shapes, `.a { min-width: 50px; }`},
		{"max-width", twoItems, shapes, `.a { max-width: 10px; }`},
		{"min-height", twoItems, shapes, `.a { min-height: 30px; }`},
		{"background", twoItems, shapes, `.a { background: red; }`},
		{"background-size", twoItems, shapes, `.a { background-size: cover; }`},
		{"background-repeat", twoItems, shapes, `.a { background-repeat: no-repeat; }`},
		{"border-radius", twoItems, shapes, `.a { border: 1px solid white; border-radius: 4px; }`},
		{"border-style:solid", twoItems, shapes, `.a { border-style: solid; border-width: 1px; border-color: white; }`},
		{"border-style:dashed", twoItems, shapes, `.a { border-style: dashed; border-width: 1px; border-color: white; }`},
		{"outline-width", twoItems, shapes, `.a { outline-width: 1px; outline-style: solid; }`},
		{"mask-image", twoItems, shapes, `.a { mask-image: url(m.png); }`},
		{"scale", twoItems, shapes, `.a { scale: 2; }`},
		{"rotate", twoItems, shapes, `.a { rotate: 90deg; }`},
		{"transform", twoItems, shapes, `.a { transform: rotate(90deg); }`},
		{"calc", twoItems, shapes, `.a { width: calc(100% - 10px); }`},
		{"var", twoItems, shapes, `:root { --w: 20px; } .a { width: var(--w); }`},
		{"inherit keyword", twoItems, shapes, `.a { color: inherit; }`},
		{"initial keyword", twoItems, shapes, `.a { color: initial; }`},
	}

	results := map[string][]string{}
	for _, test := range tests {
		outcome := classify(t, test.body, test.base, test.declaration)
		key := strings.SplitN(outcome, "  ", 2)[0]
		detail := test.property
		if key == "REFUSED" {
			detail = test.property
		}
		results[key] = append(results[key], detail)
		t.Logf("%-24s %s", test.property, outcome)
	}
	for _, key := range []string{"APPLIED", "REFUSED", "SILENT", "ERROR"} {
		names := results[key]
		sort.Strings(names)
		t.Logf("== %-8s %2d: %s", key, len(names), strings.Join(names, ", "))
	}
}
