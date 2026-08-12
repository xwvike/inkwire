package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

const testScene = `{"version":1,"root":{"type":"absolute","children":[{"bounds":{"x":0,"y":0,"width":20,"height":10},"node":{"type":"rectangle","fill":"red"}}]}}`

func TestRenderAndEncode(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})

	render := request(t, handler, "/v1/render", testScene)
	if render.Code != http.StatusOK || render.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("render = %d %q: %s", render.Code, render.Header().Get("Content-Type"), render.Body.String())
	}
	image, err := png.Decode(render.Body)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != display.GiciskyWidth || image.Bounds().Dy() != display.GiciskyHeight {
		t.Fatalf("render bounds = %v", image.Bounds())
	}

	encoded := request(t, handler, "/v1/encode", testScene)
	if encoded.Code != http.StatusOK || encoded.Body.Len() != display.GiciskyPayloadSize {
		t.Fatalf("encode = %d, %d bytes: %s", encoded.Code, encoded.Body.Len(), encoded.Body.String())
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
	handler := New(Config{Logf: func(string, ...any) {}, Push: func(_ context.Context, selected string, bytes []byte) error {
		target = selected
		payload = append([]byte(nil), bytes...)
		return nil
	}})
	response := request(t, handler, "/v1/display?device=PICKSMART", testScene)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if target != "PICKSMART" || len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("push target=%q payload=%d", target, len(payload))
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
	frame, err := png.Decode(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(frame.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("uploaded resource pixel = %#v", got)
	}
}

func TestMultipartDisplayPushesUploadedImage(t *testing.T) {
	scene := `{"version":1,"root":{"type":"image","source":"photo.png","size":{"width":8,"height":8}}}`
	var payload []byte
	handler := New(Config{Logf: func(string, ...any) {}, Push: func(_ context.Context, _ string, data []byte) error {
		payload = append([]byte(nil), data...)
		return nil
	}})
	response := multipartRequest(t, handler, "/v1/display", scene, map[string][]byte{"photo.png": solidPNG(t, color.NRGBA{A: 0xff})})
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
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
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
