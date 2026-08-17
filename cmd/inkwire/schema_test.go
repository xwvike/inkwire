package main

import (
	"bytes"
	"os"
	"testing"
)

// The point of carrying the schema in the binary is that whatever is driving
// this program can ask it what a scene may contain and get the truth, rather
// than whatever version of a document it found somewhere. That only holds if
// what comes out is the document this repository publishes, so this compares
// the two rather than checking the output merely looks like a schema.
func TestSchemaPrintsTheDocumentThisRepositoryPublishes(t *testing.T) {
	for _, translation := range []struct{ language, path string }{
		{"en", "../../SCHEMA.md"},
		{"zh", "../../SCHEMA.zh-CN.md"},
	} {
		published, err := os.ReadFile(translation.path)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := run([]string{"schema", "-lang", translation.language}, &out, &errOut); code != 0 {
			t.Errorf("schema -lang %s exited %d: %s", translation.language, code, errOut.String())
			continue
		}
		if out.String() != string(published) {
			t.Errorf("schema -lang %s printed %d bytes, %s holds %d",
				translation.language, out.Len(), translation.path, len(published))
		}
	}
}

// A language nobody has a translation for is refused rather than silently
// answered in English, which would read as a translation that exists.
func TestSchemaRefusesALanguageItDoesNotHave(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"schema", "-lang", "fr"}, &out, &errOut); code == 0 {
		t.Errorf("an unknown language succeeded, printing %d bytes", out.Len())
	}
	if out.Len() != 0 {
		t.Errorf("an unknown language still printed %d bytes", out.Len())
	}
}
