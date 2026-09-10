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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"syscall/js"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/markup"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/panel"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/tag"
)

func main() {
	api := js.Global().Get("Object").New()
	api.Set("render", js.FuncOf(renderJS))
	api.Set("compile", js.FuncOf(compileJS))
	api.Set("measure", js.FuncOf(measureJS))
	api.Set("identify", js.FuncOf(identifyJS))
	api.Set("identifyNRFEPD", js.FuncOf(identifyNRFEPDJS))
	api.Set("upload", js.FuncOf(uploadJS))
	api.Set("payload", js.FuncOf(payloadJS))
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

// decodedImages is remembered for as long as the module runs. Nothing evicts
// from it: a page is edited with a handful of pictures, and a browser tab that
// has been given hundreds of megabytes of them has been told to.
var decodedImages = map[string]image.Image{}

// preparedImages is the other half, and the larger one. Decoding a picture is
// the smaller cost: profiling it, mapping its tones and choosing how to dither
// it all happen at its own resolution, and none of it depends on the page
// around it. A page is laid out on every keystroke and its pictures are the
// same pictures, so this is remembered too — safely, because the key holds
// each source by identity and the decode cache above is what makes that
// identity stable.
var preparedImages = map[compose.PreparedKey]compose.PreparedImage{}

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
	//
	// The cache is what makes an editor bearable. A page is decoded on every
	// keystroke and its pictures are the same pictures each time; a 3840x2160
	// photograph costs over two seconds to decode, against eight milliseconds
	// for the rest of the page. It is keyed by content, so replacing a file
	// under the same name is a different picture rather than a stale one.
	decoder := scene.Decoder{Resources: files, ResourcesOnly: true, Images: decodedImages}
	decoded, err := decoder.Decode(bytes.NewReader(page.JSON))
	if err != nil {
		return compose.Document{}, warnings, err
	}
	decoded.Prepared = preparedImages
	return decoded, warnings, nil
}

// renderJS draws a page and answers with the picture.
//
// Arguments: markup, css, width, height, resources, and optionally a panel key
// of the form family:id. The size is the viewport, not a suggestion: a page's
// own width and height are CSS layout values, and this is what it has to fit.
// A panel key supersedes the size and brings the panel's palette with it.
func renderJS(this js.Value, args []js.Value) any {
	request, err := readCall(args)
	if err != nil {
		return failure(err, nil)
	}
	named := request.panel != ""
	var bounds image.Point
	if !named {
		if bounds, err = request.size(); err != nil {
			return failure(err, nil)
		}
	}

	decoded, warnings, err := document(request.markup, request.css, request.files)
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
	if named {
		known, err = panel.ByKey(request.panel)
		if err != nil {
			return failure(err, warnings)
		}
		result, page, renderErr = panel.Render(decoded, known)
	} else {
		result, renderErr = scene.RenderForSize(decoded, bounds)
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
		out.Set("panel", describe(known))
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
	request, err := readCall(args)
	if err != nil {
		return failure(err, nil)
	}
	page, err := compilePage(request.markup, request.css, request.files)
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
	request, err := readCall(args)
	if err != nil {
		return failure(err, nil)
	}
	bounds, err := request.size()
	if err != nil {
		return failure(err, nil)
	}

	decoded, warnings, err := document(request.markup, request.css, request.files)
	if err != nil {
		return failure(err, warnings)
	}
	result, renderErr := scene.TraceForSize(decoded, bounds)
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

// identifyNRFEPDJS reads the configuration an EPD-nRF5 tag answers an init
// with, and says which panel is in front of the page.
//
// This family keeps its model in the firmware's own flash rather than in an
// advertisement, so the only way to learn it is to connect, write the init and
// read what comes back — which is why the page could not name the panel before
// and had to ask somebody to pick one. The bytes are byte 7 of epd_config_t
// and the rest of the pin map, and this does the same lookup the session does
// so that the two cannot disagree about what a tag is.
func identifyNRFEPDJS(this js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return failure(errors.New("identifyNRFEPD needs the configuration as bytes"), nil)
	}
	data := make([]byte, args[0].Length())
	js.CopyBytesToGo(data, args[0])

	config, err := nrfepd.ParseConfig(data)
	if err != nil {
		return failure(err, nil)
	}
	out := js.Global().Get("Object").New()
	model, known := config.Model()
	if !known {
		// A tag this build has no entry for is still a tag. Saying which id it
		// reported is what somebody would need to add it.
		out.Set("ok", true)
		out.Set("identified", false)
		out.Set("id", fmt.Sprintf("0x%02X", config.ModelID))
		return out
	}
	found := panel.OfNRFEPD(model)
	out.Set("ok", true)
	out.Set("identified", true)
	out.Set("id", fmt.Sprintf("0x%02X", config.ModelID))
	out.Set("key", found.Family+":"+found.ID())
	out.Set("panel", describe(found))
	return out
}

// identifyJS reads a Gicisky advertisement and says which panel is in front of
// the browser.
//
// This is the difference between this and the pages these tags usually ship
// with. There, the panel is a dropdown and picking the wrong entry is a page
// drawn for hardware that is not there, with nothing to say so. Here the tag
// says what it is — the model is in the manufacturer data, under company
// 0x5053 — and the same table the CLI resolves it against is in this module,
// so the answer is the same answer.
//
// A tag that answers without saying what panel it has is reported as itself
// rather than refused. Nothing can be drawn for it, but "this is a Gicisky tag
// advertising id 0x00C1, which this build has no entry for" is a far better
// thing to be told than nothing, and it is the sentence that gets a model
// added to the table.
func identifyJS(this js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return failure(fmt.Errorf("identify needs the manufacturer data as bytes"), nil)
	}
	data := make([]byte, args[0].Length())
	js.CopyBytesToGo(data, args[0])

	advertised, ok := gicisky.ParseAdvertisement(data)
	out := js.Global().Get("Object").New()
	if !ok {
		out.Set("ok", false)
		out.Set("error", fmt.Sprintf(
			"the manufacturer data is %d bytes; a Gicisky advertisement is %d", len(data), 5))
		return out
	}

	out.Set("ok", true)
	out.Set("id", fmt.Sprintf("0x%04X", advertised.ID))
	out.Set("firmware", fmt.Sprintf("0x%04X", advertised.Firmware))
	// The voltage is the reading; no charge percentage is derived from it,
	// because a coin cell's curve is not linear and a made-up percentage reads
	// as fact.
	out.Set("voltage", advertised.Voltage())

	profile, known := gicisky.LookupProfile(advertised.ID, advertised.Firmware)
	if !known {
		out.Set("identified", false)
		return out
	}
	found := panel.OfGicisky(profile)
	size := found.Size()
	out.Set("identified", true)
	out.Set("key", found.Family+":"+found.ID())
	out.Set("panel", describe(found))
	out.Set("width", size.X)
	out.Set("height", size.Y)
	return out
}

// awaitPromise blocks the calling goroutine until a JS promise settles.
//
// The Go side of an upload is written as straight-line code — write, wait for
// the answer, write again — because that is what the conversation is. The
// browser's side of every one of those writes is a promise. Parking on a
// channel is what lets the two meet: the goroutine yields, the event loop runs
// and settles the promise, and the callback hands the answer back.
//
// This must never be called from a JS callback. On that stack the event loop
// is not running, so the promise cannot settle and the receive is a deadlock.
func awaitPromise(value js.Value) error {
	if value.Type() != js.TypeObject || value.Get("then").Type() != js.TypeFunction {
		return nil // Not a promise: the write was synchronous and has happened.
	}
	settled := make(chan error, 1)
	onDone := js.FuncOf(func(this js.Value, args []js.Value) any {
		settled <- nil
		return nil
	})
	defer onDone.Release()
	onFail := js.FuncOf(func(this js.Value, args []js.Value) any {
		message := "the browser refused the write"
		if len(args) > 0 && args[0].Type() == js.TypeObject {
			if text := args[0].Get("message"); text.Type() == js.TypeString {
				message = text.String()
			}
		}
		settled <- errors.New(message)
		return nil
	})
	defer onFail.Release()
	value.Call("then", onDone).Call("catch", onFail)
	return <-settled
}

// browserTransport is gicisky.Transport backed by two GATT characteristics.
//
// It is the whole of what the browser adds. Everything about the protocol —
// the stages, the block size the tag asks for, the acknowledgements, the
// timeouts — stays in internal/gicisky, which is the code the command has been
// driving real tags with. Reimplementing that in JavaScript is what every
// other browser tool for these tags does, and it is where their bugs are.
type browserTransport struct {
	control       js.Value
	data          js.Value
	notifications chan []byte
}

func (t *browserTransport) Notifications() <-chan []byte { return t.notifications }

func (t *browserTransport) WriteControl(payload []byte) error {
	return t.write(t.control, payload)
}

func (t *browserTransport) WriteData(payload []byte) error {
	return t.write(t.data, payload)
}

func (t *browserTransport) write(fn js.Value, payload []byte) error {
	if fn.Type() != js.TypeFunction {
		return errors.New("the page did not supply this write")
	}
	buffer := js.Global().Get("Uint8Array").New(len(payload))
	js.CopyBytesToJS(buffer, payload)
	return awaitPromise(fn.Invoke(buffer))
}

// nrfTransport is nrfepd's session transport over one browser characteristic.
//
// This family writes and listens on the same characteristic, so there is only
// one write here where Gicisky has two.
type nrfTransport struct {
	write         js.Value
	notifications chan []byte
}

func (t *nrfTransport) Notifications() <-chan []byte { return t.notifications }

func (t *nrfTransport) Write(frame []byte) error {
	if t.write.Type() != js.TypeFunction {
		return errors.New("the page did not supply a write")
	}
	buffer := js.Global().Get("Uint8Array").New(len(frame))
	js.CopyBytesToJS(buffer, frame)
	return awaitPromise(t.write.Invoke(buffer))
}

// uploadNRFEPD writes a page to an EPD-nRF5 tag.
//
// The panel is not named by the caller and cannot be: this family keeps its
// model in the firmware's own flash rather than in an advertisement, so it is
// learned partway through the conversation. That is what PageFor is for — the
// session asks for the page once the tag has said what shape it needs, and
// this draws it then. A caller that had to choose the panel first would be
// guessing, and a page built for the wrong size does not come out looking
// wrong: it fills the panel with bytes that mean something else.
func uploadNRFEPD(request call, wiring js.Value, warnings []compose.Warning) any {
	transport := &nrfTransport{
		write: wiring.Get("write"),
		// Buffered because the tag answers while the session is still deciding
		// to listen, and a notification dropped here stalls the conversation
		// for the full response timeout.
		notifications: make(chan []byte, 8),
	}

	logf := func(format string, values ...any) {}
	if report := wiring.Get("log"); report.Type() == js.TypeFunction {
		logf = func(format string, values ...any) {
			report.Invoke(fmt.Sprintf(format, values...))
		}
	}

	// Filled in when the tag says what it is, which is also when the page can
	// be drawn. Read only after the session finishes, on the same goroutine.
	var drawn int
	page := func(model nrfepd.Model) (black, colour []byte, err error) {
		known := panel.OfNRFEPD(model)
		logf("the tag says it is %s", describe(known))
		decoded, pageWarnings, err := document(request.markup, request.css, request.files)
		if err != nil {
			return nil, nil, err
		}
		result, packed, err := panel.Render(decoded, known)
		if err != nil {
			return nil, nil, err
		}
		// The page is drawn inside the conversation, so anything it lost has
		// no answer to be attached to. It is logged instead, which is where
		// the rest of this conversation is going.
		for _, warning := range append(pageWarnings, result.Report.Warnings...) {
			logf("%s: %s", warning.Code, warning.Message)
		}
		drawn = packed.Len()
		return packed.Black, packed.Colour, nil
	}

	settle := js.Global().Get("Object").New()
	var resolve, reject js.Value
	promise := js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve, reject = args[0], args[1]
		return nil
	}))

	notify := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 || args[0].Type() != js.TypeObject {
			return nil
		}
		value := make([]byte, args[0].Length())
		js.CopyBytesToGo(value, args[0])
		select {
		case transport.notifications <- value:
		default:
			logf("dropped a notification: %x", value)
		}
		return nil
	})

	go func() {
		defer notify.Release()
		settle := nrfepd.DefaultSettle
		if request.settleMs > 0 {
			settle = time.Duration(request.settleMs) * time.Millisecond
		}
		timings := nrfepd.Timings{Response: nrfepd.DefaultResponseTimeout, Settle: settle}
		if err := nrfepd.Session(context.Background(), transport, page, timings, logf); err != nil {
			reject.Invoke(js.Global().Get("Error").New(err.Error()))
			return
		}
		resolve.Invoke(js.ValueOf(drawn))
	}()

	settle.Set("ok", true)
	settle.Set("notify", notify)
	settle.Set("done", promise)
	settle.Set("family", "nrfepd")
	settle.Set("warnings", warningsJS(warnings))
	// Unlike Gicisky there is no size to report yet: the tag has not been asked
	// what it is. It says so in the log when it answers.
	return settle
}

// uploadJS draws a page for a panel and writes it to a tag the page has
// already connected to.
//
// Arguments: markup, css, resources, panel key, and an object carrying
// writeControl, writeData and an optional log. It answers with an object
// holding a promise that settles when the tag has taken the page, and a notify
// the page calls with every value the control characteristic reports.
//
// The connection is the page's because only the page can make one: a GATT
// server is reached through a device the user granted in a dialog this module
// cannot open. What the module keeps is the part that has been tested.
func uploadJS(this js.Value, args []js.Value) any {
	request, err := readCall(args)
	if err != nil {
		return failure(err, nil)
	}
	wiring := request.transport
	if wiring.Type() != js.TypeObject {
		return failure(errors.New("upload needs a transport"), nil)
	}

	// The two families do not agree on when the panel is known, so they do not
	// agree on what an upload needs. A Gicisky tag advertises its model, so the
	// page names one and this draws for it; an EPD-nRF5 tag keeps it in
	// firmware, so nothing can be named and the session asks for the page once
	// the tag has answered. The page says which service it found.
	if request.family == tag.NRFEPD {
		return uploadNRFEPD(request, wiring, nil)
	}

	known, err := panel.ByKey(request.panel)
	if err != nil {
		return failure(err, nil)
	}
	if known.Family != tag.Gicisky {
		return failure(fmt.Errorf("%s is not a Gicisky panel", known), nil)
	}

	decoded, warnings, err := document(request.markup, request.css, request.files)
	if err != nil {
		return failure(err, warnings)
	}
	result, page, err := panel.Render(decoded, known)
	warnings = append(warnings, result.Report.Warnings...)
	if err != nil {
		return failure(err, warnings)
	}
	if len(page.Bytes) == 0 {
		return failure(errors.New("the page packed to nothing"), warnings)
	}

	transport := &browserTransport{
		control: wiring.Get("writeControl"),
		data:    wiring.Get("writeData"),
		// Buffered because the tag answers while the uploader is still
		// deciding to listen, and a notification dropped here stalls the
		// conversation for the full response timeout.
		notifications: make(chan []byte, 8),
	}

	logf := func(format string, values ...any) {}
	if report := wiring.Get("log"); report.Type() == js.TypeFunction {
		logf = func(format string, values ...any) {
			report.Invoke(fmt.Sprintf(format, values...))
		}
	}

	settle := js.Global().Get("Object").New()
	var resolve, reject js.Value
	promise := js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve, reject = args[0], args[1]
		return nil
	}))

	notify := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 || args[0].Type() != js.TypeObject {
			return nil
		}
		value := make([]byte, args[0].Length())
		js.CopyBytesToGo(value, args[0])
		select {
		case transport.notifications <- value:
		default:
			// The uploader is not reading and the buffer is full. Dropping is
			// better than blocking a JS callback, which would stop the event
			// loop the uploader is waiting on.
			logf("dropped a notification: %x", value)
		}
		return nil
	})

	// The upload runs on its own goroutine so that every await inside it yields
	// to the event loop rather than to this call's caller.
	go func() {
		defer notify.Release()
		uploader := gicisky.NewUploader(logf)
		err := uploader.UploadWithOptions(
			context.Background(), transport, page.Bytes, known.Gicisky.Upload())
		if err != nil {
			reject.Invoke(js.Global().Get("Error").New(err.Error()))
			return
		}
		resolve.Invoke(js.ValueOf(len(page.Bytes)))
	}()

	settle.Set("ok", true)
	settle.Set("notify", notify)
	settle.Set("done", promise)
	settle.Set("payloadBytes", len(page.Bytes))
	settle.Set("panel", describe(known))
	settle.Set("warnings", warningsJS(warnings))
	return settle
}

// payloadJS answers with the bytes a tag would be sent, without sending them.
//
// It is what the uploader is handed, so it is the thing to compare an upload
// against: web/verify/push.mjs drives a complete upload into a stub tag and
// checks that what arrived is this. Without that, the only test of the push
// path was a tag on someone's desk, and the first bug it had — the resource
// map read from the wrong argument, so every picture was missing — looked from
// the outside like a successful upload of a blank page.
//
// It is also worth having on its own. A payload that can be saved is a payload
// that can be compared against another tool's, which is how a protocol
// disagreement gets found.
func payloadJS(this js.Value, args []js.Value) any {
	request, err := readCall(args)
	if err != nil {
		return failure(err, nil)
	}
	known, err := panel.ByKey(request.panel)
	if err != nil {
		return failure(err, nil)
	}
	decoded, warnings, err := document(request.markup, request.css, request.files)
	if err != nil {
		return failure(err, warnings)
	}
	result, page, err := panel.Render(decoded, known)
	warnings = append(warnings, result.Report.Warnings...)
	if err != nil {
		return failure(err, warnings)
	}

	out := js.Global().Get("Object").New()
	out.Set("ok", true)
	out.Set("panel", describe(known))
	out.Set("warnings", warningsJS(warnings))
	// Gicisky takes one buffer; EPD-nRF5 takes a black plane and, on a colour
	// panel, a second. Which fields are set follows the family, the same way
	// panel.Page sets them.
	if len(page.Bytes) > 0 {
		out.Set("bytes", base64.StdEncoding.EncodeToString(page.Bytes))
	}
	if len(page.Black) > 0 {
		out.Set("black", base64.StdEncoding.EncodeToString(page.Black))
	}
	if len(page.Colour) > 0 {
		out.Set("colour", base64.StdEncoding.EncodeToString(page.Colour))
	}
	return out
}

// call is one request from the page, read by name.
//
// It was positional, and the positions did not agree: render took a size before
// the resource map and upload took a panel, so the same index meant different
// things in different entry points. A shared reader hard-coded one of those
// positions, upload's transport was read as its files, and every page it sent
// went out with none of its pictures — an upload that succeeded in every
// visible way and arrived blank.
//
// Compile made the shape of the mistake plain before it happened: it was being
// given two zeroes it did not use, so that its files landed in the slot the
// reader expected. Nothing here has a position now.
type call struct {
	markup, css   string
	files         map[string][]byte
	panel         string
	family        string
	width, height int
	// settleMs is how long to wait for an EPD-nRF5 panel to finish drawing,
	// which the command exposes as -settle for the same reason: the default is
	// thirty seconds and there is no way to know it from here. Zero means the
	// default; the command's "no wait at all" is not reachable from a page,
	// because a page that returned before the tag stopped drawing would invite
	// a second push into the middle of the first.
	settleMs  int
	transport js.Value
}

func readCall(args []js.Value) (call, error) {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return call{}, errors.New("expected one object naming markup, css and whatever else the call needs")
	}
	fields := args[0]
	return call{
		markup:    text(fields, "markup"),
		css:       text(fields, "css"),
		panel:     text(fields, "panel"),
		family:    text(fields, "family"),
		settleMs:  number(fields, "settleMs"),
		files:     resources(fields.Get("files")),
		width:     number(fields, "width"),
		height:    number(fields, "height"),
		transport: fields.Get("transport"),
	}, nil
}

func text(fields js.Value, name string) string {
	if value := fields.Get(name); value.Type() == js.TypeString {
		return value.String()
	}
	return ""
}

func number(fields js.Value, name string) int {
	if value := fields.Get(name); value.Type() == js.TypeNumber {
		return value.Int()
	}
	return 0
}

// size is the viewport a call asks to be laid out for.
func (c call) size() (image.Point, error) {
	if c.width <= 0 || c.height <= 0 {
		return image.Point{}, fmt.Errorf("width and height must be positive, got %dx%d", c.width, c.height)
	}
	return image.Pt(c.width, c.height), nil
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

// describe names a panel for the page.
//
// It is deliberately not panel.String(), which ends with "(unverified)" when
// nobody has checked the catalogue entry against real hardware. That is a fact
// about this project rather than about the tag — the firmware is the same
// either way — so it belongs in the command's output, where the reader is
// diagnosing, and not in front of someone choosing a panel to draw for.
func describe(p panel.Panel) string {
	size := p.Size()
	palette := p.Gicisky.Palette.String()
	if p.Family == tag.NRFEPD {
		palette = p.NRFEPD.Palette.String()
	}
	return fmt.Sprintf("%s %dx%d %s", p.Name(), size.X, size.Y, palette)
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
