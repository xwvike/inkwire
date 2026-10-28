package markup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// runsIn is every run a page compiled to, keyed by its text.
func runsIn(t *testing.T, body, css string) map[string]map[string]any {
	t.Helper()
	page, err := Compile(`<div class="page">`+body+`</div>`, `.page { width: 200px; height: 60px; } `+css)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(page.JSON, &document); err != nil {
		t.Fatal(err)
	}
	found := map[string]map[string]any{}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if runs, ok := value["runs"].([]any); ok {
				for _, entry := range runs {
					written := entry.(map[string]any)
					found[strings.TrimSpace(written["text"].(string))] = written
				}
			}
			for _, inner := range value {
				walk(inner)
			}
		case []any:
			for _, inner := range value {
				walk(inner)
			}
		}
	}
	walk(document)
	return found
}

func TestFontWeightDrawsBold(t *testing.T) {
	for css, bold := range map[string]bool{
		`.a { font-weight: bold; }`:                                               true,
		`.a { font-weight: 600; }`:                                                true,
		`.a { font-weight: 500; }`:                                                false,
		`.a { font-weight: bolder; }`:                                             true,
		`.a { font-weight: bold; font-weight: normal; }`:                          false,
		`.a { font: bold 14px monaco; }`:                                          true,
		`.a { font-weight: bold; font: 14px monaco; }`:                            false,
		`.page { font-weight: 700; }`:                                             true,
		`.page { font-weight: 700; } .a { font-weight: inherit; }`:                true,
		`.page { font: 700 12px ui; } .a { font-weight: normal; font: inherit; }`: true,
	} {
		runs := runsIn(t, `<div class="a">word</div>`, css)
		if got, _ := runs["word"]["bold"].(bool); got != bold {
			t.Errorf("%s: bold is %v; want %v", css, got, bold)
		}
	}
	plain, said := renderProbe(t, `<div class="a">word</div>`, `.page { width: 100px; }`)
	bold, _ := renderProbe(t, `<div class="a">word</div>`, `.page { width: 100px; } .a { font-weight: bold; }`)
	if plain == nil || blackPixels(plain) >= blackPixels(bold) {
		t.Errorf("bold text is drawn with no more ink than plain text: %s", said)
	}
}

func blackPixels(frame *display.Frame) int {
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				count++
			}
		}
	}
	return count
}

func TestLetterSpacingIsWrittenInPixels(t *testing.T) {
	for css, spacing := range map[string]float64{
		`.a { letter-spacing: 2px; }`:                         2,
		`.a { letter-spacing: 0.15em; }`:                      2,
		`.a { letter-spacing: -1px; }`:                        -1,
		`.a { letter-spacing: 2px; letter-spacing: normal; }`: 0,
		`.page { letter-spacing: 3px; }`:                      3,
	} {
		runs := runsIn(t, `<div class="a">word</div>`, css)
		if got, _ := runs["word"]["letterSpacing"].(float64); got != spacing {
			t.Errorf("%s: letter-spacing is %v; want %v", css, got, spacing)
		}
	}
}

func TestADecorationRunsThroughWhatIsInside(t *testing.T) {
	tests := []struct {
		name, body, css string
		want            map[string]string
	}{
		{"its own text", `<div class="a">own</div>`, `.a { text-decoration: underline; }`,
			map[string]string{"own": "underline"}},
		{"a child's text", `<div class="a">own <span class="b">child</span></div>`, `.a { text-decoration: underline; }`,
			map[string]string{"own": "underline", "child": "underline"}},
		{"a grandchild's text", `<div class="a">own <span class="b"><i>grandchild</i></span></div>`, `.a { text-decoration: underline; }`,
			map[string]string{"own": "underline", "grandchild": "underline"}},
		{"a child cannot take it off", `<div class="a">own <span class="b">child</span></div>`,
			`.a { text-decoration: underline; } .b { text-decoration: none; }`,
			map[string]string{"own": "underline", "child": "underline"}},
		{"a child adds its own", `<div class="a">own <span class="b">child</span></div>`,
			`.a { text-decoration: underline; } .b { text-decoration: line-through; }`,
			map[string]string{"own": "underline", "child": "underline line-through"}},
		{"an inline-block is left out", `<div class="a">own <span class="b">child</span></div>`,
			`.a { text-decoration: underline; } .b { display: inline-block; }`,
			map[string]string{"own": "underline", "child": ""}},
		{"a positioned box is left out", `<div class="a">own <span class="b">child</span></div>`,
			`.a { position: relative; text-decoration: underline; } .b { position: absolute; }`,
			map[string]string{"own": "underline", "child": ""}},
		{"the longhand", `<div class="a">own</div>`, `.a { text-decoration-line: overline underline; }`,
			map[string]string{"own": "underline overline"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runs := runsIn(t, test.body, test.css)
			for text, want := range test.want {
				if got, _ := runs[text]["decoration"].(string); got != want {
					t.Errorf("%q is decorated %q; want %q", text, got, want)
				}
			}
		})
	}
	for css, want := range map[string]string{
		`.a { text-decoration: underline wavy; }`: "wavy is not drawn",
		`.a { text-decoration: underline red; }`:  "drawn in the colour of its text",
		`.a { text-decoration: underline 3px; }`:  "is not drawn",
		`.a { text-decoration-line: squiggle; }`:  "is not one of none, underline",
		`.a { text-decoration-style: dotted; }`:   "not drawn",
		`.a { font-style: italic; }`:              "upright only",
		`.a { font: italic bold 12px ui; }`:       "upright only",
		`.a { font-weight: heavy; }`:              "is not normal, bold",
		`.a { letter-spacing: 10%; }`:             "cannot be a share",
	} {
		if said := warningsFor(t, `<div class="a">x</div>`, css); !strings.Contains(said, want) {
			t.Errorf("%s said %q; want %q", css, said, want)
		}
	}
}

func TestAnEllipsisNeedsWhatABrowserNeeds(t *testing.T) {
	const clipped = `.a { width: 40px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }`
	page, err := Compile(`<div class="page"><div class="a">a long line of text</div></div>`,
		`.page { width: 200px; height: 60px; } `+clipped)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page.JSON), `"ellipsis": true`) || len(page.Warnings) != 0 {
		t.Errorf("a clipped line was not given an ellipsis: %v\n%s", page.Warnings, page.JSON)
	}
	for body, want := range map[string]string{
		`<div class="b">a long line of text</div>`:                      "no effect without overflow: hidden",
		`<div class="a">a long <b class="p">line</b> of text</div>`:     "with no elements inside it",
		`<div class="a"><div>a long line</div><div>of text</div></div>`: "put it on the box the text is in",
	} {
		said := warningsFor(t, body, clipped+` .b { width: 40px; white-space: nowrap; text-overflow: ellipsis; } .p { padding: 0 2px; }`)
		if !strings.Contains(said, want) {
			t.Errorf("%s said %q; want %q", body, said, want)
		}
	}
	unclipped, err := Compile(`<div class="page"><div class="b">a long line of text</div></div>`,
		`.page { width: 200px; height: 60px; } .b { width: 40px; white-space: nowrap; text-overflow: ellipsis; }`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unclipped.JSON), `"ellipsis"`) {
		t.Error("a line that is not clipped was given an ellipsis")
	}
	// What is drawn ends in the ellipsis rather than in a letter cut in half,
	// and so nothing is reported cut off.
	_, said := renderProbe(t, `<div class="a">a long line of text</div>`, clipped)
	if strings.Contains(said, "cut off") {
		t.Errorf("an ellipsised line was reported cut off: %s", said)
	}
}
