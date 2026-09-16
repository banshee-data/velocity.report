// Command lidar-closeness-audit measures the L3 background closeness threshold
// against the background grids a deployment actually learned.
//
// This answers the part of gap HW1 that arithmetic alone cannot: the threshold
// is built from a measured per-cell spread plus a modelled noise term, and only
// real snapshots say how those two compare in the field. It reports, per range
// band, the measured spread distribution, the resulting threshold, how much of
// that threshold comes from the model rather than the measurement, and how it
// compares against the sensor's own flat range-accuracy specification.
//
// The threshold itself comes from l3grid.ClosenessThresholdMetres, the same
// expression the classification loop uses, so this cannot drift from the code
// it audits.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// rangeBand is one reporting bucket. The bands are narrow near the sensor,
// where most cells are, and widen with range where cells are sparse.
type rangeBand struct {
	lo, hi float64
	name   string
}

var bands = []rangeBand{
	{0, 10, "0-10 m"},
	{10, 20, "10-20 m"},
	{20, 30, "20-30 m"},
	{30, 50, "30-50 m"},
	{50, 75, "50-75 m"},
	{75, 100, "75-100 m"},
	{100, 200, "100-200 m"},
}

// bandStats accumulates the per-cell values for one band.
//
// Both acceptance windows are tracked because the foreground decision in
// foreground.go is an OR of three conditions, so the effective bar a real
// return must clear is the widest of them, not the closeness term alone.
type bandStats struct {
	spread      []float64
	threshold   []float64 // closeness window, warmup applied
	locked      []float64 // locked-baseline window, where the cell has one
	effective   []float64 // max of the two: the actual bar
	warmup      int       // cells still inside the warmup ramp
	lockedSet   int       // cells carrying a usable locked baseline
	lockedWider int       // cells where the locked window is the binding one
}

// specRangeAccuracyMetres is the Pandar40P's specified range accuracy: flat
// within each band, not proportional to range. Mirrors the constant the l3grid
// tests use; kept here so the tool is readable on its own.
func specRangeAccuracyMetres(rangeMetres float64) float64 {
	if rangeMetres <= 30 {
		return 0.02
	}
	return 0.03
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	idx := int(p / 100 * float64(len(sorted)-1))
	return sorted[idx]
}

// snapshotParams are the three tuning values the threshold is built from. The
// snapshot's own params blob is preferred, so an audit of an old capture uses
// the tuning that was live when it was taken; the shipped defaults fill in when
// the blob is empty, which is the case for every snapshot written so far.
type snapshotParams struct {
	multiplier       float64
	noiseRelative    float64
	safety           float64
	lockedMultiplier float64
	lockedThreshold  uint32
	fromSnapshot     bool
}

func shippedParams(tuningPath string) (snapshotParams, error) {
	cfg, err := config.LoadTuningConfig(tuningPath)
	if err != nil {
		return snapshotParams{}, fmt.Errorf("loading tuning config %s: %w", tuningPath, err)
	}
	if cfg.L3.EmaBaselineV1 == nil {
		return snapshotParams{}, fmt.Errorf("tuning config %s has no l3.ema_baseline_v1 block", tuningPath)
	}
	l3 := cfg.L3.EmaBaselineV1
	return snapshotParams{
		multiplier:       l3.ClosenessMultiplier,
		noiseRelative:    l3.NoiseRelative,
		safety:           l3.SafetyMarginMetres,
		lockedMultiplier: l3.LockedBaselineMultiplier,
		lockedThreshold:  uint32(l3.LockedBaselineThreshold),
	}, nil
}

func paramsFromSnapshot(paramsJSON string, shipped snapshotParams) snapshotParams {
	var raw map[string]any
	if err := json.Unmarshal([]byte(paramsJSON), &raw); err != nil || len(raw) == 0 {
		return shipped
	}
	number := func(keys ...string) (float64, bool) {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if f, ok := v.(float64); ok && f > 0 {
					return f, true
				}
			}
		}
		return 0, false
	}
	out := shipped
	found := false
	if v, ok := number("closeness_sensitivity_multiplier", "ClosenessSensitivityMultiplier"); ok {
		out.multiplier, found = v, true
	}
	if v, ok := number("noise_relative_fraction", "NoiseRelativeFraction"); ok {
		out.noiseRelative, found = v, true
	}
	if v, ok := number("safety_margin_metres", "SafetyMarginMetres"); ok {
		out.safety, found = v, true
	}
	out.fromSnapshot = found
	return out
}

func main() {
	dbPath := flag.String("db", "sensor_data.db", "SQLite database holding lidar_bg_snapshot rows")
	minTimesSeen := flag.Uint("min-times-seen", 10,
		"skip cells observed fewer times than this: an unsettled cell's spread is not yet meaningful")
	maxRange := flag.Float64("max-range", 200, "skip cells whose learned range exceeds this (metres)")
	tuningPath := flag.String("tuning", config.DefaultConfigPath,
		"tuning config supplying the closeness parameters when a snapshot carries none")
	sensorFilter := flag.String("sensor", "", "audit only this sensor_id (default: every sensor in the database)")
	maxSnapshots := flag.Int("max-snapshots", 1000, "most recent snapshots to read per sensor")
	flag.Parse()

	shipped, err := shippedParams(*tuningPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "run from the repository root, or pass -tuning\n")
		os.Exit(1)
	}

	// Read through internal/db rather than opening SQLite here: only the db
	// and storage packages may import database/sql.
	database, err := db.NewDBWithMigrationCheck(*dbPath, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer database.Close()

	sensorIDs, err := database.ListBgSnapshotSensorIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing sensors: %v\n", err)
		os.Exit(1)
	}
	if *sensorFilter != "" {
		sensorIDs = []string{*sensorFilter}
	}
	if len(sensorIDs) == 0 {
		fmt.Fprintf(os.Stderr, "no background snapshots in %s\n", *dbPath)
		os.Exit(1)
	}

	stats := map[string]*bandStats{}
	for _, b := range bands {
		stats[b.name] = &bandStats{}
	}

	var snapshots, decodeFailures, settledCells, skippedCells int
	sensors := map[string]int{}
	paramsUsed := map[string]int{}
	active := shipped

	for _, sensorID := range sensorIDs {
		snaps, err := database.ListRecentBgSnapshots(sensorID, *maxSnapshots)
		if err != nil {
			fmt.Fprintf(os.Stderr, "listing snapshots for %s: %v\n", sensorID, err)
			os.Exit(1)
		}

		for _, snap := range snaps {
			active = paramsFromSnapshot(snap.ParamsJSON, shipped)
			origin := "shipped defaults (snapshot carried no params)"
			if active.fromSnapshot {
				origin = "snapshot params"
			}
			paramsUsed[fmt.Sprintf("multiplier=%g noise_relative=%g safety=%g [%s]",
				active.multiplier, active.noiseRelative, active.safety, origin)]++

			cells, err := decodeCells(snap.GridBlob)
			if err != nil {
				decodeFailures++
				continue
			}
			snapshots++
			sensors[sensorID]++

			for _, c := range cells {
				learnedRange := float64(c.AverageRangeMeters)
				if uint(c.TimesSeenCount) < *minTimesSeen || learnedRange <= 0.5 || learnedRange > *maxRange {
					skippedCells++
					continue
				}
				settledCells++

				spread := float64(c.RangeSpreadMeters)

				// foreground.go widens the window by up to 4x while a cell is
				// still learning its variance, so an audit that ignores warmup
				// understates the bar for exactly the cells least able to
				// report anything.
				warmup := 1.0
				if c.TimesSeenCount < l3grid.WarmupSettledCount {
					warmup = l3grid.WarmupMultiplier(c.TimesSeenCount)
				}
				threshold := l3grid.ForegroundClosenessWindowMetres(
					active.multiplier, spread, active.noiseRelative, learnedRange, active.safety, warmup)

				// The locked-baseline window is an independent acceptance path,
				// live only once the cell has locked.
				locked, hasLocked := 0.0, false
				if c.LockedBaseline > 0 && c.LockedAtCount >= active.lockedThreshold {
					locked = l3grid.LockedBaselineWindowMetres(
						active.lockedMultiplier, float64(c.LockedSpread), active.noiseRelative,
						learnedRange, active.safety)
					hasLocked = true
				}

				effective := threshold
				if hasLocked && locked > effective {
					effective = locked
				}

				for _, b := range bands {
					if learnedRange >= b.lo && learnedRange < b.hi {
						s := stats[b.name]
						s.spread = append(s.spread, spread)
						s.threshold = append(s.threshold, threshold)
						s.effective = append(s.effective, effective)
						if warmup > 1.0 {
							s.warmup++
						}
						if hasLocked {
							s.locked = append(s.locked, locked)
							s.lockedSet++
							if locked > threshold {
								s.lockedWider++
							}
						}
						break
					}
				}
			}
		}
	}

	report(snapshots, decodeFailures, settledCells, skippedCells, sensors, paramsUsed, stats, active)
}

// decodeCells unpacks a snapshot's grid blob: gzip around a gob-encoded slice.
func decodeCells(blob []byte) ([]l3grid.BackgroundCell, error) {
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()

	var cells []l3grid.BackgroundCell
	if err := gob.NewDecoder(gz).Decode(&cells); err != nil {
		return nil, fmt.Errorf("gob decode: %w", err)
	}
	return cells, nil
}

func report(snapshots, decodeFailures, settledCells, skippedCells int,
	sensors, paramsUsed map[string]int, stats map[string]*bandStats, active snapshotParams) {

	fmt.Printf("Closeness threshold audit (gap HW1)\n")
	fmt.Printf("  snapshots decoded : %d", snapshots)
	if decodeFailures > 0 {
		fmt.Printf(" (%d failed to decode)", decodeFailures)
	}
	fmt.Printf("\n  settled cells     : %d (%d skipped as unsettled or out of range)\n",
		settledCells, skippedCells)
	for id, n := range sensors {
		fmt.Printf("  sensor            : %s (%d snapshots)\n", id, n)
	}
	for p, n := range paramsUsed {
		fmt.Printf("  params            : %s x%d\n", p, n)
	}

	if settledCells == 0 {
		fmt.Printf("\nNo settled cells found: nothing to report.\n")
		return
	}

	fmt.Printf("\nMeasured spread and resulting acceptance window, by range band.\n")
	fmt.Printf("\"eff\" is the wider of the closeness and locked-baseline windows:\n")
	fmt.Printf("the foreground decision ORs them, so it is the real bar.\n\n")
	fmt.Printf("  %-11s %10s %8s %8s %8s %8s %8s %8s %8s\n",
		"band", "cells", "spr p50", "spr p90", "thr p50", "thr p90", "eff p50", "eff p90", "spec thr")
	for _, b := range bands {
		s := stats[b.name]
		if len(s.spread) == 0 {
			continue
		}
		sort.Float64s(s.spread)
		sort.Float64s(s.threshold)
		sort.Float64s(s.effective)
		mid := (b.lo + b.hi) / 2
		specThreshold := l3grid.ClosenessThresholdMetres(
			active.multiplier, percentile(s.spread, 50), 0, mid, active.safety) +
			active.multiplier*specRangeAccuracyMetres(mid)

		fmt.Printf("  %-11s %10d %8.3f %8.3f %8.3f %8.3f %8.3f %8.3f %8.3f\n",
			b.name, len(s.spread),
			percentile(s.spread, 50), percentile(s.spread, 90),
			percentile(s.threshold, 50), percentile(s.threshold, 90),
			percentile(s.effective, 50), percentile(s.effective, 90),
			specThreshold)
	}

	fmt.Printf("\nThe other two acceptance paths, as a share of cells in each band:\n\n")
	fmt.Printf("  %-11s %12s %12s %14s %12s\n",
		"band", "in warmup", "locked", "locked wider", "locked p50")
	for _, b := range bands {
		s := stats[b.name]
		n := len(s.spread)
		if n == 0 {
			continue
		}
		lockedP50 := math.NaN()
		if len(s.locked) > 0 {
			sort.Float64s(s.locked)
			lockedP50 = percentile(s.locked, 50)
		}
		fmt.Printf("  %-11s %11.1f%% %11.1f%% %13.1f%% %10.3f m\n",
			b.name,
			100*float64(s.warmup)/float64(n),
			100*float64(s.lockedSet)/float64(n),
			100*float64(s.lockedWider)/float64(n),
			lockedP50)
	}

	// This table fixes the range at the band midpoint so the shares are
	// comparable across bands; the threshold column is therefore the threshold
	// at that midpoint, not the percentile of per-cell thresholds above.
	fmt.Printf("\nWhere the threshold comes from, at each band's median spread\n")
	fmt.Printf("and the band's midpoint range:\n\n")
	fmt.Printf("  %-11s %12s %12s %14s %12s\n",
		"band", "model", "measured", "vs spec model", "thr @ mid")
	for _, b := range bands {
		s := stats[b.name]
		if len(s.spread) == 0 {
			continue
		}
		mid := (b.lo + b.hi) / 2
		medianSpread := percentile(s.spread, 50)
		modelTerm := active.noiseRelative * mid
		// The bracketed sum the multiplier scales; the safety margin sits
		// outside it and is not attributable to either term.
		inner := medianSpread + modelTerm + 0.01
		threshold := l3grid.ClosenessThresholdMetres(
			active.multiplier, medianSpread, active.noiseRelative, mid, active.safety)
		specThreshold := l3grid.ClosenessThresholdMetres(
			active.multiplier, medianSpread, 0, mid, active.safety) +
			active.multiplier*specRangeAccuracyMetres(mid)

		fmt.Printf("  %-11s %11.1f%% %11.1f%% %13.1fx %11.2f m\n",
			b.name, 100*modelTerm/inner, 100*medianSpread/inner,
			threshold/specThreshold, threshold)
	}

	fmt.Printf("\nRead this as: the modelled noise term supplies most of the window in\n")
	fmt.Printf("which an observation counts as background, while the measured spread —\n")
	fmt.Printf("the only term carrying real information about how variable each cell\n")
	fmt.Printf("actually is — supplies very little. A threshold is the minimum range\n")
	fmt.Printf("separation a foreground return needs to register at all, so a wide one\n")
	fmt.Printf("costs detections rather than adding false ones.\n")
}
