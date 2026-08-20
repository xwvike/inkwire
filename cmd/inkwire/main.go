// Command inkwire renders Scene Schema documents and writes them to BLE
// e-paper tags of two families.
//
// Every subcommand is here rather than in a package of its own because there
// is little to them: they parse flags, call a driver, and turn an error into
// an exit code. What is worth knowing about them is which of the two families
// a flag belongs to, since the families do not share defaults — see targetFor.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/xwvike/inkwire"
	"github.com/xwvike/inkwire/internal/ble"
	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/server"
	"github.com/xwvike/inkwire/internal/tag"
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
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, buildVersion())
		return 0
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "encode":
		return runEncode(args[1:], stdout, stderr)
	case "scan":
		return runScan(ctx, args[1:], logger, stdout, stderr)
	case "push":
		return runPushScene(ctx, args[1:], logger, stdout, stderr)
	case "push-payload":
		return runPushPayload(ctx, args[1:], logger, stdout, stderr)
	case "mode":
		return runMode(ctx, args[1:], logger, stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], logger, stdout, stderr)
	case "schema":
		return runSchema(args[1:], stdout, stderr)
	default:
		// Preserve the original raw-payload invocation while the JSON commands
		// become the normal user-facing path. A leading dash is never a file
		// somebody meant to send, though, and treating one as a payload path is
		// how `inkwire --help` used to answer that it could not open --help.
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(stderr, "unknown option %q\n", args[0])
			printUsage(stderr)
			return 2
		}
		return runPushPayload(ctx, args, logger, stdout, stderr)
	}
}

// parseFlags reads one command's flags, reporting the status to leave with when
// the caller should stop. Asking for help is not a failure: the flag package
// prints what was asked for and then reports ErrHelp, and exiting non-zero for
// it makes `inkwire render -h` look like a command that went wrong.
func parseFlags(flags *flag.FlagSet, args []string, help io.Writer) (int, bool) {
	// The flag package answers -h itself, but it prints wherever parse errors
	// go. Help is what was asked for rather than a complaint about what was
	// typed, so it is answered here and written to help instead.
	for _, arg := range args {
		switch arg {
		case "-h", "-help", "--help":
			flags.SetOutput(help)
			flags.Usage()
			return 0, false
		}
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

// command builds a flag set that knows the shape of its whole invocation.
//
// Go prints "Usage of push:" followed by the flags alone, which names neither
// the program nor the arguments that are not flags. Every command here already
// had the right line written for the case where the argument count was wrong;
// this is that same line, wired to -h as well, so the two cannot disagree.
func command(name, usage string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: %s\n", usage)
		flags.PrintDefaults()
	}
	return flags
}

// version is stamped by the release build. Anything built another way leaves it
// empty and is described by buildVersion from what the toolchain recorded.
var version = ""

// buildVersion says what this binary is, in the terms that actually identify
// it. A release says its tag. A `go install ...@v1.2.3` says the version the
// module proxy served. A build from a source tree says the commit, and says so
// when that tree had uncommitted changes — because a bug report from a dirty
// build otherwise names a commit that never produced this binary.
func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "devel"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		return "devel-" + revision + "-dirty"
	}
	return "devel-" + revision
}

// runSchema prints the Scene Schema reference the binary carries. Whatever is
// driving this program needs to know what a scene document may contain, and
// asking the program it is already running beats finding the right version of a
// file on the web.
func runSchema(args []string, stdout, stderr io.Writer) int {
	flags := command("schema", "inkwire schema [-lang en|zh]", stderr)
	lang := flags.String("lang", "en", "which translation to print: en or zh")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 0 {
		flags.Usage()
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

func runServe(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("serve", "inkwire serve [-listen 127.0.0.1:8080] [-device MAC-or-name] [-assets directory]", stderr)
	address := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	target := flags.String("device", "", "default BLE address or advertised name; unset means the one tag in range")
	assets := flags.String("assets", ".", "directory available to relative image sources")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 0 {
		flags.Usage()
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
	flags := command("scan", "inkwire scan [-timeout 15s]", stderr)
	timeout := flags.Duration("timeout", gicisky.DefaultScanTimeout, "how long to listen for advertisements")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if !enableBluetooth(logger) {
		return 1
	}
	// One radio runs one scan, and one scan is all this needs. Scanning is
	// promiscuous: every advertisement nearby arrives whatever family it
	// belongs to, and the families are told apart by a filter. Looking for
	// them in turn cost two windows and gave each family half the listening.
	tags, others := gicisky.NewCollector(), nrfepd.NewCollector()
	err := ble.Scan(ctx, bluetooth.DefaultAdapter, *timeout, func(result bluetooth.ScanResult) {
		tags.Observe(result)
		others.Observe(result)
	}, nil)
	if err != nil {
		logger.Print(err)
		return 1
	}
	devices, found := tags.Devices(), others.Devices()
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
			tag.NRFEPD, "ask on connect", "", "not advertised")
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
			tag.Gicisky, model, size, palette)
	}
}

func runRender(args []string, stdout, stderr io.Writer) int {
	flags := command("render", "inkwire render [-o preview.png] <scene.json>", stderr)
	output := flags.String("o", "", "PNG output path")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
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
	flags := command("encode", "inkwire encode -profile-id 0x0033 [-o payload.bin] <scene.json>", stderr)
	profileID := flags.String("profile-id", "", "Gicisky profile id from `inkwire scan` (required)")
	output := flags.String("o", "", "payload output path")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	// A panel is not a default. This command connects to nothing, so nothing
	// can tell it which model the scene is for, and a wrong guess is a file
	// that looks fine until a tag draws it. It used to assume 0x0033.
	if *profileID == "" {
		fmt.Fprintln(stderr, "encode needs -profile-id: it builds a payload for one panel and has no tag to ask.")
		fmt.Fprintln(stderr, "`inkwire scan` prints the id under MODEL; the 2.9\" BWR tag is 0x0033.")
		return 2
	}
	source := flags.Arg(0)
	if *output == "" {
		*output = replaceExtension(source, ".bin")
	}
	profile, err := parseGiciskyProfile(*profileID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, err := (scene.Decoder{}).RenderFileForSize(source, image.Pt(profile.Width, profile.Height))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := gicisky.EncodeOriented(result.Frame, result.Orientation, profile)
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

func parseGiciskyProfile(value string) (gicisky.Profile, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 0, 16)
	if err != nil {
		return gicisky.Profile{}, fmt.Errorf("invalid Gicisky profile id %q", value)
	}
	profile, known := gicisky.LookupProfile(uint16(id), 0)
	if !known {
		return gicisky.Profile{}, fmt.Errorf("unknown Gicisky profile id 0x%04X", uint16(id))
	}
	return profile, nil
}

func runPushScene(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("push", "inkwire push -device MAC-or-name [-family gicisky|nrfepd] [-settle 30s] <scene.json>", stderr)
	target := flags.String("device", "", "BLE address or advertised name (required); `inkwire scan` lists them")
	family := flags.String("family", "auto", "tag family: auto, gicisky or nrfepd")
	settle := flags.Duration("settle", nrfepd.DefaultSettle,
		"nrfepd only: how long to stay connected while the panel refreshes; leaving early cancels it")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if *target == "" {
		fmt.Fprintln(stderr, "push needs -device: it will not pick a tag for you.")
		fmt.Fprintln(stderr, "`inkwire scan` lists what is in range, under NAME and ADDRESS.")
		return 2
	}
	document, err := (scene.Decoder{}).DecodeFile(flags.Arg(0))
	if err != nil {
		logger.Print(err)
		return 1
	}
	if !enableBluetooth(logger) {
		return 1
	}
	// Which family a tag belongs to is discovered, not deduced from the shape
	// of the name somebody typed. One pass of the radio answers both "which
	// tag" and "whose tag", and -family, if given, is checked against the
	// answer rather than used in place of it.
	found, err := tag.LocateWithRetry(ctx, bluetooth.DefaultAdapter, gicisky.DefaultScanTimeout,
		*target, *family, ble.Retry{Attempts: gicisky.DefaultAttempts, Delay: gicisky.DefaultRetryDelay, Logf: logger.Printf})
	if err != nil {
		logger.Print(err)
		return 1
	}
	logger.Printf("writing to %s", found)
	if found.Family == tag.NRFEPD {
		return pushNRFEPD(ctx, found.NRFEPD, document, *settle, logger)
	}
	return pushGiciskyScene(ctx, found.Gicisky, document, logger)
}

func pushGiciskyScene(ctx context.Context, found gicisky.FoundDevice, document compose.Document, logger *log.Logger) int {
	if !found.Identified {
		logger.Printf("%s advertised id 0x%04X, which this build does not know", found.Name, found.Advertised.ID)
		return 1
	}
	profile := found.Profile
	result, err := scene.RenderForSize(document, image.Pt(profile.Width, profile.Height))
	if err != nil {
		logger.Print(err)
		return 1
	}
	printReport(logger.Writer(), result)
	payload, err := gicisky.EncodeOriented(result.Frame, result.Orientation, profile)
	if err != nil {
		logger.Print(err)
		return 1
	}
	logger.Printf("pushing %d bytes (%s %dx%d)", len(payload), profile.Model, profile.Width, profile.Height)
	driver := gicisky.NewDriver(bluetooth.DefaultAdapter, found.Address.String(), logger.Printf)
	if err := driver.PushWithRetry(ctx, found, payload, gicisky.UploadOptions{Compression2: profile.Compression2}); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

// runMode hands an EPD-nRF5 tag back to its own clock or calendar.
//
// It is the counterpart to push rather than a feature of its own. The refresh
// that ends every page puts the tag into picture mode, so pushing once stops a
// tag that was keeping time; without this there is no way back that does not
// involve the vendor's web tool, and a program that can only take a capability
// away is a bad guest on somebody's hardware.
func runMode(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("mode", "inkwire mode -device MAC-or-name [-mode picture|calendar|clock] [-week-start sunday|monday] [-settle 30s]", stderr)
	target := flags.String("device", "", "BLE address or advertised name (required); `inkwire scan` lists them")
	mode := flags.String("mode", "calendar", "what the tag draws for itself: picture, calendar or clock")
	weekStart := flags.String("week-start", "", "first column of a calendar week: sunday or monday; unset leaves the tag's own setting alone")
	settle := flags.Duration("settle", nrfepd.DefaultSettle, "how long to stay connected while the panel redraws")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if *target == "" {
		fmt.Fprintln(stderr, "mode needs -device: it will not pick a tag for you.")
		fmt.Fprintln(stderr, "`inkwire scan` lists what is in range, under NAME and ADDRESS.")
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
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

// pushNRFEPD sends a page to a tag that does not say what it is until asked.
//
// The page is handed over inside a callback because the panel's size is only
// known once the connection is up, and a page of the wrong size is the failure
// this family invites: nothing rejects it, the bytes simply land in the panel's
// RAM meaning something other than what they meant here.
func pushNRFEPD(ctx context.Context, found nrfepd.FoundDevice, document compose.Document, settle time.Duration, logger *log.Logger) int {
	driver := nrfepd.NewDriver(bluetooth.DefaultAdapter, found.Address.String(), logger.Printf)
	driver.Timings.Settle = settle
	// The page is built inside the callback because that is the only place the
	// panel is known. This used to render before connecting and then refuse
	// anything that did not match what it found, which made the callback a
	// size assertion and left the caller to guess the size it was asserting.
	var rendered scene.Result
	err := driver.PushWithRetry(ctx, func(model nrfepd.Model) ([]byte, []byte, error) {
		result, err := scene.RenderForSize(document, image.Pt(model.Width, model.Height))
		if err != nil {
			return nil, nil, err
		}
		rendered = result
		return display.EncodeNRFEPD(result.Frame, model.Palette != nrfepd.PaletteBW)
	})
	if rendered.Frame != nil {
		printReport(logger.Writer(), rendered)
	}
	if err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

func runPushPayload(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("push-payload", "inkwire push-payload [-profile-id 0x0033] <MAC-or-name> <payload.bin>", stderr)
	profileID := flags.String("profile-id", "",
		"Gicisky profile id, only for a tag whose advertised model this build does not know")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 2 {
		flags.Usage()
		return 2
	}
	target, path := flags.Arg(0), flags.Arg(1)
	var stated gicisky.Profile
	if *profileID != "" {
		var err error
		if stated, err = parseGiciskyProfile(*profileID); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
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
	driver := gicisky.NewDriver(bluetooth.DefaultAdapter, target, logger.Printf)

	// The model decides how the payload is announced, and this command is
	// about to connect to the tag that knows. Being told instead is for the
	// tag this build cannot identify, which is the only case where asking
	// cannot answer. It used to assume 0x0033 and say nothing.
	if *profileID == "" {
		found, err := driver.FindIdentifiedWithRetry(ctx)
		if err != nil {
			logger.Print(err)
			// Only a tag that answered without naming its panel can be got
			// past by stating one. A tag that was never found stays not found,
			// and sending somebody to a flag that cannot help them is worse
			// than saying nothing.
			if errors.Is(err, gicisky.ErrNotIdentified) {
				logger.Print("pass -profile-id to write to it anyway")
			}
			return 1
		}
		profile := found.Profile
		logger.Printf("pushing %d bytes to %s (%s %dx%d)", len(payload), target, profile.Model, profile.Width, profile.Height)
		if err := driver.PushWithRetry(ctx, found, payload, gicisky.UploadOptions{Compression2: profile.Compression2}); err != nil {
			logger.Print(err)
			return 1
		}
		return 0
	}
	logger.Printf("pushing %d bytes to %s (%s %dx%d, stated)", len(payload), target, stated.Model, stated.Width, stated.Height)
	if err := driver.FindAndPushWithRetry(ctx, payload, gicisky.UploadOptions{Compression2: stated.Compression2}); err != nil {
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
	for _, expansion := range result.Report.GridExpansions {
		fmt.Fprintf(writer, "grid %s: implicit-columns=%d implicit-rows=%d\n",
			expansion.Path, expansion.ImplicitColumns, expansion.ImplicitRows)
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
		// budget for either family plus room for the response itself.
		WriteTimeout: max(server.DefaultPushTimeout, server.DefaultNRFEPDPushTimeout) + 15*time.Second,
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
	fmt.Fprintln(writer, "       inkwire encode -profile-id 0x0033 [-o payload.bin] <scene.json>")
	fmt.Fprintln(writer, "       inkwire push -device MAC-or-name [-family gicisky|nrfepd] <scene.json>")
	fmt.Fprintln(writer, "       inkwire scan [-timeout 15s]")
	fmt.Fprintln(writer, "       inkwire mode -device MAC-or-name [-mode picture|calendar|clock] [-week-start sunday|monday]")
	fmt.Fprintln(writer, "       inkwire serve [-listen address] [-device MAC-or-name] [-assets directory]")
	fmt.Fprintln(writer, "       inkwire push-payload <MAC-or-name> <payload.bin>")
	fmt.Fprintln(writer, "       inkwire schema [-lang en|zh]")
	fmt.Fprintln(writer, "       inkwire version")
}
