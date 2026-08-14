package scene

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The README documents this schema node by node, which means it is full of
// documents that either decode or are wrong. A reader who copies one and gets
// an error has been told something false by the thing that was meant to
// explain it, so every example is decoded here.
//
// Examples that elide part of their content with "..." are skipped: they are
// showing a shape, not offering a document.
func TestReadmeNodeExamplesDecode(t *testing.T) {
	source, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(string(source), -1)
	checked := 0
	for _, block := range blocks {
		body := strings.TrimSpace(block[1])
		if strings.Contains(body, "...") {
			continue
		}
		var probe map[string]any
		if json.Unmarshal([]byte(body), &probe) != nil {
			continue
		}
		if _, isNode := probe["type"].(string); !isNode {
			continue
		}
		checked++
		_, err := (Decoder{}).decodeNode([]byte(body), "readme")
		// An example is allowed to name a picture the repository does not
		// carry; that says nothing about whether the document is well formed.
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Errorf("README example does not decode: %v\n%s", err, body)
		}
	}
	// If the extractor ever stops matching, it would pass by finding nothing.
	if checked < 15 {
		t.Errorf("found only %d node examples in the README, which cannot be right", checked)
	}
}
