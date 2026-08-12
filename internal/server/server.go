package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/scene"
	"tinygo.org/x/bluetooth"
)

const (
	maxSceneBytes   = 16 << 20
	maxAssetBytes   = 32 << 20
	maxRequestBytes = 64 << 20
	maxAssets       = 32
)

type Config struct {
	Adapter *bluetooth.Adapter
	Target  string
	BaseDir string
	Logf    func(string, ...any)
	Push    func(context.Context, string, []byte) error
}

type Handler struct {
	adapter *bluetooth.Adapter
	target  string
	baseDir string
	logf    func(string, ...any)
	push    func(context.Context, string, []byte) error
	mux     *http.ServeMux
}

func New(config Config) *Handler {
	if config.Adapter == nil {
		config.Adapter = bluetooth.DefaultAdapter
	}
	if config.Target == "" {
		config.Target = gicisky.TargetAddress
	}
	if config.Logf == nil {
		config.Logf = log.Printf
	}
	handler := &Handler{adapter: config.Adapter, target: config.Target, baseDir: config.BaseDir, logf: config.Logf, push: config.Push, mux: http.NewServeMux()}
	handler.mux.HandleFunc("POST /v1/render", handler.render)
	handler.mux.HandleFunc("POST /v1/encode", handler.encode)
	handler.mux.HandleFunc("POST /v1/display", handler.display)
	return handler
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.mux.ServeHTTP(writer, request)
}

func (h *Handler) render(writer http.ResponseWriter, request *http.Request) {
	result, ok := h.renderRequest(writer, request)
	if !ok {
		return
	}
	writeReportHeaders(writer.Header(), result)
	var encoded bytes.Buffer
	if err := display.WritePNG(&encoded, result.Frame); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writer.Header().Set("Content-Type", "image/png")
	writer.Header().Set("Content-Length", fmt.Sprint(encoded.Len()))
	_, _ = writer.Write(encoded.Bytes())
}

func (h *Handler) encode(writer http.ResponseWriter, request *http.Request) {
	result, ok := h.renderRequest(writer, request)
	if !ok {
		return
	}
	payload, err := result.Payload()
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err)
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
	writeReportHeaders(writer.Header(), result)
	_, _ = writer.Write(payload)
}

func (h *Handler) display(writer http.ResponseWriter, request *http.Request) {
	result, ok := h.renderRequest(writer, request)
	if !ok {
		return
	}
	payload, err := result.Payload()
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err)
		return
	}
	target := request.URL.Query().Get("device")
	if target == "" {
		target = h.target
	}
	if err := h.pushPayload(request.Context(), target, payload); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "device": target, "bytes": len(payload), "report": result.Report})
}

func (h *Handler) pushPayload(ctx context.Context, target string, payload []byte) error {
	if h.push != nil {
		return h.push(ctx, target, payload)
	}
	if err := h.adapter.Enable(); err != nil {
		return fmt.Errorf("enable Bluetooth: %w", err)
	}
	driver := gicisky.NewDriver(h.adapter, target, h.logf)
	return driver.PushWithRetry(ctx, payload)
}

func (h *Handler) renderRequest(writer http.ResponseWriter, request *http.Request) (scene.Result, bool) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		writeError(writer, http.StatusUnsupportedMediaType, fmt.Errorf("invalid Content-Type: %w", err))
		return scene.Result{}, false
	}
	var sceneBytes []byte
	var resources map[string][]byte
	switch mediaType {
	case "application/json":
		request.Body = http.MaxBytesReader(writer, request.Body, maxSceneBytes)
		sceneBytes, err = io.ReadAll(request.Body)
	case "multipart/form-data":
		request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
		sceneBytes, resources, err = readMultipart(request)
	default:
		writeError(writer, http.StatusUnsupportedMediaType, fmt.Errorf("Content-Type must be application/json or multipart/form-data"))
		return scene.Result{}, false
	}
	if err != nil {
		status := http.StatusBadRequest
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) || errors.Is(err, errPartTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(writer, status, err)
		return scene.Result{}, false
	}
	result, err := (scene.Decoder{BaseDir: h.baseDir, RestrictFiles: true, Resources: resources}).Render(bytes.NewReader(sceneBytes))
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err)
		return scene.Result{}, false
	}
	return result, true
}

var errPartTooLarge = errors.New("multipart part exceeds its size limit")

func readMultipart(request *http.Request) ([]byte, map[string][]byte, error) {
	reader, err := request.MultipartReader()
	if err != nil {
		return nil, nil, fmt.Errorf("parse multipart request: %w", err)
	}
	var sceneBytes []byte
	resources := make(map[string][]byte)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read multipart request: %w", err)
		}
		name := part.FormName()
		if name == "" {
			_ = part.Close()
			return nil, nil, fmt.Errorf("multipart part has no form name")
		}
		limit := int64(maxAssetBytes)
		if name == "scene" {
			limit = maxSceneBytes
		}
		content, err := readPart(part, limit)
		closeErr := part.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("multipart part %q: %w", name, err)
		}
		if closeErr != nil {
			return nil, nil, fmt.Errorf("close multipart part %q: %w", name, closeErr)
		}
		if name == "scene" {
			if sceneBytes != nil {
				return nil, nil, fmt.Errorf("multipart request contains more than one scene part")
			}
			sceneBytes = content
			continue
		}
		if part.FileName() == "" {
			return nil, nil, fmt.Errorf("multipart resource %q must be a file part", name)
		}
		if _, exists := resources[name]; exists {
			return nil, nil, fmt.Errorf("multipart request contains duplicate resource %q", name)
		}
		if len(resources) >= maxAssets {
			return nil, nil, fmt.Errorf("multipart request contains more than %d resources", maxAssets)
		}
		resources[name] = content
	}
	if sceneBytes == nil {
		return nil, nil, fmt.Errorf("multipart request has no scene part")
	}
	return sceneBytes, resources, nil
}

func readPart(reader io.Reader, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errPartTooLarge
	}
	return content, nil
}

func writeReportHeaders(header http.Header, result scene.Result) {
	header.Set("X-Inkwire-Warnings", fmt.Sprint(len(result.Report.Warnings)))
	header.Set("X-Inkwire-Missing-Runes", fmt.Sprint(len(result.Report.MissingRunes)))
	header.Set("X-Inkwire-Image-Decisions", fmt.Sprint(len(result.Report.Images)))
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]any{"error": err.Error()})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
