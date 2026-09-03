# Markup capabilities

Most pages are 400x300 reference renders. `potrace.html` is 500x500 so the
imported SVG can be inspected at the size it was authored for. The HTML uses
only the CSS and SVG properties listed in `MARKUP.md`.

| Page | Demonstrates |
|---|---|
| `layout.html` | box model, flex, grid, sizing and positioning |
| `inline.html` | inline flow, atomic inline boxes and vertical alignment |
| `paint.html` | inks, borders, clipping, visibility and transforms |
| `svg.html` | SVG primitives, paths, patterns, groups and CSS overrides |
| `resources.html` | relative raster assets, external SVG and `object-fit` |
| `cascade.html` | source order, specificity, `!important` and inheritance |
| `potrace.html` | external SVG with a non-zero transform and viewBox mapping |

Regenerate the PNGs with:

```bash
INKWIRE_UPDATE_REFERENCES=1 go test ./examples/markup_capabilities
```
