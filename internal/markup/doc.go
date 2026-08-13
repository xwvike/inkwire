// Package markup compiles a subset of HTML and CSS into compose nodes, so a
// page can be written the way a page is usually written and still end up as
// the same drawing commands a scene document produces.
//
// # What this describes, and what it does not
//
// A page here is boxes, text, pictures and where they go. That is what a
// stylesheet is for and it is the whole of this package's job.
//
// It is not for geometry. CSS has no vocabulary for an arc, a polygon or a
// repeating pattern, and giving it one would mean inventing a dialect that
// looks like CSS and is not. Those are described by a scene document, and a
// page embeds one with a scene element:
//
//	<scene src="plot.json"></scene>
//
// which is how the intraday chart in examples/desk is written: a layout in
// markup with ninety-six polyline points handed over, since nobody writes
// those by hand and whatever produced the series produced them too.
//
// The division is enforced rather than described. internal/compose carries a
// table of which nodes belong to a page and which to a drawing, and a test
// walks this package's source to check it only builds the first kind. It is
// there because the two formats drifted apart once already without anything
// noticing.
//
// It is not a browser and does not try to be. The panel settles most of the
// question of what to support: with three inks and no greys there is nothing
// for opacity, gradients, shadows or antialiasing to do; with bitmap strikes
// there is no font weight, no italic and no continuous font size; with one
// static frame there is nothing to animate; and with no scrolling there is
// nowhere for overflow to go. What remains is the box model, flex, and text.
//
// The rule that makes a subset usable rather than a trap is that it never
// pretends. An author writing CSS they know will reach for properties this
// does not implement, and the worst outcome is that some of it quietly does
// nothing. So every declaration is either applied or reported: an unknown
// property, an unsupported value, a colour that is not one of the three inks
// and a font size with no strike all produce a warning that names the element
// and the declaration.
//
// # Supported
//
//	display          block, flex, inline, none
//	flex-direction   row, column
//	flex-basis       <px>
//	flex-grow        <number>
//	flex             <grow> shorthand
//	gap              <px>
//	align-items      stretch, flex-start, center, flex-end
//	padding          <px>, 1 to 4 values, and the four longhands
//	margin           <px>, 1 to 4 values, and the four longhands
//	margin-left      auto, to push a flex item over
//	margin-top       auto, likewise on a column
//	width, height    <px>, <percent> on a block child
//	background       black, white, red
//	color            black, white, red
//	border           <px> solid <ink>
//	border-radius    <px>
//	font-family      ui, hzk, monaco
//	font-size        a size the family has a strike for
//	text-align       left, center, right
//	vertical-align   top, middle, bottom
//
// color, font-family, font-size, text-align and vertical-align inherit;
// everything else does not, which matches CSS.
//
// A margin needs different treatment on each axis. Along the container's, it
// is a fixed gap beside the item, which a spacer expresses exactly; across it,
// the item is inset instead, which is what wrapping it in padding does.
//
// vertical-align is the one property used slightly outside its CSS meaning.
// There it applies to inline boxes and table cells; here it says where text
// sits inside a box taller than itself, which is the table-cell case and the
// property an author reaches for. Text sits at the top without it, as in CSS.
package markup
