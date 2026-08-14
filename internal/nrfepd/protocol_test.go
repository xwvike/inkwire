package nrfepd

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// The bytes a real panel answered with, kept verbatim. A config parser checked
// only against configs it made up will agree with itself forever.
const observedConfig = "1413060504030203ff1207000000"

func TestTheConfigFromARealPanelNamesItsModel(t *testing.T) {
	data, err := hex.DecodeString(observedConfig)
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := config.ModelID, uint8(0x03); got != want {
		t.Fatalf("model id = 0x%02x, want 0x%02x", got, want)
	}
	model, ok := config.Model()
	if !ok {
		t.Fatalf("model 0x%02x is not in the table", config.ModelID)
	}
	if model.Name != "UC8176_420_BWR" || model.Width != 400 || model.Height != 300 {
		t.Errorf("model = %s, want UC8176_420_BWR 400x300", model)
	}
	// The pin map is the rest of the evidence that the fields line up. Reading
	// model_id right out of a blob that was otherwise misaligned would be luck.
	if config.MOSIPin != 20 || config.SCLKPin != 19 || config.WakeupPin != 0xff {
		t.Errorf("pins = %+v, which does not match the board this came from", config)
	}
}

// This panel's firmware is ahead of the reference project and says things the
// reference never said. Refusing to understand a device is one thing; refusing
// to talk to it because it volunteered something extra is another.
func TestUnknownNotificationsAreIgnoredRatherThanFatal(t *testing.T) {
	link, ok := ParseNotification("mtu=244 rle=1")
	if !ok || link.MTU != 244 || !link.RLE {
		t.Errorf("mtu line parsed as %+v, ok=%v", link, ok)
	}
	if link, ok := ParseNotification("mtu=23"); !ok || link.MTU != 23 || link.RLE {
		t.Errorf("a firmware without compression parsed as %+v, ok=%v", link, ok)
	}
	// Everything this panel actually sent alongside the MTU.
	for _, message := range []string{"slots=15 1 0", "sid=D332EAD65D9F83D56B923234BEFB9917", "t=1786737722", ""} {
		if _, ok := ParseNotification(message); ok {
			t.Errorf("%q was read as a link description", message)
		}
	}
}

func planes(t *testing.T, model Model) (black, colour []byte) {
	t.Helper()
	black = bytes.Repeat([]byte{0xff}, model.PlaneSize())
	colour = bytes.Repeat([]byte{0xff}, model.PlaneSize())
	// Ink in runs and a patch of noise, so neither the compressed nor the
	// uncompressed path is the only one exercised.
	for index := 500; index < 900; index++ {
		black[index] = 0x00
	}
	for index := 2000; index < 2100; index++ {
		black[index] = byte(index * 7)
	}
	for index := 3000; index < 3050; index++ {
		colour[index] = 0x00
	}
	return black, colour
}

// The plan is only worth anything if the panel can put the page back together
// from it, so this reassembles the writes the way the firmware does: split the
// planes apart by the flag bit, decompress what says it is compressed, and
// compare against what went in.
func TestThePanelCanRebuildBothPlanesFromThePlan(t *testing.T) {
	model, _ := LookupModelName("UC8176_420_BWR")
	black, colour := planes(t, model)

	for _, link := range []Link{{MTU: 244, RLE: true}, {MTU: 244}, {MTU: 23, RLE: true}} {
		plan, err := BuildPlan(model, black, colour, link)
		if err != nil {
			t.Fatal(err)
		}
		// The init is the session's, not the plan's: it has to have been sent
		// and answered before there is a model to build a plan for.
		if got := plan.Frames[0][0]; got != cmdWriteImage {
			t.Errorf("first frame starts with 0x%02x, want image data", got)
		}
		if got := plan.Frames[len(plan.Frames)-1]; !bytes.Equal(got, []byte{cmdRefresh}) {
			t.Errorf("last frame = %x, want a refresh", got)
		}

		var rebuilt [2][]byte
		var begun [2]bool
		for _, frame := range plan.Frames[:len(plan.Frames)-1] {
			if len(frame) > link.MTU {
				t.Fatalf("a frame is %d bytes, over the %d MTU", len(frame), link.MTU)
			}
			if frame[0] != cmdWriteImage {
				t.Fatalf("frame starts with 0x%02x between the init and the refresh", frame[0])
			}
			plane := frame[1] & flagColourPlane
			if frame[1]&flagBegin != 0 {
				if begun[plane] {
					t.Errorf("plane %d was begun twice", plane)
				}
				begun[plane] = true
			} else if !begun[plane] {
				t.Errorf("plane %d was continued before it was begun", plane)
			}
			payload := frame[frameOverhead:]
			if frame[1]&flagRLE != 0 {
				payload = decode(t, payload)
			}
			rebuilt[plane] = append(rebuilt[plane], payload...)
		}
		if !bytes.Equal(rebuilt[0], black) {
			t.Errorf("link %+v: black plane came back %d bytes, sent %d", link, len(rebuilt[0]), len(black))
		}
		if !bytes.Equal(rebuilt[1], colour) {
			t.Errorf("link %+v: colour plane came back %d bytes, sent %d", link, len(rebuilt[1]), len(colour))
		}
	}
}

// Compression is a choice per page, and the wrong choice is only slower rather
// than broken, so what matters is that it is measured rather than assumed.
func TestAPageThatWouldGrowIsSentUncompressed(t *testing.T) {
	model, _ := LookupModelName("UC8176_420_BWR")
	noise := make([]byte, model.PlaneSize())
	for index := range noise {
		noise[index] = byte(index % 251)
	}
	plan, err := BuildPlan(model, noise, nil, Link{MTU: 244, RLE: true})
	if err == nil {
		t.Fatal("a colour panel accepted a page with no colour plane")
	}
	_ = plan

	bw, _ := LookupModelName("UC8176_420_BW")
	plan, err = BuildPlan(bw, noise[:bw.PlaneSize()], nil, Link{MTU: 244, RLE: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Compressed {
		t.Error("a page with nothing to repeat was sent compressed, which sends more bytes than it saves")
	}
}

// A page of the wrong shape is the failure this family invites, because the
// panel never says what it is until it is asked and a page is built before
// anyone asks. It is refused where the sizes are known rather than written.
func TestAPlaneOfTheWrongSizeIsRefused(t *testing.T) {
	model, _ := LookupModelName("UC8176_420_BWR")
	full := bytes.Repeat([]byte{0xff}, model.PlaneSize())
	short := full[:len(full)-1]

	if _, err := BuildPlan(model, short, full, Link{MTU: 244}); err == nil {
		t.Error("a short black plane was accepted")
	}
	if _, err := BuildPlan(model, full, short, Link{MTU: 244}); err == nil {
		t.Error("a short colour plane was accepted")
	}
	nibbles, _ := LookupModelName("UC8159_750_LOW_BWR")
	if _, err := BuildPlan(nibbles, full, full, Link{MTU: 244}); err == nil {
		t.Error("a panel packed in nibbles was sent planes")
	}
}
