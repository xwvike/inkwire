// Package ble holds the Bluetooth machinery both tag families need.
//
// The two families differ in what a tag says about itself and in what is done
// with it once connected. They do not differ in how a radio is listened to, and
// keeping two copies of that meant fixing the same scan twice — or, as
// happened, once.
package ble

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	stopScanRetry = 200 * time.Millisecond
	stopScanLimit = 5 * time.Second
)

// Scan listens for advertisements and hands each one to observe.
//
// observe is called from the adapter's own callback and serialised, so an
// observer needs no lock of its own. stop is consulted after each
// advertisement and ends the scan as soon as it answers true.
//
// A nil stop listens for the whole window. That is what enumerating everything
// nearby has to do: there is no way to know the quietest tag has already been
// heard from, so a listing that stopped early would be a listing that quietly
// omits it. Anything looking for one known tag should pass a stop and get its
// answer in the time the tag takes to speak, not the time the window takes to
// close.
func Scan(ctx context.Context, adapter *bluetooth.Adapter, timeout time.Duration,
	observe func(bluetooth.ScanResult), stop func() bool) error {
	if adapter == nil {
		return errors.New("Bluetooth adapter is nil")
	}
	if observe == nil {
		return errors.New("scan observer must not be nil")
	}
	if timeout <= 0 {
		return fmt.Errorf("scan timeout must be positive, got %s", timeout)
	}

	var mutex sync.Mutex
	var once sync.Once
	satisfied := make(chan struct{})
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			mutex.Lock()
			defer mutex.Unlock()
			observe(result)
			if stop != nil && stop() {
				once.Do(func() { close(satisfied) })
			}
		})
	}()

	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-satisfied:
	case <-scanCtx.Done():
	case err := <-scanDone:
		// The scan ended without being asked to, so there is nothing to stop.
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		return nil
	}
	if err := stopScanning(adapter.StopScan, scanDone); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}

// stopScanning ends a scan and waits for it to say that it has ended.
//
// Asking more than once is not belt and braces. StopScan before Scan has begun
// does nothing at all, and the scan that starts a moment later has nobody left
// to stop it: the wait never returns, and whoever called this holds the adapter
// for the life of the process. The window is wide open whenever the context is
// already cancelled on the way in. That used to be routine — /v1/devices
// scanned the two families in turn, so the second scan began with a context an
// abandoned request had already cancelled, and one client hanging up wedged the
// service until it was restarted. That particular path is gone now that one
// pass serves both families, but a cancelled context arriving here is not.
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
