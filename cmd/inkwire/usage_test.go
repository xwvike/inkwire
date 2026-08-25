package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Everything this program does is a subcommand, so an unrecognised first
// argument used to be taken for a payload path. `inkwire --help` on a freshly
// downloaded binary answered "read payload: open --help: no such file or
// directory" — the first thing anybody types, answered with a complaint about
// a missing file.
func TestTheConventionalHelpAndVersionAreAnswered(t *testing.T) {
	for _, spelling := range []string{"help", "-h", "--help"} {
		var out, errOut bytes.Buffer
		if code := run([]string{spelling}, &out, &errOut); code != 0 {
			t.Errorf("%q exited %d, and asking for help is not an error", spelling, code)
		}
		// Help belongs on stdout: it is what was asked for, not a diagnostic.
		if !strings.Contains(out.String(), "usage: inkwire") {
			t.Errorf("%q printed %q to stdout", spelling, out.String())
		}
	}
	for _, spelling := range []string{"version", "-v", "--version"} {
		var out, errOut bytes.Buffer
		if code := run([]string{spelling}, &out, &errOut); code != 0 {
			t.Errorf("%q exited %d", spelling, code)
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Errorf("%q printed nothing", spelling)
		}
	}
}

// The usage text is the only place a reader of the binary alone learns what it
// can do, so every command the dispatcher answers to has to appear in it.
func TestUsageNamesEveryCommand(t *testing.T) {
	var usage bytes.Buffer
	printUsage(&usage)
	for command := range flagSets(t) {
		if !strings.Contains(usage.String(), "inkwire "+command) {
			t.Errorf("usage never mentions %q:\n%s", command, usage.String())
		}
	}
	for _, command := range []string{"schema", "version"} {
		if !strings.Contains(usage.String(), "inkwire "+command) {
			t.Errorf("usage never mentions %q", command)
		}
	}
}

// An unknown option is a mistake worth naming. Falling through to the payload
// path turned it into a puzzle about the filesystem.
func TestAnUnknownOptionIsRefusedRatherThanOpened(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--nosuchthing"}, &out, &errOut)
	if code == 0 {
		t.Error("an unknown option succeeded")
	}
	if !strings.Contains(errOut.String(), "--nosuchthing") {
		t.Errorf("the refusal does not name the option: %q", errOut.String())
	}
	if strings.Contains(errOut.String(), "read payload") {
		t.Errorf("the option was still taken for a file: %q", errOut.String())
	}
}

// Asking a command for its flags is not a failure either, and the flag package
// reports it the same way it reports a genuine parse error.
//
// What it prints matters as much as where it exits. Go's own text is
// "Usage of push:" followed by the flags, which names neither the program nor
// the arguments that are not flags — a reader of `inkwire push -h` would not
// learn that it wants a scene document. Each command has a line saying the
// whole shape, and this is what checks that -h is given that line.
func TestEveryCommandAnswersHelpWithItsOwnUsage(t *testing.T) {
	commands := []string{"render", "scan"}
	for command := range flagSets(t) {
		commands = append(commands, command)
	}
	for _, command := range commands {
		var out, errOut bytes.Buffer
		if code := run([]string{command, "-h"}, &out, &errOut); code != 0 {
			t.Errorf("%s -h exited %d: %s", command, code, errOut.String())
			continue
		}
		// Help goes to stdout: it is the answer, not a diagnostic.
		if errOut.Len() != 0 {
			t.Errorf("%s -h wrote to stderr: %q", command, errOut.String())
		}
		first, _, _ := strings.Cut(out.String(), "\n")
		if want := "usage: inkwire " + command; !strings.HasPrefix(first, want) {
			t.Errorf("%s -h begins %q, want a line starting %q", command, first, want)
		}
	}

	var out, errOut bytes.Buffer
	if code := run([]string{"render", "-nosuchflag"}, &out, &errOut); code == 0 {
		t.Error("an unknown flag was accepted")
	}
}

// The usage a command prints when it is asked, and the usage it prints when the
// arguments are wrong, are the same sentence about the same command. They used
// to be two: one written by hand for the argument-count case, and Go's default
// for -h.
func TestHelpAndTheArgumentErrorAgree(t *testing.T) {
	// Commands that refuse an empty invocation, with what they refuse it over.
	for _, command := range []string{"render", "push"} {
		var help, ignored bytes.Buffer
		run([]string{command, "-h"}, &help, &ignored)

		var out, errOut bytes.Buffer
		if code := run([]string{command}, &out, &errOut); code != 2 {
			t.Errorf("%s with no arguments exited %d, want 2", command, code)
		}
		// The complaint belongs on stderr, and has to open with the same line.
		helpFirst, _, _ := strings.Cut(help.String(), "\n")
		errFirst, _, _ := strings.Cut(errOut.String(), "\n")
		if helpFirst != errFirst {
			t.Errorf("%s: -h says %q, the argument error says %q", command, helpFirst, errFirst)
		}
	}
}

// A binary that cannot say what it is cannot be reported against. Whatever it
// was built from, it has to answer with something.
func TestBuildVersionAlwaysSaysSomething(t *testing.T) {
	if strings.TrimSpace(buildVersion()) == "" {
		t.Error("buildVersion is empty")
	}
}

// A flag description may name the argument's type by backquoting one word in
// it, and the flag package takes the first one it finds. Anything else that
// happens to be backquoted stays literal, so a description with two of them
// prints one as the type name and the other with its quotes still on.
//
// -device did this once, rendering as `-device inkwire scan`, and -panel did
// it again with an example for its type. Both were only visible by running -h
// and reading it, which is not something a test was doing.
func TestNoFlagDescriptionCarriesAStrayBackquote(t *testing.T) {
	for command := range flagSets(t) {
		var out, errOut bytes.Buffer
		run([]string{command, "-h"}, &out, &errOut)
		help := out.String() + errOut.String()
		if strings.Contains(help, "`") {
			t.Errorf("%s -h prints a backquote, so a description has more than one:\n%s", command, help)
		}
	}
}

// The summary and each command's own help are the same sentence.
//
// They were two strings for a while, one in printUsage and one handed to
// command(), and they drifted the way two copies of a sentence do: the summary
// never learned that push and mode take -settle. Nothing would show it except
// reading both and comparing, which is what this does — except that they now
// come from one table, so this is checking that they still do.
func TestTheSummaryAndEachCommandsHelpAgree(t *testing.T) {
	var summary bytes.Buffer
	printUsage(&summary)
	lines := strings.Split(strings.TrimSpace(summary.String()), "\n")
	if len(lines) != len(usageLines) {
		t.Fatalf("summary has %d lines, the table has %d entries", len(lines), len(usageLines))
	}

	for index, line := range usageLines {
		want := usageFor(line.name)
		if got := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[index]), "usage:")); got != want {
			t.Errorf("summary line %d is %q, want %q", index, got, want)
		}
		if line.name == "version" {
			continue // It takes no flags, so it has no flag set to ask.
		}
		var help, ignored bytes.Buffer
		if code := run([]string{line.name, "-h"}, &help, &ignored); code != 0 {
			t.Errorf("%s -h exited %d", line.name, code)
			continue
		}
		first, _, _ := strings.Cut(help.String(), "\n")
		if got := strings.TrimPrefix(first, "usage: "); got != want {
			t.Errorf("%s -h says %q, the summary says %q", line.name, got, want)
		}
	}
}

// Every command the dispatcher answers has a line in the table. A command that
// runs and cannot say what it takes is worse than one that does not exist:
// usageFor panics rather than printing a line with the arguments missing, and
// that panic should be a test failure here rather than something a user sees.
func TestEveryDispatchedCommandHasAUsageLine(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	dispatched := regexp.MustCompile(`case "([\w-]+)"(?:, "[^"]*")*:\n\s*(?:return run|fmt\.Fprintln\(stdout, buildVersion)`).
		FindAllStringSubmatch(string(source), -1)
	if len(dispatched) < 7 {
		t.Fatalf("found %d dispatched commands, which cannot be right", len(dispatched))
	}
	listed := map[string]bool{}
	for _, line := range usageLines {
		listed[line.name] = true
	}
	for _, match := range dispatched {
		if !listed[match[1]] {
			t.Errorf("%q is dispatched but has no line in usageLines", match[1])
		}
	}
}

// A usage line names every flag its command takes.
//
// This is what would have caught the drift that started this: -settle existed,
// push declared it, and the line describing push did not mention it. Agreement
// between the two printers cannot catch that on its own — once they read one
// table, dropping a flag from that table drops it from both and they go on
// agreeing about a sentence that is missing something.
func TestEveryUsageLineNamesEveryFlagItsCommandTakes(t *testing.T) {
	for name, flags := range flagSets(t) {
		line := usageFor(name)
		for flag := range flags {
			if !strings.Contains(line, "-"+flag) {
				t.Errorf("usage for %s does not name -%s: %q", name, flag, line)
			}
		}
	}
}
