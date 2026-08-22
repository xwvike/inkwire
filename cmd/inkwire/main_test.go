package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/server"
)

func TestRenderWritesAPreview(t *testing.T) {
	directory := t.TempDir()
	scenePath := filepath.Join(directory, "page.json")
	scene := `{
		"version":1,
		"size":{"width":296,"height":128},
		"root":{"type":"absolute","children":[
			{"bounds":{"x":0,"y":0,"width":30,"height":20},"node":{"type":"rectangle","fill":"red"}},
			{"bounds":{"x":2,"y":2,"width":26,"height":14},"node":{"type":"text","runs":[{"text":"API","font":"monaco","size":10,"ink":"black"}]}}
		]}
	}`
	if err := os.WriteFile(scenePath, []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	pngPath := filepath.Join(directory, "preview.png")
	if code := run([]string{"render", "-o", pngPath, scenePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, stderr = %s", code, stderr.String())
	}
	if info, err := os.Stat(pngPath); err != nil || info.Size() == 0 {
		t.Fatalf("preview = %v, %v", info, err)
	}
}

// serve has no authentication and every request writes to the tag, so the
// address is the whole access control. Bind it wrong and the guard is the only
// thing between the hardware and the network.
func TestServeOnlyBindsLoopback(t *testing.T) {
	allowed := []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "127.0.0.2:9000"}
	for _, address := range allowed {
		if err := requireLoopback(address); err != nil {
			t.Errorf("requireLoopback(%q) = %v, want accepted", address, err)
		}
	}
	refused := []string{":8080", "0.0.0.0:8080", "192.168.1.10:8080", "[::]:8080", "8080", ""}
	for _, address := range refused {
		if err := requireLoopback(address); err == nil {
			t.Errorf("requireLoopback(%q) was accepted and would expose the tag", address)
		}
	}
}

// A write has to outlast a whole push or the server would cut its own
// transfer off partway and report a failure the tag never had.
func TestServeTimeoutsCannotCutAPushShort(t *testing.T) {
	httpServer := newHTTPServer("127.0.0.1:0", nil)
	for family, budget := range map[string]time.Duration{
		"gicisky": server.DefaultPushTimeout,
		"nrfepd":  server.DefaultNRFEPDPushTimeout,
	} {
		if httpServer.WriteTimeout <= budget {
			t.Errorf("write timeout %s does not outlast the %s push budget %s", httpServer.WriteTimeout, family, budget)
		}
	}
	// Each of these bounds a connection that would otherwise sit on the
	// adapter, so an unset one is a hole rather than a lenient default.
	for _, timeout := range []struct {
		name  string
		value time.Duration
	}{
		{"ReadHeaderTimeout", httpServer.ReadHeaderTimeout},
		{"ReadTimeout", httpServer.ReadTimeout},
		{"IdleTimeout", httpServer.IdleTimeout},
	} {
		if timeout.value <= 0 {
			t.Errorf("%s is unset, so a connection can be held open indefinitely", timeout.name)
		}
	}
}

func TestReportPrintsImplicitGridTracks(t *testing.T) {
	var output bytes.Buffer
	printReport(&output, scene.Result{Report: compose.Report{GridExpansions: []compose.GridExpansion{{
		Path: "root", ImplicitColumns: 2, ImplicitRows: 1,
	}}}})
	if got := output.String(); !strings.Contains(got, "grid root: implicit-columns=2 implicit-rows=1") {
		t.Fatalf("report output = %q", got)
	}
}

func TestServeRefusesANonLoopbackAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve", "-listen", "0.0.0.0:8080"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRenderReportsInvalidSceneWithoutCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	scenePath := filepath.Join(directory, "bad.json")
	if err := os.WriteFile(scenePath, []byte(`{"version":1,"root":{"type":"rectangle","colour":"black"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "bad.png")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", "-o", output, scenePath}, &stdout, &stderr); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("invalid scene created output: %v", err)
	}
}

// writeScene puts a document somewhere the render command can be pointed at.
func writeScene(t *testing.T, body string) (directory, path string) {
	t.Helper()
	directory = t.TempDir()
	path = filepath.Join(directory, "page.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return directory, path
}

const redPage = `{"version":1,"root":{"type":"absolute","children":[
	{"bounds":{"x":0,"y":0,"width":30,"height":20},"node":{"type":"rectangle","fill":"red"}}
]}}`

// A preview is laid out for whatever was asked for, and a scene that states no
// size no longer means one panel's size by default without saying so.
func TestRenderLaysOutForTheSizeOrPanelAskedFor(t *testing.T) {
	directory, scenePath := writeScene(t, redPage)
	for _, given := range []struct {
		flag, value, want string
	}{
		{"-size", "400x300", "(400x300)"},
		{"-panel", "gicisky:0x0033", "(296x128)"},
		{"-panel", "nrfepd:UC8176_420_BWR", "(400x300)"},
	} {
		var stdout, stderr bytes.Buffer
		pngPath := filepath.Join(directory, "preview.png")
		code := run([]string{"render", given.flag, given.value, "-o", pngPath, scenePath}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("render %s %s = %d, stderr = %s", given.flag, given.value, code, stderr.String())
			continue
		}
		if !strings.Contains(stdout.String(), given.want) {
			t.Errorf("render %s %s said %q, want %s", given.flag, given.value, stdout.String(), given.want)
		}
	}
}

// An ink the panel has no plane for does not stop the page. It is drawn black,
// the report says which ink and why, and the run succeeds — the same answer
// size-mismatch already gave. The refusal this replaced left somebody with a
// failed command and no picture to look at.
func TestAnInkThePanelLacksIsReportedRatherThanRefused(t *testing.T) {
	directory, scenePath := writeScene(t, redPage)
	pngPath := filepath.Join(directory, "preview.png")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", "-panel", "gicisky:0x0028", "-o", pngPath, scenePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("render code = %d, want 0; stderr = %s", code, stderr.String())
	}
	printed := stdout.String() + stderr.String()
	if !strings.Contains(printed, "unsupported-ink") || !strings.Contains(printed, "red") {
		t.Errorf("output = %q, want an unsupported-ink warning naming red", printed)
	}
	if info, err := os.Stat(pngPath); err != nil || info.Size() == 0 {
		t.Fatalf("preview = %v, %v", info, err)
	}
}

// A misspelled flag value is a usage error and exits 2, the same as an unknown
// family or an unknown week start. A scene that will not lay out exits 1, and
// the difference is what says whether to fix the command or the document.
func TestAMistypedPanelOrSizeExitsAsAUsageError(t *testing.T) {
	_, scenePath := writeScene(t, redPage)
	for _, args := range [][]string{
		{"render", "-panel", "0x0033", scenePath},
		{"render", "-panel", "gicisky:nope", scenePath},
		{"render", "-panel", "chicory:0x0033", scenePath},
		{"render", "-size", "400", scenePath},
		{"render", "-size", "400x300", "-panel", "gicisky:0x0033", scenePath},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%v = %d, want 2; stderr = %s", args[1:], code, stderr.String())
		}
	}
}

// There is no default page size any more. There was one, and it was one
// family's 2.9" panel written into the display layer, so a scene that said
// nothing got somebody else's tag and was never told. Saying nothing is now a
// usage error that names all three ways to say something.
func TestASceneWithNoSizeIsRefusedAndSaysHowToFixIt(t *testing.T) {
	_, scenePath := writeScene(t, redPage)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", scenePath}, &stdout, &stderr); code != 2 {
		t.Fatalf("render code = %d, want 2; stderr = %s", code, stderr.String())
	}
	for _, wanted := range []string{"states no size", "-size", "-panel"} {
		if !strings.Contains(stderr.String(), wanted) {
			t.Errorf("stderr = %q, want it to mention %q", stderr.String(), wanted)
		}
	}
	// Any one of the three is enough.
	for _, args := range [][]string{
		{"render", "-size", "296x128", scenePath},
		{"render", "-panel", "gicisky:0x0033", scenePath},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 0 {
			t.Errorf("%v = %d, stderr = %s", args[1:], code, errOut.String())
		}
	}
}
