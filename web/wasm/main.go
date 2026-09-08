//go:build js && wasm

// Command inkwire-wasm exposes the renderer to a browser.
//
// It is the same pipeline the CLI runs — markup compiles the page, the scene
// decoder reads what it wrote, and the layout draws it — with the two things a
// browser cannot have taken out: there is no filesystem, so a page reaches its
// stylesheets and pictures through a map the caller supplies, and there is no
// radio, so the answer is a picture rather than a tag that has been written to.
//
// Everything it returns is what the CLI would have printed. A page that loses
// a declaration renders anyway and says what it lost, because the picture is
// what shows the author what went missing.
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"syscall/js"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/markup"
	"github.com/xwvike/inkwire/internal/panel"
	"github.com/xwvike/inkwire/internal/scene"
)

func main() {
	api := js.Global().Get("Object").New()
	api.Set("render", js.FuncOf(renderJS))
	api.Set("compile", js.FuncOf(compileJS))
	api.Set("measure", js.FuncOf(measureJS))
	js.Global().Set("inkwire", api)

	// A page that loads the module has to know when it may call it. The
	// callback is set before the ready flag, so anything that sees ready can
	// already render.
	if ready := js.Global().Get("inkwireReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

// resources reads the optional name-to-bytes map a caller passes as the last
// argument. A page in a browser has no directory beside it, so this map is the
// whole of what its links and pictures can reach.
func resources(value js.Value) map[string][]byte {
	if value.Type() != js.TypeObject {
		return nil
	}
	names := js.Global().Get("Object").Call("keys", value)
	out := make(map[string][]byte, names.Length())
	for i := 0; i < names.Length(); i++ {
		name := names.Index(i).String()
		data := value.Get(name)
		if data.Type() != js.TypeObject {
			continue
		}
		buffer := make([]byte, data.Length())
		js.CopyBytesToGo(buffer, data)
		out[name] = buffer
	}
	return out
}

// compilePage runs the front end over one page and its stylesheet.
//
// The sibling stylesheet the CLI reads from disk is the second argument here.
// It is named so that a page which also links it gets it once, where the link
// puts it, which is the rule the cascade is built on.
func compilePage(markupSource, cssSource string, files map[string][]byte) (markup.Document, error) {
	read := func(name string) ([]byte, error) {
		if data, ok := files[name]; ok {
			return data, nil
		}
		return nil, fmt.Errorf("%s: no file of that name was given to this page", name)
	}
	compiler := markup.Compiler{
		Stylesheets:    read,
		Drawings:       read,
		StylesheetName: "page.css",
	}
	return compiler.Compile(markupSource, cssSource)
}

// document turns a page into the scene document the layout takes, carrying the
// front end's warnings out beside the decoder's own.
func document(markupSource, cssSource string, files map[string][]byte) (compose.Document, []compose.Warning, error) {
	page, err := compilePage(markupSource, cssSource, files)
	warnings := make([]compose.Warning, 0, len(page.Warnings))
	for _, warning := range page.Warnings {
		warnings = append(warnings, compose.Warning(warning))
	}
	if err != nil {
		return compose.Document{}, warnings, err
	}
	// ResourcesOnly is what makes this safe to run on someone else's page:
	// a picture may come from the map, an HTTP URL or a data URL, and from
	// nowhere else. There is no filesystem to reach in the first place, and
	// saying so here means a path that tries reports rather than resolving.
	decoder := scene.Decoder{Resources: files, ResourcesOnly: true}
	decoded, err := decoder.Decode(bytes.NewReader(page.JSON))
	if err != nil {
		return compose.Document{}, warnings, err
	}
	return decoded, warnings, nil
}

// renderJS draws a page and answers with the picture.
//
// Arguments: markup, css, width, height, resources, and optionally a panel key
// of the form family:id. The size is the viewport, not a suggestion: a page's
// own width and height are CSS layout values, and this is what it has to fit.
// A panel key supersedes the size and brings the panel's palette with it.
func renderJS(this js.Value, args []js.Value) any {
	markupSource, cssSource, files, err := arguments(args)
	if err != nil {
		return failure(err, nil)
	}
	if len(args) < 4 {
		return failure(fmt.Errorf("render needs markup, css, width and height"), nil)
	}
	width, height := args[2].Int(), args[3].Int()
	if width <= 0 || height <= 0 {
		return failure(fmt.Errorf("render size must be positive, got %dx%d", width, height), nil)
	}

	decoded, warnings, err := document(markupSource, cssSource, files)
	if err != nil {
		return failure(err, warnings)
	}

	// Naming a panel asks for that panel's picture rather than a picture of
	// that size: the inks it cannot show are flattened, and what was flattened
	// is reported. Without one this is a plain viewport, which is what a custom
	// size means — there is no panel whose palette it could be checked against.
	var result scene.Result
	var renderErr error
	var page panel.Page
	var known panel.Panel
	named := len(args) >= 6 && args[5].Type() == js.TypeString && args[5].String() != ""
	if named {
		known, err = panel.ByKey(args[5].String())
		if err != nil {
			return failure(err, warnings)
		}
		result, page, renderErr = panel.Render(decoded, known)
	} else {
		result, renderErr = scene.RenderForSize(decoded, image.Pt(width, height))
	}
	warnings = append(warnings, result.Report.Warnings...)
	if result.Frame == nil {
		return failure(renderErr, warnings)
	}

	var encoded bytes.Buffer
	if err := display.WritePNG(&encoded, result.Frame); err != nil {
		return failure(err, warnings)
	}
	out := js.Global().Get("Object").New()
	out.Set("ok", true)
	if named {
		out.Set("panel", known.String())
		out.Set("flattened", flattenedJS(page.Flattened))
		// What the tag would be sent. Nothing here writes it, but a page that
		// will not fit is worth knowing about before the wire is involved.
		out.Set("payloadBytes", page.Len())
	}
	out.Set("png", base64.StdEncoding.EncodeToString(encoded.Bytes()))
	out.Set("width", result.Frame.Width())
	out.Set("height", result.Frame.Height())
	out.Set("warnings", warningsJS(warnings))
	out.Set("missingRunes", runesJS(result.Report.MissingRunes))
	// The picture goes out with the refusal. A page that could not be laid out
	// for this size has still been drawn, and what it looks like is what says
	// which part of it has to change.
	if renderErr != nil {
		out.Set("ok", false)
		out.Set("error", renderErr.Error())
	}
	return out
}

// compileJS answers with the scene document a page compiles to.
//
// It is the one call that stops in the middle, and it is here for the same
// reason the CLI has it: a stylesheet says what a page is and leaves the
// arithmetic to the layout, so when a box lands in the wrong place the question
// is what the CSS turned into.
func compileJS(this js.Value, args []js.Value) any {
	markupSource, cssSource, files, err := arguments(args)
	if err != nil {
		return failure(err, nil)
	}
	page, err := compilePage(markupSource, cssSource, files)
	warnings := make([]compose.Warning, 0, len(page.Warnings))
	for _, warning := range page.Warnings {
		warnings = append(warnings, compose.Warning(warning))
	}
	if err != nil {
		return failure(err, warnings)
	}
	out := js.Global().Get("Object").New()
	out.Set("ok", true)
	out.Set("json", string(page.JSON))
	out.Set("warnings", warningsJS(warnings))
	return out
}

// measureJS answers with the box every node ended up in.
//
// This is the CLI's measure command: it traces the layout, so a node that is
// not where its author expected can be read off rather than guessed at from
// the picture.
func measureJS(this js.Value, args []js.Value) any {
	markupSource, cssSource, files, err := arguments(args)
	if err != nil {
		return failure(err, nil)
	}
	if len(args) < 4 {
		return failure(fmt.Errorf("measure needs markup, css, width and height"), nil)
	}
	width, height := args[2].Int(), args[3].Int()
	if width <= 0 || height <= 0 {
		return failure(fmt.Errorf("measure size must be positive, got %dx%d", width, height), nil)
	}

	decoded, warnings, err := document(markupSource, cssSource, files)
	if err != nil {
		return failure(err, warnings)
	}
	result, renderErr := scene.TraceForSize(decoded, image.Pt(width, height))
	warnings = append(warnings, result.Report.Warnings...)
	if renderErr != nil && result.Frame == nil {
		return failure(renderErr, warnings)
	}

	nodes := js.Global().Get("Array").New()
	for _, placement := range result.Report.Placements {
		node := js.Global().Get("Object").New()
		node.Set("path", placement.Path)
		node.Set("type", placement.Type)
		node.Set("x", placement.Bounds.Min.X)
		node.Set("y", placement.Bounds.Min.Y)
		node.Set("width", placement.Bounds.Dx())
		node.Set("height", placement.Bounds.Dy())
		nodes.Call("push", node)
	}
	out := js.Global().Get("Object").New()
	out.Set("ok", renderErr == nil)
	out.Set("nodes", nodes)
	out.Set("warnings", warningsJS(warnings))
	if renderErr != nil {
		out.Set("error", renderErr.Error())
	}
	return out
}

// arguments reads the three every call shares.
func arguments(args []js.Value) (markupSource, cssSource string, files map[string][]byte, err error) {
	if len(args) < 2 {
		return "", "", nil, fmt.Errorf("expected markup and css")
	}
	files = nil
	if len(args) >= 5 {
		files = resources(args[4])
	}
	return args[0].String(), args[1].String(), files, nil
}

func warningsJS(warnings []compose.Warning) js.Value {
	array := js.Global().Get("Array").New()
	for _, warning := range warnings {
		item := js.Global().Get("Object").New()
		item.Set("path", warning.Path)
		item.Set("code", warning.Code)
		item.Set("message", warning.Message)
		array.Call("push", item)
	}
	return array
}

// flattenedJS reports the inks the panel could not show, which were drawn
// black to make this page. A page designed in red on a black-and-white tag
// still renders, and this is how it says what it lost.
func flattenedJS(inks []display.Ink) js.Value {
	array := js.Global().Get("Array").New()
	for _, ink := range inks {
		array.Call("push", ink.String())
	}
	return array
}

// runesJS reports the characters no bundled font could draw. A page whose text
// came out blank has its reason here rather than in the picture.
func runesJS(runes []rune) js.Value {
	array := js.Global().Get("Array").New()
	for _, r := range runes {
		array.Call("push", string(r))
	}
	return array
}

func failure(err error, warnings []compose.Warning) js.Value {
	out := js.Global().Get("Object").New()
	out.Set("ok", false)
	if err != nil {
		out.Set("error", err.Error())
	}
	out.Set("warnings", warningsJS(warnings))
	return out
}
