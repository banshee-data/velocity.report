// Package device implements the `velocity device` namespace — the on-device
// management surface for velocity.report.
//
// It is mounted under the multi-call velocity binary as the `device` namespace.
// Runs as root on the Raspberry Pi. Manages upgrades, rollbacks, backups,
// and service status for the velocity-report server.
//
// Subcommands:
//
//	check     Report whether a newer release is available (read-only)
//	upgrade   Check for and apply new releases from GitHub
//	rollback  Restore the previous version via atomic symlink swap
//	backup    Create a manual snapshot of binary + database
//	status    Show systemd service status
//	tailscale Install and manage tailscaled lifecycle
//	version   Print installed version information
package device

import (
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/version"
)

const usage = `velocity device — on-device management for velocity.report

Usage:
  velocity device <command> [flags]

Commands:
  check     Report whether a newer release is available (read-only)
  upgrade   Check for and apply new releases
  rollback  Restore previous version via atomic symlink swap
  backup    Snapshot binary + database
  status    Show service status
  tailscale Install and manage tailscaled lifecycle
  install   Write an embedded deploy file (network|udev|wifi)
  version   Print version information

Run 'velocity device <command> --help' for command-specific usage.`

// Main is the entry point for the device applet. args is the argument slice
// after the namespace word (i.e. for `velocity device upgrade --check` it is
// ["upgrade", "--check"]). It returns the process exit code.
func Main(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, usage)
		return 1
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "check":
		if err := runCheck(rest); err != nil {
			fmt.Fprintf(os.Stderr, "check failed: %v\n", err)
			return 1
		}
	case "upgrade":
		if err := runUpgrade(rest); err != nil {
			fmt.Fprintf(os.Stderr, "upgrade failed: %v\n", err)
			return 1
		}
	case "rollback":
		if err := runRollback(rest); err != nil {
			fmt.Fprintf(os.Stderr, "rollback failed: %v\n", err)
			return 1
		}
	case "backup":
		if err := runBackup(rest); err != nil {
			fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
			return 1
		}
	case "status":
		if err := runStatus(rest); err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			return 1
		}
	case "tailscale":
		if err := runTailscale(rest); err != nil {
			fmt.Fprintf(os.Stderr, "tailscale: %v\n", err)
			return 1
		}
	case "install":
		if err := runInstall(rest); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			return 1
		}
	case "version":
		runVersion()
	case "--help", "-h", "help":
		fmt.Println(usage)
	case "--version", "-v":
		runVersion()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s\n", cmd, usage)
		return 1
	}

	return 0
}

func runVersion() {
	version.Print("velocity")
}
