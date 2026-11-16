package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/banshee-data/velocity.report/internal/version"
)

// The real recorder, exporter, validator and slice writer surround synthetic
// analytic trajectories. Only PCAP perception and the SQL estimate reader are
// substituted; their engines have their own replay and field-run suites.
func TestPreparationPublishesVerifiablePairedReviewOrNoCompletion(t *testing.T) {
	old := version.GitSHA
	version.GitSHA = "synthetic-producer"
	t.Cleanup(func() { version.GitSHA = old })
	sc := l8behaviour.EncounterScenarios()[0]
	tr := sc.Trajectories[0]
	for _, failure := range []string{"", "replay", "source", "analysis", "slices", "export", "header", "database", "evidence", "slice-header"} {
		name := failure
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			capture, sha := fx.Capture(t, dir, "review.pcap", "synthetic packets")
			manifest := fx.Manifest(t, dir, map[string]any{"sites": []any{fx.Site(tr.Passage.SiteID, "validation")}, "captures": []any{fx.CaptureEntry("capture", tr.Passage.SiteID, sha, "review.pcap")}})
			ledger := filepath.Join(dir, "ledger.db")
			database, err := db.NewDB(ledger)
			if err != nil {
				t.Fatal(err)
			}
			database.Close()
			empty := ""
			guard := &eligibilitycli.CLIFlags{Manifest: &manifest, AnnotationRoot: &empty, LedgerDB: &ledger, Actor: &empty, Measures: &empty}
			cal := l4bobserve.Calibration{SensorID: tr.Passage.SensorID, FromFrame: "sensor", ToFrame: "sensor", Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
			calBytes, _ := json.Marshal(cal)
			calPath := filepath.Join(dir, "calibration.json")
			os.WriteFile(calPath, calBytes, 0644)
			tuning := filepath.Join(dir, "tuning.json")
			os.WriteFile(tuning, []byte("{}"), 0644)
			o := options{Capture: capture, Output: filepath.Join(dir, "review"), Site: tr.Passage.SiteID, Clip: "clip", Binding: "explicit-parent", Tuning: tuning, Calibration: calPath, Warmup: 20, Port: 2369}
			replay := func(cfg replayeval.Config) (*replayeval.Result, error) {
				if !cfg.ForegroundOnly || !cfg.RequireSettled || !cfg.IncludePoints || cfg.StartSeconds != cfg.WarmupSeconds {
					t.Fatal("review replay lost its payload or settling contract")
				}
				if failure == "replay" {
					return nil, errors.New("failed replay")
				}
				d, e := db.NewDB(cfg.ObservationDBPath)
				if e != nil {
					return nil, e
				}
				d.Close()
				rec, e := recorder.NewRecorder(cfg.OutDir, cfg.SensorID)
				if e != nil {
					return nil, e
				}
				rec.SetDeterministicConfig("config", "parameters", "config-sha", "parameters-sha", "replay", "replay", "test", version.GitSHA, nil)
				for i, s := range tr.Samples {
					if failure == "header" {
						break
					}
					f := l9endpoints.NewFrameBundle(uint64(i), cfg.SensorID, time.Unix(0, s.CaptureUnixNanos))
					f.FrameType = l9endpoints.FrameTypeForeground
					if e = rec.Record(f); e != nil {
						return nil, e
					}
				}
				if e = rec.Close(); e != nil {
					return nil, e
				}
				window, e := cfg.ObservationWindow()
				if e != nil {
					return nil, e
				}
				source, e := l4bobserve.SourceID(l4bobserve.CaptureSource{ClipID: cfg.ClipID, CapturePaths: []string{capture}, CaptureSHA256s: []string{strings.TrimPrefix(sha, "sha256:")}, ExtractorID: "l4.dbscan_xy/v1/parameters-sha", Window: &window})
				if e != nil {
					return nil, e
				}
				if failure == "source" {
					source = "wrong-source"
				}
				if failure == "slices" {
					os.Mkdir(filepath.Join(o.Output, "review-slices"), 0755)
				}
				if failure == "export" {
					os.WriteFile(filepath.Join(o.Output, "canonical-following-evidence.json"), []byte("preserve"), 0644)
				}
				if failure == "database" {
					os.Remove(cfg.ObservationDBPath)
				}
				return &replayeval.Result{VRLOGPath: cfg.OutDir, FramesRecorded: len(tr.Samples), ObservationSourceID: source}, nil
			}
			analyse := func(database sqlite.DBClient, s fieldrun.Spec) (fieldrun.Result, error) {
				var cache int
				if err := database.QueryRow("PRAGMA cache_size").Scan(&cache); err != nil || cache != -65536 {
					t.Fatalf("analysis cache: %d, %v", cache, err)
				}
				if failure == "database" {
					return fieldrun.Analyse(database, s)
				}
				if failure == "analysis" {
					return fieldrun.Result{}, errors.New("failed analysis")
				}
				if failure == "slice-header" {
					os.Remove(filepath.Join(o.Output, "vrlog", "header.json"))
				}
				if failure == "evidence" {
					return fieldrun.Result{}, nil
				}
				return fieldrun.Result{Trajectories: sc.Trajectories, Params: sc.Params,
					Estimates: sqlite.EstimateVersion{SourceID: s.SourceID, EstimatorID: tr.Estimate.EstimatorID, ObservationModelID: tr.Estimate.ObsModelID, ParamHash: tr.Estimate.ParamHash, Stage: tr.Samples[0].Stage.String()},
					Version:   l8behaviour.InteractionVersion{EstimatorID: tr.Estimate.EstimatorID, ObsModelID: tr.Estimate.ObsModelID, ParamHash: tr.Estimate.ParamHash, EstimateStage: tr.Samples[0].Stage, MethodID: l8behaviour.FollowingEncounterMethodID + "/" + sc.Params.Hash()}}, nil
			}
			err = prepareWith(o, guard, replay, analyse)
			complete := filepath.Join(o.Output, "complete.json")
			if failure != "" {
				if err == nil {
					t.Fatal("failed preparation succeeded")
				}
				if _, e := os.Stat(complete); !os.IsNotExist(e) {
					t.Fatal("failed preparation claimed completion")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, e := os.ReadFile(complete)
			if e != nil {
				t.Fatal(e)
			}
			var receipt struct {
				Evidence string      `json:"evidence"`
				Slices   []sliceFile `json:"slices"`
			}
			if e = json.Unmarshal(b, &receipt); e != nil {
				t.Fatal(e)
			}
			b, e = os.ReadFile(receipt.Evidence)
			if e != nil {
				t.Fatal(e)
			}
			var canonical fieldrun.EvidenceDocument
			if e = json.Unmarshal(b, &canonical); e != nil {
				t.Fatal(e)
			}
			if e = canonical.Validate(); e != nil {
				t.Fatal(e)
			}
			rows := 0
			for _, s := range receipt.Slices {
				sum, e := eligibility.FileSHA256(s.Path)
				if e != nil || sum != s.SHA256 {
					t.Fatal("slice digest mismatch")
				}
				rows += s.Rows
			}
			if rows != len(canonical.Rows) || rows == 0 {
				t.Fatal("native files do not cover canonical rows")
			}
		})
	}
}
