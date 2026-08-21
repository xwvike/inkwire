package nrfepd

import "fmt"

// Palette is the ink set a panel can show.
//
// The values are the firmware's own, from epd_color_t, because they travel to
// the device inside a config write. Naming them again with different numbers
// would be a second table to keep in step for no gain.
type Palette uint8

const (
	PaletteBW   Palette = 1
	PaletteBWR  Palette = 2
	PaletteBWRY Palette = 3
)

func (p Palette) String() string {
	switch p {
	case PaletteBW:
		return "BW"
	case PaletteBWR:
		return "BWR"
	case PaletteBWRY:
		return "BWRY"
	}
	return fmt.Sprintf("Palette(%d)", uint8(p))
}

// Packing is how a page becomes the bytes a panel's RAM wants.
//
// Most panels take two planes, black then colour, as two separate writes. The
// UC8159 parts take one stream instead, two pixels to a byte, with the colour
// chosen per pixel. This is not a detail that can be left until later: the two
// produce different numbers of bytes from the same page, so a panel sent the
// wrong one shows a page that is the wrong size as well as the wrong colour.
type Packing uint8

const (
	// PackingPlanes writes a black plane and, on a colour panel, a second
	// plane for the colour.
	PackingPlanes Packing = iota
	// PackingNibbles writes one stream of four-bit colour codes.
	PackingNibbles
)

// Model is one panel the firmware knows how to drive.
//
// Unlike a Gicisky tag, one of these does not say what it is. The firmware
// keeps the model in its own config and the host is what puts it there, so
// everything here has to be chosen by whoever is sending the page. Getting it
// wrong writes a page of the wrong shape into the panel's RAM.
type Model struct {
	// ID is the value the firmware knows this panel by, and what an init
	// command carries.
	ID uint8
	// Name is the firmware's own identifier for it.
	Name    string
	Driver  string
	Width   int
	Height  int
	Palette Palette
	Packing Packing
	// Verified records whether this entry has been confirmed against the panel
	// itself rather than read off the reference project. One has been. Saying
	// which is the point: a model taken on trust can still be the wrong size,
	// and the way that shows up is a ruined page.
	Verified bool
}

// PlaneSize is the length of one packed plane, or of the whole stream for a
// panel packed in nibbles.
func (m Model) PlaneSize() int {
	if m.Packing == PackingNibbles {
		return m.Width * m.Height / 2
	}
	return (m.Width + 7) / 8 * m.Height
}

// Packable reports whether this build can turn a page into the bytes this
// panel wants.
//
// It is asked in two places that see a panel at different moments: before a
// page is encoded at all, and again when the planes are turned into writes. A
// panel that cannot be packed has to be refused at the first of those, because
// the second is only reached with a tag on the other end of a connection, and
// a page rendered for one of these offline would otherwise come out looking
// like a page.
func (m Model) Packable() error {
	if m.Packing == PackingNibbles {
		return fmt.Errorf("%s packs two pixels to a byte, which is not implemented yet", m.Name)
	}
	return nil
}

// String names the panel and says whether its dimensions are known or merely
// transcribed, because that qualifier belongs everywhere the panel is named:
// the size is what an unverified entry would have wrong, and it is not
// something the resulting page shows you.
func (m Model) String() string {
	text := fmt.Sprintf("%s %dx%d %s", m.Name, m.Width, m.Height, m.Palette)
	if !m.Verified {
		text += " (unverified)"
	}
	return text
}

// models is transcribed from epd_model_id_t and the model tables in UC81xx.c
// and SSD16xx.c of tsl0922/EPD-nRF5.
//
// The IDs are not ours to renumber: the firmware stores one in its config and
// the comment above the enum there says so outright.
var models = []Model{
	{ID: 0x01, Name: "UC8176_420_BW", Driver: "UC8176", Width: 400, Height: 300, Palette: PaletteBW, Packing: PackingPlanes},
	{ID: 0x02, Name: "SSD1619_420_BWR", Driver: "SSD1619", Width: 400, Height: 300, Palette: PaletteBWR, Packing: PackingPlanes},
	// Confirmed on the panel: a 400x300 page reached it whole and drew, over a
	// 244 byte link with compression, on firmware reporting version 0x76.
	{ID: 0x03, Name: "UC8176_420_BWR", Driver: "UC8176", Width: 400, Height: 300, Palette: PaletteBWR, Packing: PackingPlanes, Verified: true},
	{ID: 0x04, Name: "SSD1619_420_BW", Driver: "SSD1619", Width: 400, Height: 300, Palette: PaletteBW, Packing: PackingPlanes},
	{ID: 0x05, Name: "JD79668_420_BWRY", Driver: "JD79668", Width: 400, Height: 300, Palette: PaletteBWRY, Packing: PackingPlanes},
	{ID: 0x06, Name: "UC8179_750_BW", Driver: "UC8179", Width: 800, Height: 480, Palette: PaletteBW, Packing: PackingPlanes},
	{ID: 0x07, Name: "UC8179_750_BWR", Driver: "UC8179", Width: 800, Height: 480, Palette: PaletteBWR, Packing: PackingPlanes},
	{ID: 0x08, Name: "UC8159_750_LOW_BW", Driver: "UC8159", Width: 640, Height: 384, Palette: PaletteBW, Packing: PackingNibbles},
	{ID: 0x09, Name: "UC8159_750_LOW_BWR", Driver: "UC8159", Width: 640, Height: 384, Palette: PaletteBWR, Packing: PackingNibbles},
	{ID: 0x0a, Name: "SSD1677_750_HD_BW", Driver: "SSD1677", Width: 880, Height: 528, Palette: PaletteBW, Packing: PackingPlanes},
	{ID: 0x0b, Name: "SSD1677_750_HD_BWR", Driver: "SSD1677", Width: 880, Height: 528, Palette: PaletteBWR, Packing: PackingPlanes},
	{ID: 0x0c, Name: "JD79665_750_BWRY", Driver: "JD79665", Width: 800, Height: 480, Palette: PaletteBWRY, Packing: PackingPlanes},
	{ID: 0x0d, Name: "JD79665_583_BWRY", Driver: "JD79665", Width: 648, Height: 480, Palette: PaletteBWRY, Packing: PackingPlanes},
	{ID: 0x0e, Name: "UC8159_583_LOW_BWR", Driver: "UC8159", Width: 600, Height: 448, Palette: PaletteBWR, Packing: PackingNibbles},
	{ID: 0x0f, Name: "UC8159_583_LOW_BW", Driver: "UC8159", Width: 600, Height: 448, Palette: PaletteBW, Packing: PackingNibbles},
	{ID: 0x10, Name: "UC8179_583_BWR", Driver: "UC8179", Width: 648, Height: 480, Palette: PaletteBWR, Packing: PackingPlanes},
	{ID: 0x11, Name: "UC8179_583_BW", Driver: "UC8179", Width: 648, Height: 480, Palette: PaletteBW, Packing: PackingPlanes},
}

// Models lists every panel in the table, in ID order. It is how panel.All
// reaches this family, which is what the READMEs' panel tables and the test
// that holds them to this one are counted from.
func Models() []Model { return append([]Model(nil), models...) }

// LookupModel finds a panel by the firmware's ID.
func LookupModel(id uint8) (Model, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

// LookupModelName finds a panel by the firmware's name for it, which is what a
// person reading the reference project or its web tool will have in hand.
func LookupModelName(name string) (Model, bool) {
	for _, model := range models {
		if model.Name == name {
			return model, true
		}
	}
	return Model{}, false
}
