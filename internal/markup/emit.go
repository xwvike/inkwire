package markup

import (
	"encoding/json"
	"fmt"
	stdimage "image"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

// This file is the whole of what this package produces. A page compiles to a
// scene document and stops there: it does not lay anything out, does not
// measure a glyph, does not open a picture, and does not build a single node
// the drawing model would recognise.
//
// That division is the point. A stylesheet is a second way of writing a scene
// document, not a second renderer, and the surest way to keep it one is to
// give it no way of reaching the panel except the schema everything else goes
// through. Every guard the decoder has — the pixel limit on an image, the
// confinement of a path, the refusal of a field nobody implements — is a
// guard this front end gets for free, and cannot be written a second time
// slightly differently.
//
// # Why one struct and not twenty
//
// emitted is a union of every field the layout half of the schema uses, so a
// rectangle's radius and a text's runs live on the same type. Nothing here
// stops a radius being put on a text.
//
// The decoder stops it. It reads each node into the struct for its own type
// with unknown fields refused, so a field on the wrong node is an error with
// the node's path in it, raised before anything is drawn. Writing the twenty
// structs again here would be writing the schema twice and checking neither.

// emitted is one node of the document being written.
type emitted struct {
	Type string `json:"type"`
	Size *size  `json:"size,omitempty"`
	// Clip belongs beside the box it applies to, which is where the documents
	// written by hand put it.
	Clip bool `json:"clip,omitempty"`

	// Children is a slice whose element type depends on Type: nodes under a
	// stack, items under a row or column, cells under a grid, anchors under
	// an anchored box.
	Children any `json:"children,omitempty"`

	Insets *insets `json:"insets,omitempty"`

	Radius int     `json:"radius,omitempty"`
	Fill   string  `json:"fill,omitempty"`
	Stroke *stroke `json:"stroke,omitempty"`

	Shape *shape `json:"shape,omitempty"`

	Scale int `json:"scale,omitempty"`
	Turns int `json:"turns,omitempty"`

	Runs          []run  `json:"runs,omitempty"`
	Align         string `json:"align,omitempty"`
	VerticalAlign string `json:"verticalAlign,omitempty"`
	Wrap          string `json:"wrap,omitempty"`
	LineHeight    int    `json:"lineHeight,omitempty"`

	Source     string     `json:"source,omitempty"`
	Processing string     `json:"processing,omitempty"`
	Overrides  *overrides `json:"overrides,omitempty"`

	// The drawing half. A page says none of these; they come from an svg
	// element, where the vocabulary is geometry rather than boxes.
	At       *point            `json:"at,omitempty"`
	Ink      string            `json:"ink,omitempty"`
	From     *point            `json:"from,omitempty"`
	To       *point            `json:"to,omitempty"`
	Points   []point           `json:"points,omitempty"`
	Center   *point            `json:"center,omitempty"`
	Start    float64           `json:"start,omitempty"`
	Sweep    float64           `json:"sweep,omitempty"`
	Inks     map[string]string `json:"inks,omitempty"`
	Commands []command         `json:"commands,omitempty"`
	Rect     *rect             `json:"rect,omitempty"`
	Degrees  float64           `json:"degrees,omitempty"`
	Origin   *origin           `json:"origin,omitempty"`
	Path     *pathValue        `json:"path,omitempty"`

	// Last, so that a node that wraps another reads as what it does before
	// what it does it to: a padding states its insets and then its child.
	Child *emitted `json:"child,omitempty"`

	Gap        int    `json:"gap,omitempty"`
	MainAlign  string `json:"mainAlign,omitempty"`
	CrossAlign string `json:"crossAlign,omitempty"`

	Columns      []any  `json:"columns,omitempty"`
	Rows         []any  `json:"rows,omitempty"`
	ColumnGap    int    `json:"columnGap,omitempty"`
	RowGap       int    `json:"rowGap,omitempty"`
	AlignItems   string `json:"alignItems,omitempty"`
	JustifyItems string `json:"justifyItems,omitempty"`

	// Raw is the description a scene element handed over, spliced in whole.
	// A page that embeds a drawing embeds the drawing, so what leaves here is
	// one document rather than a document and a promise.
	Raw json.RawMessage `json:"-"`
}

// MarshalJSON writes a spliced-in description as itself. Everything else is
// the struct above.
func (n *emitted) MarshalJSON() ([]byte, error) {
	if n.Raw != nil {
		return n.Raw, nil
	}
	type plain emitted
	return json.Marshal((*plain)(n))
}

type document struct {
	Version     int      `json:"version"`
	Orientation string   `json:"orientation,omitempty"`
	Size        *size    `json:"size,omitempty"`
	Background  string   `json:"background,omitempty"`
	Root        *emitted `json:"root,omitempty"`
}

type size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type insets struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

type stroke struct {
	Ink        string `json:"ink"`
	Width      int    `json:"width"`
	Dash       []int  `json:"dash,omitempty"`
	DashOffset int    `json:"dashOffset,omitempty"`
}

type run struct {
	Text string `json:"text"`
	Font string `json:"font,omitempty"`
	Size int    `json:"size,omitempty"`
	Ink  string `json:"ink,omitempty"`
}

type overrides struct {
	Fit string `json:"fit,omitempty"`
}

// sizeOf and insetsOf carry the two structures compose states in pixels.
func sizeOf(p stdimage.Point) *size {
	if p == (stdimage.Point{}) {
		return nil
	}
	return &size{Width: p.X, Height: p.Y}
}

func insetsOf(i compose.Insets) *insets {
	if i == (compose.Insets{}) {
		return nil
	}
	return &insets{Top: i.Top, Right: i.Right, Bottom: i.Bottom, Left: i.Left}
}

type point struct {
	X any `json:"x"`
	Y any `json:"y"`
}

// rect is where an absolutely placed child sits. Unlike a length it is always
// a whole number of pixels: a drawing states where a thing is, and there is
// nothing for it to be a percentage of.
type rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// pathValue is the outline a clipPath clips to. It is not a node — nothing is
// drawn — so it carries the commands and nothing else.
type pathValue struct {
	Commands []command `json:"commands"`
}

// origin is the point a turn happens about, in lengths because it is written
// as a share of the box more often than as a distance into it.
type origin struct {
	X any `json:"x,omitempty"`
	Y any `json:"y,omitempty"`
}

// placed is one child of an absolute box.
type placed struct {
	Bounds rect     `json:"bounds"`
	Node   *emitted `json:"node"`
}

// pixels writes a coordinate. A drawing may state one as a fraction and the
// panel has no fractions, so it rounds — which is what any renderer does with
// a coordinate finer than the thing it is drawing on.
func pixels(value float64) int { return int(math.Round(value)) }

// at is a point in a drawing's own coordinates.
func at(x, y float64) *point { return &point{X: pixels(x), Y: pixels(y)} }

// atFrame is a point written in a group's coordinates, put into the drawing's.
func atFrame(frame svgFrame, x, y float64) *point {
	placed, alsoPlaced := frame.place(x, y)
	return at(placed, alsoPlaced)
}

type shape struct {
	Kind    string  `json:"kind"`
	Insets  []any   `json:"insets,omitempty"`
	Corner  any     `json:"corner,omitempty"`
	Radius  any     `json:"radius,omitempty"`
	RadiusX any     `json:"radiusX,omitempty"`
	RadiusY any     `json:"radiusY,omitempty"`
	Center  *point  `json:"center,omitempty"`
	Points  []point `json:"points,omitempty"`
}

// layoutChild is one item of a row or a column.
type layoutChild struct {
	Node      *emitted `json:"node"`
	Basis     any      `json:"basis,omitempty"`
	Cross     any      `json:"cross,omitempty"`
	Grow      int      `json:"grow,omitempty"`
	MinMain   any      `json:"minMain,omitempty"`
	MaxMain   any      `json:"maxMain,omitempty"`
	MinCross  any      `json:"minCross,omitempty"`
	MaxCross  any      `json:"maxCross,omitempty"`
	AlignSelf string   `json:"alignSelf,omitempty"`
	Ratio     float64  `json:"ratio,omitempty"`
}

// gridChild is one cell.
type gridChild struct {
	Node        *emitted `json:"node"`
	Column      int      `json:"column,omitempty"`
	Row         int      `json:"row,omitempty"`
	ColumnSpan  int      `json:"columnSpan,omitempty"`
	RowSpan     int      `json:"rowSpan,omitempty"`
	AlignSelf   string   `json:"alignSelf,omitempty"`
	JustifySelf string   `json:"justifySelf,omitempty"`
}

// anchor is a box placed against the container's edges.
type anchor struct {
	Node   *emitted `json:"node"`
	Top    any      `json:"top,omitempty"`
	Right  any      `json:"right,omitempty"`
	Bottom any      `json:"bottom,omitempty"`
	Left   any      `json:"left,omitempty"`
	Width  any      `json:"width,omitempty"`
	Height any      `json:"height,omitempty"`
	Layer  int      `json:"layer,omitempty"`
}

// lengthValue writes a length the way the schema reads one: a plain number of
// pixels, a percentage, or a calc of the two. An unset length is nil, which
// omitempty leaves out entirely — the schema's way of saying "measure it".
//
// The three spellings are not a style choice. lengthJSON tries a number
// first and a string second, so a bare 5 and "5px" are not interchangeable:
// only the first is a length everywhere a length is read.
func lengthValue(l compose.Length) any {
	if !l.IsSet() {
		return nil
	}
	tenths, pixels := l.Parts()
	switch {
	case tenths == 0:
		return pixels
	case pixels == 0:
		return fmt.Sprintf("%s%%", trimTenths(tenths))
	case pixels > 0:
		return fmt.Sprintf("calc(%s%% + %dpx)", trimTenths(tenths), pixels)
	default:
		return fmt.Sprintf("calc(%s%% - %dpx)", trimTenths(tenths), -pixels)
	}
}

// trimTenths writes a tenth of a percent without a trailing zero, so that the
// common whole number comes out as 50 rather than 50.0.
func trimTenths(tenths int) string {
	if tenths%10 == 0 {
		return fmt.Sprintf("%d", tenths/10)
	}
	return fmt.Sprintf("%d.%d", tenths/10, tenths%10)
}

// strike settles a stated size on one the family has.
//
// A stylesheet may say any number of pixels and this build has nine sizes per
// family, so the two do not always meet. Refusing the page was the old answer
// and it is the wrong one: an author writing 13px, which is what an author
// writes, got no picture at all and a message about strikes. Rounding without
// saying so would be worse. So it rounds and says so, and the size it settled
// on is written into the document where it can be read.
func (c *compiler) strike(family string, size int, path string) int {
	sizes := display.BuiltinFontSizes(family)
	if len(sizes) == 0 || slices.Contains(sizes, size) {
		return size
	}
	nearest := sizes[0]
	for _, candidate := range sizes {
		if abs(candidate-size) < abs(nearest-size) {
			nearest = candidate
		}
	}
	c.warn(path, "substituted-font-size", fmt.Sprintf(
		"font-size: %dpx has no strike in %q; drawn at %dpx, which does. It has %s",
		size, family, nearest, joinInts(sizes)))
	return nearest
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func joinInts(values []int) string {
	written := make([]string, len(values))
	for index, value := range values {
		written[index] = strconv.Itoa(value)
	}
	return strings.Join(written, ", ")
}

func inkName(ink display.Ink) string {
	switch ink {
	case display.InkWhite:
		return "white"
	case display.InkRed:
		return "red"
	case display.InkYellow:
		return "yellow"
	}
	return "black"
}

// The enum spellings below are the schema's, and the zero value of each is
// left empty so that omitempty keeps it out of the document. A field written
// as its own default is noise in a file somebody has to read.

func mainAlignName(a compose.MainAlignment) string {
	switch a {
	case compose.MainCenter:
		return "center"
	case compose.MainEnd:
		return "end"
	}
	return ""
}

// optionalCrossAlignName writes an alignment a child may or may not have
// stated. Stretch is the default everywhere but has to be written when it is
// chosen, because a child that says stretch inside a container that says
// centre is saying something.
func optionalCrossAlignName(a *compose.CrossAlignment) string {
	if a == nil {
		return ""
	}
	if *a == compose.CrossStretch {
		return "stretch"
	}
	return crossAlignName(*a)
}

func crossAlignName(a compose.CrossAlignment) string {
	switch a {
	case compose.CrossStart:
		return "start"
	case compose.CrossCenter:
		return "center"
	case compose.CrossEnd:
		return "end"
	}
	return ""
}

func horizontalAlignName(a display.HorizontalAlign) string {
	switch a {
	case display.AlignCenter:
		return "center"
	case display.AlignEnd:
		return "end"
	}
	return ""
}

func verticalAlignName(a display.VerticalAlign) string {
	switch a {
	case display.AlignMiddle:
		return "middle"
	case display.AlignBottom:
		return "bottom"
	}
	return ""
}

func wrapName(w display.WrapMode) string {
	if w == display.WrapRunes {
		return "runes"
	}
	return ""
}

func fitName(f display.ImageFit) string {
	switch f {
	case display.FitCover:
		return "cover"
	case display.FitStretch:
		return "stretch"
	}
	return "contain"
}

// shapeOf writes a clip-path. The schema takes only the fields that belong to
// the kind, and refuses the rest, so each case names its own.
func shapeOf(s compose.Shape) *shape {
	switch s.Kind {
	case compose.ShapeInset:
		out := &shape{Kind: "inset", Corner: lengthValue(s.Corner)}
		for _, inset := range s.Insets {
			value := lengthValue(inset)
			if value == nil {
				value = 0
			}
			out.Insets = append(out.Insets, value)
		}
		return out
	case compose.ShapeCircle:
		return &shape{Kind: "circle", Radius: lengthValue(s.Radius), Center: centreOf(s)}
	case compose.ShapeEllipse:
		return &shape{Kind: "ellipse", RadiusX: lengthValue(s.RadiusX),
			RadiusY: lengthValue(s.RadiusY), Center: centreOf(s)}
	case compose.ShapePolygon:
		out := &shape{Kind: "polygon"}
		for _, corner := range s.Points {
			out.Points = append(out.Points, point{X: lengthValue(corner[0]), Y: lengthValue(corner[1])})
		}
		return out
	}
	return nil
}

func centreOf(s compose.Shape) *point {
	if !s.Centre[0].IsSet() && !s.Centre[1].IsSet() {
		return nil
	}
	return &point{X: lengthValue(s.Centre[0]), Y: lengthValue(s.Centre[1])}
}

// tracksOf writes a grid's columns or rows. A track is "auto", a count of fr,
// or a length, and the three are told apart by how they are spelled rather
// than by a field naming which one it is.
func tracksOf(tracks []compose.Track) []any {
	if len(tracks) == 0 {
		return nil
	}
	written := make([]any, len(tracks))
	for index, track := range tracks {
		switch {
		case track.Fraction > 0:
			written[index] = fmt.Sprintf("%dfr", track.Fraction)
		case track.Size.IsSet():
			written[index] = lengthValue(track.Size)
		default:
			written[index] = "auto"
		}
	}
	return written
}

// runOf writes one stretch of text with the style it is set in.
//
// The family and the size go together or not at all. The decoder passes both
// through as written, so a run naming a family and no size asks for a strike
// of zero pixels and is refused — which is what happened to every page here
// the first time this left the size out as a value equal to the default.
//
// Black is left out because it is the default ink and a page of plain text
// would otherwise say so on every line of itself.
func (c *compiler) runOf(text string, s style, path string) run {
	written := run{Text: text}
	if s.fontFamily != display.DefaultFontFamily || s.fontSize != display.DefaultFontSize {
		written.Font, written.Size = s.fontFamily, c.strike(s.fontFamily, s.fontSize, path)
	}
	if ink := inkName(s.color); ink != "black" {
		written.Ink = ink
	}
	return written
}
