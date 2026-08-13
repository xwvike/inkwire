package display

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"slices"
	"testing"
)

// Every embedded strike is pinned, so a corrupted or swapped font file fails
// here instead of silently rendering the wrong glyphs. Digests are mirrored in
// fonts/SOURCE for readers who are not running the tests.
//
// The HZK files are byte-for-byte copies from upstream and are pinned by git
// blob SHA-1, the value `git hash-object` prints for the same file in
// aguegu/BitmapFont at the recorded commit. HZK12's digest was additionally
// cross-checked against an independent mirror; HZK14 and HZK16 are recorded
// from this working copy, so they detect drift but do not by themselves prove
// provenance until someone compares them against upstream.
//
// The MONACO strikes were generated locally and have no upstream blob, so they
// are pinned by plain SHA-256.
func TestEmbeddedFontsMatchRecordedDigests(t *testing.T) {
	blobSHA1 := map[string]string{
		"HZK12": "877acf27cf08376ec3635c9f9603554d70d67734",
		"HZK14": "f71039c8d353431c25f99aa0a598e4e7ee18e14a",
		"HZK16": "8661219da1c56b74e7151fe78789121816537027",
	}
	fileSHA256 := map[string]string{
		"MONACO10": "057f04939824bf3054b32caa59ed9bfd588a006f69eb02ce01055bf31b653670",
		"MONACO12": "558820ae439e960e57931988d94adaf14ae33d2dca5814f555fcfd29a4c4757c",
		"MONACO14": "3ff16aa076214d9f4490a7d4ff65104a1c4ec2d318a2ea2761dd028d66ef5f85",
		"MONACO16": "b2427ffde1fff9d9263a634b9c86323c6f62f01efb30a8f69036463d3b008996",
	}

	loaded := make(map[string]bool, len(bundledFontSpecs))
	for _, spec := range bundledFontSpecs {
		loaded[spec.name] = true
	}

	entries, err := fs.ReadDir(bundledFontFiles, "fonts")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		data, err := bundledFontFiles.ReadFile("fonts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded[name] {
			t.Errorf("embedded font %s is dead weight: no spec loads it", name)
		}
		blob, hasBlob := blobSHA1[name]
		sum, hasSum := fileSHA256[name]
		switch {
		case hasBlob == hasSum:
			t.Errorf("embedded font %s is not pinned by exactly one digest", name)
		case hasBlob:
			header := fmt.Sprintf("blob %d\x00", len(data))
			if got := fmt.Sprintf("%x", sha1.Sum(append([]byte(header), data...))); got != blob {
				t.Errorf("%s blob SHA-1 = %s, want %s", name, got, blob)
			}
		case hasSum:
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != sum {
				t.Errorf("%s SHA-256 = %s, want %s", name, got, sum)
			}
		}
	}
	if got, want := len(entries), len(blobSHA1)+len(fileSHA256); got != want {
		t.Errorf("fonts/ embeds %d files but %d digests are recorded", got, want)
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
	// The bundled strikes, followed by the whole-number enlargements of each.
	// They are listed in full rather than derived, so that adding a factor is
	// a deliberate edit here as well as in the registry.
	if got, want := registry.Sizes("ui"), []int{12, 14, 16, 24, 28, 32, 36, 42, 48}; !slices.Equal(got, want) {
		t.Fatalf("ui sizes = %v, want %v", got, want)
	}
	if got, want := registry.Sizes("monaco"),
		[]int{10, 12, 14, 16, 20, 24, 28, 30, 32, 36, 42, 48}; !slices.Equal(got, want) {
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
