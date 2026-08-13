// Package scene decodes the versioned JSON page description used by the CLI
// and transports. It is deliberately separate from compose: JSON is the
// stable input contract, while compose remains an internal compiler model.
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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

const Version = 1

type Decoder struct {
	BaseDir       string
	RestrictFiles bool
	Resources     map[string][]byte
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
			children[index] = compose.LayoutChild{
				Node: node, Basis: child.Basis.length, Grow: child.Grow,
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
			return compose.Row{Size: value.Size.point(), Gap: value.Gap, MainAlign: mainAlign, CrossAlign: crossAlign, Children: children}, nil
		}
		return compose.Column{Size: value.Size.point(), Gap: value.Gap, MainAlign: mainAlign, CrossAlign: crossAlign, Children: children}, nil
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
		return compose.Padding{Insets: compose.Insets{Top: value.Insets.Top, Right: value.Insets.Right, Bottom: value.Insets.Bottom, Left: value.Insets.Left}, Child: child}, nil
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
		return compose.Rectangle{Size: value.Size.point(), Radius: value.Radius, Fill: fill, Stroke: stroke}, nil
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
	MainAlign  string            `json:"mainAlign,omitempty"`
	CrossAlign string            `json:"crossAlign,omitempty"`
	Children   []layoutChildJSON `json:"children,omitempty"`
}
type layoutChildJSON struct {
	Node  json.RawMessage `json:"node"`
	Basis lengthJSON      `json:"basis,omitempty"`
	Grow  int             `json:"grow,omitempty"`
}

// lengthJSON accepts a number of pixels or a percentage written as a string,
// because compose resolves both and a document that can only say one of them
// would be the poorer description.
type lengthJSON struct {
	length compose.Length
}

func (l *lengthJSON) UnmarshalJSON(data []byte) error {
	var pixels int
	if err := json.Unmarshal(data, &pixels); err == nil {
		// Zero and absent are the same in JSON, and absent means measure it.
		if pixels > 0 {
			l.length = compose.Pixels(pixels)
		}
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("a length is a number of pixels or a percentage such as \"25%%\"")
	}
	trimmed := strings.TrimSpace(text)
	if !strings.HasSuffix(trimmed, "%") {
		return fmt.Errorf("%q is not a percentage; a length is a number of pixels or a string ending in %%", text)
	}
	percent, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
	if err != nil || percent < 0 {
		return fmt.Errorf("%q is not a percentage", text)
	}
	l.length = compose.Tenths(int(percent * 10))
	return nil
}

type stackJSON struct {
	Type     string            `json:"type"`
	Size     sizeJSON          `json:"size,omitempty"`
	Children []json.RawMessage `json:"children,omitempty"`
}
type paddingJSON struct {
	Type   string          `json:"type"`
	Insets insetsJSON      `json:"insets"`
	Child  json.RawMessage `json:"child"`
}
type insetsJSON struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
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
type runJSON struct {
	Text string `json:"text"`
	Font string `json:"font,omitempty"`
	Size int    `json:"size,omitempty"`
	Ink  string `json:"ink,omitempty"`
}

func (t textJSON) node(path string) (compose.Node, error) {
	runs := make([]display.TextRun, len(t.Runs))
	for index, run := range t.Runs {
		ink, err := parseInk(run.Ink)
		if err != nil {
			return nil, fmt.Errorf("%s.runs[%d].ink: %w", path, index, err)
		}
		runs[index] = display.TextRun{Text: run.Text, Style: display.TextStyle{Font: run.Font, Size: run.Size, Ink: ink}}
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
	var reader io.Reader
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
		if parsed.Scheme != "" && parsed.Scheme != "file" {
			return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
		}
		if parsed.Scheme == "file" && parsed.Host != "" && parsed.Host != "localhost" {
			return nil, fmt.Errorf("file URL host %q is not supported", parsed.Host)
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("local image source must not contain a query or fragment")
		}
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
	decoded, format, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	return decoded, nil
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
	Type   string      `json:"type"`
	Size   sizeJSON    `json:"size,omitempty"`
	Radius int         `json:"radius,omitempty"`
	Fill   *string     `json:"fill,omitempty"`
	Stroke *strokeJSON `json:"stroke,omitempty"`
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
			result.Arc(command.Bounds.rect(), command.Start, command.Sweep)
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
}

func (s strokeJSON) style(path string) (display.StrokeStyle, error) {
	ink, err := parseInk(s.Ink)
	if err != nil {
		return display.StrokeStyle{}, fmt.Errorf("%s.ink: %w", path, err)
	}
	return display.StrokeStyle{Ink: ink, Width: s.Width, Dash: s.Dash, DashOffset: s.DashOffset}, nil
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
	default:
		return 0, enumError(value, "black", "white", "red")
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
	switch value {
	case "", "stretch":
		return display.FitStretch, nil
	case "contain":
		return display.FitContain, nil
	case "cover":
		return display.FitCover, nil
	default:
		return 0, enumError(value, "stretch", "contain", "cover")
	}
}

func parseSampling(value string) (display.SamplingMode, error) {
	switch value {
	case "", "nearest":
		return display.SampleNearest, nil
	case "bilinear":
		return display.SampleBilinear, nil
	default:
		return 0, enumError(value, "nearest", "bilinear")
	}
}

func parseDither(value string) (display.DitherMode, error) {
	switch value {
	case "", "threshold":
		return display.DitherThreshold, nil
	case "floydSteinberg":
		return display.DitherFloydSteinberg, nil
	case "ordered":
		return display.DitherOrdered, nil
	default:
		return 0, enumError(value, "threshold", "floydSteinberg", "ordered")
	}
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
