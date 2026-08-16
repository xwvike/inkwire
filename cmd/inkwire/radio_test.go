package main

import (
	"os"
	"regexp"
	"testing"
)

// Every command that reaches a tag must do so through a retrying entry point.
//
// `inkwire mode` did not, and the difference only showed on hardware: a tag
// takes an unsteady 6s to 18s to advertise again after a disconnect, against a
// 15s scan, so the single attempt reported the tag missing when it was sitting
// on the desk. Every other command already retried, which is why only this one
// failed and why nobody had noticed.
//
// The direct forms stay exported and stay useful — a caller that wants one
// attempt can have one. This says the command line is not that caller.
func TestNoCommandReachesTheRadioInOneAttempt(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// `PushWithRetry(` does not match, because the paren is what is looked for.
	direct := regexp.MustCompile(`\.(Push|SetMode)\(`)
	for _, call := range direct.FindAllString(string(source), -1) {
		t.Errorf("main.go calls %s directly; use the WithRetry form so a missed "+
			"advertising window is not reported as a missing tag", call)
	}
}
