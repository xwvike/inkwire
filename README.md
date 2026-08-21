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
| `inkwire render [-o out.png] <scene.json>` | PNG preview |
| `inkwire push -device NAME [-family gicisky\|nrfepd] [-settle 30s] <scene.json>` | Render and write |
| `inkwire mode -device NAME [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 clock / calendar mode |
| `inkwire serve [-listen ADDR] [-device NAME] [-assets DIR]` | Start the HTTP service |
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

A scene that states no `size` renders at 296x128. To preview another size,
state one at the top of the scene:

```json
{
  "version": 1,
  "size": {"width": 400, "height": 300},
  "root": {"type": "text", "runs": [{"text": "400x300", "font": "monaco", "size": 14}]}
}
```

The page-level fields are documented in full under "Page" in
[SCHEMA.md](SCHEMA.md).

### push

Renders a scene and writes it to a tag.

```bash
inkwire push -device NEMR92943861 page.json
```

```
writing to NEMR92943861 (e2ada7d1-187a-caea-21e4-d895f8240b62, gicisky)
pushing 9472 bytes (EPD 2.9" BWR 296x128)
tag requested 244 byte messages -> 240 byte blocks
upload complete, tag is refreshing
```

| Flag | Default | Meaning |
|---|---|---|
| `-device` | required | Advertised name (`NAME` column) or BLE address |
| `-family` | `auto` | Given, it asserts the device belongs to that family instead of working it out |
| `-settle` | `30s` | EPD-nRF5 only: how long to stay connected after `REFRESH`, see [Between writes](#between-writes) |

A scene stating a size the panel does not have is not refused: it is laid out
again for the panel and a `size-mismatch` warning is reported.

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
| `-settle` | `30s` | How long to stay connected while the panel redraws |

Only EPD-nRF5 has this. Pointed at a Gicisky tag it is refused by name:

```
NEMR92943861 (e2ada7d1-…, gicisky) is a gicisky tag, but the family asked for is nrfepd
```

`push` leaves a tag in picture mode, so this is the way back; see [Clock and
calendar](#clock-and-calendar).

### serve

Starts the HTTP service, for the callers that are not a command line.

```bash
inkwire serve -listen 127.0.0.1:8080 -device NEMR92943861
```

```
listening on http://127.0.0.1:8080
```

| Flag | Default | Meaning |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | Listen address, loopback only |
| `-device` | none | Default target for requests that name none |
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

Read after connecting with `INIT`. A size mismatch is not refused: the page is laid out again for the panel and a `size-mismatch` warning is reported.

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

Sending end: a Realtek RTL8761BU dongle (OS: Debian, USB `0bda:8771`, Bluetooth
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
Disconnecting early cancels that draw.

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
inkwire serve -listen 127.0.0.1:8080 -device NEMR92943861
```

| Route | What it does | Response |
|---|---|---|
| `POST /v1/render` | Renders locally, to preview the result | JSON containing `width`, `height`, base64 `pngBase64`, and full `report` |
| `POST /v1/display` | Connects to a tag and renders on the hardware | JSON result containing the device status and full `report` |
| `GET /v1/devices` | Scans for every supported tag around | The list of tags |


```bash
curl -H 'Content-Type: application/json' --data-binary @page.json \
  http://127.0.0.1:8080/v1/render -o render.json

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/display?device=NRF_EPD_C1F8'
```


### Warnings

Non-fatal.

| `code` | Meaning |
|---|---|
| `text-clipped` | Text exceeds the box it is in; characters or lines cut |
| `layout-overflow` | Children exceed the container on the main axis |
| `empty-layout` | Padding or size leaves no drawable area |
| `missing-runes` | Font lacks these glyphs |
| `size-mismatch` | The scene states a size the panel does not have; laid out again for the panel, anything beyond it clipped |

### Errors

| `code` | Status | Meaning |
|---|---|---|
| `unsupported-media-type` | 415 | Not JSON or multipart |
| `invalid-request` | 400 | Malformed multipart, or unknown `family` |
| `request-too-large` | 413 | Over the size limit |
| `invalid-scene` | 422 | Will not decode or render |
| `unprocessable-scene` | 422 | Renders successfully, but cannot be packed for the selected Gicisky panel |
| `render-failed` | 500 | PNG encoding failed |
| `device-busy` | 409 | Adapter in use |
| `device-identify-failed` | 502 | Gicisky target was not found, did not advertise a known model, or scan failed |
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

## Examples

| Page | Size |
|---|---|
| [desk](examples/desk/): [claude](examples/desk/claude.json) [disk](examples/desk/disk.json) [tasks](examples/desk/tasks.json) [btc](examples/desk/btc.json) [chart](examples/desk/chart.json) | 296x128 |
| [panel_check](examples/panel_check/): [primitives](examples/panel_check/primitives.json) [polarity](examples/panel_check/polarity.json) | 400x300 |
| [layout_showcase](examples/layout_showcase/page.json) — `grid` `anchored` `transformed` `clip` `clipShape` | 296x128 |
| [fridge](examples/fridge/page.json) | 400x300 |
| [compose_showcase](examples/compose_showcase/page.json) | 296x128 |
| [showcase](examples/showcase/page.json) — shapes and text | 296x128 |
| [paint_showcase](examples/paint_showcase/page.json) — clipping, patterns, dashes | 296x128 |
| [state_showcase](examples/state_showcase/page.json) | 296x128 |
| [card_showcase](examples/card_showcase/page.json) | 296x128 |
| [text_showcase](examples/text_showcase/page.json) | 296x128 |
| [cookbook](examples/cookbook/main.go) — display API | 296x128 |

Regenerate reference images: `INKWIRE_UPDATE_REFERENCES=1 go test ./...`

<table>
  <tr>
    <td><a href="examples/desk/btc.json"><img src="examples/desk/btc.png" alt="btc"></a></td>
    <td><a href="examples/desk/chart.json"><img src="examples/desk/chart.png" alt="chart"></a></td>
  </tr>
  <tr>
    <td><a href="examples/desk/disk.json"><img src="examples/desk/disk.png" alt="disk"></a></td>
    <td><a href="examples/desk/tasks.json"><img src="examples/desk/tasks.png" alt="tasks"></a></td>
  </tr>
  <tr>
    <td><a href="examples/layout_showcase/page.json"><img src="examples/layout_showcase/layout_showcase.png" alt="layout"></a></td>
    <td><a href="examples/compose_showcase/page.json"><img src="examples/compose_showcase/compose_showcase.png" alt="compose"></a></td>
  </tr>
  <tr>
    <td><a href="examples/card_showcase/page.json"><img src="examples/card_showcase/card_showcase.png" alt="card"></a></td>
    <td><a href="examples/showcase/page.json"><img src="examples/showcase/showcase.png" alt="showcase"></a></td>
  </tr>
  <tr>
    <td><a href="examples/paint_showcase/page.json"><img src="examples/paint_showcase/paint_showcase.png" alt="paint"></a></td>
    <td><a href="examples/state_showcase/page.json"><img src="examples/state_showcase/state_showcase.png" alt="state"></a></td>
  </tr>
  <tr>
    <td><a href="examples/text_showcase/page.json"><img src="examples/text_showcase/text_showcase.png" alt="text"></a></td>
    <td><a href="examples/fridge/page.json"><img src="examples/fridge/fridge.png" alt="fridge" width="400"></a></td>
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
