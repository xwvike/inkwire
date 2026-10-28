package markup

import (
	"encoding/json"
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// The marker is drawn in the item's colour and its text in black, so that the
// red on the page is the marker and nothing else.
const markedItems = `.page { display: block; } ul, ol { margin: 0; padding-left: 20px; }
	li { color: red; } span { color: black; }`

func TestAListItemIsMarkedBesideItsFirstLine(t *testing.T) {
	// A bullet a third of the text's height, centred on its lower case and a
	// space before it.
	got := boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems)
	expect(t, got, display.InkBlack, image.Rect(20, 3, 31, 11), "the item's text")
	expect(t, got, display.InkRed, image.Rect(10, 6, 14, 10), "the bullet")

	// The item's own padding moves its text away from the marker, not the
	// marker with it, as in a browser; its top padding moves both down.
	got = boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems+` li { padding: 4px 0 0 10px; }`)
	expect(t, got, display.InkBlack, image.Rect(30, 7, 41, 15), "the padded item's text")
	expect(t, got, display.InkRed, image.Rect(10, 10, 14, 14), "the padded item's bullet")

	// A number ends a space before the text, however wide it is.
	got = boxes(t, `<ol><li><span>ab</span></li></ol>`, markedItems)
	if red := got[display.InkRed]; red.Max.X > 14 || red.Max.X < 12 || red.Min.Y != 3 {
		t.Errorf("the number is at %v; want it to end by 14, a space before the text, on its line", red)
	}

	// A string ends as far before the text as the spaces it ends with.
	got = boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems+` ul { list-style-type: "-  "; }`)
	if red := got[display.InkRed]; red.Max.X > 8 || red.Max.X < 6 {
		t.Errorf("the string marker is at %v; want it to end by 8, two spaces before the text", red)
	}

	// Inside, the marker is the first thing on the line and the text follows.
	got = boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems+` ul { list-style-position: inside; }`)
	expect(t, got, display.InkRed, image.Rect(20, 5, 24, 9), "the bullet inside")
	expect(t, got, display.InkBlack, image.Rect(30, 3, 41, 11), "the text after it")

	// Each shape is the bullet's size and place.
	for _, kind := range []string{"circle", "square"} {
		got = boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems+` ul { list-style-type: `+kind+`; }`)
		expect(t, got, display.InkRed, image.Rect(10, 6, 14, 10), kind)
	}
}

func TestAnItemShowsNoMarkerWhenThePageSaysNone(t *testing.T) {
	for _, css := range []string{
		`ul { list-style: none; }`,
		`li { list-style-type: none; }`,
		`li { display: block; }`,
		`li { display: flex; }`,
		`ul { list-style: none outside; }`,
	} {
		if got := boxes(t, `<ul><li><span>ab</span></li></ul>`, markedItems+css); !got[display.InkRed].Empty() {
			t.Errorf("%s: a marker was drawn at %v", css, got[display.InkRed])
		}
	}
	// And any element can ask for one.
	if got := boxes(t, `<div class="d"><span>ab</span></div>`, markedItems+` .d { display: list-item; margin-left: 20px; color: red; }`); got[display.InkRed].Empty() {
		t.Error("display: list-item drew no marker")
	}
}

// markers are the text markers a page's items were given, in order.
func markers(t *testing.T, body, css string) []string {
	t.Helper()
	page, err := Compile(`<div class="page">`+body+`</div>`, `.page { width: 300px; height: 300px; } `+css)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range page.Warnings {
		t.Logf("  warning: %s", warning.Message)
	}
	var document any
	if err := json.Unmarshal(page.JSON, &document); err != nil {
		t.Fatal(err)
	}
	var found []string
	var walk func(value any, marker bool)
	walk = func(value any, marker bool) {
		switch value := value.(type) {
		case map[string]any:
			if marker {
				if runs, ok := value["runs"].([]any); ok && len(runs) > 0 {
					found = append(found, runs[0].(map[string]any)["text"].(string))
				}
				switch {
				case value["type"] == "ellipse" && value["fill"] != nil:
					found = append(found, "disc")
				case value["type"] == "ellipse":
					found = append(found, "circle")
				case value["type"] == "rectangle":
					found = append(found, "square")
				case value["type"] == "polygon":
					found = append(found, "triangle")
				}
			}
			for key, child := range value {
				walk(child, key == "children" && value["type"] == "anchored" || marker && key == "node")
			}
		case []any:
			for _, child := range value {
				walk(child, marker)
			}
		}
	}
	walk(document, false)
	return found
}

func TestAListCountsAsHTMLSays(t *testing.T) {
	items := `<li>a</li><li>b</li><li>c</li>`
	for _, test := range []struct {
		body, css string
		want      []string
	}{
		{`<ol>` + items + `</ol>`, ``, []string{"1.", "2.", "3."}},
		{`<ol start="7">` + items + `</ol>`, ``, []string{"7.", "8.", "9."}},
		{`<ol reversed>` + items + `</ol>`, ``, []string{"3.", "2.", "1."}},
		{`<ol reversed start="10">` + items + `</ol>`, ``, []string{"10.", "9.", "8."}},
		{`<ol><li>a</li><li value="5">b</li><li>c</li></ol>`, ``, []string{"1.", "5.", "6."}},
		{`<ol type="A">` + items + `</ol>`, ``, []string{"A.", "B.", "C."}},
		{`<ol type="i">` + items + `</ol>`, ``, []string{"i.", "ii.", "iii."}},
		{`<ol><li>a</li><li type="a">b</li></ol>`, ``, []string{"1.", "b."}},
		// The page's rules outrank the attribute, whatever their selector.
		{`<ol type="A">` + items + `</ol>`, `ol { list-style-type: decimal; }`, []string{"1.", "2.", "3."}},
		// An item that is not drawn is not counted.
		{`<ol><li>a</li><li style="display: none">b</li><li>c</li></ol>`, ``, []string{"1.", "2."}},
		{`<ol>` + items + `</ol>`, `ol { list-style-type: "-  "; }`, []string{"-", "-", "-"}},
		{`<ol>` + items + `</ol>`, `ol { list-style-type: cjk-decimal; }`, []string{"一、", "二、", "三、"}},
		{`<ol>` + items + `</ol>`, `ol { list-style: upper-roman inside; }`, []string{}},
		// A list in a list is marked with a circle, and one in that with a
		// square. An item's marker comes after what is in it.
		{`<ul><li>a<ul><li>b<ul><li>c</li></ul></li></ul></li></ul>`, ``, []string{"square", "circle", "disc"}},
		{`<ul><li>a</li></ul>`, `ul { list-style-type: disclosure-closed; }`, []string{"triangle"}},
	} {
		got := markers(t, test.body, test.css)
		if strings.Join(got, " ") != strings.Join(test.want, " ") {
			t.Errorf("%s with %q: markers %q, want %q", test.body, test.css, got, test.want)
		}
	}
	// The marker is not the text, and the text's lines are not drawn through
	// it, as they are not in a browser.
	runs := runsIn(t, `<ol><li>a</li></ol>`, `li { text-decoration: underline; }`)
	if runs["1."]["decoration"] != nil || runs["a"]["decoration"] != "underline" {
		t.Errorf("the marker was decorated with its item: %v", runs)
	}
	// Inside, the marker is the start of the line, and its space with it.
	runs = runsIn(t, `<ol><li>a</li></ol>`, `ol { list-style: upper-roman inside; }`)
	if runs["I."]["text"] != "I. " {
		t.Errorf("the inside marker was not the start of the line: %v", runs)
	}
}

func TestEachCounterStyleWritesNumbersAsCSSDoes(t *testing.T) {
	for _, test := range []struct {
		kind    string
		ordinal int
		want    string
	}{
		{"decimal", 12, "12. "},
		{"decimal", -2, "-2. "},
		{"decimal-leading-zero", 7, "07. "},
		{"decimal-leading-zero", -7, "-07. "},
		{"decimal-leading-zero", 12, "12. "},
		{"lower-alpha", 1, "a. "},
		{"lower-alpha", 26, "z. "},
		{"lower-alpha", 27, "aa. "},
		{"lower-alpha", 52, "az. "},
		{"lower-alpha", 0, "0. "},
		{"upper-latin", 28, "AB. "},
		{"lower-greek", 2, "β. "},
		{"lower-roman", 4, "iv. "},
		{"upper-roman", 1994, "MCMXCIV. "},
		{"upper-roman", 4000, "4000. "},
		{"cjk-decimal", 105, "一〇五、"},
		{"simp-chinese-informal", 10, "十、"},
		{"simp-chinese-informal", 15, "十五、"},
		{"simp-chinese-informal", 20, "二十、"},
		{"simp-chinese-informal", 101, "一百零一、"},
		{"simp-chinese-informal", 110, "一百一十、"},
		{"simp-chinese-informal", 1010, "一千零一十、"},
		{"cjk-ideographic", 3, "三、"},
		{"simp-chinese-informal", 10000, "10000. "},
	} {
		if got := markerText(test.kind, test.ordinal); got != test.want {
			t.Errorf("%s %d: %q, want %q", test.kind, test.ordinal, got, test.want)
		}
	}
}

func TestListStyleIsReadAsCSSReadsIt(t *testing.T) {
	for _, test := range []struct {
		css, said string
	}{
		{`ul { list-style-type: hebrew; }`, "list-style-type: hebrew is not a counter style this has; drawn as decimal"},
		{`ul { list-style: url(dot.png) square; }`, "list-style: url(dot.png) is not drawn; the marker list-style-type names is drawn instead"},
		{`ul { list-style-position: middle; }`, "is not one of inside or outside"},
		{`ul { list-style: inside; } li { display: block; } li { display: list-item; }`, ""},
		{`ul { list-style: inside; }`, ""},
	} {
		said := warningsFor(t, `<ul><li>a</li></ul>`, test.css)
		if test.said == "" && said != "" || !strings.Contains(said, test.said) {
			t.Errorf("%s said %q; want %q", test.css, said, test.said)
		}
	}
	// Inside wants a line of text to go first on, and says so when the item
	// starts with a box instead.
	if said := warningsFor(t, `<ul><li><p>a</p></li></ul>`, `ul { list-style: inside; }`); !strings.Contains(said, "the marker is drawn outside") {
		t.Errorf("an inside marker with no line for it said %q", said)
	}
	for _, test := range []struct {
		value, kind, text string
		inside            bool
	}{
		{`none`, "none", "", false},
		{`square inside`, "square", "", true},
		{`inside square`, "square", "", true},
		{`"→ " outside`, "string", "→ ", false},
		{`'a b' inside`, "string", "a b", true},
		{`"a\"b"`, "string", `a"b`, false},
		{`none url(x.png)`, "none", "", false},
		{`url(x.png) none`, "none", "", false},
		{`lower-roman none`, "lower-roman", "", false},
		{`DECIMAL`, "decimal", "", false},
		{`disc`, "", "", false},
	} {
		var s style
		s.applyListStyle(test.value, func(string) {})
		if s.listType != test.kind || s.listText != test.text || s.listInside != test.inside {
			t.Errorf("list-style: %s read as %q %q %v; want %q %q %v", test.value,
				s.listType, s.listText, s.listInside, test.kind, test.text, test.inside)
		}
	}
}
