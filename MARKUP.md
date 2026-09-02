# Markup Manual

Pages use HTML for content, CSS for layout and paint, and SVG for
geometry. The render target supplies the viewport: CLI `render` and `measure`,
and HTTP `render`, require `size` or `panel`; `push` obtains the panel from the
connected device. The root element may omit `width` and `height`; when present,
they are CSS layout properties and do not select the render target.

```html
<main class="page">
  <h1>Inkwire</h1>
</main>
```

```css
.page {
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
Keywords, units and function names are matched without regard to case, as CSS
matches them; a font family is matched the same way and reported back as the
author wrote it.
The inherited properties are `color`, `font-family`, `font-size`, `line-height`,
`text-align` and `white-space`. Other properties start from their default on
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

`overflow: hidden` clips content to the box. `white-space: pre` preserves runs
of spaces; the default collapses whitespace.

### The box model

A box is the one CSS describes, and every part of it counts.

```css
.card { width: 100px; padding: 10px; border: 5px solid black; }   /* 130 wide */
.same { width: 130px; padding: 10px; border: 5px solid black;
        box-sizing: border-box; }                                 /* 130 wide */
```

`box-sizing` starts at `content-box`, as in CSS: a stated `width`, `height`,
`min-*`, `max-*` or `flex-basis` is the size of the content, and the padding and
the border go outside it. `border-box` makes the same properties state the whole
box and leaves the content whatever is left. Most stylesheets open with
`* { box-sizing: border-box; }`, and the examples here do.

A border takes room. The content of a bordered box starts inside the border and
then inside the padding, and an absolutely positioned child is placed against
the padding box — inside the border, outside the padding — which is where CSS
places one.

There is no `border-top`, `border-right`, `border-bottom` or `border-left`: a
border here is drawn as one rectangle, so it is one width, one colour and one
style on all four sides. A single edge is a one-pixel box of its own.

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

`background` and `border` accept the supported colors. `border-style` is
`solid`, `dashed`, `dotted` or `none`, and a dot is as wide as the border and
spaced by the same, as in CSS. `border-radius` rounds the corners.
`visibility: hidden` keeps the box but removes its paint.

A border with no style is not drawn — `border-style` starts at `none`, so
`border: 1px` and a bare `border-width` draw nothing, as in a browser. The
styles that need two lines or two shades (`double`, `groove`, `ridge`, `inset`,
`outset`) are reported and not drawn: a panel with no greys has nothing to
shade one with, and drawing them as solid would put a line on the page in a
shape nobody asked for.

`transform` accepts `rotate` and whole-number `scale`. `rotate` accepts an
angle, with `transform-origin` controlling the pivot. Unsupported transform
functions are reported.

### Text in a box

`line-height` behaves as it does in CSS: the difference between it and the
text's own height is the leading, and half of it goes above the letters and half
below. So a larger `line-height` centres a line in its box rather than pushing
it down, and every line is placed by its own metrics — what is on the line above
or below cannot move it.

Inline content is laid out as line boxes. Text runs, `inline-block`,
`inline-flex`, `inline-grid`, images and inline SVG share the same line and wrap
when `white-space` permits it. Padding, margin, background, border,
`position: relative` and `vertical-align` remain attached to the inline item.

`vertical-align` follows the CSS inline values `baseline`, `top`, `middle` and
`bottom`; its initial value is `baseline`. `inline-block`, `inline-flex` and
`inline-grid` are atomic boxes, so their descendants use the normal block, flex
or grid rules inside that box.

```css
.chip { display: inline-block; height: 20px; vertical-align: middle; }
```

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
     -size 296x128 \
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
     'http://127.0.0.1:8080/v1/render?size=296x128'
```

`link` `href` follows the same relative-resource rule, and `stylesheet` may be sent as its own part.

## Warnings

Unsupported properties, values, selectors, at-rules, colors, images and fonts
produce warnings. The rest of the page continues to compile. Layout warnings
such as `text-clipped`, `layout-overflow` and `empty-layout` identify content
that does not fit the fixed canvas.
