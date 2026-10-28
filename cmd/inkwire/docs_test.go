package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The manuals in the binary are the ones in the repository, word for word,
// and each topic in each language is reachable by the command.
func TestDocsPrintsTheManualsAsWritten(t *testing.T) {
	for _, test := range []struct {
		args []string
		file string
	}{
		{[]string{"-lang", "en"}, "MARKUP.md"},
		{[]string{"-lang", "en", "markup"}, "MARKUP.md"},
		{[]string{"-lang", "zh-CN", "markup"}, "MARKUP.zh-CN.md"},
		{[]string{"-lang", "en", "readme"}, "README.md"},
		{[]string{"-lang", "zh-CN", "readme"}, "README.zh-CN.md"},
	} {
		want, err := os.ReadFile("../../" + test.file)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := runDocs(test.args, func(string) string { return "" }, &out, &errOut); code != 0 {
			t.Fatalf("docs %v exited %d: %s", test.args, code, errOut.String())
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Errorf("docs %v is not %s as written", test.args, test.file)
		}
	}
}

// Without -lang the manual is in the language the terminal is set to.
func TestDocsSpeaksTheLocaleLanguage(t *testing.T) {
	for _, test := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{}, "# Markup Manual"},
		{map[string]string{"LANG": "zh_CN.UTF-8"}, "# Markup 手册"},
		{map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "en_US.UTF-8"}, "# Markup Manual"},
		{map[string]string{"LANG": "en_GB.UTF-8", "LC_MESSAGES": "zh_TW.UTF-8"}, "# Markup 手册"},
	} {
		var out, errOut bytes.Buffer
		if code := runDocs(nil, func(name string) string { return test.env[name] }, &out, &errOut); code != 0 {
			t.Fatalf("docs exited %d: %s", code, errOut.String())
		}
		if !strings.HasPrefix(out.String(), test.want) {
			t.Errorf("with %v the manual starts %q; want %q", test.env, firstLine(out.String()), test.want)
		}
	}
}

func TestDocsRefusesWhatItDoesNotHave(t *testing.T) {
	for _, args := range [][]string{{"styles"}, {"-lang", "fr"}, {"markup", "readme"}} {
		var out, errOut bytes.Buffer
		if code := runDocs(args, func(string) string { return "" }, &out, &errOut); code != 2 {
			t.Errorf("docs %v exited %d; want 2", args, code)
		}
		if out.Len() != 0 {
			t.Errorf("docs %v printed a manual anyway", args)
		}
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
