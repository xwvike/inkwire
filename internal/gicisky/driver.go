package gicisky

import (
	"context"
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

// macHexDigits is the length of a MAC in hex digits once separators are gone.
// The name carries only the last four bytes; the leading FF:FF is shared by

func (d *Driver) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}
