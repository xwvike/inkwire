// Command cookbook is a worked reference for every drawing call the display
// layer offers, written for whoever builds the layout and scheduling layer on
// top of it.
//
// Each section renders one panel and the comments state the contract rather
// than restating the call: what the layer guarantees, what it costs, and where
// it will surprise you. The contracts that apply everywhere:
//
//   - Coordinates are integers and rectangles are half open, the same as the
//     standard image package. There is no transform beyond integer translation,
//     so nothing ever lands between pixels and nothing is ever resampled.
//   - There are three inks and no grey. Every call writes one of them outright;
//     tone has to come from dithering an image or from a pattern.
//   - A closed shape strokes inward and can never paint outside the fill of the
//     same geometry. An open path has no inside and strokes centred on the line.
//   - Clipping and pattern phase are fixed in frame coordinates. Translating
//     afterwards moves what is drawn, not where drawing is allowed.
//   - Nothing anywhere is antialiased, and nothing is going to be: the panel has
//     no intermediate tone to render a soft edge into.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path"

	"github.com/xwvike/inkwire/internal/display"
)

//go:embed icon.png
var iconPNG []byte

type section struct {
	name string
	draw func(*display.Canvas, *display.FontRegistry) error
}

func main() {
	outputDir := flag.String("out", "out", "directory for the panel images")
	sheetPath := flag.String("sheet", "cookbook.png", "contact sheet of every panel")
	flag.Parse()

	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		fail(err)
	}
	sections := []section{
		{"fills", fills},
		{"strokes", strokes},
		{"paths", paths},
		{"text", texts},
		{"state", state},
		{"images", images},
		{"displaylist", displayLists},
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fail(err)
	}
	frames := make([]*display.Frame, 0, len(sections))
	for _, s := range sections {
		frame, err := renderPanel(s, fonts)
		if err != nil {
			fail(fmt.Errorf("%s: %w", s.name, err))
		}
		frames = append(frames, frame)
		if err := writePNG(path.Join(*outputDir, s.name+".png"), frame); err != nil {
			fail(err)
		}
		// Every panel is a real payload, not just a picture of one.
		payload, err := display.EncodeGicisky(frame)
		if err != nil {
			fail(err)
		}
		fmt.Printf("%-12s %dx%d  payload %d bytes\n", s.name, frame.Width(), frame.Height(), len(payload))
	}

	sheet, err := stack(frames)
	if err != nil {
		fail(err)
	}
	if err := writePNG(*sheetPath, sheet); err != nil {
		fail(err)
	}
	fmt.Printf("\n%d panels in %s/, contact sheet %s\n", len(sections), *outputDir, *sheetPath)
}

func renderPanel(s section, fonts *display.FontRegistry) (*display.Frame, error) {
	// NewPage is NewFrame with the panel's own dimensions filled in. Ask for a
	// portrait page and you get 128x296 plus a rotation at encode time.
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	canvas := display.NewCanvas(frame)
	canvas.FillRect(image.Rect(0, 0, 296, 15), display.InkBlack)
	if err := label(canvas, fonts, image.Rect(4, 1, 292, 14), display.AlignStart,
		run(s.name, "monaco", 10, display.InkWhite)); err != nil {
		return nil, err
	}
	return frame, s.draw(canvas, fonts)
}

// ---------------------------------------------------------------------------

// fills covers every call that paints a region. All of them decide membership
// with an exact integer test, so the result is deterministic and a shape is
// either in or out with nothing in between.
func fills(c *display.Canvas, fonts *display.FontRegistry) error {
	// Set writes one pixel. It is the only call that takes bare coordinates,
	// and like every other it silently does nothing outside the clip.
	for i := range 12 {
		c.Set(6+i*2, 22, display.InkBlack)
	}

	// FillRect takes a half-open rectangle: Max is one past the last pixel, so
	// Dx() is exactly how many columns get painted. Layout arithmetic on these
	// is exact, which is the main reason the layer is integer-only.
	c.FillRect(image.Rect(6, 28, 40, 48), display.InkBlack)
	c.FillRoundRect(image.Rect(44, 28, 78, 48), 6, display.InkRed)

	// A circle is centre plus radius and is measured between pixel centres:
	// 2*radius+1 across, symmetric about the centre pixel. An ellipse is a box
	// and is measured across whole pixels so it touches all four sides. They
	// therefore intentionally differ by half a pixel over the same geometry.
	c.FillCircle(image.Pt(96, 38), 10, display.InkBlack)
	c.FillEllipse(image.Rect(112, 28, 146, 48), display.InkRed)

	// Polygons and paths fill under the even-odd rule, so a contour inside
	// another leaves a hole rather than filling it.
	c.FillPolygon([]image.Point{{X: 154, Y: 48}, {X: 170, Y: 28}, {X: 186, Y: 48}}, display.InkBlack)

	var arrow display.Path
	arrow.MoveTo(image.Pt(194, 38))
	arrow.LineTo(image.Pt(210, 28))
	arrow.LineTo(image.Pt(206, 38))
	arrow.LineTo(image.Pt(210, 48))
	arrow.Close()
	c.FillPath(arrow, display.InkRed)

	// Pie and chord are the two ways to close an arc: to the centre, or across
	// the ends. Zero degrees points right and a positive sweep turns clockwise,
	// because y grows downward.
	c.FillPie(image.Rect(218, 26, 248, 50), -90, 250, display.InkBlack)
	c.FillChord(image.Rect(254, 26, 288, 50), 20, 200, display.InkRed)

	// A pattern is the only way to put a third value between ink and paper, and
	// it reads as texture rather than as grey at this pixel pitch. Runes not
	// listed leave the frame untouched, so a hatch can overlay what is there.
	hatch, err := display.NewPattern([]string{"x...", ".x..", "..x.", "...x"},
		map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return err
	}
	c.FillPattern(image.Rect(6, 54, 90, 76), hatch)
	c.StrokeRect(image.Rect(6, 54, 90, 76), thin(display.InkBlack))

	// FillPattern only fills a rectangle. Shape it by clipping, which is why
	// there is no patterned variant of the other eight fills.
	var disc display.Path
	disc.Arc(image.Rect(96, 54, 118, 76), 0, 360)
	c.Save()
	c.ClipPath(disc)
	c.FillPattern(image.Rect(96, 54, 118, 76), hatch)
	c.Restore()

	return notes(c, fonts,
		"Set FillRect FillRoundRect FillCircle Ellipse",
		"FillPolygon FillPath FillPie FillChord Pattern",
		"each an exact integer test: in or out, no edge",
	)
}

// strokes covers every call that paints an outline. The contract that matters
// to a layout layer: a closed shape's stroke is drawn inside it, so a bordered
// box never grows past the rectangle you gave it and border widths never need
// to be subtracted from a layout.
func strokes(c *display.Canvas, fonts *display.FontRegistry) error {
	// Proof of the contract rather than an illustration of it: the fill and the
	// stroke describe the same rectangle, and the stroke stays inside.
	c.FillRect(image.Rect(6, 22, 44, 46), display.InkBlack)
	c.StrokeRect(image.Rect(6, 22, 44, 46), display.StrokeStyle{Ink: display.InkRed, Width: 3})

	// Width is a whole number of pixels. Every closed shape accepts any width
	// and clamps to its own size rather than overflowing.
	for i, width := range []int{1, 2, 4} {
		c.StrokeRoundRect(image.Rect(50+i*30, 22, 74+i*30, 46), 5,
			display.StrokeStyle{Ink: display.InkBlack, Width: width})
	}
	c.StrokeCircle(image.Pt(152, 34), 11, thin(display.InkBlack))
	c.StrokeEllipse(image.Rect(168, 22, 202, 46), thin(display.InkRed))
	c.StrokePolygon([]image.Point{{X: 210, Y: 46}, {X: 226, Y: 22}, {X: 242, Y: 46}}, thin(display.InkBlack))

	// A partial sweep is an open curve, so it is centred on its own path; a full
	// sweep closes the ellipse and is stroked inward like StrokeEllipse.
	c.DrawArc(image.Rect(250, 22, 288, 46), 150, 240, thin(display.InkRed))

	// Open paths: no inside, so the brush is centred. Ends and joins are square
	// and cannot be configured, because the stroker stamps a square brush along
	// the line rather than building an outline to fill.
	c.DrawLine(image.Pt(6, 54), image.Pt(48, 54), display.StrokeStyle{Ink: display.InkBlack, Width: 3})
	c.DrawPolyline([]image.Point{{X: 54, Y: 62}, {X: 66, Y: 52}, {X: 78, Y: 62}, {X: 90, Y: 52}},
		thin(display.InkBlack))

	// Dash lengths are distance along the outline, not counts of pixels, so a
	// run keeps its length whatever angle the line runs at. DashOffset advances
	// into the pattern; an odd-length pattern repeats before it cycles.
	dash := display.StrokeStyle{Ink: display.InkRed, Width: 1, Dash: []int{5, 4}}
	for i, degrees := range []int{0, 30, 60, 90} {
		from := image.Pt(100+i*22, 70)
		to := image.Pt(from.X+int(20*cos(degrees)), from.Y-int(20*sin(degrees)))
		c.DrawLine(from, to, dash)
	}

	return notes(c, fonts,
		"StrokeRect RoundRect Circle Ellipse Polygon",
		"StrokePath DrawLine DrawPolyline DrawArc",
		"StrokeStyle{Ink,Width,Dash}; closed in, open on",
	)
}

// paths covers Path, the escape hatch for any outline the named shapes cannot
// express. Curves are flattened to the pixel grid when they are drawn, so a
// Path costs nothing to keep around and can be reused freely.
func paths(c *display.Canvas, fonts *display.FontRegistry) error {
	// MoveTo starts a contour; LineTo, QuadraticTo and CubicTo extend it. On an
	// empty path LineTo behaves as MoveTo rather than failing.
	var wave display.Path
	wave.MoveTo(image.Pt(8, 40))
	wave.LineTo(image.Pt(24, 40))
	wave.QuadraticTo(image.Pt(38, 20), image.Pt(52, 40))
	wave.CubicTo(image.Pt(64, 56), image.Pt(78, 24), image.Pt(92, 40))
	c.StrokePath(wave, thin(display.InkBlack))

	// Arc appends an elliptical arc in the same clockwise degrees DrawArc uses,
	// joining it to the current point if there is one.
	var tab display.Path
	tab.MoveTo(image.Pt(104, 48))
	tab.LineTo(image.Pt(104, 30))
	tab.Arc(image.Rect(104, 22, 128, 46), 180, 90)
	tab.LineTo(image.Pt(140, 22))
	c.StrokePath(tab, thin(display.InkRed))

	// Close joins back to where the contour started and marks it closed, which
	// is what makes StrokePath draw it inward instead of centred.
	var ring display.Path
	for _, r := range []int{22, 12} {
		box := image.Rect(170-r, 36-r, 170+r, 36+r)
		ring.MoveTo(image.Pt(box.Min.X, 36))
		ring.Arc(box, 180, 360)
		ring.Close()
	}
	// Contours are filled together under even-odd, so the inner one cuts a hole.
	c.FillPath(ring, display.InkBlack)

	// Bounds flattens the curves and reports the half-open extent, which is what
	// a layout layer needs to place a path without drawing it first.
	bounds := wave.Bounds()
	if err := label(c, fonts, image.Rect(198, 22, 292, 50), display.AlignStart,
		run(fmt.Sprintf("Bounds\n%d,%d %dx%d", bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy()),
			"monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}

	// Clone is a deep copy; Reset empties in place. Neither is needed before
	// recording into a DisplayList, which snapshots for itself.
	scaled := wave.Clone()
	scaled.LineTo(image.Pt(120, 70))
	c.StrokePath(scaled, display.StrokeStyle{Ink: display.InkRed, Width: 1, Dash: []int{3, 3}})

	return notes(c, fonts,
		"MoveTo LineTo QuadraticTo CubicTo Arc Close",
		"Reset Clone Bounds; curves flatten to the grid",
		"contours fill even-odd, so nesting cuts holes",
	)
}

// texts covers layout. The one rule a layer above must not skip: check
// MissingRunes before a payload reaches the panel. A glyph the strike does not
// have draws a crossed box, and the panel takes ten seconds to tell you.
func texts(c *display.Canvas, fonts *display.FontRegistry) error {
	// Families resolve by name and exact pixel size. There is no scaling and no
	// synthesis: ask for a size that has no strike and layout returns an error
	// rather than approximating it.
	//   ui      12/14/16  Monaco for ASCII, HZK for CJK, baselines aligned
	//   monaco  10/12/14/16
	//   hzk     12/14/16
	if _, err := c.DrawTextBox(fonts, display.TextBox{
		Bounds:     image.Rect(6, 20, 150, 74),
		LineHeight: 17,
		Runs: []display.TextRun{
			// Runs mix freely inside one box; each carries its own face and ink.
			run("ui 16 ", "ui", 16, display.InkBlack),
			run("中英混排\n", "ui", 16, display.InkBlack),
			run("ui 12 ", "ui", 12, display.InkBlack),
			run("行高由 LineHeight 决定\n", "ui", 12, display.InkRed),
			run("monaco 10 0123456789", "monaco", 10, display.InkBlack),
		},
	}); err != nil {
		return err
	}

	// Alignment is per box, horizontally and vertically. WrapRunes breaks
	// between runes and never between words, which suits CJK and is why there
	// is no hyphenation to configure.
	if _, err := c.DrawTextBox(fonts, display.TextBox{
		Bounds:        image.Rect(156, 20, 226, 74),
		LineHeight:    14,
		Align:         display.AlignCenter,
		VerticalAlign: display.AlignMiddle,
		Wrap:          display.WrapRunes,
		Runs:          []display.TextRun{run("居中并按字换行的一段文字", "ui", 12, display.InkBlack)},
	}); err != nil {
		return err
	}

	// LayoutText measures without drawing, which is what a layout layer wants:
	// the result is immutable, reports its own size, and can be drawn later or
	// recorded into a DisplayList as it stands.
	layout, err := display.LayoutText(fonts, display.TextBox{
		Bounds:     image.Rect(232, 20, 292, 74),
		LineHeight: 13,
		Runs:       []display.TextRun{run("measure\nthen draw\n€ absent", "ui", 12, display.InkBlack)},
	})
	if err != nil {
		return err
	}
	layout.Draw(c)
	// The euro sign is not in any bundled strike, so it drew the crossed box
	// visible above and is reported here. A real caller should treat this as a
	// build failure; the count is reported rather than the character, since
	// echoing it would only produce another box.
	size := layout.Size()
	return notes(c, fonts,
		fmt.Sprintf("LayoutText measured %dx%d and found %d rune it",
			size.X, size.Y, len(layout.MissingRunes())),
		"no glyph for, drawn above as a crossed box.",
		"MissingRunes in the build, not on the panel.",
	)
}

// state covers the canvas state stack. Everything here is integer and cheap;
// there is no matrix, so a layout layer composes positions by adding them.
func state(c *display.Canvas, fonts *display.FontRegistry) error {
	// Save and Restore push and pop translation and clipping together. Restore
	// on an empty stack returns false instead of panicking, so a mismatched
	// pair is detectable rather than fatal.
	c.Save()
	c.Translate(image.Pt(8, 22))
	c.StrokeRect(image.Rect(0, 0, 40, 22), thin(display.InkBlack))
	c.Translate(image.Pt(6, 6)) // translations accumulate
	c.FillRect(image.Rect(0, 0, 12, 10), display.InkRed)
	c.Restore()

	// A clip is fixed in frame coordinates the moment it is set. Translating
	// afterwards moves the drawing, not the window it is allowed through.
	c.Save()
	c.ClipRect(image.Rect(60, 22, 110, 76))
	c.Translate(image.Pt(4, 4))
	for i := range 14 {
		c.DrawLine(image.Pt(50+i*6, 80), image.Pt(80+i*6, 10), thin(display.InkBlack))
	}
	c.Restore()
	c.StrokeRect(image.Rect(60, 22, 110, 76), thin(display.InkRed))

	// ClipPath narrows to any region, even-odd, so nested contours cut holes.
	// It applies to shapes, text and images alike, because everything reaches
	// the frame through the same pixel write.
	var star display.Path
	star.Arc(image.Rect(120, 26, 168, 74), 0, 360)
	c.Save()
	c.ClipPath(star)
	c.FillRect(image.Rect(120, 26, 168, 50), display.InkBlack)
	c.FillPattern(image.Rect(120, 50, 168, 74), mustPattern("xx", ".."))
	c.Restore()

	// Clip returns a child canvas over the same frame: it inherits the current
	// translation and clip but owns its state stack, so it cannot corrupt the
	// parent. This is the primitive a widget tree would hand to a child.
	child := c.Clip(image.Rect(180, 22, 240, 76))
	child.Translate(image.Pt(180, 22))
	child.FillRoundRect(image.Rect(2, 2, 56, 30), 4, display.InkRed)
	child.Save()
	child.ClipRect(image.Rect(0, 34, 30, 52))
	child.FillRect(image.Rect(0, 0, 100, 100), display.InkBlack)
	child.Restore()

	return notes(c, fonts,
		"Save Restore Translate ClipRect ClipPath Clip",
		"a clip is fixed in frame coordinates once set;",
		"a later Translate moves what is drawn, not it",
	)
}

// images covers getting a picture onto three inks. DrawImage never inspects
// what it was handed: measurement and choice are separate calls, so a wrong
// choice is visible to the caller instead of buried in drawing.
func images(c *display.Canvas, fonts *display.FontRegistry) error {
	icon, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return err
	}

	// ProfileImage measures the source as the panel will see it, alpha flattened
	// against white. SuggestOptions turns that into a starting point.
	profile, err := display.ProfileImage(icon)
	if err != nil {
		return err
	}

	// Left: what a fixed cut at 128 does to artwork that lives above it.
	c.DrawImage(icon, image.Rect(8, 20, 62, 74), display.ImageOptions{
		Fit: display.FitContain, Sampling: display.SampleBilinear, Dither: display.DitherThreshold,
	})
	c.StrokeRect(image.Rect(8, 20, 62, 74), thin(display.InkBlack))

	// Middle: the suggestion. This icon's shapes differ in hue at one
	// brightness, so the profile asks for tone to be measured against the paper
	// instead. Rewriting tone invalidates the measured cut, which is what
	// SuggestOptionsFor recomputes.
	prepared := image.Image(icon)
	options := profile.SuggestOptions()
	if profile.ColourCarriesStructure {
		if prepared, err = display.ToneByColourDistance(icon); err != nil {
			return err
		}
		if options, err = profile.SuggestOptionsFor(prepared); err != nil {
			return err
		}
	}
	c.DrawImage(prepared, image.Rect(70, 20, 124, 74), options)
	c.StrokeRect(image.Rect(70, 20, 124, 74), thin(display.InkRed))

	// Right: a gradient, which is the other kind of source. Continuous tone
	// wants error diffusion; EnhanceContrast is the pass a dark photograph
	// needs first, and its radius is in source pixels so it has to be scaled to
	// however far the image is about to be reduced.
	gradient := gradientImage(120, 120)
	gradientProfile, err := display.ProfileImage(gradient)
	if err != nil {
		return err
	}
	c.DrawImage(gradient, image.Rect(132, 20, 186, 74), gradientProfile.SuggestOptions())
	c.StrokeRect(image.Rect(132, 20, 186, 74), thin(display.InkBlack))

	if err := label(c, fonts, image.Rect(192, 20, 292, 76), display.AlignStart,
		run(fmt.Sprintf("icon mid %.0f%%\n otsu %d\n colour %v\ngrad mid %.0f%%\n photo %v",
			100*profile.MidToneFraction, profile.Threshold, profile.ColourCarriesStructure,
			100*gradientProfile.MidToneFraction, gradientProfile.Photographic), "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}
	return notes(c, fonts,
		"left: a fixed cut at 128 loses light artwork",
		"middle: SuggestOptionsFor, since a tone rewrite",
		"invalidates the measured cut.  right: diffusion",
	)
}

// displayLists covers recording. A DisplayList is the seam a layout layer
// should build against: it accepts every Canvas call, reports what it would
// cover before anything is drawn, and can be replayed onto any frame.
func displayLists(c *display.Canvas, fonts *display.FontRegistry) error {
	list := &display.DisplayList{}

	// Recording accepts the same calls as a canvas, state included.
	list.Save()
	list.Translate(image.Pt(10, 24))
	list.FillRoundRect(image.Rect(0, 0, 40, 20), 4, display.InkBlack)
	list.StrokeCircle(image.Pt(58, 10), 9, thin(display.InkRed))
	list.Restore()

	// Mutable inputs are copied when recorded: points, dash patterns, paths and
	// images. Changing them afterwards cannot alter what a replay draws, so a
	// caller may reuse one scratch slice for a whole tree.
	points := []image.Point{{X: 90, Y: 44}, {X: 106, Y: 24}, {X: 122, Y: 44}}
	list.FillPolygon(points, display.InkBlack)
	points[1] = image.Pt(0, 0) // has no effect on the recording

	// Bounds is the union of what the commands would cover, in the list's own
	// coordinates and already narrowed by any clip. It is available without
	// drawing, which is what makes it useful for measuring a subtree.
	recorded := list.Bounds()

	// Clone is independent; Reset empties in place and keeps the allocation.
	clone := list.Clone()
	clone.Translate(image.Pt(0, 30))
	clone.FillRect(image.Rect(90, 24, 122, 34), display.InkRed)

	// Replay leaves the target canvas's own translation, clip and save stack
	// exactly as it found them, so a parent can replay a child without
	// defending itself first.
	if err := list.Replay(c); err != nil {
		return err
	}
	if err := clone.Replay(c); err != nil {
		return err
	}

	if err := label(c, fonts, image.Rect(140, 22, 292, 60), display.AlignStart,
		run(fmt.Sprintf("Len %d  Bounds %dx%d\nclone Len %d", list.Len(), recorded.Dx(), recorded.Dy(), clone.Len()),
			"monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}
	return notes(c, fonts,
		"record once, replay onto any frame. Inputs are",
		"copied at record time, Bounds is known before",
		"drawing, Replay leaves target state untouched.",
	)
}

// ---------------------------------------------------------------------------

func thin(ink display.Ink) display.StrokeStyle {
	return display.StrokeStyle{Ink: ink, Width: 1}
}

func run(text, font string, size int, ink display.Ink) display.TextRun {
	return display.TextRun{Text: text, Style: display.TextStyle{Font: font, Size: size, Ink: ink}}
}

// notes writes the explanatory block in the lower half, which every panel
// leaves free. Monaco 10 advances 6px, so 47 characters fit across and three
// lines fit down; anything longer is silently clipped by the text box, which
// is exactly the kind of thing a layout layer above this has to solve.
func notes(c *display.Canvas, fonts *display.FontRegistry, lines ...string) error {
	// A TextBox clips whatever will not fit and says nothing, which is fine for
	// the layer but no good for a reference that is supposed to be readable.
	// Refuse to write a line that would be cut instead of shipping a truncated
	// panel; a layout layer above wants a measured fit, not a silent one.
	const columns, rows = 47, 3
	if len(lines) > rows {
		return fmt.Errorf("notes has %d lines, only %d fit", len(lines), rows)
	}
	runs := make([]display.TextRun, 0, len(lines))
	for index, line := range lines {
		if len([]rune(line)) > columns {
			return fmt.Errorf("notes line %d is %d characters, only %d fit: %q",
				index+1, len([]rune(line)), columns, line)
		}
		if index > 0 {
			line = "\n" + line
		}
		runs = append(runs, run(line, "monaco", 10, display.InkBlack))
	}
	return label(c, fonts, image.Rect(6, 84, 290, 126), display.AlignStart, runs...)
}

func label(c *display.Canvas, fonts *display.FontRegistry, bounds image.Rectangle,
	align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := c.DrawTextBox(fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: 13,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing glyphs %q", string(missing))
	}
	return nil
}

func mustPattern(rows ...string) *display.Pattern {
	pattern, err := display.NewPattern(rows, map[rune]display.Ink{'x': display.InkRed})
	if err != nil {
		panic(err)
	}
	return pattern
}

func gradientImage(width, height int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			level := uint8(min(255, 20+(x+y)*215/(width+height)))
			img.SetNRGBA(x, y, color.NRGBA{R: level, G: level, B: level, A: 0xff})
		}
	}
	return img
}

func cos(degrees int) float64 { return cosTable[degrees] }
func sin(degrees int) float64 { return sinTable[degrees] }

var cosTable = map[int]float64{0: 1, 30: 0.866, 60: 0.5, 90: 0}
var sinTable = map[int]float64{0: 0, 30: 0.5, 60: 0.866, 90: 1}

// stack builds the review sheet. It is not a panel image, so it is free to be
// taller than one.
func stack(frames []*display.Frame) (*display.Frame, error) {
	const gap = 6
	sheet, err := display.NewFrame(296+2*gap, len(frames)*(128+gap)+gap, display.InkWhite)
	if err != nil {
		return nil, err
	}
	canvas := display.NewCanvas(sheet)
	for index, frame := range frames {
		top := gap + index*(128+gap)
		for y := range 128 {
			for x := range 296 {
				ink, _ := frame.InkAt(x, y)
				canvas.Set(gap+x, top+y, ink)
			}
		}
		canvas.StrokeRect(image.Rect(gap, top, gap+296, top+128), thin(display.InkBlack))
	}
	return sheet, nil
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
