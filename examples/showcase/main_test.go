package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
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
	want, err := os.ReadFile("showcase.png")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := display.WritePNG(&got, frame); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("DisplayList showcase differs from the checked-in reference PNG")
	}
}
