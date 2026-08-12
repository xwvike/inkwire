package display

import (
	"fmt"
	"image"
	"image/draw"
	"slices"
)

// DisplayList is a reusable sequence of deterministic Canvas operations.
// Mutable inputs are copied when recorded, so later caller changes cannot
// alter replay output.
type DisplayList struct {
	commands  []displayCommand
	state     displayListState
	stack     []displayListState
	bounds    image.Rectangle
	hasBounds bool
}

type displayListState struct {
	offset  image.Point
	clip    image.Rectangle
	hasClip bool
}

type commandKind uint8

const (
	commandSave commandKind = iota
	commandRestore
	commandClipRect
	commandClipPath
	commandTranslate
	commandSet
	commandFillRect
	commandStrokeRect
	commandDrawLine
	commandDrawPolyline
	commandStrokePolygon
	commandFillPolygon
	commandFillCircle
	commandStrokeCircle
	commandFillEllipse
	commandStrokeEllipse
	commandFillRoundRect
	commandStrokeRoundRect
	commandDrawArc
	commandFillPie
	commandFillChord
	commandStrokePath
	commandFillPath
	commandDrawTextLayout
	commandDrawImage
)

type displayCommand struct {
	kind    commandKind
	point   image.Point
	point2  image.Point
	rect    image.Rectangle
	points  []image.Point
	ink     Ink
	stroke  StrokeStyle
	radius  int
	start   float64
	sweep   float64
	path    Path
	layout  *TextLayout
	source  *image.NRGBA
	options ImageOptions
}

// Len returns the number of recorded state and drawing commands.
func (d *DisplayList) Len() int {
	return len(d.commands)
}

// Bounds returns the half-open union of effective drawing bounds in the
// display list's root coordinates, after translation and clipping.
func (d *DisplayList) Bounds() image.Rectangle {
	if !d.hasBounds {
		return image.Rectangle{}
	}
	return d.bounds
}

// Reset removes all commands and restores the initial recording state.
func (d *DisplayList) Reset() {
	*d = DisplayList{}
}

// Clone returns an independently appendable copy of d.
func (d *DisplayList) Clone() *DisplayList {
	if d == nil {
		return nil
	}
	clone := *d
	clone.commands = make([]displayCommand, len(d.commands))
	for index, command := range d.commands {
		clone.commands[index] = command.clone()
	}
	clone.stack = slices.Clone(d.stack)
	return &clone
}

// Replay executes the list against canvas without changing canvas's current
// translation, clipping state, or save stack.
func (d *DisplayList) Replay(canvas *Canvas) error {
	if d == nil {
		return fmt.Errorf("display list must not be nil")
	}
	if canvas == nil {
		return fmt.Errorf("canvas must not be nil")
	}
	replayCanvas := &Canvas{frame: canvas.frame, state: canvas.state}
	for index, command := range d.commands {
		if err := command.replay(replayCanvas); err != nil {
			return fmt.Errorf("replay command %d: %w", index, err)
		}
	}
	return nil
}

// Save records and pushes the current translation and clipping state.
func (d *DisplayList) Save() {
	d.stack = append(d.stack, d.state)
	d.commands = append(d.commands, displayCommand{kind: commandSave})
}

// Restore records a state pop. It returns false and records nothing when the
// display list's state stack is empty.
func (d *DisplayList) Restore() bool {
	if len(d.stack) == 0 {
		return false
	}
	last := len(d.stack) - 1
	d.state = d.stack[last]
	d.stack = d.stack[:last]
	d.commands = append(d.commands, displayCommand{kind: commandRestore})
	return true
}

// ClipRect records a clip in the current logical coordinates.
func (d *DisplayList) ClipRect(rect image.Rectangle) {
	deviceRect := rect.Add(d.state.offset)
	if d.state.hasClip {
		d.state.clip = d.state.clip.Intersect(deviceRect)
	} else {
		d.state.clip = deviceRect
		d.state.hasClip = true
	}
	d.commands = append(d.commands, displayCommand{kind: commandClipRect, rect: rect})
}

// ClipPath records a clip to the region path covers under the even-odd rule.
// Recorded bounds narrow only to the path's bounding rectangle, which stays a
// safe over-estimate of what replay can paint.
func (d *DisplayList) ClipPath(path Path) {
	path = path.Clone()
	deviceRect := path.Bounds().Add(d.state.offset)
	if d.state.hasClip {
		d.state.clip = d.state.clip.Intersect(deviceRect)
	} else {
		d.state.clip = deviceRect
		d.state.hasClip = true
	}
	d.commands = append(d.commands, displayCommand{kind: commandClipPath, path: path})
}

// Translate records an integer origin offset for subsequent commands.
func (d *DisplayList) Translate(offset image.Point) {
	d.state.offset = d.state.offset.Add(offset)
	d.commands = append(d.commands, displayCommand{kind: commandTranslate, point: offset})
}

// Set records one pixel write.
func (d *DisplayList) Set(x, y int, ink Ink) {
	if !ink.valid() {
		return
	}
	point := image.Pt(x, y)
	d.appendDraw(displayCommand{kind: commandSet, point: point, ink: ink}, image.Rectangle{Min: point, Max: point.Add(image.Pt(1, 1))})
}

// FillRect records a filled half-open rectangle.
func (d *DisplayList) FillRect(rect image.Rectangle, ink Ink) {
	if rect.Empty() || !ink.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillRect, rect: rect, ink: ink}, rect)
}

// StrokeRect records a rectangle outline drawn inside rect.
func (d *DisplayList) StrokeRect(rect image.Rectangle, stroke StrokeStyle) {
	if rect.Empty() || !stroke.valid() {
		return
	}
	if len(stroke.Dash) > 0 && strokeCenterRect(rect, stroke.Width).Empty() {
		return
	}
	d.appendDraw(displayCommand{kind: commandStrokeRect, rect: rect, stroke: cloneStroke(stroke)}, rect)
}

// DrawLine records an inclusive line between two points.
func (d *DisplayList) DrawLine(from, to image.Point, stroke StrokeStyle) {
	if !stroke.valid() {
		return
	}
	bounds := strokedPointBounds(polygonBounds([]image.Point{from, to}), stroke.Width)
	d.appendDraw(displayCommand{kind: commandDrawLine, point: from, point2: to, stroke: cloneStroke(stroke)}, bounds)
}

// DrawPolyline records an open sequence of connected lines.
func (d *DisplayList) DrawPolyline(points []image.Point, stroke StrokeStyle) {
	if len(points) < 2 || !stroke.valid() {
		return
	}
	points = slices.Clone(points)
	bounds := strokedPointBounds(polygonBounds(points), stroke.Width)
	d.appendDraw(displayCommand{kind: commandDrawPolyline, points: points, stroke: cloneStroke(stroke)}, bounds)
}

// StrokePolygon records a closed polygon outline.
func (d *DisplayList) StrokePolygon(points []image.Point, stroke StrokeStyle) {
	if len(points) < 3 || !stroke.valid() {
		return
	}
	points = slices.Clone(points)
	bounds := strokedPointBounds(polygonBounds(points), stroke.Width)
	d.appendDraw(displayCommand{kind: commandStrokePolygon, points: points, stroke: cloneStroke(stroke)}, bounds)
}

// FillPolygon records an even-odd polygon fill.
func (d *DisplayList) FillPolygon(points []image.Point, ink Ink) {
	if len(points) < 3 || !ink.valid() {
		return
	}
	points = slices.Clone(points)
	d.appendDraw(displayCommand{kind: commandFillPolygon, points: points, ink: ink}, polygonBounds(points))
}

// FillCircle records a filled integer-radius circle.
func (d *DisplayList) FillCircle(center image.Point, radius int, ink Ink) {
	if radius < 0 || !ink.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillCircle, point: center, radius: radius, ink: ink}, circleBounds(center, radius))
}

// StrokeCircle records a circle outline inside its bounding box.
func (d *DisplayList) StrokeCircle(center image.Point, radius int, stroke StrokeStyle) {
	if radius < 0 || !stroke.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandStrokeCircle, point: center, radius: radius, stroke: cloneStroke(stroke)}, circleBounds(center, radius))
}

// FillEllipse records an ellipse fill inside bounds.
func (d *DisplayList) FillEllipse(bounds image.Rectangle, ink Ink) {
	if bounds.Empty() || !ink.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillEllipse, rect: bounds, ink: ink}, bounds)
}

// StrokeEllipse records an ellipse outline inside bounds.
func (d *DisplayList) StrokeEllipse(bounds image.Rectangle, stroke StrokeStyle) {
	if bounds.Empty() || !stroke.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandStrokeEllipse, rect: bounds, stroke: cloneStroke(stroke)}, bounds)
}

// FillRoundRect records a filled rounded rectangle.
func (d *DisplayList) FillRoundRect(rect image.Rectangle, radius int, ink Ink) {
	if rect.Empty() || !ink.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillRoundRect, rect: rect, radius: radius, ink: ink}, rect)
}

// StrokeRoundRect records a rounded rectangle outline.
func (d *DisplayList) StrokeRoundRect(rect image.Rectangle, radius int, stroke StrokeStyle) {
	if rect.Empty() || !stroke.valid() {
		return
	}
	d.appendDraw(displayCommand{kind: commandStrokeRoundRect, rect: rect, radius: radius, stroke: cloneStroke(stroke)}, rect)
}

// DrawArc records an elliptical arc using clockwise screen-space degrees.
func (d *DisplayList) DrawArc(bounds image.Rectangle, startDegrees, sweepDegrees float64, stroke StrokeStyle) {
	points, _ := ellipseArcPoints(bounds, startDegrees, sweepDegrees)
	if len(points) == 0 || !stroke.valid() {
		return
	}
	paintBounds := strokedPointBounds(polygonBounds(points), stroke.Width)
	d.appendDraw(displayCommand{
		kind: commandDrawArc, rect: bounds, start: startDegrees, sweep: sweepDegrees, stroke: cloneStroke(stroke),
	}, paintBounds)
}

// FillPie records a filled elliptical sector.
func (d *DisplayList) FillPie(bounds image.Rectangle, startDegrees, sweepDegrees float64, ink Ink) {
	points, closed := ellipseArcPoints(bounds, startDegrees, sweepDegrees)
	if len(points) == 0 || !ink.valid() {
		return
	}
	paintBounds := polygonBounds(points)
	if closed {
		paintBounds = bounds
	} else if len(points) >= 2 {
		center := image.Pt((bounds.Min.X+bounds.Max.X-1)/2, (bounds.Min.Y+bounds.Max.Y-1)/2)
		paintBounds = paintBounds.Union(image.Rectangle{Min: center, Max: center.Add(image.Pt(1, 1))})
	}
	d.appendDraw(displayCommand{kind: commandFillPie, rect: bounds, start: startDegrees, sweep: sweepDegrees, ink: ink}, paintBounds)
}

// FillChord records the fill between an elliptical arc and its endpoint chord.
func (d *DisplayList) FillChord(bounds image.Rectangle, startDegrees, sweepDegrees float64, ink Ink) {
	points, closed := ellipseArcPoints(bounds, startDegrees, sweepDegrees)
	if len(points) == 0 || len(points) == 2 || !ink.valid() {
		return
	}
	paintBounds := polygonBounds(points)
	if closed {
		paintBounds = bounds
	}
	d.appendDraw(displayCommand{kind: commandFillChord, rect: bounds, start: startDegrees, sweep: sweepDegrees, ink: ink}, paintBounds)
}

// StrokePath records all contours in path.
func (d *DisplayList) StrokePath(path Path, stroke StrokeStyle) {
	if path.Empty() || !stroke.valid() {
		return
	}
	path = path.Clone()
	paintBounds := pathContourBounds(path.flatten(), 2)
	if paintBounds.Empty() {
		return
	}
	paintBounds = strokedPointBounds(paintBounds, stroke.Width)
	d.appendDraw(displayCommand{kind: commandStrokePath, path: path, stroke: cloneStroke(stroke)}, paintBounds)
}

// FillPath records an even-odd fill of all contours in path.
func (d *DisplayList) FillPath(path Path, ink Ink) {
	if path.Empty() || !ink.valid() {
		return
	}
	path = path.Clone()
	paintBounds := pathContourBounds(path.flatten(), 3)
	if paintBounds.Empty() {
		return
	}
	d.appendDraw(displayCommand{kind: commandFillPath, path: path, ink: ink}, paintBounds)
}

// DrawTextLayout records an already measured immutable text layout.
func (d *DisplayList) DrawTextLayout(layout *TextLayout) error {
	if layout == nil {
		return fmt.Errorf("text layout must not be nil")
	}
	if len(layout.lines) == 0 {
		return nil
	}
	d.appendDraw(displayCommand{kind: commandDrawTextLayout, layout: layout}, layout.box.Bounds)
	return nil
}

// DrawTextBox lays out and records text in one call.
func (d *DisplayList) DrawTextBox(registry *FontRegistry, box TextBox) (*TextLayout, error) {
	layout, err := LayoutText(registry, box)
	if err != nil {
		return nil, err
	}
	if err := d.DrawTextLayout(layout); err != nil {
		return nil, err
	}
	return layout, nil
}

// DrawImage validates and snapshots source before recording it.
func (d *DisplayList) DrawImage(source image.Image, destination image.Rectangle, options ImageOptions) error {
	if source == nil {
		return fmt.Errorf("source image must not be nil")
	}
	if destination.Empty() {
		return fmt.Errorf("destination image bounds must not be empty")
	}
	if source.Bounds().Empty() {
		return fmt.Errorf("source image bounds must not be empty")
	}
	normalized, err := normalizeImageOptions(options)
	if err != nil {
		return err
	}
	snapshot := image.NewNRGBA(source.Bounds())
	draw.Draw(snapshot, snapshot.Bounds(), source, source.Bounds().Min, draw.Src)
	target := fitImage(snapshot.Bounds(), destination, normalized.Fit).target
	d.appendDraw(displayCommand{
		kind: commandDrawImage, rect: destination, source: snapshot, options: normalized,
	}, target)
	return nil
}

func (d *DisplayList) appendDraw(command displayCommand, bounds image.Rectangle) {
	d.commands = append(d.commands, command)
	bounds = bounds.Add(d.state.offset)
	if d.state.hasClip {
		bounds = bounds.Intersect(d.state.clip)
	}
	if bounds.Empty() {
		return
	}
	if d.hasBounds {
		d.bounds = d.bounds.Union(bounds)
	} else {
		d.bounds = bounds
		d.hasBounds = true
	}
}

func (c displayCommand) clone() displayCommand {
	c.points = slices.Clone(c.points)
	c.stroke = cloneStroke(c.stroke)
	c.path = c.path.Clone()
	if c.source != nil {
		snapshot := image.NewNRGBA(c.source.Bounds())
		draw.Draw(snapshot, snapshot.Bounds(), c.source, c.source.Bounds().Min, draw.Src)
		c.source = snapshot
	}
	return c
}

func (c displayCommand) replay(canvas *Canvas) error {
	switch c.kind {
	case commandSave:
		canvas.Save()
	case commandRestore:
		if !canvas.Restore() {
			return fmt.Errorf("restore without matching save")
		}
	case commandClipRect:
		canvas.ClipRect(c.rect)
	case commandClipPath:
		canvas.ClipPath(c.path)
	case commandTranslate:
		canvas.Translate(c.point)
	case commandSet:
		canvas.Set(c.point.X, c.point.Y, c.ink)
	case commandFillRect:
		canvas.FillRect(c.rect, c.ink)
	case commandStrokeRect:
		canvas.StrokeRect(c.rect, c.stroke)
	case commandDrawLine:
		canvas.DrawLine(c.point, c.point2, c.stroke)
	case commandDrawPolyline:
		canvas.DrawPolyline(c.points, c.stroke)
	case commandStrokePolygon:
		canvas.StrokePolygon(c.points, c.stroke)
	case commandFillPolygon:
		canvas.FillPolygon(c.points, c.ink)
	case commandFillCircle:
		canvas.FillCircle(c.point, c.radius, c.ink)
	case commandStrokeCircle:
		canvas.StrokeCircle(c.point, c.radius, c.stroke)
	case commandFillEllipse:
		canvas.FillEllipse(c.rect, c.ink)
	case commandStrokeEllipse:
		canvas.StrokeEllipse(c.rect, c.stroke)
	case commandFillRoundRect:
		canvas.FillRoundRect(c.rect, c.radius, c.ink)
	case commandStrokeRoundRect:
		canvas.StrokeRoundRect(c.rect, c.radius, c.stroke)
	case commandDrawArc:
		canvas.DrawArc(c.rect, c.start, c.sweep, c.stroke)
	case commandFillPie:
		canvas.FillPie(c.rect, c.start, c.sweep, c.ink)
	case commandFillChord:
		canvas.FillChord(c.rect, c.start, c.sweep, c.ink)
	case commandStrokePath:
		canvas.StrokePath(c.path, c.stroke)
	case commandFillPath:
		canvas.FillPath(c.path, c.ink)
	case commandDrawTextLayout:
		c.layout.Draw(canvas)
	case commandDrawImage:
		return canvas.DrawImage(c.source, c.rect, c.options)
	default:
		return fmt.Errorf("unknown display command %d", c.kind)
	}
	return nil
}

func cloneStroke(stroke StrokeStyle) StrokeStyle {
	stroke.Dash = slices.Clone(stroke.Dash)
	return stroke
}

func strokedPointBounds(bounds image.Rectangle, width int) image.Rectangle {
	if bounds.Empty() || width <= 0 {
		return image.Rectangle{}
	}
	negative := width / 2
	positive := width - negative - 1
	return image.Rectangle{
		Min: bounds.Min.Sub(image.Pt(negative, negative)),
		Max: bounds.Max.Add(image.Pt(positive, positive)),
	}
}

func pathContourBounds(contours []pathContour, minimumPoints int) image.Rectangle {
	var bounds image.Rectangle
	for _, contour := range contours {
		if len(contour.points) < minimumPoints {
			continue
		}
		contourBounds := polygonBounds(contour.points)
		if bounds.Empty() {
			bounds = contourBounds
		} else {
			bounds = bounds.Union(contourBounds)
		}
	}
	return bounds
}
