# Inkwire

A JSON Schema driven renderer for e-paper tags.

| | Gicisky | EPD-nRF5 |
|---|---|---|
| Firmware | factory | [replacement](https://github.com/tsl0922/EPD-nRF5), nRF51/nRF52 |
| Name | `PICKSMART`, `NEMR<mac8>` | `NRF_EPD_<mac4>` |
| Model from | advertisement | connection |
| Size | 212x104 … 960x640 | 400x300 … 880x528 |
| Inks | BW, BWR, BWRY | BW, BWR |

The document these commands read is the Scene Schema: **[SCHEMA.md](SCHEMA.md)**,
which the binary also carries — `inkwire schema` prints it with no network and no
second download.

## CLI

| Command | Purpose |
|---|---|
| `inkwire scan [-timeout 15s]` | List the tags a scan can currently see |
| `inkwire render [-o out.png] [-size WxH \| -panel FAMILY:ID] <page.html\|scene.json>` | PNG preview |
| `inkwire compile [-o scene.json] <page.html>` | Print the scene document a page compiles to |
| `inkwire measure [-size WxH \| -panel FAMILY:ID] [-json] <page.html\|scene.json>` | Print where every node ended up |
| `inkwire push -device NAME [-family gicisky\|nrfepd] [-settle 30s] <page.html\|scene.json>` | Render and write |
| `inkwire mode -device NAME [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 clock / calendar mode |
| `inkwire serve [-listen ADDR] [-assets DIR]` | Start the HTTP service |
| `inkwire schema [-lang en\|zh]` | Print the JSON Schema reference |
| `inkwire help` | `-h`, `--help` |
| `inkwire version` | `-v`, `--version` |

### scan

Lists the tags a scan can currently see in the area. When the target is not
known for certain, scan first and take the name or address `-device` wants.

```bash
inkwire scan -timeout 10s
```

```
ADDRESS                                NAME           RSSI  BATT  FAMILY   MODEL           SIZE      PALETTE
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 2.9" BWR    296x128   BWR
C1:57:DD:3F:C1:F8                      NRF_EPD_C1F8    -62     -  nrfepd   ask on connect            not advertised
```

| Flag | Default | Meaning |
|---|---|---|
| `-timeout` | `15s` | How long the scan window lasts |

A scan tells the two families apart: Gicisky by its `0x5053` manufacturer
data, EPD-nRF5 by its service UUID or name prefix.

Exits 1 when the scan comes back empty.

### render

Renders a scene to a PNG, to preview the result.

```bash
inkwire render -o preview.png page.json
```

```
wrote preview.png (296x128)
```

| Flag | Default | Meaning |
|---|---|---|
| `-o` | the scene's path with `.png` | PNG output path |
| `-size` | the scene's own `size` | Lay the scene out at `WxH` instead |
| `-panel` | unset | Lay the scene out for a named panel and check its inks |


```
$ inkwire render page.json
this scene states no size: give the document a size, or render with -size WxH or -panel family:id
```

`-size` says it without consulting any panel table:

```bash
inkwire render -size 400x300 page.json
```

`-panel` takes the size and the available inks from the named panel.

```bash
inkwire render -panel gicisky:0x0033 page.json
inkwire render -panel nrfepd:UC8176_420_BWR page.json
```

```
wrote page.png (296x128)
wrote page.png (400x300)
```

Anything the panel cannot show is reported:

```bash
inkwire render -panel gicisky:0x0028 page.json
```

```
wrote page.png (296x128)
BW panel cannot show red ink at (10,10)
```

The preview is still written when the panel refuses the page; the exit code
says why it was refused. A misspelled panel or size exits 2; a scene that will
not lay out exits 1.

`-size` and `-panel` are two ways of saying the same thing, so giving both is
refused.

The page-level fields are documented in full under "Page" in
[SCHEMA.md](SCHEMA.md).

### measure

Prints the box every node was given, and what the ones with an opinion would
rather have had. Nothing is rendered.

```bash
inkwire measure page.json
```

```
row         0,0    120x40
    text    0,11    40x17   wants 56x17
    text   44,11    76x17   wants 35x17

warning root.children[0] [text-clipped]: "LAST REF" does not fit 40x17: 16 pixels along the line cut off
```

| Flag | Default | Meaning |
|---|---|---|
| `-size` | the scene's own `size` | Lay the scene out at `WxH` instead |
| `-panel` | unset | Lay the scene out for a named panel |
| `-json` | off | Write the placements as JSON instead of a tree |

`wants` is the size the node asked for and appears only where that differs from
the box it got. It is the answer to how wide a piece of text is, which otherwise
took a render, a warning and a bisection to find out.

A box smaller than what a node wants is not a fault on its own: a label is
normally sized to its letters rather than to the descender and line gap under
them. `text-clipped` is what says the difference cost something.

### push

Renders a scene and writes it to a tag.

```bash
inkwire push -device NEMR92943861 page.json
```

```
writing to NEMR92943861 (e2ada7d1-187a-caea-21e4-d895f8240b62, gicisky)
pushing 9472 bytes (EPD 2.9" BWR 296x128 BWR)
tag requested 244 byte messages -> 240 byte blocks
upload complete, tag is refreshing
```

| Flag | Default | Meaning |
|---|---|---|
| `-device` | required | Advertised name (`NAME` column) or BLE address |
| `-family` | `auto` | Given, it asserts the device belongs to that family instead of working it out |
| `-settle` | `30s` | EPD-nRF5 only: how long to stay connected after `REFRESH`; `0` leaves at once, see [Between writes](#between-writes) |

A scene stating a size the panel does not have is not refused: it is laid out
again for the panel and a `size-mismatch` warning is reported. An ink the panel
has no plane for is not refused either: it is drawn black and an
`unsupported-ink` warning is reported, one per ink.

Warnings from the render are printed with the write log, see [Warnings](#warnings).

### mode

Hands an EPD-nRF5 tag back to its own clock or calendar, and sets that clock on
the way.

```bash
inkwire mode -device NRF_EPD_C1F8 -mode clock
```

```
connecting to NRF_EPD_C1F8 (C1:57:DD:3F:C1:F8)
firmware version 0x76
panel is UC8176_420_BWR 400x300 BWR
setting the clock to 2026-08-21 16:04:28 +0800 and the mode to clock
staying connected 30s while the panel refreshes; disconnecting now would cancel it
```

| Flag | Default | Meaning |
|---|---|---|
| `-device` | required | Advertised name or BLE address |
| `-mode` | `calendar` | `picture`, `calendar` or `clock` |
| `-week-start` | the tag's own setting | First column of a calendar week: `sunday` or `monday` |
| `-settle` | `30s` | How long to stay connected while the panel redraws; `0` leaves at once |

Only EPD-nRF5 has this. Pointed at a Gicisky tag it is refused by name:

```
NEMR92943861 (e2ada7d1-…, gicisky) is a gicisky tag, but the family asked for is nrfepd
```

`push` leaves a tag in picture mode, so this is the way back; see [Clock and
calendar](#clock-and-calendar).

### serve

Starts the HTTP service, for the callers that are not a command line.

```bash
inkwire serve -listen 127.0.0.1:8080
```

```
listening on http://127.0.0.1:8080
```

| Flag | Default | Meaning |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | Listen address, loopback only |
| `-assets` | `.` | Directory a relative image source may read from |

There is no authentication and every request reaches hardware, so only a
loopback address is accepted and anything else is refused. Routes and error
codes are under [Running the HTTP service](#running-the-http-service).

### schema

Prints the JSON Schema reference. It ships inside the binary.

```bash
inkwire schema -lang zh > SCHEMA.zh-CN.md
```

| Flag | Default | Meaning |
|---|---|---|
| `-lang` | `en` | Which translation to print: `en` or `zh` |



## Gicisky

| | |
|---|---|
| Panel | Detected from advertisement; 212x104 … 960x640, BW/BWR/BWRY |
| Orientation | landscape uses the profile size; portrait swaps width and height |
| Payload | model-specific planes, transforms and compression |
| GATT | service `FEF0`, control `FEF1`, data `FEF2` |

Manufacturer data, company `0x5053`:

```
byte   0        1        2   3        4
       id low   battery  firmware     id high
```

Model id = `(data[4] << 8) | data[0]`, low 14 bits. Names carry no model, so
writing a Gicisky tag requires seeing manufacturer data before rendering.

### Models

Read from the advertisement, with no connection needed.

| ID | Name | Size | Inks | Transform and packing |
|---|---|---|---|---|
| `0x000B` | EPD 2.1" BWR | 212x104 | BWR | rotate 270°, mirror X |
| `0x0028` | EPD 2.9" BW | 296x128 | BW | rotate 90° |
| `0x002E` | EPD 2.9" BWRY | 296x128 | BWRY | rotate 90°, four colours two bits each |
| **`0x0033`** | **EPD 2.9" BWR** | **296x128** | **BWR** | rotate 90° |
| `0x004B` | EPD 4.2" BWR | 400x300 | BWR | |
| `0x004E` | EPD 4.2" BWRY | 400x300 | BWRY | four colours two bits each |
| `0x008B` | EPD 10.2" BWR | 960x640 | BWR | QuickLZ |
| `0x00A0` | TFT 2.1" BW | 250x132 | BW | TFT, rotate 90°, mirror X |
| `0x010B` | EPD 2.1" BWR | 250x128 | BWR | rotate 270°, mirror X |
| `0x012B` | EPD 7.5" BWR | 800x480 | BWR | mirror Y, invert, QuickLZ |
| `0x022B` | EPD 3.7" BWR | 240x416 | BWR | rotate 180°, mirror X, column compression |

`0x012B` on firmware `0x8101` uses column compression instead.

The table comes from [hass-gicisky](https://github.com/eigger/hass-gicisky)
(MIT, © 2025 eigger). Only `0x0033` has been held up against hardware; the rest
are marked `unverified` by `inkwire scan`:

```
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 4.2" BWR    400x300   BWR (unverified)
```

An id that is not in the table is still listed, marked `unrecognised model`, and
is not driven.

## EPD-nRF5

| | |
|---|---|
| GATT | service `62750001-d828-918d-fb46-b6c11c675aec`, rw+notify `…0002`, version `…0003` |
| Commands | `INIT=0x01` `REFRESH=0x05` `WRITE_IMAGE=0x30` `SET_TIME=0x20` |
| Planes | black + colour, row major, MSB first, set bit = white, colour inverted |
| Compression | firmware RLE; blank 400x300 plane 15000 → 232 bytes |


### Models

Read after connecting with `INIT`. A size mismatch is not refused: the page is laid out again for the panel and a `size-mismatch` warning is reported. An ink the panel has no plane for is not refused — yellow on a BWR panel, red on a BW one — it is drawn black with an `unsupported-ink` warning, the same as Gicisky.

| ID | Name | Size | Inks | Packing |
|---|---|---|---|---|
| `0x01` | UC8176_420_BW | 400x300 | BW | planes |
| `0x02` | SSD1619_420_BWR | 400x300 | BWR | planes |
| **`0x03`** | **UC8176_420_BWR** | **400x300** | **BWR** | planes |
| `0x04` | SSD1619_420_BW | 400x300 | BW | planes |
| `0x06` | UC8179_750_BW | 800x480 | BW | planes |
| `0x07` | UC8179_750_BWR | 800x480 | BWR | planes |
| `0x0a` | SSD1677_750_HD_BW | 880x528 | BW | planes |
| `0x0b` | SSD1677_750_HD_BWR | 880x528 | BWR | planes |
| `0x10` | UC8179_583_BWR | 648x480 | BWR | planes |
| `0x11` | UC8179_583_BW | 648x480 | BW | planes |
| `0x08` `0x09` `0x0e` `0x0f` | UC8159 | 640x384 / 600x448 | BW/BWR | nibbles, **unimplemented** |
| `0x05` `0x0c` `0x0d` | JD796xx | 400x300 / 800x480 / 648x480 | BWRY | **unimplemented** |

`0x03` verified; others print `unverified`:

```
panel is UC8179_750_BWR 800x480 BWR (unverified), link carries 244 bytes and can decompress
```

Unimplemented packings are refused, not attempted.

### Clock and calendar

| Mode | Redraw |
|---|---|
| `picture` | none |
| `calendar` | daily at `00:00:00` |
| `clock` | every minute |

`push` resets the tag to `picture`. `mode` restores it and sets the clock:

```bash
inkwire mode -device NRF_EPD_C1F8 -mode clock
```

`-week-start` unset leaves the tag's own value.

## The Bluetooth link


### Signal and distance

RSSI belongs to the whole link — the tag's transmitter, both antennas, the path
and the receiver — so the table below is **one adapter's** situation.

Adapter under test: a Realtek RTL8761BU dongle (OS: Debian, USB `0bda:8771`, Bluetooth
5.1, integrated antenna, USB 2.0 full speed), tags standing upright with a clear
line of sight:

| Distance | Gicisky | EPD-nRF5 | Pushes |
|---|---|---|---|
| 1 m | −66 dBm | −68 dBm | 4/4 |
| 2 m | −74 dBm | −78 dBm | 2/2 |
| 3 m | −76 dBm | −80 dBm | 2/2 |
| 5 m | −80 dBm | −86 dBm | 2/2 |


### Between writes

Leave ≥45 s between two writes to one tag. A tag refuses connections while it is
refreshing; 26 s failed in practice.

`-settle` controls how long the connection is held open after the write.
Disconnecting early cancels that draw, and a cancelled draw does not look like
a failure: the page arrives faint or partly drawn, the command exits 0, and
nothing anywhere says the drawing did not finish. The write succeeded. The
panel was put to sleep in the middle of drawing it.

**The 30 s default is not enough for every panel.** A 4.2" 400x300 BWR tag
needed 90 s; at 30 s it came out faint every time. How much of the page is
coloured does not appear to matter — the same page with no red at all was no
clearer at the same setting — so this is the panel's own refresh time rather
than anything about what is on it. The figure has not been bracketed: 30 s
fails and 90 s works on that tag, and nothing in between has been tried.

`-settle` matters to `mode` for the same reason. Setting the clock redraws the
panel whatever the mode, so a short settle cancels that draw too.

**`-settle 0` means zero.** It disconnects as soon as the write lands, which
cancels the draw with it. Leave the flag out to get the 30 s default. A
negative duration is refused.

### Connection failures

| What it looks like | Where |
|---|---|
| `le-connection-abort-by-local` | Opening the connection |
| The panel does not say what it is within 5 s | After `INIT`, EPD-nRF5 only |
| `ATT error: 0x0e` | A random frame mid-transfer |

A failure like these is retried: three attempts, two seconds apart. Only when
all three fail does the command report an error and exit.

A tag going quiet between advertising windows is normal. Not seeing one is not
the same as it not being there, which is what the retries are for.

## Running the HTTP service

```bash
inkwire serve -listen 127.0.0.1:8080
```

Each route does what the subcommand of the same name does, and takes the same
parameters under the same names.

| Route | Command | What it does |
|---|---|---|
| `GET /v1/scan` | `inkwire scan` | Lists the tags a scan can currently see |
| `POST /v1/render` | `inkwire render` | Renders locally and returns a PNG preview |
| `POST /v1/push` | `inkwire push` | Renders a scene and writes it to a tag |
| `POST /v1/mode` | `inkwire mode` | Hands an EPD-nRF5 tag back to its own clock or calendar |

`?device=` is required by every route that writes, exactly as `-device` is on
the command line. The service does not pick a tag for a request.

```bash
curl http://127.0.0.1:8080/v1/scan

curl -H 'Content-Type: application/json' --data-binary @page.json \
  http://127.0.0.1:8080/v1/render -o render.json

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/render?panel=gicisky:0x0033' -o render.json

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/push?device=NRF_EPD_C1F8'

curl -X POST 'http://127.0.0.1:8080/v1/mode?device=NRF_EPD_C1F8&mode=clock'
```

| Query parameter | Used by | Meaning |
|---|---|---|
| `size` | render | Lay the scene out at `WxH` instead of the size it declares |
| `panel` | render | Lay the scene out for a named panel and check its inks |
| `device` | push, mode | Advertised name or BLE address, required |
| `family` | push | Given, it asserts the device belongs to that family |
| `mode` | mode | `picture`, `calendar` or `clock`; `calendar` by default |
| `week-start` | mode | `sunday` or `monday`; unset leaves the tag's own setting |

`/v1/render` and `/v1/push` answer with the full `report`; `/v1/push` and
`/v1/mode` also carry the device status. A page that `?panel=` refuses comes
back as `unprocessable-scene` with `pngBase64` still in the body: the page
drew, and what it looks like is what says which part of it has to change.


### Warnings

Non-fatal.

| `code` | Meaning |
|---|---|
| `text-clipped` | Text exceeds the box it is in: characters along the line, whole lines, or rows of ink off the top or bottom |
| `layout-overflow` | Children exceed the container on the main axis |
| `empty-layout` | Padding or size leaves no drawable area |
| `missing-runes` | Font lacks these glyphs |
| `size-mismatch` | The scene states a size the panel does not have; laid out again for the panel, anything beyond it clipped |
| `unsupported-ink` | The scene uses an ink the panel has no plane for; drawn black instead, one warning per ink |
| `unsupported-declaration` | A page uses a CSS property or value this renderer does not implement; it was ignored, not approximated |
| `unsupported-at-rule` | A page uses an at-rule such as `@media`; there is one panel and one frame, so there is nothing to select on |
| `unresolved-drawing` | An `svg` element that states no size, or has nothing in it this build can draw |
| `unresolved-image` | An `img` element named a picture that could not be read |
| `no-stylesheet` | The page has no style at all: no stylesheet was given, and it carries no `style` element, no `link` and no `style` attribute |
| `unresolved-stylesheet` | A `link` element named a stylesheet that could not be read |
| `unsupported-selector` | A selector this build cannot read; that rule is skipped and the rest of the stylesheet still applies |
| `unreadable-rule` | A rule with a syntax error in it; skipped the same way, so one bad rule does not cost the stylesheet |
| `substituted-font-size` | A page asked for a size or a family this build has no strike for; drawn at the nearest one it has, which the compiled document names |

### Errors

| `code` | Status | Meaning |
|---|---|---|
| `unsupported-media-type` | 415 | Not JSON or multipart |
| `invalid-request` | 400 | No `device` named, an unknown `family`, `mode` or `week-start`, or malformed multipart |
| `request-too-large` | 413 | Over the size limit |
| `invalid-scene` | 422 | Will not decode or render |
| `invalid-page` | 422 | The `page` part is not a page this renderer can compile |
| `unprocessable-scene` | 422 | Renders successfully, but cannot be packed for that panel |
| `render-failed` | 500 | PNG encoding failed |
| `device-busy` | 409 | Adapter in use |
| `device-identify-failed` | 502 | The target is not in range, belongs to the other family, did not advertise a known model, or the scan failed |
| `push-failed` | 502 | Tag error or connection failure |
| `device-timeout` | 504 | The complete device operation exceeded its family budget |
| `scan-failed` | 502 | Scan failed |

### Concurrency

One adapter, one conversation, across devices. A second write is refused with
the holder's status:

```json
{
  "error": "device PICKSMART is being written",
  "code": "device-busy",
  "status": {
    "device": "PICKSMART",
    "state": "pushing",
    "since": "2026-08-13T02:41:07Z"
  }
}
```

`status` accompanies every write result.

| Step | Measured |
|---|---|
| Scan | 4.3 – 11.5 s |
| Connect + first reply | 4.3 – 7.8 s |
| Per block | ~105 ms |
| One Gicisky write | 14.6 – 20.5 s |

Scan 15 s, reply 5 s, retry 2 s, 3 attempts.

## Image sources

| Context | `source` resolves to |
|---|---|
| CLI | Path relative to the scene document; also absolute, `file:`, data URL |
| `serve -assets DIR` | Path under `DIR`; absolute and `..` refused |
| multipart | A file field name in the same request |

```json
{
  "type": "image",
  "source": "assets/portrait.png",
  "processing": "auto"
}
```

```bash
inkwire render -o output/dashboard.png scenes/dashboard/page.json
```

```json
{
  "version": 1,
  "size": {"width": 296, "height": 128},
  "root": {
    "type": "image",
    "source": "portrait",
    "processing": "auto"
  }
}
```

```bash
curl -F 'scene=@page.json;type=application/json' \
     -F 'portrait=@photos/portrait.png;type=image/png' \
     http://127.0.0.1:8080/v1/render -o render.json
```

`scene` is the document; other file fields are assets, and shadow `-assets` for
that request.

| Limit | |
|---|---:|
| Scene JSON | 16 MiB |
| One asset | 32 MiB |
| Request | 64 MiB |
| Asset count | 32 |
| Rendered page or decoded image | 16,777,216 pixels |

## Pages in HTML and CSS

A scene document says where every node goes; a stylesheet says what the page
is and lets the layout follow. The same page written both ways is 2 to 7 times
shorter as markup, and the pages in `examples/desk/` are there in both formats
so the two can be read side by side.

**A page compiles to a scene document.** That is the whole of what the front
end does — it does not lay anything out, measure a glyph, open a picture or
draw a pixel. Everything after the compile is the one path a scene document
has always taken, so a page is held to the same limits, the same refusals and
the same checks, because it is not being read by anything else.

You can see the middle:

```
inkwire compile examples/desk/tasks.html
inkwire compile -o tasks.json examples/desk/tasks.html
```

and then use it, or skip it — every command that takes a `scene.json` takes a
`page.html`.

A page finds its style in three places, which cascade in this order: the file
with the same path and `.css` on it, a `style` element in the page, and a
`link` element naming another file. A page written in one file needs nothing
beside it; only a page with none of the three is warned about.

```
inkwire render examples/desk/tasks.html
inkwire push -device 命令行墨水屏 examples/desk/tasks.html
```

The root element's `width` and `height` are the page. Its `orientation`
attribute is `landscape`, `portrait-cw` or `portrait-ccw`, and absent means
landscape, as it does in a scene document.

Every example under `examples/` is a page and its picture, and nothing else.
`examples/schema_quickstart/` is the exception, and keeps the scene document
beside the page: a test renders both and compares them pixel for pixel, which
is what keeps the two ways of writing a page from becoming two renderers.

### Over HTTP

`POST /v1/render` and `POST /v1/push` take a page in a multipart request: a
`page` part, a `stylesheet` part, and a file part for every picture or drawing
the page names. A request carries a `scene` part or a `page` part, not both.

### What a stylesheet can say

Sixty-three properties, which is the whole of the box model, flex, grid and
text once the ones a panel has no use for are taken out.

| Group | Properties |
|---|---|
| Box | `display` `width` `height` `min-width` `max-width` `min-height` `max-height` `aspect-ratio` `box-sizing` `padding` `padding-top` `padding-right` `padding-bottom` `padding-left` `margin` `margin-top` `margin-right` `margin-bottom` `margin-left` |
| Flex and grid | `flex` `flex-direction` `flex-basis` `flex-grow` `gap` `row-gap` `column-gap` `align-items` `align-self` `justify-content` `justify-items` `justify-self` `grid-template-columns` `grid-template-rows` `grid-column` `grid-row` |
| Position | `position` `top` `right` `bottom` `left` `inset` `z-index` |
| Paint | `background` `background-color` `color` `border` `border-width` `border-style` `border-color` `border-radius` `visibility` |
| Clipping and transform | `overflow` `clip-path` `transform` `rotate` `transform-origin` `scale` |
| Drawing | `fill` `stroke` `stroke-width` — what a shape in an `svg` element is painted with, and a stylesheet outranks the attribute |
| Dashes | `stroke-dasharray` `stroke-dashoffset` — SVG's names, because a dashed line is called that everywhere else |
| Text | `font` `font-family` `font-size` `line-height` `text-align` `vertical-align` `white-space` |
| Image | `object-fit` |

`color`, `font-family`, `font-size`, `line-height`, `text-align` and
`vertical-align` inherit; everything else does not, which matches CSS.
`inherit`, `initial`, `unset` and `revert` work on any of them.

Lengths are pixels, percentages, and `calc` between the two — a size, a
position and a flex basis all take a share of the box.

The spacing between boxes does not: `padding`, `margin`, `gap`, a border width
and a radius, and a font size are counted in whole pixels, and a percentage
there is refused by name rather than taken as the pixel half of itself. That is
the one place the schema is narrower than CSS: the layout holds an inset as an
integer, so there is nothing for a share to resolve against.

`line-height` is the exception among them, because a bare number and a
percentage are both ratios of the font size and that is a number this does
know. Both are kept as the ratio and worked out once the font size is settled,
so the order the two are written in does not change the line, and a child with
its own size gets its own line from the same ratio.

`margin: auto` pushes an item to the other end of its container.
`white-space: pre` keeps the runs of spaces a page lines its columns up with.

### What it cannot, and why

There is nothing for `opacity`, gradients, shadows or antialiasing to do on a
panel with four inks and no greys; nothing for font weight or italic with
bitmap strikes; nothing to animate in one static frame; and nowhere for
overflow to scroll. Those are absent rather than approximated.

Geometry is absent for a different reason. CSS has no vocabulary for an arc, a
polygon, a pattern or a path, and giving it one would be inventing a dialect
that looks like CSS and is not. SVG is that vocabulary, already standard, and
it goes in the page — inline, or named by an `img`:

```html
<svg viewBox="0 0 214 74"><polyline points="0,8 6,2 12,8"/></svg>
<div class="frame"><img src="chart-plot.svg"></div>
```

which is how `examples/desk/chart.html` draws a ninety-six point series: the
points are not something anyone writes by hand, and whatever produced the
series produced them too. An external drawing is compiled in place, so what
comes out is one self-contained document, which is what a device is given.

### What a drawing can say

`rect` `circle` `ellipse` `line` `polyline` `polygon` `path` `g` `clipPath`
`pattern` `defs` `title` `desc`, painted with `fill` `stroke` `stroke-width`
`stroke-dasharray` `stroke-dashoffset` and clipped with `clip-path`. A `path`
reads the whole of the `d` grammar: `M L H V C S Q T A Z`, their relative
forms, a command repeating without being written again, and the control point
a smooth curve leaves out.

A `transform` is folded into the coordinates, which a `translate` and a
whole-number `scale` can be and a `rotate`, a `skew` or a `matrix` cannot —
those are reported. A `viewBox` is magnified by a whole number or not at all.

A stylesheet outranks a presentation attribute, as it does in CSS, which is
what makes a drawing somebody else made usable: the shapes are kept and the
colours are restated in terms the panel has.

```css
svg path { fill: black; }
```

Nothing is dropped quietly. An unknown property, an unsupported value, an ink
that is not one of the four and a font size with no strike each produce a
warning that names the element and the declaration.

## Examples

| Page | Size |
|---|---|
| [desk](examples/desk/): [claude](examples/desk/claude.html) [disk](examples/desk/disk.html) [tasks](examples/desk/tasks.html) [btc](examples/desk/btc.html) [chart](examples/desk/chart.html) | 296x128 |
| [panel_check](examples/panel_check/): [primitives](examples/panel_check/primitives.html) [polarity](examples/panel_check/polarity.html) | 400x300 |
| [layout_showcase](examples/layout_showcase/page.html) — `grid` `anchored` `transformed` `clip` `clipShape` | 296x128 |
| [fridge](examples/fridge/page.html) | 400x300 |
| [compose_showcase](examples/compose_showcase/page.html) | 296x128 |
| [showcase](examples/showcase/page.html) — shapes and text | 296x128 |
| [paint_showcase](examples/paint_showcase/page.html) — clipping, patterns, dashes | 296x128 |
| [state_showcase](examples/state_showcase/page.html) | 296x128 |
| [card_showcase](examples/card_showcase/page.html) | 296x128 |
| [text_showcase](examples/text_showcase/page.html) | 296x128 |
| [cookbook](examples/cookbook/main.go) — display API | 296x128 |

Regenerate reference images: `INKWIRE_UPDATE_REFERENCES=1 go test ./...`

<table>
  <tr>
    <td><a href="examples/desk/btc.html"><img src="examples/desk/btc.png" alt="btc"></a></td>
    <td><a href="examples/desk/chart.html"><img src="examples/desk/chart.png" alt="chart"></a></td>
  </tr>
  <tr>
    <td><a href="examples/desk/disk.html"><img src="examples/desk/disk.png" alt="disk"></a></td>
    <td><a href="examples/desk/tasks.html"><img src="examples/desk/tasks.png" alt="tasks"></a></td>
  </tr>
  <tr>
    <td><a href="examples/layout_showcase/page.html"><img src="examples/layout_showcase/layout_showcase.png" alt="layout"></a></td>
    <td><a href="examples/compose_showcase/page.html"><img src="examples/compose_showcase/compose_showcase.png" alt="compose"></a></td>
  </tr>
  <tr>
    <td><a href="examples/card_showcase/page.html"><img src="examples/card_showcase/card_showcase.png" alt="card"></a></td>
    <td><a href="examples/showcase/page.html"><img src="examples/showcase/showcase.png" alt="showcase"></a></td>
  </tr>
  <tr>
    <td><a href="examples/paint_showcase/page.html"><img src="examples/paint_showcase/paint_showcase.png" alt="paint"></a></td>
    <td><a href="examples/state_showcase/page.html"><img src="examples/state_showcase/state_showcase.png" alt="state"></a></td>
  </tr>
  <tr>
    <td><a href="examples/text_showcase/page.html"><img src="examples/text_showcase/text_showcase.png" alt="text"></a></td>
    <td><a href="examples/fridge/page.html"><img src="examples/fridge/fridge.png" alt="fridge" width="400"></a></td>
  </tr>
</table>

## Reference

- https://github.com/atc1441/ATC_GICISKY_ESL — protocol
- https://atc1441.github.io/ATC_GICISKY_Paper_Image_Upload.html — upload tool
- https://github.com/tinygo-org/bluetooth — Go BLE
- https://github.com/fpoli/gicisky-tag — Python
- https://github.com/eigger/hass-gicisky — Home Assistant
- https://github.com/tsl0922/EPD-nRF5 — EPD-nRF5 firmware

[中文](README.zh-CN.md)
