package compose

import (
	"encoding/json"
	"fmt"
	"image"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/display"
)

// Document is an explicit description of one panel page. A nil Background
// means white; no other content or style is inferred by the compiler.
type Document struct {
	Orientation display.Orientation
	// Size overrides the device page size for non-device previews such as
	// contact sheets. A zero value selects the orientation's native page size.
	Size       image.Point
	Background *display.Ink
	Root       Node
}

// Value returns an addressable copy for optional fields whose zero value is a
// valid explicit choice, such as Background or an ImageOverrides member.
func Value[T any](value T) *T { return &value }

// Node is a closed set of content and layout nodes supplied by this package.
// Keeping it closed lets every accepted document be validated and compiled to
// the same deterministic display-list operations.
type Node interface {
	composeNode()
	measure(*compileContext, image.Point, string) (image.Point, error)
	paint(*compileContext, *display.DisplayList, image.Rectangle, string) error
}

// Warning reports a non-fatal, deterministic consequence of the description.
type Warning struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ImageDecision makes every automatic image adaptation visible to the caller.
type ImageDecision struct {
	Path                 string               `json:"path"`
	Profile              ImageProfile         `json:"profile"`
	Options              display.ImageOptions `json:"options"`
	ToneByColourDistance bool                 `json:"toneByColourDistance"`
	ContrastEnhanced     bool                 `json:"contrastEnhanced"`
}

// GridExpansion reports tracks created by auto-placement beyond the grid's
// declared tracks. It is separate from warnings because implicit tracks are
// normal CSS grid behaviour, but they still need to be visible to a caller
// rendering into a fixed-size panel.
type GridExpansion struct {
	Path            string `json:"path"`
	ImplicitColumns int    `json:"implicitColumns"`
	ImplicitRows    int    `json:"implicitRows"`
}

// Report describes what compilation measured without changing the document.
//
// The Go shape and the JSON shape are deliberately different, and MarshalJSON
// is where they part. image.Rectangle marshals as a Min and a Max point, which
// is a fine way to hold a rectangle and a poor way to send one; a []rune is a
// list of code points, and a caller reading `[32751, 135361]` has been handed
// arithmetic rather than the characters a font could not draw. Both went out
// that way, inside an envelope whose own fields were camelCase, so one object
// in the response spoke Go and the rest spoke JSON.
type Report struct {
	Bounds         image.Rectangle
	MissingRunes   []rune
	Warnings       []Warning
	Images         []ImageDecision
	GridExpansions []GridExpansion
	// Placements is every node and the box it ended up in, in the order they
	// were painted. Empty unless the compiler was asked to trace: a page has
	// as many of these as it has nodes, and the answer to "did it render" does
	// not want them.
	Placements []Placement
}

// Placement is one node, where it was put, and what it would rather have had.
//
// Deciding a basis meant rendering, reading a warning and bisecting, because
// nothing would say how wide a piece of text was. Wanted says it. It is zero
// for nodes that have no opinion, and for those the box is the whole story.
type Placement struct {
	Path   string
	Type   string
	Bounds image.Rectangle
	Wanted image.Point
}

// reportJSON is the shape a report has on the wire. It is a separate type
// rather than tags on Report because two of the fields change shape and not
// just name.
type reportJSON struct {
	Bounds         boxJSON         `json:"bounds"`
	MissingRunes   string          `json:"missingRunes,omitempty"`
	Warnings       []Warning       `json:"warnings,omitempty"`
	Images         []ImageDecision `json:"images,omitempty"`
	GridExpansions []GridExpansion `json:"gridExpansions,omitempty"`
}

// boxJSON is a rectangle as an origin and a size, which is how every other
// rectangle in this schema is written.
type boxJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(reportJSON{
		Bounds: boxJSON{
			X: r.Bounds.Min.X, Y: r.Bounds.Min.Y,
			Width: r.Bounds.Dx(), Height: r.Bounds.Dy(),
		},
		// The runes are sent as the text they are. They arrive as a set of
		// characters some font could not draw, and what a caller does with
		// them is show them to somebody.
		MissingRunes:   string(r.MissingRunes),
		Warnings:       r.Warnings,
		Images:         r.Images,
		GridExpansions: r.GridExpansions,
	})
}

func (r *Report) UnmarshalJSON(data []byte) error {
	var wire reportJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*r = Report{
		Bounds: image.Rect(wire.Bounds.X, wire.Bounds.Y,
			wire.Bounds.X+wire.Bounds.Width, wire.Bounds.Y+wire.Bounds.Height),
		MissingRunes:   []rune(wire.MissingRunes),
		Warnings:       wire.Warnings,
		Images:         wire.Images,
		GridExpansions: wire.GridExpansions,
	}
	return nil
}

// CompiledDocument keeps the page properties beside the display list they
// were compiled for. Render always starts with the declared background.
type CompiledDocument struct {
	Orientation display.Orientation
	Background  display.Ink
	Size        image.Point
	List        *display.DisplayList
}

func (d *CompiledDocument) Render() (*display.Frame, error) {
	if d == nil || d.List == nil {
		return nil, fmt.Errorf("compiled document must not be nil")
	}
	frame, err := display.NewFrame(d.Size.X, d.Size.Y, d.Background)
	if err != nil {
		return nil, err
	}
	if err := d.List.Replay(display.NewCanvas(frame)); err != nil {
		return nil, err
	}
	return frame, nil
}

type Compiler struct {
	Fonts *display.FontRegistry
	// Trace fills Report.Placements. It is off by default because the cost is
	// paid per node and the answer is wanted by a person reading a tree, not by
	// a page on its way to a tag.
	Trace bool
}

func NewCompiler(fonts *display.FontRegistry) (*Compiler, error) {
	if fonts == nil {
		return nil, fmt.Errorf("font registry must not be nil")
	}
	return &Compiler{Fonts: fonts}, nil
}

func NewDefaultCompiler() (*Compiler, error) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		return nil, err
	}
	return NewCompiler(fonts)
}

func (c *Compiler) Compile(document Document) (*CompiledDocument, Report, error) {
	if c == nil || c.Fonts == nil {
		return nil, Report{}, fmt.Errorf("compiler font registry must not be nil")
	}
	if document.Root != nil && nilNode(document.Root) {
		return nil, Report{}, fmt.Errorf("document root contains a typed nil node")
	}
	background := display.InkWhite
	if document.Background != nil {
		background = *document.Background
	}
	// The compiler lays out for a page and will not invent one. It used to
	// fall back to a size that was one family's 2.9" panel, written into the
	// display layer under a constant named after that family — so a document
	// that said nothing got somebody else's tag, and said nothing about it.
	if !document.Orientation.Valid() {
		return nil, Report{}, fmt.Errorf("invalid orientation %d", document.Orientation)
	}
	if document.Size == (image.Point{}) {
		return nil, Report{}, fmt.Errorf("document has no size, and there is no default page to lay it out on")
	}
	if !validSize(document.Size) || document.Size.X <= 0 || document.Size.Y <= 0 {
		return nil, Report{}, fmt.Errorf("document size must be positive, got %v", document.Size)
	}
	page, err := display.NewFrame(document.Size.X, document.Size.Y, background)
	if err != nil {
		return nil, Report{}, err
	}

	ctx := &compileContext{compiler: c}
	pageBounds := page.Bounds()
	list := &display.DisplayList{}
	list.ClipRect(pageBounds)
	if document.Root != nil {
		if _, err := document.Root.measure(ctx, pageBounds.Size(), "root"); err != nil {
			return nil, Report{}, err
		}
		if err := ctx.paint(document.Root, list, pageBounds, "root"); err != nil {
			return nil, Report{}, err
		}
	}
	ctx.report.Bounds = list.Bounds()
	compiled := &CompiledDocument{
		Orientation: document.Orientation,
		Background:  background,
		Size:        pageBounds.Size(),
		List:        list,
	}
	return compiled, ctx.report.clone(), nil
}

type compileContext struct {
	compiler *Compiler
	report   Report
	seenRune map[rune]bool
	// measureInline is the containing block's inline size while a flex item
	// is measured for its intrinsic main size. The main-axis maximum is opened
	// during that measurement so text can report its natural width; percentages
	// must still resolve against the real containing block, not that sentinel.
	measureInline int
	// containing is the box against which a positioned child's percentages
	// resolve.
	containing image.Rectangle
	// wanted is what a node said it needed, before whatever it was given.
	// Only nodes that know an answer worth reporting fill it in.
	wanted map[string]image.Point
}

const unboundedMeasure = 1 << 30

func (c *compileContext) pushMeasureInline(width int) func() {
	previous := c.measureInline
	if width > 0 && width < unboundedMeasure {
		c.measureInline = width
	}
	return func() { c.measureInline = previous }
}

func (c *compileContext) measureInlineWidth(maximum image.Point) int {
	if maximum.X > 0 && maximum.X < unboundedMeasure {
		return maximum.X
	}
	return c.measureInline
}

// paint places a node and records where, then paints it.
//
// Every node in the tree goes through here, which is what makes the placements
// a tree rather than a list of whatever happened to be interesting. It is the
// same arrangement as warn: the context is the one thing that sees every node,
// so it is the one place that can count them.
func (c *compileContext) paint(node Node, list *display.DisplayList, bounds image.Rectangle, path string) error {
	return c.paintWithContaining(node, list, bounds, bounds, path)
}

// paintWithContaining records the box that contains node separately from the
// box allocated to node. Most nodes use the two interchangeably; relative
// positioning is the case where CSS makes the distinction observable.
func (c *compileContext) paintWithContaining(node Node, list *display.DisplayList, bounds, containing image.Rectangle, path string) error {
	previous := c.containing
	c.containing = containing
	defer func() { c.containing = previous }()
	if c.compiler.Trace {
		c.report.Placements = append(c.report.Placements, Placement{
			Path:   path,
			Type:   nodeName(node),
			Bounds: bounds,
			Wanted: c.wanted[path],
		})
	}
	return node.paint(c, list, bounds, path)
}

// wants records the size a node asked for, which is not always the size it was
// measured at: a container hands down a maximum and the measurement comes back
// clamped to it, so the number that says a box is too small has already been
// rounded off to the size of that box by the time anybody could read it.
func (c *compileContext) wants(path string, size image.Point) {
	if !c.compiler.Trace {
		return
	}
	if c.wanted == nil {
		c.wanted = make(map[string]image.Point)
	}
	c.wanted[path] = size
}

// nodeName is the type as a scene would spell it, so a placement can be found
// in the document that produced it.
func nodeName(node Node) string {
	name := fmt.Sprintf("%T", node)
	if index := strings.LastIndex(name, "."); index >= 0 {
		name = name[index+1:]
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func (c *compileContext) addMissing(path string, runes []rune, tried []string) {
	if len(runes) == 0 {
		return
	}
	if c.seenRune == nil {
		c.seenRune = make(map[rune]bool)
	}
	local := make(map[rune]bool)
	var missing []rune
	for _, r := range runes {
		if local[r] {
			continue
		}
		local[r] = true
		missing = append(missing, r)
		if !c.seenRune[r] {
			c.seenRune[r] = true
			c.report.MissingRunes = append(c.report.MissingRunes, r)
		}
	}
	if len(missing) != 0 {
		c.warn(path, "missing-runes", c.missingMessage(tried, missing))
	}
}

// missingMessage says which font came up short and, where there is one, which
// font to reach for instead.
//
// A character a font does not carry is nearly always the wrong font rather
// than a character nothing can draw: monaco is ASCII, so a weekday written in
// it goes missing while ui and hzk both have it. Naming the families that do
// have the characters turns the warning into the fix.
func (c *compileContext) missingMessage(tried []string, missing []rune) string {
	asked := "the font"
	if len(tried) != 0 {
		asked = "font " + quoteList(tried)
	}
	if able := c.compiler.Fonts.FamiliesWith(missing); len(able) != 0 {
		verb := "does"
		if len(able) > 1 {
			verb = "do"
		}
		return fmt.Sprintf("%s has no glyphs for %q; %s %s", asked, string(missing), quoteList(able), verb)
	}
	return fmt.Sprintf("%s has no glyphs for %q, and no bundled font does", asked, string(missing))
}

func quoteList(names []string) string {
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = strconv.Quote(name)
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}

func (c *compileContext) warn(path, code, message string) {
	c.report.Warnings = append(c.report.Warnings, Warning{Path: path, Code: code, Message: message})
}

func (r Report) clone() Report {
	r.MissingRunes = slices.Clone(r.MissingRunes)
	r.Warnings = slices.Clone(r.Warnings)
	r.Images = slices.Clone(r.Images)
	r.GridExpansions = slices.Clone(r.GridExpansions)
	return r
}

func childPath(parent, field string, index int) string {
	return fmt.Sprintf("%s.%s[%d]", parent, field, index)
}

func validSize(size image.Point) bool {
	return size.X >= 0 && size.Y >= 0
}

func nilNode(node Node) bool {
	if node == nil {
		return true
	}
	value := reflect.ValueOf(node)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func constrainSize(size, maximum image.Point) image.Point {
	if size.X < 0 {
		size.X = 0
	}
	if size.Y < 0 {
		size.Y = 0
	}
	if maximum.X >= 0 {
		size.X = min(size.X, maximum.X)
	}
	if maximum.Y >= 0 {
		size.Y = min(size.Y, maximum.Y)
	}
	return size
}

func preferredSize(natural, preferred, maximum image.Point) (image.Point, error) {
	if !validSize(preferred) {
		return image.Point{}, fmt.Errorf("preferred size must not be negative, got %v", preferred)
	}
	if preferred.X > 0 {
		natural.X = preferred.X
	}
	if preferred.Y > 0 {
		natural.Y = preferred.Y
	}
	return constrainSize(natural, maximum), nil
}
