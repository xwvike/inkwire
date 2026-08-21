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
	"strconv"
	"strings"

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

// All lists every panel this build knows, whichever family describes it.
//
// Both families keep a catalogue and neither can see the other's, so counting
// the panels this build knows meant walking two lists and joining them at
// whichever place wanted the answer. nrfepd.Models was deleted once as a
// function nothing called and added back the next day for exactly this reason:
// its only caller was a documentation test, which had to write the join itself
// because there was nowhere else for it to live.
func All() []Panel {
	profiles, models := gicisky.KnownProfiles(), nrfepd.Models()
	panels := make([]Panel, 0, len(profiles)+len(models))
	for _, profile := range profiles {
		panels = append(panels, OfGicisky(profile))
	}
	for _, model := range models {
		panels = append(panels, OfNRFEPD(model))
	}
	return panels
}

// ByKey finds a panel named the way a person has to name one when there is no
// tag in the room to ask.
//
// The family has to be stated. It is not a formality: the two catalogues are
// numbered independently, so 0x33 means one panel to one firmware and another
// to the other, and there is no shape to an id that says which it is.
//
// Gicisky panels can only be asked for by id, because their names do not
// identify them — 0x000B and 0x010B are both called EPD 2.1" BWR and they are
// different sizes. An EPD-nRF5 panel takes either the firmware's name for it
// or its id, since those names are unique and are what the reference project
// and its web tool put in front of somebody.
func ByKey(key string) (Panel, error) {
	family, rest, ok := strings.Cut(key, ":")
	if !ok {
		return Panel{}, fmt.Errorf("panel %q does not say which family: write %s:ID or %s:NAME", key, tag.Gicisky, tag.NRFEPD)
	}
	family = strings.ToLower(strings.TrimSpace(family))
	rest = strings.TrimSpace(rest)
	switch family {
	case tag.Gicisky:
		id, err := parseHex(rest, 16)
		if err != nil {
			return Panel{}, fmt.Errorf("panel %q: %w, and a Gicisky panel can only be asked for by id because its name does not identify it", key, err)
		}
		profile, known := gicisky.LookupProfile(uint16(id), 0)
		if !known {
			return Panel{}, fmt.Errorf("no Gicisky panel 0x%04X in this build", id)
		}
		return OfGicisky(profile), nil
	case tag.NRFEPD:
		if model, known := nrfepd.LookupModelName(rest); known {
			return OfNRFEPD(model), nil
		}
		id, err := parseHex(rest, 8)
		if err != nil {
			return Panel{}, fmt.Errorf("no EPD-nRF5 panel named %q in this build, and it is not an id either", rest)
		}
		model, known := nrfepd.LookupModel(uint8(id))
		if !known {
			return Panel{}, fmt.Errorf("no EPD-nRF5 panel 0x%02x in this build", id)
		}
		return OfNRFEPD(model), nil
	}
	return Panel{}, fmt.Errorf("unknown family %q: use %s or %s", family, tag.Gicisky, tag.NRFEPD)
}

// ParseSize reads the WxH a page is laid out at when it is a size rather than
// a panel being asked for.
//
// A bare size is not a Panel and deliberately does not become one: nothing
// about 400x300 says which inks are available, so a page laid out at a size
// gets no ink check and should not look as though it had one.
func ParseSize(text string) (image.Point, error) {
	width, height, ok := strings.Cut(strings.ToLower(strings.TrimSpace(text)), "x")
	if !ok {
		return image.Point{}, fmt.Errorf("size %q is not WxH, such as 400x300", text)
	}
	x, errX := strconv.Atoi(strings.TrimSpace(width))
	y, errY := strconv.Atoi(strings.TrimSpace(height))
	if errX != nil || errY != nil || x <= 0 || y <= 0 {
		return image.Point{}, fmt.Errorf("size %q is not WxH with both sides positive, such as 400x300", text)
	}
	return image.Pt(x, y), nil
}

// parseHex reads an id with or without the 0x the tables are written with.
func parseHex(text string, bits int) (uint64, error) {
	digits := strings.TrimPrefix(strings.TrimPrefix(text, "0X"), "0x")
	if digits == "" {
		return 0, fmt.Errorf("%q is not an id", text)
	}
	value, err := strconv.ParseUint(digits, 16, bits)
	if err != nil {
		return 0, fmt.Errorf("%q is not a %d-bit hexadecimal id", text, bits)
	}
	return value, nil
}

// ID is the identifier this build writes for the panel.
//
// The two are different widths because they are different things: a Gicisky
// tag advertises a 14-bit id, and the EPD-nRF5 firmware stores a byte it will
// not let anybody renumber. Writing each to its own width is what keeps one
// family's 0x0033 from reading as the other's 0x33.
func (p Panel) ID() string {
	if p.Family == tag.NRFEPD {
		return fmt.Sprintf("0x%02x", p.NRFEPD.ID)
	}
	return fmt.Sprintf("0x%04X", p.Gicisky.ID)
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

// Palette names the ink set the panel can show.
func (p Panel) Palette() string {
	if p.Family == tag.NRFEPD {
		return p.NRFEPD.Palette.String()
	}
	return p.Gicisky.Palette.String()
}

// Verified reports whether this entry has been checked against the panel
// itself rather than transcribed from another project. What an unverified
// entry would have wrong is the size, and a page of the wrong size is not
// something the result shows you.
func (p Panel) Verified() bool {
	if p.Family == tag.NRFEPD {
		return p.NRFEPD.Verified
	}
	return p.Gicisky.Verified
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
	page, err := p.Encode(result.Frame, result.Orientation)
	if err != nil {
		return result, Page{}, err
	}
	return result, page, nil
}

// Encode packs a frame already drawn at this panel's size.
//
// It is separate from Render for the caller that has a frame rather than a
// document: an example page checks that it still packs for the panel it was
// drawn for, which is how a page that has quietly stopped fitting shows up
// while it is still only a test failure.
func (p Panel) Encode(frame *display.Frame, orientation display.Orientation) (Page, error) {
	switch p.Family {
	case tag.NRFEPD:
		// Asked here rather than left to the driver: the driver only sees a
		// page with a tag already on the other end, and this is reachable
		// without one.
		if err := p.NRFEPD.Packable(); err != nil {
			return Page{}, err
		}
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
