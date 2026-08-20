package main

import (
	"os"
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

	sets := map[string]map[string]bool{}
	for index, name := range names {
		body, _, _ := strings.Cut(sections[index+1], "flags.Parse")
		flags := map[string]bool{}
		for _, match := range declaration.FindAllStringSubmatch(body, -1) {
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
