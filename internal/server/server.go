// Package server exposes rendering and writing over HTTP, for the callers that
// are not a command line: a cron job, a home automation rule, a phone.
//
// One radio can hold one conversation, so a write is claimed exclusively and a
// second one is refused outright rather than queued behind ten seconds the
// caller cannot see. Everything else follows from that: the budgets, the
// device-busy status, and the fact that this binds loopback only. There is no
// authentication and every request reaches hardware, so the address is the
// whole of the access control.
package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/panel"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/tag"
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
	// stays open for a 30-second refresh after scanning and transfer. Real
	// writes have completed inside the server's existing 60-second window;
	// retries share this total budget rather than multiplying it.
	DefaultNRFEPDPushTimeout = 60 * time.Second
)

type Config struct {
	Adapter *bluetooth.Adapter
	BaseDir string
	// Attempts and PushTimeout bound one /v1/push request. Zero selects
	// the defaults above. PushTimeout applies to a Gicisky write;
	// NRFEPDPushTimeout to the other family, which needs longer for reasons
	// that are the panel's rather than this server's.
	Attempts          int
	PushTimeout       time.Duration
	NRFEPDPushTimeout time.Duration
	// ScanTimeout bounds one /v1/scan request. Zero selects the driver's
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
	// SetMode stands in for the conversation that hands a tag back to its own
	// clock. It is separate from PushPage because it is a different exchange,
	// not a page written a different way.
	SetMode func(context.Context, string, nrfepd.Mode, *time.Weekday) error
}

// suppliesItsOwnTransport reports whether the caller has taken the radio out
// of this handler's hands.
//
// Supplying any one of the five hooks says so, and the ones left unset then do
// nothing rather than falling through to the adapter. Falling through is the
// behaviour that looks harmless and is not: a caller who stubbed the Gicisky
// scan to keep a test off the hardware would find the second family scanning
// for real, fifteen seconds at a time, on whatever tags happen to be in the
// room.
func (c Config) suppliesItsOwnTransport() bool {
	return c.Scan != nil || c.Push != nil || c.ScanNRFEPD != nil || c.PushPage != nil || c.SetMode != nil
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
	setMode           func(context.Context, string, nrfepd.Mode, *time.Weekday) error
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
		setMode:           config.SetMode,
		ownTransport:      config.suppliesItsOwnTransport(),
		mux:               http.NewServeMux(),
		history:           make(map[string]DeviceStatus),
	}
	// The routes are named after the subcommands that do the same thing. They
	// used to be /v1/scan and /v1/push, which meant one program with two
	// vocabularies — and the old set was not even consistent with itself, one
	// resource noun among two verbs.
	handler.mux.HandleFunc("GET /v1/scan", handler.serveScan)
	handler.mux.HandleFunc("POST /v1/render", handler.serveRender)
	handler.mux.HandleFunc("POST /v1/push", handler.servePush)
	handler.mux.HandleFunc("POST /v1/mode", handler.serveMode)
	return handler
}

// devices reports every tag advertising nearby and what panel each one has.
// It takes the same claim a write does, because both need the radio.
func (h *Handler) serveScan(writer http.ResponseWriter, request *http.Request) {
	busy, claimed := h.claim(scanHolder)
	if !claimed {
		writeStatus(writer, http.StatusConflict, "device-busy",
			fmt.Errorf("the adapter is busy writing %s", busy.Device), busy)
		return
	}
	// One pass of the radio covers both families, so the budget is one window
	// and the slack to stop it in.
	ctx, cancel := context.WithTimeout(request.Context(), h.scanTimeout+5*time.Second)
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
	found, others, err := h.scanRadio(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(found)+len(others))
	for _, device := range found {
		devices = append(devices, describeDevice(device))
	}
	for _, device := range others {
		devices = append(devices, Device{
			Address: device.Address.String(), Name: device.Name, RSSI: device.RSSI,
			Family: tag.NRFEPD,
			// Drivable without being described: this family is written to by
			// asking it what it is first, so not knowing the panel here is
			// the normal case rather than the refusal it is for a Gicisky tag.
			Drivable: true,
		})
	}
	return devices, nil
}

func describeDevice(found gicisky.FoundDevice) Device {
	device := Device{Address: found.Address.String(), Name: found.Name, RSSI: found.RSSI, Family: tag.Gicisky}
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

// scanRadio listens once and sorts the result into the two families.
//
// Scanning is promiscuous, so both families arrive on the same pass and are
// told apart by a filter rather than by a scan each. Looking for them in turn
// cost two windows and gave each family half the listening.
//
// The hooks stay per-family because that is what a test wants to supply. They
// replace the pass's answer for their own family rather than sitting on a
// second code path, so the radio route is the one route.
func (h *Handler) scanRadio(ctx context.Context) ([]gicisky.FoundDevice, []nrfepd.FoundDevice, error) {
	// A handler holding its own transport must not reach for a radio, whatever
	// it was or was not given. Its hooks are the whole of its world, and one
	// that finds a real tag in the room is a test that passes for the wrong
	// reason — or, on a machine with a tag nearby, fails for one.
	if h.ownTransport {
		if h.scan == nil && h.scanNRFEPD == nil {
			return nil, nil, errors.New("this handler was given its own transport and no Scan or ScanNRFEPD hook, so it cannot find a tag")
		}
		var tags []gicisky.FoundDevice
		var others []nrfepd.FoundDevice
		if h.scan != nil {
			found, err := h.scan(ctx)
			if err != nil {
				return nil, nil, err
			}
			tags = found
		}
		if h.scanNRFEPD != nil {
			found, err := h.scanNRFEPD(ctx)
			if err != nil {
				return nil, nil, err
			}
			others = found
		}
		return tags, others, nil
	}

	// Scanning is promiscuous, so both families arrive on the same pass and are
	// told apart by a filter rather than by a scan each. Looking for them in
	// turn cost two windows and gave each family half the listening.
	if err := h.enableAdapter(); err != nil {
		return nil, nil, err
	}
	gathered, otherGathered := gicisky.NewCollector(), nrfepd.NewCollector()
	err := ble.Scan(ctx, h.adapter, h.scanTimeout, func(result bluetooth.ScanResult) {
		gathered.Observe(result)
		otherGathered.Observe(result)
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	return gathered.Devices(), otherGathered.Devices(), nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.mux.ServeHTTP(writer, request)
}

func (h *Handler) serveRender(writer http.ResponseWriter, request *http.Request) {
	document, ok := h.decodeRequest(writer, request)
	if !ok {
		return
	}
	query := request.URL.Query()
	size, key := query.Get("size"), query.Get("panel")
	if size != "" && key != "" {
		writeError(writer, http.StatusBadRequest, "invalid-request",
			errors.New("give size or panel, not both: they are two ways of saying the same thing"))
		return
	}
	// The three ways a page can be sized, each failing in its own vocabulary:
	// a key or a size the caller mistyped is a bad request, while a scene that
	// will not lay out is a bad scene.
	var result scene.Result
	var renderErr error
	switch {
	case key != "":
		known, err := panel.ByKey(key)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "invalid-request", err)
			return
		}
		result, _, renderErr = panel.Render(document, known)
	case size != "":
		bounds, err := panel.ParseSize(size)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "invalid-request", err)
			return
		}
		result, renderErr = scene.RenderForSize(document, bounds)
	default:
		result, renderErr = scene.Render(document)
	}
	if errors.Is(renderErr, scene.ErrNoSize) {
		// The document is fine; nobody said how big to draw it. That is the
		// request's shape rather than the scene's contents.
		writeError(writer, http.StatusBadRequest, "invalid-request",
			fmt.Errorf("%w: give the document a size, or ask for one with ?size=WxH or ?panel=family:id", renderErr))
		return
	}
	if result.Frame == nil {
		writeError(writer, http.StatusUnprocessableEntity, "invalid-scene", renderErr)
		return
	}
	var encoded bytes.Buffer
	if err := display.WritePNG(&encoded, result.Frame); err != nil {
		writeErrorWithReport(writer, http.StatusInternalServerError, "render-failed", err, result.Report)
		return
	}
	body := map[string]any{
		"width":     result.Frame.Width(),
		"height":    result.Frame.Height(),
		"pngBase64": base64.StdEncoding.EncodeToString(encoded.Bytes()),
		"report":    result.Report,
	}
	// The preview goes out with the refusal. A page named for a panel it
	// cannot go on has still been drawn, and what it looks like is what says
	// which part of it has to change; answering with the error alone would
	// leave a caller to render it a second time without the panel to find out.
	if renderErr != nil {
		body["error"] = renderErr.Error()
		body["code"] = "unprocessable-scene"
		writeJSON(writer, http.StatusUnprocessableEntity, body)
		return
	}
	writeJSON(writer, http.StatusOK, body)
}

func (h *Handler) servePush(writer http.ResponseWriter, request *http.Request) {
	document, ok := h.decodeRequest(writer, request)
	if !ok {
		return
	}
	target := request.URL.Query().Get("device")
	// Checked before anything is claimed or listened for: a misspelled family
	// and an unnamed device are bad requests, not device failures.
	asserted := request.URL.Query().Get("family")
	if err := tag.ValidateFamily(asserted); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid-request", err)
		return
	}
	if target == "" {
		writeError(writer, http.StatusBadRequest, "invalid-request",
			errors.New("no device: name one with ?device=, and GET /v1/scan lists what is in range"))
		return
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

	// Which family the target belongs to is discovered, so the listening comes
	// first and gets its own budget. The write that follows is billed to the
	// family that answered, which is the only point either budget was measured
	// against.
	locateCtx, done := context.WithTimeout(request.Context(), h.scanTimeout+5*time.Second)
	found, err := h.locate(locateCtx, target, asserted)
	locateFailed := locateCtx.Err()
	done()
	if err != nil {
		status := h.release(target, 0, err)
		code, httpStatus := "device-identify-failed", http.StatusBadGateway
		if errors.Is(locateFailed, context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		writeStatus(writer, httpStatus, code, err, status)
		return
	}

	budget := h.pushTimeout
	if found.Family == tag.NRFEPD {
		budget = h.nrfepdPushTimeout
	}
	ctx, cancel := context.WithTimeout(request.Context(), budget)
	defer cancel()

	if found.Family == tag.Gicisky {
		h.writeGicisky(writer, ctx, target, found.Gicisky, document)
		return
	}
	// The page is built once the panel has said what it is, so there is no
	// report to answer with until the write has been attempted.
	written, rendered, pushErr := h.pushPageTo(ctx, target, found.NRFEPD, document)
	status := h.release(target, written, pushErr)
	if pushErr != nil {
		code, httpStatus := "push-failed", http.StatusBadGateway
		// This deadline bounds the complete device operation, including every
		// retry and any refresh wait. It is a timeout even if the link made
		// partial progress, rather than an immediate driver failure.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		if rendered.Frame == nil {
			writeStatus(writer, httpStatus, code, pushErr, status)
			return
		}
		writeStatusWithReport(writer, httpStatus, code, pushErr, status, rendered.Report)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "device": target, "family": tag.NRFEPD, "bytes": written,
		"status": status, "report": rendered.Report,
	})
}

// serveMode hands an EPD-nRF5 tag back to its own clock or calendar, which is
// what `inkwire mode` does and what this service could not.
//
// A tag left in picture mode by a write stays there. Anybody driving this over
// HTTP rather than a command line had no way back, which made the service a
// thing that could only take a capability away.
func (h *Handler) serveMode(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	target := query.Get("device")
	if target == "" {
		writeError(writer, http.StatusBadRequest, "invalid-request",
			errors.New("no device: name one with ?device=, and GET /v1/scan lists what is in range"))
		return
	}
	name := query.Get("mode")
	if name == "" {
		name = "calendar"
	}
	chosen, err := nrfepd.ParseMode(name)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid-request", err)
		return
	}
	day, err := nrfepd.ParseWeekStart(query.Get("week-start"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid-request", err)
		return
	}

	busy, claimed := h.claim(target)
	if !claimed {
		writeStatus(writer, http.StatusConflict, "device-busy",
			fmt.Errorf("device %s is being written", busy.Device), busy)
		return
	}

	// nrfepd is asserted rather than discovered. A Gicisky tag has no mode to
	// set — the whole route is a property of the other firmware — so being told
	// that is more use than being told no EPD-nRF5 tag answered.
	locateCtx, done := context.WithTimeout(request.Context(), h.scanTimeout+5*time.Second)
	found, err := h.locate(locateCtx, target, tag.NRFEPD)
	locateFailed := locateCtx.Err()
	done()
	if err != nil {
		status := h.release(target, 0, err)
		code, httpStatus := "device-identify-failed", http.StatusBadGateway
		if errors.Is(locateFailed, context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		writeStatus(writer, httpStatus, code, err, status)
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), h.nrfepdPushTimeout)
	defer cancel()
	err = h.setModeOn(ctx, target, found.NRFEPD, chosen, day)
	status := h.release(target, 0, err)
	if err != nil {
		code, httpStatus := "push-failed", http.StatusBadGateway
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		writeStatus(writer, httpStatus, code, err, status)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "device": target, "family": tag.NRFEPD, "mode": chosen.String(),
		"status": status,
	})
}

func (h *Handler) setModeOn(ctx context.Context, target string, found nrfepd.FoundDevice,
	mode nrfepd.Mode, weekStart *time.Weekday) error {
	if h.setMode != nil {
		return h.setMode(ctx, target, mode, weekStart)
	}
	if h.ownTransport {
		return errors.New("this handler was given its own transport and no SetMode hook, so it cannot reach an EPD-nRF5 tag")
	}
	if err := h.enableAdapter(); err != nil {
		return err
	}
	driver := nrfepd.NewDriver(h.adapter, found.Address.String(), h.logf)
	driver.Attempts = h.attempts
	// The clock is read per attempt so a retry does not set the tag to the
	// time the first attempt was made.
	return driver.SetModeWithRetry(ctx, found, time.Now, mode, weekStart)
}

// locate works out which tag a request means, from one pass of the radio or
// from whatever the hooks supply in place of one.
func (h *Handler) locate(ctx context.Context, target, asserted string) (tag.Found, error) {
	tags, others, err := h.scanRadio(ctx)
	if err != nil {
		return tag.Found{}, err
	}
	return tag.Choose(tags, others, target, asserted)
}

func (h *Handler) writeGicisky(writer http.ResponseWriter, ctx context.Context,
	target string, found gicisky.FoundDevice, document compose.Document) {
	if !found.Identified {
		err := fmt.Errorf("%s advertised id 0x%04X, which this build does not know", target, found.Advertised.ID)
		status := h.release(target, 0, err)
		writeStatus(writer, http.StatusBadGateway, "device-identify-failed", err, status)
		return
	}
	result, page, err := panel.Render(document, panel.OfGicisky(found.Profile))
	if err != nil {
		status := h.release(target, 0, err)
		// A nil frame means the layout failed and there is no report to send.
		// A frame beside the error means the page drew and would not pack for
		// this panel, and the report is what says why.
		if result.Frame == nil {
			writeStatus(writer, http.StatusUnprocessableEntity, "unprocessable-scene", err, status)
			return
		}
		writeStatusWithReport(writer, http.StatusUnprocessableEntity, "unprocessable-scene", err, status, result.Report)
		return
	}

	pushErr := h.pushGiciskyPayload(ctx, target, found, page.Bytes, found.Profile.Upload())
	status := h.release(target, page.Len(), pushErr)
	if pushErr != nil {
		code, httpStatus := "push-failed", http.StatusBadGateway
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, httpStatus = "device-timeout", http.StatusGatewayTimeout
		}
		writeStatusWithReport(writer, httpStatus, code, pushErr, status, result.Report)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "device": target, "family": tag.Gicisky, "bytes": page.Len(),
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

func (h *Handler) pushGiciskyPayload(ctx context.Context, target string, found gicisky.FoundDevice, payload []byte, options gicisky.UploadOptions) error {
	if h.push != nil {
		return h.push(ctx, target, payload)
	}
	if h.ownTransport {
		return errors.New("this handler was given its own transport and no Push hook, so it cannot reach a Gicisky tag")
	}
	if err := h.enableAdapter(); err != nil {
		return err
	}
	return h.newDriver(target).PushWithRetry(ctx, found, payload, options)
}

// pushPageTo sends a rendered page to an EPD-nRF5 tag, reporting how many
// bytes of panel data it turned out to be.
//
// The count comes back rather than being known in advance because the page is
// not encoded until the panel has said what it is. A page whose size does not
// match is refused here, at the one moment both are known.
// pushPageTo takes both the target as the caller named it and the tag the scan
// found under that name. The hook is given the name, because that is what a
// caller recognises; the driver is given the address, because that is what it
// connects to.
func (h *Handler) pushPageTo(ctx context.Context, target string, found nrfepd.FoundDevice, document compose.Document) (int, scene.Result, error) {
	written := 0
	var rendered scene.Result
	// The panel is only known here, inside the callback, which is why the
	// document rather than a finished frame is carried this far down.
	page := func(model nrfepd.Model) ([]byte, []byte, error) {
		result, packed, err := panel.Render(document, panel.OfNRFEPD(model))
		rendered = result
		written = packed.Len()
		return packed.Black, packed.Colour, err
	}
	if h.pushPage != nil {
		// Called before the count is read: the page builds the planes, so
		// written means nothing until it has run.
		err := h.pushPage(ctx, target, page)
		return written, rendered, err
	}
	if h.ownTransport {
		return 0, rendered, errors.New("this handler was given its own transport and no PushPage hook, so it cannot reach an EPD-nRF5 tag")
	}
	if err := h.enableAdapter(); err != nil {
		return 0, rendered, err
	}
	driver := nrfepd.NewDriver(h.adapter, found.Address.String(), h.logf)
	driver.Attempts = h.attempts
	err := driver.PushWithRetry(ctx, found, page)
	return written, rendered, err
}

// newDriver is separate so the attempt cap can be checked without a radio.
// Every test here replaces Push outright, which left this wiring unexercised.
func (h *Handler) newDriver(target string) *gicisky.Driver {
	driver := gicisky.NewDriver(h.adapter, target, h.logf)
	driver.Attempts = h.attempts
	return driver
}

func (h *Handler) decodeRequest(writer http.ResponseWriter, request *http.Request) (compose.Document, bool) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		writeError(writer, http.StatusUnsupportedMediaType, "unsupported-media-type", fmt.Errorf("invalid Content-Type: %w", err))
		return compose.Document{}, false
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
		return compose.Document{}, false
	}
	if err != nil {
		status, code := http.StatusBadRequest, "invalid-request"
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) || errors.Is(err, errPartTooLarge) {
			status, code = http.StatusRequestEntityTooLarge, "request-too-large"
		}
		writeError(writer, status, code, err)
		return compose.Document{}, false
	}
	document, err := (scene.Decoder{BaseDir: h.baseDir, RestrictFiles: true, Resources: resources}).Decode(bytes.NewReader(sceneBytes))
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "invalid-scene", err)
		return compose.Document{}, false
	}
	return document, true
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

// Every failure carries a stable code so a caller can branch on the kind of
// problem without matching on the human-readable message.
func writeError(writer http.ResponseWriter, status int, code string, err error) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code})
}

func writeErrorWithReport(writer http.ResponseWriter, status int, code string, err error, report compose.Report) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code, "report": report})
}

func writeStatus(writer http.ResponseWriter, status int, code string, err error, device DeviceStatus) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code, "status": device})
}

func writeStatusWithReport(writer http.ResponseWriter, status int, code string, err error, device DeviceStatus, report compose.Report) {
	writeJSON(writer, status, map[string]any{"error": err.Error(), "code": code, "status": device, "report": report})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
