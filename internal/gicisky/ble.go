package gicisky

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

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

func (d *Driver) Find(ctx context.Context) (FoundDevice, error) {
	if d.Adapter == nil {
		return FoundDevice{}, errors.New("Bluetooth adapter is nil")
	}
	timeout := d.ScanTimeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	found := make(chan FoundDevice, 1)
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- d.Adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := result.LocalName()
			if !d.matches(name, result.Address.String()) {
				return
			}
			select {
			case found <- FoundDevice{Address: result.Address, Name: name, RSSI: result.RSSI}:
				_ = adapter.StopScan()
			default:
			}
		})
	}()

	select {
	case device := <-found:
		if err := <-scanDone; err != nil {
			return FoundDevice{}, fmt.Errorf("stop scan: %w", err)
		}
		return device, nil
	case err := <-scanDone:
		if err != nil {
			return FoundDevice{}, fmt.Errorf("scan: %w", err)
		}
		select {
		case device := <-found:
			return device, nil
		default:
			return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s)", d.Target)
		}
	case <-scanCtx.Done():
		if err := stopScanning(d.Adapter.StopScan, scanDone); err != nil {
			return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s), and %w", d.Target, err)
		}
		return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s): %w", d.Target, scanCtx.Err())
	}
}

// ScanAll reports every Gicisky tag advertising nearby instead of stopping at
// the first match, and identifies each one from its advertisement.
//
// Results are merged by address across packets on purpose. A tag does not put
// its local name and its manufacturer data in the same advertisement, so a
// single packet tells you either what it is called or what it is, never both.
func (d *Driver) ScanAll(ctx context.Context) ([]FoundDevice, error) {
	if d.Adapter == nil {
		return nil, errors.New("Bluetooth adapter is nil")
	}
	timeout := d.ScanTimeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}

	var mutex sync.Mutex
	seen := newDeviceSet()
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- d.Adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			advertised, ok := giciskyAdvertisement(result)
			mutex.Lock()
			defer mutex.Unlock()
			seen.observe(result.Address, result.LocalName(), result.RSSI, advertised, ok)
		})
	}()

	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	<-scanCtx.Done()
	if err := stopScanning(d.Adapter.StopScan, scanDone); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	return seen.sorted(), nil
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

func (d *Driver) Push(ctx context.Context, payload []byte) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	found, err := d.Find(ctx)
	if err != nil {
		return err
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

	return d.Uploader.Upload(ctx, transport, payload)
}

func (d *Driver) PushWithRetry(ctx context.Context, payload []byte) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	attempts := d.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	retryDelay := d.RetryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := d.Push(ctx, payload); err == nil {
			return nil
		} else {
			lastErr = err
			d.logf("attempt %d/%d failed: %v", attempt, attempts, err)
		}
		if attempt < attempts {
			if err := wait(ctx, retryDelay); err != nil {
				return err
			}
		}
	}
	return lastErr
}

func (d *Driver) matches(name, address string) bool {
	if strings.EqualFold(name, d.Target) || strings.EqualFold(address, d.Target) {
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
	if derived, ok := advertisedName(d.Target); ok {
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
const (
	stopScanRetry = 100 * time.Millisecond
	stopScanLimit = 5 * time.Second
)

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
func stopScanning(stop func() error, done <-chan error) error {
	deadline := time.Now().Add(stopScanLimit)
	for {
		_ = stop()
		select {
		case err := <-done:
			return err
		case <-time.After(stopScanRetry):
		}
		if time.Now().After(deadline) {
			return errors.New("the scan did not stop when it was asked to")
		}
	}
}
