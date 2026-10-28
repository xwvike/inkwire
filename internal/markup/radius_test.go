package markup

import (
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func inkAt(t *testing.T, frame *display.Frame, x, y int) bool {
	t.Helper()
	ink, _ := frame.InkAt(x, y)
	return ink == display.InkBlack
}

func TestAPercentageRadiusIsAShareOfTheBox(t *testing.T) {
	const page = `.page { width: 100px; height: 40px; align-items: flex-start; } `
	t.Run("half of a square is a circle", func(t *testing.T) {
		frame, said := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 20px; height: 20px; background: black; border-radius: 50%; }`)
		if frame == nil {
			t.Fatal(said)
		}
		if inkAt(t, frame, 1, 1) || inkAt(t, frame, 18, 18) || !inkAt(t, frame, 10, 0) || !inkAt(t, frame, 0, 10) {
			t.Error("a square with border-radius: 50% is not drawn as a circle")
		}
		if said != "" {
			t.Errorf("a circle said %q", said)
		}
	})
	t.Run("half of an oblong is an ellipse", func(t *testing.T) {
		frame, said := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 40px; height: 20px; background: black; border: 1px solid black; box-sizing: border-box; border-radius: 50%; }`)
		if frame == nil {
			t.Fatal(said)
		}
		// Inside the round end a stadium would have, outside the ellipse.
		if inkAt(t, frame, 4, 2) {
			t.Error("an oblong with border-radius: 50% is drawn with round ends, not as an ellipse")
		}
		if !inkAt(t, frame, 20, 0) || !inkAt(t, frame, 0, 10) || !inkAt(t, frame, 20, 10) {
			t.Error("the ellipse does not reach the middle of each edge")
		}
		if said != "" {
			t.Errorf("an ellipse said %q", said)
		}
	})
	t.Run("a smaller share of a square is round", func(t *testing.T) {
		percent, said := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 20px; height: 20px; background: black; border-radius: 25%; }`)
		pixels, _ := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 20px; height: 20px; background: black; border-radius: 5px; }`)
		if percent == nil || !samePixels(percent, pixels) {
			t.Errorf("25%% of a 20px square is not drawn as a 5px radius: %s", said)
		}
	})
	t.Run("a smaller share of an oblong is said", func(t *testing.T) {
		_, said := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 40px; height: 20px; background: black; border-radius: 25%; }`)
		if !strings.Contains(said, "10px across and 5px down") {
			t.Errorf("elliptical corners drawn round were not said: %q", said)
		}
		percent, _ := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 40px; height: 20px; background: black; border-radius: 25%; }`)
		pixels, _ := renderProbe(t, `<div class="a"></div>`,
			page+`.a { width: 40px; height: 20px; background: black; border-radius: 5px; }`)
		if percent == nil || !samePixels(percent, pixels) {
			t.Error("25% of a 40x20 box is not drawn round at the smaller 5px")
		}
	})
	t.Run("a clip follows the corners", func(t *testing.T) {
		frame, said := renderProbe(t, `<div class="a"><div class="b"></div></div>`,
			page+`.a { width: 20px; height: 20px; overflow: hidden; border-radius: 50%; } .b { width: 20px; height: 20px; background: black; }`)
		if frame == nil {
			t.Fatal(said)
		}
		if inkAt(t, frame, 1, 1) || !inkAt(t, frame, 10, 10) {
			t.Error("overflow: hidden with border-radius: 50% does not clip to the circle")
		}
		frame, said = renderProbe(t, `<div class="a"><div class="b"></div></div>`,
			page+`.a { width: 40px; height: 20px; overflow: hidden; border-radius: 50%; } .b { width: 40px; height: 20px; background: black; }`)
		if frame == nil {
			t.Fatal(said)
		}
		if inkAt(t, frame, 4, 2) || !inkAt(t, frame, 20, 10) {
			t.Error("overflow: hidden with border-radius: 50% does not clip an oblong to its ellipse")
		}
	})
}

func TestRadiiThisCannotDrawAreSaid(t *testing.T) {
	for css, want := range map[string]string{
		`.a { border-radius: 4px 4px 0 0; }`: "gives corners different radii",
		`.a { border-radius: 10px / 5px; }`:  "gives elliptical corners",
		`.a { border-radius: -2px; }`:        "must not be negative",
		`.a { border-radius: auto; }`:        "auto is not a value border-radius takes",
	} {
		said := warningsFor(t, `<div class="a">x</div>`, css)
		if !strings.Contains(said, want) {
			t.Errorf("%s said %q; want %q", css, said, want)
		}
	}
}
