package compose

import (
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
	Path    string
	Code    string
	Message string
}

// ImageDecision makes every automatic image adaptation visible to the caller.
type ImageDecision struct {
	Path                 string
	Profile              ImageProfile
	Options              display.ImageOptions
	ToneByColourDistance bool
	ContrastEnhanced     bool
}

// Report describes what compilation measured without changing the document.
type Report struct {
	Bounds       image.Rectangle
	MissingRunes []rune
	Warnings     []Warning
	Images       []ImageDecision
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
	native, err := display.NewPage(d.Orientation, d.Background)
	if err != nil {
		return nil, err
	}
	var frame *display.Frame
	if native.Bounds().Size() == d.Size {
		frame = native
	} else {
		frame, err = display.NewFrame(d.Size.X, d.Size.Y, d.Background)
		if err != nil {
			return nil, err
		}
	}
	if err := d.List.Replay(display.NewCanvas(frame)); err != nil {
		return nil, err
	}
	return frame, nil
}

type Compiler struct {
	Fonts *display.FontRegistry
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
	var page *display.Frame
	var err error
	if document.Size != (image.Point{}) {
		if !validSize(document.Size) || document.Size.X == 0 || document.Size.Y == 0 {
			return nil, Report{}, fmt.Errorf("document size must be positive, got %v", document.Size)
		}
		page, err = display.NewFrame(document.Size.X, document.Size.Y, background)
	} else {
		page, err = display.NewPage(document.Orientation, background)
	}
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
		if err := document.Root.paint(ctx, list, pageBounds, "root"); err != nil {
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
