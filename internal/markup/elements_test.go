package markup

import (
	"strings"
	"testing"
)

func TestElementsLookAsTheirNamesSay(t *testing.T) {
	tests := []struct {
		body, css, field string
		want             any
	}{
		{`<b>word</b>`, ``, "bold", true},
		{`<strong>word</strong>`, ``, "bold", true},
		{`<b>word</b>`, `b { font-weight: normal; }`, "bold", nil},
		{`<b>word</b>`, `b { font: 12px ui; }`, "bold", nil},
		{`<u>word</u>`, ``, "decoration", "underline"},
		{`<ins>word</ins>`, ``, "decoration", "underline"},
		{`<s>word</s>`, ``, "decoration", "line-through"},
		{`<del>word</del>`, ``, "decoration", "line-through"},
		{`<del>word</del>`, `del { text-decoration: none; }`, "decoration", nil},
		{`<span>word</span>`, ``, "bold", nil},
	}
	for _, test := range tests {
		runs := runsIn(t, test.body, test.css)
		if got := runs["word"][test.field]; got != test.want {
			t.Errorf("%s with %q: %s is %v; want %v", test.body, test.css, test.field, got, test.want)
		}
	}
}

func TestWhatABrowserDrawsForAnElementAndThisDoesNotIsSaid(t *testing.T) {
	for _, test := range []struct{ body, css, want string }{
		{`<input value="x">`, ``, "input is not drawn"},
		{`<select><option>a</option></select>`, ``, "select is not drawn"},
		{`<canvas></canvas>`, ``, "canvas is not drawn"},
		{`<button>ok</button>`, ``, "button's border and background"},
		{`<table><tr><td>a</td></tr></table>`, ``, "table layout is not supported"},
		{`<em>word</em>`, ``, "sets em in italics"},
		{`<p>a <i>word</i></p>`, ``, "sets i in italics"},
		{`<ul><li style="list-style: url(dot.png)">a</li></ul>`, ``, "list-style: url(dot.png) is not drawn"},
		{`<ul><li>a</li></ul>`, `li { list-style-image: url(dot.png); }`, "list-style-image: url(dot.png) is not drawn"},
		{`<ul><li>a</li></ul>`, `li { list-style-position: middle; }`, "is not one of inside or outside"},
	} {
		if said := warningsFor(t, test.body, test.css); !strings.Contains(said, test.want) {
			t.Errorf("%s with %q said %q; want %q", test.body, test.css, said, test.want)
		}
	}
	// Each is quiet once the page says what it wants instead.
	for _, test := range []struct{ body, css string }{
		{`<input value="x">`, `input { display: none; }`},
		{`<button>ok</button>`, `button { border: 1px solid black; }`},
		{`<button>ok</button>`, `button { background: black; color: white; }`},
		{`<hr>`, ``},
		{`<ul><li>a</li><li>b</li></ul><ol><li>c</li></ol>`, ``},
		{`<table><tr><td>a</td></tr></table>`, `table { display: grid; } tr, tbody { display: contents; }`},
		{`<ul><li>a</li></ul>`, `ul { list-style: none; }`},
		{`<ul><li>a</li></ul>`, `li { list-style-type: none; }`},
		{`<ul><li>a</li></ul>`, `ul { list-style: none; } li { list-style: inherit; }`},
		{`<ul><li>a</li></ul>`, `ul { list-style: none outside; }`},
		{`<ul><li>a</li></ul>`, `li { display: inline-block; }`},
		{`<em>word</em>`, `em { font-style: normal; }`},
		{`<i class="icon"></i>`, ``},
		{`<p><b>bold</b> <s>gone</s></p>`, ``},
	} {
		if said := warningsFor(t, test.body, test.css); said != "" {
			t.Errorf("%s with %q said %q", test.body, test.css, said)
		}
	}
	// A page of emphasis says so once.
	body := `<address>a</address><address>b</address><address>c</address>`
	if said := warningsFor(t, body, ``); strings.Count(said, "\n") != 1 {
		t.Errorf("%s said %q; want it said once", body, said)
	}
}
