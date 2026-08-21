package nrfepd

import (
	"fmt"
	"strconv"
	"strings"
)

// The commands the firmware answers to, from enum EPD_CMDS. Only the ones a
// page needs are here; the rest of the enum sets pins, erases config and
// resets the chip, and none of that belongs on the path that draws a picture.
const (
	cmdInit       = 0x01
	cmdRefresh    = 0x05
	cmdWriteImage = 0x30
)

// The flag bits on a write. Bit 0 chooses the plane, bit 1 says this is the
// first piece of it, and bit 2 says the piece is compressed.
const (
	flagColourPlane = 0x01
	flagBegin       = 0x02
	flagRLE         = 0x04
)

// frameOverhead is the command byte and the flag byte that precede every piece
// of image data, which is what a piece has to leave room for inside the MTU.
const frameOverhead = 2

// Config is what the panel says about itself when it is initialised.
//
// It matters because this family does not advertise what it is. The firmware
// keeps the model in its own flash and hands it back here, so this is the one
// place the panel's size and palette can be learned rather than assumed.
type Config struct {
	MOSIPin, SCLKPin, CSPin, DCPin uint8
	ResetPin, BusyPin, BSPin       uint8
	ModelID                        uint8
	WakeupPin, LEDPin, EnablePin   uint8
	DisplayMode, WeekStart         uint8
}

// configLength is sizeof(epd_config_t). A device may answer with more than
// this: the firmware in the field is ahead of the reference project and has
// grown fields on the end. Trailing bytes are left alone rather than refused,
// because a field this package does not know about is not a field it needs.
const configLength = 13

// ParseConfig reads the configuration blob the firmware returns after an init.
func ParseConfig(data []byte) (Config, error) {
	if len(data) < configLength {
		return Config{}, fmt.Errorf("config is %d bytes, want at least %d", len(data), configLength)
	}
	return Config{
		MOSIPin: data[0], SCLKPin: data[1], CSPin: data[2], DCPin: data[3],
		ResetPin: data[4], BusyPin: data[5], BSPin: data[6],
		ModelID:   data[7],
		WakeupPin: data[8], LEDPin: data[9], EnablePin: data[10],
		DisplayMode: data[11], WeekStart: data[12],
	}, nil
}

// Model finds the panel this configuration describes.
func (c Config) Model() (Model, bool) { return LookupModel(c.ModelID) }

// Link is what the panel says it can carry, which it volunteers as a
// notification after being initialised.
type Link struct {
	MTU int
	RLE bool
}

// ParseNotification reads one of the strings the firmware sends back.
//
// Anything it does not recognise is not an error. The firmware in the field
// sends notifications this package has never heard of — slot counts and a
// session id among them — and a driver that treated an unknown message as a
// failure would refuse to talk to the very devices it is for.
func ParseNotification(message string) (Link, bool) {
	if !strings.HasPrefix(message, "mtu=") {
		return Link{}, false
	}
	// The line is "mtu=244 rle=1": the size first, then whatever else the
	// firmware wants to say about the link.
	fields := strings.Fields(message)
	mtu, err := strconv.Atoi(strings.TrimPrefix(fields[0], "mtu="))
	if err != nil || mtu <= frameOverhead {
		return Link{}, false
	}
	link := Link{MTU: mtu}
	for _, field := range fields[1:] {
		if field == "rle=1" {
			link.RLE = true
		}
	}
	return link, true
}

// IsText tells the firmware's status strings from a configuration blob, which
// arrive on the same characteristic with nothing to label them.
//
// A config cannot be mistaken for text: its first byte is a pin number, and a
// pin on these parts is under 32 or else 0xFF for one that is not wired. Both
// are outside printable ASCII, so a message made entirely of printable bytes
// is a string and nothing else can be.
func IsText(message []byte) bool {
	if len(message) == 0 {
		return false
	}
	for _, b := range message {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

// Plan is the exact sequence of writes that puts one page on one panel.
//
// It is built rather than performed so that it can be checked without a radio.
// The order is the firmware's: fill the panel's RAM one plane at a time, then
// refresh. Nothing is drawn until the refresh, which is what makes a half-sent
// page show as the previous page rather than as half a page.
//
// The init that precedes all this is not here. It has to be sent before a plan
// can be built at all, because what it returns is the panel's own account of
// what it is, and that is what decides the size of everything below.
type Plan struct {
	Frames [][]byte
	// Compressed records whether the image data was sent compressed, because
	// the choice is made per page and it is the first thing worth knowing when
	// a page arrives looking like noise.
	Compressed bool
}

// BuildPlan turns a page's planes into the writes that send it.
//
// colour is nil for a panel without one. link says how much can be carried at
// a time and whether the firmware can decompress; a firmware that cannot gets
// the planes as they are.
func BuildPlan(model Model, black, colour []byte, link Link) (Plan, error) {
	if err := model.Packable(); err != nil {
		return Plan{}, err
	}
	if want := model.PlaneSize(); len(black) != want {
		return Plan{}, fmt.Errorf("black plane is %d bytes, %s wants %d", len(black), model.Name, want)
	}
	if (colour != nil) != (model.Palette != PaletteBW) {
		return Plan{}, fmt.Errorf("%s is %s and was given colour=%v", model.Name, model.Palette, colour != nil)
	}
	if colour != nil && len(colour) != model.PlaneSize() {
		return Plan{}, fmt.Errorf("colour plane is %d bytes, %s wants %d", len(colour), model.Name, model.PlaneSize())
	}
	if link.MTU <= frameOverhead {
		return Plan{}, fmt.Errorf("an MTU of %d leaves no room for image data", link.MTU)
	}

	var plan Plan
	for _, plane := range []struct {
		data   []byte
		colour bool
	}{{black, false}, {colour, true}} {
		if plane.data == nil {
			continue
		}
		frames, compressed := imageFrames(plane.data, plane.colour, link)
		plan.Frames = append(plan.Frames, frames...)
		plan.Compressed = plan.Compressed || compressed
	}
	plan.Frames = append(plan.Frames, []byte{cmdRefresh})
	return plan, nil
}

// imageFrames cuts one plane into writes.
//
// Compression is offered rather than assumed. A plane of unlike bytes comes out
// larger compressed, and sending more bytes to save none is worse than sending
// the plane, so the two are measured and the smaller wins.
func imageFrames(plane []byte, colour bool, link Link) (frames [][]byte, compressed bool) {
	limit := link.MTU - frameOverhead
	var pieces [][]byte
	if link.RLE {
		if encoded := chunkRLE(plane, limit); totalLength(encoded) < len(plane) {
			pieces, compressed = encoded, true
		}
	}
	if pieces == nil {
		for offset := 0; offset < len(plane); offset += limit {
			pieces = append(pieces, plane[offset:min(offset+limit, len(plane))])
		}
	}
	for index, piece := range pieces {
		flags := byte(0)
		if colour {
			flags |= flagColourPlane
		}
		if index == 0 {
			flags |= flagBegin
		}
		if compressed {
			flags |= flagRLE
		}
		frame := make([]byte, 0, frameOverhead+len(piece))
		frame = append(frame, cmdWriteImage, flags)
		frames = append(frames, append(frame, piece...))
	}
	return frames, compressed
}

func totalLength(pieces [][]byte) int {
	total := 0
	for _, piece := range pieces {
		total += len(piece)
	}
	return total
}
