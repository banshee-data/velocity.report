package device

import (
	"errors"
	"strings"
	"testing"
)

func TestRunTailscaleWithActions(t *testing.T) {
	var enabled, disabled int
	actions := tailscaleActions{
		enable: func() error {
			enabled++
			return nil
		},
		disable: func() error {
			disabled++
			return nil
		},
	}

	if err := runTailscaleWithActions([]string{"enable-tailscaled"}, actions); err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if err := runTailscaleWithActions([]string{"disable-tailscaled"}, actions); err != nil {
		t.Fatalf("disable failed: %v", err)
	}
	if enabled != 1 || disabled != 1 {
		t.Fatalf("actions called enabled=%d disabled=%d, want 1/1", enabled, disabled)
	}
}

func TestRunTailscaleErrors(t *testing.T) {
	boom := errors.New("boom")
	actions := tailscaleActions{
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
