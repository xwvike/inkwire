package markup

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
)

// Nothing a web developer writes may stop a page being drawn.
//
// This is the rule the front end exists to keep, and it is worth a test of its
// own because it is easy to break one declaration at a time. A property this
// renderer does not have, a colour the panel cannot make, a unit that means
// nothing on a fixed panel, a font nobody bundled — each of those is a thing
// the author has to be told about and none of them is a reason to hand back
// nothing. A page that fails is a page whose author has to learn the subset
// before they can write anything at all, and having to learn it is the cost
// this whole front end exists to remove.
//
// Two of these used to fail outright: a font size with no strike, and a family
// this build does not have. Between them they cover most of what a stylesheet
// copied from anywhere says about text.
func TestNothingAnAuthorWritesStopsThePageBeingDrawn(t *testing.T) {
	const frame = `.page { display: flex; flex-direction: column;
	                       width: 200px; height: 80px; background: white; }`
	tests := map[string]string{
		"a font nobody bundled":     `.a { font-family: Arial, sans-serif; }`,
		"a size with no strike":     `.a { font-size: 13px; }`,
		"a size past the largest":   `.a { font-size: 100px; }`,
		"a colour with no ink":      `.a { color: blue; }`,
		"a colour written as hex":   `.a { color: #336699; }`,
		"a unit that means nothing": `.a { font-size: 1.2rem; width: 50vw; }`,
		"a weight there is one of":  `.a { font-weight: 600; }`,
		"a property nobody has":     `.a { opacity: .5; box-shadow: 0 1px 2px #000; transition: all .2s; }`,
		"an at-rule":                `@media (min-width: 10px) { .a { color: red; } }`,
		"a shorthand":               `.a { font: italic 600 13px/1.4 "Segoe UI", Roboto, sans-serif; }`,
		"a whole block of it": `.a {
			font-family: -apple-system, "Segoe UI", Roboto, sans-serif;
			font-size: 13px; font-weight: 600; line-height: 1.4;
			color: #333; letter-spacing: .02em; text-transform: uppercase;
			box-shadow: 0 1px 2px rgba(0,0,0,.1); border-radius: 6px;
			transition: color .2s ease; opacity: 1; float: left; }`,
	}
	for name, css := range tests {
		t.Run(name, func(t *testing.T) {
			page, err := Compile(`<div class="page"><span class="a">Hello 世界</span></div>`, frame+css)
			if err != nil {
				t.Fatalf("the page did not compile: %v", err)
			}
			document, err := (scene.Decoder{}).Decode(bytes.NewReader(page.JSON))
			if err != nil {
				t.Fatalf("the document it compiled to was refused: %v\n%s", err, page.JSON)
			}
			compiler, err := compose.NewDefaultCompiler()
			if err != nil {
				t.Fatal(err)
			}
			compiled, _, err := compiler.Compile(document)
			if err != nil {
				t.Fatalf("the page did not lay out: %v", err)
			}
			frame, err := compiled.Render()
			if err != nil {
				t.Fatalf("the page did not draw: %v", err)
			}
			ink := 0
			for y := 0; y < frame.Height(); y++ {
				for x := 0; x < frame.Width(); x++ {
					if at, _ := frame.InkAt(x, y); at != display.InkWhite {
						ink++
					}
				}
			}
			if ink == 0 {
				t.Error("the page drew nothing at all")
			}
		})
	}
}

// Drawn anyway is only half of it. Silently drawn anyway is the other failure,
// and the worse one: an author who is not told has no way to find out except
// by looking very hard at a panel.
func TestWhatCannotBeHonouredIsAlwaysSaid(t *testing.T) {
	const frame = `.page { display: flex; width: 200px; height: 80px; background: white; }`
	tests := map[string]string{
		"Arial":      `.a { font-family: Arial; }`,
		"13px":       `.a { font-size: 13px; }`,
		"blue":       `.a { color: blue; }`,
		"opacity":    `.a { opacity: .5; }`,
		"@media":     `@media print { .a { color: red; } }`,
		"1.2rem":     `.a { font-size: 1.2rem; }`,
		"box-shadow": `.a { box-shadow: 0 1px 2px #000; }`,
	}
	for want, css := range tests {
		t.Run(want, func(t *testing.T) {
			page, err := Compile(`<div class="page"><span class="a">x</span></div>`, frame+css)
			if err != nil {
				t.Fatal(err)
			}
			var said string
			for _, warning := range page.Warnings {
				said += warning.Message + "\n"
			}
			if !strings.Contains(said, want) {
				t.Errorf("nothing said %q; it said %q", want, said)
			}
		})
	}
}
