// Package nrfepd drives e-paper tags running the EPD-nRF5 replacement
// firmware, https://github.com/tsl0922/EPD-nRF5.
//
// A tag of this family advertises a name and nothing else. What panel is
// attached lives in the firmware's own flash and comes out only once the tag
// has been connected to and asked, so a page cannot be built in advance: the
// conversation learns the panel first and renders for the answer. That is the
// one difference from the other family that cannot be designed away.
//
// Two more come from the firmware rather than the panel. A page is run-length
// encoded when that makes it smaller and sent plain when it would not, because
// the link is slow enough for the difference to matter. And the connection is
// held open after the refresh command rather than dropped: disconnecting
// cancels the redraw, which is why DefaultSettle is a correctness setting and
// not a courtesy.
package nrfepd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
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
	return ble.Retry{Attempts: attempts, Delay: delay, Logf: d.Logf}.Do(ctx, "", attempt)
}
