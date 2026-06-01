package main

import (
	"flag"
	"testing"
)

func TestParseCommandFlagsTreatsHelpAsHandled(t *testing.T) {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.Bool("check", false, "Check for updates without applying")

	handled, err := parseCommandFlags(fs, []string{"--help"})
	if err != nil {
		t.Fatalf("parseCommandFlags returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected help request to be reported as handled")
	}
}

func TestRunUpgradeHelpReturnsNil(t *testing.T) {
	if err := runUpgrade([]string{"--help"}); err != nil {
		t.Fatalf("runUpgrade returned error for help: %v", err)
	}
}
