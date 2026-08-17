package main

import (
	"bytes"
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
func TestCommandHelpExitsZeroButBadFlagsDoNot(t *testing.T) {
	for command := range flagSets(t) {
		var out, errOut bytes.Buffer
		if code := run([]string{command, "-h"}, &out, &errOut); code != 0 {
			t.Errorf("%s -h exited %d", command, code)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"render", "-nosuchflag"}, &out, &errOut); code == 0 {
		t.Error("an unknown flag was accepted")
	}
}

// A binary that cannot say what it is cannot be reported against. Whatever it
// was built from, it has to answer with something.
func TestBuildVersionAlwaysSaysSomething(t *testing.T) {
	if strings.TrimSpace(buildVersion()) == "" {
		t.Error("buildVersion is empty")
	}
}
