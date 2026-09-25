// Package root implements the top-level routing for the single velocity binary.
package root

import (
	"fmt"
	"os"
	"strings"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/cmd/device"
	jobscmd "github.com/banshee-data/velocity.report/internal/cmd/jobs"
	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
	scenecmd "github.com/banshee-data/velocity.report/internal/cmd/scene"
	"github.com/banshee-data/velocity.report/internal/cmd/server"
	"github.com/banshee-data/velocity.report/internal/cmd/tune"
	"github.com/banshee-data/velocity.report/internal/cmd/worker"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/version"
)

func init() {
	// Every subcommand funnels through Dispatch, so this is the one place
	// that has to run once: MustLoadDefaultConfig's relative-path search
	// only ever resolves from inside the repository tree (or from a Go
	// test's own working directory), and a compiled binary run as a
	// background service is neither. Without this, a worker daemon started
	// from any other directory panics the moment a job needs the default
	// tuning document.
	config.SetEmbeddedDefaults(radarassets.TuningDefaults)
}

var (
	serverMain   = server.Main
	deviceMain   = device.Main
	lidarMain    = lidar.Main
	tuneMain     = tune.Main
	sceneMain    = scenecmd.Main
	workerMain   = worker.Main
	jobsMain     = jobscmd.Main
	printVersion = version.Print
)

const topLevelUsage = `velocity — privacy-preserving traffic monitoring

Usage:
  velocity <namespace> [command] [flags]

Namespaces:
  serve     Run the radar/LiDAR server
  device    On-device lifecycle: check, upgrade, rollback, backup, status, tailscale
  lidar     LiDAR capture diagnostics: pcap-split, settling-eval
  data      Database operations: migrate, transits, sql
  report    Generate PDF reports: pdf, headway
  scene     Export a recorded VRLOG as static web assets: export
  tune      Parameter tuning: sweep
  worker    Run analysis jobs submitted to this host's API, one at a time
  jobs      Submit, watch and fetch jobs on a worker: status, submit, list, show, log, fetch, wait
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
	case "lidar":
		return lidarMain(args[1:])
	case "data":
		if len(args) >= 2 && (args[1] == "migrate" || args[1] == "transits" || args[1] == "sql") {
			return serverMain(args[1:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity data <migrate|transits|sql> ...")
		return 2
	case "report":
		if len(args) >= 2 && (args[1] == "pdf" || args[1] == "headway") {
			return serverMain(args[1:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity report <pdf|headway> ...")
		return 2
	case "scene":
		return sceneMain(args[1:])
	case "tune":
		if len(args) >= 2 && args[1] == "sweep" {
			return tuneMain(args[2:])
		}
		fmt.Fprintln(os.Stderr, "usage: velocity tune sweep ...")
		return 2
	case "worker":
		return workerMain(args[1:])
	case "jobs":
		return jobsMain(args[1:])
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
