package server

import (
	"bytes"
	"encoding/json"
	"image/color"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A page reaches the service the way a scene document does, and everything a
// scene document is held to holds for it. The parts it refers to come with it:
// nothing here reads a file, so a page sent over the wire cannot name one.
func pageRequest(t *testing.T, path string, parts map[string]string, files map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, content := range parts {
		field, err := writer.CreateFormField(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := field.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		field, err := writer.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := field.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

const servedPage = `<div class="page"><span>OK</span></div>`
const servedCSS = `.page { display: flex; width: 60px; height: 20px; background: white;
                           align-items: center; padding: 0 4px; }
                   span { font-family: monaco; font-size: 10px; }`

func TestRenderTakesAPageAndItsStylesheet(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pageRequest(t, "/v1/render?size=80x30",
		map[string]string{"page": servedPage, "stylesheet": servedCSS}, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Width, Height int
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Width != 80 || decoded.Height != 30 {
		t.Errorf("the page rendered at %dx%d, not the requested target size", decoded.Width, decoded.Height)
	}
}

// The drawing a page hands over travels with it, under the name the img names
// it by. Nothing here reads a file, so a page sent over the wire cannot name
// one that was not sent too.
func TestAPageReachesTheDrawingSentWithIt(t *testing.T) {
	const withPlot = `<div class="page"><img src="assets/dot.svg"></div>`
	const css = `.page { display: flex; width: 40px; height: 40px; background: white; }
	             img { display: block; flex-grow: 1; }`
	handler := New(Config{Logf: func(string, ...any) {}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pageRequest(t, "/v1/render?size=40x40",
		map[string]string{"page": withPlot, "stylesheet": css},
		map[string]string{"assets/dot.svg": `<svg width="40" height="40"><circle cx="20" cy="20" r="15" fill="black"/></svg>`}))
	if response.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", response.Code, response.Body.String())
	}

	// A page whose drawing did not arrive still lays out and still comes
	// back, with the hole named in the report. That is the same answer the
	// command line gives, and for the same reason: the picture is what shows
	// an author what went missing, and refusing it outright would leave them
	// with a status code and nothing to look at.
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, pageRequest(t, "/v1/render?size=40x40",
		map[string]string{"page": withPlot, "stylesheet": css}, nil))
	if missing.Code != http.StatusOK {
		t.Fatalf("a page naming a part nobody sent got %d: %s", missing.Code, missing.Body.String())
	}
	if !strings.Contains(missing.Body.String(), "assets/dot.svg") {
		t.Errorf("the missing part was not named in the report: %s", missing.Body.String())
	}
	if !strings.Contains(missing.Body.String(), "unresolved-drawing") {
		t.Errorf("the report carries no warning about it: %s", missing.Body.String())
	}
}

func TestAPageUsesBitmapSentUnderItsSrcName(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pageRequest(t, "/v1/render?size=16x16",
		map[string]string{
			"page": `<div class="page"><img src="assets/portrait.png"></div>`,
			"stylesheet": `.page { display: flex; width: 16px; height: 16px; background: white; }
			               img { display: block; width: 8px; height: 8px; }`,
		}, map[string]string{
			"assets/portrait.png": string(solidPNG(t, color.NRGBA{A: 0xff})),
		}))
	if response.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAPageCannotReadAnImageFromTheServerWorkingDirectory(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	assetName := filepath.Base(t.TempDir()) + ".png"
	assetPath := filepath.Join(workingDir, assetName)
	if err := os.WriteFile(assetPath, solidPNG(t, color.NRGBA{R: 0xff, A: 0xff}), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(assetPath) })

	handler := New(Config{Logf: func(string, ...any) {}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pageRequest(t, "/v1/render?size=16x16",
		map[string]string{
			"page": `<div class="page"><img src="` + assetName + `"></div>`,
			"stylesheet": `.page { display: flex; width: 16px; height: 16px; background: white; }
			               img { display: block; width: 8px; height: 8px; }`,
		}, nil))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "local image sources are not allowed") {
		t.Fatalf("error does not identify the rejected local image: %s", response.Body.String())
	}
}

// One or the other. A request carrying both is a request whose author does not
// know which one the service will draw, and neither would the author of this.
func TestARequestCarriesAPageOrASceneAndNotBoth(t *testing.T) {
	handler := New(Config{Logf: func(string, ...any) {}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pageRequest(t, "/v1/render", map[string]string{
		"page":  servedPage,
		"scene": `{"version":1,"size":{"width":10,"height":10},"root":{"type":"spacer"}}`,
	}, nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, body = %s", response.Code, response.Body.String())
	}
}
