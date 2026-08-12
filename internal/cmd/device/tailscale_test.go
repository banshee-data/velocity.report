package device

import (
	"errors"
	"strings"
	"testing"
)

func TestRunTailscaleWithActions(t *testing.T) {
	var installed, enabled, disabled int
	actions := tailscaleActions{
		install: func() error {
			installed++
			return nil
		},
		enable: func() error {
			enabled++
			return nil
		},
		disable: func() error {
			disabled++
			return nil
		},
	}

	if err := runTailscaleWithActions([]string{"install"}, actions); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if err := runTailscaleWithActions([]string{"enable-tailscaled"}, actions); err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if err := runTailscaleWithActions([]string{"disable-tailscaled"}, actions); err != nil {
		t.Fatalf("disable failed: %v", err)
	}
	if installed != 1 || enabled != 1 || disabled != 1 {
		t.Fatalf("actions called installed/enabled/disabled=%d/%d/%d, want 1/1/1", installed, enabled, disabled)
	}
}

func TestRunTailscaleErrors(t *testing.T) {
	boom := errors.New("boom")
	actions := tailscaleActions{
		install: func() error { return boom },
		enable:  func() error { return boom },
		disable: func() error { return boom },
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing subcommand", nil, "usage:"},
		{"unknown subcommand", []string{"bogus"}, "unknown tailscale subcommand"},
		{"install error", []string{"install"}, "boom"},
		{"enable error", []string{"enable-tailscaled"}, "boom"},
		{"disable error", []string{"disable-tailscaled"}, "boom"},
		{"flag parse error", []string{"--bogus"}, "flag provided but not defined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runTailscaleWithActions(tc.args, actions)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runTailscaleWithActions(%v) error = %v, want containing %q", tc.args, err, tc.want)
			}
		})
	}
}

func TestRunTailscaleHelpReturnsNil(t *testing.T) {
	if err := runTailscaleWithActions([]string{"--help"}, tailscaleActions{}); err != nil {
		t.Fatalf("runTailscaleWithActions --help returned error: %v", err)
	}
}
