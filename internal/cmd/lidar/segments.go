//go:build pcap

package lidar

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "modernc.org/sqlite"
)

func SegmentsMain(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-segments", flag.ContinueOnError)
	dbPath := fs.String("db", "", "Evidence database (read only)")
	run := fs.String("run", "", "Server analysis run ID; otherwise use versioned estimates")
	source := fs.String("source", "", "Estimate source ID")
	stage := fs.String("stage", "online", "Estimate stage")
	finder := fs.String("finder", "following", "following, leader_changes, lateral_jump, split_flags, exposure, random")
	role := fs.String("role", "tuning", "tuning or held_out")
	top := fs.Int("top", 15, "Maximum windows to print (0 = all)")
	p := segments.DefaultParams()
	fs.Float64Var(&p.WindowSeconds, "window-seconds", p.WindowSeconds, "Window width")
	fs.Float64Var(&p.MinSpeed, "min-speed", p.MinSpeed, "Minimum speed in m/s")
	fs.Float64Var(&p.MaxHeadingDeg, "max-heading-deg", p.MaxHeadingDeg, "Maximum heading difference")
	fs.Float64Var(&p.MinGap, "min-gap", p.MinGap, "Minimum following gap")
	fs.Float64Var(&p.MaxGap, "max-gap", p.MaxGap, "Maximum following gap")
	fs.Float64Var(&p.MaxLateral, "max-lateral", p.MaxLateral, "Maximum lateral separation")
	fs.Float64Var(&p.JumpThreshold, "jump-threshold", p.JumpThreshold, "Lateral residual threshold")
	fs.Float64Var(&p.JumpMaxGapSeconds, "jump-max-gap", p.JumpMaxGapSeconds, "Maximum gap within five-point fit")
	fs.Int64Var(&p.RandomSeed, "seed", p.RandomSeed, "Reproducible random seed")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *dbPath == "" || *top < 0 || (*run != "" && *source != "") {
		fmt.Fprintln(os.Stderr, "segments: --db is required; --run and --source are alternatives; --top must be nonnegative")
		return 2
	}
	db, err := sqlite.OpenReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	var pts []segments.Point
	var src string
	if *run != "" {
		src = *run
		pts, err = segments.LoadRun(db, *run)
	} else {
		pts, src, err = segments.LoadEstimates(db, *source, *stage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "segments: %v\n", err)
		return 1
	}
	var captures []segments.Capture
	if len(pts) > 0 {
		lo, hi := pts[0].TimeNs, pts[0].TimeNs
		for _, pt := range pts {
			if pt.TimeNs < lo {
				lo = pt.TimeNs
			}
			if pt.TimeNs > hi {
				hi = pt.TimeNs
			}
		}
		captures, err = segments.CapturesForRange(db, lo, hi)
		if err != nil {
			fmt.Fprintf(os.Stderr, "segments: capture index: %v\n", err)
			return 1
		}
	}
	windows, err := segments.Find(pts, *finder, src, *role, p, captures)
	if err != nil {
		fmt.Fprintf(os.Stderr, "segments: %v\n", err)
		return 2
	}
	if *top > 0 && len(windows) > *top {
		windows = windows[:*top]
	}
	report := struct {
		Schema     string            `json:"schema"`
		Source     string            `json:"source"`
		Finder     string            `json:"finder"`
		Version    int               `json:"version"`
		Role       string            `json:"role"`
		Parameters segments.Params   `json:"parameters"`
		Windows    []segments.Window `json:"windows"`
	}{"velocity.report/segments", src, *finder, segments.Version, *role, p, windows}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
