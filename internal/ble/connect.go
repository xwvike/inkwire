package ble

import (
	"errors"
	"fmt"

	"tinygo.org/x/bluetooth"
)

// Service names the one service a connection is opened for, and the
// characteristics wanted from it.
//
// Name is how errors refer to the service. A bare UUID in a message is
// complete and useless; "FEF0" and "EPD" are what the datasheets and this
// repository's notes call them.
type Service struct {
	Name            string
	UUID            bluetooth.UUID
	Characteristics []bluetooth.UUID
}

// Link is an open connection and what was discovered on it.
type Link struct {
	Device          bluetooth.Device
	Characteristics []bluetooth.DeviceCharacteristic
}

// Connect opens one device, discovers one service on it and the
// characteristics named, and hands them to use. The connection is closed when
// use returns, whatever it returns.
//
// How many characteristics are enough is left to the caller, because the two
// families disagree: one needs exactly two and refuses anything else, the
// other needs one and treats a second as a bonus it will read a version out of
// if it is there. So this reports what was discovered rather than judging it.
func Connect(adapter *bluetooth.Adapter, address bluetooth.Address, service Service, use func(Link) error) error {
	if adapter == nil {
		return errors.New("Bluetooth adapter is nil")
	}
	if use == nil {
		return errors.New("connection callback must not be nil")
	}
	device, err := adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer device.Disconnect()

	services, err := device.DiscoverServices([]bluetooth.UUID{service.UUID})
	if err != nil {
		return fmt.Errorf("discover the %s service: %w", service.Name, err)
	}
	if len(services) != 1 {
		return fmt.Errorf("discover the %s service: expected 1, found %d", service.Name, len(services))
	}
	characteristics, err := services[0].DiscoverCharacteristics(service.Characteristics)
	if err != nil {
		return fmt.Errorf("discover the %s characteristics: %w", service.Name, err)
	}
	return use(Link{Device: device, Characteristics: characteristics})
}

// Notifications turns a characteristic's notifications into a channel.
//
// The send never blocks. A notification arrives on the Bluetooth stack's own
// callback, so a queue that has filled has to be dropped into rather than
// waited on: a full queue means nothing is reading it any more, and waiting
// there stalls the stack itself.
//
// Both families reach here now. Only one of them used to do this. The Gicisky
// driver sent without a default, so an upload that returned early — an error
// mid-conversation, a retry giving up — left notifications enabled with nobody
// reading, and the next message the tag volunteered would block the callback
// for good. Nothing was seen to hit it, and putting the two side by side is
// what made it visible.
func Notifications(characteristic bluetooth.DeviceCharacteristic, size int) (<-chan []byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("notification queue must hold at least one message, got %d", size)
	}
	messages := make(chan []byte, size)
	err := characteristic.EnableNotifications(func(buffer []byte) {
		// The buffer is only valid for the life of the callback.
		select {
		case messages <- append([]byte(nil), buffer...):
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	return messages, nil
}
