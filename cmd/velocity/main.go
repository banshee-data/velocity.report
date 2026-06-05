// Command velocity is the single multi-call binary for velocity.report.
//
// It dispatches on the program name (argv[0]) for the `velocity-report` and
// `velocity-ctl` compatibility aliases, and on the first argument for the
// canonical `velocity <namespace> ...` surface. Each namespace mounts an
// applet package under internal/cmd/.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/cmd/device"
	"github.com/banshee-data/velocity.report/internal/cmd/server"
	"github.com/banshee-data/velocity.report/internal/cmd/tune"
	"github.com/banshee-data/velocity.report/internal/version"
)

const topLevelUsage = `velocity — privacy-preserving traffic monitoring

Usage:
  velocity <namespace> [command] [flags]

Namespaces:
  serve     Run the radar/LiDAR server
  device    On-device lifecycle: check, upgrade, rollback, backup, status, tailscale
  data      Database operations: migrate, transits
  report    Generate PDF reports: pdf
  tune      Parameter tuning: sweep
  version   Print version information
  help      Show this help

Compatibility aliases:
  velocity-report   server-oriented alias (serve is the default)
  velocity-ctl      deprecated alias for 'velocity device ...' (removed next release)

Run 'velocity <namespace> --help' for namespace-specific usage.`

func main() {
	os.Exit(dispatch(filepath.Base(os.Args[0]), os.Args[1:]))
}

// dispatch routes by program name (prog) and the remaining args. It is split
// out from main so the routing can be unit-tested without spawning processes.
func dispatch(prog string, args []string) int {
	// argv[0] is matched by prefix so dev and release artefact names resolve to
	// the right alias: velocity-report-local, velocity-report-0.5.1-linux-arm64,
	// and the /usr/local/bin/velocity-report symlink all map to the server.
	// Order matters: velocity-ctl before velocity-report before the canonical
	// surface (all three share the "velocity" stem).
	switch {
	case strings.HasPrefix(prog, "velocity-ctl"):
		// Transitional deprecation shim into the device namespace. Removed
		// next release once image, sudoers, MOTD, and docs all speak the
		// new surface.
		fmt.Fprintln(os.Stderr, "warning: 'velocity-ctl' is deprecated and will be removed next release; use 'velocity device ...' instead")
		return device.Main(args)
	case strings.HasPrefix(prog, "velocity-report"):
		// Server-oriented compatibility alias. The server applet is the
		// default and preserves its own migrate/transits/pdf/version
		// subcommands for existing operator habits and the systemd unit.
		// Strip an optional leading "serve" so `velocity-report serve …`
		// is equivalent to `velocity-report …` and its flags still parse.
		if len(args) > 0 && args[0] == "serve" {
			args = args[1:]
		}
		return server.Main(args)
	}

	// Canonical surface: velocity <namespace> ...
	if len(args) == 0 {
		fmt.Println(topLevelUsage)
		return 0
	}

	switch args[0] {
	case "serve":
		return server.Main(args[1:])
	case "device":
		return device.Main(args[1:])
	case "data":
		// data migrate|transits — route by explicit subcommand into the
		// server applet, which already parses these by name. Never fall
		// through to a bare server start.
		if len(args) >= 2 && (args[1] == "migrate" || args[1] == "transits") {
			return server.Main(args[1:])
		}
		fmt.Fprintf(os.Stderr, "usage: velocity data <migrate|transits> ...\n")
		return 2
	case "report":
		if len(args) >= 2 && args[1] == "pdf" {
			return server.Main(args[1:])
		}
		fmt.Fprintf(os.Stderr, "usage: velocity report pdf ...\n")
		return 2
	case "tune":
		if len(args) >= 2 && args[1] == "sweep" {
			return tune.Main(args[2:])
		}
		fmt.Fprintf(os.Stderr, "usage: velocity tune sweep ...\n")
		return 2
	case "version", "--version", "-v":
		version.Print("velocity")
		return 0
	case "help", "--help", "-h":
		fmt.Println(topLevelUsage)
		return 0
	default:
		// Per the failure registry: print the canonical help and exit
		// non-zero rather than silently falling through to the server.
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s\n", args[0], topLevelUsage)
		return 2
	}
}
