//go:build pcap

package lidar

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const laneFieldUsage = `velocity lidar lane-field — write a run's lane field for Gerardus

Usage:
  velocity lidar lane-field --db FILE --run RUN_ID --out DIR [--part PART] [--vrlog-dir DIR]

Writes the run's lane field (docs/plans/lidar-lane-proposals-from-tracks-plan.md
§1), from its stored track observations, or from its recording when it stored
none, as an analysis replay does: distinct tracks, box extents, eight heading bins and a speed sum per
0.25 m cell and class group, no cell under five distinct tracks. It writes
the whole run and its two halves, or the one --part, as
DIR/<run_id>.lane-field.<part>.json, byte for byte what
GET /api/lidar/runs/{run_id}/lane-field?part=<part> serves. Upload each with:

  gerardus upload-lane-field --json FILE --l16 TOKEN --record-id RECORD

The site token and the accepted registration record are left empty in the
file, since a run does not know them.

Flags:`

// LaneFieldMain runs `velocity lidar lane-field`.
func LaneFieldMain(args []string) int {
	return laneFieldMainWithOpen(args, sqlite.OpenReadOnly)
}

func laneFieldMainWithOpen(args []string, open func(string) (*sqlite.SQLDB, error)) int {
	fs := flag.NewFlagSet("velocity-lidar-lane-field", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), laneFieldUsage)
		fs.PrintDefaults()
	}
	dbPath := fs.String("db", "", "Database holding the run (read only)")
	runID := fs.String("run", "", "Analysis run ID")
	outDir := fs.String("out", "", "Directory to write the field files into")
	partFlag := fs.String("part", "", "Write only this part: all, first_half or second_half (default all three)")
	vrlogDir := fs.String("vrlog-dir", "", "Read the run's recording from this directory, by its stored name, instead of its stored path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *dbPath == "" || *runID == "" || *outDir == "" || fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "lane-field: --db, --run and --out are required")
		return 2
	}
	if strings.ContainsAny(*runID, `/\`) || *runID == "." || *runID == ".." {
		fmt.Fprintf(os.Stderr, "lane-field: run id %q cannot name a file\n", *runID)
		return 2
	}
	parts := l8analytics.LaneFieldParts
	if *partFlag != "" {
		p, err := l8analytics.ParseLaneFieldPart(*partFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 2
		}
		parts = []l8analytics.LaneFieldPart{p}
	}

	db, err := open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lane-field: open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()
	store := sqlite.NewRunStore(db)
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
		return 1
	}
	resolve := func(stored string) (string, error) {
		path := stored
		if *vrlogDir != "" {
			path = filepath.Join(*vrlogDir, filepath.Base(stored))
		}
		if _, err := os.Stat(filepath.Join(path, "header.json")); err != nil {
			return "", fmt.Errorf("no recording at %s", path)
		}
		return path, nil
	}
	for _, p := range parts {
		field, source, err := lanefieldrun.Build(store, *runID, p, resolve)
		if errors.Is(err, sqlite.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "lane-field: run %s not found\n", *runID)
			return 1
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 1
		}
		body, err := field.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 1
		}
		path := filepath.Join(*outDir, fmt.Sprintf("%s.lane-field.%s.json", *runID, p))
		if err := os.WriteFile(path, body, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 1
		}
		fmt.Printf("%s: %d cells, %d tracks, from the run's %s\n", path, field.CellCount(), field.Header.TrackCount, source)
	}
	return 0
}
