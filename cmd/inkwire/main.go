package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/xwvike/inkwire"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/server"
	"tinygo.org/x/bluetooth"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	logger := log.New(stdout, "", 0)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "encode":
		return runEncode(args[1:], stdout, stderr)
	case "scan":
		return runScan(ctx, args[1:], logger, stdout, stderr)
	case "push":
		return runPushScene(ctx, args[1:], logger, stderr)
	case "push-payload":
		return runPushPayload(ctx, args[1:], logger, stderr)
	case "mode":
		return runMode(ctx, args[1:], logger, stderr)
	case "serve":
		return runServe(ctx, args[1:], logger, stderr)
	case "schema":
		return runSchema(args[1:], stdout, stderr)
	default:
		// Preserve the original raw-payload invocation while the JSON commands
		// become the normal user-facing path.
		return runPushPayload(ctx, args, logger, stderr)
	}
}

// runSchema prints the Scene Schema reference the binary carries. Whatever is
// driving this program needs to know what a scene document may contain, and
// asking the program it is already running beats finding the right version of a
// file on the web.
func runSchema(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("schema", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lang := flags.String("lang", "en", "which translation to print: en or zh")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: inkwire schema [-lang en|zh]")
		return 2
	}
	switch strings.ToLower(*lang) {
	case "en":
		fmt.Fprint(stdout, inkwire.Schema)
	case "zh":
		fmt.Fprint(stdout, inkwire.SchemaChinese)
	default:
		fmt.Fprintf(stderr, "unknown language %q: use en or zh\n", *lang)
		return 2
	}
	return 0
}

func runServe(ctx context.Context, args []string, logger *log.Logger, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	address := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	target := flags.String("device", gicisky.TargetAddress, "default BLE address or advertised name")
	assets := flags.String("assets", ".", "directory available to relative image sources")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: inkwire serve [-listen 127.0.0.1:8080] [-device MAC-or-name] [-assets directory]")
		return 2
	}
	if err := requireLoopback(*address); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	handler := server.New(server.Config{Adapter: bluetooth.DefaultAdapter, Target: *target, BaseDir: *assets, Logf: logger.Printf})
	httpServer := newHTTPServer(*address, handler)
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()
	logger.Printf("listening on http://%s", *address)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Printf("serve: %v", err)
		return 1
	}
	return 0
}

func runScan(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	timeout := flags.Duration("timeout", gicisky.DefaultScanTimeout, "how long to listen for advertisements")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: inkwire scan [-timeout 15s]")
		return 2
	}
	if !enableBluetooth(logger) {
		return 1
	}
	// One radio can only run one scan, so the families are looked for in turn
	// and the timeout is what each of them gets.
	driver := gicisky.NewDriver(bluetooth.DefaultAdapter, "", logger.Printf)
	driver.ScanTimeout = *timeout
	devices, err := driver.ScanAll(ctx)
	if err != nil {
		logger.Print(err)
		return 1
	}
	others := nrfepd.NewDriver(bluetooth.DefaultAdapter, "", logger.Printf)
	others.ScanTimeout = *timeout
	found, err := others.ScanAll(ctx)
	if err != nil {
		logger.Print(err)
		return 1
	}
	if len(devices) == 0 && len(found) == 0 {
		fmt.Fprintln(stdout, "no tags found")
		return 1
	}
	printDevices(stdout, devices, found)
	return 0
}

func printDevices(writer io.Writer, devices []gicisky.FoundDevice, others []nrfepd.FoundDevice) {
	fmt.Fprintf(writer, "%-38s %-13s %5s %5s  %-8s %-15s %-9s %s\n",
		"ADDRESS", "NAME", "RSSI", "BATT", "FAMILY", "MODEL", "SIZE", "PALETTE")
	for _, device := range others {
		// Nothing beyond the name is knowable without connecting: this family
		// keeps its panel in the firmware's own flash and an advertisement
		// never mentions it. Saying that is the useful thing to print, because
		// a blank column otherwise reads as a tag this build failed to
		// recognise.
		fmt.Fprintf(writer, "%-38s %-13s %5d %5s  %-8s %-15s %-9s %s\n",
			device.Address.String(), device.Name, device.RSSI, "-",
			familyNRFEPD, "ask on connect", "", "not advertised")
	}
	for _, device := range devices {
		model, size, palette := "unknown", "", ""
		switch {
		case device.Identified:
			model = device.Profile.Model
			size = fmt.Sprintf("%dx%d", device.Profile.Width, device.Profile.Height)
			palette = device.Profile.Palette.String()
			if !device.Profile.Verified {
				palette += " (unverified)"
			}
		case device.HasAdvertised:
			// The tag said what it is and this build does not recognise it.
			// Saying so is more useful than omitting the tag entirely.
			model = fmt.Sprintf("id 0x%04X", device.Advertised.ID)
			palette = "unrecognised model"
		default:
			palette = "no advertisement data"
		}
		battery := "-"
		if device.HasAdvertised {
			battery = fmt.Sprintf("%.1fV", device.Advertised.Voltage())
		}
		fmt.Fprintf(writer, "%-38s %-13s %5d %5s  %-8s %-15s %-9s %s\n",
			device.Address.String(), device.Name, device.RSSI, battery,
			familyGicisky, model, size, palette)
	}
}

func runRender(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("o", "", "PNG output path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: inkwire render [-o preview.png] <scene.json>")
		return 2
	}
	source := flags.Arg(0)
	if *output == "" {
		*output = replaceExtension(source, ".png")
	}
	result, err := (scene.Decoder{}).RenderFile(source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	file, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(stderr, "create preview: %v\n", err)
		return 1
	}
	writeErr := display.WritePNG(file, result.Frame)
	closeErr := file.Close()
	if writeErr != nil {
		fmt.Fprintln(stderr, writeErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "close preview: %v\n", closeErr)
		return 1
	}
	printReport(stdout, result)
	fmt.Fprintf(stdout, "wrote %s (%dx%d)\n", *output, result.Frame.Width(), result.Frame.Height())
	return 0
}

func runEncode(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("encode", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("o", "", "payload output path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: inkwire encode [-o payload.bin] <scene.json>")
		return 2
	}
	source := flags.Arg(0)
	if *output == "" {
		*output = replaceExtension(source, ".bin")
	}
	result, err := (scene.Decoder{}).RenderFile(source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := result.Payload()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(*output, payload, 0o644); err != nil {
		fmt.Fprintf(stderr, "write payload: %v\n", err)
		return 1
	}
	printReport(stdout, result)
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *output, len(payload))
	return 0
}

func runPushScene(ctx context.Context, args []string, logger *log.Logger, stderr io.Writer) int {
	flags := flag.NewFlagSet("push", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("device", gicisky.TargetAddress, "BLE address or advertised name")
	family := flags.String("family", "auto", "tag family: auto, gicisky or nrfepd")
	settle := flags.Duration("settle", nrfepd.DefaultSettle,
		"nrfepd only: how long to stay connected while the panel refreshes; leaving early cancels it")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: inkwire push [-device MAC-or-name] [-family auto|gicisky|nrfepd] [-settle 30s] <scene.json>")
		return 2
	}
	chosen, err := resolveFamily(*family, *target)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, err := (scene.Decoder{}).RenderFile(flags.Arg(0))
	if err != nil {
		logger.Print(err)
		return 1
	}
	printReport(logger.Writer(), result)
	if !enableBluetooth(logger) {
		return 1
	}
	if chosen == familyNRFEPD {
		return pushNRFEPD(ctx, *target, result.Frame, *settle, logger)
	}
	// The Gicisky payload is built here rather than inside the driver because
	// that family's page size is fixed and known before anything is connected
	// to, which is exactly what the other one cannot assume.
	payload, err := result.Payload()
	if err != nil {
		logger.Print(err)
		return 1
	}
	return push(ctx, bluetooth.DefaultAdapter, *target, payload, logger)
}

const (
	familyGicisky = "gicisky"
	familyNRFEPD  = "nrfepd"
)

// runMode hands an EPD-nRF5 tag back to its own clock or calendar.
//
// It is the counterpart to push rather than a feature of its own. The refresh
// that ends every page puts the tag into picture mode, so pushing once stops a
// tag that was keeping time; without this there is no way back that does not
// involve the vendor's web tool, and a program that can only take a capability
// away is a bad guest on somebody's hardware.
func runMode(ctx context.Context, args []string, logger *log.Logger, stderr io.Writer) int {
	flags := flag.NewFlagSet("mode", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("device", "", "BLE address or advertised name")
	mode := flags.String("mode", "calendar", "what the tag draws for itself: picture, calendar or clock")
	weekStart := flags.String("week-start", "", "first column of a calendar week: sunday or monday; unset leaves the tag's own setting alone")
	settle := flags.Duration("settle", nrfepd.DefaultSettle, "how long to stay connected while the panel redraws")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: inkwire mode [-device MAC-or-name] [-mode picture|calendar|clock] [-week-start sunday|monday] [-settle 30s]")
		return 2
	}
	chosen, err := nrfepd.ParseMode(*mode)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	day, err := parseWeekStart(*weekStart)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !enableBluetooth(logger) {
		return 1
	}
	driver := nrfepd.NewDriver(bluetooth.DefaultAdapter, *target, logger.Printf)
	driver.Timings.Settle = *settle
	// The clock is read per attempt rather than in the flag parsing, so that
	// what the tag is told is the time the exchange actually happened.
	if err := driver.SetModeWithRetry(ctx, time.Now, chosen, day); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

// parseWeekStart reads the day a calendar week starts on. An empty value is
// not a default: it means say nothing, and leave whatever the tag already has.
func parseWeekStart(name string) (*time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return nil, nil
	case "sunday":
		day := time.Sunday
		return &day, nil
	case "monday":
		day := time.Monday
		return &day, nil
	}
	return nil, fmt.Errorf("unknown week start %q: use sunday or monday", name)
}

// resolveFamily decides which driver a target wants.
//
// The two families share nothing: different service, different commands,
// different packing. Sending one family's bytes to the other does not fail
// politely, so this would rather refuse than pick wrong.
//
// A name is enough to tell them apart, because this is the one thing an
// EPD-nRF5 tag does say about itself in an advertisement. An address is not,
// so it keeps the family that has always been the default and leaves -family
// for saying otherwise.
func resolveFamily(requested, target string) (string, error) {
	switch requested {
	case familyGicisky, familyNRFEPD:
		return requested, nil
	case "auto":
		if strings.HasPrefix(strings.ToUpper(target), nrfepd.NamePrefix) {
			return familyNRFEPD, nil
		}
		return familyGicisky, nil
	}
	return "", fmt.Errorf("unknown family %q: use auto, %s or %s", requested, familyGicisky, familyNRFEPD)
}

// pushNRFEPD sends a page to a tag that does not say what it is until asked.
//
// The page is handed over inside a callback because the panel's size is only
// known once the connection is up, and a page of the wrong size is the failure
// this family invites: nothing rejects it, the bytes simply land in the panel's
// RAM meaning something other than what they meant here.
func pushNRFEPD(ctx context.Context, target string, frame *display.Frame, settle time.Duration, logger *log.Logger) int {
	driver := nrfepd.NewDriver(bluetooth.DefaultAdapter, target, logger.Printf)
	driver.Timings.Settle = settle
	err := driver.PushWithRetry(ctx, func(model nrfepd.Model) ([]byte, []byte, error) {
		if frame.Width() != model.Width || frame.Height() != model.Height {
			return nil, nil, fmt.Errorf(
				"the page is %dx%d and the panel is %s; render it at the panel's size",
				frame.Width(), frame.Height(), model)
		}
		return display.EncodeNRFEPD(frame, model.Palette != nrfepd.PaletteBW)
	})
	if err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

func runPushPayload(ctx context.Context, args []string, logger *log.Logger, stderr io.Writer) int {
	var target, path string
	switch len(args) {
	case 1:
		target, path = gicisky.TargetAddress, args[0]
	case 2:
		target, path = args[0], args[1]
	default:
		fmt.Fprintln(stderr, "usage: inkwire push-payload [MAC-or-name] <payload.bin>")
		return 2
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		logger.Printf("read payload: %v", err)
		return 1
	}
	if err := gicisky.ValidatePayload(payload); err != nil {
		logger.Print(err)
		return 1
	}
	if !enableBluetooth(logger) {
		return 1
	}
	return push(ctx, bluetooth.DefaultAdapter, target, payload, logger)
}

func push(ctx context.Context, adapter *bluetooth.Adapter, target string, payload []byte, logger *log.Logger) int {
	logger.Printf("pushing %d bytes to %s", len(payload), target)
	driver := gicisky.NewDriver(adapter, target, logger.Printf)
	if err := driver.PushWithRetry(ctx, payload); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

func enableBluetooth(logger *log.Logger) bool {
	if err := bluetooth.DefaultAdapter.Enable(); err != nil {
		logger.Printf("enable Bluetooth: %v", err)
		return false
	}
	return true
}

func printReport(writer io.Writer, result scene.Result) {
	if len(result.Report.MissingRunes) != 0 {
		fmt.Fprintf(writer, "missing runes: %q\n", string(result.Report.MissingRunes))
	}
	for _, warning := range result.Report.Warnings {
		fmt.Fprintf(writer, "warning %s [%s]: %s\n", warning.Path, warning.Code, warning.Message)
	}
	for _, decision := range result.Report.Images {
		fmt.Fprintf(writer, "image %s: dither=%d fit=%d sampling=%d threshold=%d red-disabled=%t\n",
			decision.Path, decision.Options.Dither, decision.Options.Fit, decision.Options.Sampling,
			decision.Options.Threshold, decision.Options.DisableRed)
	}
}

// newHTTPServer is separate from runServe so the timeouts are reachable from a
// test. Left inline they were invisible: deleting one changed nothing that
// anything checked.
func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		// A write must outlast a full push, so this is the server's push
		// budget plus room for the response itself.
		WriteTimeout: server.DefaultPushTimeout + 15*time.Second,
		IdleTimeout:  time.Minute,
	}
}

// requireLoopback keeps the server unreachable from the network. It has no
// authentication and every request can drive the tag, so the address is
// constrained here rather than left to whoever writes the command line.
func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("listen address %q is not loopback; inkwire serve has no authentication and writes to hardware, so it only binds localhost", address)
}

func replaceExtension(path, extension string) string {
	current := filepath.Ext(path)
	return strings.TrimSuffix(path, current) + extension
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: inkwire render [-o preview.png] <scene.json>")
	fmt.Fprintln(writer, "       inkwire encode [-o payload.bin] <scene.json>")
	fmt.Fprintln(writer, "       inkwire push [-device MAC-or-name] [-family auto|gicisky|nrfepd] <scene.json>")
	fmt.Fprintln(writer, "       inkwire scan [-timeout 15s]")
	fmt.Fprintln(writer, "       inkwire mode [-device MAC-or-name] [-mode picture|calendar|clock] [-week-start sunday|monday]")
	fmt.Fprintln(writer, "       inkwire serve [-listen address] [-device MAC-or-name] [-assets directory]")
	fmt.Fprintln(writer, "       inkwire push-payload [MAC-or-name] <payload.bin>")
	fmt.Fprintln(writer, "       inkwire schema [-lang en|zh]")
}
