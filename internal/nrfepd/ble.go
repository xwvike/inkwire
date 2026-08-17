package nrfepd

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
	// NamePrefix is what this firmware advertises: the name is DEVICE_NAME
	// with the last two bytes of the address after it, so every tag of this
	// family starts the same way and none of them share a whole name.
	//
	// Unlike a Gicisky tag, the name is all there is. Nothing in the
	// advertisement says what panel is attached; that is kept in the
	// firmware's own flash and only comes out once it is asked.
	NamePrefix = "NRF_EPD"

	DefaultScanTimeout = 15 * time.Second
	DefaultRetryDelay  = 2 * time.Second
	DefaultAttempts    = 3
)

// The service is a vendor 128-bit UUID with a 16-bit slot in it, which is how
// Nordic's soft device builds one. The base is the byte array in
// EPD_service.h, least significant byte first; the slot is octets 12 and 13,
// which the firmware fills with 0x0001 for the service and 0x0002 for the
// characteristic that carries both directions of the conversation.
var (
	serviceUUID = mustParseUUID("62750001-d828-918d-fb46-b6c11c675aec")
	epdUUID     = mustParseUUID("62750002-d828-918d-fb46-b6c11c675aec")
	versionUUID = mustParseUUID("62750003-d828-918d-fb46-b6c11c675aec")
)

// mustParseUUID refuses to start rather than carrying on with a zero UUID.
// These are constants that are either right or a typing mistake, and the way a
// mistyped one fails is service discovery quietly finding nothing, which reads
// as a tag that is not there.
func mustParseUUID(text string) bluetooth.UUID {
	parsed, err := bluetooth.ParseUUID(text)
	if err != nil {
		panic(fmt.Sprintf("nrfepd: malformed UUID %q: %v", text, err))
	}
	return parsed
}

type Driver struct {
	Adapter     *bluetooth.Adapter
	Target      string
	ScanTimeout time.Duration
	RetryDelay  time.Duration
	Attempts    int
	// Timings covers the two waits inside one conversation: how long the panel
	// has to identify itself, and how long to hold the connection open while
	// it draws. The second is not a nicety, see DefaultSettle.
	Timings Timings
	Logf    func(string, ...any)
}

func NewDriver(adapter *bluetooth.Adapter, target string, logf func(string, ...any)) *Driver {
	return &Driver{
		Adapter:     adapter,
		Target:      target,
		ScanTimeout: DefaultScanTimeout,
		RetryDelay:  DefaultRetryDelay,
		Attempts:    DefaultAttempts,
		Timings:     Timings{Response: DefaultResponseTimeout, Settle: DefaultSettle},
		Logf:        logf,
	}
}

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

func (d *Driver) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

// matches decides whether a scan result is the tag that was asked for. An empty
// target takes the first tag of this family, which is what a single-tag setup
// wants; anything else is matched by name or by address.
func (d *Driver) matches(name, address string) bool {
	if d.Target == "" {
		return looksLikeTag(name)
	}
	return strings.EqualFold(name, d.Target) || strings.EqualFold(address, d.Target)
}

func looksLikeTag(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), NamePrefix)
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
		<-scanDone
		return device, nil
	case err := <-scanDone:
		if err != nil {
			return FoundDevice{}, fmt.Errorf("scan: %w", err)
		}
		select {
		case device := <-found:
			return device, nil
		default:
			return FoundDevice{}, fmt.Errorf("no %s tag found (target %q)", NamePrefix, d.Target)
		}
	case <-scanCtx.Done():
		if err := stopScanning(d.Adapter.StopScan, scanDone); err != nil {
			return FoundDevice{}, fmt.Errorf("no %s tag found (target %q), and %w", NamePrefix, d.Target, err)
		}
		return FoundDevice{}, fmt.Errorf("no %s tag found (target %q): %w", NamePrefix, d.Target, scanCtx.Err())
	}
}

// ScanAll reports every tag of this family advertising nearby.
func (d *Driver) ScanAll(ctx context.Context) ([]FoundDevice, error) {
	if d.Adapter == nil {
		return nil, errors.New("Bluetooth adapter is nil")
	}
	timeout := d.ScanTimeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}

	var mutex sync.Mutex
	seen := map[string]FoundDevice{}
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- d.Adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := result.LocalName()
			if !looksLikeTag(name) {
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			seen[result.Address.String()] = FoundDevice{
				Address: result.Address, Name: name, RSSI: result.RSSI,
			}
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
	devices := make([]FoundDevice, 0, len(seen))
	for _, device := range seen {
		devices = append(devices, device)
	}
	slices.SortFunc(devices, func(a, b FoundDevice) int {
		if a.RSSI != b.RSSI {
			return int(b.RSSI) - int(a.RSSI)
		}
		return strings.Compare(a.Address.String(), b.Address.String())
	})
	return devices, nil
}

// Push connects to a tag and puts one page on it.
//
// The page is asked for rather than passed in, because until this has connected
// nobody knows what panel is on the other end. See PageFor.
func (d *Driver) Push(ctx context.Context, page PageFor) error {
	return d.converse(ctx, func(link transport) error {
		return Session(ctx, link, page, d.Timings, d.Logf)
	})
}

// SetMode hands the tag back to drawing its own clock or calendar, and sets
// that clock on the way.
//
// It is here because Push takes this away: the refresh that ends every page
// puts the tag into picture mode. Without a way back, this program would only
// ever subtract from what the tag could do before it arrived.
func (d *Driver) SetMode(ctx context.Context, when time.Time, mode Mode, weekStart *time.Weekday) error {
	return d.converse(ctx, func(link transport) error {
		return ModeSession(ctx, link, when, mode, weekStart, d.Timings, d.Logf)
	})
}

// converse finds the tag, opens the one characteristic that carries both
// directions, and hands it to whatever wants to talk over it.
func (d *Driver) converse(ctx context.Context, talk func(transport) error) error {
	if d.Adapter == nil {
		return errors.New("Bluetooth adapter is nil")
	}
	found, err := d.Find(ctx)
	if err != nil {
		return err
	}
	d.logf("connecting to %s (%s)", found.Name, found.Address.String())

	device, err := d.Adapter.Connect(found.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer device.Disconnect()

	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
	if err != nil {
		return fmt.Errorf("discover the EPD service: %w", err)
	}
	if len(services) != 1 {
		return fmt.Errorf("discover the EPD service: found %d", len(services))
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{epdUUID, versionUUID})
	if err != nil {
		return fmt.Errorf("discover the EPD characteristic: %w", err)
	}
	if len(characteristics) == 0 {
		return errors.New("the EPD service has no characteristics")
	}
	d.logFirmwareVersion(characteristics)

	link := &bleTransport{
		characteristic: characteristics[0],
		// Buffered because the panel volunteers several messages at once and
		// the notification callback must not block the Bluetooth stack.
		notifications: make(chan []byte, 16),
	}
	if err := link.characteristic.EnableNotifications(func(buffer []byte) {
		select {
		case link.notifications <- append([]byte(nil), buffer...):
		default:
			// A full queue means nobody is listening any more, and dropping is
			// better than wedging the stack's callback.
		}
	}); err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}

	return talk(link)
}

// logFirmwareVersion reports what the tag is running when it offers to say.
//
// It is worth logging and not worth failing on. The firmware in the field is
// ahead of the project it came from and numbers itself differently, so a
// version this build has never seen says nothing about whether the page will
// go on.
func (d *Driver) logFirmwareVersion(characteristics []bluetooth.DeviceCharacteristic) {
	if len(characteristics) < 2 {
		return
	}
	value := make([]byte, 1)
	if _, err := characteristics[1].Read(value); err != nil {
		return
	}
	d.logf("firmware version 0x%02x", value[0])
}

func (d *Driver) PushWithRetry(ctx context.Context, page PageFor) error {
	return d.retrying(ctx, func() error { return d.Push(ctx, page) })
}

// SetModeWithRetry is SetMode, given the same second chance as a push.
//
// The time is asked for once per attempt rather than passed in, because a
// retry that set the tag to the time the first attempt was made would put the
// clock out by however long the failures took.
func (d *Driver) SetModeWithRetry(ctx context.Context, now func() time.Time, mode Mode, weekStart *time.Weekday) error {
	return d.retrying(ctx, func() error { return d.SetMode(ctx, now(), mode, weekStart) })
}

// retrying runs an attempt until one succeeds or they run out.
//
// A scan that finds nothing is not evidence that the tag is not there. On
// 2026-08-17 `inkwire mode` reported no tag while that tag sat on the desk and
// answered a scan seconds later — a whole 15s scan, not a clipped one. How
// often that happens is not known here, and the first attempt to measure it
// produced numbers that turned out to be the sampling period rather than the
// tag. Twelve mode changes since have all found it first time. Once is enough:
// what a single scan proves is that it saw nothing, which is a different claim
// from the tag being absent, and only this one entry point used to confuse the
// two.
func (d *Driver) retrying(ctx context.Context, attempt func() error) error {
	attempts := d.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	delay := d.RetryDelay
	if delay <= 0 {
		delay = DefaultRetryDelay
	}
	var lastErr error
	for number := 1; number <= attempts; number++ {
		if err := attempt(); err == nil {
			return nil
		} else {
			lastErr = err
			d.logf("attempt %d/%d failed: %v", number, attempts, err)
		}
		if number < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

// bleTransport carries the session over one characteristic, which is both
// where frames are written and where the panel's answers arrive.
type bleTransport struct {
	characteristic bluetooth.DeviceCharacteristic
	notifications  chan []byte
}

func (t *bleTransport) Notifications() <-chan []byte { return t.notifications }

func (t *bleTransport) Write(frame []byte) error {
	written, err := t.characteristic.Write(frame)
	if err != nil {
		return err
	}
	if written != len(frame) {
		return fmt.Errorf("short write: %d of %d bytes", written, len(frame))
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
