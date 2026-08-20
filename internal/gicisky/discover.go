package gicisky

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"tinygo.org/x/bluetooth"
)

type FoundDevice struct {
	Address bluetooth.Address
	Name    string
	RSSI    int16

	// Advertised is present when the tag included manufacturer data. Profile
	// is only filled in when that id is one this build knows; an unknown tag
	// is still reported, because refusing to mention it would be worse than
	// saying it cannot be driven.
	Advertised    Advertisement
	HasAdvertised bool
	Profile       Profile
	Identified    bool
}

// ErrNotIdentified marks the tag that answered but did not say what panel it
// has, whether because no manufacturer data was seen or because the id it gave
// is not in this build's table.
//
// It is worth telling apart from a tag that was never found, because only this
// one can be got past by stating the model: a caller that already has the bytes
// can go on without the answer. Suggesting that to somebody whose tag is simply
// not there sends them to a flag that cannot help.
var ErrNotIdentified = errors.New("the tag did not identify its panel")

func SelectIdentified(devices []FoundDevice, target string) (FoundDevice, error) {
	for _, device := range devices {
		if !MatchesTarget(target, device.Name, device.Address.String()) {
			continue
		}
		if !device.HasAdvertised {
			return FoundDevice{}, fmt.Errorf("the Gicisky tag %s sent no model advertisement: %w", describeTarget(target), ErrNotIdentified)
		}
		if !device.Identified {
			return FoundDevice{}, fmt.Errorf("the Gicisky tag %s advertised id 0x%04X, which this build does not know: %w",
				describeTarget(target), device.Advertised.ID, ErrNotIdentified)
		}
		return device, nil
	}
	return FoundDevice{}, fmt.Errorf("no Gicisky tag %s is in range", describeTarget(target))
}

// deviceSet accumulates advertisements into one entry per address.
//
// It is separate from the scan itself so that the merging can be exercised
// without a radio. Merging is precisely what a second tag brings into play,
// and a build that has only ever seen one tag has never run it.
type deviceSet struct {
	found map[string]*FoundDevice
}

func newDeviceSet() *deviceSet { return &deviceSet{found: make(map[string]*FoundDevice)} }

// observe folds one advertisement into the set and reports whether it was
// kept. A packet with neither manufacturer data nor a tag-shaped name belongs
// to somebody else's device.
func (s *deviceSet) observe(address bluetooth.Address, name string, rssi int16, advertised Advertisement, hasAdvertised bool) bool {
	// Without manufacturer data the only evidence left is the name, and every
	// tag advertises the same name shape whatever panel it has, so the name is
	// a weaker signal kept only so nameless and named packets from one tag end
	// up as one entry.
	if !hasAdvertised && !looksLikeTag(name) {
		return false
	}
	key := address.String()
	device := s.found[key]
	if device == nil {
		device = &FoundDevice{Address: address}
		s.found[key] = device
	}
	// A later packet without a name must not erase a name already learned.
	if name != "" {
		device.Name = name
	}
	device.RSSI = rssi
	if hasAdvertised {
		device.Advertised, device.HasAdvertised = advertised, true
		device.Profile, device.Identified = LookupProfile(advertised.ID, advertised.Firmware)
	}
	return true
}

func (s *deviceSet) sorted() []FoundDevice {
	devices := make([]FoundDevice, 0, len(s.found))
	for _, device := range s.found {
		devices = append(devices, *device)
	}
	// Strongest first, because that is the one within reach; the address
	// breaks ties so repeated scans list a fixed set in a fixed order.
	slices.SortFunc(devices, func(a, b FoundDevice) int {
		if a.RSSI != b.RSSI {
			return int(b.RSSI) - int(a.RSSI)
		}
		return strings.Compare(a.Address.String(), b.Address.String())
	})
	return devices
}

func looksLikeTag(name string) bool {
	return strings.EqualFold(name, TargetName) || strings.HasPrefix(strings.ToUpper(name), "NEMR")
}

// Collector accumulates one scan into this family's devices.
//
// It exists so a single pass of the radio can feed both families at once.
// Scanning is promiscuous — every advertisement nearby arrives whatever it
// belongs to — so the families are told apart by a filter, not by a scan each.
// Observe is called from ble.Scan, which serialises it, so there is no lock
// here.
type Collector struct{ seen *deviceSet }

func NewCollector() *Collector { return &Collector{seen: newDeviceSet()} }

func (c *Collector) Observe(result bluetooth.ScanResult) {
	advertised, ok := giciskyAdvertisement(result)
	c.seen.observe(result.Address, result.LocalName(), result.RSSI, advertised, ok)
}

func (c *Collector) Devices() []FoundDevice { return c.seen.sorted() }

// describeTarget names what was asked for, in a sentence that still reads when
// nothing was.
func describeTarget(target string) string {
	if target == "" {
		return "of any name"
	}
	return strconv.Quote(target)
}
