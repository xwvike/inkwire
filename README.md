# Inkwire

Scene Schema → e-paper tags. Two families, one renderer.

| | Gicisky | EPD-nRF5 |
|---|---|---|
| Firmware | factory | [replacement](https://github.com/tsl0922/EPD-nRF5), nRF51/nRF52 |
| Name | `PICKSMART`, `NEMR<mac8>` | `NRF_EPD_<mac4>` |
| Model from | advertisement | connection |
| Size | 296x128 | 400x300 … 880x528 |
| Inks | BWR | BW, BWR |

The document these commands read is the Scene Schema: **[SCHEMA.md](SCHEMA.md)**,
which the binary also carries — `inkwire schema` prints it with no network and no
second download.

## CLI

| Command | Purpose |
|---|---|
| `inkwire render [-o out.png] <scene.json>` | PNG preview |
| `inkwire encode [-o out.bin] <scene.json>` | Device payload, Gicisky only |
| `inkwire push [-device NAME] [-family auto\|gicisky\|nrfepd] [-settle 30s] <scene.json>` | Render and write |
| `inkwire scan [-timeout 15s]` | List tags |
| `inkwire mode [-device NAME] [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 clock and mode |
| `inkwire serve [-listen ADDR] [-device NAME] [-assets DIR]` | HTTP service |
| `inkwire push-payload [NAME] <payload.bin>` | Write a raw payload |
| `inkwire schema [-lang en\|zh]` | Print the Scene Schema reference |
| `inkwire help` | This list; also `-h`, `--help` |
| `inkwire version` | Release tag, or the commit a source build came from; also `-v`, `--version` |

```
$ inkwire scan
ADDRESS                                NAME           RSSI  BATT  FAMILY   MODEL           SIZE      PALETTE
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 2.9" BWR    296x128   BWR
C1:57:DD:3F:C1:F8                      NRF_EPD_C1F8    -62     -  nrfepd   ask on connect            not advertised
```

```bash
inkwire render -o preview.png page.json
inkwire push -device NEMR92943861 page.json
```

| Flag | Notes |
|---|---|
| `-device` | Advertised name (`NAME` column) or BLE address. Default `FF:FF:92:94:38:61` |
| `-family` | `auto` maps `NRF_EPD*` → nrfepd, else gicisky. An address carries no family |
| `-timeout` | Per family; both are scanned in turn |
| `-settle` | Connection held open after `REFRESH`. See [Refresh](#refresh) |

## Gicisky

| | |
|---|---|
| Panel | 296x128, BWR, no greyscale |
| Orientation | landscape 296x128, portrait 128x296 either way |
| Payload | black plane + red plane, 9472 bytes |
| GATT | service `FEF0`, control `FEF1`, data `FEF2` |
| Name | `PICKSMART` during boot only; `NEMR<mac8>` once settled |

Manufacturer data, company `0x5053`:

```
byte   0        1        2   3        4
       id low   battery  firmware     id high
```

Model id = `(data[4] << 8) | data[0]`, low 14 bits. Names carry no model.

11 models (212x104 … 960x640, BW/BWR/BWRY) from
[hass-gicisky](https://github.com/eigger/hass-gicisky) (MIT, © 2025 eigger).
`0x0033` verified; the rest print `unverified`. Unknown ids list but do not drive.

## EPD-nRF5

| | |
|---|---|
| GATT | service `62750001-d828-918d-fb46-b6c11c675aec`, rw+notify `…0002`, version `…0003` |
| Commands | `INIT=0x01` `REFRESH=0x05` `WRITE_IMAGE=0x30` `SET_TIME=0x20` |
| Planes | black + colour, row major, MSB first, set bit = white, colour inverted |
| Compression | firmware RLE; blank 400x300 plane 15000 → 232 bytes |

```bash
inkwire push -device NRF_EPD_C1F8 examples/fridge/page.json
```

### Models

Read after `INIT`. Size mismatch is refused:

```
the page is 296x128 and the panel is UC8176_420_BWR 400x300 BWR; render it at the panel's size
```

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

### Refresh

```
sending 40 frames compressed
staying connected 30s while the panel refreshes; disconnecting now would cancel it
```

| | |
|---|---|
| `-settle` | 30 s default. Disconnecting early cancels the draw. 60 s for large or BWR panels |
| Between writes | ≥45 s to one tag; 26 s fails — the tag refuses connections while refreshing |

RSSI belongs to the whole link — the tag's transmitter, both antennas, the path
and the receiver — so the distances below are one adapter's reach and not the
tags' limit. Measured 2026-08-17 against a Realtek RTL8761BU dongle (USB
`0bda:8771`, Bluetooth 5.1, integrated antenna, USB 2.0 full speed), tags
standing upright with a clear line of sight:

| Distance | Gicisky | EPD-nRF5 | Pushes |
|---|---|---|---|
| 1 m | −66 dBm | −68 dBm | 4/4 |
| 2 m | −74 dBm | −78 dBm | 2/2 |
| 3 m | −76 dBm | −80 dBm | 2/2 |
| 5 m | −80 dBm | −86 dBm | 2/2 |

A card with a better receiver and external antennas — an AX210, say — reads
higher at the same distance and works at the same reading further away. Read the
dBm column, not the metres.

Transfer time does not follow RSSI: 18–29 s Gicisky and 40–48 s EPD-nRF5 across
that whole 18 dB span, for 21-frame and 40-frame pages alike.

Orientation is worth about 12 dB. One tag in one spot read −80 dBm lying flat
and −68 dBm standing up, which is the difference between 1 m and 4 m. Stand
tags up before moving them closer.

An earlier session recorded 0/8 at −88 dBm. Nothing since reproduces a cliff
there — −86 dBm is 2/2 — so it stands as an observation rather than a threshold.

Failure looks like `le-connection-abort-by-local` at connect, a missing config
reply, or `ATT error: 0x0e` at a random frame.

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

## HTTP

```bash
inkwire serve -listen 127.0.0.1:8080 -device NEMR92943861
```

| Route | Response |
|---|---|
| `POST /v1/render` | `image/png` |
| `POST /v1/encode` | 9472 bytes, `application/octet-stream`, Gicisky only |
| `POST /v1/display` | JSON result |
| `GET /v1/devices` | Tags, each with `family` |

`-listen` is loopback only. `/v1/display` takes `?device=` and
`?family=auto|gicisky|nrfepd`. Budget: 45 s Gicisky, 150 s EPD-nRF5.
`/v1/encode` refuses an EPD-nRF5 target with `size-unknown`: that panel reports
its size only once connected. Send the scene to `/v1/display` instead.

```bash
curl -H 'Content-Type: application/json' --data-binary @page.json \
  http://127.0.0.1:8080/v1/render -o preview.png

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/display?device=NRF_EPD_C1F8'
```

Headers: `X-Inkwire-Warnings`, `X-Inkwire-Missing-Runes`, `X-Inkwire-Image-Decisions`.

### Warnings

Non-fatal.

| `code` | Meaning |
|---|---|
| `text-clipped` | Text exceeds its box; characters or lines cut |
| `layout-overflow` | Children exceed the container on the main axis |
| `empty-layout` | Padding or size leaves no drawable area |
| `missing-runes` | Font lacks these glyphs |

### Errors

| `code` | Status | Meaning |
|---|---|---|
| `unsupported-media-type` | 415 | Not JSON or multipart |
| `invalid-request` | 400 | Malformed multipart, or unknown `family` |
| `request-too-large` | 413 | Over the size limit |
| `invalid-scene` | 422 | Will not decode or render |
| `unprocessable-scene` | 422 | Renders, will not encode |
| `size-unknown` | 400 | `/v1/encode` asked for an EPD-nRF5 target |
| `render-failed` | 500 | PNG encoding failed |
| `device-busy` | 409 | Adapter in use |
| `push-failed` | 502 | Tag error or connection failure |
| `device-timeout` | 504 | Retries exhausted |
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
     http://127.0.0.1:8080/v1/render -o preview.png
```

`scene` is the document; other file fields are assets, and shadow `-assets` for
that request.

| Limit | |
|---|---:|
| Scene JSON | 16 MiB |
| One asset | 32 MiB |
| Request | 64 MiB |
| Asset count | 32 |

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
