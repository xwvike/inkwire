package compose

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func testCompiler(t *testing.T) *Compiler {
	t.Helper()
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	return compiler
}

func compileAndRender(t *testing.T, document Document) (*display.Frame, Report) {
	t.Helper()
	compiled, report, err := testCompiler(t).Compile(document)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	return frame, report
}

func TestDocumentPaintsOnlyExplicitContent(t *testing.T) {
	frame, report := compileAndRender(t, Document{
		Root: Absolute{Children: []Placed{
			{Bounds: image.Rect(5, 7, 15, 12), Node: Rectangle{Fill: Ink(display.InkBlack)}},
		}},
	})

	if got := countInk(frame, display.InkBlack); got != 50 {
		t.Fatalf("black pixels = %d, want exactly the requested 10x5 rectangle", got)
	}
	if got := countInk(frame, display.InkRed); got != 0 {
		t.Fatalf("compiler added %d red pixels", got)
	}
	if report.Bounds != image.Rect(5, 7, 15, 12) {
		t.Fatalf("report bounds = %v, want explicit rectangle", report.Bounds)
	}
}

func TestCompiledDocumentKeepsOrientationAndBackground(t *testing.T) {
	compiled, _, err := testCompiler(t).Compile(Document{
		Orientation: display.OrientationPortraitClockwise,
		Background:  Ink(display.InkRed),
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Bounds().Size(), image.Pt(128, 296); got != want {
		t.Fatalf("page size = %v, want %v", got, want)
	}
	if got := countInk(frame, display.InkRed); got != 128*296 {
		t.Fatalf("red background has %d pixels", got)
	}
}

func TestDocumentWithoutRootIsAnExplicitBackgroundOnlyPage(t *testing.T) {
	frame, report := compileAndRender(t, Document{})
	if got := countInk(frame, display.InkWhite); got != 296*128 {
		t.Fatalf("background-only page has %d white pixels", got)
	}
	if !report.Bounds.Empty() || len(report.Warnings) != 0 {
		t.Fatalf("background-only report = %#v", report)
	}
}

func TestDocumentSizeSupportsNonDevicePreviews(t *testing.T) {
	compiled, _, err := testCompiler(t).Compile(Document{
		Size:       image.Pt(40, 24),
		Background: Ink(display.InkRed),
		Root:       Absolute{Size: image.Pt(40, 24), Children: []Placed{{Bounds: image.Rect(0, 0, 10, 8), Node: Rectangle{Fill: Ink(display.InkBlack)}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Bounds().Size(); got != image.Pt(40, 24) {
		t.Fatalf("custom document size = %v", got)
	}
	if ink, _ := frame.InkAt(20, 20); ink != display.InkRed {
		t.Fatalf("custom preview background = %v, want red", ink)
	}
}

func TestRowAllocatesIntegerGrowthWithoutChangingOrder(t *testing.T) {
	frame, _ := compileAndRender(t, Document{Root: Row{
		Gap: 2,
		Children: []LayoutChild{
			{Basis: 10, Node: Rectangle{Fill: Ink(display.InkBlack)}},
			{Basis: 10, Grow: 1, Node: Rectangle{Fill: Ink(display.InkRed)}},
			{Basis: 10, Grow: 2, Node: Rectangle{Fill: Ink(display.InkBlack)}},
		},
	}})

	for _, test := range []struct {
		x   int
		ink display.Ink
	}{
		{0, display.InkBlack},
		{9, display.InkBlack},
		{10, display.InkWhite},
		{12, display.InkRed},
		{109, display.InkRed},
		{110, display.InkWhite},
		{112, display.InkBlack},
		{295, display.InkBlack},
	} {
		got, _ := frame.InkAt(test.x, 0)
		if got != test.ink {
			t.Fatalf("pixel (%d,0) = %d, want %d", test.x, got, test.ink)
		}
	}
}

func TestColumnPaddingStackAndSpacerAreMechanical(t *testing.T) {
	frame, _ := compileAndRender(t, Document{Root: Padding{
		Insets: Insets{Top: 4, Right: 5, Bottom: 6, Left: 7},
		Child: Column{Gap: 3, Children: []LayoutChild{
			{Basis: 10, Node: Stack{Children: []Node{
				Rectangle{Fill: Ink(display.InkBlack)},
				Absolute{Children: []Placed{{Bounds: image.Rect(2, 2, 6, 6), Node: Rectangle{Fill: Ink(display.InkRed)}}}},
			}}},
			{Basis: 5, Node: Spacer{}},
			{Grow: 1, Node: Rectangle{Fill: Ink(display.InkRed)}},
		}},
	}})

	assertFrameInk(t, frame, 7, 4, display.InkBlack)
	assertFrameInk(t, frame, 9, 6, display.InkRed)
	assertFrameInk(t, frame, 7, 15, display.InkWhite)
	assertFrameInk(t, frame, 7, 25, display.InkRed)
	assertFrameInk(t, frame, 1, 1, display.InkWhite)
}

func TestTextReportsMissingRunesWithoutChangingText(t *testing.T) {
	frame, report := compileAndRender(t, Document{Root: Absolute{Children: []Placed{
		{Bounds: image.Rect(0, 0, 80, 20), Node: Text{Runs: []display.TextRun{{
			Text: "A😀B", Style: display.TextStyle{Font: "ui", Size: 12, Ink: display.InkBlack},
		}}}},
	}}})
	if got := string(report.MissingRunes); got != "😀" {
		t.Fatalf("missing runes = %q, want the unmodified missing input", got)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Code != "missing-runes" {
		t.Fatalf("warnings = %#v", report.Warnings)
	}
	if countInk(frame, display.InkBlack) == 0 {
		t.Fatal("text painted nothing")
	}
}

func TestEmptyTextIsAnExplicitNoOp(t *testing.T) {
	frame, report := compileAndRender(t, Document{Root: Absolute{Children: []Placed{
		{Bounds: image.Rect(0, 0, 40, 20), Node: Text{}},
	}}})
	if got := countInk(frame, display.InkBlack); got != 0 {
		t.Fatalf("empty text painted %d pixels", got)
	}
	if len(report.Warnings) != 0 || len(report.MissingRunes) != 0 {
		t.Fatalf("empty text report = %#v", report)
	}
}

func TestAutoImageRecordsDecisionAndHonoursOverrides(t *testing.T) {
	source := solidImage(32, 32, func(x, y int) color.NRGBA {
		return color.NRGBA{R: uint8(40 + x*5), G: uint8(40 + x*5), B: uint8(40 + x*5), A: 0xff}
	})
	dither := display.DitherOrdered
	disableRed := true
	_, report := compileAndRender(t, Document{Root: Absolute{Children: []Placed{
		{Bounds: image.Rect(0, 0, 32, 32), Node: Image{
			Source: source, Processing: ImageAuto,
			Overrides: ImageOverrides{Dither: &dither, DisableRed: &disableRed},
		}},
	}}})
	if len(report.Images) != 1 {
		t.Fatalf("image decisions = %d, want 1", len(report.Images))
	}
	decision := report.Images[0]
	if decision.Options.Dither != display.DitherOrdered || !decision.Options.DisableRed {
		t.Fatalf("automatic decision ignored explicit overrides: %#v", decision.Options)
	}
	if decision.Options.Threshold == 0 || decision.Options.RedThreshold == 0 || decision.Options.RedMaxGreen == 0 {
		t.Fatalf("automatic decision did not report concrete thresholds: %#v", decision.Options)
	}
	if decision.Path != "root.children[0]" {
		t.Fatalf("decision path = %q", decision.Path)
	}
}

func TestManualImageDoesNotProfileContent(t *testing.T) {
	source := solidImage(8, 8, func(x, y int) color.NRGBA {
		return color.NRGBA{R: 0xff, G: 0x20, B: 0x20, A: 0xff}
	})
	frame, report := compileAndRender(t, Document{Root: Absolute{Children: []Placed{
		{Bounds: image.Rect(0, 0, 8, 8), Node: Image{
			Source: source, Options: display.ImageOptions{DisableRed: true},
		}},
	}}})
	if len(report.Images) != 0 {
		t.Fatalf("manual image produced automatic decisions: %#v", report.Images)
	}
	if got := countInk(frame, display.InkRed); got != 0 {
		t.Fatalf("manual DisableRed still painted %d red pixels", got)
	}
}

func TestAllPrimitiveNodesCompileAndPaint(t *testing.T) {
	pattern, err := display.NewPattern([]string{"x."}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		t.Fatal(err)
	}
	var path display.Path
	path.MoveTo(image.Pt(0, 9))
	path.LineTo(image.Pt(5, 0))
	path.LineTo(image.Pt(10, 9))
	path.Close()
	black1 := display.StrokeStyle{Ink: display.InkBlack, Width: 1}
	red := display.InkRed
	black := display.InkBlack

	nodes := []Placed{
		{image.Rect(2, 2, 3, 3), Pixel{Ink: black}},
		{image.Rect(5, 2, 20, 12), Rectangle{Fill: &black, Stroke: &black1}},
		{image.Rect(22, 2, 42, 12), Rectangle{Radius: 3, Fill: &red}},
		{image.Rect(44, 2, 64, 12), Line{From: image.Pt(0, 0), To: image.Pt(19, 9), Stroke: black1}},
		{image.Rect(66, 2, 86, 12), Polyline{Points: []image.Point{{0, 9}, {10, 0}, {19, 9}}, Stroke: black1}},
		{image.Rect(88, 2, 108, 12), Polygon{Points: []image.Point{{0, 9}, {10, 0}, {19, 9}}, Fill: &red}},
		{image.Rect(110, 2, 132, 24), Circle{Center: image.Pt(10, 10), Radius: 10, Stroke: &black1}},
		{image.Rect(134, 2, 158, 24), Ellipse{Fill: &black}},
		{image.Rect(160, 2, 184, 24), Arc{Start: 0, Sweep: 270, Stroke: black1}},
		{image.Rect(186, 2, 210, 24), Pie{Start: -90, Sweep: 210, Ink: red}},
		{image.Rect(212, 2, 236, 24), Chord{Start: 0, Sweep: 180, Ink: black}},
		{image.Rect(238, 2, 250, 14), Path{Path: path, Fill: &red}},
		{image.Rect(252, 2, 272, 14), Pattern{Pattern: pattern}},
		{image.Rect(2, 30, 22, 50), ClipRect{Rect: image.Rect(5, 5, 15, 15), Child: Rectangle{Fill: &black}}},
		{image.Rect(24, 30, 44, 50), ClipPath{Path: path, Child: Rectangle{Fill: &red}}},
	}
	frame, _ := compileAndRender(t, Document{Root: Absolute{Children: nodes}})
	if countInk(frame, display.InkBlack) == 0 || countInk(frame, display.InkRed) == 0 {
		t.Fatal("primitive document did not paint both requested inks")
	}
	assertFrameInk(t, frame, 3, 33, display.InkWhite)
	assertFrameInk(t, frame, 8, 38, display.InkBlack)
}

func TestCompilerRejectsAmbiguousImageModesAndReportsPaths(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	_, _, err := testCompiler(t).Compile(Document{Root: Absolute{Children: []Placed{
		{Bounds: image.Rect(0, 0, 1, 1), Node: Image{
			Source: source, Processing: ImageAuto,
			Options: display.ImageOptions{DisableRed: true},
		}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "root.children[0]") {
		t.Fatalf("error = %v, want the exact failing node path", err)
	}
}

func TestCompilerRejectsTypedNilNodes(t *testing.T) {
	var root *Spacer
	_, _, err := testCompiler(t).Compile(Document{Root: root})
	if err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("typed nil root error = %v", err)
	}
}

func TestExcessivePaddingDoesNotFlipIntoDrawableBounds(t *testing.T) {
	frame, report := compileAndRender(t, Document{Root: Padding{
		Insets: Insets{Top: 100, Right: 200, Bottom: 100, Left: 200},
		Child:  Rectangle{Fill: Ink(display.InkBlack)},
	}})
	if got := countInk(frame, display.InkBlack); got != 0 {
		t.Fatalf("excessive padding painted %d pixels", got)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Code != "empty-layout" {
		t.Fatalf("warnings = %#v", report.Warnings)
	}
}

func assertFrameInk(t *testing.T, frame *display.Frame, x, y int, want display.Ink) {
	t.Helper()
	got, ok := frame.InkAt(x, y)
	if !ok || got != want {
		t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got, want)
	}
}

// The clipping check lives in display; this is the wiring that turns it into
// something a caller sees. Without a test here, deleting the wiring leaves
// every other test passing while the panel quietly shows a wrong number.
func TestClippedTextIsReported(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	document := Document{Root: Absolute{Children: []Placed{{
		// Ten characters of Monaco 12 need seventy pixels.
		Bounds: image.Rect(0, 0, 40, 15),
		Node: Text{Runs: []display.TextRun{{
			Text:  "3260/3720G",
			Style: display.TextStyle{Font: "monaco", Size: 12, Ink: display.InkBlack},
		}}},
	}}}}
	_, report, err := compiler.Compile(document)
	if err != nil {
		t.Fatal(err)
	}
	var clipped *Warning
	for index, warning := range report.Warnings {
		if warning.Code == "text-clipped" {
			clipped = &report.Warnings[index]
		}
	}
	if clipped == nil {
		t.Fatalf("no text-clipped warning; report has %v", report.Warnings)
	}
	if !strings.Contains(clipped.Message, "3260/3720G") {
		t.Errorf("the warning does not name the text that was cut: %q", clipped.Message)
	}
	if clipped.Path == "" {
		t.Error("the warning does not say which node was cut")
	}
}

// Text that fits must stay silent, or the warning is noise and gets ignored.
func TestTextThatFitsIsNotReported(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	document := Document{Root: Absolute{Children: []Placed{{
		Bounds: image.Rect(0, 0, 200, 15),
		Node: Text{Runs: []display.TextRun{{
			Text:  "3260/3720G",
			Style: display.TextStyle{Font: "monaco", Size: 12, Ink: display.InkBlack},
		}}},
	}}}}
	_, report, err := compiler.Compile(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range report.Warnings {
		if warning.Code == "text-clipped" {
			t.Errorf("text that fits was reported as clipped: %s", warning.Message)
		}
	}
}
