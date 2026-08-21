package gicisky

import "fmt"

// The model table below is ported from the Home Assistant Gicisky integration,
// https://github.com/eigger/hass-gicisky, used under its MIT licence:
//
//	MIT License
//	Copyright (c) 2025 eigger
//
// Only the 2.9" BWR entry (0x0033) has been checked against hardware here.
// Every other row is taken on that project's authority, which is why Verified
// exists and why an unverified panel is reported rather than silently driven.

// Palette is the ink set a panel can actually show. It is independent of size:
// the same 2.9" 296x128 panel ships in all three.
type Palette uint8

const (
	PaletteBW Palette = iota
	PaletteBWR
	PaletteBWRY
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

// Profile is one model's fixed properties. Width and Height describe the
// visual page; Rotation, MirrorX and MirrorY describe how that page is
// transformed on its way into the panel's own frame buffer, and Compression
// which packing the firmware expects.
//
// The transform and packing fields are recorded now because they come from the
// same table and re-deriving them later would mean reading a second project
// again. Nothing encodes with them yet, so they are not reported over the API:
// a field no caller consumes is a field that drifts.
type Profile struct {
	ID       uint16
	Model    string
	Width    int
	Height   int
	Palette  Palette
	TFT      bool
	Verified bool

	Rotation        int
	MirrorX         bool
	MirrorY         bool
	Compression     bool
	Compression2    bool
	InvertLuminance bool
	FourColor       bool
	MaxVoltage      float64
	MinVoltage      float64
}

func (p Profile) Pixels() int { return p.Width * p.Height }

// String names the panel and says whether its dimensions are known or merely
// transcribed, in the same shape nrfepd.Model uses. That qualifier belongs
// wherever the panel is named: the size is what an unverified entry would have
// wrong, and a page of the wrong size is not something the result shows you.
func (p Profile) String() string {
	text := fmt.Sprintf("%s %dx%d %s", p.Model, p.Width, p.Height, p.Palette)
	if !p.Verified {
		text += " (unverified)"
	}
	return text
}

// Upload is the packing this profile implies, which the uploader has to be
// told separately because it works on a payload rather than on a panel.
func (p Profile) Upload() UploadOptions {
	return UploadOptions{Compression2: p.Compression2}
}

// profiles is keyed by the 14-bit id every tag advertises. Absence means this
// build does not know the panel, not that the panel does not exist: the
// upstream table lists many more ids than it implements entries for.
var profiles = map[uint16]Profile{
	0x00A0: {ID: 0x00A0, Model: `TFT 2.1" BW`, Width: 250, Height: 132, Palette: PaletteBW, TFT: true,
		Rotation: 90, MirrorX: true, MaxVoltage: 2.9, MinVoltage: 2.2},
	0x000B: {ID: 0x000B, Model: `EPD 2.1" BWR`, Width: 212, Height: 104, Palette: PaletteBWR,
		Rotation: 270, MirrorX: true, MaxVoltage: 2.9, MinVoltage: 2.2},
	0x010B: {ID: 0x010B, Model: `EPD 2.1" BWR`, Width: 250, Height: 128, Palette: PaletteBWR,
		Rotation: 270, MirrorX: true, MaxVoltage: 2.9, MinVoltage: 2.2},
	0x0028: {ID: 0x0028, Model: `EPD 2.9" BW`, Width: 296, Height: 128, Palette: PaletteBW,
		Rotation: 90, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x0033: {ID: 0x0033, Model: `EPD 2.9" BWR`, Width: 296, Height: 128, Palette: PaletteBWR, Verified: true,
		Rotation: 90, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x002E: {ID: 0x002E, Model: `EPD 2.9" BWRY`, Width: 296, Height: 128, Palette: PaletteBWRY,
		Rotation: 90, FourColor: true, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x022B: {ID: 0x022B, Model: `EPD 3.7" BWR`, Width: 240, Height: 416, Palette: PaletteBWR,
		Rotation: 180, MirrorX: true, Compression: true, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x004B: {ID: 0x004B, Model: `EPD 4.2" BWR`, Width: 400, Height: 300, Palette: PaletteBWR,
		MaxVoltage: 3.0, MinVoltage: 2.2},
	0x004E: {ID: 0x004E, Model: `EPD 4.2" BWRY`, Width: 400, Height: 300, Palette: PaletteBWRY,
		FourColor: true, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x012B: {ID: 0x012B, Model: `EPD 7.5" BWR`, Width: 800, Height: 480, Palette: PaletteBWR,
		MirrorY: true, InvertLuminance: true, Compression2: true, MaxVoltage: 3.0, MinVoltage: 2.2},
	0x008B: {ID: 0x008B, Model: `EPD 10.2" BWR`, Width: 960, Height: 640, Palette: PaletteBWR,
		Compression2: true, MaxVoltage: 3.2, MinVoltage: 2.2},
}

// LookupProfile resolves an advertised id, applying the one firmware-specific
// correction the upstream table carries: an older 7.5" firmware uses the
// earlier compression scheme.
func LookupProfile(id, firmware uint16) (Profile, bool) {
	profile, ok := profiles[id]
	if !ok {
		return Profile{}, false
	}
	if id == 0x012B && firmware == 0x8101 {
		profile.Compression, profile.Compression2 = true, false
	}
	return profile, true
}

// KnownProfiles returns every profile this build can identify, ordered by id so
// that listings and tests are stable.
func KnownProfiles() []Profile {
	ordered := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		ordered = append(ordered, profile)
	}
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].ID < ordered[j-1].ID; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	return ordered
}
