package nrfepd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/xwvike/inkwire/internal/ble"
	"tinygo.org/x/bluetooth"
)

// FoundDevice is one tag seen advertising.
//
// There is no model here and there cannot be. This family says nothing about
// its panel until it is connected to and asked, so a scan can report that a tag
// exists and nothing else about it.
type FoundDevice struct {
	Address bluetooth.Address
	Name    string
	RSSI    int16
}

// Advertises reports whether an advertisement belongs to this family.
//
// The service UUID is the real evidence and the name is the fallback. A tag
// puts 62750001-… in its advertisement because that is the service it serves,
// which is a fact about the firmware rather than a convention; the name is
// DEVICE_NAME and whoever builds the firmware may set it to anything.
//
// Both are accepted because only one tag has been seen here. Trusting the UUID
// alone would drop any build that advertises the service without listing it,
// and trusting the name alone is what this used to do — which meant a renamed
// tag vanished from a scan that could plainly see it.
func Advertises(result bluetooth.ScanResult) bool {
	return result.HasServiceUUID(serviceUUID) || LooksLikeName(result.LocalName())
}

// LooksLikeName reports whether an advertised name follows this firmware's
// convention. It answers a weaker question than Advertises and is kept for the
// places that have a name and nothing else, such as a target somebody typed.
func LooksLikeName(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), NamePrefix)
}

// deviceSet accumulates a scan into one entry per address.
//
// Unlike the other family there is nothing to merge: this tag puts its name in
// the same advertisement as everything else knowable about it without
// connecting, so a later packet only refreshes the signal. The panel is not in
// there at all — it is in the firmware's flash, and asking for it is what a
// connection is for.
type deviceSet struct{ found map[string]FoundDevice }

func newDeviceSet() *deviceSet { return &deviceSet{found: make(map[string]FoundDevice)} }

func (s *deviceSet) observe(result bluetooth.ScanResult) {
	if !Advertises(result) {
		return
	}
	// A packet that carries the service UUID need not carry the name, so this
	// merges the way the other family has to: a later nameless packet must not
	// erase a name an earlier one taught.
	key := result.Address.String()
	device := s.found[key]
	device.Address, device.RSSI = result.Address, result.RSSI
	if name := result.LocalName(); name != "" {
		device.Name = name
	}
	s.found[key] = device
}

func (s *deviceSet) match(matches func(name, address string) bool) (FoundDevice, bool) {
	for _, device := range s.found {
		if matches(device.Name, device.Address.String()) {
			return device, true
		}
	}
	return FoundDevice{}, false
}

func (s *deviceSet) sorted() []FoundDevice {
	devices := make([]FoundDevice, 0, len(s.found))
	for _, device := range s.found {
		devices = append(devices, device)
	}
	slices.SortFunc(devices, func(a, b FoundDevice) int {
		if a.RSSI != b.RSSI {
			return int(b.RSSI) - int(a.RSSI)
		}
		return strings.Compare(a.Address.String(), b.Address.String())
	})
	return devices
}

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
	if err := ble.Scan(ctx, d.Adapter, timeout, seen.observe, satisfied); err != nil {
		return nil, err
	}
	return seen.sorted(), nil
}

// Find resolves the driver's target and stops looking the moment it has it.
func (d *Driver) Find(ctx context.Context) (FoundDevice, error) {
	devices, err := d.scanUntil(ctx, func(seen *deviceSet) bool {
		_, ok := seen.match(d.matches)
		return ok
	})
	if err != nil {
		return FoundDevice{}, err
	}
	for _, device := range devices {
		if d.matches(device.Name, device.Address.String()) {
			return device, nil
		}
	}
	return FoundDevice{}, fmt.Errorf("no %s tag found (target %q)", NamePrefix, d.Target)
}

// ScanAll reports every tag of this family advertising nearby.
//
// It waits the whole window, because there is no way to know the quietest tag
// has already been heard from.
func (d *Driver) ScanAll(ctx context.Context) ([]FoundDevice, error) {
	return d.scanUntil(ctx, nil)
}

// Collector accumulates one scan into this family's devices, so that a single
// pass of the radio can feed both families. See gicisky.Collector.
type Collector struct{ seen *deviceSet }

func NewCollector() *Collector { return &Collector{seen: newDeviceSet()} }

func (c *Collector) Observe(result bluetooth.ScanResult) { c.seen.observe(result) }

func (c *Collector) Devices() []FoundDevice { return c.seen.sorted() }
