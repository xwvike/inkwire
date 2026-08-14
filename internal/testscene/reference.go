package testscene

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// updateVariable rewrites the reference images instead of comparing against
// them:
//
//	INKWIRE_UPDATE_REFERENCES=1 go test ./...
//
// It exists because there was no stated way to produce one of these files, so
// they were produced several different ways. Every reference held the right
// pixels and eleven of them held those pixels in a different encoding to the
// rest, which is invisible until something regenerates one and a binary file
// churns for no reason anybody can see. Now there is one way.
//
// It is an environment variable rather than the -update flag this would
// usually be, because go test hands its flags to every package's binary and
// most of them have never heard of this one. A flag here would mean nobody
// could run the whole suite in one command.
//
// A reference is only worth as much as the render that produced it, so look at
// the picture afterwards rather than at the diff, which can only tell you that
// some bytes moved.
const updateVariable = "INKWIRE_UPDATE_REFERENCES"

func AssertMatchesPNG(t *testing.T, path string, frame *display.Frame) {
	t.Helper()
	if os.Getenv(updateVariable) != "" {
		writeReference(t, path, frame)
		return
	}

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

func writeReference(t *testing.T, path string, frame *display.Frame) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, frame); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	t.Logf("rewrote %s", path)
}
