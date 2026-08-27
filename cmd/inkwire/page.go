package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/markup"
	"github.com/xwvike/inkwire/internal/scene"
)

// loadDocument reads a page in whichever of the two formats it is written in,
// so that every command that takes a scene takes a stylesheet too.
//
// Which one a file is, is decided by its extension and nothing else. Sniffing
// the contents would make a mistyped name render something rather than say so.
//
// The second return is what the front end could not honour on the way in. It
// is separate from the document because a page that lost a declaration still
// has to be rendered — the picture is what shows the author what went missing.
func loadDocument(source string) (compose.Document, []compose.Warning, error) {
	if !isPage(source) {
		document, err := (scene.Decoder{}).DecodeFile(source)
		return document, nil, err
	}
	written, warnings, err := compilePage(source)
	if err != nil {
		return compose.Document{}, warnings, err
	}
	// The page is now a scene document like any other, and goes in the same
	// way. Everything the decoder checks — the pixel limit on a picture, the
	// formats it will decode, a field nobody implements, a path that points
	// out of the directory — a page is held to because it is not being read
	// by anything else.
	document, err := (scene.Decoder{BaseDir: filepath.Dir(source)}).Decode(bytes.NewReader(written))
	if err != nil {
		return compose.Document{}, warnings, fmt.Errorf("%s: %w", source, err)
	}
	return document, warnings, nil
}

// isPage reports whether a path names a page rather than a scene document.
func isPage(source string) bool { return strings.EqualFold(filepath.Ext(source), ".html") }

// compilePage turns an HTML page and its stylesheets into the scene document
// it describes.
//
// A page may carry its style in three places and this reaches all of them: the
// file beside it, named by having the same path with .css on it; a style
// element in the page itself; and a link element naming another file. They
// cascade in that order, so a page overrides what it shares.
func compilePage(source string) ([]byte, []compose.Warning, error) {
	markupSource, err := os.ReadFile(source)
	if err != nil {
		return nil, nil, err
	}
	// The stylesheet beside the page is one source of style and no longer the
	// only one: a page may carry its own in a style element, or name one in a
	// link. So a missing sibling is not itself worth saying anything about —
	// the compiler says so if the page turns out to have no style at all.
	stylesheet := strings.TrimSuffix(source, filepath.Ext(source)) + ".css"
	cssSource, err := os.ReadFile(stylesheet)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	var warnings []compose.Warning

	base := filepath.Dir(source)
	compiler := markup.Compiler{
		Stylesheets: func(href string) ([]byte, error) {
			return os.ReadFile(filepath.Join(base, filepath.Clean("/"+href)))
		},
		Scenes: func(reference markup.SceneRef) (json.RawMessage, error) {
			if reference.Src == "" {
				return json.RawMessage(reference.Inline), nil
			}
			// A page reaches the drawings beside it and no further, which is
			// the rule the decoder applies to a picture and the reason this
			// is here rather than in the compiler: where a file may be read
			// from is a question about the page's origin, not its contents.
			embedded, err := os.ReadFile(filepath.Join(base, filepath.Clean("/"+reference.Src)))
			if err != nil {
				return nil, err
			}
			return embedded, nil
		},
	}
	page, err := compiler.Compile(string(markupSource), string(cssSource))
	for _, warning := range page.Warnings {
		warnings = append(warnings, compose.Warning(warning))
	}
	if err != nil {
		return nil, warnings, fmt.Errorf("%s: %w", source, err)
	}
	return page.JSON, warnings, nil
}

// runCompile prints the scene document a page compiles to.
//
// It is the one command that stops in the middle, and it is here because the
// middle is worth seeing. A stylesheet says what a page is and leaves the
// arithmetic to the layout, so the question an author has when a box is in the
// wrong place is what their CSS turned into — and until this, the answer was
// somewhere inside a process that only ever emitted a picture.
//
// What it prints is a whole document. It goes into render, into push, into the
// HTTP service, or onto a device, and nothing here is a debugging format.
func runCompile(args []string, stdout, stderr io.Writer) int {
	flags := command("compile", stderr)
	output := flags.String("o", "", "write the document here instead of to standard output")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	source := flags.Arg(0)
	if !isPage(source) {
		fmt.Fprintf(stderr, "compile takes a page: %s is already a scene document\n", source)
		return 2
	}
	written, warnings, err := compilePage(source)
	printWarnings(stderr, warnings)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *output == "" {
		fmt.Fprintln(stdout, string(written))
		return 0
	}
	if err := os.WriteFile(*output, append(written, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "write %s: %v\n", *output, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *output, len(written))
	return 0
}
