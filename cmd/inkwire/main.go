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

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/gicisky"
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
		if len(args) != 1 {
			printUsage(stderr)
			return 2
		}
		if !enableBluetooth(logger) {
			return 1
		}
		driver := gicisky.NewDriver(bluetooth.DefaultAdapter, gicisky.TargetAddress, logger.Printf)
		device, err := driver.Find(ctx)
		if err != nil {
			logger.Print(err)
			return 1
		}
		fmt.Fprintf(stdout, "%s %s RSSI=%d\n", device.Address.String(), device.Name, device.RSSI)
		return 0
	case "push":
		return runPushScene(ctx, args[1:], logger, stderr)
	case "push-payload":
		return runPushPayload(ctx, args[1:], logger, stderr)
	case "serve":
		return runServe(ctx, args[1:], logger, stderr)
	default:
		// Preserve the original raw-payload invocation while the JSON commands
		// become the normal user-facing path.
		return runPushPayload(ctx, args, logger, stderr)
	}
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
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: inkwire push [-device MAC-or-name] <scene.json>")
		return 2
	}
	result, err := (scene.Decoder{}).RenderFile(flags.Arg(0))
	if err != nil {
		logger.Print(err)
		return 1
	}
	payload, err := result.Payload()
	if err != nil {
		logger.Print(err)
		return 1
	}
	printReport(logger.Writer(), result)
	if !enableBluetooth(logger) {
		return 1
	}
	return push(ctx, bluetooth.DefaultAdapter, *target, payload, logger)
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
	fmt.Fprintln(writer, "       inkwire push [-device MAC-or-name] <scene.json>")
	fmt.Fprintln(writer, "       inkwire scan")
	fmt.Fprintln(writer, "       inkwire serve [-listen address] [-device MAC-or-name] [-assets directory]")
	fmt.Fprintln(writer, "       inkwire push-payload [MAC-or-name] <payload.bin>")
}
