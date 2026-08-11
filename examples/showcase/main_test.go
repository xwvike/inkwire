package main

import (
	"testing"

	"inkwire/internal/display"
)

func TestRenderShowcase(t *testing.T) {
	frame, err := renderShowcase()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width() != display.GiciskyWidth || frame.Height() != display.GiciskyHeight {
		t.Fatalf("showcase dimensions = %dx%d", frame.Width(), frame.Height())
	}
	payload, err := display.EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("showcase payload = %d bytes", len(payload))
	}
}
