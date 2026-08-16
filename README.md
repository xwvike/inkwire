# Inkwire

Scene Schema → e-paper tags. Two families, one renderer.

| | Gicisky | EPD-nRF5 |
|---|---|---|
| Firmware | factory | [replacement](https://github.com/tsl0922/EPD-nRF5), nRF51/nRF52 |
| Name | `PICKSMART`, `NEMR<mac8>` | `NRF_EPD_<mac4>` |
| Model from | advertisement | connection |
| Size | 296x128 | 400x300 … 880x528 |
| Inks | BWR | BW, BWR |

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

Reliability tracks RSSI, not frame count:

| RSSI | 39-frame page |
|---|---|
| −88 dBm | 0/8 |
| −50 dBm | 12/12 |

A weak link surfaces as `le-connection-abort-by-local` at connect, a missing
config reply, or `ATT error: 0x0e` at a random frame. Check RSSI first.

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

## Scene Schema

Integer pixels throughout.

```json
{
  "version": 1,
  "orientation": "landscape",
  "background": "white",
  "root": {
    "type": "absolute",
    "clip": true,
    "children": [
      {
        "bounds": {"x": 4, "y": 4, "width": 288, "height": 24},
        "node": {
          "type": "rectangle",
          "radius": 4,
          "fill": "black"
        }
      },
      {
        "bounds": {"x": 10, "y": 8, "width": 276, "height": 16},
        "node": {
          "type": "text",
          "runs": [
            {"text": "INKWIRE ", "font": "monaco", "size": 12, "ink": "white"},
            {"text": "23.5℃", "font": "ui", "size": 14, "ink": "red"}
          ]
        }
      },
      {
        "bounds": {"x": 12, "y": 42, "width": 272, "height": 72},
        "node": {
          "type": "text",
          "wrap": "runes",
          "lineHeight": 16,
          "runs": [
            {"text": "今天的电子纸页面由 JSON 直接生成。", "font": "ui", "size": 14, "ink": "black"}
          ]
        }
      }
    ]
  }
}
```

![quickstart](examples/schema_quickstart/schema_quickstart.png)

### Page

| Field | Type | Values | Default |
|---|---|---|---|
| `version` | integer | `1` | required |
| `orientation` | string | `landscape`, `portraitClockwise`, `portraitCounterClockwise` | `landscape` |
| `size` | size | preview only | device size |
| `background` | ink | `white`, `black`, `red` | `white` |
| `root` | node | | empty |

### Values

```json
{
  "size": {"width": 100, "height": 40},
  "point": {"x": 10, "y": 5},
  "rect": {"x": 10, "y": 5, "width": 100, "height": 40}
}
```

`ink` is `black`, `white` or `red`; optional colours default to `black`.

```json
{
  "ink": "black",
  "width": 2,
  "dash": [4, 2],
  "dashOffset": 0
}
```

| Field | Type | | Default |
|---|---|---|---|
| `ink` | ink | | `black` |
| `width` | integer | > `0` | required |
| `dash` | integer[] | on/off lengths, each > `0` | solid |
| `dashOffset` | integer | | `0` |

Every node needs `type`. `size` is a preferred size in flow, ignored inside
`absolute.children[].bounds`.

### Length

| Form | Meaning |
|---|---|
| `64` | pixels |
| `"25%"` | share of the container on that axis |
| `"calc(100% - 12px)"` | percentage ± pixels, both terms required |

```json
{"basis": 64, "cross": "25%", "maxMain": "calc(100% - 12px)"}
```

`0` is a length; an absent field is automatic. On `anchored`, `"right": 0` pins
to the edge, no `right` leaves it free.

Distances accept negatives — `anchored` edges, `clipShape` `inset`, `circle` and
`ellipse` `center`, `polygon` `points`:

```json
{"left": -6, "right": "-10%", "top": "calc(0% - 6px)"}
```

Sizes may not: `basis`, `cross`, `width`, `height`, `min*`/`max*`, tracks,
`radius`, `corner`. Percentages and `fr` do not count towards intrinsic size.

## Layout

### absolute

```json
{
  "type": "absolute",
  "clip": true,
  "children": [
    {
      "bounds": {"x": 4, "y": 4, "width": 100, "height": 20},
      "node": {"type": "rectangle", "fill": "black"}
    }
  ]
}
```

| Field | Type | | Default |
|---|---|---|---|
| `size` | size | | content |
| `clip` | boolean | clip to bounds | `false` |
| `children[].bounds` | rect | | required |
| `children[].node` | node | | required |

### row / column

```json
{
  "type": "row",
  "gap": 4,
  "mainAlign": "start",
  "crossAlign": "center",
  "children": [
    {
      "basis": 64,
      "node": {"type": "text", "runs": [{"text": "LEFT"}]}
    },
    {
      "grow": 1,
      "node": {"type": "text", "align": "end", "runs": [{"text": "RIGHT"}]}
    }
  ]
}
```

| Field | Type | | Default |
|---|---|---|---|
| `gap` | integer | | `0` |
| `mainAlign` | string | `start`, `center`, `end` | `start` |
| `crossAlign` | string | `start`, `center`, `end`, `stretch` | `stretch` |
| `children[].grow` | integer | share of leftover | `0` |
| `children[].basis` | length | main size before growing | content |
| `children[].cross` | length | cross size | container |
| `children[].minMain` / `maxMain` | length | | none |
| `children[].minCross` / `maxCross` | length | | none |
| `children[].ratio` | number | main ÷ cross | none |
| `children[].alignSelf` | string | overrides `crossAlign` | container |

### grid

```json
{
  "type": "grid",
  "columns": ["auto", "1fr", "auto"],
  "rows": [15, 15],
  "columnGap": 5,
  "rowGap": 3,
  "children": [
    {"node": {"type": "text", "runs": [{"text": "/data"}]}},
    {"node": {"type": "rectangle", "stroke": {"ink": "black", "width": 1}}},
    {"node": {"type": "text", "align": "end", "runs": [{"text": "1180G"}]}},
    {"column": 1, "columnSpan": 3, "node": {"type": "spacer"}}
  ]
}
```

| Field | Type | | Default |
|---|---|---|---|
| `columns` / `rows` | track[] | `"auto"`, length, or `"1fr"` | one auto track |
| `columnGap` / `rowGap` | integer | | `0` |
| `alignItems` / `justifyItems` | string | `start`, `center`, `end`, `stretch` | `stretch` |
| `children[].column` / `row` | integer | 1-based line; `0` = next cell | `0` |
| `children[].columnSpan` / `rowSpan` | integer | | `1` |
| `children[].alignSelf` / `justifySelf` | string | | grid |

Auto tracks take their widest content; `fr` divides the remainder.

### anchored

```json
{
  "type": "anchored",
  "children": [
    {"left": "50%", "top": 0, "width": 40, "height": 12,
     "node": {"type": "rectangle", "fill": "black"}},
    {"right": 0, "bottom": 0, "width": 20, "height": 20, "layer": 1,
     "node": {"type": "rectangle", "fill": "red"}}
  ]
}
```

| Field | Type | |
|---|---|---|
| `children[].top` `right` `bottom` `left` | length | distance from that edge |
| `children[].width` `height` | length | size on that axis |

Both edges plus a size on one axis is refused.

### transformed

```json
{
  "type": "transformed",
  "scale": 2,
  "turns": 1,
  "child": {"type": "text", "runs": [{"text": "42"}]}
}
```

| Field | Type | | Default |
|---|---|---|---|
| `scale` | integer | whole numbers only | `1` |
| `turns` | integer | quarter turns clockwise | `0` |
| `child` | node | | required |

Anything requiring resampling is refused.

### stack / padding / spacer

```json
{
  "type": "stack",
  "children": [
    {"type": "rectangle", "fill": "white", "stroke": {"ink": "black", "width": 1}},
    {"type": "text", "align": "center", "verticalAlign": "middle", "runs": [{"text": "OK"}]}
  ]
}
```

```json
{
  "type": "padding",
  "insets": {"top": 4, "right": 8, "bottom": 4, "left": 8},
  "child": {"type": "text", "runs": [{"text": "PAD"}]}
}
```

```json
{"type": "spacer", "size": {"width": 8, "height": 8}}
```

| Node | Fields |
|---|---|
| `stack` | `size`, `children[]` — drawn in order over one area |
| `padding` | `insets` (`top` `right` `bottom` `left`), `child` |
| `spacer` | `size` |

## Text

```json
{
  "type": "text",
  "align": "start",
  "verticalAlign": "top",
  "wrap": "none",
  "lineHeight": 14,
  "runs": [
    {"text": "温度 ", "font": "hzk", "size": 14, "ink": "black"},
    {"text": "23.5", "font": "monaco", "size": 14, "ink": "red"},
    {"text": "℃", "font": "hzk", "size": 14, "ink": "red"}
  ]
}
```

| Field | Type | | Default |
|---|---|---|---|
| `runs[].text` | string | | required |
| `runs[].font` | string | `ui`, `hzk`, `monaco` | `ui` |
| `runs[].size` | integer | | `12` |
| `runs[].ink` | ink | | `black` |
| `align` | string | `start`, `center`, `end` | `start` |
| `verticalAlign` | string | `top`, `middle`, `bottom` | `top` |
| `wrap` | string | `none`, `runes` | `none` |
| `lineHeight` | integer | | font |

| Family | Sizes | Coverage |
|---|---|---|
| `ui` | 12 14 16 24 28 32 36 42 48 | Latin + CJK |
| `hzk` | 12 14 16 24 28 32 36 42 48 | Latin + CJK |
| `monaco` | 10 12 14 16 20 24 28 30 32 36 42 48 | **ASCII only** |

Sizes above 16 are integer enlargements. Split mixed lines into runs.

## Image

```json
{
  "type": "image",
  "source": "portrait.png",
  "processing": "manual",
  "options": {
    "fit": "cover",
    "sampling": "bilinear",
    "dither": "floydSteinberg",
    "threshold": 128,
    "redThreshold": 170,
    "redMaxGreen": 170,
    "disableRed": false
  },
  "contrast": {
    "radius": 4,
    "amount": 1.3
  }
}
```

| Field | Type | | Default |
|---|---|---|---|
| `source` | string | see [Image sources](#image-sources) | required |
| `processing` | string | `manual`, `auto` | `manual` |
| `options` | image options | manual parameters | defaults |
| `overrides` | image overrides | override chosen fields of `auto` | none |
| `contrast.radius` | integer | local contrast radius, ≥0 | off |
| `contrast.amount` | number | local contrast strength | off |

Image options and overrides carry the same fields; overrides are per-field.

| Field | Type | | Default |
|---|---|---|---|
| `fit` | string | `stretch`, `contain`, `cover` | `stretch` |
| `sampling` | string | `nearest`, `bilinear` | `nearest` |
| `dither` | string | `threshold`, `floydSteinberg`, `ordered` | `threshold` |
| `threshold` | integer | 0–255 luminance cut | `128` |
| `redThreshold` | integer | 0–255 red channel cut | |
| `redMaxGreen` | integer | 0–255 green ceiling for red | |
| `disableRed` | boolean | drop the red plane | `false` |

`auto` picks threshold, dithering and red extraction from the image and reports
them in `X-Inkwire-Image-Decisions`.

```json
{
  "type": "image",
  "source": "portrait.png",
  "processing": "auto",
  "overrides": {
    "fit": "cover",
    "sampling": "bilinear",
    "dither": "floydSteinberg",
    "disableRed": true
  },
  "contrast": {
    "radius": 4,
    "amount": 1.3
  }
}
```

```json
{
  "type": "image",
  "source": "data:image/png;base64,iVBORw0KGgoAAA...",
  "processing": "manual"
}
```

## Shapes

Area comes from `absolute` bounds or a shared `stack`.

| `type` | Fields | Optional |
|---|---|---|
| `pixel` | `at`, `ink` | `size` |
| `rectangle` | `fill` or `stroke` | `size`, `radius` |
| `line` | `from`, `to`, `stroke` | `size` |
| `polyline` | `points` ≥2, `stroke` | `size` |
| `polygon` | `points` ≥3, `fill` or `stroke` | `size` |
| `circle` | `center`, `radius`, `fill` or `stroke` | `size` |
| `ellipse` | `fill` or `stroke` | `size` |
| `arc` | `start`, `sweep`, `stroke` | `size` |
| `pie` | `start`, `sweep`, `ink` | `size` |
| `chord` | `start`, `sweep`, `ink` | `size` |

Angles in degrees. `ellipse` `arc` `pie` `chord` use the whole rectangle.

```json
{
  "type": "absolute",
  "children": [
    {
      "bounds": {"x": 2, "y": 2, "width": 40, "height": 24},
      "node": {"type": "rectangle", "radius": 4, "fill": "white", "stroke": {"ink": "black", "width": 1}}
    },
    {
      "bounds": {"x": 46, "y": 2, "width": 32, "height": 32},
      "node": {"type": "circle", "center": {"x": 16, "y": 16}, "radius": 14, "stroke": {"ink": "red", "width": 2}}
    },
    {
      "bounds": {"x": 82, "y": 2, "width": 40, "height": 24},
      "node": {"type": "ellipse", "fill": "red", "stroke": {"ink": "black", "width": 1}}
    },
    {
      "bounds": {"x": 126, "y": 2, "width": 40, "height": 24},
      "node": {"type": "arc", "start": -90, "sweep": 270, "stroke": {"ink": "black", "width": 3}}
    },
    {
      "bounds": {"x": 170, "y": 2, "width": 40, "height": 24},
      "node": {"type": "pie", "start": -90, "sweep": 120, "ink": "red"}
    },
    {
      "bounds": {"x": 214, "y": 2, "width": 40, "height": 24},
      "node": {"type": "chord", "start": 20, "sweep": 220, "ink": "black"}
    }
  ]
}
```

```json
{
  "type": "stack",
  "children": [
    {"type": "pixel", "at": {"x": 3, "y": 3}, "ink": "black"},
    {"type": "line", "from": {"x": 0, "y": 20}, "to": {"x": 60, "y": 0}, "stroke": {"ink": "red", "width": 2, "dash": [4, 2]}},
    {"type": "polyline", "points": [{"x": 0, "y": 8}, {"x": 6, "y": 2}, {"x": 12, "y": 8}], "stroke": {"ink": "black", "width": 1}},
    {"type": "polygon", "points": [{"x": 20, "y": 0}, {"x": 40, "y": 10}, {"x": 20, "y": 20}, {"x": 0, "y": 10}], "fill": "red"}
  ]
}
```

## Path

```json
{
  "type": "path",
  "commands": [
    {"op": "move", "to": {"x": 0, "y": 20}},
    {"op": "line", "to": {"x": 20, "y": 0}},
    {"op": "quadratic", "control": {"x": 30, "y": 20}, "to": {"x": 40, "y": 5}},
    {"op": "cubic", "control1": {"x": 45, "y": 0}, "control2": {"x": 50, "y": 20}, "to": {"x": 60, "y": 10}},
    {"op": "arc", "bounds": {"x": 60, "y": 0, "width": 30, "height": 30}, "start": 0, "sweep": 180},
    {"op": "close"}
  ],
  "fill": "black",
  "stroke": {"ink": "red", "width": 1}
}
```

| `op` | Fields |
|---|---|
| `move` | `to` |
| `line` | `to` |
| `quadratic` | `control`, `to` |
| `cubic` | `control1`, `control2`, `to` |
| `arc` | `bounds`, `start`, `sweep` |
| `close` | |

≥1 command, and `fill` or `stroke`.

## Pattern

```json
{
  "type": "pattern",
  "rows": [
    "x...",
    ".r..",
    "..x.",
    "...r"
  ],
  "inks": {
    "x": "black",
    "r": "red"
  }
}
```

Equal-length rows. `inks` keys are single characters; unmapped characters leave
the surface untouched. Tiles across the area.

## Clipping

| Node | Clips to |
|---|---|
| `clip` | the child's own box |
| `clipRect` | a rectangle |
| `clipShape` | `inset`, `circle`, `ellipse`, `polygon` |
| `clipPath` | a path |

| Node | Fields |
|---|---|
| `clip` | `child`, `layer` |
| `clipRect` | `rect`, `child`, `layer` |
| `clipShape` | `shape`, `child`, `layer` |
| `clipPath` | `commands`, `child`, `layer` |

`shape.kind` is `inset`, `circle`, `ellipse` or `polygon`, with `insets`,
`corner`, `radius`, `radiusX`, `radiusY`, `center`, `points` as the kind needs.
`layer` orders nested clips.

```json
{"type": "clip", "child": {"type": "text", "runs": [{"text": "很长的一行"}]}}
```

```json
{
  "type": "clipRect",
  "rect": {"x": 0, "y": 0, "width": 80, "height": 40},
  "child": {"type": "rectangle", "fill": "red"}
}
```

```json
{
  "type": "clipShape",
  "shape": {"kind": "circle", "radius": "50%"},
  "child": {"type": "image", "source": "portrait.png", "processing": "auto"}
}
```

```json
{
  "type": "clipPath",
  "path": {
    "commands": [
      {"op": "arc", "bounds": {"x": 0, "y": 0, "width": 64, "height": 64}, "start": 0, "sweep": 360},
      {"op": "close"}
    ]
  },
  "child": {
    "type": "image",
    "source": "portrait.png",
    "processing": "auto",
    "overrides": {"fit": "cover", "disableRed": true}
  }
}
```

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
