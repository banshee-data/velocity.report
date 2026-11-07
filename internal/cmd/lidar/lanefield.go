//go:build pcap

package lidar

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/gerardus"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const laneFieldUsage = `velocity lidar lane-field — write a run's lane field for Gerardus

Usage:
  velocity lidar lane-field --db FILE --run RUN_ID --out DIR [--part PART] [--vrlog-dir DIR]
  velocity lidar lane-field --db FILE --run RUN_ID --upload --l16 TOKEN [--record-id RECORD] [--gerardus URL] [--out DIR]

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

With --upload the parts go straight to Gerardus over gRPC (UploadLaneField),
placed by --record-id or, without it, by the accepted record of the run's
sensor at --l16 whose capture names the run's source, as the lane proposals
job chooses (docs/plans/lidar-lane-field-batch-plan.md). It needs no access
to Gerardus's data directory. --out is then optional.

Flags:`

// LaneFieldMain runs `velocity lidar lane-field`.
func LaneFieldMain(args []string) int {
	return laneFieldMainWith(args, sqlite.OpenReadOnly, dialLaneFieldUploader)
}

func laneFieldMainWithOpen(args []string, open func(string) (*sqlite.SQLDB, error)) int {
	return laneFieldMainWith(args, open, dialLaneFieldUploader)
}

// laneFieldUploader is what --upload needs from Gerardus.
type laneFieldUploader interface {
	Records(ctx context.Context, l16 string) ([]gerardus.Record, error)
	UploadLaneField(ctx context.Context, f l8analytics.LaneField) (gerardus.Upload, error)
	Close() error
}

func dialLaneFieldUploader(address string) (laneFieldUploader, error) { return gerardus.Dial(address) }

func laneFieldMainWith(args []string, open func(string) (*sqlite.SQLDB, error), dial func(string) (laneFieldUploader, error)) int {
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
	upload := fs.Bool("upload", false, "Upload each part to Gerardus over gRPC instead of (or as well as) writing it")
	l16 := fs.String("l16", "", "With --upload: the site's S2 level-16 token")
	recordID := fs.String("record-id", "", "With --upload: the accepted registration record that places the field (default: chosen from the site's records)")
	gerardusURL := fs.String("gerardus", "", "With --upload: the Gerardus server (default $GERARDUS_URL or "+gerardus.DefaultAddress+")")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *dbPath == "" || *runID == "" || (*outDir == "" && !*upload) || fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "lane-field: --db, --run and --out (or --upload) are required")
		return 2
	}
	if *upload && *l16 == "" {
		fmt.Fprintln(os.Stderr, "lane-field: --upload needs --l16")
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
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 1
		}
	}
	var up laneFieldUploader
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if *upload {
		run, err := store.GetRun(*runID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: run %s: %v\n", *runID, err)
			return 1
		}
		address := gerardus.Address(*gerardusURL)
		if up, err = dial(address); err != nil {
			fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
			return 1
		}
		defer up.Close()
		if *recordID == "" {
			records, err := up.Records(ctx, *l16)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
				return 1
			}
			reg := gerardus.AcceptedRecord(records, run.SensorID, []string{filepath.Base(run.SourcePath)})
			if !reg.Placed() {
				fmt.Fprintf(os.Stderr, "lane-field: no record places run %s at %s (%s): %s; name one with --record-id\n", *runID, *l16, reg.State, reg.Reason)
				return 1
			}
			*recordID = reg.RecordID
		}
		fmt.Printf("uploading to %s, placed by %s\n", address, *recordID)
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
		if *outDir != "" {
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
		if up != nil {
			field.Header.L16Token, field.Header.RecordID = *l16, *recordID
			res, err := up.UploadLaneField(ctx, field)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lane-field: %v\n", err)
				return 1
			}
			stored := "stored"
			if res.AlreadyStored {
				stored = "already stored"
			}
			fmt.Printf("%s: %s, %d cells, %s\n", p, res.FieldID, res.Cells, stored)
		}
	}
	return 0
}
