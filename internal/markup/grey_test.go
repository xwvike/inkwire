package markup

import (
	"os"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestAGreyIsBlackAtTheNearestSixteenth(t *testing.T) {
	for value, want := range map[string]display.Ink{
		"gray":                    display.Tone(display.InkBlack, 8), // 0x80
		"GREY":                    display.Tone(display.InkBlack, 8),
		"#888":                    display.Tone(display.InkBlack, 7), // 0x88
		"#808080":                 display.Tone(display.InkBlack, 8),
		"silver":                  display.Tone(display.InkBlack, 4), // 0xc0
		"#111111":                 display.Tone(display.InkBlack, 15),
		"#fefefe":                 display.InkWhite,                  // rounds to none of it
		"#010101":                 display.InkBlack,                  // rounds to all of it
		"whitesmoke":              display.Tone(display.InkBlack, 1), // 0xf5
		"#808080ff":               display.Tone(display.InkBlack, 8),
		"#888f":                   display.Tone(display.InkBlack, 7),
		"rgb(128, 128, 128)":      display.Tone(display.InkBlack, 8),
		"RGB(128 128 128)":        display.Tone(display.InkBlack, 8),
		"rgb(50% 50% 50%)":        display.Tone(display.InkBlack, 8),
		"rgba(128, 128, 128, 1)":  display.Tone(display.InkBlack, 8),
		"rgb(128 128 128 / 100%)": display.Tone(display.InkBlack, 8),
		"rgb(255, 0, 0)":          display.InkRed,
		"rgb(255 255 0)":          display.InkYellow,
		"rgb(0 0 0)":              display.InkBlack,
		"rgb(300 300 300)":        display.InkWhite, // clamped, as CSS clamps
		"rgb( 255 , 255 , 255 )":  display.InkWhite,
	} {
		var said []string
		got, ok := parseInk(value, "background", func(message string) { said = append(said, message) })
		if !ok || got != want || len(said) != 0 {
			t.Errorf("%s is %s (ok %v, said %q), want %s", value, got, ok, said, want)
		}
	}
	// A colour that is not a grey is still not approximated.
	for _, value := range []string{"#d97757", "pink", "rgb(217, 119, 87)", "rgb(1 2)", "rgb(a b c)", "hsl(0 0% 50%)"} {
		var said []string
		if got, ok := parseInk(value, "background", func(message string) { said = append(said, message) }); ok || len(said) != 1 {
			t.Errorf("%s was read as %s (ok %v, said %q)", value, got, ok, said)
		}
	}
}

// Text cannot be grey, and a page that asks for it is told so and gets black.
func TestGreyTextIsDrawnBlackAndSaid(t *testing.T) {
	page, err := Compile(`<div class="page">word</div>`,
		`.page { width: 40px; height: 16px; font-family: ui; font-size: 12px; color: gray; }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0].Message, "drawn black") {
		t.Fatalf("warnings: %v", page.Warnings)
	}
	if strings.Contains(string(page.JSON), "/16") {
		t.Fatalf("a grey reached the text:\n%s", page.JSON)
	}
}

// A grey background compiles to a tone, is read back, and is drawn as black
// and white with the mid grey's share of black.
func TestAGreyBackgroundIsDitheredOnThePage(t *testing.T) {
	page, err := Compile(`<div class="page"></div>`,
		`.page { width: 16px; height: 16px; background: gray; }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 0 || !strings.Contains(string(page.JSON), `"black 8/16"`) {
		t.Fatalf("warnings %v, document:\n%s", page.Warnings, page.JSON)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	black := 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				black++
			}
		}
	}
	if black != 128 {
		t.Fatalf("%d of 256 pixels are black, want half", black)
	}
}

// Every grey the parser knows by name is one the references list.
func TestMarkupDocsNameEveryGrey(t *testing.T) {
	for _, path := range markupDocs {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for name := range greyNames {
			if !strings.Contains(string(source), "`"+name+"`") {
				t.Errorf("%s does not name the grey %s, which readInk accepts", path, name)
			}
		}
	}
}

// A grey with a tint is a grey to anyone reading the page: drawn as the grey
// of its lightness, and said, because the tint is lost.
func TestATintedGreyIsDrawnAsAGreyAndSaid(t *testing.T) {
	for value, want := range map[string]display.Ink{
		"#6b7280":            display.Tone(display.InkBlack, 9), // Tailwind gray-500
		"#64748b":            display.Tone(display.InkBlack, 9), // slate-500, the widest spread
		"#808081":            display.Tone(display.InkBlack, 8),
		"rgb(107, 114, 128)": display.Tone(display.InkBlack, 9),
		"#f8fafc":            display.InkWhite, // slate-50, too light for any black
	} {
		var said []string
		got, ok := parseInk(value, "background", func(message string) { said = append(said, message) })
		if !ok || got != want {
			t.Errorf("%s is %s (ok %v), want %s", value, got, ok, want)
		}
		if len(said) != 1 || !strings.Contains(said[0], "tinted grey") {
			t.Errorf("%s: said %q, want one message about a tinted grey", value, said)
		}
	}
}

// What a panel cannot show — a colour seen through — is refused, and says why.
func TestASeeThroughColourIsRefused(t *testing.T) {
	for _, value := range []string{"#8888", "#80808080", "rgba(0, 0, 0, 0.5)", "rgb(0 0 0 / 50%)"} {
		var said []string
		if _, ok := parseInk(value, "background", func(message string) { said = append(said, message) }); ok ||
			len(said) != 1 || !strings.Contains(said[0], "see-through") {
			t.Errorf("%s: ok %v, said %q", value, ok, said)
		}
	}
}

// A colour written as rgb() survives the places that split a value at its
// spaces, and a tinted grey says so once wherever it is written.
func TestFunctionalColoursWorkWhereverAColourIs(t *testing.T) {
	page, err := Compile(`<div class="page"><p class="a">x</p><p class="b">y</p></div>`,
		`.page { width: 64px; height: 40px; font-family: ui; font-size: 12px;
		         background: linear-gradient(to right, rgb(255 255 255), rgb(0, 0, 0) 80%); }
		 .a { border: 2px solid rgb(128, 128, 128); }
		 .b { color: #6b7280; }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0].Message, "color: #6b7280 is a tinted grey, which text cannot be drawn in") {
		t.Fatalf("warnings: %v", page.Warnings)
	}
	written := string(page.JSON)
	if !strings.Contains(written, `"black 8/16"`) || !strings.Contains(written, `"gradient"`) {
		t.Fatalf("document:\n%s", written)
	}

	// A tinted grey near enough to white is white text, not black.
	page, err = Compile(`<div class="page">x</div>`,
		`.page { width: 16px; height: 16px; font-family: ui; font-size: 12px; background: black; color: #f8fafc; }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0].Message, "drawn as the grey of its lightness, white") {
		t.Fatalf("near-white tinted text: warnings %v", page.Warnings)
	}
	if !strings.Contains(string(page.JSON), `"ink": "white"`) {
		t.Fatalf("near-white tinted text is not white:\n%s", page.JSON)
	}

	page, err = Compile(`<div class="page"></div>`,
		`.page { width: 8px; height: 8px; background: linear-gradient(white, #6b7280); }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0].Message, "tinted grey") ||
		!strings.Contains(string(page.JSON), `"gradient"`) {
		t.Fatalf("a tinted grey in a gradient: warnings %v", page.Warnings)
	}
}
