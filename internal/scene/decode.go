// Package scene decodes the versioned internal page representation used between
// the Markup compiler and the renderer. It is deliberately separate from
// compose: the encoded document is the boundary shared by the two stages,
// while compose remains an internal layout model.
//
// # What this describes
//
// Everything compose can draw. A document lays out a page with rows, columns,
// grids, relative offsets and anchored boxes, and it states geometry as coordinates: arcs,
// polygons, paths, patterns and single pixels. The second kind is what a
// generator produces rather than what a person writes, which is why it is
// stated as numbers and not as a style.
//
// That the two lists are the same list is checked rather than asserted.
// coverage_test.go walks compose for everything implementing Node and fails
// if this package cannot build one, because the gap it is looking for opened
// once already and stayed open: grid, quarter turns, shape clipping and
// anchored boxes once reached compose before this decoder could build them, and
// no test noticed while the document examples were the only coverage.
//
// Pages written with HTML and CSS enter through internal/markup, which emits
// this representation before the renderer decodes it.
package scene

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

const Version = 1

const maxRemoteImageBytes = 32 << 20

var remoteImageClient = &http.Client{Timeout: 15 * time.Second}

type Decoder struct {
	BaseDir       string
	RestrictFiles bool
	Resources     map[string][]byte
	// ResourcesOnly prevents image sources from falling back to the local
	// filesystem. Resource maps, HTTP(S) URLs and data URLs remain available.
	ResourcesOnly bool
}

type documentJSON struct {
	Version     int             `json:"version"`
	Orientation string          `json:"orientation,omitempty"`
	Size        *sizeJSON       `json:"size,omitempty"`
	Background  *string         `json:"background,omitempty"`
	Root        json.RawMessage `json:"root,omitempty"`
}

func (d Decoder) Decode(reader io.Reader) (compose.Document, error) {
	if reader == nil {
		return compose.Document{}, fmt.Errorf("scene reader must not be nil")
	}
	var source documentJSON
	if err := decodeStrict(reader, &source); err != nil {
		return compose.Document{}, fmt.Errorf("document: %w", err)
	}
	if source.Version != Version {
		return compose.Document{}, fmt.Errorf("document.version: unsupported version %d, want %d", source.Version, Version)
	}
	orientation, err := parseOrientation(source.Orientation)
	if err != nil {
		return compose.Document{}, fmt.Errorf("document.orientation: %w", err)
	}
	document := compose.Document{Orientation: orientation}
	if source.Size != nil {
		document.Size = source.Size.point()
	}
	if source.Background != nil {
		ink, err := parseInk(*source.Background)
		if err != nil {
			return compose.Document{}, fmt.Errorf("document.background: %w", err)
		}
		document.Background = compose.Ink(ink)
	}
	if len(source.Root) != 0 && string(source.Root) != "null" {
		document.Root, err = d.decodeNode(source.Root, "document.root")
		if err != nil {
			return compose.Document{}, err
		}
	}
	return document, nil
}

func (d Decoder) DecodeFile(path string) (compose.Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return compose.Document{}, fmt.Errorf("open scene: %w", err)
	}
	defer file.Close()
	if d.BaseDir == "" {
		d.BaseDir = filepath.Dir(path)
	}
	return d.Decode(file)
}

type nodeHeader struct {
	Type string `json:"type"`
}

func (d Decoder) decodeNode(raw json.RawMessage, path string) (compose.Node, error) {
	var header nodeHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if header.Type == "" {
		return nil, fmt.Errorf("%s.type: field is required", path)
	}

	switch header.Type {
	case "text":
		var value textJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		return value.node(path)
	case "inline":
		var value inlineJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		return d.decodeInline(value, path)
	case "image":
		var value imageJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		return d.decodeImage(value, path)
	case "absolute":
		var value absoluteJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		children := make([]compose.Placed, len(value.Children))
		for index, child := range value.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, index)
			if err := rejectOwnSize(child.Node, childPath+".node", "bounds"); err != nil {
				return nil, err
			}
			node, err := d.decodeNode(child.Node, childPath+".node")
			if err != nil {
				return nil, err
			}
			children[index] = compose.Placed{Bounds: child.Bounds.rect(), Node: node}
		}
		return compose.Absolute{Size: value.Size.point(), Clip: value.Clip, Children: children}, nil
	case "row", "column":
		var value flowJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		children := make([]compose.LayoutChild, len(value.Children))
		for index, child := range value.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, index)
			node, err := d.decodeNode(child.Node, childPath+".node")
			if err != nil {
				return nil, err
			}
			alignSelf, err := optionalCrossAlign(child.AlignSelf, childPath+".alignSelf")
			if err != nil {
				return nil, err
			}
			// ratio works out the cross size from the main one, so a stated
			// cross size leaves it nothing to do.
			if child.Ratio != 0 && child.Cross.length.IsSet() {
				return nil, fmt.Errorf("%s: ratio works out the cross size and cross states it; give one or the other", childPath)
			}
			margin := [4]compose.Length{}
			if child.Margin != nil {
				margin = child.Margin.lengths()
			}
			children[index] = compose.LayoutChild{
				Node: node, Basis: child.Basis.length, Cross: child.Cross.length,
				Grow: child.Grow, Shrink: child.Shrink,
				Margin:    margin,
				MinMain:   child.MinMain.length,
				MaxMain:   child.MaxMain.length,
				MinCross:  child.MinCross.length,
				MaxCross:  child.MaxCross.length,
				AlignSelf: alignSelf,
				Ratio:     child.Ratio,
			}
		}
		mainAlign, err := parseMainAlign(value.MainAlign)
		if err != nil {
			return nil, fmt.Errorf("%s.mainAlign: %w", path, err)
		}
		crossAlign, err := parseCrossAlign(value.CrossAlign)
		if err != nil {
			return nil, fmt.Errorf("%s.crossAlign: %w", path, err)
		}
		if header.Type == "row" {
			return compose.Row{Size: value.Size.point(), Gap: value.Gap, GapLength: value.GapLength.length, MainAlign: mainAlign, CrossAlign: crossAlign, Children: children}, nil
		}
		return compose.Column{Size: value.Size.point(), Gap: value.Gap, GapLength: value.GapLength.length, MainAlign: mainAlign, CrossAlign: crossAlign, Children: children}, nil
	case "grid":
		var value gridJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		return d.decodeGrid(value, path)
	case "anchored":
		var value anchoredJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		children := make([]compose.Anchor, len(value.Children))
		for index, child := range value.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, index)
			if err := rejectOwnSize(child.Node, childPath+".node", "insets"); err != nil {
				return nil, err
			}
			if err := child.overConstrained(childPath); err != nil {
				return nil, err
			}
			node, err := d.decodeNode(child.Node, childPath+".node")
			if err != nil {
				return nil, err
			}
			children[index] = compose.Anchor{
				Top: child.Top.length, Right: child.Right.length,
				Bottom: child.Bottom.length, Left: child.Left.length,
				Width: child.Width.length, Height: child.Height.length,
				Layer: child.Layer, Node: node,
			}
		}
		return compose.Anchored{Size: value.Size.point(), Children: children}, nil
	case "relative":
		var value relativeJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.Relative{
			Top: value.Top.length, Right: value.Right.length,
			Bottom: value.Bottom.length, Left: value.Left.length,
			Child: child,
		}, nil
	case "stack":
		var value stackJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		children := make([]compose.Node, len(value.Children))
		for index, child := range value.Children {
			var err error
			children[index], err = d.decodeNode(child, fmt.Sprintf("%s.children[%d]", path, index))
			if err != nil {
				return nil, err
			}
		}
		return compose.Stack{Size: value.Size.point(), Children: children}, nil
	case "padding":
		var value paddingJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		padding := compose.Padding{Insets: compose.Insets{Top: value.Insets.Top, Right: value.Insets.Right, Bottom: value.Insets.Bottom, Left: value.Insets.Left}, Child: child}
		if value.LengthInsets != nil {
			padding.Lengths = value.LengthInsets.lengths()
		}
		return padding, nil
	case "spacer":
		var value sizedJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		return compose.Spacer{Size: value.Size.point()}, nil
	case "pixel":
		var value pixelJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		ink, err := parseInk(value.Ink)
		if err != nil {
			return nil, fmt.Errorf("%s.ink: %w", path, err)
		}
		return compose.Pixel{At: value.At.point(), Ink: ink, Size: value.Size.point()}, nil
	case "rectangle":
		var value rectangleJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		fill, stroke, err := parsePaint(value.Fill, value.Stroke, path)
		if err != nil {
			return nil, err
		}
		top, right, bottom, left, err := optionalStrokes(value.StrokeTop, value.StrokeRight, value.StrokeBottom, value.StrokeLeft)
		if err != nil {
			return nil, err
		}
		return compose.Rectangle{Size: value.Size.point(), Radius: value.Radius, Fill: fill, Stroke: stroke,
			StrokeTop: top, StrokeRight: right, StrokeBottom: bottom, StrokeLeft: left}, nil
	case "line":
		var value lineJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		stroke, err := value.Stroke.style(path + ".stroke")
		if err != nil {
			return nil, err
		}
		return compose.Line{Size: value.Size.point(), From: value.From.point(), To: value.To.point(), Stroke: stroke}, nil
	case "polyline":
		var value polylineJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		stroke, err := value.Stroke.style(path + ".stroke")
		if err != nil {
			return nil, err
		}
		return compose.Polyline{Size: value.Size.point(), Points: points(value.Points), Stroke: stroke}, nil
	case "polygon":
		var value polygonJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		fill, stroke, err := parsePaint(value.Fill, value.Stroke, path)
		if err != nil {
			return nil, err
		}
		return compose.Polygon{Size: value.Size.point(), Points: points(value.Points), Fill: fill, Stroke: stroke}, nil
	case "circle":
		var value circleJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		fill, stroke, err := parsePaint(value.Fill, value.Stroke, path)
		if err != nil {
			return nil, err
		}
		return compose.Circle{Size: value.Size.point(), Center: value.Center.point(), Radius: value.Radius, Fill: fill, Stroke: stroke}, nil
	case "ellipse":
		var value paintedJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		fill, stroke, err := parsePaint(value.Fill, value.Stroke, path)
		if err != nil {
			return nil, err
		}
		return compose.Ellipse{Size: value.Size.point(), Fill: fill, Stroke: stroke}, nil
	case "arc":
		var value arcJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		stroke, err := value.Stroke.style(path + ".stroke")
		if err != nil {
			return nil, err
		}
		return compose.Arc{Size: value.Size.point(), Start: value.Start, Sweep: value.Sweep, Stroke: stroke}, nil
	case "pie", "chord":
		var value segmentJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		ink, err := parseInk(value.Ink)
		if err != nil {
			return nil, fmt.Errorf("%s.ink: %w", path, err)
		}
		if header.Type == "pie" {
			return compose.Pie{Size: value.Size.point(), Start: value.Start, Sweep: value.Sweep, Ink: ink}, nil
		}
		return compose.Chord{Size: value.Size.point(), Start: value.Start, Sweep: value.Sweep, Ink: ink}, nil
	case "path":
		var value pathJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		shape, err := value.path(path + ".commands")
		if err != nil {
			return nil, err
		}
		fill, stroke, err := parsePaint(value.Fill, value.Stroke, path)
		if err != nil {
			return nil, err
		}
		return compose.Path{Size: value.Size.point(), Path: shape, Fill: fill, Stroke: stroke}, nil
	case "pattern":
		var value patternJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		inks := make(map[rune]display.Ink, len(value.Inks))
		for symbol, name := range value.Inks {
			runes := []rune(symbol)
			if len(runes) != 1 {
				return nil, fmt.Errorf("%s.inks: key %q must be one rune", path, symbol)
			}
			ink, err := parseInk(name)
			if err != nil {
				return nil, fmt.Errorf("%s.inks[%q]: %w", path, symbol, err)
			}
			inks[runes[0]] = ink
		}
		pattern, err := display.NewPattern(value.Rows, inks)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return compose.Pattern{Size: value.Size.point(), Pattern: pattern}, nil
	case "clipPath":
		var value clipPathJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		shape, err := value.Path.path(path + ".path.commands")
		if err != nil {
			return nil, err
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.ClipPath{Size: value.Size.point(), Path: shape, Child: child}, nil
	case "clipRect":
		var value clipRectJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.ClipRect{Size: value.Size.point(), Rect: value.Rect.rect(), Child: child}, nil
	case "clip":
		var value clipJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.Clip{Size: value.Size.point(), Child: child}, nil
	case "clipShape":
		var value clipShapeJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		shape, err := value.Shape.shape(path + ".shape")
		if err != nil {
			return nil, err
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.ClipShape{Size: value.Size.point(), Shape: shape, Child: child}, nil
	case "rotated":
		var value rotatedJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		node := compose.Rotated{Size: value.Size.point(), Degrees: value.Degrees, Child: child}
		if value.Origin != nil {
			node.Origin = &[2]compose.Length{value.Origin.X.length, value.Origin.Y.length}
		}
		return node, nil
	case "transformed":
		var value transformedJSON
		if err := decodeStrictBytes(raw, &value); err != nil {
			return nil, nodeError(path, err)
		}
		// Transform quietly rounds a nonsense scale up to one. A document that
		// asked for something else should be told it cannot have it.
		if value.Scale < 0 {
			return nil, fmt.Errorf("%s.scale: must not be negative", path)
		}
		child, err := d.decodeNode(value.Child, path+".child")
		if err != nil {
			return nil, err
		}
		return compose.Transformed{
			Size:      value.Size.point(),
			Transform: display.Transform{Scale: value.Scale, Turns: value.Turns},
			Child:     child,
		}, nil
	default:
		return nil, fmt.Errorf("%s.type: unknown node type %q", path, header.Type)
	}
}

type sizeJSON struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type pointJSON struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type rectJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (s sizeJSON) point() image.Point  { return image.Pt(s.Width, s.Height) }
func (p pointJSON) point() image.Point { return image.Pt(p.X, p.Y) }
func (r rectJSON) rect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height)
}

type sizedJSON struct {
	Type string   `json:"type"`
	Size sizeJSON `json:"size,omitempty"`
}

type absoluteJSON struct {
	Type     string       `json:"type"`
	Size     sizeJSON     `json:"size,omitempty"`
	Clip     bool         `json:"clip,omitempty"`
	Children []placedJSON `json:"children,omitempty"`
}
type placedJSON struct {
	Bounds rectJSON        `json:"bounds"`
	Node   json.RawMessage `json:"node"`
}
type flowJSON struct {
	Type       string            `json:"type"`
	Size       sizeJSON          `json:"size,omitempty"`
	Gap        int               `json:"gap,omitempty"`
	GapLength  lengthJSON        `json:"gapLength,omitempty"`
	MainAlign  string            `json:"mainAlign,omitempty"`
	CrossAlign string            `json:"crossAlign,omitempty"`
	Children   []layoutChildJSON `json:"children,omitempty"`
}
type layoutChildJSON struct {
	Node      json.RawMessage   `json:"node"`
	Basis     lengthJSON        `json:"basis,omitempty"`
	Cross     lengthJSON        `json:"cross,omitempty"`
	Grow      float64           `json:"grow,omitempty"`
	Shrink    float64           `json:"shrink,omitempty"`
	Margin    *lengthInsetsJSON `json:"margin,omitempty"`
	MinMain   lengthJSON        `json:"minMain,omitempty"`
	MaxMain   lengthJSON        `json:"maxMain,omitempty"`
	MinCross  lengthJSON        `json:"minCross,omitempty"`
	MaxCross  lengthJSON        `json:"maxCross,omitempty"`
	AlignSelf *string           `json:"alignSelf,omitempty"`
	Ratio     float64           `json:"ratio,omitempty"`
}

type gridJSON struct {
	Type            string          `json:"type"`
	Size            sizeJSON        `json:"size,omitempty"`
	Columns         []trackJSON     `json:"columns,omitempty"`
	Rows            []trackJSON     `json:"rows,omitempty"`
	ColumnGap       int             `json:"columnGap,omitempty"`
	RowGap          int             `json:"rowGap,omitempty"`
	ColumnGapLength lengthJSON      `json:"columnGapLength,omitempty"`
	RowGapLength    lengthJSON      `json:"rowGapLength,omitempty"`
	AlignItems      string          `json:"alignItems,omitempty"`
	JustifyItems    string          `json:"justifyItems,omitempty"`
	Children        []gridChildJSON `json:"children,omitempty"`
}
type gridChildJSON struct {
	Node        json.RawMessage   `json:"node"`
	Column      int               `json:"column,omitempty"`
	Row         int               `json:"row,omitempty"`
	ColumnSpan  int               `json:"columnSpan,omitempty"`
	RowSpan     int               `json:"rowSpan,omitempty"`
	AlignSelf   *string           `json:"alignSelf,omitempty"`
	JustifySelf *string           `json:"justifySelf,omitempty"`
	Margin      *lengthInsetsJSON `json:"margin,omitempty"`
}

// trackJSON accepts the three ways a grid track can be sized, spelled the way
// CSS spells them because there is no reason to invent a second spelling:
// "auto" takes the size of the widest thing in the track, "2fr" takes a share
// of what no other track claimed, and a number or a percentage states it.
type trackJSON struct {
	track compose.Track
}

func (t *trackJSON) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		trimmed := strings.TrimSpace(text)
		if trimmed == "auto" {
			t.track = compose.Track{}
			return nil
		}
		if fraction, found := strings.CutSuffix(trimmed, "fr"); found {
			count, err := strconv.Atoi(strings.TrimSpace(fraction))
			if err != nil || count <= 0 {
				return fmt.Errorf("%q is not a fraction; write a whole number of fr such as \"1fr\"", text)
			}
			t.track = compose.Track{Fraction: count}
			return nil
		}
	}
	var length lengthJSON
	if err := length.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("a track is \"auto\", a number of fr such as \"2fr\", a number of pixels or a percentage")
	}
	t.track = compose.Track{Size: length.length}
	return nil
}

func tracks(source []trackJSON) []compose.Track {
	if len(source) == 0 {
		return nil
	}
	result := make([]compose.Track, len(source))
	for index, track := range source {
		result[index] = track.track
	}
	return result
}

func (d Decoder) decodeGrid(value gridJSON, path string) (compose.Node, error) {
	children := make([]compose.GridChild, len(value.Children))
	for index, child := range value.Children {
		childPath := fmt.Sprintf("%s.children[%d]", path, index)
		node, err := d.decodeNode(child.Node, childPath+".node")
		if err != nil {
			return nil, err
		}
		alignSelf, err := optionalCrossAlign(child.AlignSelf, childPath+".alignSelf")
		if err != nil {
			return nil, err
		}
		justifySelf, err := optionalCrossAlign(child.JustifySelf, childPath+".justifySelf")
		if err != nil {
			return nil, err
		}
		margin := [4]compose.Length{}
		if child.Margin != nil {
			margin = child.Margin.lengths()
		}
		children[index] = compose.GridChild{
			Node: node, Column: child.Column, Row: child.Row,
			ColumnSpan: child.ColumnSpan, RowSpan: child.RowSpan,
			Margin:    margin,
			AlignSelf: alignSelf, JustifySelf: justifySelf,
		}
	}
	alignItems, err := parseCrossAlign(value.AlignItems)
	if err != nil {
		return nil, fmt.Errorf("%s.alignItems: %w", path, err)
	}
	justifyItems, err := parseCrossAlign(value.JustifyItems)
	if err != nil {
		return nil, fmt.Errorf("%s.justifyItems: %w", path, err)
	}
	return compose.Grid{
		Size:            value.Size.point(),
		Columns:         tracks(value.Columns),
		Rows:            tracks(value.Rows),
		ColumnGap:       value.ColumnGap,
		RowGap:          value.RowGap,
		ColumnGapLength: value.ColumnGapLength.length,
		RowGapLength:    value.RowGapLength.length,
		AlignItems:      alignItems, JustifyItems: justifyItems,
		Children: children,
	}, nil
}

// rejectOwnSize refuses a node that states a size where its parent is going to
// give it one.
//
// absolute and anchored do not measure their children; they hand each one a
// rectangle. A size on such a child never reaches the drawing, and a field
// that is read, accepted and then ignored is worse than one that is refused:
// it is written in good faith and the page comes out wrong somewhere else.
func rejectOwnSize(raw json.RawMessage, path, stated string) error {
	var probe struct {
		Size *sizeJSON `json:"size"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil // decodeNode is about to report this properly
	}
	if probe.Size == nil || (probe.Size.Width == 0 && probe.Size.Height == 0) {
		return nil
	}
	return fmt.Errorf("%s.size: this node is placed by its parent's %s, which leaves nothing for a size to do; remove it", path, stated)
}

// overConstrained refuses an anchor that states both edges and a size on the
// same axis, where one of the three has to be discarded to satisfy the others.
func (a anchorJSON) overConstrained(path string) error {
	axes := []struct {
		start, end, size       compose.Length
		startName, endName, of string
	}{
		{a.Left.length, a.Right.length, a.Width.length, "left", "right", "width"},
		{a.Top.length, a.Bottom.length, a.Height.length, "top", "bottom", "height"},
	}
	for _, axis := range axes {
		if axis.start.IsSet() && axis.end.IsSet() && axis.size.IsSet() {
			return fmt.Errorf("%s: %s, %s and %s cannot all hold at once; drop one",
				path, axis.startName, axis.endName, axis.of)
		}
	}
	return nil
}

func optionalCrossAlign(value *string, path string) (*compose.CrossAlignment, error) {
	if value == nil {
		return nil, nil
	}
	alignment, err := parseCrossAlign(*value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &alignment, nil
}

type anchoredJSON struct {
	Type     string       `json:"type"`
	Size     sizeJSON     `json:"size,omitempty"`
	Children []anchorJSON `json:"children,omitempty"`
}

type relativeJSON struct {
	Type   string          `json:"type"`
	Top    offsetJSON      `json:"top,omitempty"`
	Right  offsetJSON      `json:"right,omitempty"`
	Bottom offsetJSON      `json:"bottom,omitempty"`
	Left   offsetJSON      `json:"left,omitempty"`
	Child  json.RawMessage `json:"child"`
}

type anchorJSON struct {
	Node   json.RawMessage `json:"node"`
	Top    offsetJSON      `json:"top,omitempty"`
	Right  offsetJSON      `json:"right,omitempty"`
	Bottom offsetJSON      `json:"bottom,omitempty"`
	Left   offsetJSON      `json:"left,omitempty"`
	Width  lengthJSON      `json:"width,omitempty"`
	Height lengthJSON      `json:"height,omitempty"`
	Layer  int             `json:"layer,omitempty"`
}

type clipJSON struct {
	Type  string          `json:"type"`
	Size  sizeJSON        `json:"size,omitempty"`
	Child json.RawMessage `json:"child"`
}

type rotatedJSON struct {
	Type    string          `json:"type"`
	Size    sizeJSON        `json:"size,omitempty"`
	Degrees float64         `json:"degrees,omitempty"`
	Origin  *originJSON     `json:"origin,omitempty"`
	Child   json.RawMessage `json:"child"`
}

// originJSON is a point stated in lengths, so that half way along a box can be
// said before the box has a size.
type originJSON struct {
	X offsetJSON `json:"x,omitempty"`
	Y offsetJSON `json:"y,omitempty"`
}

type transformedJSON struct {
	Type  string          `json:"type"`
	Size  sizeJSON        `json:"size,omitempty"`
	Scale int             `json:"scale,omitempty"`
	Turns int             `json:"turns,omitempty"`
	Child json.RawMessage `json:"child"`
}

type clipShapeJSON struct {
	Type  string          `json:"type"`
	Size  sizeJSON        `json:"size,omitempty"`
	Shape shapeJSON       `json:"shape"`
	Child json.RawMessage `json:"child"`
}
type shapeJSON struct {
	Kind    string            `json:"kind"`
	Insets  []offsetJSON      `json:"insets,omitempty"`
	Corner  lengthJSON        `json:"corner,omitempty"`
	Radius  lengthJSON        `json:"radius,omitempty"`
	RadiusX lengthJSON        `json:"radiusX,omitempty"`
	RadiusY lengthJSON        `json:"radiusY,omitempty"`
	Center  *lengthPointJSON  `json:"center,omitempty"`
	Points  []lengthPointJSON `json:"points,omitempty"`
}
type lengthPointJSON struct {
	X offsetJSON `json:"x"`
	Y offsetJSON `json:"y"`
}

func (s shapeJSON) shape(path string) (compose.Shape, error) {
	var shape compose.Shape
	switch s.Kind {
	case "inset":
		shape.Kind = compose.ShapeInset
		if len(s.Insets) != 4 {
			return compose.Shape{}, fmt.Errorf("%s.insets: an inset shape needs four lengths: top, right, bottom, left", path)
		}
		for index, inset := range s.Insets {
			shape.Insets[index] = inset.length
		}
		shape.Corner = s.Corner.length
	case "circle":
		shape.Kind = compose.ShapeCircle
		shape.Radius = s.Radius.length
	case "ellipse":
		shape.Kind = compose.ShapeEllipse
		shape.RadiusX, shape.RadiusY = s.RadiusX.length, s.RadiusY.length
	case "polygon":
		shape.Kind = compose.ShapePolygon
		if len(s.Points) < 3 {
			return compose.Shape{}, fmt.Errorf("%s.points: a polygon needs at least three corners, got %d", path, len(s.Points))
		}
		shape.Points = make([][2]compose.Length, len(s.Points))
		for index, point := range s.Points {
			shape.Points[index] = [2]compose.Length{point.X.length, point.Y.length}
		}
	default:
		return compose.Shape{}, fmt.Errorf("%s.kind: %w", path, enumError(s.Kind, "inset", "circle", "ellipse", "polygon"))
	}
	if s.Center != nil {
		// Only a circle and an ellipse have one. Accepting it elsewhere would
		// mean accepting a document that says something and gets nothing.
		if shape.Kind != compose.ShapeCircle && shape.Kind != compose.ShapeEllipse {
			return compose.Shape{}, fmt.Errorf("%s.center: only a circle or an ellipse has a centre", path)
		}
		shape.Centre = [2]compose.Length{s.Center.X.length, s.Center.Y.length}
	}
	return shape, nil
}

// lengthJSON accepts a number of pixels or a percentage written as a string,
// because compose resolves both and a document that can only say one of them
// would be the poorer description.
//
// Zero is a length like any other. This method only runs when the field is
// present, so an absent field still leaves the zero value, which is automatic;
// there is no need to spend zero on saying so. Spending it that way was a bug:
// an anchored box asking for "right": 0 is asking to sit against the right
// edge, and it silently got no horizontal constraint at all.
type lengthJSON struct {
	length compose.Length
}

// offsetJSON is a length for the fields that measure a distance rather than a
// size, and so may be negative: an anchor's four edges, a shape's insets, the
// centre of a circle and the corners of a polygon.
//
// A negative size is refused instead, because there is no such thing and
// accepting one would only mean clamping it back to zero behind the author's
// back.
type offsetJSON struct {
	length compose.Length
}

func (o *offsetJSON) UnmarshalJSON(data []byte) error {
	var length lengthJSON
	if err := length.unmarshal(data, true); err != nil {
		return err
	}
	o.length = length.length
	return nil
}

func (l *lengthJSON) UnmarshalJSON(data []byte) error { return l.unmarshal(data, false) }

func (l *lengthJSON) unmarshal(data []byte, signed bool) error {
	var pixels int
	if err := json.Unmarshal(data, &pixels); err == nil {
		if pixels < 0 && !signed {
			return fmt.Errorf("a length must not be negative")
		}
		l.length = compose.Pixels(pixels)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("a length is a number of pixels, a percentage such as \"25%%\", " +
			"or \"calc(100%% - 10px)\"")
	}
	trimmed := strings.TrimSpace(text)
	if inner, isCalc := strings.CutPrefix(trimmed, "calc("); isCalc {
		body, closed := strings.CutSuffix(inner, ")")
		if !closed {
			return fmt.Errorf("%q is missing its closing bracket", text)
		}
		length, err := parseCalc(body, signed)
		if err != nil {
			return fmt.Errorf("%q: %w", text, err)
		}
		l.length = length
		return nil
	}
	percent, err := parsePercent(trimmed, signed)
	if err != nil {
		return fmt.Errorf("%q is not a length; write a number of pixels, "+
			"a percentage such as \"25%%\", or \"calc(100%% - 10px)\"", text)
	}
	l.length = compose.Tenths(percent)
	return nil
}

// parseCalc reads the one expression a Length can hold: a share of the
// container plus or minus a fixed number of pixels.
//
// Spaces around the sign are optional. CSS insists on them because a term
// there may be negative and "100%-10px" would be ambiguous; neither term here
// may be, so the first sign after the opening term is always the operator and
// there is nothing to be careful about.
func parseCalc(body string, signed bool) (compose.Length, error) {
	packed := strings.Join(strings.Fields(body), "")
	at := strings.IndexAny(packed[1:], "+-") + 1
	if at < 1 {
		return compose.Length{}, fmt.Errorf(
			"calc takes a percentage, a + or -, and a number of pixels")
	}
	sign := 1
	if packed[at] == '-' {
		sign = -1
	}

	percentText, pixelText := packed[:at], packed[at+1:]
	// Either order reads naturally, and both mean the same sum.
	if strings.HasSuffix(percentText, "px") {
		percentText, pixelText = pixelText, percentText
		if sign < 0 {
			return compose.Length{}, fmt.Errorf(
				"a fixed number of pixels minus a share of the container is not a length here")
		}
	}
	percent, err := parsePercent(percentText, signed)
	if err != nil {
		return compose.Length{}, fmt.Errorf("%q is not a percentage", percentText)
	}
	digits, isPixels := strings.CutSuffix(pixelText, "px")
	if !isPixels {
		return compose.Length{}, fmt.Errorf("%q is not a number of pixels", pixelText)
	}
	pixels, err := strconv.Atoi(digits)
	if err != nil || pixels < 0 {
		return compose.Length{}, fmt.Errorf("%q is not a number of pixels", pixelText)
	}
	return compose.Calc(percent, sign*pixels), nil
}

// parsePercent reads a percentage into the tenths compose counts them in.
func parsePercent(text string, signed bool) (int, error) {
	digits, isPercent := strings.CutSuffix(strings.TrimSpace(text), "%")
	if !isPercent {
		return 0, fmt.Errorf("%q does not end in %%", text)
	}
	value, err := strconv.ParseFloat(digits, 64)
	if err != nil || (value < 0 && !signed) {
		return 0, fmt.Errorf("%q is not a percentage", text)
	}
	return int(value * 10), nil
}

type stackJSON struct {
	Type     string            `json:"type"`
	Size     sizeJSON          `json:"size,omitempty"`
	Children []json.RawMessage `json:"children,omitempty"`
}
type paddingJSON struct {
	Type         string            `json:"type"`
	Insets       insetsJSON        `json:"insets,omitempty"`
	LengthInsets *lengthInsetsJSON `json:"lengthInsets,omitempty"`
	Child        json.RawMessage   `json:"child"`
}
type insetsJSON struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

type lengthInsetsJSON struct {
	Top    lengthJSON `json:"top"`
	Right  lengthJSON `json:"right"`
	Bottom lengthJSON `json:"bottom"`
	Left   lengthJSON `json:"left"`
}

func (i lengthInsetsJSON) lengths() [4]compose.Length {
	return [4]compose.Length{i.Top.length, i.Right.length, i.Bottom.length, i.Left.length}
}

type textJSON struct {
	Type          string    `json:"type"`
	Size          sizeJSON  `json:"size,omitempty"`
	Runs          []runJSON `json:"runs,omitempty"`
	Align         string    `json:"align,omitempty"`
	VerticalAlign string    `json:"verticalAlign,omitempty"`
	Wrap          string    `json:"wrap,omitempty"`
	LineHeight    int       `json:"lineHeight,omitempty"`
}

type inlineJSON struct {
	Type       string           `json:"type"`
	Size       sizeJSON         `json:"size,omitempty"`
	Items      []inlineItemJSON `json:"items,omitempty"`
	Align      string           `json:"align,omitempty"`
	Wrap       string           `json:"wrap,omitempty"`
	LineHeight int              `json:"lineHeight,omitempty"`
}

type inlineItemJSON struct {
	Runs          []runJSON         `json:"runs,omitempty"`
	Node          json.RawMessage   `json:"node,omitempty"`
	Break         bool              `json:"break,omitempty"`
	Padding       insetsJSON        `json:"padding,omitempty"`
	Margin        insetsJSON        `json:"margin,omitempty"`
	PaddingLength *lengthInsetsJSON `json:"paddingLength,omitempty"`
	MarginLength  *lengthInsetsJSON `json:"marginLength,omitempty"`
	Background    *string           `json:"background,omitempty"`
	Border        *strokeJSON       `json:"border,omitempty"`
	BorderTop     *strokeJSON       `json:"borderTop,omitempty"`
	BorderRight   *strokeJSON       `json:"borderRight,omitempty"`
	BorderBottom  *strokeJSON       `json:"borderBottom,omitempty"`
	BorderLeft    *strokeJSON       `json:"borderLeft,omitempty"`
	Radius        int               `json:"radius,omitempty"`
	LineHeight    int               `json:"lineHeight,omitempty"`
	VerticalAlign string            `json:"verticalAlign,omitempty"`
	Wrap          string            `json:"wrap,omitempty"`
	Top           offsetJSON        `json:"top,omitempty"`
	Right         offsetJSON        `json:"right,omitempty"`
	Bottom        offsetJSON        `json:"bottom,omitempty"`
	Left          offsetJSON        `json:"left,omitempty"`
}

func (d Decoder) decodeInline(value inlineJSON, path string) (compose.Node, error) {
	align, err := parseHorizontalAlign(value.Align)
	if err != nil {
		return nil, fmt.Errorf("%s.align: %w", path, err)
	}
	wrap, err := parseWrap(value.Wrap)
	if err != nil {
		return nil, fmt.Errorf("%s.wrap: %w", path, err)
	}
	items := make([]compose.InlineItem, len(value.Items))
	for index, source := range value.Items {
		itemPath := fmt.Sprintf("%s.items[%d]", path, index)
		runs := make([]display.TextRun, len(source.Runs))
		for runIndex, run := range source.Runs {
			ink, err := parseInk(run.Ink)
			if err != nil {
				return nil, fmt.Errorf("%s.runs[%d].ink: %w", itemPath, runIndex, err)
			}
			runs[runIndex] = display.TextRun{Text: run.Text,
				Style: display.TextStyle{Font: run.Font, Fallback: run.Fallback, Size: run.Size, Ink: ink}}
		}
		var node compose.Node
		if len(source.Node) != 0 && string(source.Node) != "null" {
			var err error
			node, err = d.decodeNode(source.Node, itemPath+".node")
			if err != nil {
				return nil, err
			}
		}
		var background *display.Ink
		if source.Background != nil {
			ink, err := parseInk(*source.Background)
			if err != nil {
				return nil, fmt.Errorf("%s.background: %w", itemPath, err)
			}
			background = compose.Ink(ink)
		}
		var border *display.StrokeStyle
		if source.Border != nil {
			value, err := source.Border.style(itemPath + ".border")
			if err != nil {
				return nil, err
			}
			border = compose.Stroke(value)
		}
		top, right, bottom, left, err := optionalStrokes(source.BorderTop, source.BorderRight, source.BorderBottom, source.BorderLeft)
		if err != nil {
			return nil, err
		}
		vertical, err := parseInlineVerticalAlign(source.VerticalAlign)
		if err != nil {
			return nil, fmt.Errorf("%s.verticalAlign: %w", itemPath, err)
		}
		itemWrap, err := parseWrap(source.Wrap)
		if err != nil {
			return nil, fmt.Errorf("%s.wrap: %w", itemPath, err)
		}
		paddingLengths := [4]compose.Length{}
		if source.PaddingLength != nil {
			paddingLengths = source.PaddingLength.lengths()
		}
		marginLengths := [4]compose.Length{}
		if source.MarginLength != nil {
			marginLengths = source.MarginLength.lengths()
		}
		items[index] = compose.InlineItem{
			Runs: runs, Node: node, Break: source.Break,
			Padding:        compose.Insets{Top: source.Padding.Top, Right: source.Padding.Right, Bottom: source.Padding.Bottom, Left: source.Padding.Left},
			Margin:         compose.Insets{Top: source.Margin.Top, Right: source.Margin.Right, Bottom: source.Margin.Bottom, Left: source.Margin.Left},
			PaddingLengths: paddingLengths, MarginLengths: marginLengths,
			Background: background, Border: border,
			BorderTop: top, BorderRight: right, BorderBottom: bottom, BorderLeft: left,
			Radius:     source.Radius,
			LineHeight: source.LineHeight, VerticalAlign: vertical, Wrap: itemWrap,
			Top: source.Top.length, Right: source.Right.length,
			Bottom: source.Bottom.length, Left: source.Left.length,
		}
	}
	return compose.Inline{Size: value.Size.point(), Items: items, Align: align, Wrap: wrap, LineHeight: value.LineHeight}, nil
}

type runJSON struct {
	Text     string   `json:"text"`
	Font     string   `json:"font,omitempty"`
	Fallback []string `json:"fallback,omitempty"`
	Size     int      `json:"size,omitempty"`
	Ink      string   `json:"ink,omitempty"`
}

func (t textJSON) node(path string) (compose.Node, error) {
	runs := make([]display.TextRun, len(t.Runs))
	for index, run := range t.Runs {
		ink, err := parseInk(run.Ink)
		if err != nil {
			return nil, fmt.Errorf("%s.runs[%d].ink: %w", path, index, err)
		}
		runs[index] = display.TextRun{Text: run.Text, Style: display.TextStyle{Font: run.Font, Fallback: run.Fallback, Size: run.Size, Ink: ink}}
	}
	align, err := parseHorizontalAlign(t.Align)
	if err != nil {
		return nil, fmt.Errorf("%s.align: %w", path, err)
	}
	vertical, err := parseVerticalAlign(t.VerticalAlign)
	if err != nil {
		return nil, fmt.Errorf("%s.verticalAlign: %w", path, err)
	}
	wrap, err := parseWrap(t.Wrap)
	if err != nil {
		return nil, fmt.Errorf("%s.wrap: %w", path, err)
	}
	return compose.Text{Size: t.Size.point(), Runs: runs, Align: align, VerticalAlign: vertical, Wrap: wrap, LineHeight: t.LineHeight}, nil
}

type imageJSON struct {
	Type       string             `json:"type"`
	Size       sizeJSON           `json:"size,omitempty"`
	Source     string             `json:"source"`
	Processing string             `json:"processing,omitempty"`
	Options    imageOptionsJSON   `json:"options,omitempty"`
	Overrides  imageOverridesJSON `json:"overrides,omitempty"`
	Contrast   *contrastJSON      `json:"contrast,omitempty"`
}
type contrastJSON struct {
	Radius int     `json:"radius"`
	Amount float64 `json:"amount"`
}
type imageOptionsJSON struct {
	Fit          string `json:"fit,omitempty"`
	Sampling     string `json:"sampling,omitempty"`
	Dither       string `json:"dither,omitempty"`
	Threshold    int    `json:"threshold,omitempty"`
	RedThreshold int    `json:"redThreshold,omitempty"`
	RedMaxGreen  int    `json:"redMaxGreen,omitempty"`
	DisableRed   bool   `json:"disableRed,omitempty"`
}
type imageOverridesJSON struct {
	Fit          *string `json:"fit,omitempty"`
	Sampling     *string `json:"sampling,omitempty"`
	Dither       *string `json:"dither,omitempty"`
	Threshold    *int    `json:"threshold,omitempty"`
	RedThreshold *int    `json:"redThreshold,omitempty"`
	RedMaxGreen  *int    `json:"redMaxGreen,omitempty"`
	DisableRed   *bool   `json:"disableRed,omitempty"`
}

func (d Decoder) decodeImage(value imageJSON, path string) (compose.Node, error) {
	source, err := d.loadImage(value.Source)
	if err != nil {
		return nil, fmt.Errorf("%s.source: %w", path, err)
	}
	processing := compose.ImageManual
	switch value.Processing {
	case "", "manual":
	case "auto":
		processing = compose.ImageAuto
	default:
		return nil, fmt.Errorf("%s.processing: unknown value %q", path, value.Processing)
	}
	options, err := value.Options.options(path + ".options")
	if err != nil {
		return nil, err
	}
	overrides, err := value.Overrides.overrides(path + ".overrides")
	if err != nil {
		return nil, err
	}
	var contrast *compose.Contrast
	if value.Contrast != nil {
		contrast = &compose.Contrast{Radius: value.Contrast.Radius, Amount: value.Contrast.Amount}
	}
	return compose.Image{Size: value.Size.point(), Source: source, Processing: processing, Options: options, Overrides: overrides, Contrast: contrast}, nil
}

func (d Decoder) loadImage(source string) (image.Image, error) {
	if source == "" {
		return nil, fmt.Errorf("field is required")
	}
	var reader io.ReadSeeker
	if resource, ok := d.Resources[source]; ok {
		reader = bytes.NewReader(resource)
	} else if strings.HasPrefix(source, "data:") {
		comma := strings.IndexByte(source, ',')
		if comma < 0 || !strings.HasSuffix(source[:comma], ";base64") {
			return nil, fmt.Errorf("only base64 data URLs are supported")
		}
		decoded, err := base64.StdEncoding.DecodeString(source[comma+1:])
		if err != nil {
			return nil, fmt.Errorf("decode data URL: %w", err)
		}
		reader = bytes.NewReader(decoded)
	} else {
		parsed, err := url.Parse(source)
		if err != nil {
			return nil, fmt.Errorf("invalid source: %w", err)
		}
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			content, err := fetchRemoteImage(source)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(content)
		} else if parsed.Scheme != "" && parsed.Scheme != "file" {
			return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
		} else if parsed.Scheme == "file" && parsed.Host != "" && parsed.Host != "localhost" {
			return nil, fmt.Errorf("file URL host %q is not supported", parsed.Host)
		} else if parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("local image source must not contain a query or fragment")
		} else if d.ResourcesOnly {
			return nil, fmt.Errorf("local image sources are not allowed")
		} else {
			path := source
			if parsed.Scheme == "file" {
				path = parsed.Path
			}
			if d.RestrictFiles && (parsed.Scheme == "file" || filepath.IsAbs(path)) {
				return nil, fmt.Errorf("absolute image paths are not allowed")
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(d.BaseDir, path)
			}
			if d.RestrictFiles {
				path, err = confinedPath(d.BaseDir, path)
				if err != nil {
					return nil, err
				}
			}
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			reader = file
		}
	}
	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		return nil, fmt.Errorf("decode image configuration: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > display.MaxFramePixels/config.Height {
		return nil, fmt.Errorf("image dimensions %dx%d exceed the %d pixel limit",
			config.Width, config.Height, display.MaxFramePixels)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind image: %w", err)
	}
	decoded, format, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	return decoded, nil
}

func fetchRemoteImage(source string) ([]byte, error) {
	response, err := remoteImageClient.Get(source)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch image: HTTP %s", response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image response: %w", err)
	}
	if len(content) > maxRemoteImageBytes {
		return nil, fmt.Errorf("image response exceeds the %d byte limit", maxRemoteImageBytes)
	}
	return content, nil
}

func confinedPath(root, candidate string) (string, error) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve asset root: %w", err)
	}
	candidatePath, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve image path: %w", err)
	}
	relative, err := filepath.Rel(rootPath, candidatePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("image path escapes the configured asset directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve asset directory: %w", err)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidatePath)
	if err != nil {
		return "", err
	}
	relative, err = filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("image path escapes the configured asset directory")
	}
	return resolvedCandidate, nil
}

type pixelJSON struct {
	Type string    `json:"type"`
	Size sizeJSON  `json:"size,omitempty"`
	At   pointJSON `json:"at"`
	Ink  string    `json:"ink"`
}
type rectangleJSON struct {
	Type         string      `json:"type"`
	Size         sizeJSON    `json:"size,omitempty"`
	Radius       int         `json:"radius,omitempty"`
	Fill         *string     `json:"fill,omitempty"`
	Stroke       *strokeJSON `json:"stroke,omitempty"`
	StrokeTop    *strokeJSON `json:"strokeTop,omitempty"`
	StrokeRight  *strokeJSON `json:"strokeRight,omitempty"`
	StrokeBottom *strokeJSON `json:"strokeBottom,omitempty"`
	StrokeLeft   *strokeJSON `json:"strokeLeft,omitempty"`
}
type lineJSON struct {
	Type   string     `json:"type"`
	Size   sizeJSON   `json:"size,omitempty"`
	From   pointJSON  `json:"from"`
	To     pointJSON  `json:"to"`
	Stroke strokeJSON `json:"stroke"`
}
type polylineJSON struct {
	Type   string      `json:"type"`
	Size   sizeJSON    `json:"size,omitempty"`
	Points []pointJSON `json:"points"`
	Stroke strokeJSON  `json:"stroke"`
}
type polygonJSON struct {
	Type   string      `json:"type"`
	Size   sizeJSON    `json:"size,omitempty"`
	Points []pointJSON `json:"points"`
	Fill   *string     `json:"fill,omitempty"`
	Stroke *strokeJSON `json:"stroke,omitempty"`
}
type circleJSON struct {
	Type   string      `json:"type"`
	Size   sizeJSON    `json:"size,omitempty"`
	Center pointJSON   `json:"center"`
	Radius int         `json:"radius"`
	Fill   *string     `json:"fill,omitempty"`
	Stroke *strokeJSON `json:"stroke,omitempty"`
}
type paintedJSON struct {
	Type   string      `json:"type"`
	Size   sizeJSON    `json:"size,omitempty"`
	Fill   *string     `json:"fill,omitempty"`
	Stroke *strokeJSON `json:"stroke,omitempty"`
}
type arcJSON struct {
	Type   string     `json:"type"`
	Size   sizeJSON   `json:"size,omitempty"`
	Start  float64    `json:"start,omitempty"`
	Sweep  float64    `json:"sweep,omitempty"`
	Stroke strokeJSON `json:"stroke"`
}
type segmentJSON struct {
	Type  string   `json:"type"`
	Size  sizeJSON `json:"size,omitempty"`
	Start float64  `json:"start,omitempty"`
	Sweep float64  `json:"sweep,omitempty"`
	Ink   string   `json:"ink"`
}
type pathJSON struct {
	Type     string            `json:"type"`
	Size     sizeJSON          `json:"size,omitempty"`
	Commands []pathCommandJSON `json:"commands"`
	Fill     *string           `json:"fill,omitempty"`
	Stroke   *strokeJSON       `json:"stroke,omitempty"`
}
type pathCommandJSON struct {
	Op       string    `json:"op"`
	To       pointJSON `json:"to,omitempty"`
	Control  pointJSON `json:"control,omitempty"`
	Control1 pointJSON `json:"control1,omitempty"`
	Control2 pointJSON `json:"control2,omitempty"`
	Bounds   rectJSON  `json:"bounds,omitempty"`
	Start    float64   `json:"start,omitempty"`
	Sweep    float64   `json:"sweep,omitempty"`
	Rotation float64   `json:"rotation,omitempty"`
}

func (p pathJSON) path(path string) (display.Path, error) {
	var result display.Path
	for index, command := range p.Commands {
		switch command.Op {
		case "move":
			result.MoveTo(command.To.point())
		case "line":
			result.LineTo(command.To.point())
		case "quadratic":
			result.QuadraticTo(command.Control.point(), command.To.point())
		case "cubic":
			result.CubicTo(command.Control1.point(), command.Control2.point(), command.To.point())
		case "arc":
			result.Arc(display.Oval{Bounds: command.Bounds.rect(), Rotation: command.Rotation}, command.Start, command.Sweep)
		case "close":
			result.Close()
		default:
			return display.Path{}, fmt.Errorf("%s[%d].op: unknown operation %q", path, index, command.Op)
		}
	}
	return result, nil
}

type patternJSON struct {
	Type string            `json:"type"`
	Size sizeJSON          `json:"size,omitempty"`
	Rows []string          `json:"rows"`
	Inks map[string]string `json:"inks"`
}
type clipPathJSON struct {
	Type  string          `json:"type"`
	Size  sizeJSON        `json:"size,omitempty"`
	Path  pathJSON        `json:"path"`
	Child json.RawMessage `json:"child"`
}
type clipRectJSON struct {
	Type  string          `json:"type"`
	Size  sizeJSON        `json:"size,omitempty"`
	Rect  rectJSON        `json:"rect"`
	Child json.RawMessage `json:"child"`
}

type strokeJSON struct {
	Ink        string `json:"ink"`
	Width      int    `json:"width"`
	Dash       []int  `json:"dash,omitempty"`
	DashOffset int    `json:"dashOffset,omitempty"`
	Cap        string `json:"cap,omitempty"`
	Join       string `json:"join,omitempty"`
}

func (s strokeJSON) style(path string) (display.StrokeStyle, error) {
	ink, err := parseInk(s.Ink)
	if err != nil {
		return display.StrokeStyle{}, fmt.Errorf("%s.ink: %w", path, err)
	}
	cap := display.StrokeCapSquare
	if s.Cap != "" {
		switch s.Cap {
		case "butt":
			cap = display.StrokeCapButt
		case "round":
			cap = display.StrokeCapRound
		case "square":
			cap = display.StrokeCapSquare
		default:
			return display.StrokeStyle{}, fmt.Errorf("%s.cap: %q is not one of butt, round or square", path, s.Cap)
		}
	}
	join := display.StrokeJoinMiter
	if s.Join != "" {
		switch s.Join {
		case "miter":
			join = display.StrokeJoinMiter
		case "round":
			join = display.StrokeJoinRound
		case "bevel":
			join = display.StrokeJoinBevel
		default:
			return display.StrokeStyle{}, fmt.Errorf("%s.join: %q is not one of miter, round or bevel", path, s.Join)
		}
	}
	return display.StrokeStyle{Ink: ink, Width: s.Width, Dash: s.Dash, DashOffset: s.DashOffset, Cap: cap, Join: join}, nil
}

func parsePaint(fillName *string, strokeSource *strokeJSON, path string) (*display.Ink, *display.StrokeStyle, error) {
	var fill *display.Ink
	if fillName != nil {
		ink, err := parseInk(*fillName)
		if err != nil {
			return nil, nil, fmt.Errorf("%s.fill: %w", path, err)
		}
		fill = compose.Ink(ink)
	}
	var stroke *display.StrokeStyle
	if strokeSource != nil {
		value, err := strokeSource.style(path + ".stroke")
		if err != nil {
			return nil, nil, err
		}
		stroke = compose.Stroke(value)
	}
	return fill, stroke, nil
}

func optionalStrokes(sources ...*strokeJSON) (*display.StrokeStyle, *display.StrokeStyle, *display.StrokeStyle, *display.StrokeStyle, error) {
	result := [4]*display.StrokeStyle{}
	names := []string{"Top", "Right", "Bottom", "Left"}
	for index, source := range sources {
		if source == nil {
			continue
		}
		value, err := source.style("rectangle.stroke" + names[index])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		result[index] = compose.Stroke(value)
	}
	return result[0], result[1], result[2], result[3], nil
}

func points(source []pointJSON) []image.Point {
	result := make([]image.Point, len(source))
	for index, point := range source {
		result[index] = point.point()
	}
	return result
}

func (o imageOptionsJSON) options(path string) (display.ImageOptions, error) {
	fit, err := parseFit(o.Fit)
	if err != nil {
		return display.ImageOptions{}, fmt.Errorf("%s.fit: %w", path, err)
	}
	sampling, err := parseSampling(o.Sampling)
	if err != nil {
		return display.ImageOptions{}, fmt.Errorf("%s.sampling: %w", path, err)
	}
	dither, err := parseDither(o.Dither)
	if err != nil {
		return display.ImageOptions{}, fmt.Errorf("%s.dither: %w", path, err)
	}
	return display.ImageOptions{Fit: fit, Sampling: sampling, Dither: dither, Threshold: o.Threshold, RedThreshold: o.RedThreshold, RedMaxGreen: o.RedMaxGreen, DisableRed: o.DisableRed}, nil
}

func (o imageOverridesJSON) overrides(path string) (compose.ImageOverrides, error) {
	result := compose.ImageOverrides{Threshold: o.Threshold, RedThreshold: o.RedThreshold, RedMaxGreen: o.RedMaxGreen, DisableRed: o.DisableRed}
	if o.Fit != nil {
		value, err := parseFit(*o.Fit)
		if err != nil {
			return result, fmt.Errorf("%s.fit: %w", path, err)
		}
		result.Fit = &value
	}
	if o.Sampling != nil {
		value, err := parseSampling(*o.Sampling)
		if err != nil {
			return result, fmt.Errorf("%s.sampling: %w", path, err)
		}
		result.Sampling = &value
	}
	if o.Dither != nil {
		value, err := parseDither(*o.Dither)
		if err != nil {
			return result, fmt.Errorf("%s.dither: %w", path, err)
		}
		result.Dither = &value
	}
	return result, nil
}

func parseInk(value string) (display.Ink, error) {
	switch value {
	case "", "black":
		return display.InkBlack, nil
	case "white":
		return display.InkWhite, nil
	case "red":
		return display.InkRed, nil
	case "yellow":
		return display.InkYellow, nil
	default:
		return 0, enumError(value, "black", "white", "red", "yellow")
	}
}

func parseOrientation(value string) (display.Orientation, error) {
	switch value {
	case "", "landscape":
		return display.OrientationLandscape, nil
	case "portraitClockwise":
		return display.OrientationPortraitClockwise, nil
	case "portraitCounterClockwise":
		return display.OrientationPortraitCounterClockwise, nil
	default:
		return 0, enumError(value, "landscape", "portraitClockwise", "portraitCounterClockwise")
	}
}

func parseHorizontalAlign(value string) (display.HorizontalAlign, error) {
	switch value {
	case "", "start":
		return display.AlignStart, nil
	case "center":
		return display.AlignCenter, nil
	case "end":
		return display.AlignEnd, nil
	default:
		return 0, enumError(value, "start", "center", "end")
	}
}

func parseVerticalAlign(value string) (display.VerticalAlign, error) {
	switch value {
	case "", "top":
		return display.AlignTop, nil
	case "middle":
		return display.AlignMiddle, nil
	case "bottom":
		return display.AlignBottom, nil
	default:
		return 0, enumError(value, "top", "middle", "bottom")
	}
}

func parseInlineVerticalAlign(value string) (compose.InlineVerticalAlign, error) {
	switch value {
	case "", "baseline":
		return compose.InlineBaseline, nil
	case "top":
		return compose.InlineTop, nil
	case "middle":
		return compose.InlineMiddle, nil
	case "bottom":
		return compose.InlineBottom, nil
	default:
		return 0, enumError(value, "baseline", "top", "middle", "bottom")
	}
}

func parseWrap(value string) (display.WrapMode, error) {
	switch value {
	case "", "none":
		return display.NoWrap, nil
	case "runes":
		return display.WrapRunes, nil
	default:
		return 0, enumError(value, "none", "runes")
	}
}

func parseMainAlign(value string) (compose.MainAlignment, error) {
	switch value {
	case "", "start":
		return compose.MainStart, nil
	case "center":
		return compose.MainCenter, nil
	case "end":
		return compose.MainEnd, nil
	default:
		return 0, enumError(value, "start", "center", "end")
	}
}

func parseCrossAlign(value string) (compose.CrossAlignment, error) {
	switch value {
	case "", "stretch":
		return compose.CrossStretch, nil
	case "start":
		return compose.CrossStart, nil
	case "center":
		return compose.CrossCenter, nil
	case "end":
		return compose.CrossEnd, nil
	default:
		return 0, enumError(value, "stretch", "start", "center", "end")
	}
}

func parseFit(value string) (display.ImageFit, error) {
	fit, known := display.ParseImageFit(value)
	if !known {
		return 0, enumError(value, display.ImageFitNames()...)
	}
	return fit, nil
}

func parseSampling(value string) (display.SamplingMode, error) {
	sampling, known := display.ParseSamplingMode(value)
	if !known {
		return 0, enumError(value, display.SamplingModeNames()...)
	}
	return sampling, nil
}

func parseDither(value string) (display.DitherMode, error) {
	dither, known := display.ParseDitherMode(value)
	if !known {
		return 0, enumError(value, display.DitherModeNames()...)
	}
	return dither, nil
}

func enumError(value string, allowed ...string) error {
	quoted := make([]string, len(allowed))
	for index, item := range allowed {
		quoted[index] = fmt.Sprintf("%q", item)
	}
	return fmt.Errorf("unknown value %q, want %s", value, strings.Join(quoted, ", "))
}

func decodeStrict(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func decodeStrictBytes(source []byte, destination any) error {
	return decodeStrict(bytes.NewReader(source), destination)
}

func nodeError(path string, err error) error { return fmt.Errorf("%s: %w", path, err) }

// DecodeNode decodes a single node, for a host that embeds one description
// inside another.
func (d Decoder) DecodeNode(source []byte) (compose.Node, error) {
	return d.decodeNode(json.RawMessage(source), "node")
}
