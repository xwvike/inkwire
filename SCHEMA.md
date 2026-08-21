# Inkwire Scene Schema

The document `inkwire render`, `push` and `/v1/push` decode.
Run `inkwire schema` to print this. [中文](SCHEMA.zh-CN.md) · [README](README.md)

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
| `background` | ink | `white`, `black`, `red`, `yellow` | `white` |
| `root` | node | | empty |

Rendered pages and decoded source images are limited to 16,777,216 pixels.

### Values

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

| Node | Fields |
|---|---|
| `stack` | `size`, `children[]` — drawn in order over one area; later children are on top |
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
| `clip` | `size`, `child` |
| `clipRect` | `size`, `rect`, `child` |
| `clipShape` | `size`, `shape`, `child` |
| `clipPath` | `size`, `path`, `child` (`path.commands` holds the commands) |

`shape.kind` is `inset`, `circle`, `ellipse` or `polygon`, with `insets`,
`corner`, `radius`, `radiusX`, `radiusY`, `center`, `points` as the kind needs.
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
