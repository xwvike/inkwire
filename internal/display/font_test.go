package display

import (
	"crypto/sha1"
	"fmt"
	"slices"
	"testing"
)

func TestBundledHZK12MatchesVerifiedUpstreamBlob(t *testing.T) {
	data, err := bundledFontFiles.ReadFile("fonts/HZK12")
	if err != nil {
		t.Fatal(err)
	}
	header := fmt.Sprintf("blob %d\x00", len(data))
	hash := sha1.Sum(append([]byte(header), data...))
	if got, want := fmt.Sprintf("%x", hash), "877acf27cf08376ec3635c9f9603554d70d67734"; got != want {
		t.Fatalf("HZK12 blob SHA = %s, want %s", got, want)
	}
}

func TestBuiltinFontRegistryRoutesASCIIAndGB2312(t *testing.T) {
	registry := builtinRegistry(t)
	set, ok := registry.Lookup(DefaultFont)
	if !ok {
		t.Fatalf("default font %q is not registered", DefaultFont)
	}

	latin, ok := set.Resolve('A')
	if !ok || latin.Face != "MONACO10" || latin.Glyph.Width != 8 || latin.Glyph.Height != 12 || latin.Glyph.Advance != 6 {
		t.Fatalf("ASCII resolution = %+v, %v", latin, ok)
	}
	cjk, ok := set.Resolve('中')
	if !ok || cjk.Face != "HZK12" || cjk.Glyph.Width != 12 || cjk.Glyph.Height != 12 {
		t.Fatalf("CJK resolution = %+v, %v", cjk, ok)
	}
	if glyphIsBlank(cjk.Glyph) {
		t.Fatal("the HZK12 glyph for 中 is blank")
	}
	for _, r := range []rune{'a', '0', '?'} {
		glyph, ok := set.Resolve(r)
		if !ok || glyph.Face != "MONACO10" {
			t.Errorf("ASCII %q resolved to %q, want MONACO10", r, glyph.Face)
		}
	}
	for _, r := range []rune{'Ａ', 'ａ', '０', '？'} {
		glyph, ok := set.Resolve(r)
		if !ok || glyph.Face != "HZK12" {
			t.Errorf("full-width %q resolved to %q, want HZK12", r, glyph.Face)
		}
	}
}

func TestBuiltinFontsExposeNativeSizesAndStyles(t *testing.T) {
	registry := builtinRegistry(t)
	for _, name := range []string{"HZK12", "HZK14", "HZK16"} {
		set, ok := registry.Lookup(name)
		if !ok {
			t.Errorf("font %s is not registered", name)
			continue
		}
		glyph, ok := set.Resolve('餐')
		if !ok || glyphIsBlank(glyph.Glyph) {
			t.Errorf("font %s has no usable glyph for 餐", name)
		}
	}
}

func TestFontRegistryMatchesLogicalUISizes(t *testing.T) {
	registry := builtinRegistry(t)
	if got, want := registry.Sizes("ui"), []int{12, 14, 16}; !slices.Equal(got, want) {
		t.Fatalf("ui sizes = %v, want %v", got, want)
	}
	if got, want := registry.Sizes("monaco"), []int{10, 12, 14, 16}; !slices.Equal(got, want) {
		t.Fatalf("Monaco sizes = %v, want %v", got, want)
	}
	set, ok := registry.Match("ui", 14)
	if !ok {
		t.Fatal("ui 14px did not resolve")
	}
	latin, ok := set.Resolve('A')
	if !ok || latin.Face != "MONACO12" {
		t.Fatalf("ui 14px ASCII face = %q, want MONACO12", latin.Face)
	}
	cjk, ok := set.Resolve('中')
	if !ok || cjk.Face != "HZK14" {
		t.Fatalf("ui 14px CJK face = %q, want HZK14", cjk.Face)
	}
	set, ok = registry.Match("ui", 16)
	if !ok {
		t.Fatal("ui 16px did not resolve")
	}
	cjk, ok = set.Resolve('中')
	if !ok || cjk.Face != "HZK16" {
		t.Fatalf("ui 16px CJK face = %q, want HZK16", cjk.Face)
	}
	if _, ok := registry.Match("ui", 15); ok {
		t.Fatal("ui unexpectedly matched a non-native 15px strike")
	}
}

func TestFontSetNormalizesKnownCompatibilityCharacters(t *testing.T) {
	registry := builtinRegistry(t)
	set, _ := registry.Lookup(DefaultFont)
	for input, want := range map[rune]rune{'¥': '￥', '—': '―'} {
		glyph, ok := set.Resolve(input)
		if !ok {
			t.Errorf("%q did not resolve", input)
			continue
		}
		if glyph.Rune != want {
			t.Errorf("%q normalized to %q, want %q", input, glyph.Rune, want)
		}
	}
	if _, ok := set.Resolve('😀'); ok {
		t.Fatal("emoji unexpectedly resolved in the GB2312 font set")
	}
}

func builtinRegistry(t *testing.T) *FontRegistry {
	t.Helper()
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func glyphIsBlank(glyph Glyph) bool {
	for _, value := range glyph.Data {
		if value != 0 {
			return false
		}
	}
	return true
}
