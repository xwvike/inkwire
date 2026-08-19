package gicisky

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
	"tinygo.org/x/bluetooth"
)

const (
	// TargetAddress is the tag this build was developed against, kept as the
	// default so a single-tag setup needs no arguments. Any other tag is
	// reached with -device.
	TargetAddress = "FF:FF:92:94:38:61"
	// TargetName is advertised by every tag while it powers up, before it
	// settles on its own NEMR name. It identifies the product, not a tag.
	TargetName = "PICKSMART"

	// Finding the tag is the slowest and least predictable step: six
	// measured scans at RSSI -47 to -54 took between 4.3 and 11.5 seconds,
	// which is the tag's advertising interval rather than anything this
	// driver controls. Shortening this timeout turns a healthy tag into a
	// failed attempt, so it stays well above the slowest scan seen.
	DefaultScanTimeout = 15 * time.Second
	DefaultRetryDelay  = 2 * time.Second
	// Three attempts is the point of diminishing returns: a tag that has
	// not answered twice is reporting a Bluetooth problem, and each further
	// attempt costs another full scan timeout.
	DefaultAttempts = 3
)

var (
	serviceUUID = bluetooth.New16BitUUID(0xFEF0)
	controlUUID = bluetooth.New16BitUUID(0xFEF1)
	dataUUID    = bluetooth.New16BitUUID(0xFEF2)
)

type Driver struct {
	Adapter     *bluetooth.Adapter
	Target      string
	ScanTimeout time.Duration
	RetryDelay  time.Duration
	Attempts    int
	Uploader    Uploader
	Logf        func(string, ...any)
}

func NewDriver(adapter *bluetooth.Adapter, target string, logf func(string, ...any)) *Driver {
	if target == "" {
		target = TargetAddress
	}
	return &Driver{
		Adapter:     adapter,
		Target:      target,
		ScanTimeout: DefaultScanTimeout,
		RetryDelay:  DefaultRetryDelay,
		Attempts:    DefaultAttempts,
		Uploader:    NewUploader(logf),
		Logf:        logf,
	}
}

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
	return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s)", d.targetOrDefault())
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

func SelectIdentified(devices []FoundDevice, target string) (FoundDevice, error) {
	if target == "" {
		target = TargetAddress
	}
	for _, device := range devices {
		if !MatchesTarget(target, device.Name, device.Address.String()) {
			continue
		}
		if !device.HasAdvertised {
			return FoundDevice{}, fmt.Errorf("PICKSMART tag found (target %s), but no model advertisement was seen", target)
		}
		if !device.Identified {
			return FoundDevice{}, fmt.Errorf("PICKSMART tag found (target %s), but advertised id 0x%04X is not supported", target, device.Advertised.ID)
		}
		return device, nil
	}
	return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s)", target)
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

func giciskyAdvertisement(result bluetooth.ScanResult) (Advertisement, bool) {
	for _, element := range result.ManufacturerData() {
		if element.CompanyID != ManufacturerCompanyID {
			continue
		}
		return ParseAdvertisement(element.Data)
	}
	return Advertisement{}, false
}

func looksLikeTag(name string) bool {
	return strings.EqualFold(name, TargetName) || strings.HasPrefix(strings.ToUpper(name), "NEMR")
}

// Push writes a payload to a tag that has already been found, in one attempt.
//
// Finding and writing are separate because the tag says which panel it has in
// its advertisement, so a scene cannot be rendered — let alone encoded — until
// the tag has been found. A caller that has a payload already and no need for
// that answer can use FindAndPushWithRetry instead.
func (d *Driver) Push(ctx context.Context, found FoundDevice, payload []byte, options UploadOptions) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	if d.Adapter == nil {
		return errors.New("Bluetooth adapter is nil")
	}
	d.logf("resolved target %s (%s)", found.Name, found.Address.String())

	device, err := d.Adapter.Connect(found.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer device.Disconnect()

	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
	if err != nil {
		return fmt.Errorf("discover service FEF0: %w", err)
	}
	if len(services) != 1 {
		return fmt.Errorf("discover service FEF0: expected 1 service, got %d", len(services))
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{controlUUID, dataUUID})
	if err != nil {
		return fmt.Errorf("discover characteristics FEF1/FEF2: %w", err)
	}
	if len(characteristics) != 2 {
		return fmt.Errorf("discover characteristics FEF1/FEF2: expected 2, got %d", len(characteristics))
	}

	transport := &bleTransport{
		device:        device,
		control:       characteristics[0],
		data:          characteristics[1],
		notifications: make(chan []byte, 16),
	}
	if err := transport.control.EnableNotifications(func(buffer []byte) {
		message := append([]byte(nil), buffer...)
		transport.notifications <- message
	}); err != nil {
		return fmt.Errorf("enable FEF1 notifications: %w", err)
	}

	return d.Uploader.UploadWithOptions(ctx, transport, payload, options)
}

// PushWithRetry writes to a tag that has already been found, retrying the
// write alone. Use it when the tag was found in order to learn its panel, so
// that a failed write does not throw that answer away.
func (d *Driver) PushWithRetry(ctx context.Context, found FoundDevice, payload []byte, options UploadOptions) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	return d.retrying(ctx, "write", func() error {
		return d.Push(ctx, found, payload, options)
	})
}

// FindAndPushWithRetry scans for the driver's target and writes to it, taking
// a fresh scan on every attempt. A tag takes an unsteady several seconds to
// advertise again after a disconnect, so the address that answered last time
// is not the thing worth retrying — the scan is.
func (d *Driver) FindAndPushWithRetry(ctx context.Context, payload []byte, options UploadOptions) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	return d.retrying(ctx, "write", func() error {
		found, err := d.Find(ctx)
		if err != nil {
			return err
		}
		return d.Push(ctx, found, payload, options)
	})
}

// retrying runs one attempt up to Attempts times. Every command that reaches
// the radio goes through it; see ble.Retry for why.
func (d *Driver) retrying(ctx context.Context, what string, attempt func() error) error {
	attempts := d.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	delay := d.RetryDelay
	if delay <= 0 {
		delay = DefaultRetryDelay
	}
	return ble.Retry{Attempts: attempts, Delay: delay, Logf: d.Logf}.Do(ctx, what, attempt)
}

func (d *Driver) targetOrDefault() string {
	if d.Target == "" {
		return TargetAddress
	}
	return d.Target
}

func MatchesTarget(target, name, address string) bool {
	if target == "" {
		target = TargetAddress
	}
	if strings.EqualFold(name, target) || strings.EqualFold(address, target) {
		return true
	}
	// A MAC never equals the address on a host that does not expose one:
	// CoreBluetooth substitutes a per-host UUID, so every MAC target would
	// otherwise be unreachable on macOS. The advertised name is derived from
	// the MAC, so match on that instead.
	//
	// TargetName is deliberately not accepted here. Every tag advertises it
	// while powering up, so honouring it for a MAC target would let a write
	// aimed at one tag land on whichever tag happened to be booting. Ask for
	// it by name if that is genuinely what you want.
	if derived, ok := advertisedName(target); ok {
		return strings.EqualFold(name, derived)
	}
	return false
}

// advertisedName derives the name a tag settles on from its MAC. Gicisky tags
// are addressed FF:FF:xx:yy:zz:kk and advertise NEMRxxyyzzkk, which makes the
// name the one identifier that is both unique per tag and identical on every
// host that sees it.
//
// It reports false for anything that is not a complete MAC, so names and
// CoreBluetooth UUIDs pass through untouched.
func advertisedName(target string) (string, bool) {
	cleaned := strings.NewReplacer(":", "", "-", "").Replace(target)
	if len(cleaned) != macHexDigits {
		return "", false
	}
	for _, digit := range cleaned {
		if !isHexDigit(digit) {
			return "", false
		}
	}
	return "NEMR" + strings.ToUpper(cleaned[macHexDigits-8:]), true
}

// macHexDigits is the length of a MAC in hex digits once separators are gone.
// The name carries only the last four bytes; the leading FF:FF is shared by
// every tag and so identifies none of them.
const macHexDigits = 12

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func (d *Driver) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

type bleTransport struct {
	device        bluetooth.Device
	control       bluetooth.DeviceCharacteristic
	data          bluetooth.DeviceCharacteristic
	notifications chan []byte
}

func (t *bleTransport) Notifications() <-chan []byte {
	return t.notifications
}

func (t *bleTransport) WriteControl(data []byte) error {
	return writeCharacteristic(t.control, data)
}

func (t *bleTransport) WriteData(data []byte) error {
	return writeCharacteristic(t.data, data)
}

func writeCharacteristic(characteristic bluetooth.DeviceCharacteristic, data []byte) error {
	written, err := characteristic.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return fmt.Errorf("short GATT write: wrote %d of %d bytes", written, len(data))
	}
	return nil
}

// stopScanRetry is how often the scan is asked again to stop, and
// stopScanLimit is how long that is worth doing before giving up on it.
const ()

// stopScanning ends a scan and waits for it to say that it has ended.
//
// Asking more than once is not belt and braces. StopScan before Scan has begun
// does nothing at all, and the scan that starts a moment later has nobody left
// to stop it: the wait never returns, and whoever called this holds the adapter
// for the life of the process. The window is wide open whenever the context is
// already cancelled on the way in — which for the HTTP service is every time a
// client hangs up mid-request, because /v1/devices scans both families in turn
// and the second scan begins with a context that is already done. One abandoned
// request wedged the service until it was restarted.
//
// The waiting is bounded for the same reason. A scan that will not stop is bad;
// saying so lets the caller release the adapter and report a failure, which is
// recoverable. Blocking here is not.
//
// stop is passed as a function rather than an adapter so that this can be
// exercised without a radio.

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
