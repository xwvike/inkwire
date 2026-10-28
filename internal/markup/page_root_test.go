package markup

import (
	"strings"
	"testing"
)

func TestABodyIsDrawnWholeAsABrowserDrawsIt(t *testing.T) {
	for _, test := range []struct {
		name, html, css string
		want            []string
	}{
		{"a heading and the paragraphs under it",
			`<body><h1>heading</h1><p>first</p><p>second</p></body>`, ``,
			[]string{`"heading"`, `"first"`, `"second"`}},
		{"text written straight into the body",
			`<body>loose <b>bold</b></body>`, ``,
			[]string{`"loose `, `"bold"`}},
		{"a style element before the content",
			`<body><style>p { color: red; }</style><p>words</p></body>`, ``,
			[]string{`"words"`, `"red"`}},
		{"what the page says to the body",
			`<body><div>words</div></body>`, `body { color: red; }`,
			[]string{`"words"`, `"red"`}},
		{"a style attribute on the body",
			`<body style="color: red"><div>words</div></body>`, ``,
			[]string{`"red"`}},
		// A wrapper round the page states its size and orientation, and still
		// does when the body round it is drawn too.
		{"one wrapper",
			`<body><div class="page" orientation="portrait-cw">words</div></body>`, `.page { width: 300px; height: 400px; }`,
			[]string{`"portraitClockwise"`, `"width": 300`}},
		{"one wrapper in a body the page styles",
			`<body><div class="page" orientation="portrait-cw">words</div></body>`, `body { color: red; } .page { width: 300px; height: 400px; }`,
			[]string{`"portraitClockwise"`, `"width": 300`, `"red"`}},
	} {
		page, err := Compile(test.html, test.css)
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
			continue
		}
		for _, want := range test.want {
			if !strings.Contains(string(page.JSON), want) {
				t.Errorf("%s: %s is not in\n%s", test.name, want, page.JSON)
			}
		}
	}
}

func TestAPageWrittenAsOneWrapperIsTheWrapper(t *testing.T) {
	// A reset for every element says nothing about which is the page, so the
	// wrapper is still the root: the page compiles as it does with the same
	// rule written for the wrapper alone.
	const sized = ` .page { width: 200px; height: 100px; padding: 7px; }`
	reset, err := Compile(`<body><div class="page">words</div></body>`, `* { margin: 0; }`+sized)
	if err != nil {
		t.Fatal(err)
	}
	alone, err := Compile(`<body><div class="page">words</div></body>`, `.page { margin: 0; }`+sized)
	if err != nil {
		t.Fatal(err)
	}
	if string(reset.JSON) != string(alone.JSON) {
		t.Errorf("a reset for every element changed the page:\n%s\nwant\n%s", reset.JSON, alone.JSON)
	}
	if _, err := Compile(`<body>  <!-- nothing --> <style>p {}</style></body>`, ``); err == nil {
		t.Error("a body with nothing to draw compiled")
	}
}
