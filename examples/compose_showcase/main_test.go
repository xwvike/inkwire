package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestRenderComposeShowcase(t *testing.T) {
	frame, report, err := renderComposeShowcase()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MissingRunes) != 0 {
		t.Fatalf("showcase has missing runes: %q", string(report.MissingRunes))
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("showcase has warnings: %+v", report.Warnings)
	}
	if len(report.Images) != 1 {
		t.Fatalf("image decisions = %d, want one automatic image decision", len(report.Images))
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
	assertMatchesReferencePNG(t, "compose_showcase.png", frame)
}

func assertMatchesReferencePNG(t *testing.T, path string, frame *display.Frame) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reference, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if reference.Bounds() != frame.Bounds() {
		t.Fatalf("reference %s is %v, frame is %v", path, reference.Bounds(), frame.Bounds())
	}
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			want := color.NRGBAModel.Convert(reference.At(x, y))
			if got := color.NRGBAModel.Convert(frame.At(x, y)); got != want {
				t.Fatalf("pixel (%d,%d) = %v, reference %s has %v", x, y, got, path, want)
			}
		}
	}
}
