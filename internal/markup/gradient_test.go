package markup

import (
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func gradientOrFail(t *testing.T, value string) *display.Gradient {
	t.Helper()
	var said []string
	gradient, ok := parseLinearGradient(value, "background", func(message string) { said = append(said, message) })
	if !ok || len(said) != 0 {
		t.Fatalf("%s was refused: %q", value, said)
	}
	return gradient
}

func TestAGradientIsReadInEachWayCSSWritesADirection(t *testing.T) {
	for value, want := range map[string]display.GradientDirection{
		"linear-gradient(white, black)":             display.GradientToBottom,
		"linear-gradient(to right, white, black)":   display.GradientToRight,
		"linear-gradient(TO LEFT, white, black)":    display.GradientToLeft,
		"linear-gradient(to top, white, black)":     display.GradientToTop,
		"linear-gradient(90deg, white, black)":      display.GradientToRight,
		"linear-gradient(180deg, white, black)":     display.GradientToBottom,
		"linear-gradient(-90deg, white, black)":     display.GradientToLeft,
		"linear-gradient(0.25turn, white, black)":   display.GradientToRight,
		"linear-gradient(0deg, white, black)":       display.GradientToTop,
		"linear-gradient(450deg, white, black)":     display.GradientToRight,
		"linear-gradient(to bottom, #fff, #000)":    display.GradientToBottom,
		"linear-gradient( to right , white,black )": display.GradientToRight,
	} {
		if got := gradientOrFail(t, value).Direction; got != want {
			t.Errorf("%s runs to %s, want %s", value, got, want)
		}
	}
}

// Stops a page leaves unplaced are placed the way CSS places them.
func TestStopsArePlacedTheWayCSSPlacesThem(t *testing.T) {
	for value, want := range map[string][]float64{
		"linear-gradient(white, black)":                   {0, 1},
		"linear-gradient(white, gray, black)":             {0, 0.5, 1},
		"linear-gradient(white, silver, gray, black)":     {0, 1.0 / 3, 2.0 / 3, 1},
		"linear-gradient(white 20%, black)":               {0.2, 1},
		"linear-gradient(white, gray 80%, black)":         {0, 0.8, 1},
		"linear-gradient(white, silver, gray 60%, black)": {0, 0.3, 0.6, 1},
		// A stop placed before one ahead of it is moved up to it.
		"linear-gradient(black 50%, white 20%)": {0.5, 0.5},
		// And a stop may sit beyond either edge.
		"linear-gradient(white -50%, black 150%)": {-0.5, 1.5},
		"linear-gradient(black 0, white)":         {0, 1},
	} {
		gradient := gradientOrFail(t, value)
		if len(gradient.Stops) != len(want) {
			t.Fatalf("%s has %d stops", value, len(gradient.Stops))
		}
		for index, stop := range gradient.Stops {
			if diff := stop.At - want[index]; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("%s: stop %d is at %v, want %v", value, index, stop.At, want[index])
			}
		}
	}
}

// What cannot be drawn is refused with a reason, rather than drawn as
// something else.
func TestAGradientThisCannotDrawIsRefusedAndSaysWhy(t *testing.T) {
	for value, reason := range map[string]string{
		"radial-gradient(white, black)":                  "radial-gradient is not supported",
		"repeating-linear-gradient(white, black 10%)":    "repeating-linear-gradient is not supported",
		"linear-gradient(to top right, white, black)":    "not a direction this draws",
		"linear-gradient(45deg, white, black)":           "not a direction this draws",
		"linear-gradient(black, red)":                    "different inks",
		"linear-gradient(white, black 20px)":             "positions are percentages",
		"linear-gradient(white, 30%, black)":             "colour hints are not supported",
		"linear-gradient(white 10% 20%, black)":          "two-position stops",
		"linear-gradient(transparent, black)":            "use white",
		"linear-gradient(black)":                         "at least two colours",
		"linear-gradient(white, #d97757)":                "not one of the panel's inks",
		"linear-gradient(white, black), white":           "a background holds one gradient or one colour",
		"linear-gradient(to right, white, black) repeat": "a background holds one gradient or one colour",
	} {
		var said []string
		if _, ok := parseLinearGradient(value, "background", func(message string) { said = append(said, message) }); ok {
			t.Errorf("%s was accepted", value)
			continue
		}
		if len(said) != 1 || !strings.Contains(said[0], reason) {
			t.Errorf("%s: said %q, want one message about %q", value, said, reason)
		}
	}
}

// background is a shorthand for both the colour and the image, so each
// replaces the other; background-color and background-image each set only
// their own, and the image is painted over the colour.
func TestTheBackgroundPropertiesSetWhatCSSSaysTheySet(t *testing.T) {
	gradient := "linear-gradient(to right, white, black)"
	for css, want := range map[string]struct{ fill, gradient bool }{
		"background: black; background: " + gradient:                  {false, true},
		"background: " + gradient + "; background: black":             {true, false},
		"background-color: black; background-image: " + gradient:      {true, true},
		"background-image: " + gradient + "; background-color: black": {true, true},
		"background: " + gradient + "; background-image: none":        {false, false},
		"background: " + gradient + "; background: none":              {false, false},
	} {
		page, err := Compile(`<div class="page"></div>`, `.page { width: 32px; height: 8px; `+css+` }`)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Warnings) != 0 {
			t.Errorf("%s: warnings %v", css, page.Warnings)
		}
		written := string(page.JSON)
		if got := strings.Contains(written, `"fill"`); got != want.fill {
			t.Errorf("%s: colour drawn %v, want %v", css, got, want.fill)
		}
		if got := strings.Contains(written, `"gradient"`); got != want.gradient {
			t.Errorf("%s: gradient drawn %v, want %v", css, got, want.gradient)
		}
	}
}

// A picture as a background is refused and pointed at the element that draws
// pictures.
func TestABackgroundPictureIsRefused(t *testing.T) {
	page, err := Compile(`<div class="page"></div>`, `.page { width: 8px; height: 8px; background-image: url(a.png); }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0].Message, "<img>") {
		t.Fatalf("warnings: %v", page.Warnings)
	}
}

// A gradient written in CSS reaches the page: compiled, read back and drawn,
// it runs from white to black across the box.
func TestAGradientReachesThePage(t *testing.T) {
	page, err := Compile(`<div class="page"></div>`,
		`.page { width: 64px; height: 8px; background: linear-gradient(to right, white, black); }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 0 {
		t.Fatalf("warnings: %v", page.Warnings)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	share := func(from, to int) int {
		black := 0
		for y := 0; y < 8; y++ {
			for x := from; x < to; x++ {
				if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
					black++
				}
			}
		}
		return black
	}
	if left, right := share(0, 32), share(32, 64); left >= right {
		t.Fatalf("the right half is not the darker: %d then %d black pixels", left, right)
	}
	if first, last := share(0, 1), share(63, 64); first != 0 || last != 8 {
		t.Fatalf("the ends are %d and %d black of 8, want white and black", first, last)
	}
}

// An inline box takes a gradient as a block does.
func TestAnInlineBoxCarriesAGradient(t *testing.T) {
	page, err := Compile(`<div class="page"><span class="tag">x</span></div>`,
		`.page { width: 40px; height: 16px; font-family: ui; font-size: 12px; }
		 .tag { padding: 0 8px; background-image: linear-gradient(to right, white, black); }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Warnings) != 0 || !strings.Contains(string(page.JSON), `"gradient"`) {
		t.Fatalf("warnings %v, document:\n%s", page.Warnings, page.JSON)
	}
	frame, _ := renderDocument(t, "", page.JSON)
	black := 0
	for y := 0; y < 16; y++ {
		if ink, _ := frame.InkAt(0, y); ink == display.InkBlack {
			black++
		}
	}
	if black != 0 {
		t.Fatalf("the span's light end has %d black pixels", black)
	}
}
