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
	"strings"
	"sync"
	"time"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
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
	// An EPD-nRF5 write is a different shape and needs its own budget. The
	// panel is not drawn when the last frame is acknowledged: the connection
	// has to be held open while it refreshes, which is half a minute on a
	// three colour part and is not time this server can shorten. One attempt
	// is a scan, a connection, the frames and that wait; this clears two of
	// them, which is a failed scan and a full retry.
	DefaultNRFEPDPushTimeout = 150 * time.Second
)

type Config struct {
	Adapter *bluetooth.Adapter
	Target  string
	BaseDir string
	// Attempts and PushTimeout bound one /v1/display request. Zero selects
	// the defaults above. PushTimeout applies to a Gicisky write;
	// NRFEPDPushTimeout to the other family, which needs longer for reasons
	// that are the panel's rather than this server's.
	Attempts          int
	PushTimeout       time.Duration
	NRFEPDPushTimeout time.Duration
	// ScanTimeout bounds one /v1/devices request. Zero selects the driver's
	// own default, which is sized to the tag's advertising interval.
	ScanTimeout time.Duration
	Logf        func(string, ...any)
	Push        func(context.Context, string, []byte) error
	Scan        func(context.Context) ([]gicisky.FoundDevice, error)
	// PushPage and ScanNRFEPD are the same two hooks for the other family.
	// A page rather than a payload, because that family will not say what
	// panel it has until it has been connected to, so what to build cannot
	// be known before the write starts.
	PushPage   func(context.Context, string, nrfepd.PageFor) error
	ScanNRFEPD func(context.Context) ([]nrfepd.FoundDevice, error)
}

// suppliesItsOwnTransport reports whether the caller has taken the radio out
// of this handler's hands.
//
// Supplying any one of the four hooks says so, and the ones left unset then do
// nothing rather than falling through to the adapter. Falling through is the
// behaviour that looks harmless and is not: a caller who stubbed the Gicisky
// scan to keep a test off the hardware would find the second family scanning
// for real, fifteen seconds at a time, on whatever tags happen to be in the
// room.
func (c Config) suppliesItsOwnTransport() bool {
	return c.Scan != nil || c.Push != nil || c.ScanNRFEPD != nil || c.PushPage != nil
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
	// Family says which driver reaches this tag, and is the field that
	// decides what the rest of this struct can say. An EPD-nRF5 tag keeps
	// its panel in its own flash and mentions it in no advertisement, so
	// everything from Model down is absent for one and present for the
	// other. Absent here means unknowable without connecting, not missing.
	Family string `json:"family"`
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

	nrfepdPushTimeout time.Duration
	pushPage          func(context.Context, string, nrfepd.PageFor) error
	scanNRFEPD        func(context.Context) ([]nrfepd.FoundDevice, error)
	ownTransport      bool

	// The adapter is enabled at most once for the life of the handler. The
	// call is not idempotent: it succeeds the first time and reports "already
	// calling Enable function" for ever after, so a server that enabled per
	// request would serve exactly one request and then refuse every other.
	enableOnce sync.Once
	enableErr  error

	mu      sync.Mutex
	active  string
	since   time.Time
	history map[string]DeviceStatus
}

func (h *Handler) enableAdapter() error {
	h.enableOnce.Do(func() { h.enableErr = h.adapter.Enable() })
	if h.enableErr != nil {
		return fmt.Errorf("enable Bluetooth: %w", h.enableErr)
	}
	return nil
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
	if config.NRFEPDPushTimeout <= 0 {
		config.NRFEPDPushTimeout = DefaultNRFEPDPushTimeout
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

		nrfepdPushTimeout: config.NRFEPDPushTimeout,
		pushPage:          config.PushPage,
		scanNRFEPD:        config.ScanNRFEPD,
		ownTransport:      config.suppliesItsOwnTransport(),
		mux:               http.NewServeMux(),
		history:           make(map[string]DeviceStatus),
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
	// One radio runs one scan, so the families are looked for in turn and the
	// budget covers both.
	ctx, cancel := context.WithTimeout(request.Context(), 2*h.scanTimeout+5*time.Second)
	defer cancel()
	devices, err := h.scanBothFamilies(ctx)
	h.release(scanHolder, 0, err)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "scan-failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"devices": devices})
}

func (h *Handler) scanBothFamilies(ctx context.Context) ([]Device, error) {
	found, err := h.scanDevices(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(found))
	for _, device := range found {
		devices = append(devices, describeDevice(device))
	}
	others, err := h.scanNRFEPDDevices(ctx)
	if err != nil {
		return nil, err
	}
	for _, device := range others {
		devices = append(devices, Device{
			Address: device.Address.String(), Name: device.Name, RSSI: device.RSSI,
			Family: familyNRFEPD,
			// Drivable without being described: this family is written to by
			// asking it what it is first, so not knowing the panel here is
			// the normal case rather than the refusal it is for a Gicisky tag.
			Drivable: true,
		})
	}
	return devices, nil
}

func describeDevice(found gicisky.FoundDevice) Device {
	device := Device{Address: found.Address.String(), Name: found.Name, RSSI: found.RSSI, Family: familyGicisky}
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
	if err := h.enableAdapter(); err != nil {
		return nil, err
	}
	driver := h.newDriver("")
	driver.ScanTimeout = h.scanTimeout
	return driver.ScanAll(ctx)
}

func (h *Handler) scanNRFEPDDevices(ctx context.Context) ([]nrfepd.FoundDevice, error) {
	if h.scanNRFEPD != nil {
		return h.scanNRFEPD(ctx)
	}
	if h.ownTransport {
		return nil, nil
	}
	if err := h.enableAdapter(); err != nil {
		return nil, err
	}
	driver := nrfepd.NewDriver(h.adapter, "", h.logf)
	driver.ScanTimeout = h.scanTimeout
	return driver.ScanAll(ctx)
}

// The two families a target can belong to. They are strings rather than a type
// because they leave this package as JSON and arrive as a query parameter.
const (
	familyGicisky = "gicisky"
	familyNRFEPD  = "nrfepd"
)

// resolveFamily decides which driver a request wants, the same way the command
// line does: a name settles it, an address does not, and saying so outright
// always wins. Sending one family's bytes to the other does not fail politely.
func resolveFamily(requested, target string) (string, error) {
	switch requested {
	case familyGicisky, familyNRFEPD:
		return requested, nil
	case "", "auto":
		if strings.HasPrefix(strings.ToUpper(target), nrfepd.NamePrefix) {
			return familyNRFEPD, nil
		}
		return familyGicisky, nil
	}
	return "", fmt.Errorf("unknown family %q: use auto, %s or %s", requested, familyGicisky, familyNRFEPD)
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
	target := request.URL.Query().Get("device")
	if target == "" {
		target = h.target
	}
	family, err := resolveFamily(request.URL.Query().Get("family"), target)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid-request", err)
		return
	}
	// A Gicisky page is encoded before anything is connected to, because that
	// family's size is fixed and known. The other one cannot be: the panel is
	// asked what it is first and the page is built for the answer, which is
	// why the two take different routes from here.
	var payload []byte
	if family == familyGicisky {
		payload, err = result.Payload()
		if err != nil {
			writeError(writer, http.StatusUnprocessableEntity, "unprocessable-scene", err)
			return
		}
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
	timeout := h.pushTimeout
	if family == familyNRFEPD {
		timeout = h.nrfepdPushTimeout
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()

	var pushErr error
	var written int
	if family == familyNRFEPD {
		written, pushErr = h.pushPageTo(ctx, target, result.Frame)
	} else {
		written, pushErr = len(payload), h.pushPayload(ctx, target, payload)
	}
	status := h.release(target, written, pushErr)
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
		"ok": true, "device": target, "family": family, "bytes": written,
		"status": status, "report": result.Report,
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
	if err := h.enableAdapter(); err != nil {
		return err
	}
	return h.newDriver(target).PushWithRetry(ctx, payload)
}

// pushPageTo sends a rendered page to an EPD-nRF5 tag, reporting how many
// bytes of panel data it turned out to be.
//
// The count comes back rather than being known in advance because the page is
// not encoded until the panel has said what it is. A page whose size does not
// match is refused here, at the one moment both are known.
func (h *Handler) pushPageTo(ctx context.Context, target string, frame *display.Frame) (int, error) {
	written := 0
	page := func(model nrfepd.Model) ([]byte, []byte, error) {
		if frame.Width() != model.Width || frame.Height() != model.Height {
			return nil, nil, fmt.Errorf("the page is %dx%d and the panel is %s; render it at the panel's size",
				frame.Width(), frame.Height(), model)
		}
		black, colour, err := display.EncodeNRFEPD(frame, model.Palette != nrfepd.PaletteBW)
		written = len(black) + len(colour)
		return black, colour, err
	}
	if h.pushPage != nil {
		// Called before the count is read: the page builds the planes, so
		// written means nothing until it has run.
		err := h.pushPage(ctx, target, page)
		return written, err
	}
	if h.ownTransport {
		return 0, errors.New("this handler was given its own transport and no PushPage hook, so it cannot reach an EPD-nRF5 tag")
	}
	if err := h.enableAdapter(); err != nil {
		return 0, err
	}
	driver := nrfepd.NewDriver(h.adapter, target, h.logf)
	driver.Attempts = h.attempts
	err := driver.PushWithRetry(ctx, page)
	return written, err
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
