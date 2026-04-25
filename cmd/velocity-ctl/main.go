// Package main implements velocity-ctl, the on-device management tool
// for velocity.report.
//
// Runs as root on the Raspberry Pi. Manages upgrades, rollbacks, backups,
// and service status for the velocity-report server.
//
// Subcommands:
//
//	upgrade   Check for and apply new releases from GitHub
//	rollback  Restore the previous version from a timestamped backup
//	backup    Create a manual snapshot of binary + database
//	status    Show systemd service status
//	version   Print installed version information
package main

import (
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/version"
)

const usage = `velocity-ctl — on-device management for velocity.report

Usage:
  velocity-ctl <command> [flags]

Commands:
  upgrade   Check for and apply new releases
  rollback  Restore previous version from backup
  backup    Snapshot binary + database
  status    Show service status
  tailscale Manage tailscaled lifecycle (enable/disable)
  version   Print version information

Run 'velocity-ctl <command> --help' for command-specific usage.`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "upgrade":
		if err := runUpgrade(args); err != nil {
			fmt.Fprintf(os.Stderr, "upgrade failed: %v\n", err)
			os.Exit(1)
		}
	case "rollback":
		if err := runRollback(args); err != nil {
			fmt.Fprintf(os.Stderr, "rollback failed: %v\n", err)
			os.Exit(1)
		}
	case "backup":
		if err := runBackup(args); err != nil {
			fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := runStatus(args); err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			os.Exit(1)
		}
	case "tailscale":
		if err := runTailscale(args); err != nil {
			fmt.Fprintf(os.Stderr, "tailscale: %v\n", err)
			os.Exit(1)
		}
	case "version":
		runVersion()
	case "--help", "-h", "help":
		fmt.Println(usage)
	case "--version", "-v":
		runVersion()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s\n", cmd, usage)
		os.Exit(1)
	}
}

func runVersion() {
	version.Print("velocity-ctl")
}
