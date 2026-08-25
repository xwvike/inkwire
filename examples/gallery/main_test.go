package main

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/testscene"
)

// Every asset lands on the contact sheet, so pinning the sheet pins all of
// them at once, including the treatment each one was given.
func TestRenderContactSheet(t *testing.T) {
	entries, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 5 {
		t.Fatalf("only %d assets embedded; the sample is meant to be varied", len(entries))
	}
	sheet, err := renderSheet(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	testscene.AssertMatchesPNG(t, "gallery.png", sheet)
}

// Each asset also has to fit a real panel and encode to a real payload, since
// the point of the gallery is that any of them could be sent.
func TestEveryAssetFitsAPanel(t *testing.T) {
	entries, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			frame, err := renderCard(entry, nil)
			if err != nil {
				t.Fatal(err)
			}
			testscene.AssertEncodesFor(t, 0x0033, frame, display.OrientationLandscape)
		})
	}
}

// The two failures this whole mechanism exists to prevent: artwork lighter than
// a fixed cut thresholding away to nothing, and a nearly grey photograph
// scattering red it does not contain.
func TestSuggestionsAvoidTheKnownFailures(t *testing.T) {
	entries, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			target := image.Rect(0, 0, 88, 88)
			prepared, options, err := prepare(entry, target)
			if err != nil {
				t.Fatal(err)
			}
			compiler, err := compose.NewDefaultCompiler()
			if err != nil {
				t.Fatal(err)
			}
			compiled, report, err := compiler.Compile(compose.Document{
				Size:       image.Pt(88, 88),
				Background: compose.Value(display.InkWhite),
				Root:       compose.Image{Size: image.Pt(88, 88), Source: prepared, Processing: compose.ImageManual, Options: options},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Warnings) != 0 || len(report.MissingRunes) != 0 {
				t.Fatalf("unexpected compose report: %+v", report)
			}
			frame, err := compiled.Render()
			if err != nil {
				t.Fatal(err)
			}
			ink := 88*88 - countInk(frame, display.InkWhite)
			if ink == 0 {
				t.Fatal("the suggested options rendered nothing at all")
			}
			if !entry.profile.RedIsMeaningful && countInk(frame, display.InkRed) != 0 {
				t.Errorf("red reached the panel from a source whose reds are only warm (separation %d)",
					entry.profile.RedSeparation)
			}
		})
	}
}

func countInk(frame *display.Frame, target display.Ink) int {
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == target {
				count++
			}
		}
	}
	return count
}
