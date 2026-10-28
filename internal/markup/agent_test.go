package markup

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"golang.org/x/net/html"
)

func TestElementsLookAsABrowsersOwnStylesheetDrawsThem(t *testing.T) {
	tests := []struct {
		body, css, text, field string
		want                   any
	}{
		// Headings are bold, and as large as the nearest strike to what a
		// browser makes them: twice, one and a half, and 1.17 times the 12
		// pixels of the page, and no smaller than its smallest — which, as
		// the page's own size, a run does not write down.
		{`<h1>word</h1>`, ``, "word", "size", 24.0},
		{`<h1>word</h1>`, ``, "word", "bold", true},
		{`<h2>word</h2>`, ``, "word", "size", 16.0},
		{`<h3>word</h3>`, ``, "word", "size", 14.0},
		{`<h4>word</h4>`, ``, "word", "size", nil},
		{`<h6>word</h6>`, ``, "word", "size", nil},
		{`<h4>word</h4>`, ``, "word", "bold", true},
		// The size is the heading's own em, so what is inside it is measured
		// from it, and a page that states a root size scales them all.
		{`<h2><span>word</span></h2>`, `span { font-size: 1.5em; }`, "word", "size", 24.0},
		{`<h1>word</h1>`, `.page { font-size: 16px; }`, "word", "size", 32.0},
		{`<table><tr><th>word</th></tr></table>`, `table { display: block; }`, "word", "bold", true},
		{`<a href="/">word</a>`, ``, "word", "decoration", "underline"},
		{`<a>word</a>`, ``, "word", "decoration", nil},
		// Whatever the page says comes after, and wins, whatever its selector.
		{`<h1>word</h1>`, `* { font-size: 14px; }`, "word", "size", 14.0},
		{`<h1>word</h1>`, `* { font-weight: normal; }`, "word", "bold", nil},
		{`<a href="/">word</a>`, `* { text-decoration: none; }`, "word", "decoration", nil},
	}
	for _, test := range tests {
		runs := runsIn(t, test.body, test.css)
		if got := runs[test.text][test.field]; got != test.want {
			t.Errorf("%s with %q: %s is %v; want %v", test.body, test.css, test.field, got, test.want)
		}
	}
}

func TestBlocksAreSpacedAsABrowsersOwnStylesheetSpacesThem(t *testing.T) {
	const frame = `.page { display: block; } .a { background: black; height: 5px; } .b { background: red; height: 5px; }`
	// Paragraphs are a line apart, the 12 pixels of the font, and the two
	// margins between them are one.
	got := boxes(t, `<p class="a"></p><p class="b"></p>`, frame)
	expect(t, got, display.InkBlack, image.Rect(0, 12, 100, 17), "the first paragraph")
	expect(t, got, display.InkRed, image.Rect(0, 29, 100, 34), "the second paragraph")
	// A heading's margins are its own ems, at the size it is drawn: 0.83 of
	// the 16 an h2 settles on, rather than of the 18 it was asked to be.
	got = boxes(t, `<h2 class="a"></h2>`, frame)
	expect(t, got, display.InkBlack, image.Rect(0, 13, 100, 18), "the h2")
	// A rule the page writes outranks the default, however general.
	got = boxes(t, `<p class="a"></p><p class="b"></p>`, frame+` * { margin: 0; }`)
	expect(t, got, display.InkRed, image.Rect(0, 5, 100, 10), "the second paragraph with no margins")
	// A list is indented, and a figure and a quotation inset on both sides.
	got = boxes(t, `<ul class="a"><li style="list-style: none" class="b"></li></ul>`, frame+` ul { margin: 0; }`)
	expect(t, got, display.InkRed, image.Rect(40, 0, 100, 5), "the item in its list")
	got = boxes(t, `<figure class="a"></figure>`, frame)
	expect(t, got, display.InkBlack, image.Rect(40, 12, 60, 17), "the figure")
	// A list in a list is not spaced from its item.
	got = boxes(t, `<ul style="margin: 0"><li style="list-style: none"><ul class="a"></ul></li></ul>`, frame)
	expect(t, got, display.InkBlack, image.Rect(40, 0, 100, 5), "the inner list")
	// A rule is a line, in the colour of the text.
	got = boxes(t, `<hr style="color: red">`, frame)
	expect(t, got, display.InkRed, image.Rect(0, 6, 100, 7), "the rule")
}

func TestTheDefaultStylesheetSaysNothingAboutAPageThatUsesIt(t *testing.T) {
	// Every rule in it, applied to every element it names, with nothing for
	// the page to be told: the page did not write any of it.
	var body strings.Builder
	for _, name := range []string{"h1", "h2", "h3", "h4", "h5", "h6", "p", "blockquote", "figure",
		"dl", "dd", "b", "strong", "th", "caption", "center", "small", "u", "ins", "s", "strike", "del", "pre"} {
		body.WriteString("<" + name + ">x</" + name + ">")
	}
	body.WriteString(`<a href="/">x</a><hr><ul><li>x<ul><li>x<ul><li>x</li></ul></li></ul></li></ul><ol type="a"><li>x<ol><li>x</li></ol></li></ol><menu><li>x</li></menu>`)
	if said := warningsFor(t, body.String(), `.page { display: block; height: auto; }`); said != "" {
		t.Errorf("the default stylesheet said:\n%s", said)
	}
	// A size the page did write is the page's, and is still said.
	if said := warningsFor(t, `<h2>x</h2>`, `h2 { font-size: 1.5em; }`); !strings.Contains(said, "font-size: 18px has no strike") {
		t.Errorf("a size the page wrote said %q", said)
	}
	// It is the page's HTML it is for: an SVG's a is not a link to underline.
	parsed, err := html.Parse(strings.NewReader(`<svg><a href="/"><text>x</text></a></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	var link *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			link = node
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(parsed)
	if link == nil || len(agentDeclarations(link)) != 0 {
		t.Errorf("an SVG a was given the default stylesheet's rules: %v", agentDeclarations(link))
	}
}
