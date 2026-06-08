// Package root implements the top-level routing for the single velocity binary.
package root

import (
	"fmt"
	"os"
	"strings"

	"github.com/banshee-data/velocity.report/internal/cmd/device"
	"github.com/banshee-data/velocity.report/internal/cmd/server"
	"github.com/banshee-data/velocity.report/internal/cmd/tune"
	"github.com/banshee-data/velocity.report/internal/version"
)

var (
	serverMain   = server.Main
	deviceMain   = device.Main
	tuneMain     = tune.Main
	printVersion = version.Print
)

const topLevelUsage = `velocity — privacy-preserving traffic monitoring

Usage:
  velocity <namespace> [command] [flags]

Namespaces:
  serve     Run the radar/LiDAR server
  device    On-device lifecycle: check, upgrade, rollback, backup, status, tailscale
  data      Database operations: migrate, transits, sql
  report    Generate PDF reports: pdf
  tune      Parameter tuning: sweep
  version   Print version information
  help      Show this help

Compatibility alias:
  velocity-report   server-oriented alias (serve is the default)

Run 'velocity help' for this overview.`

// Dispatch routes by program name (prog) and the remaining args. prog is
// matched by prefix for the velocity-report compatibility alias so suffixed dev
// and release artifact names still resolve to the server-oriented surface.
func Dispatch(prog string, args []string) int {
	switch {
	case strings.HasPrefix(prog, "velocity-report"):
		if len(args) > 0 && args[0] == "serve" {
			args = args[1:]
		}
		return serverMain(args)
	}

	if len(args) == 0 {
		fmt.Println(topLevelUsage)
		return 0
	}

	switch args[0] {
	case "serve":
		return serverMain(args[1:])
	case "device":
		return deviceMain(args[1:])
	case "data":
		if len(args) >= 2 && (args[1] == "migrate" || args[1] == "transits" || args[1] == "sql") {
			return serverMain(args[1:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity data <migrate|transits|sql> ...")
		return 2
	case "report":
		if len(args) >= 2 && args[1] == "pdf" {
			return serverMain(args[1:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity report pdf ...")
		return 2
	case "tune":
		if len(args) >= 2 && args[1] == "sweep" {
			return tuneMain(args[2:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity tune sweep ...")
		return 2
	case "version", "--version", "-v":
		printVersion("velocity")
		return 0
	case "help", "--help", "-h":
		fmt.Println(topLevelUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s\n", args[0], topLevelUsage)
		return 2
	}
}
