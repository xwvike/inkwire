# Inkwire Studio

A browser page for writing an e-paper page, seeing exactly what a tag will
show, and writing it to one.

The preview is not an approximation: the renderer is the same Go the command
runs, compiled to wasm, held to identical PNG bytes by `verify/parity.mjs`.

Scratch space, not a workspace. Nothing is saved and a reload starts over.

## Run

```sh
./web/build.sh          # static/inkwire.wasm, wasm_exec.js, panels.json, completions.json
python3 web/serve.py    # http://127.0.0.1:8731/
```

Deployed at <https://xwvike.github.io/inkwire/>, rebuilt from `main` by
`.github/workflows/pages.yml`. Web Bluetooth needs a secure context, so pushing
works over HTTPS and over localhost, nowhere else.

Any static server works. `serve.py` adds `application/wasm` and `no-store`.

## Using it

The bar is three steps; later ones stay disabled until their turn.

**1 · Panel** decides what the page is drawn for — its size, and which inks it
can show. An ink the panel lacks is flattened to black in the preview exactly
as the tag would flatten it, and the report names it.

**2 · Tag** opens the browser's chooser, then connects and checks the tag
serves a protocol this speaks. An EPD-nRF5 tag reports its own model here, so
step 1 is filled in and locked. `Change` reaches a different tag, `Clear` stops
pointing at one.

**3 · Push** writes the page, logging each stage with timings: the block size
the tag asked for, every part, the refresh.

Left pane:

- **HTML** and **CSS** — CodeMirror, completing from this renderer's 89
  properties and their values rather than the browser's several hundred.
  Nothing it offers compiles to a warning.
- **Files** — drop a stylesheet, image or SVG anywhere on the window. A page in
  a browser has no directory beside it, so a file is reached by the name the
  page writes: `assets/photo.png` is called `assets/photo.png` here. Each entry
  says whether the page currently references it.

Right pane:

- **Preview** — the frame at 1×–6×, nearest-neighbour.
- **Layout** — every node and the box it ended up in. The `measure` command.
- **Scene** — what the CSS compiled to. The `compile` command.

Below both: every declaration the renderer could not honour, every glyph no
bundled font could draw, and the push log.

Source that does not parse is not rendered. The last good frame stays up and
the status line says what it is waiting for.

## Limits

- **Chromium only.** Safari and Firefox have no Web Bluetooth; both buttons say
  so rather than failing when pressed.
- **A Gicisky tag's model is only in its advertisement**, and Chrome keeps
  `watchAdvertisements` behind
  `chrome://flags/#enable-experimental-web-platform-features`. With the flag,
  choosing a tag reads the model, selects it, and says so if you then pick
  another; without it, pick the panel from the list.
- **A tag lasts as long as the page.** `getDevices` is behind the same flag, so
  a reload asks again.
- **Go, not TinyGo.** TinyGo builds 4.28 MB against Go's 16.7 MB and then
  panics inside `douceur` on the first page. `build.sh tinygo` still tries it.

## Checks

```sh
go build -o web/verify/inkwire ./cmd/inkwire   # parity.mjs drives it

node web/verify/parity.mjs        # wasm against the command, byte for byte, 93 renders
node web/verify/push.mjs          # a whole upload into a stub tag, both families
node web/verify/completions.mjs   # every completion offered renders without a warning
node web/verify/starters.mjs      # the pages the editor opens with render clean
node web/verify/parsing.mjs       # half-written source is told from finished
node web/verify/calls.mjs         # app.js and worker.js parse, name only files that
                                  # exist, call nothing undefined, and agree on their
                                  # messages and on the transport the module reads
```

A browser is the only thing that runs `app.js` and `worker.js`, and `calls.mjs`
is what stands in for it. The Pages workflow runs all six before deploying.

## Layout

```
web/
  wasm/main.go           render, compile, measure, payload, upload, identify
  tools/panels/          static/panels.json, from the driver catalogues
  tools/completions/     static/completions.json, from MARKUP.md
  static/                index.html, app.js, app.css, worker.js
  static/vendor/         CodeMirror 6, bundled and committed (MIT)
  vendor/                how that bundle is rebuilt, and its licence
  verify/                the checks above
  build.sh
  serve.py
```

Rendering runs in `worker.js`, so typing never waits on it. Pushing runs there
too, with characteristic writes proxied back to the page — that is where a GATT
characteristic exists.

CodeMirror is committed rather than fetched: the editor works offline and no
page load reaches a third-party host. `vendor/build-codemirror.sh` rebuilds it
without leaving a JavaScript toolchain in the repository.
