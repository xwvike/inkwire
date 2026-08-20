package gicisky

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xwvike/inkwire/internal/ble"
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

// matchedBy and identifiedBy are the two questions a scan can be stopped on.
//
// They are named rather than written inline at the call sites so that a test
// can hold the real ones, not a copy that goes on passing after the driver has
// changed under it.
func matchedBy(target string) func(*deviceSet) bool {
	return func(seen *deviceSet) bool {
		_, ok := seen.match(target)
		return ok
	}
}

func identifiedBy(target string) func(*deviceSet) bool {
	return func(seen *deviceSet) bool {
		device, ok := seen.match(target)
		return ok && device.HasAdvertised
	}
}

// scanUntil merges advertisements into one entry per address and stops as soon
// as stop is satisfied, or when the scan window closes if it never is.
//
// A tag does not put its local name and its manufacturer data in the same
// advertisement, so what a caller is waiting for is never a single packet: it
// is one address having been seen enough times to answer the question asked.
// Passing nil for stop waits the whole window, which is what enumerating
// everything nearby has to do.
func (d *Driver) scanUntil(ctx context.Context, stop func(*deviceSet) bool) ([]FoundDevice, error) {
	timeout := d.ScanTimeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}
	seen := newDeviceSet()
	var satisfied func() bool
	if stop != nil {
		satisfied = func() bool { return stop(seen) }
	}
	err := ble.Scan(ctx, d.Adapter, timeout, func(result bluetooth.ScanResult) {
		advertised, ok := giciskyAdvertisement(result)
		seen.observe(result.Address, result.LocalName(), result.RSSI, advertised, ok)
	}, satisfied)
	if err != nil {
		return nil, err
	}
	return seen.sorted(), nil
}

// Find resolves the driver's target and stops looking the moment it has it.
//
// This answers "which tag", not "what panel". A caller that already knows the
// model — push-payload is told it on the command line — needs nothing more.
func (d *Driver) Find(ctx context.Context) (FoundDevice, error) {
	devices, err := d.scanUntil(ctx, matchedBy(d.Target))
	if err != nil {
		return FoundDevice{}, err
	}
	for _, device := range devices {
		if MatchesTarget(d.Target, device.Name, device.Address.String()) {
			return device, nil
		}
	}
	return FoundDevice{}, fmt.Errorf("no Gicisky tag %s is in range", describeTarget(d.Target))
}

// ScanAll reports every Gicisky tag advertising nearby instead of stopping at
// the first match, and identifies each one from its advertisement.
//
// It waits the whole window on purpose. There is no way to know that the last
// tag has been heard from, so a listing that stopped early would be a listing
// that quietly omits whichever tag advertises least often.
func (d *Driver) ScanAll(ctx context.Context) ([]FoundDevice, error) {
	return d.scanUntil(ctx, nil)
}

// FindIdentified resolves the target and the panel it has, stopping as soon as
// one address has answered both questions.
//
// Waiting for both halves costs nothing measurable: the tag sends its name and
// its manufacturer data in the same advertising burst, about a millisecond
// apart. What varies is when that burst lands, which on hardware here is
// anywhere from 0.7s to 5s — against the 15s this used to spend every time,
// because it read the whole window meant for listing every tag nearby.
func (d *Driver) FindIdentified(ctx context.Context) (FoundDevice, error) {
	devices, err := d.scanUntil(ctx, identifiedBy(d.Target))
	if err != nil {
		return FoundDevice{}, err
	}
	return SelectIdentified(devices, d.Target)
}

func (d *Driver) FindIdentifiedWithRetry(ctx context.Context) (FoundDevice, error) {
	var device FoundDevice
	err := d.retrying(ctx, "identify", func() error {
		found, err := d.FindIdentified(ctx)
		device = found
		return err
	})
	if err != nil {
		return FoundDevice{}, err
	}
	return device, nil
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

// match reports the entry the target names, if one has been heard from yet.
// It is the same question SelectIdentified asks afterwards, asked early so a
// scan can stop as soon as the answer exists.
func (s *deviceSet) match(target string) (*FoundDevice, bool) {
	for _, device := range s.found {
		if MatchesTarget(target, device.Name, device.Address.String()) {
			return device, true
		}
	}
	return nil, false
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
