// Command lidar-region-map reads the L3 region map a replay persisted into its
// evidence database (lidar-state-estimation-baseline -evidence-dir) and writes
// one row per background-grid cell: its ring, azimuth bin, beam elevation and
// bin-centre azimuth, its region and that region's variance tercile and
// override parameters, the cell's own tercile and settling variance, its
// settled range, and its position in the sensor frame.
//
//	lidar-region-map -db EV/observations.db -list
//	lidar-region-map -db EV/observations.db -out regions.csv
//	lidar-region-map -db EV/observations.db -reason replay_end -source kirk0 -out end.json
//
// Without -region-set it takes the one region set whose grid snapshot has
// -reason (by default the map as identified, which the overrides read) and,
// when given, whose source path contains -source; more or fewer than one is
// an error that lists the candidates. The output format follows the -out
// extension: .json writes the map with its provenance, anything else CSV.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "lidar-region-map:", err)
		os.Exit(1)
	}
}

// regionMapFile is the JSON output: the map and where it came from.
type regionMapFile struct {
	Database         string `json:"database"`
	RegionSetID      int64  `json:"region_set_id"`
	SnapshotID       int64  `json:"snapshot_id"`
	SnapshotReason   string `json:"snapshot_reason"`
	SensorID         string `json:"sensor_id"`
	SourcePath       string `json:"source_path"`
	CreatedUnixNanos int64  `json:"created_unix_nanos"`
	*l3grid.RegionMap
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("lidar-region-map", flag.ContinueOnError)
	dbPath := fs.String("db", "", "evidence database a replay wrote (observations.db or <case>.db)")
	list := fs.Bool("list", false, "list the region sets in the database and exit")
	regionSet := fs.Int64("region-set", 0, "region_set_id to dump; 0 selects by -reason and -source")
	reason := fs.String("reason", l3grid.SnapshotReasonRegionsIdentified, "snapshot reason to select: "+
		l3grid.SnapshotReasonRegionsIdentified+" (the map as identified) or "+l3grid.SnapshotReasonReplayEnd)
	source := fs.String("source", "", "select only region sets whose source path contains this text")
	outPath := fs.String("out", "", "output file; .json for JSON, anything else for CSV")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("-db is required")
	}
	if !*list && *outPath == "" {
		return fmt.Errorf("-out is required unless -list is set")
	}
	// db.OpenDB would create an empty database at a mistyped path, whose
	// empty tables would read as a replay that persisted no map.
	if _, err := os.Stat(*dbPath); err != nil {
		return fmt.Errorf("evidence database: %w", err)
	}
	database, err := db.OpenDB(*dbPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", *dbPath, err)
	}
	defer database.Close()

	sets, err := database.ListRegionSnapshots()
	if err != nil {
		return fmt.Errorf("list region sets: %w", err)
	}
	if *list {
		writeList(stdout, sets)
		return nil
	}
	chosen, err := selectRegionSet(sets, *regionSet, *reason, *source)
	if err != nil {
		return err
	}
	regions, err := database.GetRegionSnapshotByID(chosen.RegionSetID)
	if err != nil {
		return fmt.Errorf("read region set %d: %w", chosen.RegionSetID, err)
	}
	if regions == nil {
		return fmt.Errorf("region set %d not found", chosen.RegionSetID)
	}
	grid, err := database.GetBgSnapshotByID(regions.SnapshotID)
	if err != nil {
		return fmt.Errorf("read grid snapshot %d: %w", regions.SnapshotID, err)
	}
	if grid == nil {
		return fmt.Errorf("region set %d links grid snapshot %d, which is not in the database", chosen.RegionSetID, regions.SnapshotID)
	}
	m, err := l3grid.BuildRegionMap(grid, regions)
	if err != nil {
		return fmt.Errorf("region set %d: %w", chosen.RegionSetID, err)
	}
	file := regionMapFile{
		Database: *dbPath, RegionSetID: chosen.RegionSetID, SnapshotID: regions.SnapshotID,
		SnapshotReason: grid.SnapshotReason, SensorID: regions.SensorID, SourcePath: regions.SourcePath,
		CreatedUnixNanos: regions.CreatedUnixNanos, RegionMap: m,
	}
	if err := writeOutput(*outPath, file); err != nil {
		return err
	}
	writeSummary(stdout, file)
	return nil
}

// selectRegionSet picks the region set to dump: the one named, or the only
// one matching reason and source.
func selectRegionSet(sets []db.RegionSnapshotSummary, id int64, reason, source string) (db.RegionSnapshotSummary, error) {
	if id != 0 {
		for _, s := range sets {
			if s.RegionSetID == id {
				return s, nil
			}
		}
		return db.RegionSnapshotSummary{}, fmt.Errorf("no region set %d in the database", id)
	}
	var matches []db.RegionSnapshotSummary
	for _, s := range sets {
		if s.SnapshotReason == reason && strings.Contains(s.SourcePath, source) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	var b strings.Builder
	writeList(&b, sets)
	return db.RegionSnapshotSummary{}, fmt.Errorf("%d region sets have reason %q and a source path containing %q; name one with -region-set:\n%s",
		len(matches), reason, source, b.String())
}

func writeList(w io.Writer, sets []db.RegionSnapshotSummary) {
	if len(sets) == 0 {
		fmt.Fprintln(w, "no region sets")
		return
	}
	for _, s := range sets {
		fmt.Fprintf(w, "region_set=%d snapshot=%d reason=%s regions=%d settling_frames=%d created_unix_nanos=%d source=%q\n",
			s.RegionSetID, s.SnapshotID, s.SnapshotReason, s.RegionCount, s.SettlingFrames, s.CreatedUnixNanos,
			strings.ReplaceAll(s.SourcePath, "\n", ";"))
	}
}

func writeOutput(path string, file regionMapFile) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		err = enc.Encode(file)
	} else {
		err = writeCSV(f, file.RegionMap)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

var csvHeader = []string{
	"cell", "ring", "azimuth_bin", "elevation_deg", "azimuth_deg", "region_id", "region_tercile",
	"region_volatile", "region_noise_relative_fraction", "region_neighbour_confirmation", "cell_tercile",
	"settling_variance", "settled_range_m", "locked_baseline_m", "range_spread_m", "times_seen",
	"x_m", "y_m", "z_m",
}

// writeCSV writes one row per cell, in cell-index order, with the columns
// named as in the JSON.
func writeCSV(w io.Writer, m *l3grid.RegionMap) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	// Grid fields are float32; print them at their own precision.
	f32 := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	for _, c := range m.Cells {
		if err := cw.Write([]string{
			strconv.Itoa(c.Cell), strconv.Itoa(c.Ring), strconv.Itoa(c.AzimuthBin), f(c.ElevationDeg), f(c.AzimuthDeg),
			strconv.Itoa(c.RegionID), c.RegionTercile, strconv.FormatBool(c.RegionVolatile),
			f32(c.RegionNoiseRelativeFraction), strconv.Itoa(c.RegionNeighbourConfirmation), c.CellTercile,
			f(c.SettlingVariance), f32(c.SettledRangeM), f32(c.LockedBaselineM), f32(c.RangeSpreadM),
			strconv.FormatUint(uint64(c.TimesSeen), 10), f(c.XM), f(c.YM), f(c.ZM),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// writeSummary prints the counts a reader checks first: regions, cells per
// tercile by the cell's own spread and by its region, and the volatile share.
func writeSummary(w io.Writer, file regionMapFile) {
	m := file.RegionMap
	seen, volatileCells := 0, 0
	cellTerciles, regionTerciles := map[string]int{}, map[string]int{}
	volatileRegions := map[int]bool{}
	for _, c := range m.Cells {
		if c.TimesSeen > 0 {
			seen++
		}
		cellTerciles[c.CellTercile]++
		regionTerciles[c.RegionTercile]++
		if c.RegionVolatile {
			volatileCells++
			volatileRegions[c.RegionID] = true
		}
	}
	fmt.Fprintf(w, "region_set=%d snapshot=%d reason=%s source=%q\n", file.RegionSetID, file.SnapshotID,
		file.SnapshotReason, strings.ReplaceAll(file.SourcePath, "\n", ";"))
	fmt.Fprintf(w, "grid=%dx%d cells=%d seen=%d regions=%d volatile_regions=%d settling_frames=%d tercile_thresholds=%v\n",
		m.Rings, m.AzimuthBins, len(m.Cells), seen, m.RegionCount, len(volatileRegions), m.SettlingFrames, m.TercileThresholds)
	fmt.Fprintf(w, "cells by own tercile: %s\n", formatCounts(cellTerciles))
	fmt.Fprintf(w, "cells by region tercile: %s (volatile cells=%d)\n", formatCounts(regionTerciles), volatileCells)
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		name := k
		if name == "" {
			name = "unrecorded"
		}
		parts[i] = fmt.Sprintf("%s=%d", name, counts[k])
	}
	return strings.Join(parts, " ")
}
