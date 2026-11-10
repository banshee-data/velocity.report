// Command lidar-gest1-pool pools the per-case G-EST-1 track records that
// lidar-state-estimation-baseline -gest1-report writes (gest1_tracks.json.gz)
// and reads l5est0007's restated criterion 2, the strata, the jerk and
// turning checks and criteria 3 to 5 over the corpus: pooled, per case, and
// with each case left out.
//
//	lidar-gest1-pool -out pooled.json case1/first/gest1_tracks.json.gz ...
//
// An argument LABEL=PATH reads PATH under the case name LABEL, so two
// segments of one site (each recorded under the site's case) pool as two
// cases: haight-divisadero-s2=runs/.../gest1_tracks.json.gz.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// leaveOneOut is the pooled read's deciding figures without one case.
type leaveOneOut struct {
	Case           string  `json:"case_left_out"`
	Tracks         int     `json:"tracks"`
	AlongRMS       float64 `json:"along_one_step_rms"`
	AlongBeyond    float64 `json:"along_one_step_share_beyond_1_96"`
	OverDispersed  bool    `json:"along_over_dispersed"`
	AcrossRMS      float64 `json:"across_one_step_rms"`
	R              float64 `json:"z_along_sq_accel_r"`
	BootP05        float64 `json:"z_along_sq_accel_bootstrap_p05"`
	CorrelationErr string  `json:"z_along_sq_accel_error,omitempty"`
	// The age split (l5est0008): one-step along z RMS at most and after
	// the boundary, and the after-boundary correlation's 5th percentile.
	YoungAlongRMS  float64 `json:"young_along_one_step_rms"`
	MatureAlongRMS float64 `json:"mature_along_one_step_rms"`
	MatureR        float64 `json:"mature_z_along_sq_accel_r"`
	MatureBootP05  float64 `json:"mature_z_along_sq_accel_bootstrap_p05"`
}

type caseRead struct {
	Case string                      `json:"case"`
	Read l8analytics.GEST1CorpusRead `json:"read"`
}

type sourcePool struct {
	Source      string                      `json:"source"`
	Pooled      l8analytics.GEST1CorpusRead `json:"pooled"`
	PerCase     []caseRead                  `json:"per_case"`
	LeaveOneOut []leaveOneOut               `json:"leave_one_out"`
}

type pool struct {
	Cases              []string     `json:"cases"`
	TurnRateThresholds []float64    `json:"turn_rate_thresholds_rad_s"`
	Sources            []sourcePool `json:"sources"`
}

func main() {
	out := flag.String("out", "", "pooled JSON to write (required)")
	draws := flag.Int("bootstrap-draws", l8analytics.DefaultGEST1CorpusConfig().BootstrapDraws, "whole-track resamples behind every interval")
	level := flag.Float64("level", l8analytics.DefaultGEST1CorpusConfig().Level, "two-sided coverage of every bootstrap interval (0.9 gives the 5th to 95th)")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: lidar-gest1-pool -out pooled.json gest1_tracks.json.gz ...")
		os.Exit(2)
	}
	cfg := l8analytics.DefaultGEST1CorpusConfig()
	cfg.BootstrapDraws = *draws
	if !(*level > 0 && *level < 1) {
		fmt.Fprintln(os.Stderr, "lidar-gest1-pool: -level must lie strictly between 0 and 1")
		os.Exit(2)
	}
	cfg.Level = *level
	p, err := run(flag.Args(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lidar-gest1-pool:", err)
		os.Exit(1)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err == nil {
		err = os.WriteFile(*out, append(b, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lidar-gest1-pool:", err)
		os.Exit(1)
	}
	for _, s := range p.Sources {
		fmt.Printf("%s cases=%d %s\n", s.Source, len(s.PerCase), s.Pooled.String())
	}
}

// run loads every record and reads each source pooled, per case and with
// each case left out. Records must name distinct cases and share the turning
// thresholds their counts were made at.
func run(paths []string, cfg l8analytics.GEST1CorpusConfig) (pool, error) {
	var p pool
	bySource := map[string]map[string][]l8analytics.GEST1CorpusTrack{}
	var sources []string
	seen := map[string]bool{}
	for i, arg := range paths {
		label, path := "", arg
		if k := strings.Index(arg, "="); k > 0 {
			label, path = arg[:k], arg[k+1:]
		}
		rec, err := l8analytics.LoadGEST1CorpusRecord(path)
		if err != nil {
			return p, err
		}
		if label != "" {
			rec.Case = label
		}
		if rec.Case == "" || seen[rec.Case] {
			return p, fmt.Errorf("%s: case %q is empty or repeated", path, rec.Case)
		}
		seen[rec.Case] = true
		p.Cases = append(p.Cases, rec.Case)
		if i == 0 {
			p.TurnRateThresholds = rec.Config.TurnRateThresholds
		} else if !reflect.DeepEqual(p.TurnRateThresholds, rec.Config.TurnRateThresholds) {
			return p, fmt.Errorf("%s: turning thresholds %v differ from %v", path, rec.Config.TurnRateThresholds, p.TurnRateThresholds)
		}
		for _, s := range rec.Sources {
			if bySource[s.Source] == nil {
				bySource[s.Source] = map[string][]l8analytics.GEST1CorpusTrack{}
				sources = append(sources, s.Source)
			}
			bySource[s.Source][rec.Case] = append(bySource[s.Source][rec.Case], s.Tracks...)
		}
	}
	sort.Strings(p.Cases)
	for _, src := range sources {
		cases := bySource[src]
		sp := sourcePool{Source: src}
		var all []l8analytics.GEST1CorpusTrack
		for _, c := range p.Cases {
			all = append(all, cases[c]...)
		}
		sp.Pooled = l8analytics.ReadGEST1Corpus(all, p.TurnRateThresholds, cfg)
		for _, c := range p.Cases {
			if _, ok := cases[c]; !ok {
				continue
			}
			sp.PerCase = append(sp.PerCase, caseRead{Case: c, Read: l8analytics.ReadGEST1Corpus(cases[c], p.TurnRateThresholds, cfg)})
			var rest []l8analytics.GEST1CorpusTrack
			for _, o := range p.Cases {
				if o != c {
					rest = append(rest, cases[o]...)
				}
			}
			r := l8analytics.ReadGEST1Corpus(rest, p.TurnRateThresholds, cfg)
			sp.LeaveOneOut = append(sp.LeaveOneOut, leaveOneOut{Case: c, Tracks: r.Tracks,
				AlongRMS: r.Along.RMS, AlongBeyond: r.Along.Beyond, OverDispersed: r.Along.OverDispersed,
				AcrossRMS: r.Across.RMS, R: r.Correlation.R.R, BootP05: r.Correlation.BootP05,
				CorrelationErr: r.Correlation.Err,
				YoungAlongRMS:  r.AgeSplit.Young.Along.RMS, MatureAlongRMS: r.AgeSplit.Mature.Along.RMS,
				MatureR: r.AgeSplit.Mature.Correlation.R.R, MatureBootP05: r.AgeSplit.Mature.Correlation.BootP05})
		}
		p.Sources = append(p.Sources, sp)
	}
	return p, nil
}
