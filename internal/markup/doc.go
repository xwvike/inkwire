// Package markup compiles a subset of HTML and CSS into a scene document, so
// a page can be written the way a page is usually written and still reach the
// panel through the renderer's shared internal representation.
//
// # What it produces, and what it does not do
//
// A page compiles to bytes for the internal scene decoder. That is the whole
// of this package's output: it does not lay anything out,
// measure a glyph, open a picture or build a node the drawing model would
// recognise, and it has no way of reaching a panel except by handing those
// bytes to the decoder everything else goes through.
//
// The division is what keeps a second way of writing a page from becoming a
// second renderer. Every check the decoder makes — the pixel limit on an
// image, the two formats it will decode, the confinement of a path, the
// refusal of a field nobody implements — a page is held to, because there is
// nothing else reading it. An earlier version of this package built the nodes
// itself and had its own picture loader, which was ten lines against the
// decoder's fifty-five, and the difference was invisible until it was a hole.
//
// Tests render the example pages through this compiler and compare their
// frames with checked-in references.
//
// # What this describes, and what it does not
//
// A page here is boxes, text, pictures and where they go. That is what a
// stylesheet is for and it is the whole of this package's job.
//
// It is not for geometry. CSS has no vocabulary for an arc, a polygon or a
// repeating pattern, and giving it one would mean inventing a dialect that
// looks like CSS and is not. SVG is that vocabulary, already standard and
// already what every drawing tool exports, and it goes in the page:
//
//	<svg viewBox="0 0 214 74"><polyline points="0,8 6,2 …"/></svg>
//	<img src="plot.svg">
//
// which is how the intraday chart in examples/markup_capabilities is written: a layout in
// markup with ninety-six polyline points handed over, since nobody writes
// those by hand and whatever produced the series produced them too. An
// external drawing is compiled in place, so what leaves here is one
// self-contained description rather than a description and a promise.
//
// There was a scene element here once, which embedded the internal scene
// representation in a page. It was a tag nobody else has, describing geometry
// in a format nobody else writes, and SVG turned out to reach every drawing
// node the renderer needs - so it went, and with it the last thing in this
// package that a browser would not recognise.
//
// The division is enforced rather than described. internal/compose carries a
// table of which nodes belong to a page and which to a drawing, and a test
// walks this package's source to check it only builds the first kind. It is
// there because the two formats drifted apart once already without anything
// noticing.
//
// It is not a browser and does not try to be. The panel settles most of the
// question of what to support: with four inks and no greys there is nothing
// for opacity, gradients, shadows or antialiasing to do; with bitmap strikes
// there is no font weight, no italic and no continuous font size; with one
// static frame there is nothing to animate; and with no scrolling there is
// nowhere for overflow to go. What remains is the box model, flex, grid and
// text, which is sixty-odd properties. Which ones exactly is written down in
// the Markup references, and a test derives the list from the switch in style.go
// that the document cannot drift from what is implemented. A third copy here
// would be a third thing to keep in step, so there is not one.
//
// # The two rules
//
// A subset is usable rather than a trap when it never pretends. Every
// declaration is either applied or reported: an unknown property, an
// unsupported value, a colour that is not one of the inks, a selector this
// build cannot read, a rule with a syntax error in it, and a font or a size
// nobody bundled all produce a warning that names what it was.
//
// And nothing an author writes may stop a page being drawn. A stylesheet
// copied from anywhere carries ::placeholder, :is(), a vendor's own
// pseudo-element and a font this build has never heard of, and each of those
// used to cost the whole page. An author who has to learn the subset before
// they can write anything has been handed the cost this package exists to
// remove, so what cannot be honoured is skipped or settled and said, and the
// rest of the page is drawn. Two properties cannot be skipped, because text
// has to have a size and a family: those settle on the nearest thing this
// build has, and the document says which.
//
// The second rule has a third part, which is that the compiler must come back.
// A repeat count nobody meant and a custom property that refers to itself are
// both a stylesheet stopping a program, and on a service they are worse than a
// wrong picture. Both are bounded, and the test that holds them there is
// written with a timeout, because a loop that does not terminate cannot be
// caught by looking at its answer.
//
// # Translation notes
//
// A margin needs different treatment on each axis. Along the container's, it
// is a fixed gap beside the item, which a spacer expresses exactly; across it,
// the item is inset instead, which is what wrapping it in padding does.
package markup
