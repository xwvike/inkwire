package display

import "testing"

func builtinFace(t *testing.T, name string) Face {
	t.Helper()
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := registry.Lookup(name)
	if !ok {
		t.Fatalf("font set %s is not registered", name)
	}
	return &setFace{set: set}
}

// setFace adapts a FontSet to the Face interface so the scaling can be checked
// against whatever the set resolves, without reaching into package internals.
type setFace struct{ set *FontSet }

func (f *setFace) Name() string         { return f.set.Name() }
func (f *setFace) Size() int            { return f.set.Metrics().Ascent }
func (f *setFace) Metrics() FontMetrics { return f.set.Metrics() }
func (f *setFace) Glyph(r rune) (Glyph, bool) {
	resolved, ok := f.set.Resolve(r)
	if !ok {
		return Glyph{}, false
	}
	return resolved.Glyph, true
}

// The property that makes whole-number enlargement exact: every pixel of the
// result is the pixel the source had at the corresponding position. Nothing is
// interpolated, so nothing is invented and nothing is lost.
func TestEnlargedGlyphsAreTheSourceShapeBlockByBlock(t *testing.T) {
	base := builtinFace(t, "HZK16")
	for _, factor := range []int{2, 3, 4} {
		enlarged, err := newScaledFace(base, factor)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range []rune{'中', '文', '字', '一', '餐'} {
			source, ok := base.Glyph(r)
			if !ok {
				t.Fatalf("base cannot draw %q", r)
			}
			result, ok := enlarged.Glyph(r)
			if !ok {
				t.Fatalf("%dx cannot draw %q", factor, r)
			}
			if result.Width != source.Width*factor || result.Height != source.Height*factor {
				t.Fatalf("%q at %dx is %dx%d, want %dx%d", r, factor,
					result.Width, result.Height, source.Width*factor, source.Height*factor)
			}
			if result.Advance != source.Advance*factor {
				t.Errorf("%q at %dx advances %d, want %d", r, factor, result.Advance, source.Advance*factor)
			}
			for y := 0; y < result.Height; y++ {
				for x := 0; x < result.Width; x++ {
					if got, want := result.On(x, y), source.On(x/factor, y/factor); got != want {
						t.Fatalf("%q at %dx: pixel (%d,%d) = %v, source pixel (%d,%d) = %v",
							r, factor, x, y, got, x/factor, y/factor, want)
					}
				}
			}
		}
	}
}

// Ink coverage scaling by exactly the square of the factor is a second, cheap
// check that no pixel was dropped at a row or byte boundary, which is where a
// bit-packing mistake would show up.
func TestEnlargementPreservesInkExactly(t *testing.T) {
	base := builtinFace(t, "MONACO16")
	for _, factor := range []int{2, 3} {
		enlarged, _ := newScaledFace(base, factor)
		for _, r := range []rune{'W', 'i', '8', '@', '/'} {
			source, _ := base.Glyph(r)
			result, _ := enlarged.Glyph(r)
			if got, want := countOn(result), countOn(source)*factor*factor; got != want {
				t.Errorf("%q at %dx has %d set pixels, want %d", r, factor, got, want)
			}
		}
	}
}

func countOn(g Glyph) int {
	count := 0
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if g.On(x, y) {
				count++
			}
		}
	}
	return count
}

func TestMetricsScaleWithTheGlyphs(t *testing.T) {
	base := builtinFace(t, "HZK14")
	enlarged, _ := newScaledFace(base, 3)
	before, after := base.Metrics(), enlarged.Metrics()
	if after.Ascent != before.Ascent*3 || after.Descent != before.Descent*3 || after.LineGap != before.LineGap*3 {
		t.Errorf("metrics %+v, want every field of %+v tripled", after, before)
	}
}

func TestAFactorOfOneIsTheFaceItself(t *testing.T) {
	base := builtinFace(t, "HZK12")
	same, err := newScaledFace(base, 1)
	if err != nil {
		t.Fatal(err)
	}
	if same != base {
		t.Error("scaling by one wrapped the face instead of returning it")
	}
}

func TestFractionalOrNegativeFactorsAreRefused(t *testing.T) {
	base := builtinFace(t, "HZK12")
	for _, factor := range []int{0, -1, -3} {
		if _, err := newScaledFace(base, factor); err == nil {
			t.Errorf("factor %d was accepted", factor)
		}
	}
}

// A rune the strike does not have must stay missing at every size. Enlargement
// that invented an empty box would report full coverage and print blanks.
func TestEnlargementDoesNotInventMissingRunes(t *testing.T) {
	base := builtinFace(t, "MONACO12")
	const missing = '中' // Monaco is ASCII only
	if _, ok := base.Glyph(missing); ok {
		t.Fatalf("the base strike unexpectedly has %q", missing)
	}
	for _, factor := range []int{2, 3} {
		enlarged, _ := newScaledFace(base, factor)
		if _, ok := enlarged.Glyph(missing); ok {
			t.Errorf("%q appeared at %dx", missing, factor)
		}
	}
}

// The sizes a document can ask for are the point of the whole exercise.
func TestRegistryOffersTheEnlargedSizes(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for family, want := range map[string][]int{
		"ui":     {12, 14, 16, 24, 28, 32, 36, 42, 48},
		"hzk":    {12, 14, 16, 24, 28, 32, 36, 42, 48},
		"monaco": {10, 12, 14, 16, 20, 24, 28, 30, 32, 36, 42, 48},
	} {
		got := registry.Sizes(family)
		if len(got) != len(want) {
			t.Errorf("%s offers %v, want %v", family, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s offers %v, want %v", family, got, want)
				break
			}
		}
	}

	// A size in between is still absent: these are strikes, not a range.
	if _, ok := registry.Match("hzk", 20); ok {
		t.Error("hzk reported a 20px size, which no whole-number multiple produces")
	}
	// And the enlarged set really is bigger.
	small, _ := registry.Match("hzk", 16)
	large, ok := registry.Match("hzk", 48)
	if !ok {
		t.Fatal("hzk 48 is missing")
	}
	smallGlyph, _ := small.Resolve('中')
	largeGlyph, _ := large.Resolve('中')
	if largeGlyph.Glyph.Height != smallGlyph.Glyph.Height*3 {
		t.Errorf("hzk 48 glyph is %d tall, want three times %d", largeGlyph.Glyph.Height, smallGlyph.Glyph.Height)
	}
}

// The ui sets pair a Latin and a Chinese face whose baselines line up. Scaling
// them by different amounts would break exactly what the pairing is for.
func TestEnlargedUISetsKeepTheirFacesInStep(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{12, 24, 36} {
		set, ok := registry.Match("ui", size)
		if !ok {
			t.Fatalf("ui %d is missing", size)
		}
		latin, okLatin := set.Resolve('A')
		han, okHan := set.Resolve('中')
		if !okLatin || !okHan {
			t.Fatalf("ui %d cannot draw both scripts", size)
		}
		if latin.Metrics.Ascent != han.Metrics.Ascent {
			t.Errorf("ui %d: Latin ascent %d, Chinese ascent %d; the pairing exists to keep these equal",
				size, latin.Metrics.Ascent, han.Metrics.Ascent)
		}
	}
}
