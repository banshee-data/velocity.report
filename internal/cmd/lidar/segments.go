//go:build pcap

package lidar

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "modernc.org/sqlite"
)

func SegmentsMain(args []string) int {
	return segmentsMainWithOpen(args, sqlite.OpenReadOnly)
}

func segmentsMainWithOpen(args []string, open func(string) (*sqlite.SQLDB, error)) int {
	fs := flag.NewFlagSet("velocity-lidar-segments", flag.ContinueOnError)
	dbPath := fs.String("db", "", "Evidence database (read only)")
	run := fs.String("run", "", "Server analysis run ID; otherwise use versioned estimates")
	source := fs.String("source", "", "Estimate source ID")
	stage := fs.String("stage", "online", "Estimate stage")
	capture := fs.String("capture", "", "Capture path for placement (required for random evidence selection)")
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
	db, err := open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	var pts []segments.Point
	var src string
	if *run != "" {
		src = *run
		pts, err = loadRunSeries(db, *run)
	} else {
		pts, src, err = segments.LoadEstimates(db, *source, *stage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "segments: %v\n", err)
		return 1
	}
	var captures []segments.Capture
	if *finder == "random" || *capture != "" {
		path := *capture
		if path == "" && *run != "" {
			record, recordErr := sqlite.NewAnalysisRunStore(db).GetRun(*run)
			if recordErr != nil {
				fmt.Fprintf(os.Stderr, "segments: run: %v\n", recordErr)
				return 1
			}
			path = record.SourcePath
		}
		if !filepath.IsAbs(path) {
			fmt.Fprintln(os.Stderr, "segments: --capture needs an absolute path for random selection")
			return 2
		}
		indexed, indexErr := segments.CapturesForRange(db, 0, 1<<63-1)
		if indexErr == nil {
			for _, indexedCapture := range indexed {
				if filepath.Clean(indexedCapture.Path) == filepath.Clean(path) {
					captures = append(captures, indexedCapture)
				}
			}
		}
		if len(captures) == 0 {
			port, portErr := network.DetectUDPPort(path)
			if portErr != nil {
				fmt.Fprintf(os.Stderr, "segments: capture probe: %v\n", portErr)
				return 1
			}
			extent, probeErr := network.CountPCAPPackets(path, port)
			if probeErr != nil || extent.Count == 0 || extent.LastTimestampNs <= extent.FirstTimestampNs {
				fmt.Fprintf(os.Stderr, "segments: capture %q has no usable packet-time extent: %v\n", path, probeErr)
				return 1
			}
			captures = []segments.Capture{{Path: path, FirstNs: extent.FirstTimestampNs, LastNs: extent.LastTimestampNs}}
		}
	} else if len(pts) > 0 {
		captures, err = segments.CapturesForRange(db, pts[0].TimeNs, pts[len(pts)-1].TimeNs)
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

// loadRunSeries reads a run's stored observations, or its recording when it
// stored none: an analysis replay keeps its tracks in the recording only.
func loadRunSeries(db *sqlite.SQLDB, runID string) ([]segments.Point, error) {
	pts, err := segments.LoadRun(db, runID)
	if err != nil || len(pts) > 0 {
		return pts, err
	}
	record, err := sqlite.NewAnalysisRunStore(db).GetRun(runID)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	if record.VRLogPath == "" {
		return pts, nil
	}
	pts, err = segments.LoadRunRecording(db, runID, record.VRLogPath)
	if err != nil {
		return nil, fmt.Errorf("run has no stored observations and its recording could not be read: %w", err)
	}
	return pts, nil
}
