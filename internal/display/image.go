package display

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
)

type ImageFit uint8

const (
	FitStretch ImageFit = iota
	FitContain
	FitCover
)

type SamplingMode uint8

const (
	SampleNearest SamplingMode = iota
	SampleBilinear
)

type DitherMode uint8

const (
	DitherThreshold DitherMode = iota
	DitherFloydSteinberg
	DitherOrdered
)

// The three enums name themselves with the words a scene uses to ask for them,
// so a report reads back in the vocabulary it was written in. They used to
// leave a report as bare integers: dither=2 told a caller nothing it could
// look up, and nothing said which list to count along.

func (f ImageFit) String() string {
	switch f {
	case FitStretch:
		return "stretch"
	case FitContain:
		return "contain"
	case FitCover:
		return "cover"
	}
	return fmt.Sprintf("ImageFit(%d)", uint8(f))
}

func (f ImageFit) MarshalJSON() ([]byte, error) { return json.Marshal(f.String()) }

func (s SamplingMode) String() string {
	switch s {
	case SampleNearest:
		return "nearest"
	case SampleBilinear:
		return "bilinear"
	}
	return fmt.Sprintf("SamplingMode(%d)", uint8(s))
}

func (s SamplingMode) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (d DitherMode) String() string {
	switch d {
	case DitherThreshold:
		return "threshold"
	case DitherFloydSteinberg:
		return "floydSteinberg"
	case DitherOrdered:
		return "ordered"
	}
	return fmt.Sprintf("DitherMode(%d)", uint8(d))
}

func (d DitherMode) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// The parsers are here beside the names rather than in the decoder that first
// needed them, so one list spells each word once. A report that sends "cover"
// and a scene that asks for "cover" are now reading the same table.

// ParseImageFit reads a fit by name. An empty name is the default rather than
// an error, which is what a scene omitting the field means.
func ParseImageFit(name string) (ImageFit, bool) {
	switch name {
	case "", "stretch":
		return FitStretch, true
	case "contain":
		return FitContain, true
	case "cover":
		return FitCover, true
	}
	return 0, false
}

// ImageFitNames lists the spellings ParseImageFit accepts, for the errors that
// have to say what was allowed.
func ImageFitNames() []string { return []string{"stretch", "contain", "cover"} }

func (f *ImageFit) UnmarshalJSON(data []byte) error { return unmarshalEnum(data, f, ParseImageFit) }

// ParseSamplingMode reads a sampling mode by name.
func ParseSamplingMode(name string) (SamplingMode, bool) {
	switch name {
	case "", "nearest":
		return SampleNearest, true
	case "bilinear":
		return SampleBilinear, true
	}
	return 0, false
}

// SamplingModeNames lists the spellings ParseSamplingMode accepts.
func SamplingModeNames() []string { return []string{"nearest", "bilinear"} }

func (s *SamplingMode) UnmarshalJSON(data []byte) error {
	return unmarshalEnum(data, s, ParseSamplingMode)
}

// ParseDitherMode reads a dither mode by name.
func ParseDitherMode(name string) (DitherMode, bool) {
	switch name {
	case "", "threshold":
		return DitherThreshold, true
	case "floydSteinberg":
		return DitherFloydSteinberg, true
	case "ordered":
		return DitherOrdered, true
	}
	return 0, false
}

// DitherModeNames lists the spellings ParseDitherMode accepts.
func DitherModeNames() []string { return []string{"threshold", "floydSteinberg", "ordered"} }

func (d *DitherMode) UnmarshalJSON(data []byte) error { return unmarshalEnum(data, d, ParseDitherMode) }

func unmarshalEnum[T ~uint8](data []byte, into *T, parse func(string) (T, bool)) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	value, known := parse(name)
	if !known {
		return fmt.Errorf("unknown value %q", name)
	}
	*into = value
	return nil
}

// Defaults verified against the target panel.
const (
	defaultThreshold    = 128
	defaultRedThreshold = 170
	defaultRedMaxGreen  = 170
)

// DefaultImageOptions returns the concrete values used when ImageOptions
// leaves its numeric thresholds unset. Callers that analyse an image before
// drawing can use the same device conversion limits without duplicating them.
func DefaultImageOptions() ImageOptions {
	return ImageOptions{
		Threshold:    defaultThreshold,
		RedThreshold: defaultRedThreshold,
		RedMaxGreen:  defaultRedMaxGreen,
	}
}

// ImageOptions controls how a source image is reduced to three inks. The three
// integer limits treat zero as "use the default", so a caller cannot ask for a
// limit of exactly zero; those values are degenerate anyway, except that
// RedMaxGreen of zero is the natural way to say "no red", which is what
// DisableRed exists for.
type ImageOptions struct {
	Fit          ImageFit     `json:"fit"`
	Sampling     SamplingMode `json:"sampling"`
	Dither       DitherMode   `json:"dither"`
	Threshold    int          `json:"threshold"`    // luminance above which a pixel is white; 0 means 128
	RedThreshold int          `json:"redThreshold"` // red channel above which a pixel may be red; 0 means 170
	RedMaxGreen  int          `json:"redMaxGreen"`  // green channel below which a pixel may be red; 0 means 170
	DisableRed   bool         `json:"disableRed"`   // keep the red plane empty whatever the source contains
}

type imageWindow struct {
	target       image.Rectangle
	sourceX      float64
	sourceY      float64
	sourceWidth  float64
	sourceHeight float64
}

type sampledColor struct {
	r, g, b float64
	a       float64
}

func (c *Canvas) DrawImage(source image.Image, destination image.Rectangle, options ImageOptions) error {
	if source == nil {
		return fmt.Errorf("source image must not be nil")
	}
	if destination.Empty() {
		return fmt.Errorf("destination image bounds must not be empty")
	}
	if source.Bounds().Empty() {
		return fmt.Errorf("source image bounds must not be empty")
	}
	options, err := normalizeImageOptions(options)
	if err != nil {
		return err
	}
	window := fitImage(source.Bounds(), destination, options.Fit)
	drawRect := window.target.Intersect(c.logicalClip())
	if drawRect.Empty() {
		return nil
	}

	luminance := make([]float64, drawRect.Dx()*drawRect.Dy())
	redPixels := make([]bool, len(luminance))
	for y := drawRect.Min.Y; y < drawRect.Max.Y; y++ {
		for x := drawRect.Min.X; x < drawRect.Max.X; x++ {
			sourceX := window.sourceX + (float64(x-window.target.Min.X)+0.5)*window.sourceWidth/float64(window.target.Dx()) - 0.5
			sourceY := window.sourceY + (float64(y-window.target.Min.Y)+0.5)*window.sourceHeight/float64(window.target.Dy()) - 0.5
			sample := sampleImage(source, sourceX, sourceY, options.Sampling)
			point := c.devicePoint(image.Pt(x, y))
			ink, _ := c.frame.InkAt(point.X, point.Y)
			sample = compositeOverInk(sample, ink)
			index := (y-drawRect.Min.Y)*drawRect.Dx() + x - drawRect.Min.X
			luminance[index] = 0.2126*sample.r + 0.7152*sample.g + 0.0722*sample.b
			redPixels[index] = !options.DisableRed &&
				sample.r > float64(options.RedThreshold) && sample.g < float64(options.RedMaxGreen)
		}
	}

	switch options.Dither {
	case DitherThreshold:
		drawThresholdImage(c, drawRect, luminance, redPixels, float64(options.Threshold))
	case DitherFloydSteinberg:
		drawFloydSteinbergImage(c, drawRect, luminance, redPixels, float64(options.Threshold))
	case DitherOrdered:
		drawOrderedImage(c, drawRect, luminance, redPixels)
	}
	return nil
}

func normalizeImageOptions(options ImageOptions) (ImageOptions, error) {
	if options.Fit > FitCover {
		return options, fmt.Errorf("invalid image fit %d", options.Fit)
	}
	if options.Sampling > SampleBilinear {
		return options, fmt.Errorf("invalid sampling mode %d", options.Sampling)
	}
	if options.Dither > DitherOrdered {
		return options, fmt.Errorf("invalid dither mode %d", options.Dither)
	}
	defaults := DefaultImageOptions()
	if options.Threshold == 0 {
		options.Threshold = defaults.Threshold
	}
	if options.RedThreshold == 0 {
		options.RedThreshold = defaults.RedThreshold
	}
	if options.RedMaxGreen == 0 {
		options.RedMaxGreen = defaults.RedMaxGreen
	}
	for name, value := range map[string]int{
		"threshold": options.Threshold, "red threshold": options.RedThreshold, "red max green": options.RedMaxGreen,
	} {
		if value < 0 || value > 255 {
			return options, fmt.Errorf("%s must be between 0 and 255, got %d", name, value)
		}
	}
	return options, nil
}

func fitImage(source, destination image.Rectangle, fit ImageFit) imageWindow {
	window := imageWindow{
		target: destination, sourceX: float64(source.Min.X), sourceY: float64(source.Min.Y),
		sourceWidth: float64(source.Dx()), sourceHeight: float64(source.Dy()),
	}
	switch fit {
	case FitContain:
		scale := math.Min(float64(destination.Dx())/float64(source.Dx()), float64(destination.Dy())/float64(source.Dy()))
		width := max(1, int(math.Round(float64(source.Dx())*scale)))
		height := max(1, int(math.Round(float64(source.Dy())*scale)))
		x := destination.Min.X + (destination.Dx()-width)/2
		y := destination.Min.Y + (destination.Dy()-height)/2
		window.target = image.Rect(x, y, x+width, y+height)
	case FitCover:
		sourceRatio := float64(source.Dx()) / float64(source.Dy())
		targetRatio := float64(destination.Dx()) / float64(destination.Dy())
		if sourceRatio > targetRatio {
			width := float64(source.Dy()) * targetRatio
			window.sourceX += (float64(source.Dx()) - width) / 2
			window.sourceWidth = width
		} else {
			height := float64(source.Dx()) / targetRatio
			window.sourceY += (float64(source.Dy()) - height) / 2
			window.sourceHeight = height
		}
	}
	return window
}

func sampleImage(source image.Image, x, y float64, mode SamplingMode) sampledColor {
	if mode == SampleNearest {
		return colorAt(source, int(math.Round(x)), int(math.Round(y)))
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := x0+1, y0+1
	tx, ty := x-float64(x0), y-float64(y0)
	c00, c10 := colorAt(source, x0, y0), colorAt(source, x1, y0)
	c01, c11 := colorAt(source, x0, y1), colorAt(source, x1, y1)
	return sampledColor{
		r: bilerp(c00.r, c10.r, c01.r, c11.r, tx, ty),
		g: bilerp(c00.g, c10.g, c01.g, c11.g, tx, ty),
		b: bilerp(c00.b, c10.b, c01.b, c11.b, tx, ty),
		a: bilerp(c00.a, c10.a, c01.a, c11.a, tx, ty),
	}
}

func colorAt(source image.Image, x, y int) sampledColor {
	bounds := source.Bounds()
	x = min(max(x, bounds.Min.X), bounds.Max.X-1)
	y = min(max(y, bounds.Min.Y), bounds.Max.Y-1)
	c := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
	return sampledColor{r: float64(c.R), g: float64(c.G), b: float64(c.B), a: float64(c.A) / 255}
}

func compositeOverInk(source sampledColor, destination Ink) sampledColor {
	background := destination.RGBA()
	return sampledColor{
		r: source.r*source.a + float64(background.R)*(1-source.a),
		g: source.g*source.a + float64(background.G)*(1-source.a),
		b: source.b*source.a + float64(background.B)*(1-source.a),
		a: 1,
	}
}

func bilerp(c00, c10, c01, c11, tx, ty float64) float64 {
	top := c00 + (c10-c00)*tx
	bottom := c01 + (c11-c01)*tx
	return top + (bottom-top)*ty
}

func drawThresholdImage(canvas *Canvas, rect image.Rectangle, values []float64, red []bool, threshold float64) {
	for index, value := range values {
		x := rect.Min.X + index%rect.Dx()
		y := rect.Min.Y + index/rect.Dx()
		canvas.Set(x, y, quantizedInk(value, red[index], threshold))
	}
}

func drawFloydSteinbergImage(canvas *Canvas, rect image.Rectangle, values []float64, red []bool, threshold float64) {
	width := rect.Dx()
	errors := make([]float64, len(values))
	for index, base := range values {
		x := index % width
		y := index / width
		if red[index] {
			canvas.Set(rect.Min.X+x, rect.Min.Y+y, InkRed)
			continue
		}
		value := min(255, max(0, base+errors[index]))
		ink := quantizedInk(value, false, threshold)
		canvas.Set(rect.Min.X+x, rect.Min.Y+y, ink)
		output := 0.0
		if ink == InkWhite {
			output = 255
		}
		errorValue := value - output
		if x+1 < width {
			errors[index+1] += errorValue * 7 / 16
		}
		if y+1 < rect.Dy() {
			if x > 0 {
				errors[index+width-1] += errorValue * 3 / 16
			}
			errors[index+width] += errorValue * 5 / 16
			if x+1 < width {
				errors[index+width+1] += errorValue / 16
			}
		}
	}
}

func drawOrderedImage(canvas *Canvas, rect image.Rectangle, values []float64, red []bool) {
	bayer := [4][4]int{
		{0, 8, 2, 10},
		{12, 4, 14, 6},
		{3, 11, 1, 9},
		{15, 7, 13, 5},
	}
	for index, value := range values {
		x := index % rect.Dx()
		y := index / rect.Dx()
		threshold := (float64(bayer[y%4][x%4]) + 0.5) * 255 / 16
		canvas.Set(rect.Min.X+x, rect.Min.Y+y, quantizedInk(value, red[index], threshold))
	}
}

func quantizedInk(luminance float64, red bool, threshold float64) Ink {
	if red {
		return InkRed
	}
	if luminance > threshold {
		return InkWhite
	}
	return InkBlack
}
