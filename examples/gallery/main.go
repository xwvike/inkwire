package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

//go:embed assets/*.png assets/*.jpeg
var assets embed.FS

// The gallery answers one question: what happens when a caller hands over an
// image without saying what it is. Each asset is profiled, drawn with whatever
// the profile suggests, and shown next to the numbers that produced the
// suggestion, so a wrong call is visible rather than mysterious.
func main() {
	outputDir := flag.String("out", "out", "directory for the per-asset panel images")
	sheetPath := flag.String("sheet", "gallery.png", "contact sheet of every asset")
	payloadFor := flag.String("payload-for", "", "write a Gicisky payload for one asset, by name")
	payloadPath := flag.String("payload", "", "payload output path")
	flag.Parse()

	entries, err := loadAssets()
	if err != nil {
		fail(err)
	}
	if len(entries) == 0 {
		fail(fmt.Errorf("no assets embedded"))
	}

	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		fail(err)
	}

	if *payloadFor != "" {
		entry, ok := findAsset(entries, *payloadFor)
		if !ok {
			fail(fmt.Errorf("no asset named %q", *payloadFor))
		}
		frame, err := renderCard(entry, fonts)
		if err != nil {
			fail(err)
		}
		payload, err := display.EncodeGicisky(frame)
		if err != nil {
			fail(err)
		}
		if *payloadPath == "" {
			*payloadPath = entry.name + ".bin"
		}
		if err := os.WriteFile(*payloadPath, payload, 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s (%d bytes) for %s\n", *payloadPath, len(payload), entry.name)
		return
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fail(err)
	}
	fmt.Printf("%-14s %10s %10s %14s\n", "asset", "mid-tone", "otsu", "treatment")
	for _, entry := range entries {
		frame, err := renderCard(entry, fonts)
		if err != nil {
			fail(err)
		}
		if err := writePNG(path.Join(*outputDir, entry.name+".png"), frame); err != nil {
			fail(err)
		}
		fmt.Printf("%-14s %9.1f%% %10d %14s\n",
			entry.name, 100*entry.profile.MidToneFraction, entry.profile.Threshold, treatmentOf(entry.profile))
	}

	sheet, err := renderSheet(entries, fonts)
	if err != nil {
		fail(err)
	}
	if err := writePNG(*sheetPath, sheet); err != nil {
		fail(err)
	}
	fmt.Printf("\n%d panels in %s/, contact sheet %s\n", len(entries), *outputDir, *sheetPath)
}

type asset struct {
	name    string
	image   image.Image
	profile compose.ImageProfile
}

func loadAssets() ([]asset, error) {
	files, err := assets.ReadDir("assets")
	if err != nil {
		return nil, err
	}
	var entries []asset
	for _, file := range files {
		data, err := assets.ReadFile("assets/" + file.Name())
		if err != nil {
			return nil, err
		}
		// image.Decode rather than png.Decode: the point of the gallery is
		// that a caller hands over whatever they have.
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", file.Name(), err)
		}
		profile, err := compose.ProfileImage(decoded)
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", file.Name(), err)
		}
		entries = append(entries, asset{
			name:    strings.TrimSuffix(strings.TrimSuffix(file.Name(), ".png"), ".jpeg"),
			image:   decoded,
			profile: profile,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].profile.MidToneFraction < entries[j].profile.MidToneFraction
	})
	return entries, nil
}

func findAsset(entries []asset, name string) (asset, bool) {
	for _, entry := range entries {
		if entry.name == name {
			return entry, true
		}
	}
	return asset{}, false
}

func treatmentOf(profile compose.ImageProfile) string {
	if profile.Photographic {
		return "diffusion"
	}
	return "threshold"
}

// prepare applies the contrast pass a photograph needs and flat artwork does
// not. The radius is given in source pixels but decides which feature size
// survives on the panel, so it is scaled by how far the image is about to be
// reduced; a fixed radius that suits one source is wrong for the next.
//
// Whether every photograph wants this is not settled. It rescues a dark
// subject, and the sample here contains exactly one photograph, so the rule is
// applied where it is known to help and left visible rather than buried.
func prepare(entry asset, target image.Rectangle) (image.Image, display.ImageOptions, error) {
	if entry.profile.ColourCarriesStructure {
		// Brightness would throw this drawing away; measure it against the
		// paper instead. That moves the tone, so the cut has to be taken again
		// from what is actually going to be drawn.
		toned, err := compose.ToneByColourDistance(entry.image)
		if err != nil {
			return nil, display.ImageOptions{}, err
		}
		options, err := entry.profile.SuggestOptionsFor(toned)
		return toned, options, err
	}
	if !entry.profile.Photographic {
		return entry.image, entry.profile.SuggestOptions(), nil
	}
	// A very dark subject wants more than this and cel-shaded artwork wants
	// less; no measurement in the profile separates those two cases, so one
	// moderate amount is used for both rather than a rule invented from a
	// single example of each.
	const featureSizeOnPanel, contrastAmount = 7, 1.4
	reduction := max(1, entry.image.Bounds().Dx()/max(1, target.Dx()))
	sharpened, err := compose.EnhanceContrast(entry.image, featureSizeOnPanel*reduction, contrastAmount)
	return sharpened, entry.profile.SuggestOptions(), err
}

func redVerdict(profile compose.ImageProfile) string {
	if profile.RedIsMeaningful {
		return fmt.Sprintf("%d KEEP", profile.RedSeparation)
	}
	return fmt.Sprintf("%d OFF", profile.RedSeparation)
}

// One asset on one panel, with the measurement beside it.
func renderCard(entry asset, fonts *display.FontRegistry) (*display.Frame, error) {
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	canvas := display.NewCanvas(frame)

	canvas.FillRect(image.Rect(0, 0, 296, 18), display.InkBlack)
	if err := text(canvas, fonts, image.Rect(5, 1, 200, 17), 15, display.AlignStart,
		run("GALLERY ", "monaco", 12, display.InkWhite),
		run("图片自动适配", "hzk", 12, display.InkWhite),
	); err != nil {
		return nil, err
	}

	frameBox := image.Rect(6, 24, 110, 122)
	canvas.StrokeRoundRect(frameBox, 4, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	prepared, options, err := prepare(entry, frameBox.Inset(4))
	if err != nil {
		return nil, err
	}
	if err := canvas.DrawImage(prepared, frameBox.Inset(4), options); err != nil {
		return nil, err
	}

	if err := text(canvas, fonts, image.Rect(120, 24, 290, 42), 18, display.AlignStart,
		run(entry.name, "monaco", 14, display.InkBlack),
	); err != nil {
		return nil, err
	}
	canvas.DrawLine(image.Pt(120, 46), image.Pt(290, 46), display.StrokeStyle{
		Ink: display.InkBlack, Width: 1, Dash: []int{3, 3},
	})

	rows := []struct{ label, value string }{
		{"中间调", fmt.Sprintf("%.1f%%", 100*entry.profile.MidToneFraction)},
		{"OTSU", fmt.Sprintf("%d", entry.profile.Threshold)},
		{"红色", redVerdict(entry.profile)},
	}
	for index, row := range rows {
		top := 50 + index*17
		if err := text(canvas, fonts, image.Rect(120, top, 176, top+14), 14, display.AlignStart,
			run(row.label, "ui", 12, display.InkBlack),
		); err != nil {
			return nil, err
		}
		if err := text(canvas, fonts, image.Rect(180, top, 290, top+14), 14, display.AlignStart,
			run(row.value, "monaco", 12, display.InkBlack),
		); err != nil {
			return nil, err
		}
	}

	verdict, ink := "阈值 · 图形", display.InkBlack
	if entry.profile.Photographic {
		verdict, ink = "误差扩散 · 照片", display.InkRed
	}
	badge := image.Rect(120, 102, 290, 121)
	canvas.StrokeRoundRect(badge, 3, display.StrokeStyle{Ink: ink, Width: 1})
	return frame, text(canvas, fonts, badge.Inset(3), 14, display.AlignCenter,
		run(verdict, "ui", 12, ink),
	)
}

// The contact sheet is a review artifact rather than a panel image, so it is
// free to be larger than 296x128.
func renderSheet(entries []asset, fonts *display.FontRegistry) (*display.Frame, error) {
	const cell, gap, caption = 92, 8, 28
	columns := 5
	rows := (len(entries) + columns - 1) / columns
	frame, err := display.NewFrame(columns*(cell+gap)+gap, rows*(cell+gap+caption)+gap, display.InkWhite)
	if err != nil {
		return nil, err
	}
	canvas := display.NewCanvas(frame)

	for index, entry := range entries {
		column, row := index%columns, index/columns
		box := image.Rect(
			gap+column*(cell+gap), gap+row*(cell+gap+caption),
			gap+column*(cell+gap)+cell, gap+row*(cell+gap+caption)+cell,
		)
		prepared, options, err := prepare(entry, box)
		if err != nil {
			return nil, err
		}
		if err := canvas.DrawImage(prepared, box, options); err != nil {
			return nil, err
		}
		canvas.StrokeRect(box, display.StrokeStyle{Ink: display.InkBlack, Width: 1})

		ink := display.InkBlack
		mark := "T"
		if entry.profile.Photographic {
			ink, mark = display.InkRed, "FS"
		}
		if err := text(canvas, fonts, image.Rect(box.Min.X, box.Max.Y+1, box.Max.X, box.Max.Y+14), 13,
			display.AlignStart, run(entry.name, "monaco", 10, display.InkBlack)); err != nil {
			return nil, err
		}
		if err := text(canvas, fonts, image.Rect(box.Min.X, box.Max.Y+14, box.Max.X, box.Max.Y+27), 13,
			display.AlignStart,
			run(fmt.Sprintf("%.0f%% t%d ", 100*entry.profile.MidToneFraction, entry.profile.Threshold), "monaco", 10, display.InkBlack),
			run(mark, "monaco", 10, ink),
		); err != nil {
			return nil, err
		}
	}
	return frame, nil
}

func text(canvas *display.Canvas, fonts *display.FontRegistry, bounds image.Rectangle,
	lineHeight int, align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := canvas.DrawTextBox(fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: lineHeight,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing gallery glyphs: %q", string(missing))
	}
	return nil
}

func run(value, font string, size int, ink display.Ink) display.TextRun {
	return display.TextRun{Text: value, Style: display.TextStyle{Font: font, Size: size, Ink: ink}}
}

func writePNG(name string, frame *display.Frame) error {
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	if err := display.WritePNG(file, frame); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
