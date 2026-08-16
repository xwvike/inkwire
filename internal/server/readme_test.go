package server

import (
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The READMEs describe this package's HTTP surface, and a caller reads them
// instead of reading server.go. Both are checked, because a route or a code
// documented in one language and not the other is documented for half the
// readers.
var readmes = []string{"../../README.md", "../../README.zh-CN.md"}

// Every route the mux serves. A route added here and not to the document is a
// feature nobody can find.
func TestBothReadmesDocumentEveryRoute(t *testing.T) {
	source := readServer(t)
	routes := regexp.MustCompile(`HandleFunc\("(?:GET|POST) (/[\w/]+)"`).FindAllStringSubmatch(source, -1)
	if len(routes) < 4 {
		t.Fatalf("found %d routes, which cannot be right", len(routes))
	}
	for _, path := range readmes {
		doc := readDoc(t, path)
		for _, route := range routes {
			if !strings.Contains(doc, route[1]) {
				t.Errorf("%s never mentions the %s route", path, route[1])
			}
		}
	}
}

// Error codes are the part of an HTTP API a caller writes a switch against, so
// a code that exists and is not documented is a branch nobody wrote, and a
// status documented as one number while the server sends another is a branch
// written wrongly. Both halves come from the call sites rather than from the
// document's own table.
func TestBothReadmesDocumentEveryErrorCode(t *testing.T) {
	source := readServer(t)
	status := map[string]string{}
	// The two arguments appear in either order depending on whether the call
	// site writes the response directly or picks a code to write later.
	for _, match := range regexp.MustCompile(`http\.(Status\w+),\s*"([a-z-]+)"`).FindAllStringSubmatch(source, -1) {
		status[match[2]] = match[1]
	}
	for _, match := range regexp.MustCompile(`"([a-z-]+)",\s*http\.(Status\w+)`).FindAllStringSubmatch(source, -1) {
		status[match[1]] = match[2]
	}
	if len(status) < 10 {
		t.Fatalf("found %d error codes, which cannot be right", len(status))
	}

	for _, path := range readmes {
		doc := readDoc(t, path)
		for code, constant := range status {
			want := statusNumber(t, constant)
			row := regexp.MustCompile(`(?m)^\|\s*` + "`" + regexp.QuoteMeta(code) + "`" + `\s*\|\s*(\d+)`).FindStringSubmatch(doc)
			if row == nil {
				t.Errorf("%s has no error table row for %q (%d)", path, code, want)
				continue
			}
			if got, _ := strconv.Atoi(row[1]); got != want {
				t.Errorf("%s says %q is %d, server sends %d", path, code, got, want)
			}
		}
	}
}

// statusNumber turns a net/http constant name back into its number, so the
// expected status comes from net/http rather than from a table written here
// that could disagree with it.
func statusNumber(t *testing.T, constant string) int {
	t.Helper()
	name := strings.TrimPrefix(constant, "Status")
	for code := 100; code < 600; code++ {
		if strings.ReplaceAll(http.StatusText(code), " ", "") == name {
			return code
		}
	}
	t.Fatalf("net/http has no status called %q", constant)
	return 0
}

func readServer(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}
