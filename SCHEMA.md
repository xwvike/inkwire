# Inkwire Scene Schema

The document `inkwire render`, `push` and `/v1/push` decode.
Run `inkwire schema` to print this. [中文](SCHEMA.zh-CN.md) · [README](README.md)

## Scene Schema

Integer pixels throughout.

```json
{
  "version": 1,
  "orientation": "landscape",
  "size": {"width": 296, "height": 128},
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

### Page

| Field | Type | Values | Default |
|---|---|---|---|
| `version` | integer | `1` | required |
| `orientation` | string | `landscape`, `portraitClockwise`, `portraitCounterClockwise` | `landscape` |
| `size` | size | the page this scene is laid out on | required unless the caller states one |
| `background` | ink | `white`, `black`, `red`, `yellow` | `white` |
| `root` | node | | empty |

Rendered pages and decoded source images are limited to 16,777,216 pixels.

### Values

The three compound values the rest of this document refers to by name.

| Value | Fields | Type | Values |
|---|---|---|---|
| size | `width`, `height` | integer | ≥ `0` |
| point | `x`, `y` | integer | may be negative |
| rect | `x`, `y`, `width`, `height` | integer | `x` and `y` may be negative |

```json
{
  "size": {"width": 100, "height": 40},
  "point": {"x": 10, "y": 5},
  "rect": {"x": 10, "y": 5, "width": 100, "height": 40}
}
```

`ink` is `black`, `white`, `red` or `yellow`; optional colours default to `black`.

```json
{
  "ink": "black",
  "width": 2,
  "dash": [4, 2],
  "dashOffset": 0
}
```

| Field | Type | Values | Default |
|---|---|---|---|
| `ink` | ink | `black`, `white`, `red`, `yellow` | `black` |
| `width` | integer | > `0` | required |
| `dash` | array of integer | on/off lengths, each > `0` | solid |
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

Auto tracks take their widest content; `fr` divides the remainder. Placement
follows CSS grid's sparse row flow: fully positioned children may overlap, a
child with only `row` creates implicit columns when that row is full, a child
with only `column` searches downward, and fully automatic children create
implicit rows. Implicit tracks are automatic tracks.

CLI commands print each expansion. HTTP rendering responses keep the full
per-grid records in the JSON body's `report.GridExpansions` field.

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
| `children[].layer` | integer | higher layers paint later; equal layers keep document order |
| `children[].node` | node | content |

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

| Field | Type | Values | Default |
|---|---|---|---|
| `type` | string | `stack`, `padding`, `spacer` | required |
| `size` | size | `stack` and `spacer` only | `stack` the container's, `spacer` `0`×`0` |
| `children[]` | array of node | `stack` only: drawn in order over one area, later children on top | empty |
| `insets` | object | `padding` only: `top`, `right`, `bottom`, `left`, each an integer | `0` a side |
| `child` | node | `padding` only | required on `padding` |

A `spacer` needs no `size`. Left out it is zero by zero, which is what it should
be when `grow` is what decides how much room it takes.

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
| `lineHeight` | integer | the line box height; the spare is split above and below | font |

| Family | Sizes | Coverage |
|---|---|---|
| `ui` | 12 14 16 24 28 32 36 42 48 | Latin + CJK |
| `hzk` | 12 14 16 24 28 32 36 42 48 | **CJK only** — GB2312, no ASCII |
| `monaco` | 10 12 14 16 20 24 28 30 32 36 42 48 | **ASCII only** |

`ui` is the only family that covers both: it is `monaco` and `hzk` paired, with
`monaco` answering first. Asking `hzk` for a digit gets a `missing-runes`
warning and a placeholder box, so mixed text either uses `ui` or is split into
one run per script.

Sizes above 16 are integer enlargements.

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
| `source` | string | see [Image sources](README.md#image-sources) | required |
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

`auto` picks threshold, dithering and red extraction from the image.
`/v1/render` and `/v1/push` return the decisions in `report.Images`, and CLI
commands print them. A successful `/v1/render` response is always JSON and
carries its PNG in base64 `pngBase64` beside the report.

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

| Field | Type | Values | Default |
|---|---|---|---|
| `type` | string | `pixel`, `rectangle`, `line`, `polyline`, `polygon`, `circle`, `ellipse`, `arc`, `pie`, `chord` | required |
| `size` | size | the area to draw in | the container's |
| `fill` | ink | `rectangle`, `polygon`, `circle`, `ellipse` | unfilled |
| `stroke` | stroke | every type except `pixel`, `pie` and `chord` | unstroked |
| `ink` | ink | `pixel`, `pie` and `chord` only | `black` |
| `at` | point | `pixel` only: where it goes | required on `pixel` |
| `radius` | length | `rectangle` corner radius, or the `circle` radius | `0` |
| `from` | point | `line` only: the first end | required on `line` |
| `to` | point | `line` only: the other end | required on `line` |
| `points` | array of points | `polyline` at least 2, `polygon` at least 3 | required on both |
| `center` | point | `circle` only: the centre | the area's centre |
| `start` | number | `arc`, `pie`, `chord` only: first angle in degrees | required on those three |
| `sweep` | number | `arc`, `pie`, `chord` only: angle swept in degrees | required on those three |
| `rotation` | number | `ellipse`, `arc`, `pie` and `chord`: degrees clockwise the ellipse is turned about the centre of its area. It turns the ellipse and not the area, so a turned one reaches outside the area it was measured in | `0` |

A shape needs a `fill` or a `stroke` to draw anything, except `pixel`, `pie`
and `chord`, which take an `ink`. `ellipse`, `arc`, `pie` and `chord` use the
whole rectangle rather than a centre and radius.

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
| `arc` | `bounds`, `start`, `sweep`, `rotation` |
| `close` | |

≥1 command, and `fill` or `stroke`.

## Pattern

| Field | Type | Values | Default |
|---|---|---|---|
| `type` | string | `pattern` | required |
| `size` | size | the area to tile | the container's |
| `rows` | array of string | one string per row, all the same length | required |
| `inks` | object | single-character key to `black`, `white`, `red`, `yellow` | required |

A character with no entry in `inks` leaves the surface untouched. The rows tile
across the area.

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

## Clipping

Four nodes, one child each, differing only in what they clip to.

| Field | Type | Values | Default |
|---|---|---|---|
| `type` | string | `clip`, `clipRect`, `clipShape`, `clipPath` | required |
| `size` | size | the area to clip within | the container's |
| `child` | node | the node being clipped | required |
| `rect` | rect | `clipRect` only: the rectangle to keep | required on `clipRect` |
| `shape` | shape | `clipShape` only: see below | required on `clipShape` |
| `path` | path | `clipPath` only: `path.commands` holds the commands | required on `clipPath` |

`clip` takes none of the last three: it clips to the child's own box.

### shape

| Field | Type | Values | Default |
|---|---|---|---|
| `kind` | string | `inset`, `circle`, `ellipse`, `polygon` | required |
| `insets` | array of 4 lengths | `inset` only: top, right, bottom, left | required on `inset` |
| `corner` | length | `inset` only: corner radius | `0` |
| `radius` | length | `circle` only | required on `circle` |
| `radiusX` | length | `ellipse` only | required on `ellipse` |
| `radiusY` | length | `ellipse` only | required on `ellipse` |
| `center` | point of lengths | `circle` and `ellipse` only; refused on the others | the area's centre |
| `points` | array of points of lengths | `polygon` only, at least three | required on `polygon` |

Clips do not own painting order: each has one child and nested clips intersect.
For overlapping sibling clips, put them in a `stack` (later child on top), or
use `anchored.children[].layer` when they also need positioned boxes.

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

## Report

Every render answers with a report of what compilation measured. It changes
nothing about the document; it says what the description turned out to mean.
`inkwire render` prints it, and `/v1/render` and `/v1/push` return it as
`report`.

```json
{
  "bounds": {"x": 0, "y": 0, "width": 296, "height": 128},
  "missingRunes": "翯𡃁",
  "warnings": [
    {"path": "root.children[2]", "code": "text-clipped", "message": "\"3260/3720G\" does not fit 40x15: 6 pixels of the line and 0 whole lines are cut off"}
  ],
  "gridExpansions": [
    {"path": "root", "implicitColumns": 0, "implicitRows": 2}
  ],
  "images": [
    {
      "path": "root.children[0]",
      "options": {"fit": "contain", "sampling": "bilinear", "dither": "floydSteinberg", "threshold": 128, "redThreshold": 170, "redMaxGreen": 170, "disableRed": false},
      "profile": {"midToneFraction": 0.61, "threshold": 97, "redSeparation": 0, "lostToLuminance": 0.02, "photographic": true, "redIsMeaningful": false, "colourCarriesStructure": false},
      "toneByColourDistance": false,
      "contrastEnhanced": true
    }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `bounds` | rect | What the page actually drew into, as an origin and a size |
| `missingRunes` | string | The characters no bundled font could draw, as the characters themselves |
| `warnings` | array | Non-fatal consequences of the description; the codes are listed in the README. `unsupported-ink` appears only when a panel was named, since the ink set is not known without one |
| `gridExpansions` | array | Tracks auto-placement created beyond the ones a grid declared |
| `images` | array | What automatic image processing decided, one entry per `"processing": "auto"` image |

Only `bounds` is always present. The four arrays are omitted when there is
nothing to say, so a clean render's report is its bounds and nothing else.

`images[].options` uses the same spellings the `image` node accepts, so a
decision reads back in the vocabulary it would be written in. `images[].profile`
is what the image measured before those options were chosen, and exists so an
automatic decision can be disagreed with: `overrides` on the node is how.
