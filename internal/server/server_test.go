package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/tag"
	"tinygo.org/x/bluetooth"
)

const testScene = `{"version":1,"root":{"type":"absolute","children":[{"bounds":{"x":0,"y":0,"width":20,"height":10},"node":{"type":"rectangle","fill":"red"}}]}}`

func TestRenderReturnsJSONReport(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})

	render := request(t, handler, "/v1/render", testScene)
	if render.Code != http.StatusOK || render.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("render = %d %q: %s", render.Code, render.Header().Get("Content-Type"), render.Body.String())
	}
	frame, report := decodeRenderResponse(t, render)
	if frame.Bounds().Dx() != display.GiciskyWidth || frame.Bounds().Dy() != display.GiciskyHeight {
		t.Fatalf("render bounds = %v", frame.Bounds())
	}
	if report.Bounds.Empty() {
		t.Fatal("render response omitted its report")
	}
}

func TestHTTPDoesNotExposeRawEncoder(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := request(t, handler, "/v1/encode", testScene)
	if response.Code != http.StatusNotFound {
		t.Fatalf("encode status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body.String())
	}
}

func TestInvalidSceneReturnsJSONError(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := request(t, handler, "/v1/render", `{"version":1,"root":{"type":"rectangle","colour":"red"}}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["error"], "unknown field") {
		t.Fatalf("error = %q", body["error"])
	}
}

func TestHTTPAssetRootRejectsTraversalAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("not needed"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(Config{BaseDir: root, Logf: func(string, ...any) {}})
	for _, source := range []string{"../outside.png", outside, "file://" + outside} {
		body := `{"version":1,"root":{"type":"image","source":` + quote(source) + `,"size":{"width":10,"height":10}}}`
		response := request(t, handler, "/v1/render", body)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("source %q status = %d: %s", source, response.Code, response.Body.String())
		}
	}
}

func TestDisplayUsesSameSceneAndSelectedDevice(t *testing.T) {
	var target string
	var payload []byte
	handler := New(Config{
		Logf: func(string, ...any) {},
		Scan: scanFor(t, "PICKSMART", 0x0033),
		Push: func(_ context.Context, selected string, bytes []byte) error {
			target = selected
			payload = append([]byte(nil), bytes...)
			return nil
		},
	})
	response := request(t, handler, "/v1/display?device=PICKSMART", testScene)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if target != "PICKSMART" || len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("push target=%q payload=%d", target, len(payload))
	}
}

func TestDisplayRendersGiciskySceneForTheIdentifiedPanel(t *testing.T) {
	var payload []byte
	handler := New(Config{
		Logf: func(string, ...any) {},
		Scan: scanFor(t, "PICKSMART", 0x004B),
		Push: func(_ context.Context, _ string, data []byte) error {
			payload = append([]byte(nil), data...)
			return nil
		},
	})
	response := request(t, handler, "/v1/display?device=PICKSMART", testScene)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if got, want := len(payload), 400*300/8*2; got != want {
		t.Fatalf("payload length = %d, want %d", got, want)
	}
}

// One adapter drives one conversation, so a second write must be refused
// while the first is still uploading rather than corrupting it or queueing
// invisibly behind a ten-second transfer.
func TestConcurrentDisplayIsRefusedWithTheHolderStatus(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	// Only the first write holds the adapter open; later ones complete at
	// once so the test can check that the claim was handed back.
	var hold sync.Once
	handler := New(Config{
		Logf: func(string, ...any) {},
		Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
			return []gicisky.FoundDevice{
				scanResult(t, 0x01, -40, 0x0033, "first"),
				scanResult(t, 0x02, -41, 0x0033, "second"),
			}, nil
		},
		Push: func(context.Context, string, []byte) error {
			hold.Do(func() {
				entered <- struct{}{}
				<-release
			})
			return nil
		},
	})

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- request(t, handler, "/v1/display?device=first", testScene) }()
	<-entered

	second := request(t, handler, "/v1/display?device=second", testScene)
	if second.Code != http.StatusConflict {
		t.Fatalf("second write status = %d, want %d: %s", second.Code, http.StatusConflict, second.Body.String())
	}
	var refusal struct {
		Code   string         `json:"code"`
		Status DeviceStatus   `json:"status"`
		Report compose.Report `json:"report"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Code != "device-busy" {
		t.Errorf("code = %q, want device-busy", refusal.Code)
	}
	// The refusal has to name the device actually holding the adapter, not
	// the one that was asked for, or it cannot be acted on.
	if refusal.Status.Device != "first" || refusal.Status.State != "pushing" {
		t.Errorf("status = %+v, want the first device reported as pushing", refusal.Status)
	}
	if refusal.Status.Since == "" {
		t.Error("a pushing device reported no start time")
	}
	close(release)
	if got := <-first; got.Code != http.StatusOK {
		t.Fatalf("first write status = %d: %s", got.Code, got.Body.String())
	}

	// Once released the adapter is free again and the outcome is recorded.
	third := request(t, handler, "/v1/display?device=first", testScene)
	if third.Code != http.StatusOK {
		t.Fatalf("write after release status = %d: %s", third.Code, third.Body.String())
	}
	var success struct {
		Status DeviceStatus `json:"status"`
	}
	if err := json.Unmarshal(third.Body.Bytes(), &success); err != nil {
		t.Fatal(err)
	}
	if success.Status.State != "idle" || success.Status.LastResult != "ok" {
		t.Errorf("status after a successful write = %+v", success.Status)
	}
	if success.Status.LastBytes != display.GiciskyPayloadSize {
		t.Errorf("recorded %d bytes, want %d", success.Status.LastBytes, display.GiciskyPayloadSize)
	}
}

// Exceeding the handler's own deadline means the retries never got the tag to
// answer, which is a different problem from a scene the server cannot render.
func TestPushDeadlineReportsTheBluetoothLink(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}, PushTimeout: 20 * time.Millisecond,
		Scan: scanFor(t, "NEMR92943861", 0x0033),
		Push: func(ctx context.Context, _ string, _ []byte) error {
			<-ctx.Done()
			return ctx.Err()
		}})
	response := request(t, handler, "/v1/display?device=NEMR92943861", testScene)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusGatewayTimeout, response.Body.String())
	}
	var body struct {
		Code   string         `json:"code"`
		Status DeviceStatus   `json:"status"`
		Report compose.Report `json:"report"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "device-timeout" {
		t.Errorf("code = %q, want device-timeout", body.Code)
	}
	if body.Status.LastResult == "ok" || body.Status.LastBytes != 0 {
		t.Errorf("a timed-out write was recorded as %+v", body.Status)
	}
	// A failed write still releases the adapter, otherwise one bad tag wedges
	// the server until it is restarted.
	if body.Status.State != "idle" {
		t.Errorf("state after a failed write = %q, want idle", body.Status.State)
	}
	if body.Report.Bounds.Empty() {
		t.Error("device-timeout response omitted the completed render report")
	}
}

// A push that fails for a reason other than the deadline is a different code,
// so a caller can distinguish "the tag refused" from "the tag never answered".
func TestFailedPushIsReportedSeparatelyFromATimeout(t *testing.T) {
	handler := New(Config{
		Logf: func(string, ...any) {},
		Scan: scanFor(t, "NEMR92943861", 0x0033),
		Push: func(context.Context, string, []byte) error {
			return errors.New("tag reported error 0503")
		},
	})
	response := request(t, handler, "/v1/display?device=NEMR92943861", testScene)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadGateway, response.Body.String())
	}
	var body struct {
		Code   string         `json:"code"`
		Status DeviceStatus   `json:"status"`
		Report compose.Report `json:"report"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "push-failed" {
		t.Errorf("code = %q, want push-failed", body.Code)
	}
	if body.Status.LastResult != "tag reported error 0503" {
		t.Errorf("last result = %q, want the driver error", body.Status.LastResult)
	}
	if body.Report.Bounds.Empty() {
		t.Error("push-failed response omitted the completed render report")
	}
}

// A code only earns its keep if it is the one the caller will branch on, so
// every reachable failure is triggered here rather than assumed.
//
// render-failed is deliberately absent: it guards a PNG encode into a
// bytes.Buffer, which cannot fail for a frame the compiler has already
// produced. It stays as a defensive branch with no reachable test.
func TestEveryErrorCarriesACode(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	tests := []struct {
		name    string
		request func() *http.Request
		code    string
	}{
		{"invalid scene", func() *http.Request {
			return jsonRequest("/v1/render", `{"version":1,"root":{"type":"nonesuch"}}`)
		}, "invalid-scene"},
		{"oversized scene", func() *http.Request {
			return jsonRequest("/v1/render", `{"version":1,"padding":"`+strings.Repeat("x", maxSceneBytes)+`"}`)
		}, "request-too-large"},
		{"unusable Content-Type", func() *http.Request {
			request := jsonRequest("/v1/render", testScene)
			request.Header.Set("Content-Type", "!!! not a media type")
			return request
		}, "unsupported-media-type"},
		{"wrong Content-Type", func() *http.Request {
			request := jsonRequest("/v1/render", testScene)
			request.Header.Set("Content-Type", "text/plain")
			return request
		}, "unsupported-media-type"},
		{"multipart without a scene", func() *http.Request {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, _ := writer.CreateFormFile("photo.png", "photo.png")
			_, _ = part.Write([]byte("not an image"))
			_ = writer.Close()
			request := httptest.NewRequest(http.MethodPost, "/v1/render", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			return request
		}, "invalid-request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, test.request())
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("status %d body %q: %v", response.Code, response.Body.String(), err)
			}
			if body["code"] != test.code {
				t.Errorf("code = %q, want %q (status %d, %q)", body["code"], test.code, response.Code, body["error"])
			}
			if body["error"] == "" {
				t.Error("the code replaced the message instead of joining it")
			}
			if response.Code < 400 {
				t.Errorf("status = %d, want a failure", response.Code)
			}
		})
	}
}

// Tests replace Push outright, so the cap the server hands the real driver is
// never exercised by them.
func TestDriverGetsTheServersAttemptCap(t *testing.T) {
	if got := New(Config{Attempts: 2, Logf: func(string, ...any) {}}).newDriver("PICKSMART").Attempts; got != 2 {
		t.Errorf("driver attempts = %d, want the configured cap 2", got)
	}
	if got := New(Config{Logf: func(string, ...any) {}}).newDriver("PICKSMART").Attempts; got != DefaultAttempts {
		t.Errorf("driver attempts = %d, want the default %d", got, DefaultAttempts)
	}
}

// The budget is the one number a caller feels, so it is checked against what
// the hardware actually costs rather than left as a round figure.
func TestPushBudgetSurvivesAMissedScanAndStillRetries(t *testing.T) {
	const slowestHealthyWrite = 20521 * time.Millisecond

	if DefaultPushTimeout <= slowestHealthyWrite {
		t.Fatalf("budget %s cuts the slowest measured healthy write %s", DefaultPushTimeout, slowestHealthyWrite)
	}
	// A scan can miss the tag's advertising window, so the budget has to
	// cover one wasted attempt and a complete write after it. Without this
	// the server would report a Bluetooth fault for an ordinary retry.
	recovery := gicisky.DefaultScanTimeout + gicisky.DefaultRetryDelay + slowestHealthyWrite
	if DefaultPushTimeout < recovery {
		t.Errorf("budget %s leaves no room to recover from one missed scan, which needs %s", DefaultPushTimeout, recovery)
	}
}

func TestRejectsOversizedScene(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	body := `{"version":1,"padding":"` + strings.Repeat("x", maxSceneBytes) + `"}`
	response := request(t, handler, "/v1/render", body)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func TestMultipartResourceOverridesAssetDirectory(t *testing.T) {
	root := t.TempDir()
	if err := writeSolidPNG(filepath.Join(root, "portrait.png"), color.NRGBA{A: 0xff}); err != nil {
		t.Fatal(err)
	}
	scene := `{"version":1,"root":{"type":"image","source":"portrait.png","size":{"width":8,"height":8},"options":{"dither":"threshold"}}}`
	uploaded := solidPNG(t, color.NRGBA{R: 0xff, A: 0xff})
	handler := New(Config{BaseDir: root, Logf: func(string, ...any) {}})
	response := multipartRequest(t, handler, "/v1/render", scene, map[string][]byte{"portrait.png": uploaded})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	frame, _ := decodeRenderResponse(t, response)
	if got := color.NRGBAModel.Convert(frame.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("uploaded resource pixel = %#v", got)
	}
}

func TestRenderReportsImplicitGridTracksInBody(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	scene := `{
		"version":1,"size":{"width":100,"height":20},
		"root":{"type":"grid","columns":["1fr","1fr"],"children":[
			{"row":1,"columnSpan":2,"node":{"type":"rectangle","fill":"black"}},
			{"row":1,"node":{"type":"rectangle","size":{"width":10,"height":10},"fill":"red"}}
		]}
	}`
	response := request(t, handler, "/v1/render", scene)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	_, report := decodeRenderResponse(t, response)
	if len(report.GridExpansions) != 1 || report.GridExpansions[0].ImplicitColumns != 1 {
		t.Fatalf("grid expansions = %+v", report.GridExpansions)
	}
}

func TestMultipartDisplayPushesUploadedImage(t *testing.T) {
	scene := `{"version":1,"root":{"type":"image","source":"photo.png","size":{"width":8,"height":8}}}`
	var payload []byte
	handler := New(Config{
		Logf: func(string, ...any) {},
		Scan: scanFor(t, "NEMR92943861", 0x0033),
		Push: func(_ context.Context, _ string, data []byte) error {
			payload = append([]byte(nil), data...)
			return nil
		},
	})
	response := multipartRequest(t, handler, "/v1/display?device=NEMR92943861", scene, map[string][]byte{"photo.png": solidPNG(t, color.NRGBA{A: 0xff})})
	if response.Code != http.StatusOK || len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("status=%d payload=%d: %s", response.Code, len(payload), response.Body.String())
	}
}

func TestMultipartValidation(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	tests := []struct {
		name      string
		scene     *string
		resources map[string][]byte
	}{
		{"missing scene", nil, map[string][]byte{"photo.png": solidPNG(t, color.NRGBA{})}},
		{"duplicate resource", ptr(testScene), map[string][]byte{"photo.png": nil}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if test.scene != nil {
				part, _ := writer.CreateFormField("scene")
				_, _ = part.Write([]byte(*test.scene))
			}
			for name, content := range test.resources {
				for range 2 {
					part, _ := writer.CreateFormFile(name, name)
					_, _ = part.Write(content)
					if test.name != "duplicate resource" {
						break
					}
				}
			}
			_ = writer.Close()
			request := httptest.NewRequest(http.MethodPost, "/v1/render", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func request(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, jsonRequest(path, body))
	return response
}

func jsonRequest(path, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func multipartRequest(t *testing.T, handler http.Handler, path, scene string, resources map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	scenePart, err := writer.CreateFormField("scene")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scenePart.Write([]byte(scene)); err != nil {
		t.Fatal(err)
	}
	for name, content := range resources {
		part, err := writer.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func solidPNG(t *testing.T, ink color.NRGBA) []byte {
	t.Helper()
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			source.SetNRGBA(x, y, ink)
		}
	}
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func decodeRenderResponse(t *testing.T, response *httptest.ResponseRecorder) (image.Image, compose.Report) {
	t.Helper()
	var body struct {
		Width     int            `json:"width"`
		Height    int            `json:"height"`
		PNGBase64 string         `json:"pngBase64"`
		Report    compose.Report `json:"report"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode render JSON: %v", err)
	}
	encoded, err := base64.StdEncoding.DecodeString(body.PNGBase64)
	if err != nil {
		t.Fatalf("decode render PNG base64: %v", err)
	}
	frame, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode render PNG: %v", err)
	}
	if body.Width != frame.Bounds().Dx() || body.Height != frame.Bounds().Dy() {
		t.Fatalf("reported render size %dx%d, PNG is %v", body.Width, body.Height, frame.Bounds())
	}
	return frame, body.Report
}

func writeSolidPNG(path string, ink color.NRGBA) error {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			source.SetNRGBA(x, y, ink)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, source)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func ptr(value string) *string { return &value }

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// zeroAddress is what an Address stringifies to when Set has been given a
// literal of the wrong kind, which it ignores in silence.
var zeroAddress = func() string { var address bluetooth.Address; return address.String() }()

// scanResult builds one found tag. The address is checked rather than assumed
// because an Address is a CoreBluetooth UUID on macOS and a MAC on Linux: a
// literal of the wrong kind leaves every test device sharing the zero address,
// and the test then passes without distinguishing them at all.
// nrfResult stubs one tag of the other family. A write now has to find the tag
// before it can address it, so a test that stubs the push must stub the scan.
func nrfResult(t *testing.T, seed byte, rssi int16, name string) nrfepd.FoundDevice {
	t.Helper()
	device := nrfepd.FoundDevice{Name: name, RSSI: rssi}
	for _, literal := range []string{
		fmt.Sprintf("0000f00d-0000-1000-8000-00805f9b%04x", uint16(seed)),
		fmt.Sprintf("AA:BB:CC:DD:EE:%02X", seed),
	} {
		device.Address.Set(literal)
		if device.Address.String() != zeroAddress {
			break
		}
	}
	if device.Address.String() == zeroAddress {
		t.Fatal("no address literal this test knows is accepted on this platform")
	}
	return device
}

func scanResult(t *testing.T, seed byte, rssi int16, id uint16, name string) gicisky.FoundDevice {
	t.Helper()
	device := gicisky.FoundDevice{Name: name, RSSI: rssi}
	for _, literal := range []string{
		fmt.Sprintf("0000f00d-0000-1000-8000-00805f9b%04x", uint16(seed)),
		fmt.Sprintf("AA:BB:CC:DD:EE:%02X", seed),
	} {
		device.Address.Set(literal)
		if device.Address.String() != zeroAddress {
			break
		}
	}
	if device.Address.String() == zeroAddress {
		t.Fatal("no address literal this test knows is accepted on this platform")
	}
	device.Advertised = gicisky.Advertisement{ID: id}
	device.HasAdvertised = true
	device.Profile, device.Identified = gicisky.LookupProfile(id, 0)
	return device
}

func scanFor(t *testing.T, name string, id uint16) func(context.Context) ([]gicisky.FoundDevice, error) {
	return func(context.Context) ([]gicisky.FoundDevice, error) {
		return []gicisky.FoundDevice{scanResult(t, 0x01, -40, id, name)}, nil
	}
}

func TestDevicesReportsWhatEachTagIs(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}, Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
		return []gicisky.FoundDevice{
			scanResult(t, 0x01, -40, 0x0033, "NEMR000001"),
			// Present, advertising, and not in this build's table.
			scanResult(t, 0x02, -70, 0x3FFE, ""),
		}, nil
	}})
	request := httptest.NewRequest(http.MethodGet, "/v1/devices", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Devices) != 2 {
		t.Fatalf("reported %d devices, want 2", len(body.Devices))
	}

	known := body.Devices[0]
	if known.Model != `EPD 2.9" BWR` || known.Width != 296 || known.Height != 128 || known.Palette != "BWR" {
		t.Errorf("identified tag = %+v", known)
	}
	if !known.Drivable || !known.Verified || known.ID != "0x0033" {
		t.Errorf("identified tag should be drivable and verified: %+v", known)
	}

	// The whole point of reporting an unknown tag is that it is visible but
	// must not be written to, since its page size would be a guess.
	unknown := body.Devices[1]
	if unknown.Drivable {
		t.Error("an unrecognised tag was reported as drivable")
	}
	if unknown.ID != "0x3FFE" {
		t.Errorf("id = %q, want the advertised id so it can be reported upstream", unknown.ID)
	}
	if unknown.Width != 0 || unknown.Model != "" {
		t.Errorf("an unrecognised tag was given a size or model: %+v", unknown)
	}
	if unknown.Address == "" {
		t.Error("an unrecognised tag was reported without an address")
	}
}

// Scanning and writing share one radio, so a scan during a write must be
// refused the same way a second write is.
func TestScanIsRefusedWhileTheAdapterIsWriting(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var hold sync.Once
	handler := New(Config{Logf: func(string, ...any) {},
		Push: func(context.Context, string, []byte) error {
			hold.Do(func() {
				entered <- struct{}{}
				<-release
			})
			return nil
		},
		Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
			return []gicisky.FoundDevice{scanResult(t, 0x01, -40, 0x0033, "writing")}, nil
		}})

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request(t, handler, "/v1/display?device=writing", testScene) }()
	<-entered

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("scan during a write = %d, want %d: %s", response.Code, http.StatusConflict, response.Body.String())
	}
	var refusal struct {
		Code   string       `json:"code"`
		Status DeviceStatus `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Code != "device-busy" || refusal.Status.Device != "writing" {
		t.Errorf("refusal = %+v, want device-busy naming the writing device", refusal)
	}

	close(release)
	<-done

	// The scan must succeed once the write is done, and must not have left a
	// history entry of its own behind.
	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if after.Code != http.StatusOK {
		t.Fatalf("scan after the write = %d: %s", after.Code, after.Body.String())
	}
	handler.mu.Lock()
	_, recorded := handler.history[scanHolder]
	handler.mu.Unlock()
	if recorded {
		t.Error("scanning left a per-device history entry, but it writes to no device")
	}
}

func TestScanFailureIsReportedWithACode(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}, Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
		return nil, errors.New("enable Bluetooth: adapter is off")
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "scan-failed" {
		t.Errorf("code = %q, want scan-failed", body["code"])
	}
	// A failed scan still hands the radio back.
	handler.mu.Lock()
	active := handler.active
	handler.mu.Unlock()
	if active != "" {
		t.Errorf("the adapter is still held by %q after a failed scan", active)
	}
}

// An EPD-nRF5 page cannot be encoded before the write starts, because the
// panel has not said what it is yet. So this route hands over a page rather
// than a payload, and the size check happens at the one moment both the page
// and the panel are known.
func TestDisplayBuildsAnNRFEPDPageForThePanelItFinds(t *testing.T) {
	var gotTarget string
	var black, colour []byte
	handler := New(Config{Logf: func(string, ...any) {},
		ScanNRFEPD: func(context.Context) ([]nrfepd.FoundDevice, error) {
			return []nrfepd.FoundDevice{nrfResult(t, 0x11, -60, "NRF_EPD_C1F8")}, nil
		},
		Push: func(context.Context, string, []byte) error {
			t.Error("the Gicisky driver was used for an EPD-nRF5 target")
			return nil
		},
		PushPage: func(_ context.Context, target string, page nrfepd.PageFor) error {
			gotTarget = target
			model, _ := nrfepd.LookupModelName("UC8176_420_BWR")
			var err error
			black, colour, err = page(model)
			return err
		}})

	scene := `{"version":1,"size":{"width":400,"height":300},"root":{"type":"absolute","children":[]}}`
	response := request(t, handler, "/v1/display?device=NRF_EPD_C1F8", scene)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if gotTarget != "NRF_EPD_C1F8" {
		t.Errorf("target = %q", gotTarget)
	}
	// Two planes of a 400x300 panel, fifty bytes to a row.
	if len(black) != 50*300 || len(colour) != 50*300 {
		t.Errorf("planes are %d and %d bytes, want %d each", len(black), len(colour), 50*300)
	}
	var body struct {
		Family string `json:"family"`
		Bytes  int    `json:"bytes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Family != tag.NRFEPD || body.Bytes != 2*50*300 {
		t.Errorf("body reports family %q and %d bytes", body.Family, body.Bytes)
	}
}

// This family does not say what panel it has until it is connected to, so a
// page cannot be built before then. It used to be built anyway and refused if
// the panel turned out to disagree, which made the caller guess a size in
// order to be allowed to send anything.
//
// Now the page is laid out for whatever panel answered, and a scene that
// stated a different one is told so rather than turned away.
func TestDisplayLaysAnNRFEPDPageOutForThePanelThatAnswered(t *testing.T) {
	const statesTwoNineInch = `{"version":1,"size":{"width":296,"height":128},` +
		`"root":{"type":"absolute","children":[{"bounds":{"x":0,"y":0,"width":20,"height":10},` +
		`"node":{"type":"rectangle","fill":"red"}}]}}`

	handler := New(Config{Logf: func(string, ...any) {},
		ScanNRFEPD: func(context.Context) ([]nrfepd.FoundDevice, error) {
			return []nrfepd.FoundDevice{nrfResult(t, 0x11, -60, "NRF_EPD_C1F8")}, nil
		},
		PushPage: func(_ context.Context, _ string, page nrfepd.PageFor) error {
			model, _ := nrfepd.LookupModelName("UC8176_420_BWR")
			_, _, err := page(model)
			return err
		}})
	response := request(t, handler, "/v1/display?device=NRF_EPD_C1F8", statesTwoNineInch)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body struct {
		Bytes  int `json:"bytes"`
		Report struct {
			Warnings []struct{ Path, Code, Message string }
		} `json:"report"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// The planes are the panel's, not the scene's: 400x300 over two planes.
	if want := 2 * 400 * 300 / 8; body.Bytes != want {
		t.Errorf("wrote %d bytes, want %d for the panel that answered", body.Bytes, want)
	}
	var found bool
	for _, w := range body.Report.Warnings {
		if w.Code == "size-mismatch" {
			found = true
			for _, mention := range []string{"296x128", "400x300"} {
				if !strings.Contains(w.Message, mention) {
					t.Errorf("the warning does not mention %s: %s", mention, w.Message)
				}
			}
		}
	}
	if !found {
		t.Errorf("nothing warned that the scene stated another panel: %+v", body.Report.Warnings)
	}
}

// Which family a tag belongs to comes from the scan that found it, not from
// the shape of the name somebody typed. A Gicisky tag called anything at all is
// still a Gicisky tag, and an address — which is what most targets are — says
// nothing either way.
//
// ?family= is obeyed and then checked against that answer, because writing one
// family's bytes to the other is not a polite failure.
func TestDisplayTakesTheFamilyFromTheScanAndChecksWhatWasAsserted(t *testing.T) {
	tests := []struct {
		name, query string
		wantGicisky bool
		refused     string
	}{
		{name: "an EPD-nRF5 tag by name", query: "?device=NRF_EPD_C1F8"},
		{name: "a Gicisky tag by name", query: "?device=NEMR92943861", wantGicisky: true},
		{
			// The name is a Gicisky one but the scan put it in the other
			// family's list, and the scan is what counts.
			name: "the scan overrules the name", query: "?device=NRF_EPD_C1F8&family=nrfepd",
		},
		{
			name: "an assertion the tag contradicts", query: "?device=NRF_EPD_C1F8&family=gicisky",
			refused: "is a nrfepd tag, but the family asked for is gicisky",
		},
		{
			name: "a target nothing answers to", query: "?device=NEMR000000FF",
			refused: "no tag matching",
		},
		{name: "a misspelled family", query: "?device=NEMR92943861&family=nrf", refused: "unknown family"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usedGicisky, nrf := false, false
			handler := New(Config{Logf: func(string, ...any) {},
				Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
					return []gicisky.FoundDevice{scanResult(t, 0x01, -40, 0x0033, "NEMR92943861")}, nil
				},
				ScanNRFEPD: func(context.Context) ([]nrfepd.FoundDevice, error) {
					return []nrfepd.FoundDevice{nrfResult(t, 0x02, -42, "NRF_EPD_C1F8")}, nil
				},
				Push:     func(context.Context, string, []byte) error { usedGicisky = true; return nil },
				PushPage: func(context.Context, string, nrfepd.PageFor) error { nrf = true; return nil }})

			// A 296x128 scene so the Gicisky route can encode it.
			response := request(t, handler, "/v1/display"+test.query, testScene)
			if test.refused != "" {
				if usedGicisky || nrf {
					t.Fatalf("a refused request still wrote: gicisky=%v nrfepd=%v", usedGicisky, nrf)
				}
				if !strings.Contains(response.Body.String(), test.refused) {
					t.Fatalf("refusal does not say %q: %s", test.refused, response.Body.String())
				}
				return
			}
			if usedGicisky != test.wantGicisky || nrf == test.wantGicisky {
				t.Errorf("used gicisky=%v nrfepd=%v: %s", usedGicisky, nrf, response.Body.String())
			}
		})
	}
}

func TestDisplayRefusesAnUnknownFamily(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := request(t, handler, "/v1/display?family=nrf", testScene)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

// Both families come back from one listing, and the one that cannot say what
// panel it has says that rather than looking like a tag that failed to be
// recognised.
func TestDevicesListsBothFamilies(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {},
		Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
			return []gicisky.FoundDevice{{Name: "NEMR92943861", RSSI: -50}}, nil
		},
		ScanNRFEPD: func(context.Context) ([]nrfepd.FoundDevice, error) {
			return []nrfepd.FoundDevice{{Name: "NRF_EPD_C1F8", RSSI: -59}}, nil
		}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Devices) != 2 {
		t.Fatalf("listed %d devices, want one of each family", len(body.Devices))
	}
	byFamily := map[string]Device{}
	for _, device := range body.Devices {
		byFamily[device.Family] = device
	}
	if _, ok := byFamily[tag.Gicisky]; !ok {
		t.Error("no gicisky tag in the listing")
	}
	other, ok := byFamily[tag.NRFEPD]
	if !ok {
		t.Fatal("no nrfepd tag in the listing")
	}
	if other.Model != "" || other.Width != 0 {
		t.Errorf("an nrfepd tag reported a panel it cannot know: %+v", other)
	}
	if !other.Drivable {
		t.Error("an nrfepd tag was reported as not drivable; it is written by asking it what it is")
	}
}

// Supplying any transport hook means this handler has no radio. The families
// left unstubbed must then do nothing, because the alternative is a test that
// quietly scans the room for fifteen seconds per request.
func TestAStubbedHandlerNeverReachesForTheRadio(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {},
		Scan: func(context.Context) ([]gicisky.FoundDevice, error) {
			return []gicisky.FoundDevice{{Name: "NEMR92943861"}}, nil
		}})
	start := time.Now()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("the listing took %s, which means it went to the radio", elapsed)
	}
	var body struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Devices) != 1 || body.Devices[0].Family != tag.Gicisky {
		t.Errorf("listing = %+v, want only the stubbed family", body.Devices)
	}

	// And the write path refuses from what it was given rather than reaching
	// for a radio it was told it does not have. It is told about one Gicisky
	// tag, so a request for the other family is answered out of that: nothing
	// here matches, and here is what does.
	write := request(t, handler, "/v1/display?device=NRF_EPD_C1F8", testScene)
	if write.Code != http.StatusBadGateway {
		t.Fatalf("write status = %d, want %d: %s", write.Code, http.StatusBadGateway, write.Body.String())
	}
	for _, want := range []string{"no tag matching", "NEMR92943861"} {
		if !strings.Contains(write.Body.String(), want) {
			t.Errorf("the refusal does not say %q: %s", want, write.Body.String())
		}
	}
}
