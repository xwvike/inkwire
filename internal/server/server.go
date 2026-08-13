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
	"sync"
	"time"

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

const (
	// Three measured writes to a healthy tag took 14.6, 18.1 and 20.5
	// seconds, almost all of it spent scanning and connecting. The budget
	// below clears the slowest of those twice over, so one failed scan and
	// a full retry still fit; a request held longer than that has stopped
	// being a slow tag and become a Bluetooth problem worth reporting.
	DefaultAttempts    = 3
	DefaultPushTimeout = 45 * time.Second
)

type Config struct {
	Adapter *bluetooth.Adapter
	Target  string
	BaseDir string
	// Attempts and PushTimeout bound one /v1/display request. Zero selects
	// the defaults above.
	Attempts    int
	PushTimeout time.Duration
	// ScanTimeout bounds one /v1/devices request. Zero selects the driver's
	// own default, which is sized to the tag's advertising interval.
	ScanTimeout time.Duration
	Logf        func(string, ...any)
	Push        func(context.Context, string, []byte) error
	Scan        func(context.Context) ([]gicisky.FoundDevice, error)
}

// scanHolder names the radio's holder while a scan is running. Scanning and
// writing share one adapter, so they share one claim; this is not a device and
// therefore never enters the per-device history.
const scanHolder = "(scan)"

// Device is one tag as the scan found it. Only fields something acts on are
// reported: the model table also carries rotation, mirroring and compression,
// but nothing encodes with them yet, and an unread field is one that drifts.
type Device struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	RSSI    int16  `json:"rssi"`
	// Voltage is the tag's own reading. No charge percentage accompanies it:
	// a coin cell's voltage curve is not linear, and a derived percentage
	// would read as a measurement rather than as the guess it would be.
	Voltage  float64 `json:"voltage,omitempty"`
	ID       string  `json:"id,omitempty"`
	Model    string  `json:"model,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Palette  string  `json:"palette,omitempty"`
	Verified bool    `json:"verified"`
	// Drivable is false when the tag is present but this build cannot say
	// what panel it has, which is exactly when writing to it would be a
	// guess rather than a render.
	Drivable bool `json:"drivable"`
}

// DeviceStatus is everything the server knows about one panel. It rides along
// with every refused or failed write so a caller can tell a busy adapter from
// a tag that is not answering.
type DeviceStatus struct {
	Device     string `json:"device"`
	State      string `json:"state"`
	Since      string `json:"since,omitempty"`
	LastWrite  string `json:"lastWrite,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
	LastBytes  int    `json:"lastBytes,omitempty"`
}

type Handler struct {
	adapter     *bluetooth.Adapter
	target      string
	baseDir     string
	attempts    int
	pushTimeout time.Duration
	scanTimeout time.Duration
	logf        func(string, ...any)
	push        func(context.Context, string, []byte) error
	scan        func(context.Context) ([]gicisky.FoundDevice, error)
	mux         *http.ServeMux

	mu      sync.Mutex
	active  string
	since   time.Time
	history map[string]DeviceStatus
}

func New(config Config) *Handler {
	if config.Adapter == nil {
		config.Adapter = bluetooth.DefaultAdapter
	}
	if config.Target == "" {
		config.Target = gicisky.TargetAddress
	}
	if config.Attempts <= 0 {
		config.Attempts = DefaultAttempts
	}
	if config.PushTimeout <= 0 {
		config.PushTimeout = DefaultPushTimeout
	}
	if config.ScanTimeout <= 0 {
		config.ScanTimeout = gicisky.DefaultScanTimeout
	}
	if config.Logf == nil {
		config.Logf = log.Printf
	}
	handler := &Handler{
		adapter:     config.Adapter,
		target:      config.Target,
		baseDir:     config.BaseDir,
		attempts:    config.Attempts,
		pushTimeout: config.PushTimeout,
		scanTimeout: config.ScanTimeout,
		logf:        config.Logf,
		push:        config.Push,
		scan:        config.Scan,
		mux:         http.NewServeMux(),
		history:     make(map[string]DeviceStatus),
	}
	handler.mux.HandleFunc("POST /v1/render", handler.render)
	handler.mux.HandleFunc("POST /v1/encode", handler.encode)
	handler.mux.HandleFunc("POST /v1/display", handler.display)
	handler.mux.HandleFunc("GET /v1/devices", handler.devices)
	return handler
}

// devices reports every tag advertising nearby and what panel each one has.
// It takes the same claim a write does, because both need the radio.
func (h *Handler) devices(writer http.ResponseWriter, request *http.Request) {
	busy, claimed := h.claim(scanHolder)
	if !claimed {
		writeStatus(writer, http.StatusConflict, "device-busy",
			fmt.Errorf("the adapter is busy writing %s", busy.Device), busy)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.scanTimeout+5*time.Second)
	defer cancel()
	found, err := h.scanDevices(ctx)
	h.release(scanHolder, 0, err)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "scan-failed", err)
		return
	}
	devices := make([]Device, 0, len(found))
	for _, device := range found {
		devices = append(devices, describeDevice(device))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"devices": devices})
}

func describeDevice(found gicisky.FoundDevice) Device {
	device := Device{Address: found.Address.String(), Name: found.Name, RSSI: found.RSSI}
	if found.HasAdvertised {
		device.ID = fmt.Sprintf("0x%04X", found.Advertised.ID)
		device.Voltage = found.Advertised.Voltage()
	}
	if !found.Identified {
		return device
	}
	device.Model = found.Profile.Model
	device.Width = found.Profile.Width
	device.Height = found.Profile.Height
	device.Palette = found.Profile.Palette.String()
	device.Verified = found.Profile.Verified
	device.Drivable = true
	return device
}

func (h *Handler) scanDevices(ctx context.Context) ([]gicisky.FoundDevice, error) {
	if h.scan != nil {
		return h.scan(ctx)
	}
	if err := h.adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable Bluetooth: %w", err)
	}
	driver := h.newDriver("")
	driver.ScanTimeout = h.scanTimeout
	return driver.ScanAll(ctx)
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
		writeError(writer, http.StatusInternalServerError, "render-failed", err)
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
		writeError(writer, http.StatusUnprocessableEntity, "unprocessable-scene", err)
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
		writeError(writer, http.StatusUnprocessableEntity, "unprocessable-scene", err)
		return
	}
	target := request.URL.Query().Get("device")
	if target == "" {
		target = h.target
	}

	busy, claimed := h.claim(target)
	if !claimed {
		// One adapter can hold one conversation, so a second write is
		// refused outright instead of queued behind a ten-second upload the
		// caller cannot see.
		writeStatus(writer, http.StatusConflict, "device-busy",
			fmt.Errorf("device %s is being written", busy.Device), busy)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.pushTimeout)
	defer cancel()
	pushErr := h.pushPayload(ctx, target, payload)
	status := h.release(target, len(payload), pushErr)
	if pushErr != nil {
		code, httpStatus := "push-failed", http.StatusBadGateway
		// The deadline belongs to this handler, so exceeding it means the
		// retries never got the tag to answer: report the Bluetooth link,
		// not the scene.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		writeStatus(writer, httpStatus, code, pushErr, status)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "device": target, "bytes": len(payload), "status": status, "report": result.Report,
	})
}

// claim makes a write exclusive across every device, because exclusivity is a
// property of the single Bluetooth adapter rather than of the target tag. The
// returned status describes whoever holds it.
func (h *Handler) claim(target string) (DeviceStatus, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active != "" {
		return h.statusLocked(h.active), false
	}
	h.active = target
	h.since = time.Now()
	return h.statusLocked(target), true
}

func (h *Handler) release(target string, size int, err error) DeviceStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	if target == scanHolder {
		// A scan writes nothing, so it leaves no per-device history behind.
		h.active = ""
		return DeviceStatus{Device: target, State: "idle"}
	}
	record := h.history[target]
	record.Device = target
	record.LastWrite = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		record.LastResult = err.Error()
		record.LastBytes = 0
	} else {
		record.LastResult = "ok"
		record.LastBytes = size
	}
	h.history[target] = record
	h.active = ""
	record.State = "idle"
	return record
}

func (h *Handler) statusLocked(device string) DeviceStatus {
	status := h.history[device]
	status.Device = device
	status.State = "idle"
	if h.active == device {
		status.State = "pushing"
		status.Since = h.since.UTC().Format(time.RFC3339)
	}
	return status
}

func (h *Handler) pushPayload(ctx context.Context, target string, payload []byte) error {
	if h.push != nil {
		return h.push(ctx, target, payload)
	}
	if err := h.adapter.Enable(); err != nil {
		return fmt.Errorf("enable Bluetooth: %w", err)
	}
	return h.newDriver(target).PushWithRetry(ctx, payload)
}

// newDriver is separate so the attempt cap can be checked without a radio.
// Every test here replaces Push outright, which left this wiring unexercised.
func (h *Handler) newDriver(target string) *gicisky.Driver {
	driver := gicisky.NewDriver(h.adapter, target, h.logf)
	driver.Attempts = h.attempts
	return driver
}

func (h *Handler) renderRequest(writer http.ResponseWriter, request *http.Request) (scene.Result, bool) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		writeError(writer, http.StatusUnsupportedMediaType, "unsupported-media-type", fmt.Errorf("invalid Content-Type: %w", err))
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
		writeError(writer, http.StatusUnsupportedMediaType, "unsupported-media-type", fmt.Errorf("Content-Type must be application/json or multipart/form-data"))
		return scene.Result{}, false
	}
	if err != nil {
		status, code := http.StatusBadRequest, "invalid-request"
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) || errors.Is(err, errPartTooLarge) {
			status, code = http.StatusRequestEntityTooLarge, "request-too-large"
		}
		writeError(writer, status, code, err)
		return scene.Result{}, false
	}
	result, err := (scene.Decoder{BaseDir: h.baseDir, RestrictFiles: true, Resources: resources}).Render(bytes.NewReader(sceneBytes))
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "invalid-scene", err)
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

// Every failure carries a stable code so a caller can branch on the kind of
// problem without matching on the human-readable message.
func writeError(writer http.ResponseWriter, status int, code string, err error) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code})
}

func writeStatus(writer http.ResponseWriter, status int, code string, err error, device DeviceStatus) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code, "status": device})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
