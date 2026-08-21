// Package panel names one e-paper panel independently of the family whose
// firmware drives it, and is the one place a rendered scene becomes bytes for
// a particular panel.
//
// The two families describe a panel with unrelated types that carry the same
// facts. gicisky.Profile and nrfepd.Model both have an id, a name, a size, an
// ink set and a record of whether the entry has been checked against hardware;
// nothing joined them. So every caller that wanted "a panel, whichever family
// it came from" wrote the join again, and every caller that wanted "lay this
// document out for that panel and pack it" wrote those four lines again, once
// per family per caller, four times over.
//
// Lifetime is what keeps this out of tag.Found rather than on it. A Gicisky
// tag says which panel it has in its advertisement, so the panel is known
// before anything connects; an EPD-nRF5 tag keeps the model in the firmware's
// own config, so it is not known until the conversation is already under way.
// A panel is a different thing from the tag carrying it, and it arrives at a
// different moment.
package panel

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/tag"
)

// Panel is one panel and the family that describes it. Exactly one of the two
// description fields is meaningful; Family says which, the same way tag.Found
// carries a device.
type Panel struct {
	Family  string
	Gicisky gicisky.Profile
	NRFEPD  nrfepd.Model
}

// OfGicisky names the panel a Gicisky tag advertised.
func OfGicisky(profile gicisky.Profile) Panel {
	return Panel{Family: tag.Gicisky, Gicisky: profile}
}

// OfNRFEPD names the panel an EPD-nRF5 tag reported once connected.
func OfNRFEPD(model nrfepd.Model) Panel {
	return Panel{Family: tag.NRFEPD, NRFEPD: model}
}

// Size is the panel's own width and height, which is the size a document has
// to be laid out for and the size its encoder will insist on.
func (p Panel) Size() image.Point {
	if p.Family == tag.NRFEPD {
		return image.Pt(p.NRFEPD.Width, p.NRFEPD.Height)
	}
	return image.Pt(p.Gicisky.Width, p.Gicisky.Height)
}

// Name is what this build calls the panel. The two families keep it under
// different field names for no reason either of them chose.
func (p Panel) Name() string {
	if p.Family == tag.NRFEPD {
		return p.NRFEPD.Name
	}
	return p.Gicisky.Model
}

// String names the panel, its size and its inks, and says when the entry has
// only been transcribed rather than confirmed against the panel itself.
func (p Panel) String() string {
	if p.Family == tag.NRFEPD {
		return p.NRFEPD.String()
	}
	return p.Gicisky.String()
}

// hasColour reports whether the panel has a plane for anything but black.
func (p Panel) hasColour() bool {
	if p.Family == tag.NRFEPD {
		return p.NRFEPD.Palette != nrfepd.PaletteBW
	}
	return p.Gicisky.Palette != gicisky.PaletteBW
}

// Page is one scene packed for one panel.
//
// The two families disagree about the shape of the answer as well as its
// contents: Gicisky takes a single buffer, while EPD-nRF5 takes a black plane
// and, on a colour panel, a second one. Which fields are set follows the
// panel's family, so a caller that knows the family reads the fields it knows
// and a caller that does not still gets a byte count out of Len.
type Page struct {
	Bytes         []byte
	Black, Colour []byte
}

// Len is how many bytes the page takes on the wire, whichever shape it is in.
func (p Page) Len() int { return len(p.Bytes) + len(p.Black) + len(p.Colour) }

// Render lays a document out for one panel and packs the result for it.
//
// The two steps are one function because they were four copies of the same two
// steps, one per family in the command line and one per family in the service.
// They belong together for a better reason than that, though: the size a
// document is laid out for and the size the encoder will accept are the same
// number, and after this nothing is in a position to disagree about it.
//
// The Result comes back on the encode failure as well as on success, and that
// is the point of returning it beside the error rather than instead of it. A
// page that draws and then will not pack — red ink on a panel with no colour
// plane is the usual way — is a failure the caller can explain, but only with
// the report in hand. A nil Frame means the layout itself failed and there is
// nothing to report.
func Render(document compose.Document, p Panel) (scene.Result, Page, error) {
	result, err := scene.RenderForSize(document, p.Size())
	if err != nil {
		return scene.Result{}, Page{}, err
	}
	page, err := p.encode(result.Frame, result.Orientation)
	if err != nil {
		return result, Page{}, err
	}
	return result, page, nil
}

func (p Panel) encode(frame *display.Frame, orientation display.Orientation) (Page, error) {
	switch p.Family {
	case tag.NRFEPD:
		// Orientation is not passed on: this family is written the way the
		// page was drawn, and RenderForSize has already turned a portrait
		// document into a frame of the panel's own shape.
		black, colour, err := display.EncodeNRFEPD(frame, p.hasColour())
		if err != nil {
			return Page{}, err
		}
		return Page{Black: black, Colour: colour}, nil
	case tag.Gicisky:
		payload, err := gicisky.EncodeOriented(frame, orientation, p.Gicisky)
		if err != nil {
			return Page{}, err
		}
		return Page{Bytes: payload}, nil
	}
	return Page{}, fmt.Errorf("unknown family %q", p.Family)
}
