package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A page is a page whichever format it is written in. render and measure are
// the two that can be checked without a tag in the room; push reads its
// document through the same loader, which is the point of there being one.
func TestRenderAndMeasureTakeAPageAsWellAsAScene(t *testing.T) {
	directory := t.TempDir()
	page := filepath.Join(directory, "page.html")
	write(t, page, `<div class="page"><span>OK</span></div>`)
	write(t, filepath.Join(directory, "page.css"), `
		.page { display: flex; width: 60px; height: 20px; background: white;
		        align-items: center; padding: 0 4px; }
		span { font-family: monaco; font-size: 10px; }`)

	var stdout, stderr bytes.Buffer
	preview := filepath.Join(directory, "page.png")
	if code := run([]string{"render", "-o", preview, page}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(60x20)") {
		t.Errorf("the root element's size did not become the page: %s", stdout.String())
	}
	if info, err := os.Stat(preview); err != nil || info.Size() == 0 {
		t.Fatalf("preview = %v, %v", info, err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"measure", page}, &stdout, &stderr); code != 0 {
		t.Fatalf("measure code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "text") {
		t.Errorf("measure said nothing about the page's nodes: %s", stdout.String())
	}
}

// The geometry a stylesheet has no words for is handed to a scene element, and
// the resolver that reads one is the CLI's job rather than the compiler's. It
// is worth a test of its own because a page whose scene did not resolve still
// renders, so the failure is a picture with a hole in it rather than an error.
func TestAPageDrawsTheSceneItPointsAt(t *testing.T) {
	directory := t.TempDir()
	page := filepath.Join(directory, "page.html")
	write(t, page, `<div class="page"><scene src="dot.json"></scene></div>`)
	write(t, filepath.Join(directory, "page.css"),
		`.page { display: flex; width: 40px; height: 40px; background: white; }
		 scene { display: block; flex-grow: 1; }`)
	write(t, filepath.Join(directory, "dot.json"),
		`{"type":"circle","center":{"x":20,"y":20},"radius":15,"fill":"black"}`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", "-o", filepath.Join(directory, "page.png"), page}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "unresolved-scene") {
		t.Errorf("the scene element was not resolved: %s", stdout.String())
	}
}

// A page with no style anywhere lays out as almost nothing, and the error that
// follows is about a missing size. Said on its own that sends an author to the
// flags; what they are actually missing is the stylesheet.
//
// A missing file beside the page is no longer the test, because a page may
// carry its style in a style element or name it in a link. Only a page with
// none of the three is missing anything.
func TestAPageWithNoStyleAtAllSaysSo(t *testing.T) {
	directory := t.TempDir()
	page := filepath.Join(directory, "page.html")
	write(t, page, `<div><span>OK</span></div>`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", page}, &stdout, &stderr); code != 2 {
		t.Fatalf("render code = %d, want 2; stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no-stylesheet") {
		t.Errorf("the missing style was not reported: %s", stderr.String())
	}
}

// A page that carries its own style needs no file beside it, and must not be
// told it is missing one.
func TestAPageInOneFileNeedsNothingBesideIt(t *testing.T) {
	directory := t.TempDir()
	page := filepath.Join(directory, "page.html")
	write(t, page, `<style>.page { display: flex; width: 60px; height: 20px; background: white; }
		span { font-family: monaco; font-size: 10px; }</style>
		<div class="page"><span>OK</span></div>`)

	var stdout, stderr bytes.Buffer
	preview := filepath.Join(directory, "page.png")
	if code := run([]string{"render", "-o", preview, page}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "no-stylesheet") {
		t.Errorf("a page carrying its own style was told it had none: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "(60x20)") {
		t.Errorf("the style element was not applied: %s", stdout.String())
	}
}

// A link is read against the directory the page is in, the way every other
// file a page names is.
func TestALinkedStylesheetIsReadBesideThePage(t *testing.T) {
	directory := t.TempDir()
	page := filepath.Join(directory, "page.html")
	write(t, page, `<link rel="stylesheet" href="shared.css">
		<div class="page"><span>OK</span></div>`)
	write(t, filepath.Join(directory, "shared.css"),
		`.page { display: flex; width: 80px; height: 20px; background: white; }
		 span { font-family: monaco; font-size: 10px; }`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", "-o", filepath.Join(directory, "page.png"), page}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(80x20)") {
		t.Errorf("the linked stylesheet was not applied: %s stderr=%s", stdout.String(), stderr.String())
	}
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
