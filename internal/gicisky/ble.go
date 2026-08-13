package gicisky

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	TargetAddress = "FF:FF:92:94:38:61"
	TargetName    = "PICKSMART"
	FallbackName  = "NEMR92943861"

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
		_ = d.Adapter.StopScan()
		<-scanDone
		return FoundDevice{}, fmt.Errorf("PICKSMART tag not found (target %s): %w", d.Target, scanCtx.Err())
	}
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
	if !strings.EqualFold(d.Target, TargetAddress) {
		return false
	}
	return strings.EqualFold(name, TargetName) || strings.EqualFold(name, FallbackName)
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
