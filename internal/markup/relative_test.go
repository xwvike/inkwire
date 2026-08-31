package markup

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestRelativeOffsetsMovePaintWithoutChangingFlowSlot(t *testing.T) {
	got := boxes(t, `<i class="a"></i><i class="b"></i>`,
		`.a { display: block; flex-basis: 20px; height: 10px; position: relative;
			top: 5px; left: 7px; background: black; }
		 .b { display: block; flex-basis: 20px; height: 10px; background: red; }`)
	expect(t, got, display.InkBlack, image.Rect(7, 5, 27, 15), "relative offsets move the painted box")
	expect(t, got, display.InkRed, image.Rect(20, 0, 40, 10), "the next item keeps the original flow slot")
}

func TestRelativePercentagesUseTheContainingBox(t *testing.T) {
	got := boxes(t, `<i class="a"></i>`,
		`.a { display: block; width: 20px; height: 10px; position: relative;
			top: 10%; left: 20%; background: black; }`)
	expect(t, got, display.InkBlack, image.Rect(20, 5, 40, 15), "relative percentages use the page box")
}
