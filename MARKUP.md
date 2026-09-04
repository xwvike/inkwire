# Markup Manual

Markup uses HTML for content, CSS for layout and paint, and SVG for geometry. `render`, `measure`,
and HTTP `render` require `size` or `panel`; `push` obtains the panel size and inks from the target
device. The root element may omit `width` and `height`: they affect CSS layout only and do not select
the render target. Root `orientation` accepts `landscape`, `portrait-cw`, and `portrait-ccw`; the
default is `landscape`.

## Supported CSS properties

An omitted value is not implemented and produces `unsupported-declaration`. Shorthands retain CSS
syntax only for the subproperties listed here.

| Category | Properties | Accepted values or syntax | Notes |
|---|---|---|---|
| Box | `display` | `block`, `inline`, `inline-block`, `flex`, `inline-flex`, `grid`, `inline-grid`, `contents`, `none` | `inline-*` creates the corresponding atomic box; `contents` creates no box of its own |
| Size | `width`, `height`, `min-width`, `max-width`, `min-height`, `max-height`, `flex-basis` | `px`, `%`, `calc()`, `auto` where meaningful | Percentages resolve against the containing block; `flex-basis` supplies the flex base size |
| Ratio | `aspect-ratio` | `number` or `number / number` | Both numbers must be positive |
| Box | `box-sizing` | `content-box`, `border-box` | Applies to `width`, `height`, `min-*`, `max-*`, and `flex-basis` |
| Padding | `padding`, `padding-top`, `padding-right`, `padding-bottom`, `padding-left` | Non-negative `px`, `%`, `calc()` | Percentages resolve against the containing block's inline size and settle to whole pixels |
| Margin | `margin`, `margin-top`, `margin-right`, `margin-bottom`, `margin-left` | `px`, `%`, `calc()`, `auto` | `auto` participates in free-space distribution; percentages use the containing block's inline size |
| Flex | `flex` | `none`, `auto`, `<grow>`, `<grow> <shrink>`, `<grow> <shrink> <basis>` | A one-number form uses `0%` basis; `<basis>` accepts `auto`, `px`, `%`, `calc()` |
| Flex | `flex-direction` | `row`, `column` | — |
| Flex | `flex-grow`, `flex-shrink` | Non-negative numbers | The initial `flex-shrink` is `1` |
| Gap | `gap`, `row-gap`, `column-gap` | Non-negative `px`, `%`, `calc()` | `row-gap` percentages resolve against the containing block's block size, `column-gap` against its inline size; the first `gap` value is row-gap and the second is column-gap; values settle to whole pixels |
| Alignment | `align-items`, `align-self`, `justify-items`, `justify-self` | `stretch`, `flex-start`, `start`, `center`, `flex-end`, `end`; `align-self` and `justify-self` also accept `auto` | `justify-*` uses the same alignment set |
| Alignment | `justify-content` | `flex-start`, `start`, `normal`, `center`, `flex-end`, `end`, `space-between` | `space-around` and `space-evenly` are not implemented |
| Grid | `grid-template-columns`, `grid-template-rows` | `px`, `%`, `calc()`, positive `fr`, `auto`, `min-content`, `max-content`, `repeat()` | `repeat()` is limited to 400 tracks |
| Grid | `grid-column`, `grid-row` | `auto`, a line number, `span N`, `N / M`, `N / span M` | Line numbers and spans are positive integers |
| Position | `position` | `static`, `relative`, `absolute` | `fixed` and `sticky` are not implemented |
| Position | `top`, `right`, `bottom`, `left`, `inset` | `px`, `%`, `calc()`, `auto` | Percentages resolve against the positioned containing block |
| Position | `z-index` | Number | Changes paint order only for positioned elements |
| Colour | `background`, `background-color`, `color`, `border-color`, `border-top-color`, `border-right-color`, `border-bottom-color`, `border-left-color` | `black`, `white`, `red`, `yellow`, and their three- or six-digit hexadecimal forms | No alpha, colour functions, or other named colours; unsupported colours are not approximated |
| Border | `border`, `border-top`, `border-right`, `border-bottom`, `border-left` | One width, line style, and colour in any order | An omitted line style is `none` and draws nothing |
| Border | `border-width`, `border-top-width`, `border-right-width`, `border-bottom-width`, `border-left-width` | Non-negative `px` or a pixel-only `calc()` | Rounded to whole pixels at layout; percentages and multi-value border widths are not implemented |
| Border | `border-style`, `border-top-style`, `border-right-style`, `border-bottom-style`, `border-left-style` | `solid`, `dashed`, `dotted`, `none`, `hidden` | A dotted mark and its gap each equal the line width |
| Border | `border-radius` | Non-negative `px` or a pixel-only `calc()` | Rounded to whole pixels at layout; percentages and elliptical two-value radii are not implemented |
| Visibility | `visibility` | `visible`, `hidden` | `hidden` keeps the layout box and removes its paint |
| Clipping | `overflow` | `visible`, `hidden`, `clip` | There is no scrolling |
| Clipping | `clip-path` | `none`, `inset()`, `circle()`, `ellipse()`, `polygon()` | Function arguments accept `px`, `%`, and `calc()`; `inset()` has one radius at most |
| Transform | `transform` | `none`, `scale()`, `rotate()` | Only `scale` and `rotate` functions are implemented |
| Transform | `rotate` | `deg`, `grad`, `rad`, `turn`, or a unitless angle | — |
| Transform | `transform-origin` | One or two keywords, `px`, `%`, or `calc()` | Keywords are `left`, `center`, `right`, `top`, and `bottom` |
| Transform | `scale` | One or two equal integers, each at least 1 | No resampling; fractions and non-uniform pairs are not implemented |
| SVG paint | `fill`, `stroke` | Supported inks, `none`, `transparent` | These properties inherit; CSS overrides an SVG presentation attribute on inline SVG |
| SVG paint | `stroke-width` | Non-negative `px`, a pixel-only `calc()`, or an SVG unitless number | Rounded to whole pixels at layout; values below one pixel draw at one pixel and warn |
| SVG paint | `stroke-dasharray`, `stroke-dashoffset` | Space- or comma-separated integer pixels | — |
| SVG paint | `stroke-linecap` | `butt`, `round`, `square` | Applies to open SVG lines, polylines and paths; SVG defaults to `butt` |
| SVG paint | `stroke-linejoin` | `miter`, `round`, `bevel` | Applies where adjacent SVG path segments meet; SVG defaults to `miter` |
| Font | `font` | `size[/line-height] family` | Only size, line-height, and family are read; style and weight fields warn |
| Font | `font-family` | `ui`, `hzk`, `monaco`, or a family stack | Families are tried per glyph in declaration order; unknown names are skipped, and an entirely unavailable stack uses the default with a warning |
| Font | `font-size` | `px` or a pixel-only `calc()` | The nearest bitmap strike is used and `substituted-font-size` is emitted |
| Text | `line-height` | `px`, a pixel-only `calc()`, a unitless ratio, or a percentage of the font size | Unitless values resolve against the element's own size; percentages inherit their computed length |
| Text | `text-align` | `left`, `start`, `center`, `right`, `end` | — |
| Text | `vertical-align` | `baseline`, `top`, `middle`, `bottom` | Inline-level only; the initial value is `baseline` and it is not inherited |
| Text | `white-space` | `normal`, `nowrap`, `pre`, `pre-wrap` | — |
| Image | `object-fit` | `fill`, `contain`, `cover` | Applies to `img` and external drawings inside their boxes |

Every implemented property accepts `inherit`, `initial`, `unset`, and `revert`. The properties that
inherit by default are `color`, `fill`, `stroke`, `stroke-width`, `stroke-dasharray`,
`stroke-dashoffset`, `stroke-linecap`, `stroke-linejoin`, `font`, `font-family`, `font-size`, `line-height`, `text-align`, `white-space`,
and `visibility`; `vertical-align` is not inherited. Custom properties named `--name` may be declared,
then read with `var()`, and cascade and inherit by the same rules.

Built-in bitmap strikes:

| Family | Available sizes (px) |
|---|---|
| `ui`, `hzk` | `12`, `14`, `16`, `24`, `28`, `32`, `36`, `42`, `48` |
| `monaco` | `10`, `12`, `14`, `16`, `20`, `24`, `28`, `30`, `32`, `42`, `48` |

## Unsupported properties and values

Every CSS property absent from the table above is unsupported. An unsupported value for an implemented
property has the same result: the declaration is ignored and `unsupported-declaration` is emitted.
Common unsupported items include:

- Layout: `float`, `clear`, `order`, `flex-wrap`, `align-content`, `grid-template-areas`,
  `grid-auto-columns`, `grid-auto-rows`, `grid-auto-flow`, `place-content`, `place-items`,
  `place-self`, `table-layout`.
- Position and effects: `position: fixed`, `position: sticky`, `opacity`, `filter`, `box-shadow`,
  `text-shadow`, `mix-blend-mode`, `background-image`, `background-repeat`, `background-size`.
- Text: `font-weight`, `font-style`, `font-variant`, `text-decoration`, `text-indent`,
  `letter-spacing`, `word-spacing`, `text-transform`, `text-overflow`, `hyphens`.
- Images: `object-position`.
- Colour and transform values: `rgb()`, `rgba()`, `hsl()`, gradients, alpha; transform functions
  other than `scale()` and `rotate()`; fractional, negative, or non-uniform CSS `scale`; `border-style`
  values `double`, `groove`, `ridge`, `inset`, `outset`; `overflow: auto` and `overflow: scroll`.

## Implementation notes

- `size` or `panel` defines the canvas. Root `width` and `height` do not change it; content outside
  the canvas produces layout or clipping warnings.
- `box-sizing`, padding, and borders participate in box geometry. An absolutely positioned child is
  anchored to the padding box of its nearest positioned ancestor: inside its border and outside its
  padding.
- The `flex` shorthand preserves grow, shrink, and basis. `flex: 1` is `1 1 0%`; the initial
  `flex-shrink` is `1`.
- Inline content, images, and inline SVG share line boxes. `vertical-align` applies only to inline-level
  boxes; a declaration on a block, flex item, or grid item warns. For a single text line, a line-height
  equal to the box height provides its vertical alignment.
- Stylesheet order is CLI stylesheet, page `<style>`, then linked stylesheets in document order.
  Normal declarations compare specificity and source order; inline style outranks normal selectors and
  `!important` outranks normal declarations. Custom properties cascade and inherit by the same rules;
  an undefined or cyclic `var()` drops the declaration that uses it.
- Padding and margins resolve percentages against the containing block's inline size; `row-gap` uses
  its block size and `column-gap` its inline size. All settle to whole pixels at layout time. Border
  widths, radii, and font sizes accept pixels or pixel-only `calc()`; `calc()` supports addition and
  subtraction of `px` and `%` terms only.
- Fonts come from bitmap families bundled at build time. An unknown family falls back to the default;
  a family stack falls back per glyph; an unavailable size falls back to the nearest strike. The
  bundled faces have no weight variants, so `font-weight` and the default bold styling of `<b>` and
  `<strong>` do not change the rendered glyphs.
- Compilation continues after unsupported declarations, selectors, at-rules, colours, fonts, or
  resources. Warnings are not proof of a complete render; inspect the resulting frame for the warning
  category involved.

## SVG

Supported elements are `rect`, `circle`, `ellipse`, `line`, `polyline`, `polygon`, `path`, `g`, `use`,
`clipPath`, `pattern`, `defs`, `title`, and `desc`. `path` accepts `M`, `L`, `H`, `V`, `C`, `S`, `Q`,
`T`, `A`, and `Z`, including relative commands. `<use href="#id">` and `xlink:href` reuse a named
supported element or group from the same SVG.

SVG paint supports `fill`, `stroke`, `stroke-width`, `stroke-dasharray`, `stroke-dashoffset`,
`stroke-linecap`, `stroke-linejoin`, and `clip-path`. `viewBox` maps to the viewport using the browser's `xMidYMid meet` rule; geometry outside
the viewport is clipped. SVG `transform` supports `translate`, `scale`, and `rotate`; `scale` accepts
finite non-zero factors, and `rotate` accepts an angle or an angle with a rotation centre. CSS
`transform` accepts only the functions listed above.

## Images and resources

`img src` accepts an HTTP/HTTPS URL or a relative path. `.png`, `.jpg`, and `.jpeg` are loaded as
bitmaps; `.svg` is loaded as an external SVG drawing. Relative paths are resolved only through the
page resource map; the HTTP service never reads a client path or an arbitrary file in its working
directory.

An SVG in `<img>` is a replaced image: page CSS sizes and clips its image box, but does not cascade
into the SVG document. Its supported SVG paint and viewport are retained. Page CSS declarations for
SVG paint properties on the image produce `unsupported-declaration` warnings. Inline `<svg>` remains
part of the page's CSS cascade.

The CLI injects local resources with repeatable `-asset SRC=FILE` flags. Without a mapping, it reads
relative resources beside the page:

```bash
inkwire render -size 296x128 \
  -asset assets/portrait.png=photos/portrait.png \
  -asset assets/chart.svg=charts/chart.svg page.html
```

HTTP uses multipart: `page` is the HTML document; local resources are file parts whose field names
exactly match `src`, including directory prefixes. Remote URLs need no file part. `link` `href` follows
the same rule, and a stylesheet may be uploaded as its own part.

## Warnings

Markup may emit: `text-clipped`, `layout-overflow`, `empty-layout`, `missing-runes`, `size-mismatch`,
`unsupported-ink`, `unsupported-declaration`, `unsupported-at-rule`, `unresolved-drawing`,
`unresolved-image`, `no-stylesheet`, `unresolved-stylesheet`, `duplicate-stylesheet`,
`over-constrained`, `unsupported-selector`, `unreadable-rule`, and `substituted-font-size`.
