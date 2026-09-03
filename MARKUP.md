# Markup Manual

HTML defines content, CSS defines layout and paint, and SVG defines geometry.
CLI `render` and `measure`, and HTTP `render`, require `size` or `panel`; `push`
obtains the panel size and inks from the target device. The root element may
omit `width` and `height`; when present, they affect CSS layout but do not select
the render target.

`orientation` on the root may be `landscape`, `portrait-cw` or
`portrait-ccw`. It defaults to `landscape`.

## Supported properties

Listing a property does not imply that every browser value is accepted. Values
outside the limits below produce a warning and are ignored.

| Group | Properties |
|---|---|
| Box | `display` `width` `height` `min-width` `max-width` `min-height` `max-height` `aspect-ratio` `box-sizing` `padding` `padding-top` `padding-right` `padding-bottom` `padding-left` `margin` `margin-top` `margin-right` `margin-bottom` `margin-left` |
| Flex and grid | `flex` `flex-direction` `flex-basis` `flex-grow` `flex-shrink` `gap` `row-gap` `column-gap` `align-items` `align-self` `justify-content` `justify-items` `justify-self` `grid-template-columns` `grid-template-rows` `grid-column` `grid-row` |
| Position | `position` `top` `right` `bottom` `left` `inset` `z-index` |
| Paint | `background` `background-color` `color` `border` `border-width` `border-style` `border-color` `border-top` `border-right` `border-bottom` `border-left` `border-top-width` `border-right-width` `border-bottom-width` `border-left-width` `border-top-style` `border-right-style` `border-bottom-style` `border-left-style` `border-top-color` `border-right-color` `border-bottom-color` `border-left-color` `border-radius` `visibility` |
| Clipping and transform | `overflow` `clip-path` `transform` `rotate` `transform-origin` `scale` |
| SVG paint | `fill` `stroke` `stroke-width` `stroke-dasharray` `stroke-dashoffset` |
| Text | `font` `font-family` `font-size` `line-height` `text-align` `vertical-align` `white-space` |
| Image | `object-fit` |

All implemented properties accept `inherit`, `initial`, `unset` and `revert`.
Keywords, units and function names are matched case-insensitively, as in CSS;
font family names are matched the same way and reported as authored. The
inherited properties are `color`, `font-family`, `font-size`, `line-height`,
`text-align`, `white-space` and custom properties. Other properties start from
their initial value on each element.

### Values

- Lengths support `px`, percentages and `calc()`.
- `width`, `height`, `min-*`, `max-*`, `flex-basis`, `top`, `right`, `bottom`,
  `left`, `inset`, `transform-origin` and track sizes accept percentages.
- Padding, margins and gaps resolve against the containing block's inline size
  and settle to whole pixels at layout time. Border widths, radii and font sizes
  are pixel-only.
- `line-height` accepts a pixel length, a unitless ratio, or a percentage of the
  font size.
- Colors are `black`, `white`, `red` and `yellow`, including their three- or
  six-digit hexadecimal forms (for example, `#000` and `#000000`). Other colors
  are reported, not approximated.
- Font families are limited to the bitmap fonts bundled with the build. An
  unavailable family uses the default and reports `unsupported-declaration`; an
  unavailable size uses the nearest strike and reports `substituted-font-size`.

### Cascade and inheritance

Stylesheets are merged in source order: the stylesheet supplied by the CLI comes
first, followed by page `<style>` elements and `link rel="stylesheet"` resources
in document order. For one element, normal declarations are ordered by selector
specificity; a later declaration wins ties. A normal inline `style` declaration
outranks normal selector declarations. `!important` declarations outrank all
normal declarations and are then compared by the same rules.

Custom properties also cascade by the same rules and inherit from the parent.
`var()` resolves them through that inheritance chain; an undefined or cyclic
reference produces a warning and drops the declaration that uses it.

### Display and layout

`display` accepts `block`, `inline`, `inline-block`, `flex`, `inline-flex`,
`grid`, `inline-grid`, `contents` and `none`.

Flex containers use `flex-direction: row` or `column`. `flex-grow` divides
remaining space and `flex-shrink` absorbs negative free space. `flex-basis` sets
the initial size. The `flex` shorthand accepts the CSS forms `flex: <grow>`,
`<grow> <shrink>`, and `<grow> <shrink> <basis>`; a one-number form uses a zero
percent basis, as in a browser. `gap`, `row-gap` and `column-gap` separate items.
`align-items`, `align-self`, `justify-content`, `justify-items` and `justify-self`
control alignment. `margin: auto` can push an item to the far side of its
container.

Grid containers use `grid-template-columns` and `grid-template-rows` with
pixel, percentage, `fr` and `auto` tracks. `grid-column` and `grid-row` place
an item; `span` can specify its extent.

`overflow: hidden` clips content to the box. `white-space: pre` preserves runs
of spaces; the default collapses whitespace.

### The box model

The box model follows CSS; size, padding and borders all participate in layout.

```css
.card { width: 100px; padding: 10px; border: 5px solid black; }   /* 130 wide */
.same { width: 130px; padding: 10px; border: 5px solid black;
        box-sizing: border-box; }                                 /* 130 wide */
```

`box-sizing` defaults to `content-box`: a stated `width`, `height`, `min-*`,
`max-*` or `flex-basis` is the content size, with padding and borders added
outside. `border-box` makes those properties state the whole box and leaves the
content the remaining space.

A border takes room. Content is inside the border and padding; an absolutely
positioned child is placed against the padding box, inside the border and
outside the padding.

`border-top`, `border-right`, `border-bottom` and `border-left` set individual
edges. A side border participates in the box model and supports the same width,
colour and line styles as `border`.

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
`solid`, `dashed`, `dotted`, `none` or `hidden`; a dotted mark and its gap each
equal the border width. `border-radius` rounds the corners. `visibility: hidden`
keeps the box but removes its paint.

`border-style` starts at `none`; without a line style, `border: 1px` and a bare
`border-width` draw nothing. Styles that require multiple lines or shades
(`double`, `groove`, `ridge`, `inset`, `outset`) produce a warning and are skipped.

`transform` accepts `rotate` and whole-number `scale`. `rotate` accepts an
angle, with `transform-origin` controlling the pivot. Unsupported transform
functions are reported.

### Text in a box

`line-height` follows CSS. The difference from the text height is leading, split
between the top and bottom of the line box. Increasing `line-height` expands the
line box; it does not move the text downward. Each line uses its own font metrics.

Inline content is laid out as line boxes. Text, `inline-block`, `inline-flex`,
`inline-grid`, images and inline SVG share a line and wrap when `white-space`
allows it. Padding, margin, background, border, `position: relative` and
`vertical-align` remain attached to their inline item.

`vertical-align` accepts `baseline`, `top`, `middle` and `bottom`; its initial
value is `baseline`. `text-top`, `text-bottom`, `sub`, `super` and length values
produce a warning and are ignored. The property applies only to inline-level
boxes and is not inherited; using it on a block, flex item or grid item produces
a warning.

`inline-block`, `inline-flex` and `inline-grid` are atomic boxes. Their
descendants use the corresponding block, flex or grid formatting context.

To align content inside a box, use `align-items` on the container,
`align-self` on the item, or a `line-height` equal to the box height for a
single text line.

```css
.chip { display: inline-block; height: 20px; vertical-align: middle; }

.row    { display: flex; align-items: center; }   /* centre every item */
.figure { align-self: center; }                   /* centre one item */
.label  { height: 20px; line-height: 20px; }      /* centre one text line */
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
presentation attribute when both specify the same property.
`fill` and `stroke` accept the colors above, `none` and `transparent`; other
colors produce a warning and skip that paint.

## Images and files

An `img` references a resource through `src`. `src` may be an HTTP/HTTPS URL or
a relative path. `.png`, `.jpg` and `.jpeg` are loaded as bitmaps; `.svg` is
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

When no mapping is given, the CLI reads relative resources beside the page. HTTP
uses multipart: `page` is the HTML document; local resources are binary file
parts whose field names exactly match `src`, including any directory prefix.
HTTP/HTTPS resources need no file part. Client-local paths are not visible to the
service.

```bash
curl -F 'page=@page.html;type=text/html' \
     -F 'assets/portrait.png=@assets/portrait.png;type=image/png' \
     -F 'assets/chart.svg=@assets/chart.svg;type=image/svg+xml' \
     'http://127.0.0.1:8080/v1/render?size=296x128'
```

`link` `href` follows the same relative-resource rule; a stylesheet may be sent
as its own part.

## Warnings

Unsupported properties, values, selectors, at-rules, colors, images and fonts
produce warnings; the rest of the page continues to compile. Layout warnings
such as `text-clipped`, `layout-overflow` and `empty-layout` identify content
that cannot fit the fixed canvas.
