package gicisky

import (
	"context"
	"fmt"

	"github.com/xwvike/inkwire/internal/ble"
	"tinygo.org/x/bluetooth"
)

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
	d.logf("resolved target %s (%s)", found.Name, found.Address.String())

	return ble.Connect(d.Adapter, found.Address, ble.Service{
		Name:            "FEF0",
		UUID:            serviceUUID,
		Characteristics: []bluetooth.UUID{controlUUID, dataUUID},
	}, func(link ble.Link) error {
		// Two exactly: the conversation needs a control channel and a data
		// channel, and guessing which is which from a short list would be a
		// write to the wrong one.
		if len(link.Characteristics) != 2 {
			return fmt.Errorf("discover characteristics FEF1/FEF2: expected 2, got %d", len(link.Characteristics))
		}
		notifications, err := ble.Notifications(link.Characteristics[0], notificationQueue)
		if err != nil {
			return fmt.Errorf("enable FEF1 notifications: %w", err)
		}
		return d.Uploader.UploadWithOptions(ctx, &bleTransport{
			control:       link.Characteristics[0],
			data:          link.Characteristics[1],
			notifications: notifications,
		}, payload, options)
	})
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

// notificationQueue is how many messages the tag may volunteer before one is
// read. The tag answers each write, so the queue only has to cover the tag
// speaking twice before this side looks.
const notificationQueue = 16

type bleTransport struct {
	control       bluetooth.DeviceCharacteristic
	data          bluetooth.DeviceCharacteristic
	notifications <-chan []byte
}

func (t *bleTransport) Notifications() <-chan []byte { return t.notifications }

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
