package gicisky

import (
	"context"
	"errors"
	"fmt"

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
