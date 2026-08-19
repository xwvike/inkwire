// Command gallery answers one question: what happens when a caller hands over
// an image without saying what it is.
//
// Each asset is profiled, drawn with whatever the profile suggests, and shown
// next to the numbers that produced the suggestion, so a wrong call is visible
// rather than mysterious.
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
	"github.com/xwvike/inkwire/internal/gicisky"
)

//go:embed assets/*.png assets/*.jpeg
var assets embed.FS

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

	if *payloadFor != "" {
		entry, ok := findAsset(entries, *payloadFor)
		if !ok {
			fail(fmt.Errorf("no asset named %q", *payloadFor))
		}
		frame, err := renderCard(entry, nil)
		if err != nil {
			fail(err)
		}
		payload, err := encodeForTag(frame)
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
		frame, err := renderCard(entry, nil)
		if err != nil {
			fail(err)
		}
		if err := writePNG(path.Join(*outputDir, entry.name+".png"), frame); err != nil {
			fail(err)
		}
		fmt.Printf("%-14s %9.1f%% %10d %14s\n",
			entry.name, 100*entry.profile.MidToneFraction, entry.profile.Threshold, treatmentOf(entry.profile))
	}

	sheet, err := renderSheet(entries, nil)
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
func renderCard(entry asset, _ *display.FontRegistry) (*display.Frame, error) {
	frameBox := image.Rect(6, 24, 110, 122)
	prepared, options, err := prepare(entry, frameBox.Inset(4))
	if err != nil {
		return nil, err
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, report, err := compiler.Compile(compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{Size: image.Pt(296, 128), Clip: true, Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 296, 18), Node: compose.Rectangle{Size: image.Pt(296, 18), Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(5, 1, 200, 17), Node: compose.Text{Size: image.Pt(195, 16), Runs: []display.TextRun{run("GALLERY ", "monaco", 12, display.InkWhite), run("图片自动适配", "hzk", 12, display.InkWhite)}}},
			{Bounds: frameBox, Node: compose.Stack{Size: frameBox.Size(), Children: []compose.Node{
				compose.Rectangle{Size: frameBox.Size(), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})},
				compose.Image{Size: frameBox.Inset(4).Size(), Source: prepared, Processing: compose.ImageManual, Options: options},
			}}},
			{Bounds: image.Rect(120, 24, 290, 42), Node: compose.Text{Size: image.Pt(170, 18), Runs: []display.TextRun{run(entry.name, "monaco", 14, display.InkBlack)}}},
			{Bounds: image.Rect(120, 46, 290, 48), Node: compose.Line{Size: image.Pt(170, 2), From: image.Pt(0, 0), To: image.Pt(170, 0), Stroke: display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{3, 3}}}},
			{Bounds: image.Rect(120, 50, 176, 64), Node: compose.Text{Size: image.Pt(56, 14), Runs: []display.TextRun{run("中间调", "ui", 12, display.InkBlack)}}},
			{Bounds: image.Rect(180, 50, 290, 64), Node: compose.Text{Size: image.Pt(110, 14), Runs: []display.TextRun{run(fmt.Sprintf("%.1f%%", 100*entry.profile.MidToneFraction), "monaco", 12, display.InkBlack)}}},
			{Bounds: image.Rect(120, 67, 176, 81), Node: compose.Text{Size: image.Pt(56, 14), Runs: []display.TextRun{run("OTSU", "monaco", 12, display.InkBlack)}}},
			{Bounds: image.Rect(180, 67, 290, 81), Node: compose.Text{Size: image.Pt(110, 14), Runs: []display.TextRun{run(fmt.Sprintf("%d", entry.profile.Threshold), "monaco", 12, display.InkBlack)}}},
			{Bounds: image.Rect(120, 84, 176, 98), Node: compose.Text{Size: image.Pt(56, 14), Runs: []display.TextRun{run("红色", "ui", 12, display.InkBlack)}}},
			{Bounds: image.Rect(180, 84, 290, 98), Node: compose.Text{Size: image.Pt(110, 14), Runs: []display.TextRun{run(redVerdict(entry.profile), "monaco", 12, display.InkBlack)}}},
			{Bounds: image.Rect(120, 102, 290, 121), Node: compose.Rectangle{Size: image.Pt(170, 19), Radius: 3, Stroke: compose.Stroke(display.StrokeStyle{Ink: verdictInk(entry.profile), Width: 1})}},
			{Bounds: image.Rect(123, 105, 287, 118), Node: compose.Text{Size: image.Pt(164, 13), Align: display.AlignCenter, Runs: []display.TextRun{run(verdictText(entry.profile), "ui", 12, verdictInk(entry.profile))}}},
		}},
	})
	if err != nil {
		return nil, err
	}
	if len(report.MissingRunes) != 0 || len(report.Warnings) != 0 {
		return nil, fmt.Errorf("gallery compose report: missing=%q warnings=%v", string(report.MissingRunes), report.Warnings)
	}
	return compiled.Render()
}

func verdictInk(profile compose.ImageProfile) display.Ink {
	if profile.Photographic {
		return display.InkRed
	}
	return display.InkBlack
}
func verdictText(profile compose.ImageProfile) string {
	if profile.Photographic {
		return "误差扩散 · 照片"
	}
	return "阈值 · 图形"
}

// The contact sheet is a review artifact rather than a panel image, so it is
// free to be larger than 296x128.
func renderSheet(entries []asset, _ *display.FontRegistry) (*display.Frame, error) {
	const cell, gap, caption = 92, 8, 28
	columns := 5
	rows := (len(entries) + columns - 1) / columns
	sheetSize := image.Pt(columns*(cell+gap)+gap, rows*(cell+gap+caption)+gap)
	children := make([]compose.Placed, 0, len(entries)*3)
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
		children = append(children,
			compose.Placed{Bounds: box, Node: compose.Image{Size: box.Size(), Source: prepared, Processing: compose.ImageManual, Options: options}},
			compose.Placed{Bounds: box, Node: compose.Rectangle{Size: box.Size(), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		)
		ink := display.InkBlack
		mark := "T"
		if entry.profile.Photographic {
			ink, mark = display.InkRed, "FS"
		}
		children = append(children,
			compose.Placed{Bounds: image.Rect(box.Min.X, box.Max.Y+1, box.Max.X, box.Max.Y+14), Node: compose.Text{Size: image.Pt(box.Dx(), 13), Runs: []display.TextRun{run(entry.name, "monaco", 10, display.InkBlack)}}},
			compose.Placed{Bounds: image.Rect(box.Min.X, box.Max.Y+14, box.Max.X, box.Max.Y+27), Node: compose.Text{Size: image.Pt(box.Dx(), 13), Runs: []display.TextRun{
				run(fmt.Sprintf("%.0f%% t%d ", 100*entry.profile.MidToneFraction, entry.profile.Threshold), "monaco", 10, display.InkBlack), run(mark, "monaco", 10, ink),
			}}},
		)
	}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, report, err := compiler.Compile(compose.Document{Size: sheetSize, Background: compose.Value(display.InkWhite), Root: compose.Absolute{Size: sheetSize, Clip: true, Children: children}})
	if err != nil {
		return nil, err
	}
	if len(report.MissingRunes) != 0 || len(report.Warnings) != 0 {
		return nil, fmt.Errorf("gallery sheet compose report: missing=%q warnings=%v", string(report.MissingRunes), report.Warnings)
	}
	return compiled.Render()
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

// encodeForTag packs a page for the 2.9" BWR tag these examples are drawn for.
// Real writes discover the model from its advertisement; an offline example
// has to name one.
func encodeForTag(frame *display.Frame) ([]byte, error) {
	profile, known := gicisky.LookupProfile(0x0033, 0)
	if !known {
		return nil, fmt.Errorf("no Gicisky profile 0x0033")
	}
	return gicisky.Encode(frame, profile)
}
