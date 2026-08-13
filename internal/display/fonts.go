package display

import (
	"embed"
	"fmt"
)

const (
	DefaultFontFamily = "ui"
	DefaultFontSize   = 12
	DefaultFont       = "ui-12"
	fontSource        = "https://github.com/aguegu/BitmapFont@fd7dd723d26e59a815a0871e63fa077825f11c00"
)

//go:embed fonts/HZK* fonts/MONACO*
var bundledFontFiles embed.FS

type bundledFontSpec struct {
	name       string
	encoding   BitmapEncoding
	size       int
	width      int
	height     int
	rowBytes   int
	glyphCount int
	baseline   int
	lineGap    int
	advance    int
}

var bundledFontSpecs = []bundledFontSpec{
	{name: "MONACO10", encoding: BitmapASCII, size: 10, width: 8, height: 12, rowBytes: 1, glyphCount: 127, baseline: 10, lineGap: 2, advance: 6},
	{name: "MONACO12", encoding: BitmapASCII, size: 12, width: 8, height: 15, rowBytes: 1, glyphCount: 127, baseline: 12, lineGap: 2, advance: 7},
	{name: "MONACO14", encoding: BitmapASCII, size: 14, width: 8, height: 18, rowBytes: 1, glyphCount: 127, baseline: 14, lineGap: 2, advance: 8},
	{name: "MONACO16", encoding: BitmapASCII, size: 16, width: 10, height: 20, rowBytes: 2, glyphCount: 127, baseline: 16, lineGap: 2, advance: 10},
	{name: "HZK12", encoding: BitmapGB2312, width: 12, height: 12, rowBytes: 2, glyphCount: 87 * 94},
	{name: "HZK14", encoding: BitmapGB2312, width: 14, height: 14, rowBytes: 2, glyphCount: 87 * 94},
	{name: "HZK16", encoding: BitmapGB2312, width: 16, height: 16, rowBytes: 2, glyphCount: 87 * 94},
}

// NewBuiltinFontRegistry loads the selected HZK and Monaco bitmap strikes.
func NewBuiltinFontRegistry() (*FontRegistry, error) {
	registry := NewFontRegistry()
	faces := make(map[string]Face, len(bundledFontSpecs))
	for _, spec := range bundledFontSpecs {
		data, err := bundledFontFiles.ReadFile("fonts/" + spec.name)
		if err != nil {
			return nil, fmt.Errorf("read embedded font %s: %w", spec.name, err)
		}
		face, err := NewBitmapFace(BitmapFontSpec{
			Name:         spec.name,
			Encoding:     spec.encoding,
			Size:         spec.size,
			Width:        spec.width,
			Height:       spec.height,
			RowBytes:     spec.rowBytes,
			GlyphCount:   spec.glyphCount,
			Baseline:     defaultInt(spec.baseline, max(1, spec.height-2)),
			LineGap:      defaultInt(spec.lineGap, 2),
			Advance:      spec.advance,
			Data:         data,
			MaxASCIIRune: 0x7e,
		})
		if err != nil {
			return nil, err
		}
		faces[spec.name] = face
		set, err := NewFontSet(spec.name, face)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(set); err != nil {
			return nil, err
		}
	}

	for name, pair := range map[string][2]string{
		"ui-12": {"MONACO10", "HZK12"},
		"ui-14": {"MONACO12", "HZK14"},
		"ui-16": {"MONACO14", "HZK16"},
	} {
		set, err := NewFontSet(name, faces[pair[0]], faces[pair[1]])
		if err != nil {
			return nil, err
		}
		if err := registry.Register(set); err != nil {
			return nil, err
		}
	}

	families := map[string]map[int]string{
		"ui":     {12: "ui-12", 14: "ui-14", 16: "ui-16"},
		"hzk":    {12: "HZK12", 14: "HZK14", 16: "HZK16"},
		"monaco": {10: "MONACO10", 12: "MONACO12", 14: "MONACO14", 16: "MONACO16"},
	}
	for family, sizes := range families {
		for size, setName := range sizes {
			if err := registry.RegisterFamily(family, size, setName); err != nil {
				return nil, err
			}
		}
	}
	if err := registerEnlargements(registry, faces); err != nil {
		return nil, err
	}
	return registry, nil
}

// enlargements are the whole-number factors the bundled strikes are offered at.
// Three is the practical limit: a 16 pixel strike at 3x is 48 pixels, which is
// a third of the short edge of the largest panel known here.
var enlargements = []int{2, 3}

// registerEnlargements offers each strike at whole-number multiples, so that a
// document can ask for a size a bigger panel needs without a second font file.
//
// A ui set is enlarged as a set, with both of its faces taking the same factor.
// The two faces are chosen so their baselines line up, and scaling them apart
// would break the alignment that pairing exists for.
func registerEnlargements(registry *FontRegistry, faces map[string]Face) error {
	pairs := map[string][2]string{
		"ui-12": {"MONACO10", "HZK12"},
		"ui-14": {"MONACO12", "HZK14"},
		"ui-16": {"MONACO14", "HZK16"},
	}
	base := map[string]map[int]string{
		"ui":     {12: "ui-12", 14: "ui-14", 16: "ui-16"},
		"hzk":    {12: "HZK12", 14: "HZK14", 16: "HZK16"},
		"monaco": {10: "MONACO10", 12: "MONACO12", 14: "MONACO14", 16: "MONACO16"},
	}
	for _, factor := range enlargements {
		scaled := make(map[string]Face, len(faces))
		for name, face := range faces {
			enlarged, err := newScaledFace(face, factor)
			if err != nil {
				return err
			}
			scaled[name] = enlarged
			if err := registerSet(registry, enlarged.Name(), enlarged); err != nil {
				return err
			}
		}
		for name, pair := range pairs {
			setName := fmt.Sprintf("%s@%dx", name, factor)
			if err := registerSet(registry, setName, scaled[pair[0]], scaled[pair[1]]); err != nil {
				return err
			}
		}
		for family, sizes := range base {
			for size, setName := range sizes {
				enlargedSet := setName
				if family == "ui" {
					enlargedSet = fmt.Sprintf("%s@%dx", setName, factor)
				} else {
					enlargedSet = scaled[setName].Name()
				}
				if err := registry.RegisterFamily(family, size*factor, enlargedSet); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func registerSet(registry *FontRegistry, name string, faces ...Face) error {
	set, err := NewFontSet(name, faces...)
	if err != nil {
		return err
	}
	return registry.Register(set)
}

func BundledFontSource() string {
	return fontSource
}

func defaultInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
