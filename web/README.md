# Inkwire Studio

An editor for e-paper pages that previews them with the renderer itself.

The reference projects for these tags all ship a page that takes a line of text
and a picture and sends the result over Bluetooth. This is that page with the
renderer behind it: the same Go the CLI runs, compiled to wasm, so the preview
is the frame the tag would be written with rather than a browser's impression
of it.

## Build and run

```sh
./web/build.sh              # writes static/inkwire.wasm, wasm_exec.js, panels.json
cd web/static && python3 -m http.server 8731
```

Then open <http://127.0.0.1:8731/>. Any static file server will do; the page
falls back from `instantiateStreaming` when one does not send
`application/wasm`.

## What it does

| | |
|---|---|
| Editor | CodeMirror 6 — highlighting, undo history, search, bracket matching, multiple cursors |
| Completion | This renderer's 89 properties and their values, generated from `MARKUP.md`, plus the SVG elements it draws |
| Preview | The rendered frame at 1×–6×, nearest-neighbour, on a paper ground — with the panel's own palette, so an ink it cannot show is flattened here exactly as the tag would flatten it |
| Layout | Every node and the box it ended up in — the `measure` command |
| Scene | What the CSS compiled to — the `compile` command |
| Report | Every declaration the renderer could not honour, and every glyph no bundled font could draw |
| Panels | All 28 catalogued models, grouped by family, sized and labelled by palette |
| Files | Drop a stylesheet, image or SVG anywhere on the window and the page can link it, under the name the page writes |

## Verifying it

The preview is only worth having if it is exact, so that claim is checked
rather than asserted:

```sh
node web/verify/parity.mjs       # wasm vs CLI, byte for byte, on every example
node web/verify/completions.mjs  # every completion offered actually renders
node web/verify/starters.mjs     # the pages the editor opens with render clean
node web/verify/calls.mjs        # the page calls nothing that does not exist
```

`parity.mjs` needs the CLI beside it: `go build -o web/verify/inkwire ./cmd/inkwire`.

`calls.mjs` is there because a browser is the only thing that runs `app.js`,
and a ReferenceError in it is silent to everything else — the module keeps
working, the preview keeps drawing, and one tab quietly stops filling in.

At the time of writing every example page produces identical PNG bytes from
the module and from the command — 93 renders, each page as a bare viewport and
again for every catalogued panel of its size — and all 89 properties, 175
values and 13 SVG elements the editor offers render without a warning.

That second check is the point of generating the vocabulary rather than taking
a stock CSS list: a browser has some 500 properties and this renderer has 89,
so a stock completion would spend most of its suggestions proposing
declarations that compile to a warning — teaching the wrong vocabulary at the
moment someone is learning it.

## Known limits

- **No push.** The page renders and downloads; it does not write to a tag. The
  wire payload is computed — the status line says how many bytes the tag would
  be sent — but nothing carries it. Web Bluetooth would, and is Chromium-only,
  so a push button needs a story for Safari and Firefox before it is worth
  having.
- **Go, not TinyGo.** TinyGo produces 4.28 MB against Go's 16.7 MB and then
  panics inside `douceur`'s declaration parser on the first page. `build.sh
  tinygo` still builds it, so the next attempt costs one command.

## Layout

```
web/
  wasm/main.go           the module: render, compile, measure
  tools/panels/          generates static/panels.json from the driver catalogues
  tools/completions/     generates static/completions.json from MARKUP.md
  static/                the page — index.html, app.js, app.css
  static/vendor/         CodeMirror 6, bundled and committed (MIT)
  vendor/                how that bundle is rebuilt, and its licence notice
  verify/                parity, completion and starter checks
  build.sh
```

CodeMirror is committed rather than fetched at load time, so the editor works
offline and no page load reaches a third-party host. Rebuild it with
`web/vendor/build-codemirror.sh`, which installs npm, the packages and esbuild
into a temporary directory and throws all of it away again — the repository
keeps no JavaScript toolchain.
