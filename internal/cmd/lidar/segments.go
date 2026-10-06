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
	selectorsPath := fs.String("selectors", "", "Selector file (default "+segments.DefaultSelectorsPath+", else the copy built into the binary)")
	selectorID := fs.String("selector", "", "Selector to rank with, from --list-selectors (default following)")
	finder := fs.String("finder", "", "A finder's standard selector, instead of --selector: following, leader_changes, lateral_jump, split_flags, exposure, random")
	listSelectors := fs.Bool("list-selectors", false, "Print the selectors and exit")
	role := fs.String("role", "tuning", "tuning or held_out")
	top := fs.Int("top", 15, "Maximum windows to print (0 = all)")
	// Each parameter flag, when given, replaces the selector's own value.
	given := segments.DefaultParams()
	fs.Float64Var(&given.WindowSeconds, "window-seconds", given.WindowSeconds, "Window width")
	fs.Float64Var(&given.MinSpeed, "min-speed", given.MinSpeed, "Minimum speed in m/s")
	fs.Float64Var(&given.MaxHeadingDeg, "max-heading-deg", given.MaxHeadingDeg, "Maximum heading difference")
	fs.Float64Var(&given.MinGap, "min-gap", given.MinGap, "Minimum following gap")
	fs.Float64Var(&given.MaxGap, "max-gap", given.MaxGap, "Maximum following gap")
	fs.Float64Var(&given.MaxLateral, "max-lateral", given.MaxLateral, "Maximum lateral separation")
	fs.Float64Var(&given.JumpThreshold, "jump-threshold", given.JumpThreshold, "Lateral residual threshold")
	fs.Float64Var(&given.JumpMaxGapSeconds, "jump-max-gap", given.JumpMaxGapSeconds, "Maximum gap within five-point fit")
	fs.Int64Var(&given.RandomSeed, "seed", given.RandomSeed, "Reproducible random seed")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	catalogue, err := loadSelectorCatalogue(*selectorsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "segments: %v\n", err)
		return 2
	}
	if *listSelectors {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"version": catalogue.Version, "digest": catalogue.Digest, "source": catalogue.Source, "selectors": catalogue.Listing()}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if *dbPath == "" || *top < 0 || (*run != "" && *source != "") || (*selectorID != "" && *finder != "") {
		fmt.Fprintln(os.Stderr, "segments: --db is required; --run and --source are alternatives, as are --selector and --finder; --top must be nonnegative")
		return 2
	}
	id := *selectorID
	if id == "" {
		id = *finder
	}
	if id == "" {
		id = "following"
	}
	sel, ok := catalogue.Selector(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "segments: unknown selector %q (see --list-selectors)\n", id)
		return 2
	}
	overridden := false
	fs.Visit(func(f *flag.Flag) {
		if apply, ok := parameterFlags[f.Name]; ok {
			apply(&sel.Parameters, given)
			overridden = true
		}
	})
	// A caller's parameters could steer even a traffic measure towards
	// tracker failure, so a held-out window takes none.
	if overridden && *role == "held_out" {
		fmt.Fprintln(os.Stderr, "segments: a held-out window is chosen at its selector's own parameters; drop the parameter flags")
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
	if sel.Finder == "random" || *capture != "" {
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
	windows, err := segments.Rank(pts, sel, src, *role, captures)
	if err != nil {
		fmt.Fprintf(os.Stderr, "segments: %v\n", err)
		return 2
	}
	if *top > 0 && len(windows) > *top {
		windows = windows[:*top]
	}
	report := struct {
		Schema     string                      `json:"schema"`
		Source     string                      `json:"source"`
		Finder     string                      `json:"finder"`
		Version    int                         `json:"version"`
		Role       string                      `json:"role"`
		Parameters segments.Params             `json:"parameters"`
		Selector   segments.SelectorProvenance `json:"selector"`
		Windows    []segments.Window           `json:"windows"`
	}{"velocity.report/segments", src, sel.Finder, segments.Version, *role, sel.Parameters, sel.Provenance(), windows}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// parameterFlags copies one parameter flag's value onto a selector's
// parameters.
var parameterFlags = map[string]func(p *segments.Params, given segments.Params){
	"window-seconds":  func(p *segments.Params, given segments.Params) { p.WindowSeconds = given.WindowSeconds },
	"min-speed":       func(p *segments.Params, given segments.Params) { p.MinSpeed = given.MinSpeed },
	"max-heading-deg": func(p *segments.Params, given segments.Params) { p.MaxHeadingDeg = given.MaxHeadingDeg },
	"min-gap":         func(p *segments.Params, given segments.Params) { p.MinGap = given.MinGap },
	"max-gap":         func(p *segments.Params, given segments.Params) { p.MaxGap = given.MaxGap },
	"max-lateral":     func(p *segments.Params, given segments.Params) { p.MaxLateral = given.MaxLateral },
	"jump-threshold":  func(p *segments.Params, given segments.Params) { p.JumpThreshold = given.JumpThreshold },
	"jump-max-gap":    func(p *segments.Params, given segments.Params) { p.JumpMaxGapSeconds = given.JumpMaxGapSeconds },
	"seed":            func(p *segments.Params, given segments.Params) { p.RandomSeed = given.RandomSeed },
}

// loadSelectorCatalogue reads the selector file named, or the default one.
func loadSelectorCatalogue(path string) (*segments.Catalogue, error) {
	if path == "" {
		return segments.DefaultCatalogue()
	}
	return segments.LoadSelectors(path)
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
