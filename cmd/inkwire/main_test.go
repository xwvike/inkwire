package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/server"
)

func TestRenderAndEncodeCommands(t *testing.T) {
	directory := t.TempDir()
	scenePath := filepath.Join(directory, "page.json")
	scene := `{
		"version":1,
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

	stdout.Reset()
	stderr.Reset()
	payloadPath := filepath.Join(directory, "payload.bin")
	if code := run([]string{"encode", "-o", payloadPath, scenePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("encode code = %d, stderr = %s", code, stderr.String())
	}
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("payload = %d bytes", len(payload))
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
	if httpServer.WriteTimeout <= server.DefaultPushTimeout {
		t.Errorf("write timeout %s does not outlast the push budget %s", httpServer.WriteTimeout, server.DefaultPushTimeout)
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

// Picking the wrong family is not a failure that announces itself. The two
// speak different services and pack pixels differently, so a page sent to the
// wrong driver either finds nothing to talk to or writes bytes that mean
// something else into a panel's RAM.
func TestTheFamilyIsChosenFromTheTargetOrStatedOutright(t *testing.T) {
	tests := []struct {
		requested, target, want string
		refused                 bool
	}{
		// The name is the one thing an EPD-nRF5 tag says about itself.
		{requested: "auto", target: "NRF_EPD_C1F8", want: familyNRFEPD},
		{requested: "auto", target: "nrf_epd_c1f8", want: familyNRFEPD},
		// Everything else keeps the family that has always been the default.
		{requested: "auto", target: "PICKSMART", want: familyGicisky},
		{requested: "auto", target: "FF:FF:92:94:38:61", want: familyGicisky},
		// An address says nothing about the family, so saying so is how an
		// EPD-nRF5 tag is reached by address at all.
		{requested: familyNRFEPD, target: "FF:FF:92:94:38:61", want: familyNRFEPD},
		{requested: familyGicisky, target: "NRF_EPD_C1F8", want: familyGicisky},
		// A misspelling is refused rather than quietly treated as auto.
		{requested: "nrf", target: "NRF_EPD_C1F8", refused: true},
	}
	for _, test := range tests {
		got, err := resolveFamily(test.requested, test.target)
		if test.refused {
			if err == nil {
				t.Errorf("family %q was accepted", test.requested)
			}
			continue
		}
		if err != nil {
			t.Errorf("family %q target %q: %v", test.requested, test.target, err)
			continue
		}
		if got != test.want {
			t.Errorf("family %q target %q chose %s, want %s", test.requested, test.target, got, test.want)
		}
	}
}
