package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
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
