// Command inkwire renders Markup pages and writes them to BLE e-paper tags of
// two families.
//
// Every subcommand is here rather than in a package of its own because there
// is little to them: they parse flags, call a driver, and turn an error into
// an exit code. What is worth knowing about them is which of the two families
// a flag belongs to, since the families do not share defaults — see targetFor.
package main

import (
	"context"
	"encoding/json"
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
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"github.com/xwvike/inkwire/internal/panel"
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
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "measure":
		return runMeasure(args[1:], stdout, stderr)
	case "scan":
		return runScan(ctx, args[1:], logger, stdout, stderr)
	case "push":
		return runPushScene(ctx, args[1:], logger, stdout, stderr)
	case "mode":
		return runMode(ctx, args[1:], logger, stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], logger, stdout, stderr)
	default:
		// Every invocation names a command. There used to be a bare form that
		// took a file path and wrote it to a tag, which is how `inkwire
		// --help` once answered that it could not open a file called --help.
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(stderr, "unknown option %q\n", args[0])
		} else {
			fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		}
		printUsage(stderr)
		return 2
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
// usageLines is the one place a command's shape is written down.
//
// It used to be written twice: once for the line -h answers with, and once in
// the summary `inkwire` prints when it is given nothing. The two drifted, as
// two copies of a sentence do — the summary never learned that push and mode
// take -settle, and nobody would find that by reading either one, only by
// reading both and comparing. The order here is the order the summary lists.
var usageLines = []struct{ name, takes string }{
	{"render", "[-o preview.png] [-size WxH | -panel family:id] [-asset SRC=FILE] <page.html>"},
	{"compile", "[-o scene.json] [-asset SRC=FILE] <page.html>"},
	{"push", "-device MAC-or-name [-family gicisky|nrfepd] [-settle 30s] [-asset SRC=FILE] <page.html>"},
	{"measure", "[-size WxH | -panel family:id] [-json] [-asset SRC=FILE] <page.html>"},
	{"scan", "[-timeout 15s]"},
	{"mode", "-device MAC-or-name [-mode picture|calendar|clock] [-week-start sunday|monday] [-settle 30s]"},
	{"serve", "[-listen address] [-assets directory]"},
	{"version", ""},
}

// usageFor is the whole line, command included, as both places print it.
func usageFor(name string) string {
	for _, line := range usageLines {
		if line.name == name {
			if line.takes == "" {
				return "inkwire " + name
			}
			return "inkwire " + name + " " + line.takes
		}
	}
	// Unreachable in a build whose commands are all listed, which every
	// command's -h test walks. Saying so beats printing a usage line with the
	// arguments missing.
	panic("inkwire: no usage line for " + name + "; add it to usageLines")
}

func command(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: %s\n", usageFor(name))
		flags.PrintDefaults()
	}
	return flags
}

type assetMapping struct {
	source string
	path   string
}

type assetFlags struct {
	mappings []assetMapping
}

func (f *assetFlags) String() string {
	values := make([]string, 0, len(f.mappings))
	for _, mapping := range f.mappings {
		values = append(values, mapping.source+"="+mapping.path)
	}
	return strings.Join(values, ",")
}

func (f *assetFlags) Set(value string) error {
	source, path, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(source) == "" || strings.TrimSpace(path) == "" {
		return fmt.Errorf("-asset expects SRC=FILE")
	}
	for _, mapping := range f.mappings {
		if mapping.source == source {
			return fmt.Errorf("-asset %q was specified more than once", source)
		}
	}
	f.mappings = append(f.mappings, assetMapping{source: source, path: path})
	return nil
}

func (f *assetFlags) read() (map[string][]byte, error) {
	resources := make(map[string][]byte, len(f.mappings))
	for _, mapping := range f.mappings {
		content, err := os.ReadFile(mapping.path)
		if err != nil {
			return nil, fmt.Errorf("read asset %q for %q: %w", mapping.path, mapping.source, err)
		}
		resources[mapping.source] = content
	}
	return resources, nil
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

func runServe(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("serve", stderr)
	address := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	assets := flags.String("assets", ".", "directory available to relative JSON-scene resources")
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
	handler := server.New(server.Config{Adapter: bluetooth.DefaultAdapter, BaseDir: *assets, Logf: logger.Printf})
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
	flags := command("scan", stderr)
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
	flags := command("render", stderr)
	output := flags.String("o", "", "PNG output path")
	size := flags.String("size", "", "lay the scene out at this size instead of the one it declares, as `WxH`")
	target := flags.String("panel", "", "lay the scene out for a named `family:id` panel and check its inks, such as gicisky:0x0033 or nrfepd:UC8176_420_BWR")
	assets := new(assetFlags)
	flags.Var(assets, "asset", "read a local resource as SRC=FILE; repeat for multiple resources")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	// Both say what size to lay out for, and only one of them can be right.
	// Taking the last one written would make which flag wins a thing to
	// remember rather than a thing to read.
	if *size != "" && *target != "" {
		fmt.Fprintln(stderr, "render takes -size or -panel, not both: they are two ways of saying the same thing.")
		return 2
	}
	source := flags.Arg(0)
	if *output == "" {
		*output = replaceExtension(source, ".png")
	}
	resources, err := assets.read()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, renderErr, usage := renderFile(source, *size, *target, resources)
	if usage != nil {
		// A page that lost a declaration on the way in is often why there is
		// nothing to lay out. Printing the usage error on its own would send
		// the author looking at the flags instead of at the file.
		printWarnings(stderr, result.Report.Warnings)
		fmt.Fprintln(stderr, usage)
		return 2
	}
	// A page refused by -panel still drew, and the preview of it is the thing
	// that shows what has to change. Writing it and then failing says more
	// than failing with nothing to look at: the ink the panel cannot show is
	// somewhere on that picture.
	if result.Frame == nil {
		fmt.Fprintln(stderr, renderErr)
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
	if renderErr != nil {
		fmt.Fprintln(stderr, renderErr)
		return 1
	}
	return 0
}

func runPushScene(ctx context.Context, args []string, logger *log.Logger, stdout, stderr io.Writer) int {
	flags := command("push", stderr)
	target := flags.String("device", "", "BLE address or advertised name, required; inkwire scan lists them")
	family := flags.String("family", "auto", "tag family: auto, gicisky or nrfepd")
	settle := flags.Duration("settle", nrfepd.DefaultSettle,
		"nrfepd only: how long to stay connected while the panel refreshes; leaving early cancels it, and 0 leaves immediately")
	assets := new(assetFlags)
	flags.Var(assets, "asset", "read a local resource as SRC=FILE; repeat for multiple resources")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if !settleIsUsable(*settle, stderr) {
		return 2
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
	// A misspelled family is a bad argument, not a tag that could not be
	// reached, so it is refused here rather than fifteen seconds later with
	// the exit code of a device failure.
	if err := tag.ValidateFamily(*family); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	resources, err := assets.read()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	document, warnings, err := loadDocument(flags.Arg(0), resources)
	if err != nil {
		logger.Print(err)
		return 1
	}
	for _, warning := range warnings {
		logger.Printf("warning %s [%s]: %s", warning.Path, warning.Code, warning.Message)
	}
	if !enableBluetooth(logger) {
		return 1
	}
	// Which family a tag belongs to is discovered, not deduced from the shape
	// of the name somebody typed. One pass of the radio answers both "which
	// tag" and "whose tag", and -family, if given, is checked against the
	// answer rather than used in place of it.
	found, err := locate(ctx, *target, *family, logger)
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
	p := panel.OfGicisky(found.Profile)
	result, page, err := panel.Render(document, p)
	// Reported whenever there was a page to report on, which includes the
	// encode failing: a page that draws and will not pack is exactly the
	// failure the report explains.
	if result.Frame != nil {
		printReport(logger.Writer(), result)
	}
	if err != nil {
		logger.Print(err)
		return 1
	}
	logger.Printf("pushing %d bytes (%s)", page.Len(), p)
	driver := gicisky.NewDriver(bluetooth.DefaultAdapter, found.Address.String(), logger.Printf)
	if err := driver.PushWithRetry(ctx, found, page.Bytes, found.Profile.Upload()); err != nil {
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
	flags := command("mode", stderr)
	target := flags.String("device", "", "BLE address or advertised name, required; inkwire scan lists them")
	mode := flags.String("mode", "calendar", "what the tag draws for itself: picture, calendar or clock")
	weekStart := flags.String("week-start", "", "first column of a calendar week: sunday or monday; unset leaves the tag's own setting alone")
	settle := flags.Duration("settle", nrfepd.DefaultSettle, "how long to stay connected while the panel redraws; 0 leaves immediately")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if !settleIsUsable(*settle, stderr) {
		return 2
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
	day, err := nrfepd.ParseWeekStart(*weekStart)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !enableBluetooth(logger) {
		return 1
	}
	// Asserting the family is how a Gicisky tag gets told what is wrong with
	// asking it for a clock. It has no mode to set — the whole command is a
	// property of the other firmware — and "no NRF_EPD tag found" is not what
	// somebody looking at the tag on their desk needs to hear.
	found, err := locate(ctx, *target, tag.NRFEPD, logger)
	if err != nil {
		logger.Print(err)
		return 1
	}
	driver := nrfepd.NewDriver(bluetooth.DefaultAdapter, found.Address(), logger.Printf)
	driver.Timings.Settle = *settle
	// The clock is read per attempt rather than in the flag parsing, so that
	// what the tag is told is the time the exchange actually happened.
	if err := driver.SetModeWithRetry(ctx, found.NRFEPD, time.Now, chosen, day); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

// settleIsUsable refuses a negative settle rather than reading it as zero.
//
// Waiting for a negative length of time is not something somebody means, so it
// is a typo — most likely a duration that lost its unit and picked up a sign.
// Reading it as "do not wait" would carry the typo all the way to a page that
// sends perfectly and never appears, which is the failure this flag exists to
// prevent. Zero is spelled zero.
func settleIsUsable(settle time.Duration, stderr io.Writer) bool {
	if settle < 0 {
		fmt.Fprintf(stderr, "-settle cannot be negative: %s.\n", settle)
		fmt.Fprintln(stderr, "Leave it out for the default, or write -settle 0 to disconnect as soon as the write lands.")
		return false
	}
	return true
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
// locate is how every command that writes to a tag finds it: one pass of the
// radio, the family taken from what answered, and a stated family checked
// against that. Commands used to find their own, each in its own way, and the
// two that did could only report what their own family had failed to see.
func locate(ctx context.Context, target, family string, logger *log.Logger) (tag.Found, error) {
	return tag.LocateWithRetry(ctx, bluetooth.DefaultAdapter, gicisky.DefaultScanTimeout, target, family,
		ble.Retry{Attempts: gicisky.DefaultAttempts, Delay: gicisky.DefaultRetryDelay, Logf: logger.Printf})
}

func pushNRFEPD(ctx context.Context, found nrfepd.FoundDevice, document compose.Document, settle time.Duration, logger *log.Logger) int {
	driver := nrfepd.NewDriver(bluetooth.DefaultAdapter, found.Address.String(), logger.Printf)
	driver.Timings.Settle = settle
	// The page is built inside the callback because that is the only place the
	// panel is known. This used to render before connecting and then refuse
	// anything that did not match what it found, which made the callback a
	// size assertion and left the caller to guess the size it was asserting.
	var rendered scene.Result
	err := driver.PushWithRetry(ctx, found, func(model nrfepd.Model) ([]byte, []byte, error) {
		result, page, err := panel.Render(document, panel.OfNRFEPD(model))
		rendered = result
		return page.Black, page.Colour, err
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

func enableBluetooth(logger *log.Logger) bool {
	if err := bluetooth.DefaultAdapter.Enable(); err != nil {
		logger.Printf("enable Bluetooth: %v", err)
		return false
	}
	return true
}

// runMeasure prints where every node ended up, and what the ones with an
// opinion would rather have had.
//
// It exists because the only way to find out how wide a piece of text was, or
// whether a box was a pixel short of the letters in it, was to render, read a
// warning and bisect. A 296x128 panel does not forgive that: the difference
// between right and wrong is a pixel or two, and looking at the picture will
// not tell you which node owns it.
func runMeasure(args []string, stdout, stderr io.Writer) int {
	flags := command("measure", stderr)
	size := flags.String("size", "", "lay the scene out at this size instead of the one it declares, as `WxH`")
	target := flags.String("panel", "", "lay the scene out for a named `family:id` panel, such as gicisky:0x0033")
	asJSON := flags.Bool("json", false, "write the placements as JSON instead of a tree")
	assets := new(assetFlags)
	flags.Var(assets, "asset", "read a local resource as SRC=FILE; repeat for multiple resources")
	if code, ok := parseFlags(flags, args, stdout); !ok {
		return code
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if *size != "" && *target != "" {
		fmt.Fprintln(stderr, "measure takes -size or -panel, not both: they are two ways of saying the same thing.")
		return 2
	}

	resources, err := assets.read()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, renderErr, usage := measureFile(flags.Arg(0), *size, *target, resources)
	if usage != nil {
		printWarnings(stderr, result.Report.Warnings)
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if renderErr != nil {
		fmt.Fprintln(stderr, renderErr)
		return 1
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(placementsJSON(result.Report.Placements)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	printPlacements(stdout, result.Report.Placements)
	for _, warning := range result.Report.Warnings {
		fmt.Fprintf(stdout, "\nwarning %s [%s]: %s\n", warning.Path, warning.Code, warning.Message)
	}
	return 0
}

// printPlacements indents by path depth, so the tree in the output is the tree
// in the document.
func printPlacements(writer io.Writer, placements []compose.Placement) {
	width := 0
	for _, p := range placements {
		if n := depthOf(p.Path)*2 + len(p.Type); n > width {
			width = n
		}
	}
	for _, p := range placements {
		indent := strings.Repeat("  ", depthOf(p.Path))
		line := fmt.Sprintf("%s%-*s %4d,%-4d %3dx%-3d", indent, width-depthOf(p.Path)*2, p.Type,
			p.Bounds.Min.X, p.Bounds.Min.Y, p.Bounds.Dx(), p.Bounds.Dy())
		if p.Wanted != (image.Point{}) && p.Wanted != p.Bounds.Size() {
			line += fmt.Sprintf("  wants %dx%d", p.Wanted.X, p.Wanted.Y)
		}
		fmt.Fprintln(writer, line)
	}
}

// depthOf counts how far down the tree a path sits. Paths are written as
// root.children[0].child, so the separators are the depth.
func depthOf(path string) int {
	return strings.Count(path, ".") + strings.Count(path, "[")
}

type placementJSON struct {
	Path   string    `json:"path"`
	Type   string    `json:"type"`
	Bounds boxJSON   `json:"bounds"`
	Wanted *sizeJSON `json:"wants,omitempty"`
}

// boxJSON and sizeJSON spell a rectangle the way every other rectangle in this
// program's JSON is spelled: an origin and a size, in lower case.
type boxJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type sizeJSON struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func placementsJSON(placements []compose.Placement) []placementJSON {
	out := make([]placementJSON, 0, len(placements))
	for _, p := range placements {
		row := placementJSON{Path: p.Path, Type: p.Type,
			Bounds: boxJSON{p.Bounds.Min.X, p.Bounds.Min.Y, p.Bounds.Dx(), p.Bounds.Dy()}}
		if p.Wanted != (image.Point{}) {
			row.Wanted = &sizeJSON{p.Wanted.X, p.Wanted.Y}
		}
		out = append(out, row)
	}
	return out
}

// measureFile is renderFile with the trace on. The two stay side by side
// because they answer the same question about which size to lay out at.
func measureFile(source, size, key string, resources map[string][]byte) (scene.Result, error, error) {
	document, warnings, err := loadDocument(source, resources)
	if err != nil {
		return scene.Result{}, err, nil
	}
	result, traceErr, usage := traceDocument(document, size, key)
	result.Report.Warnings = append(warnings, result.Report.Warnings...)
	return result, traceErr, usage
}

func traceDocument(document compose.Document, size, key string) (scene.Result, error, error) {
	switch {
	case key != "":
		known, err := panel.ByKey(key)
		if err != nil {
			return scene.Result{}, nil, err
		}
		result, err := scene.TraceForSize(document, known.Size())
		return result, err, nil
	case size != "":
		bounds, err := panel.ParseSize(size)
		if err != nil {
			return scene.Result{}, nil, err
		}
		result, err := scene.TraceForSize(document, bounds)
		return result, err, nil
	}
	if document.Size == (image.Point{}) {
		return scene.Result{}, nil, fmt.Errorf("%w: give the document a size, or measure with -size WxH or -panel family:id", scene.ErrNoSize)
	}
	result, err := scene.TraceForSize(document, document.Size)
	return result, err, nil
}

// renderFile lays a scene out at whichever size was asked for.
//
// Naming a panel does more than set the size: the page is packed for it too,
// and the bytes thrown away. That is the only thing here that catches an ink
// the panel cannot show, which a preview cannot — a PNG of a red heading looks
// right whether or not the panel bound for it has a colour plane.
//
// The third return is for what the caller wrote rather than what the scene
// contains. A misspelled panel or size is a usage error and exits 2, the same
// as an unknown week start or an unknown family; a scene that will not lay out
// exits 1.
func renderFile(source, size, key string, resources map[string][]byte) (scene.Result, error, error) {
	document, warnings, err := loadDocument(source, resources)
	if err != nil {
		return scene.Result{}, err, nil
	}
	result, renderErr, usage := renderDocument(document, size, key)
	// What the front end could not honour is reported beside what the layout
	// could not honour, because an author reading the output does not care
	// which half of the pipeline dropped their declaration.
	result.Report.Warnings = append(warnings, result.Report.Warnings...)
	return result, renderErr, usage
}

func renderDocument(document compose.Document, size, key string) (scene.Result, error, error) {
	switch {
	case key != "":
		known, err := panel.ByKey(key)
		if err != nil {
			return scene.Result{}, nil, err
		}
		result, _, err := panel.Render(document, known)
		return result, err, nil
	case size != "":
		bounds, err := panel.ParseSize(size)
		if err != nil {
			return scene.Result{}, nil, err
		}
		result, err := scene.RenderForSize(document, bounds)
		return result, err, nil
	}
	if document.Size == (image.Point{}) {
		// A usage error rather than a bad scene: the document is fine, nobody
		// said how big to draw it, and there are three ways to say.
		return scene.Result{}, nil, fmt.Errorf("%w: give the document a size, or render with -size WxH or -panel family:id", scene.ErrNoSize)
	}
	result, err := scene.RenderForSize(document, document.Size)
	return result, err, nil
}

func printWarnings(writer io.Writer, warnings []compose.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(writer, "warning %s [%s]: %s\n", warning.Path, warning.Code, warning.Message)
	}
}

func printReport(writer io.Writer, result scene.Result) {
	if len(result.Report.MissingRunes) != 0 {
		fmt.Fprintf(writer, "missing runes: %q\n", string(result.Report.MissingRunes))
	}
	printWarnings(writer, result.Report.Warnings)
	for _, expansion := range result.Report.GridExpansions {
		fmt.Fprintf(writer, "grid %s: implicit-columns=%d implicit-rows=%d\n",
			expansion.Path, expansion.ImplicitColumns, expansion.ImplicitRows)
	}
	for _, decision := range result.Report.Images {
		// Named rather than numbered. These printed as the integers behind the
		// enums, so a line reading dither=2 said nothing anybody could look up
		// and did not say which list to count along.
		fmt.Fprintf(writer, "image %s: dither=%s fit=%s sampling=%s threshold=%d red-disabled=%t\n",
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
	prefix := "usage: "
	for _, line := range usageLines {
		fmt.Fprintf(writer, "%s%s\n", prefix, usageFor(line.name))
		prefix = "       "
	}
}
