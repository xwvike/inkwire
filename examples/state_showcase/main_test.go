package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestRenderStateShowcase(t *testing.T) {
	frame, err := renderStateShowcase()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width() != display.GiciskyWidth || frame.Height() != display.GiciskyHeight {
		t.Fatalf("state showcase dimensions = %dx%d", frame.Width(), frame.Height())
	}
	payload, err := display.EncodeGicisky(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("state showcase payload = %d bytes", len(payload))
	}
	want, err := os.ReadFile("state_showcase.png")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := display.WritePNG(&got, frame); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("state showcase differs from the checked-in reference PNG")
	}
}
