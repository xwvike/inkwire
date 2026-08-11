package display

import (
	"fmt"
	"slices"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

type FontMetrics struct {
	Ascent  int
	Descent int
	LineGap int
}

func (m FontMetrics) Height() int {
	return m.Ascent + m.Descent + m.LineGap
}

type Glyph struct {
	Width    int
	Height   int
	RowBytes int
	Advance  int
	Data     []byte
}

func (g Glyph) On(x, y int) bool {
	if x < 0 || y < 0 || x >= g.Width || y >= g.Height {
		return false
	}
	b := g.Data[y*g.RowBytes+x/8]
	return b&(0x80>>uint(x%8)) != 0
}

type Face interface {
	Name() string
	Size() int
	Metrics() FontMetrics
	Glyph(rune) (Glyph, bool)
}

type BitmapEncoding uint8

const (
	BitmapASCII BitmapEncoding = iota
	BitmapGB2312
)

type BitmapFontSpec struct {
	Name         string
	Encoding     BitmapEncoding
	Size         int
	Width        int
	Height       int
	RowBytes     int
	GlyphCount   int
	Baseline     int
	LineGap      int
	Advance      int
	Data         []byte
	DataOffset   int
	MaxASCIIRune rune
}

type BitmapFace struct {
	spec       BitmapFontSpec
	recordSize int
}

func NewBitmapFace(spec BitmapFontSpec) (*BitmapFace, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("bitmap font name must not be empty")
	}
	if spec.Width <= 0 || spec.Height <= 0 || spec.RowBytes <= 0 {
		return nil, fmt.Errorf("font %s has invalid dimensions", spec.Name)
	}
	if spec.Size == 0 {
		spec.Size = spec.Height
	}
	if spec.Size < 0 {
		return nil, fmt.Errorf("font %s has invalid size %d", spec.Name, spec.Size)
	}
	if spec.Width > spec.RowBytes*8 {
		return nil, fmt.Errorf("font %s width %d exceeds its %d-byte rows", spec.Name, spec.Width, spec.RowBytes)
	}
	if spec.GlyphCount <= 0 {
		return nil, fmt.Errorf("font %s has invalid glyph count %d", spec.Name, spec.GlyphCount)
	}
	if spec.Baseline <= 0 || spec.Baseline > spec.Height {
		return nil, fmt.Errorf("font %s has invalid baseline %d", spec.Name, spec.Baseline)
	}
	if spec.Advance == 0 {
		spec.Advance = spec.Width
	}
	if spec.Advance < 0 {
		return nil, fmt.Errorf("font %s has invalid advance %d", spec.Name, spec.Advance)
	}
	recordSize := spec.RowBytes * spec.Height
	required := spec.DataOffset + spec.GlyphCount*recordSize
	if required > len(spec.Data) {
		return nil, fmt.Errorf("font %s needs %d bytes, got %d", spec.Name, required, len(spec.Data))
	}
	if spec.Encoding == BitmapASCII && spec.MaxASCIIRune == 0 {
		spec.MaxASCIIRune = 0x7e
	}
	return &BitmapFace{spec: spec, recordSize: recordSize}, nil
}

func (f *BitmapFace) Name() string {
	return f.spec.Name
}

func (f *BitmapFace) Size() int {
	return f.spec.Size
}

func (f *BitmapFace) Metrics() FontMetrics {
	return FontMetrics{
		Ascent:  f.spec.Baseline,
		Descent: f.spec.Height - f.spec.Baseline,
		LineGap: f.spec.LineGap,
	}
}

func (f *BitmapFace) Glyph(r rune) (Glyph, bool) {
	index, ok := f.glyphIndex(r)
	if !ok || index < 0 || index >= f.spec.GlyphCount {
		return Glyph{}, false
	}
	start := f.spec.DataOffset + index*f.recordSize
	end := start + f.recordSize
	return Glyph{
		Width:    f.spec.Width,
		Height:   f.spec.Height,
		RowBytes: f.spec.RowBytes,
		Advance:  f.spec.Advance,
		Data:     f.spec.Data[start:end],
	}, true
}

func (f *BitmapFace) glyphIndex(r rune) (int, bool) {
	switch f.spec.Encoding {
	case BitmapASCII:
		if r < 0 || r > f.spec.MaxASCIIRune {
			return 0, false
		}
		return int(r), true
	case BitmapGB2312:
		index, ok := gb2312Indices()[r]
		return index, ok
	default:
		return 0, false
	}
}

var gb2312Indices = sync.OnceValue(func() map[rune]int {
	indices := make(map[rune]int, 87*94)
	decoder := simplifiedchinese.GBK.NewDecoder()
	for b1 := 0xa1; b1 <= 0xf7; b1++ {
		for b2 := 0xa1; b2 <= 0xfe; b2++ {
			decoded, err := decoder.Bytes([]byte{byte(b1), byte(b2)})
			if err != nil {
				continue
			}
			r, size := utf8.DecodeRune(decoded)
			if r == utf8.RuneError || size != len(decoded) {
				continue
			}
			index := (b1-0xa1)*94 + b2 - 0xa1
			if _, exists := indices[r]; !exists {
				indices[r] = index
			}
		}
	}
	// CP936 and GB2312 disagree on the preferred Unicode dash code point.
	indices['\u2015'] = int(0xaa - 0xa1)
	return indices
})

type ResolvedGlyph struct {
	Rune    rune
	Glyph   Glyph
	Metrics FontMetrics
	Face    string
}

type FontSet struct {
	name  string
	faces []Face
}

func NewFontSet(name string, faces ...Face) (*FontSet, error) {
	if name == "" {
		return nil, fmt.Errorf("font set name must not be empty")
	}
	if len(faces) == 0 {
		return nil, fmt.Errorf("font set %s must contain at least one face", name)
	}
	return &FontSet{name: name, faces: slices.Clone(faces)}, nil
}

func (s *FontSet) Name() string {
	return s.name
}

func (s *FontSet) Metrics() FontMetrics {
	metrics := s.faces[0].Metrics()
	for _, face := range s.faces[1:] {
		candidate := face.Metrics()
		metrics.Ascent = max(metrics.Ascent, candidate.Ascent)
		metrics.Descent = max(metrics.Descent, candidate.Descent)
		metrics.LineGap = max(metrics.LineGap, candidate.LineGap)
	}
	return metrics
}

func (s *FontSet) Resolve(r rune) (ResolvedGlyph, bool) {
	r = normalizeRune(r)
	for _, face := range s.faces {
		glyph, ok := face.Glyph(r)
		if ok {
			return ResolvedGlyph{
				Rune:    r,
				Glyph:   glyph,
				Metrics: face.Metrics(),
				Face:    face.Name(),
			}, true
		}
	}
	return ResolvedGlyph{}, false
}

func normalizeRune(r rune) rune {
	switch r {
	case '\u00a5':
		return '\uffe5'
	case '\u2014':
		return '\u2015'
	default:
		return r
	}
}

type FontRegistry struct {
	sets     map[string]*FontSet
	families map[fontKey]string
}

type fontKey struct {
	family string
	size   int
}

func NewFontRegistry() *FontRegistry {
	return &FontRegistry{
		sets:     make(map[string]*FontSet),
		families: make(map[fontKey]string),
	}
}

func (r *FontRegistry) Register(set *FontSet) error {
	if set == nil {
		return fmt.Errorf("font set must not be nil")
	}
	if _, exists := r.sets[set.Name()]; exists {
		return fmt.Errorf("font set %s is already registered", set.Name())
	}
	r.sets[set.Name()] = set
	return nil
}

func (r *FontRegistry) Lookup(name string) (*FontSet, bool) {
	set, ok := r.sets[name]
	return set, ok
}

func (r *FontRegistry) RegisterFamily(family string, size int, setName string) error {
	if family == "" || size <= 0 {
		return fmt.Errorf("font family and positive size are required")
	}
	if _, ok := r.sets[setName]; !ok {
		return fmt.Errorf("font set %s is not registered", setName)
	}
	key := fontKey{family: family, size: size}
	if _, exists := r.families[key]; exists {
		return fmt.Errorf("font family %s already has size %d", family, size)
	}
	r.families[key] = setName
	return nil
}

// Match resolves either an exact set name or a family plus pixel size.
func (r *FontRegistry) Match(name string, size int) (*FontSet, bool) {
	if size <= 0 {
		return r.Lookup(name)
	}
	setName, ok := r.families[fontKey{family: name, size: size}]
	if !ok {
		return nil, false
	}
	return r.Lookup(setName)
}

func (r *FontRegistry) Sizes(family string) []int {
	var sizes []int
	for key := range r.families {
		if key.family == family {
			sizes = append(sizes, key.size)
		}
	}
	slices.Sort(sizes)
	return sizes
}

func (r *FontRegistry) Names() []string {
	names := make([]string, 0, len(r.sets))
	for name := range r.sets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
