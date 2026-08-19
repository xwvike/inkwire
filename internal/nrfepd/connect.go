package nrfepd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tinygo.org/x/bluetooth"
)

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
