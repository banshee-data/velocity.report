//go:build pcap

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func selectedSegmentJob(t *testing.T) (*Server, capjobs.Job, string) {
	t.Helper()
	ws, _ := segmentServer(t)
	listing := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following", nil, ws.handleSegments)
	if listing.Code != 200 {
		t.Fatal(listing.Body.String())
	}
	var ranked struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(listing.Body.Bytes(), &ranked); err != nil {
		t.Fatal(err)
	}
	chosen := ranked.Windows[0]
	created := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, ws.handleSegmentByID)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var selected struct {
		ReplayCaseID string `json:"replay_case_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	queued := callSegment(t, ws, "POST", "/api/lidar/scenes/"+selected.ReplayCaseID+"/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, selected.ReplayCaseID) })
	if queued.Code != 202 {
		t.Fatal(queued.Body.String())
	}
	var response struct {
		Job struct {
			ID string `json:"job_id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return ws, capjobs.Job{JobID: response.Job.ID}, selected.ReplayCaseID
}

func TestSegmentClipRejectsAnEditedReplayCase(t *testing.T) {
	ws, job, caseID := selectedSegmentJob(t)
	if _, err := ws.db.Exec(`UPDATE lidar_replay_cases SET pcap_start_secs=55 WHERE replay_case_id=?`, caseID); err != nil {
		t.Fatal(err)
	}
	err := ws.runSegmentClipJob(context.Background(), job, func(capjobs.Progress) {})
	if err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("edited case was accepted: %v", err)
	}
}

func segmentClipTestOperations(t *testing.T) segmentClipOperations {
	t.Helper()
	packDir := t.TempDir()
	return segmentClipOperations{
		mkdirAll:   os.MkdirAll,
		mkdirTemp:  os.MkdirTemp,
		detectPort: func(string) (int, error) { return 2369, nil },
		run: func(cfg replayeval.Config) (*replayeval.Result, error) {
			if !cfg.IncludePoints || !cfg.RequireSettled || cfg.Context == nil {
				t.Fatalf("clip replay lost point, settle or cancellation contract: %+v", cfg)
			}
			return &replayeval.Result{VRLOGPath: t.TempDir()}, nil
		},
		readFile: func(string) ([]byte, error) {
			return []byte(`{"scoring_start_ns":1788466680000000000,"scoring_duration_seconds":10}`), nil
		},
		export: func(cfg annotation.ExportConfig) (*annotation.Pack, error) {
			if cfg.EndNs <= cfg.StartNs || cfg.Coverage != annotation.CoverageForegroundOnly {
				t.Fatalf("invalid export bounds or coverage: %+v", cfg)
			}
			return &annotation.Pack{Dir: packDir, Manifest: annotation.Manifest{PackDigest: "sha256:" + strings.Repeat("a", 64)}}, nil
		},
		writeRecord: func(_ string, record segments.Record) error { return record.Validate() },
	}
}

func TestSegmentClipJobReportsEveryOutputBoundary(t *testing.T) {
	failure := errors.New("forced boundary failure")
	for _, tc := range []struct {
		name string
		edit func(*Server, *segmentClipOperations, context.CancelFunc)
	}{
		{"mkdir all", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.mkdirAll = func(string, os.FileMode) error { return failure }
		}},
		{"mkdir temp", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.mkdirTemp = func(string, string) (string, error) { return "", failure }
		}},
		{"port", func(ws *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ws.udpPort = 0
			ops.detectPort = func(string) (int, error) { return 0, failure }
		}},
		{"replay", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.run = func(replayeval.Config) (*replayeval.Result, error) { return nil, failure }
		}},
		{"cancel after replay", func(_ *Server, ops *segmentClipOperations, cancel context.CancelFunc) {
			base := ops.run
			ops.run = func(cfg replayeval.Config) (*replayeval.Result, error) {
				result, err := base(cfg)
				cancel()
				return result, err
			}
		}},
		{"manifest read", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.readFile = func(string) ([]byte, error) { return nil, failure }
		}},
		{"manifest JSON", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.readFile = func(string) ([]byte, error) { return []byte("{"), nil }
		}},
		{"manifest bounds", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.readFile = func(string) ([]byte, error) { return []byte(`{}`), nil }
		}},
		{"export", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.export = func(annotation.ExportConfig) (*annotation.Pack, error) { return nil, failure }
		}},
		{"cancel after export", func(_ *Server, ops *segmentClipOperations, cancel context.CancelFunc) {
			base := ops.export
			ops.export = func(cfg annotation.ExportConfig) (*annotation.Pack, error) {
				pack, err := base(cfg)
				cancel()
				return pack, err
			}
		}},
		{"record", func(_ *Server, ops *segmentClipOperations, _ context.CancelFunc) {
			ops.writeRecord = func(string, segments.Record) error { return failure }
		}},
		{"job update", func(ws *Server, _ *segmentClipOperations, _ context.CancelFunc) {
			if _, err := ws.db.Exec(`CREATE TRIGGER reject_clip_pack BEFORE UPDATE ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced job update failure'); END`); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, job, _ := selectedSegmentJob(t)
			ws.udpPort = 2369
			ops := segmentClipTestOperations(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tc.edit(ws, &ops, cancel)
			if err := ws.runSegmentClipJobWith(ctx, job, func(capjobs.Progress) {}, ops); err == nil {
				t.Fatal("failed clip boundary was reported as ready")
			}
		})
	}
}

func TestSegmentClipJobRejectsMissingOrCorruptSelectionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"missing job", `DELETE FROM lidar_segment_clip_jobs`},
		{"unreadable selection", `ALTER TABLE lidar_segment_selections RENAME COLUMN role TO broken_role`},
		{"bad parameters", `UPDATE lidar_segment_selections SET parameters_json='{'`},
		{"bad window", `UPDATE lidar_segment_selections SET window_json='{'`},
		{"missing case", `DROP TABLE lidar_replay_cases`},
		{"unreadable case files", `ALTER TABLE lidar_replay_case_files RENAME COLUMN pcap_file TO broken_path`},
		{"empty case files", `DELETE FROM lidar_replay_case_files; UPDATE lidar_replay_cases SET pcap_file=''`},
		{"unsafe case file", `UPDATE lidar_replay_case_files SET pcap_file='../outside.pcap'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, job, _ := selectedSegmentJob(t)
			if _, err := ws.db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			if err := ws.runSegmentClipJob(context.Background(), job, func(capjobs.Progress) {}); err == nil {
				t.Fatal("invalid clip selection was accepted")
			}
		})
	}
}

func TestSegmentClipJobChecksCancellationAndPassesCaseSequence(t *testing.T) {
	ws, job, _ := selectedSegmentJob(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ws.runSegmentClipJobWith(ctx, job, func(capjobs.Progress) {}, segmentClipTestOperations(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled before replay: %v", err)
	}
	if _, err := ws.db.Exec(`INSERT INTO lidar_replay_case_files(replay_case_id,ordinal,pcap_file) SELECT replay_case_id,1,pcap_file FROM lidar_replay_case_files WHERE ordinal=0`); err != nil {
		t.Fatal(err)
	}
	ws.udpPort = 2369
	ops := segmentClipTestOperations(t)
	base := ops.run
	ops.run = func(cfg replayeval.Config) (*replayeval.Result, error) {
		if len(cfg.PCAPFiles) != 2 || cfg.PCAPFile != "" {
			t.Fatalf("case sequence lost: %+v", cfg)
		}
		return base(cfg)
	}
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
		t.Fatal(err)
	}
}

func TestSegmentClipJobCutsKirk0Pack(t *testing.T) {
	if testing.Short() {
		t.Skip("real PCAP replay")
	}
	pcap, err := filepath.Abs("../perf/pcap/kirk0.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pcap); err != nil {
		t.Skip("kirk0 fixture unavailable")
	}
	extent, err := probeCaptureExtent(pcap, 2369)
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := segmentServer(t)
	ws.pcapSafeDir = filepath.Dir(pcap)
	start, duration := 40.0, 5.0
	scene := &sqlite.ReplayCase{SensorID: "kirk0-segments", PCAPFile: filepath.Base(pcap), PCAPStartSecs: &start, PCAPDurationSecs: &duration}
	if err := sqlite.NewReplayCaseStore(ws.db).InsertScene(scene); err != nil {
		t.Fatal(err)
	}
	params := segments.DefaultParams()
	chosen := segments.Window{Finder: "following", Version: segments.Version, Source: "run", Role: "tuning", StartNs: extent.FirstPacketNs + 40_000_000_000, EndNs: extent.FirstPacketNs + 45_000_000_000, PeakNs: extent.FirstPacketNs + 42_000_000_000, Capture: pcap, OffsetSeconds: 40}
	params.WindowSeconds = 5
	chosen.ID = segments.Identity(chosen.Finder, chosen.Source, chosen.Role, params, chosen.StartNs)
	parametersJSON, _ := json.Marshal(params)
	windowJSON, _ := json.Marshal(chosen)
	if _, err := ws.db.Exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,role,finder,parameters_json,window_json,created_at_ns) VALUES(?,?,?,?,?,?,?,1)`, chosen.ID, "run", scene.ReplayCaseID, chosen.Role, chosen.Finder, string(parametersJSON), string(windowJSON)); err != nil {
		t.Fatal(err)
	}
	queued := callSegment(t, ws, "POST", "/api/lidar/scenes/"+scene.ReplayCaseID+"/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, scene.ReplayCaseID) })
	if queued.Code != 202 {
		t.Fatal(queued.Body.String())
	}
	var response struct {
		Job struct {
			ID string `json:"job_id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	job := capjobs.Job{JobID: response.Job.ID}
	cancelContext, cancel := context.WithCancel(context.Background())
	err = ws.runSegmentClipJob(cancelContext, job, func(progress capjobs.Progress) {
		if progress.Detail == "replaying capture with points" {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("cancelled clip continued through replay")
	}
	if err := ws.runSegmentClipJob(context.Background(), job, func(capjobs.Progress) {}); err != nil {
		t.Fatal(err)
	}
	var packDir string
	if err := ws.db.QueryRow(`SELECT pack_dir FROM lidar_segment_clip_jobs WHERE job_id=?`, job.JobID).Scan(&packDir); err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Samples) == 0 {
		t.Fatal("clip job exported an empty pack")
	}
	b, err := os.ReadFile(filepath.Join(packDir, "segment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record segments.Record
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	if record.PackDigest != pack.Manifest.PackDigest || record.Segment.ID != chosen.ID {
		t.Fatalf("pack and selection are unbound: %+v", record)
	}
	// A worker retry must reuse the complete pack instead of creating another.
	if err := ws.runSegmentClipJob(context.Background(), job, func(capjobs.Progress) {}); err != nil {
		t.Fatal(err)
	}
}
