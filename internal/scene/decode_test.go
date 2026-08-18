package scene

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestDecodeRenderRepresentativeScene(t *testing.T) {
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{A: 0xff})
	source.SetNRGBA(1, 0, color.NRGBA{R: 0xff, A: 0xff})
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	scene := `{
		"version": 1,
		"orientation": "landscape",
		"background": "white",
		"root": {
			"type": "absolute",
			"clip": true,
			"children": [
				{"bounds":{"x":0,"y":0,"width":20,"height":14},"node":{"type":"rectangle","fill":"black"}},
				{"bounds":{"x":2,"y":1,"width":16,"height":12},"node":{"type":"text","runs":[{"text":"OK","font":"monaco","size":10,"ink":"white"}]}},
				{"bounds":{"x":22,"y":0,"width":8,"height":8},"node":{"type":"image","source":"` + dataURL + `","options":{"fit":"stretch","sampling":"nearest","dither":"threshold"}}},
				{"bounds":{"x":31,"y":0,"width":12,"height":12},"node":{"type":"path","commands":[{"op":"move","to":{"x":0,"y":11}},{"op":"line","to":{"x":6,"y":0}},{"op":"line","to":{"x":11,"y":11}},{"op":"close"}],"fill":"red"}}
			]
		}
	}`

	result, err := (Decoder{}).Render(strings.NewReader(scene))
	if err != nil {
		t.Fatal(err)
	}
	if result.Frame.Bounds().Size() != image.Pt(display.GiciskyWidth, display.GiciskyHeight) {
		t.Fatalf("frame size = %v", result.Frame.Bounds().Size())
	}
	if len(result.Report.MissingRunes) != 0 || len(result.Report.Warnings) != 0 {
		t.Fatalf("report = %#v", result.Report)
	}
	if got, _ := result.Frame.InkAt(31+6, 4); got != display.InkRed {
		t.Fatalf("path pixel = %v, want red", got)
	}
	payload, err := result.Payload()
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("payload size = %d", len(payload))
	}
}

func TestDecoderRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"document", `{"version":1,"guess":true}`, "unknown field"},
		{"node", `{"version":1,"root":{"type":"rectangle","colour":"red"}}`, "unknown field"},
		{"nested", `{"version":1,"root":{"type":"text","runs":[{"text":"x","font":"monaco","size":10,"colour":"red"}]}}`, "unknown field"},
		{"trailing", `{"version":1} {"version":1}`, "multiple JSON values"},
		{"version", `{"version":2}`, "unsupported version"},
		{"type", `{"version":1,"root":{"type":"button"}}`, "unknown node type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Decoder{}).Decode(strings.NewReader(test.value))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDecoderResolvesImagesRelativeToSceneFile(t *testing.T) {
	directory := t.TempDir()
	asset := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	asset.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, asset); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory+"/asset.png", encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	page := `{"version":1,"root":{"type":"image","source":"asset.png","size":{"width":10,"height":10}}}`
	if err := os.WriteFile(directory+"/page.json", []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Decoder{}).RenderFile(directory + "/page.json")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := result.Frame.InkAt(0, 0); got != display.InkRed {
		t.Fatalf("relative image pixel = %v, want red", got)
	}
}

func TestDecodeRendersYellowInk(t *testing.T) {
	result, err := (Decoder{}).Render(strings.NewReader(`{"version":1,"size":{"width":8,"height":8},"root":{"type":"rectangle","fill":"yellow"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := result.Frame.InkAt(0, 0); got != display.InkYellow {
		t.Fatalf("yellow pixel = %v, want yellow", got)
	}
}

func TestDecodeEveryNodeType(t *testing.T) {
	nodes := []string{
		`{"type":"row","gap":1,"mainAlign":"center","crossAlign":"end","children":[{"basis":2,"grow":1,"node":{"type":"spacer","size":{"width":2,"height":2}}}]}`,
		`{"type":"column","children":[{"node":{"type":"padding","insets":{"top":1,"right":1,"bottom":1,"left":1},"child":{"type":"rectangle","fill":"black"}}}]}`,
		`{"type":"polyline","points":[{"x":0,"y":0},{"x":2,"y":2}],"stroke":{"ink":"black","width":1}}`,
		`{"type":"polygon","points":[{"x":0,"y":0},{"x":4,"y":0},{"x":2,"y":4}],"stroke":{"ink":"black","width":1}}`,
		`{"type":"ellipse","size":{"width":8,"height":6},"stroke":{"ink":"red","width":1}}`,
		`{"type":"arc","size":{"width":8,"height":8},"start":0,"sweep":180,"stroke":{"ink":"black","width":1}}`,
		`{"type":"pie","size":{"width":8,"height":8},"start":0,"sweep":90,"ink":"red"}`,
		`{"type":"chord","size":{"width":8,"height":8},"start":0,"sweep":90,"ink":"black"}`,
		`{"type":"pattern","size":{"width":8,"height":8},"rows":["x.",".r"],"inks":{"x":"black","r":"red"}}`,
		`{"type":"clipPath","size":{"width":8,"height":8},"path":{"commands":[{"op":"move","to":{"x":0,"y":0}},{"op":"line","to":{"x":7,"y":0}},{"op":"line","to":{"x":0,"y":7}},{"op":"close"}]},"child":{"type":"rectangle","fill":"red"}}`,
		`{"type":"clipRect","size":{"width":8,"height":8},"rect":{"x":1,"y":1,"width":6,"height":6},"child":{"type":"rectangle","fill":"black"}}`,
	}
	for index, node := range nodes {
		value := `{"version":1,"size":{"width":32,"height":32},"root":` + node + `}`
		if _, err := (Decoder{}).Render(strings.NewReader(value)); err != nil {
			t.Fatalf("node %d: %v", index, err)
		}
	}
}

func TestDecodePortraitPayload(t *testing.T) {
	value := `{"version":1,"orientation":"portraitClockwise","root":{"type":"rectangle","fill":"red"}}`
	result, err := (Decoder{}).Render(strings.NewReader(value))
	if err != nil {
		t.Fatal(err)
	}
	if result.Frame.Bounds().Size() != image.Pt(128, 296) {
		t.Fatalf("portrait frame = %v", result.Frame.Bounds())
	}
	if payload, err := result.Payload(); err != nil || len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("portrait payload = %d, %v", len(payload), err)
	}
}

func TestHTTPFileRestrictionsConfineSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	asset := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, asset); err != nil {
		t.Fatal(err)
	}
	outsideAsset := outside + "/outside.png"
	if err := os.WriteFile(outsideAsset, encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideAsset, root+"/linked.png"); err != nil {
		t.Fatal(err)
	}
	value := `{"version":1,"root":{"type":"image","source":"linked.png","size":{"width":1,"height":1}}}`
	_, err := (Decoder{BaseDir: root, RestrictFiles: true}).Render(strings.NewReader(value))
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("error = %v", err)
	}
}

func TestInMemoryResourcePrecedesFilesystemAndDataURLRules(t *testing.T) {
	resource := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	resource.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, resource); err != nil {
		t.Fatal(err)
	}
	value := `{"version":1,"root":{"type":"image","source":"../not-allowed.png","size":{"width":2,"height":2}}}`
	result, err := (Decoder{BaseDir: t.TempDir(), RestrictFiles: true, Resources: map[string][]byte{"../not-allowed.png": encoded.Bytes()}}).Render(strings.NewReader(value))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := result.Frame.InkAt(0, 0); got != display.InkRed {
		t.Fatalf("resource pixel = %v, want red", got)
	}
}

func TestDecoderRejectsOversizedImageBeforeDecodingPixels(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	// Change only IHDR. DecodeConfig sees the declared dimensions without
	// allocating the enormous pixel buffer that this test is meant to reject.
	binary.BigEndian.PutUint32(data[16:20], 4097)
	binary.BigEndian.PutUint32(data[20:24], 4096)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))

	source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	value := `{"version":1,"root":{"type":"image","source":"` + source + `"}}`
	_, err := (Decoder{}).Render(strings.NewReader(value))
	if err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("error = %v", err)
	}
}
