package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// runTailscale handles `velocity-ctl tailscale enable-tailscaled|disable-tailscaled`.
//
// These subcommands exist so the velocity-report server (running as the
// non-root `velocity` user) can drive the daemon via a narrow sudoers
// allowlist:
//
//	pi/velocity ALL=(root) NOPASSWD: /usr/local/bin/velocity-ctl tailscale *
//
// We deliberately do not expose `tailscale up` / `tailscale logout`
// here — those are reachable through tailscaled's local API socket and
// don't need root.  Only the systemd lifecycle (mask/unmask/start/stop)
// requires elevation, and that's all this wrapper does.
func runTailscale(args []string) error {
	fs := flag.NewFlagSet("tailscale", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: velocity-ctl tailscale <enable-tailscaled|disable-tailscaled>")
	}
	switch rest[0] {
	case "enable-tailscaled":
		return enableTailscaled()
	case "disable-tailscaled":
		return disableTailscaled()
	default:
		return fmt.Errorf("unknown tailscale subcommand: %s", rest[0])
	}
}

func enableTailscaled() error {
	// Order matters: unmask first (no-op if already unmasked), then
	// enable for persistence across reboots, then start now.  Each
	// step is logged so failures surface clearly in the journal.
	steps := [][]string{
		{"systemctl", "unmask", "tailscaled.service"},
		{"systemctl", "enable", "tailscaled.service"},
		{"systemctl", "start", "tailscaled.service"},
	}
	for _, step := range steps {
		if out, err := exec.Command(step[0], step[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %v: %s", step[1], err, string(out))
		}
	}

	// Wait for the local-API socket to appear, then grant the
	// `velocity` service account operator rights on it.  Without this,
	// the Go server (running as `velocity`, non-root) cannot drive
	// login/serve via tailscale.com/client/local — `tailscale set
	// --operator` adjusts the socket's permissions to match.
	socket := "/var/run/tailscale/tailscaled.sock"
	socketReady := false
	for range 60 {
		if _, err := os.Stat(socket); err == nil {
			socketReady = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !socketReady {
		return fmt.Errorf("tailscaled socket %s did not appear within 15s", socket)
	}
	if out, err := exec.Command("tailscale", "set", "--operator=velocity").CombinedOutput(); err != nil {
		// Fatal: without this the velocity service user cannot drive
		// the local API socket and the UI flow will fail with a
		// confusing "permission denied" several seconds later.  Fail
		// loudly here instead.
		return fmt.Errorf("set --operator=velocity: %v: %s", err, string(out))
	}
	return nil
}

func disableTailscaled() error {
	// Stop and mask so the daemon does not auto-start on next boot.
	// `disable` is implied by `mask` but we issue it explicitly so the
	// transition is visible in journalctl.
	steps := [][]string{
		{"systemctl", "stop", "tailscaled.service"},
		{"systemctl", "disable", "tailscaled.service"},
		{"systemctl", "mask", "tailscaled.service"},
	}
	for _, step := range steps {
		if out, err := exec.Command(step[0], step[1:]...).CombinedOutput(); err != nil {
			// Don't abort on stop failures — the daemon may already be down.
			// Continue so mask still happens.
			fmt.Fprintf(os.Stderr, "velocity-ctl tailscale: %s: %v: %s\n", step[1], err, string(out))
		}
	}
	return nil
}
