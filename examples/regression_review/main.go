// Command regression_review renders the same three regression cases before and
// after a fix. It does not select or simulate either implementation.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/markup"
	"github.com/xwvike/inkwire/internal/scene"
)

//go:embed page.html
var page string

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func save(dir, name string, frame *display.Frame) {
	// Enlarge with nearest-neighbour sampling so every physical pixel is visible.
	const scale = 12
	enlarged := image.NewNRGBA(image.Rect(0, 0, frame.Width()*scale, frame.Height()*scale))
	for y := 0; y < enlarged.Bounds().Dy(); y++ {
		for x := 0; x < enlarged.Bounds().Dx(); x++ {
			enlarged.Set(x, y, frame.At(x/scale, y/scale))
		}
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	must(err)
	must(png.Encode(file, enlarged))
	must(file.Close())
}

func main() {
	out := flag.String("out", "examples/regression_review/after", "output directory")
	flag.Parse()
	must(os.MkdirAll(*out, 0755))
	slope, err := display.NewFrame(28, 20, display.InkWhite)
	must(err)
	display.NewCanvas(slope).DrawPolyline([]image.Point{image.Pt(10, 10), image.Pt(17, 11)}, display.StrokeStyle{Width: 2, Ink: display.InkBlack, Cap: display.StrokeCapButt})
	save(*out, "slope", slope)
	circles, err := display.NewFrame(18, 10, display.InkWhite)
	must(err)
	canvas := display.NewCanvas(circles)
	canvas.FillCircle(image.Pt(4, 4), 0, display.InkBlack)
	canvas.StrokeCircle(image.Pt(12, 4), 1, display.StrokeStyle{Width: 1, Ink: display.InkBlack})
	save(*out, "circles", circles)
	compiled, err := markup.Compile(page, "")
	must(err)
	must(os.WriteFile(filepath.Join(*out, "scene.json"), compiled.JSON, 0644))
	rendered, err := (scene.Decoder{}).Render(bytes.NewReader(compiled.JSON))
	must(err)
	save(*out, "orientation", rendered.Frame)
	missing, _ := slope.InkAt(13, 9)
	point, _ := circles.InkAt(4, 4)
	center, _ := circles.InkAt(12, 4)
	summary, err := json.MarshalIndent(map[string]any{
		"slopePixel13_9": missing.String(), "zeroRadiusCenter": point.String(), "ringCenter": center.String(),
		"orientation": rendered.Orientation, "width": rendered.Frame.Width(), "height": rendered.Frame.Height(),
	}, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*out, "result.json"), summary, 0644))
}
