# Markup Manual

Markup pages use HTML for content, CSS for layout and paint, and SVG for
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

## HTML

Elements are boxes. `div`, `main`, `section`, `header`, `footer`, `p`, headings
and other block elements start as block boxes. `span`, `b`, `i`, `small`, `em`,
`strong`, `a`, `label` and `code` are inline until CSS changes their `display`.
`style`, `link`, `script`, `title`, `meta`, `head`, `base` and `template` are
not drawn.

Use `class`, `id` and `style` attributes in the usual way. An `img` places a
bitmap or an external SVG drawing. An inline `svg` places vector geometry.

## CSS cascade

Styles are collected in this order:

1. The `.css` file beside the page.
2. `style` elements and linked stylesheets in document order.

The `style` attribute is local to its element and has higher priority than
normal stylesheet rules. For one property, declarations are resolved in this
order:

```text
important declaration
        |
normal style attribute
        |
normal selector rule: specificity, then source order
        |
inherited value or property default
```

Selector specificity follows CSS:

```css
/* element */
p { color: black; }

/* class */
.warning { color: red; }

/* id */
#title { color: yellow; }

/* combinations and relationships */
main.warning { color: red; }
main .warning { color: red; }      /* descendant */
main > .warning { color: red; }    /* direct child */
```

Compare IDs first, then classes, attributes and pseudo-classes, then element
names. Additional selectors increase their own specificity category, but an id
still outranks any number of classes or elements. When specificity is equal,
the later declaration wins. `!important` follows the same syntax as CSS and
wins over normal declarations. An inline `!important` declaration wins over an
important stylesheet declaration. Among important declarations, specificity and
source order still decide:

```css
.warning { color: black !important; }
#title { color: red; }
```

Custom properties use the same cascade and inherit through the element tree:

```css
:root { --accent: red; }
.badge { color: var(--accent); }
.badge span { color: var(--accent, black); }
```

`var(--name, fallback)` uses the fallback when the name is not declared. A
missing name without a fallback, or a cycle between custom properties, produces
an `unsupported-declaration` warning.

Selectors the compiler cannot read are skipped with an `unsupported-selector`
warning. Unsupported pseudo-elements and selector functions such as `:is()`
and `:has()` are not silently approximated.

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

- Lengths use `px`, percentages, or `calc()` combining the two.
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

Text uses `vertical-align: middle` when no value is supplied. Use `top` or
`bottom` when an edge position is intentional.

### Paint and transform

`background` and `border` accept the supported colors. Borders use
`border-style: solid`, `dashed` or `none`; `border-radius` rounds the corners.
`visibility: hidden` keeps the box but removes its paint.

`transform` accepts `rotate` and whole-number `scale`. `rotate` accepts an
angle, with `transform-origin` controlling the pivot. Unsupported transform
functions are reported.

## SVG

Use SVG for geometry that CSS does not describe:

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

An `img` whose source ends in `.svg` is compiled as a drawing. Other supported
image formats are loaded as bitmaps. Relative sources resolve beside the page
for the CLI, under `-assets` for the HTTP service, or from multipart file parts
with the same name.

```html
<img src="assets/portrait.png" class="portrait" />
<img src="chart-plot.svg" class="chart" />
```

## Warnings

Unsupported properties, values, selectors, at-rules, colors, images and fonts
produce warnings. The rest of the page continues to compile. Layout warnings
such as `text-clipped`, `layout-overflow` and `empty-layout` identify content
that does not fit the fixed canvas.
