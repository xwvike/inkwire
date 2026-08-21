package compose

import (
	"encoding/json"
	"image"
	"reflect"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// A report leaves this program as JSON, and it used to leave it speaking Go.
// The envelope around it was camelCase while the report's own fields were the
// Go field names; a rectangle went out as a Min and a Max point; and the runes
// no font could draw went out as the integers behind them, so a caller was
// handed arithmetic instead of characters.
func TestAReportLeavesInTheSameLanguageAsWhatCarriesIt(t *testing.T) {
	report := Report{
		Bounds:       image.Rect(2, 3, 42, 23),
		MissingRunes: []rune("翯𡃁"),
		Warnings:     []Warning{{Path: "root", Code: "text-clipped", Message: "does not fit"}},
		Images: []ImageDecision{{
			Path:    "root.children[0]",
			Options: display.ImageOptions{Fit: display.FitCover, Sampling: display.SampleBilinear, Dither: display.DitherFloydSteinberg, Threshold: 128},
			Profile: ImageProfile{Photographic: true, Threshold: 97},
		}},
		GridExpansions: []GridExpansion{{Path: "root", ImplicitRows: 2}},
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}

	// Every key the report sends is camelCase, the way the envelope's are.
	for key := range wire {
		if key == "" || strings.ToLower(key[:1]) != key[:1] {
			t.Errorf("report key %q is not camelCase", key)
		}
	}

	// A rectangle is an origin and a size, like every other rectangle here.
	bounds, ok := wire["bounds"].(map[string]any)
	if !ok {
		t.Fatalf("bounds = %T, want an object", wire["bounds"])
	}
	for key, want := range map[string]float64{"x": 2, "y": 3, "width": 40, "height": 20} {
		if bounds[key] != want {
			t.Errorf("bounds.%s = %v, want %v", key, bounds[key], want)
		}
	}
	if _, leaked := bounds["Min"]; leaked {
		t.Error("bounds still sends Min and Max")
	}

	// The characters no font could draw arrive as characters.
	if got := wire["missingRunes"]; got != "翯𡃁" {
		t.Errorf("missingRunes = %#v, want the text itself", got)
	}

	// The enums arrive as the words a scene uses to ask for them.
	options := wire["images"].([]any)[0].(map[string]any)["options"].(map[string]any)
	for key, want := range map[string]string{"fit": "cover", "sampling": "bilinear", "dither": "floydSteinberg"} {
		if options[key] != want {
			t.Errorf("options.%s = %#v, want %q", key, options[key], want)
		}
	}

	// And it comes back the way it went out, because a caller that decodes one
	// of these has the same right to the Go shape as the one that made it.
	var round Report
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(round, report) {
		t.Errorf("round trip = %+v,\n            want %+v", round, report)
	}
}

// A clean render says so by having nothing to say, rather than by four keys
// holding null.
func TestAReportWithNothingToSayIsJustItsBounds(t *testing.T) {
	encoded, err := json.Marshal(Report{Bounds: image.Rect(0, 0, 296, 128)})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"bounds":{"x":0,"y":0,"width":296,"height":128}}`; got != want {
		t.Errorf("empty report = %s,\n           want %s", got, want)
	}
}
