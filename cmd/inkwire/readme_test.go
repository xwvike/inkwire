package main

import (
	"fmt"
	"github.com/xwvike/inkwire/internal/panel"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var readmes = []string{"../../README.md", "../../README.zh-CN.md"}

// flagSets returns the flags each subcommand defines, read from the flag set it
// builds. The document's command reference is checked against this rather than
// against a list written alongside it, which would only record what its author
// believed the commands took.
func flagSets(t *testing.T) map[string]map[string]bool {
	t.Helper()
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// Each subcommand names its flag set after itself, so splitting on the
	// constructor gives one section per command. The constructor is this
	// package's own `command`, which is flag.NewFlagSet plus the usage line -h
	// answers with.
	constructor := regexp.MustCompile(`command\("([\w-]+)"`)
	sections := constructor.Split(string(source), -1)
	names := constructor.FindAllStringSubmatch(string(source), -1)
	declaration := regexp.MustCompile(`flags\.(?:String|Int|Bool|Duration|Float64)\("([\w-]+)"`)
	variableDeclaration := regexp.MustCompile(`flags\.Var\([^,]+,\s*"([\w-]+)"`)

	sets := map[string]map[string]bool{}
	for index, name := range names {
		body, _, _ := strings.Cut(sections[index+1], "flags.Parse")
		flags := map[string]bool{}
		for _, match := range declaration.FindAllStringSubmatch(body, -1) {
			flags[match[1]] = true
		}
		for _, match := range variableDeclaration.FindAllStringSubmatch(body, -1) {
			flags[match[1]] = true
		}
		sets[name[1]] = flags
	}
	if len(sets) < 6 {
		t.Fatalf("found %d flag sets, which cannot be right", len(sets))
	}
	return sets
}

// Every subcommand main dispatches to, and every flag each of them defines.
func TestBothReadmesDocumentEveryCommandAndFlag(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	commands := regexp.MustCompile(`case "([\w-]+)":\n\s*return run`).FindAllStringSubmatch(string(source), -1)
	if len(commands) < 6 {
		t.Fatalf("found %d subcommands, which cannot be right", len(commands))
	}
	sets := flagSets(t)

	for _, path := range readmes {
		doc := readDoc(t, path)
		for _, command := range commands {
			if !strings.Contains(doc, "inkwire "+command[1]) {
				t.Errorf("%s never shows the %q subcommand", path, command[1])
			}
			for flag := range sets[command[1]] {
				if !strings.Contains(doc, "-"+flag) {
					t.Errorf("%s never shows %s's -%s flag", path, command[1], flag)
				}
			}
		}
	}
}

// The other direction: a flag the document shows against a command that does
// not define it. This reads as working right up until it is run, because the
// flag is real — it just belongs to a different command.
func TestNeitherReadmeInventsAFlag(t *testing.T) {
	sets := flagSets(t)
	invocation := regexp.MustCompile(`inkwire ([\w-]+)((?: [^\n|]*)?)`)
	written := regexp.MustCompile(`(?:^|[^\w-])-([a-z][\w-]*)`)

	for _, path := range readmes {
		doc := readDoc(t, path)
		for _, call := range invocation.FindAllStringSubmatch(doc, -1) {
			flags, isCommand := sets[call[1]]
			if !isCommand {
				continue
			}
			for _, flag := range written.FindAllStringSubmatch(call[2], -1) {
				if !flags[flag[1]] {
					t.Errorf("%s shows `inkwire %s -%s`, which that command does not define",
						path, call[1], flag[1])
				}
			}
		}
	}
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

// Every warning code the compiler and the scene layer can produce, checked
// against both READMEs.
//
// size-mismatch was reported for a fortnight before either README mentioned it.
// A warning nobody has written down is a warning nobody can look up, and the
// tables are exactly the kind of list that stops being complete the moment the
// code that fills them moves on.
func TestBothReadmesDocumentEveryWarningCode(t *testing.T) {
	// panel joined the list when it started warning: the ink a page asked for
	// and the panel it is bound for are only both known there, so that is
	// where the warning about the two disagreeing has to be built. markup and
	// this directory joined it with the stylesheet front end, which reports a
	// declaration it could not honour rather than quietly dropping it — a
	// promise worth nothing if the codes are not documented.
	sources := []string{
		"../../internal/compose", "../../internal/scene", "../../internal/panel",
		"../../internal/markup", ".",
	}
	emitted := map[string]string{}
	// ctx.warn(path, "code", …), a Warning literal a package builds by hand,
	// and a report callback handed a code are the three ways one reaches a
	// report. The third was added after two codes went undocumented: they
	// were raised where the stylesheet is parsed, which has no compiler to
	// call warn on and so passes a function instead.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\.warn\([^,]+,\s*"([a-z-]+)"`),
		regexp.MustCompile(`Code:\s*"([a-z-]+)"`),
		regexp.MustCompile(`report\(\s*"([a-z-]+)"\s*,`),
	}
	for _, dir := range sources {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, pattern := range patterns {
				for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
					emitted[match[1]] = filepath.Join(dir, entry.Name())
				}
			}
		}
	}
	if len(emitted) < 4 {
		t.Fatalf("found %d warning codes, which cannot be right", len(emitted))
	}

	for _, path := range readmes {
		doc := readDoc(t, path)
		for code, where := range emitted {
			// A row of the table, not a mention in passing. Naming a code in
			// a sentence somewhere is not the same as being able to look it
			// up, and the first version of this test accepted one for the
			// other.
			row := regexp.MustCompile(`(?m)^\| ` + "`" + code + "`" + ` \|`)
			if !row.MatchString(doc) {
				t.Errorf("%s has no table row for the %s warning, which %s emits", path, code, where)
			}
		}
	}
}

// Every panel either family knows, checked against both READMEs by id and by
// size.
//
// A table in a document is a copy of a table in the code, and the copy is the
// one nobody remembers. The Gicisky list went unwritten entirely until it was
// asked for; the warning codes drifted the same way a week earlier.
func TestBothReadmesListEveryPanelModel(t *testing.T) {
	type row struct{ id, size, name string }
	var panels []row
	for _, known := range panel.All() {
		panels = append(panels, row{
			id:   fmt.Sprintf("`%s`", known.ID()),
			size: fmt.Sprintf("%dx%d", known.Size().X, known.Size().Y),
			name: known.Name(),
		})
	}
	if len(panels) < 20 {
		t.Fatalf("found %d panels between the two families, which cannot be right", len(panels))
	}

	for _, path := range readmes {
		doc := readDoc(t, path)
		for _, p := range panels {
			row := regexp.MustCompile(`(?m)^\|[^|]*` + regexp.QuoteMeta(p.id) + `[^|]*\|(.*)$`)
			match := row.FindStringSubmatch(doc)
			if match == nil {
				t.Errorf("%s has no row for %s (%s)", path, p.id, p.name)
				continue
			}
			if !strings.Contains(match[1], p.size) {
				t.Errorf("%s lists %s (%s) without %s", path, p.id, p.name, p.size)
			}
		}
	}
}
