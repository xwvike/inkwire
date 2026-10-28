# Markup Manual

The binary carries this manual: `inkwire docs` prints it.

Markup uses HTML for content, CSS for layout and paint, and SVG for geometry. `render`, `measure`,
and HTTP `render` require `size` or `panel`; `push` obtains the panel size and inks from the target
device. The page is the body, and all of it is drawn, as in a browser. A body that holds one element
and nothing else, such as the usual `<div class="page">`, is drawn from that element unless a rule
styles the body itself; either way, a body that states neither an `orientation` nor a pixel size
takes both from that element. The root element below is the one the page is drawn from.

The root element may omit `width` and `height`: they affect CSS layout only and do not select
the render target. Root `orientation` accepts `landscape`, `portrait-cw`, and `portrait-ccw`; the
default is `landscape`.

## Supported CSS properties

An omitted value is not implemented and produces `unsupported-declaration`. Shorthands retain CSS
syntax only for the subproperties listed here.

| Category | Properties | Accepted values or syntax | Notes |
|---|---|---|---|
| Box | `display` | `block`, `list-item`, `inline`, `inline-block`, `flex`, `inline-flex`, `grid`, `inline-grid`, `contents`, `none` | `inline-*` creates the corresponding atomic box; `contents` creates no box of its own; `list-item` is a block with a marker, and is what `li` is |
| Size | `width`, `height` | `px`, `%`, `calc()`, `auto`, `fit-content` | Percentages resolve against the containing block; a `fit-content` box is as wide (or tall) as its content and is not stretched by its container; `max-content` draws as `fit-content` and warns, and so does `min-content` |
| Size | `min-width`, `min-height`, `flex-basis` | `px`, `%`, `calc()`, `auto` | `flex-basis` supplies the flex base size; as in a browser, a flex item's `min-width`/`min-height` starts at `auto` and it does not shrink below its longest word (or its content's height in a column) unless that is set to `0` or it has `overflow: hidden`; `min-content` is the same as `auto` |
| Size | `max-width`, `max-height` | `px`, `%`, `calc()`, `none` | `none` leaves the box unbounded |
| Ratio | `aspect-ratio` | `number` or `number / number` | Both numbers must be positive; the size not given is worked out from the one that is, or from the width a block fills, and with both given it is ignored |
| Box | `box-sizing` | `content-box`, `border-box` | Applies to `width`, `height`, `min-*`, `max-*`, and `flex-basis` |
| Padding | `padding`, `padding-top`, `padding-right`, `padding-bottom`, `padding-left` | Non-negative `px`, `%`, `calc()` | Percentages resolve against the containing block's inline size and settle to whole pixels |
| Margin | `margin`, `margin-top`, `margin-right`, `margin-bottom`, `margin-left` | `px`, `%`, `calc()`, `auto`; may be negative | `auto` takes a flex line's free space before `justify-content` and places flex and grid items across; `margin: 0 auto` centres a block with a width; percentages use the containing block's inline size; in block flow, vertical margins collapse as in a browser (see below) |
| Flex | `flex` | `none`, `auto`, `<grow>`, `<grow> <shrink>`, `<grow> <shrink> <basis>` | A one-number form uses `0%` basis; `<basis>` accepts `auto`, `px`, `%`, `calc()` |
| Flex | `flex-direction` | `row`, `row-reverse`, `column`, `column-reverse` | A reversed line places its items from the far end; `justify-content: start`, `end`, `left` and `right` keep naming the line's own ends, as in a browser |
| Flex | `flex-wrap` | `nowrap`, `wrap` | Items that do not fit go on further lines, as in a browser: each line is flexed on its own, `row-gap` (`column-gap` in a column) separates the lines, and `align-content` places them; `wrap-reverse` draws as `wrap` and warns |
| Flex | `flex-flow` | `row`, `row-reverse`, `column`, `column-reverse`, `nowrap`, `wrap` | A direction and a wrap, in either order |
| Flex | `flex-grow`, `flex-shrink` | Non-negative numbers | The initial `flex-shrink` is `1` |
| Gap | `gap`, `row-gap`, `column-gap` | `normal` (no gap), non-negative `px`, `%`, `calc()` | `row-gap` percentages resolve against the containing block's block size, `column-gap` against its inline size; the first `gap` value is row-gap and the second is column-gap; values settle to whole pixels |
| Alignment | `align-items`, `align-self`, `justify-items`, `justify-self` | `stretch`, `flex-start`, `start`, `center`, `flex-end`, `end`; `align-self` and `justify-self` also accept `auto` | `justify-*` uses the same alignment set |
| Alignment | `justify-content` | `flex-start`, `start`, `normal`, `center`, `flex-end`, `end`, `space-between`, `space-around`, `space-evenly` | Shares only the space nothing grew into; on a grid it places the columns, and `normal` stretches `auto` columns to fill instead |
| Alignment | `align-content` | `flex-start`, `start`, `normal`, `stretch`, `center`, `flex-end`, `end`, `space-between`, `space-around`, `space-evenly` | Places a grid's rows as `justify-content` places its columns, and the lines of a wrapping flex box; `normal` and `stretch` stretch them into the room left; a flex box that does not wrap has one line and ignores it, as in a browser |
| Alignment | `place-items`, `place-self` | `stretch`, `flex-start`, `start`, `center`, `flex-end`, `end`; `place-self` also accepts `auto` | The `align-*` value, then the `justify-*` value; one value sets both |
| Alignment | `place-content` | `flex-start`, `start`, `normal`, `stretch`, `center`, `flex-end`, `end`, `space-between`, `space-around`, `space-evenly` | `align-content`, then `justify-content`; one value sets both |
| Grid | `grid-template-columns`, `grid-template-rows` | `px`, `%`, `calc()`, positive `fr`, `auto`, `min-content`, `max-content`, `repeat()` | `repeat()` is limited to 400 tracks |
| Grid | `grid-auto-columns`, `grid-auto-rows` | `px`, `%`, `calc()`, positive `fr`, `auto`, `min-content`, `max-content`; one size or several | Sizes the tracks a grid adds past the ones it states, taking several sizes in turn, as in a browser |
| Grid | `grid-column`, `grid-row` | `auto`, a line number, `span N`, `N / M`, `N / span M` | Line numbers and spans are positive integers |
| Position | `position` | `static`, `relative`, `absolute` | `fixed` and `sticky` are not implemented |
| Position | `top`, `right`, `bottom`, `left`, `inset` | `px`, `%`, `calc()`, `auto` | Percentages resolve against the positioned containing block |
| Position | `z-index` | Number | Changes paint order only for positioned elements |
| Colour | `color` | `black`, `white`, `red`, `yellow`, written by name, in hexadecimal, or as `rgb()`, `rgba()`, `hsl()`, `hsla()`, `oklab()` or `oklch()` | A colour within two steps of an ink in every channel, as an `oklch()` or `hsl()` form of one often comes out, is that ink. A grey or tinted grey is drawn black with a warning; transparency, other colour functions, and other named colours are not supported |
| Colour | `background`, `background-color`, `border-color`, `border-top-color`, `border-right-color`, `border-bottom-color`, `border-left-color` | `black`, `white`, `red`, `yellow`, the greys `gray`, `grey`, `silver`, `lightgray`, `lightgrey`, `darkgray`, `darkgrey`, `dimgray`, `dimgrey`, `gainsboro`, `whitesmoke`, and any of these or any grey written in hexadecimal or as `rgb()`, `rgba()`, `hsl()`, `hsla()`, `oklab()` or `oklch()` | A colour within two steps of an ink in every channel is that ink. A grey is dithered from black and white. A tinted grey, whose red, green and blue differ by at most 48, such as `#6b7280` or Tailwind 4's `oklch(55.1% 0.027 264.364)`, is drawn as the grey of its lightness with a warning. Transparency, other colour functions, and other named colours are not supported; other colours are not approximated |
| Colour | `background`, `background-image` | `none`, or `linear-gradient()` towards `to top`, `to right`, `to bottom`, `to left`, or `0deg`, `90deg`, `180deg`, `270deg` and the same angles in `turn`, `rad`, `grad`; two or more stops in white and one ink or its greys, each with an optional `%` position | The image is painted over `background-color`. Corner directions, other angles, two inks in one gradient, colour hints, two-position stops, length positions, `radial-gradient()`, `conic-gradient()`, `repeating-*-gradient()`, and `url()` warn and drop the declaration |
| Border | `border`, `border-top`, `border-right`, `border-bottom`, `border-left` | One width, line style, and colour in any order | An omitted line style is `none` and draws nothing; an omitted colour, like `currentColor` in any border colour, is the colour of the element's text |
| Border | `border-width`, `border-top-width`, `border-right-width`, `border-bottom-width`, `border-left-width` | Non-negative `px`, a pixel-only `calc()`, or `thin`, `medium`, `thick` (1, 3 and 5px) | Rounded to whole pixels at layout; percentages and multi-value border widths are not implemented |
| Border | `border-style`, `border-top-style`, `border-right-style`, `border-bottom-style`, `border-left-style` | `solid`, `dashed`, `dotted`, `none`, `hidden` | A dotted mark and its gap each equal the line width |
| Border | `border-radius` | One non-negative `px`, `%`, or `calc()` radius for all four corners | Rounded to whole pixels at layout; a percentage is a share of the box, and at `50%` or more the box is drawn as a circle or ellipse; a smaller percentage on a box that is not square would make elliptical corners, which are drawn round at the smaller radius and warn; per-corner and `/` elliptical radii warn |
| Visibility | `visibility` | `visible`, `hidden` | `hidden` keeps the layout box and removes its paint |
| Clipping | `overflow` | `visible`, `hidden`, `clip` | There is no scrolling |
| Clipping | `clip-path` | `none`, `inset()`, `circle()`, `ellipse()`, `polygon()` | Function arguments accept `px`, `%`, and `calc()`; `inset()` has one radius at most |
| Transform | `transform` | `none`, `scale()`, `rotate()` | Only `scale` and `rotate` functions are implemented |
| Transform | `rotate` | `deg`, `grad`, `rad`, `turn`, or a unitless angle | — |
| Transform | `transform-origin` | One or two keywords, `px`, `%`, or `calc()` | Keywords are `left`, `center`, `right`, `top`, and `bottom` |
| Transform | `scale` | One or two equal integers, each at least 1 | No resampling; fractions and non-uniform pairs are not implemented |
| SVG paint | `fill`, `stroke` | Supported inks and greys, `none`, `transparent` | These properties inherit; CSS overrides an SVG presentation attribute on inline SVG |
| SVG paint | `stroke-width` | Non-negative `px`, a pixel-only `calc()`, or an SVG unitless number | Rounded to whole pixels at layout; values below one pixel draw at one pixel and warn |
| SVG paint | `stroke-dasharray`, `stroke-dashoffset` | Space- or comma-separated integer pixels | — |
| SVG paint | `stroke-linecap` | `butt`, `round`, `square` | Applies to open SVG lines, polylines and paths; SVG defaults to `butt` |
| SVG paint | `stroke-linejoin` | `miter`, `round`, `bevel` | Applies where adjacent SVG path segments meet; SVG defaults to `miter` |
| Font | `font` | `size[/line-height] family` | Only size, line-height, and family are read; style and weight fields warn |
| Font | `font-family` | `ui`, `hzk`, `monaco`, or a family stack | Families are tried per glyph in declaration order; unknown names are skipped, and an entirely unavailable stack uses the default with a warning |
| Font | `font-size` | `px`, a pixel-only `calc()`, or a percentage of the parent's font size | The nearest bitmap strike is used and `substituted-font-size` is emitted |
| Text | `line-height` | `normal`, `px`, a pixel-only `calc()`, a unitless ratio, or a percentage of the font size | Unitless values resolve against the element's own size; percentages inherit their computed length |
| Text | `text-align` | `left`, `start`, `center`, `right`, `end` | — |
| Text | `vertical-align` | `baseline`, `top`, `middle`, `bottom` | Inline-level only; the initial value is `baseline` and it is not inherited |
| Text | `white-space` | `normal`, `nowrap`, `pre`, `pre-wrap` | Lines wrap between words as a browser wraps them: after spaces and hyphens, and between Chinese, Japanese and Korean characters except before closing punctuation |
| Text | `word-break` | `normal`, `break-all`, `break-word` | `break-all` lets a line end between any two characters; `keep-all` warns and acts as `normal` |
| Text | `overflow-wrap`, `word-wrap` | `normal`, `break-word`, `anywhere` | A word longer than its line is always broken where it stops fitting, so every value draws the same |
| Text | `font-weight` | `normal`, `bold`, `bolder`, `lighter`, or a number from 1 to 1000 | 600 and above draw bold: each glyph is drawn again a pixel to its right and takes a pixel more room, since the bundled faces have one weight |
| Text | `font-style` | `normal` | `italic` and `oblique` warn; the bundled faces are upright only |
| Text | `letter-spacing` | `normal`, `px`, a pixel-only `calc()` | Added after every character, and may be negative |
| Text | `text-decoration`, `text-decoration-line` | `none`, `underline`, `line-through`, `overline` | Solid lines in the text's colour, a pixel thick for every 16 of the font's height; as in CSS, the lines run through descendants' text, a descendant's `none` cannot remove them, and they stop at inline-blocks and positioned boxes; a style, colour, or thickness in the shorthand warns |
| Text | `text-decoration-style` | `solid` | Other styles warn |
| Text | `text-decoration-color` | `currentColor` | A decoration is drawn in its text's colour; other colours warn |
| Text | `text-decoration-thickness`, `text-underline-offset` | `auto` | Other values warn |
| Text | `text-overflow` | `clip`, `ellipsis` | `ellipsis` ends a line cut off by `overflow: hidden` with an ellipsis, as in a browser: "…" after Chinese, and three full stops after Latin text, whose bundled faces have no "…"; on text with no elements inside it; elsewhere it warns |
| List | `list-style`, `list-style-type` | `disc`, `circle`, `square`, `none`, `decimal`, `decimal-leading-zero`, `lower-alpha`, `upper-alpha`, `lower-latin`, `upper-latin`, `lower-roman`, `upper-roman`, `lower-greek`, `cjk-decimal`, `simp-chinese-informal`, `trad-chinese-informal`, `cjk-ideographic`, `disclosure-open`, `disclosure-closed`, or a string; the shorthand also takes a position | A marker is drawn for each `display: list-item` box in its colour and font, numbered from the `ol`'s `start`, `reversed` and each `li`'s `value`; a type with no counter style here is drawn as `decimal` and warns |
| List | `list-style-position` | `outside`, `inside` | `outside` hangs the marker beside the first line, outside the item's border; `inside` puts it first on the line, and warns and is drawn outside when the item starts with a box rather than text |
| List | `list-style-image` | `none` | An image warns, and the type's marker is drawn |
| Image | `object-fit` | `fill`, `contain`, `cover` | Applies to `img` and external drawings inside their boxes |

Every implemented property accepts `inherit`, `initial`, `unset`, and `revert`. The properties that
inherit by default are `color`, `fill`, `stroke`, `stroke-width`, `stroke-dasharray`,
`stroke-dashoffset`, `stroke-linecap`, `stroke-linejoin`, `font`, `font-family`, `font-size`, `font-weight`, `font-style`,
`letter-spacing`, `line-height`, `text-align`, `white-space`, `word-break`, `overflow-wrap`, `list-style` and its longhands, and `visibility`; `vertical-align` is not inherited. Custom properties named `--name` may be declared,
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

- Layout: `float`, `clear`, `order`, `grid-template-areas`, `grid-auto-flow`, `table-layout`.
- Position and effects: `position: fixed`, `position: sticky`, `opacity`, `filter`, `box-shadow`,
  `text-shadow`, `mix-blend-mode`, `background-repeat`, `background-size`.
- Text: `font-variant`, `text-indent`, `word-spacing`, `text-transform`, `hyphens`.
- Images: `object-position`.
- Colour and transform values: `hwb()`, `lab()`, `lch()`, `color()`, `color-mix()`, and other colour functions not listed above, `radial-gradient()`, `conic-gradient()`, repeating gradients, alpha; transform functions
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
- Vertical margins in block flow collapse as in a browser: margins that meet come to the largest
  positive one plus the most negative one, through empty boxes, and a first or last child's margin
  leaves its parent where no padding, border or (below) height of the parent's own is in the way.
  Flex and grid items, inline-blocks, positioned boxes, `overflow: hidden` boxes and the page keep
  their children's margins inside. A percentage margin is not collapsed; where it meets another it
  warns, and the two are added.
- Elements look as a browser's own stylesheet draws them, and every rule the page writes outranks it,
  whatever its selector: `h1` to `h6` are bold, at 2, 1.5, 1.17, 1, 0.83 and 0.67 times their
  parent's font size, settled quietly on the nearest size the face has; `p`, `ul`, `ol`, `dl`,
  `menu`, `pre` and the headings have a browser's top and bottom margins; `blockquote` and `figure`
  are inset 40px on both sides and `dd` on the left; `ul`, `ol` and `menu` are indented 40px and
  their items marked, a list in a list with no margins and a circle, then a square; `ol`'s and
  `li`'s `type` attribute names the marker; `b`, `strong` and `th` are bold, `th`, `caption` and
  `center` centred; `u`, `ins` and `a` with an `href` underlined; `s`, `del` and `strike` struck
  through; `small` is 0.83 times; `hr` is a 1px line in the colour of the text; and `pre` keeps its
  spaces. Link colours are not drawn. Where a browser would draw something this does not,
  `unsupported-element` says so, and goes quiet once the page says what it wants instead: form
  controls, `canvas`, `video`, `audio`, `iframe`, `object` and `embed` are not drawn; a `button`
  with no border or background draws no box; a `table` without a `display` is laid out as blocks;
  and `em`, `i`, `cite`, `var`, `dfn` and `address` are upright. An italic element's name is said
  once per page.
- Inline content, images, and inline SVG share line boxes. `vertical-align` applies only to inline-level
  boxes; a declaration on a block, flex item, or grid item warns. For a single text line, a line-height
  equal to the box height provides its vertical alignment.
- Stylesheet order is CLI stylesheet, page `<style>`, then linked stylesheets in document order.
  Normal declarations compare specificity and source order; inline style outranks normal selectors and
  `!important` outranks normal declarations. Custom properties cascade and inherit by the same rules;
  an undefined or cyclic `var()` drops the declaration that uses it.
- Padding and margins resolve percentages against the containing block's inline size; `row-gap` uses
  its block size and `column-gap` its inline size. All settle to whole pixels at layout time. Border
  widths and font sizes accept pixels or pixel-only `calc()`. `calc()` adds and subtracts lengths,
  multiplies and divides them by numbers, and nests brackets; `min()`, `max()`, and `clamp()` work
  wherever `calc()` does, as long as none of their arguments is a percentage.
- Wherever a pixel length is accepted, including inside `calc()` and custom properties, a length may
  also be written in `em` (the element's font size; in `font-size`, the parent's), `rem` (the root
  element's font size, 12px unless the root sets one), `vw`, `vh`, `vmin`, `vmax` (shares of the root
  element's `width` and `height`, which must both be given in pixels, or the declaration warns and is
  dropped), or `pt`, `pc`, `in`, `cm`, `mm`, `Q` at 96px to the inch. `ch` and `ex` warn.
- Fonts come from bitmap families bundled at build time. An unknown family falls back to the default;
  a family stack falls back per glyph; an unavailable size falls back to the nearest strike. The
  bundled faces have one weight, and bold is drawn from it.
- A grey is black dithered over white in a 4x4 ordered pattern, at the nearest of sixteen coverages:
  `#808080` is 8/16, and a grey that rounds to none or all of them is white or black. The pattern is
  keyed to frame coordinates, so neighbouring grey areas meet without a seam and a page draws the
  same pixels every time. Text drawn over a grey clears a 1px ring of the grey around its glyphs.
  Text itself cannot be grey: a dithered 1px stroke loses half its pixels. A 1px grey border or
  stroke has the same problem and reads as a dotted line. `compile` writes a grey as `black 8/16`.
  A tinted grey takes the lightness its red, green and blue give it (0.2126, 0.7152, 0.0722), and
  loses its tint. A colour with any transparency — `#0008`, `rgb(0 0 0 / 50%)` — is refused,
  because what shows through it is not known when it is drawn; write the colour it should be.
- A `linear-gradient()` is drawn with the same dither: each pixel takes the coverage the stops give
  its place along the box, rounded to the nearest of seventeen, so a gradient is at most seventeen
  bands and the two end bands are half as wide as the rest. Stops are placed as CSS places them,
  and may sit outside 0%–100%. A gradient runs from white to one ink — black through the greys, or
  red or yellow through their tints — because a pixel cannot be part one ink and part another; a
  rounded box clips its gradient to the same corners as a fill. A gradient the panel cannot draw is
  refused with a warning and the declaration dropped, as a browser drops one it cannot read, so an
  earlier `background` written as a fallback still applies.
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
directory. The HTTP service fetches a URL only from the public internet — never from the machine it
runs on or the network it is on — while the command line fetches from anywhere. An SVG named by URL
is held to the same rule. The service connects directly rather than through the system proxy, so a
picture reachable only through a proxy is not drawn by it, and it refuses 198.18.0.0/15, the
addresses a proxy's fake-IP DNS hands out: under such DNS it can fetch nothing. A picture that cannot
be read, fetched or decoded leaves an empty box and an `unresolved-image` warning.

An `<img>` or inline `<svg>` is a box like any other: its padding, background and border are drawn
round the picture, and `border-radius` clips the picture to its corners, so `border-radius: 50%`
makes a round portrait.

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
`over-constrained`, `unsupported-selector`, `unreadable-rule`, `substituted-font-size`, `approximate-corner`, and `unsupported-element`.
