# Markup Manual

Pages use HTML for content, CSS for layout and paint, and SVG for
geometry. The page size comes from the root element:

```html
<main class="page">
  <h1>Inkwire</h1>
</main>
```

```css
.page {
  width: 296px;
  height: 128px;
  background: white;
}
```

`orientation` on the root may be `landscape`, `portrait-cw` or
`portrait-ccw`. It defaults to `landscape`.

## Supported properties

The property name does not imply that every browser value is accepted. Values
outside the lists and restrictions below produce a warning and are ignored.

| Group | Properties |
|---|---|
| Box | `display` `width` `height` `min-width` `max-width` `min-height` `max-height` `aspect-ratio` `box-sizing` `padding` `padding-top` `padding-right` `padding-bottom` `padding-left` `margin` `margin-top` `margin-right` `margin-bottom` `margin-left` |
| Flex and grid | `flex` `flex-direction` `flex-basis` `flex-grow` `gap` `row-gap` `column-gap` `align-items` `align-self` `justify-content` `justify-items` `justify-self` `grid-template-columns` `grid-template-rows` `grid-column` `grid-row` |
| Position | `position` `top` `right` `bottom` `left` `inset` `z-index` |
| Paint | `background` `background-color` `color` `border` `border-width` `border-style` `border-color` `border-radius` `visibility` |
| Clipping and transform | `overflow` `clip-path` `transform` `rotate` `transform-origin` `scale` |
| SVG paint | `fill` `stroke` `stroke-width` `stroke-dasharray` `stroke-dashoffset` |
| Text | `font` `font-family` `font-size` `line-height` `text-align` `vertical-align` `white-space` |
| Image | `object-fit` |

All implemented properties accept `inherit`, `initial`, `unset` and `revert`.
The inherited properties are `color`, `font-family`, `font-size`, `line-height`,
`text-align` and `vertical-align`. Other properties start from their default on
each element.

### Values

- Lengths support `px`, percentages, and `calc()`.
- `width`, `height`, `min-*`, `max-*`, `flex-basis`, `top`, `right`, `bottom`,
  `left`, `inset`, `transform-origin` and track sizes accept percentages.
- Padding, margins, gaps, border widths, radii and font sizes resolve to whole
  pixels. Percentage spacing is rejected.
- `line-height` accepts a pixel length, a unitless ratio, or a percentage of the
  font size.
- Colors are `black`, `white`, `red` and `yellow`. Other colors are reported,
  not approximated.
- Fonts and sizes are limited to the bitmap strikes bundled with the build. A
  missing family or size uses the nearest available strike and reports a
  `substituted-font-size` warning.

### Display and layout

`display` accepts `block`, `inline`, `inline-block`, `flex`, `inline-flex`,
`grid`, `inline-grid`, `contents` and `none`.

Flex containers use `flex-direction: row` or `column`. `flex-grow` divides
remaining space, `flex-basis` sets an initial size, and `gap`, `row-gap` and
`column-gap` separate items. `align-items`, `align-self`, `justify-content`,
`justify-items` and `justify-self` control alignment. `margin: auto` can push an
item to the far side of its container.

Grid containers use `grid-template-columns` and `grid-template-rows` with
pixel, percentage, `fr` and `auto` tracks. `grid-column` and `grid-row` place
an item; `span` can specify its extent.

`box-sizing` accepts `content-box` and `border-box`. `overflow: hidden` clips
content to the box. `white-space: pre` preserves runs of spaces; the default
collapses whitespace.

### Positioning

```css
.page { position: relative; }
.badge { position: absolute; right: 4px; top: 4px; }
.hint { position: relative; left: 2px; top: -1px; }
```

`position: relative` keeps the element in normal flow and offsets its painted
box by `top`, `right`, `bottom`, `left` or `inset`. `position: absolute` removes
the element from flow and places it against the nearest positioned ancestor.
`z-index` controls the paint order of positioned elements.

### Paint and transform

`background` and `border` accept the supported colors. Borders use
`border-style: solid`, `dashed` or `none`; `border-radius` rounds the corners.
`visibility: hidden` keeps the box but removes its paint.

`transform` accepts `rotate` and whole-number `scale`. `rotate` accepts an
angle, with `transform-origin` controlling the pivot. Unsupported transform
functions are reported.

## SVG

```html
<svg viewBox="0 0 214 74">
  <polyline points="0,8 6,2 12,8" />
</svg>
<img src="chart-plot.svg" />
```

Supported elements are `rect`, `circle`, `ellipse`, `line`, `polyline`,
`polygon`, `path`, `g`, `clipPath`, `pattern`, `defs`, `title` and `desc`.
Supported paint properties are `fill`, `stroke`, `stroke-width`,
`stroke-dasharray`, `stroke-dashoffset` and `clip-path`.

`path` accepts `M L H V C S Q T A Z` and relative commands. `translate` and
whole-number `scale` transforms are supported. A CSS property overrides an SVG
presentation attribute when both specify the same value.

## Images and files

An `img` references a resource through `src`. `src` may be an HTTP/HTTPS URL or
a relative path. `.png`, `.jpg` and `.jpeg` are loaded as bitmaps. `.svg` is
compiled as an external drawing.

```
page.html
assets/
  portrait.png
  chart.svg
```

```html
<img src="assets/portrait.png" class="portrait" />
<img src="assets/chart.svg" class="chart" />
<img src="https://example.com/portrait.png" class="remote" />
```

The CLI injects local resources with repeatable `-asset SRC=FILE` flags:

```bash
inkwire render \
     -asset assets/portrait.png=photos/portrait.png \
     -asset assets/chart.svg=charts/chart.svg \
     page.html
```

When no mapping is given, the CLI still reads relative resources beside the
page. HTTP uses multipart: `page` is the HTML document; each local resource is a
binary file part whose field name exactly matches `src`, including any directory
prefix. HTTP/HTTPS resources need no file part.

A client-local path is not visible to the service; local HTTP resources must be
uploaded with the request.

```bash
curl -F 'page=@page.html;type=text/html' \
     -F 'assets/portrait.png=@assets/portrait.png;type=image/png' \
     -F 'assets/chart.svg=@assets/chart.svg;type=image/svg+xml' \
     http://127.0.0.1:8080/v1/render
```

`link` `href` follows the same relative-resource rule, and `stylesheet` may be sent as its own part.

## Warnings

Unsupported properties, values, selectors, at-rules, colors, images and fonts
produce warnings. The rest of the page continues to compile. Layout warnings
such as `text-clipped`, `layout-overflow` and `empty-layout` identify content
that does not fit the fixed canvas.
