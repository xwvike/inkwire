package compose

import (
	"fmt"
	stdimage "image"
	"math"
	"strings"

	"github.com/xwvike/inkwire/internal/display"
)

// Text paints exactly the supplied runs. Size is its preferred size in flow
// layout; zero axes use the measured text size within the available space.
type Text struct {
	Size          stdimage.Point
	Runs          []display.TextRun
	Align         display.HorizontalAlign
	VerticalAlign display.VerticalAlign
	Wrap          display.WrapMode
	LineHeight    int
}

func (Text) composeNode() {}

func (t Text) measure(ctx *compileContext, maximum stdimage.Point, path string) (stdimage.Point, error) {
	if err := t.validate(path); err != nil {
		return stdimage.Point{}, err
	}
	boxSize := maximum
	if t.Size.X > 0 {
		boxSize.X = min(t.Size.X, maximum.X)
	}
	if t.Size.Y > 0 {
		boxSize.Y = min(t.Size.Y, maximum.Y)
	}
	if boxSize.X <= 0 || boxSize.Y <= 0 {
		return stdimage.Point{}, nil
	}
	layout, err := display.LayoutText(ctx.compiler.Fonts, t.box(stdimage.Rectangle{Max: boxSize}))
	if err != nil {
		return stdimage.Point{}, fmt.Errorf("%s: %w", path, err)
	}
	size := layout.Size()
	if t.Size.X > 0 {
		size.X = t.Size.X
	}
	if t.Size.Y > 0 {
		size.Y = t.Size.Y
	}
	// Said before the clamp. Afterwards the number that would tell somebody the
	// box is three pixels short has been rounded down to the size of that box.
	ctx.wants(path, size)
	return constrainSize(size, maximum), nil
}

func (t Text) paint(ctx *compileContext, list *display.DisplayList, bounds stdimage.Rectangle, path string) error {
	if bounds.Empty() {
		ctx.warn(path, "empty-layout", "text has no drawable area")
		return nil
	}
	layout, err := list.DrawTextBox(ctx.compiler.Fonts, t.box(bounds))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	ctx.addMissing(path, layout.MissingRunes(), layout.MissingFonts())
	if columns, lines, rows := layout.Clipped(); columns > 0 || lines > 0 || rows > 0 {
		ctx.warn(path, "text-clipped", fmt.Sprintf("%q does not fit %dx%d: %s cut off",
			runText(t.Runs), bounds.Dx(), bounds.Dy(), lostText(columns, lines, rows)))
	}
	return nil
}

// lostText names what the box took, listing only the kinds that were actually
// lost. A warning that says nought pixels and nought lines before the one
// number that matters reads as though it is unsure which it means.
func lostText(columns, lines, rows int) string {
	var parts []string
	if columns > 0 {
		parts = append(parts, fmt.Sprintf("%d pixels along the line", columns))
	}
	if lines > 0 {
		parts = append(parts, fmt.Sprintf("%s", plural(lines, "whole line", "whole lines")))
	}
	if rows > 0 {
		parts = append(parts, fmt.Sprintf("%s of ink", plural(rows, "row", "rows")))
	}
	return strings.Join(parts, " and ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// runText joins the runs so a warning can name the text that was cut rather
// than only the path to it.
func runText(runs []display.TextRun) string {
	var joined string
	for _, run := range runs {
		joined += run.Text
	}
	return joined
}

func (t Text) validate(path string) error {
	if !validSize(t.Size) {
		return fmt.Errorf("%s: text size must not be negative, got %v", path, t.Size)
	}
	if t.Align > display.AlignEnd || t.VerticalAlign > display.AlignBottom || t.Wrap > display.WrapRunes {
		return fmt.Errorf("%s: invalid text alignment or wrap mode", path)
	}
	if t.LineHeight < 0 {
		return fmt.Errorf("%s: line height must not be negative", path)
	}
	return nil
}

func (t Text) box(bounds stdimage.Rectangle) display.TextBox {
	return display.TextBox{
		Bounds:        bounds,
		Runs:          t.Runs,
		Align:         t.Align,
		VerticalAlign: t.VerticalAlign,
		Wrap:          t.Wrap,
		LineHeight:    t.LineHeight,
	}
}

type ImageProcessing uint8

const (
	ImageManual ImageProcessing = iota
	ImageAuto
)

// ImageOverrides distinguishes an explicit override from an unset field while
// auto processing is active. Explicit values always win over suggestions.
type ImageOverrides struct {
	Fit          *display.ImageFit
	Sampling     *display.SamplingMode
	Dither       *display.DitherMode
	Threshold    *int
	RedThreshold *int
	RedMaxGreen  *int
	DisableRed   *bool
}

// Contrast requests an explicit local-contrast pass before image reduction.
// It is never enabled from the image's subject or filename.
type Contrast struct {
	Radius int
	Amount float64
}

// Image paints Source into its allocated rectangle. Manual mode passes Options
// straight to display. Auto mode profiles pixels and suggests lossy conversion
// settings; Overrides and Contrast remain explicit caller decisions.
type Image struct {
	Size       stdimage.Point
	Source     stdimage.Image
	Processing ImageProcessing
	Options    display.ImageOptions
	Overrides  ImageOverrides
	Contrast   *Contrast
}

func (Image) composeNode() {}

func (i Image) measure(_ *compileContext, maximum stdimage.Point, path string) (stdimage.Point, error) {
	if err := i.validate(path); err != nil {
		return stdimage.Point{}, err
	}
	natural := i.Source.Bounds().Size()
	return preferredSize(natural, i.Size, maximum)
}

func (i Image) paint(ctx *compileContext, list *display.DisplayList, bounds stdimage.Rectangle, path string) error {
	if bounds.Empty() {
		ctx.warn(path, "empty-layout", "image has no drawable area")
		return nil
	}
	prepared, options, decision, err := i.prepare(path)
	if err != nil {
		return err
	}
	if decision != nil {
		ctx.report.Images = append(ctx.report.Images, *decision)
	}
	if err := list.DrawImage(prepared, bounds, options); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (i Image) validate(path string) error {
	if !validSize(i.Size) {
		return fmt.Errorf("%s: image size must not be negative, got %v", path, i.Size)
	}
	if i.Source == nil {
		return fmt.Errorf("%s: image source must not be nil", path)
	}
	if i.Source.Bounds().Empty() {
		return fmt.Errorf("%s: image source bounds must not be empty", path)
	}
	if i.Processing > ImageAuto {
		return fmt.Errorf("%s: invalid image processing mode %d", path, i.Processing)
	}
	if i.Processing == ImageManual && !i.Overrides.empty() {
		return fmt.Errorf("%s: image overrides require auto processing", path)
	}
	if i.Processing == ImageAuto && i.Options != (display.ImageOptions{}) {
		return fmt.Errorf("%s: manual image options cannot be combined with auto processing; use overrides", path)
	}
	if i.Contrast != nil {
		if i.Contrast.Radius < 0 || math.IsNaN(i.Contrast.Amount) || math.IsInf(i.Contrast.Amount, 0) {
			return fmt.Errorf("%s: invalid contrast radius or amount", path)
		}
	}
	return nil
}

func (i Image) prepare(path string) (stdimage.Image, display.ImageOptions, *ImageDecision, error) {
	prepared := i.Source
	if i.Processing == ImageManual {
		if i.Contrast != nil {
			var err error
			prepared, err = EnhanceContrast(prepared, i.Contrast.Radius, i.Contrast.Amount)
			if err != nil {
				return nil, display.ImageOptions{}, nil, fmt.Errorf("%s: %w", path, err)
			}
		}
		return prepared, i.Options, nil, nil
	}

	profile, err := ProfileImage(i.Source)
	if err != nil {
		return nil, display.ImageOptions{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	toneChanged := false
	if profile.ColourCarriesStructure {
		prepared, err = ToneByColourDistance(prepared)
		if err != nil {
			return nil, display.ImageOptions{}, nil, fmt.Errorf("%s: %w", path, err)
		}
		toneChanged = true
	}
	if i.Contrast != nil {
		prepared, err = EnhanceContrast(prepared, i.Contrast.Radius, i.Contrast.Amount)
		if err != nil {
			return nil, display.ImageOptions{}, nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	options, err := profile.SuggestOptionsFor(prepared)
	if err != nil {
		return nil, display.ImageOptions{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	options = i.Overrides.apply(options)
	options = concreteImageOptions(options)
	return prepared, options, &ImageDecision{
		Path:                 path,
		Profile:              profile,
		Options:              options,
		ToneByColourDistance: toneChanged,
		ContrastEnhanced:     i.Contrast != nil,
	}, nil
}

func concreteImageOptions(options display.ImageOptions) display.ImageOptions {
	defaults := display.DefaultImageOptions()
	if options.Threshold == 0 {
		options.Threshold = defaults.Threshold
	}
	if options.RedThreshold == 0 {
		options.RedThreshold = defaults.RedThreshold
	}
	if options.RedMaxGreen == 0 {
		options.RedMaxGreen = defaults.RedMaxGreen
	}
	return options
}

func (o ImageOverrides) apply(options display.ImageOptions) display.ImageOptions {
	if o.Fit != nil {
		options.Fit = *o.Fit
	}
	if o.Sampling != nil {
		options.Sampling = *o.Sampling
	}
	if o.Dither != nil {
		options.Dither = *o.Dither
	}
	if o.Threshold != nil {
		options.Threshold = *o.Threshold
	}
	if o.RedThreshold != nil {
		options.RedThreshold = *o.RedThreshold
	}
	if o.RedMaxGreen != nil {
		options.RedMaxGreen = *o.RedMaxGreen
	}
	if o.DisableRed != nil {
		options.DisableRed = *o.DisableRed
	}
	return options
}

func (o ImageOverrides) empty() bool {
	return o.Fit == nil && o.Sampling == nil && o.Dither == nil && o.Threshold == nil &&
		o.RedThreshold == nil && o.RedMaxGreen == nil && o.DisableRed == nil
}
