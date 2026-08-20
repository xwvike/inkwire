// Package gicisky drives the factory-firmware BLE price tags sold as Gicisky
// and advertised as PICKSMART.
//
// A tag of this family says what panel it has without being connected to. The
// model id, the firmware version and the battery voltage are all in the
// advertisement, so a page can be rendered and encoded for the right panel
// before anything is connected to — which is precisely what the other family
// cannot do, and the reason writing to the two takes different shapes.
//
// Two things follow from that and are easy to get wrong. The name and the
// manufacturer data arrive in separate advertisements, so identifying one tag
// means merging packets by address rather than reading one packet. And the
// panel table is somebody else's work: only the 2.9" BWR entry has been held
// up against hardware here, which is what Profile.Verified records and why an
// unrecognised panel is reported rather than quietly driven.
package gicisky

import (
	"context"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
	"tinygo.org/x/bluetooth"
)

const (
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

// macHexDigits is the length of a MAC in hex digits once separators are gone.
// The name carries only the last four bytes; the leading FF:FF is shared by

func (d *Driver) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}
