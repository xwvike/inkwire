package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/xwvike/inkwire/internal/gicisky"
	"tinygo.org/x/bluetooth"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := log.New(os.Stdout, "", 0)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := bluetooth.DefaultAdapter.Enable(); err != nil {
		logger.Printf("enable Bluetooth: %v", err)
		return 1
	}

	args := os.Args[1:]
	if len(args) == 1 && args[0] == "scan" {
		driver := gicisky.NewDriver(bluetooth.DefaultAdapter, gicisky.TargetAddress, logger.Printf)
		device, err := driver.Find(ctx)
		if err != nil {
			logger.Print(err)
			return 1
		}
		fmt.Printf("%s %s RSSI=%d\n", device.Address.String(), device.Name, device.RSSI)
		return 0
	}

	var target, path string
	switch len(args) {
	case 1:
		target, path = gicisky.TargetAddress, args[0]
	case 2:
		target, path = args[0], args[1]
	default:
		fmt.Fprintf(os.Stderr, "usage: %s scan\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s <payload.bin>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s <MAC-or-name> <payload.bin>\n", os.Args[0])
		return 2
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		logger.Printf("read payload: %v", err)
		return 1
	}
	if err := gicisky.ValidatePayload(payload); err != nil {
		logger.Print(err)
		return 1
	}

	logger.Printf("pushing %d bytes to %s", len(payload), target)
	driver := gicisky.NewDriver(bluetooth.DefaultAdapter, target, logger.Printf)
	if err := driver.PushWithRetry(ctx, payload); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}
