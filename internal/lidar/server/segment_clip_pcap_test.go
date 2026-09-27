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

const clipTestDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func segmentClipTestOperations(t *testing.T) segmentClipOperations {
	t.Helper()
	return segmentClipOperations{
		mkdirAll:   os.MkdirAll,
		mkdirTemp:  os.MkdirTemp,
		removeAll:  os.RemoveAll,
		readDir:    os.ReadDir,
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
			// The pack is written where the job asked, inside its attempt.
			return &annotation.Pack{Dir: cfg.OutDir, Manifest: annotation.Manifest{PackDigest: clipTestDigest}}, nil
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
			// A failed attempt holds a recording with points and no job row
			// names it, so it must not stay on disk.
			if left := clipAttempts(t, ws); len(left) != 0 {
				t.Fatalf("failed clip left its output behind: %v", left)
			}
		})
	}
}

// clipAttempts lists what clip jobs have left in the packs directory.
func clipAttempts(t *testing.T, ws *Server) []string {
	t.Helper()
	entries, err := os.ReadDir(ws.annotationPacksDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestSegmentClipJobRejectsMissingOrCorruptSelectionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"missing job", `DELETE FROM lidar_segment_clip_jobs`},
		{"unreadable selection", `ALTER TABLE lidar_segment_selections RENAME COLUMN role TO broken_role`},
		// The database refuses a document that is not JSON, so what is left
		// to refuse here is one that is JSON and not what the worker reads.
		{"parameters of the wrong shape", `UPDATE lidar_segment_selections SET parameters_json='{"window_seconds":"ten"}'`},
		{"window of the wrong shape", `UPDATE lidar_segment_selections SET window_json=json_set(window_json,'$.score','high')`},
		{"missing case", `DROP TABLE lidar_replay_cases`},
		{"unreadable case", `ALTER TABLE lidar_replay_cases RENAME COLUMN description TO broken_description`},
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
	if kept := clipAttempts(t, ws); len(kept) != 1 || !strings.HasPrefix(kept[0], "clip-"+job.JobID+"-") {
		t.Fatalf("completed clip output was not kept: %v", kept)
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
	if err := sqlite.NewSegmentStore(ws.db).InsertSelection(chosen.ID, "run", scene.ReplayCaseID, parametersJSON, windowJSON); err != nil {
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
	clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	// The row names the pack from the packs directory, so that it is still
	// found when that directory is moved.
	if filepath.IsAbs(clip.PackDir) || !strings.HasPrefix(clip.PackDir, "clip-"+job.JobID+"-") || !strings.HasSuffix(clip.PackDir, "/pack") {
		t.Fatalf("stored pack directory: %q", clip.PackDir)
	}
	packDir := ws.segmentPackPath(clip.PackDir)
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
	if record.PackDigest != pack.Manifest.PackDigest || record.Segment.ID != chosen.ID || clip.PackDigest != pack.Manifest.PackDigest {
		t.Fatalf("pack, selection and job row are unbound: %+v, row digest %q", record, clip.PackDigest)
	}
	// A worker retry must reuse the complete pack instead of creating another.
	if err := ws.runSegmentClipJob(context.Background(), job, func(capjobs.Progress) {}); err != nil {
		t.Fatal(err)
	}
	// The cancelled attempt's recording is gone and the retry made no second one.
	if kept := clipAttempts(t, ws); len(kept) != 1 || filepath.Join(ws.annotationPacksDir, kept[0], "pack") != packDir {
		t.Fatalf("want only the completed attempt, holding %s: %v", packDir, kept)
	}
}

func TestCaptureWorkerRunsClipJobs(t *testing.T) {
	ws, job, _ := selectedSegmentJob(t)
	job.Kind = "vrlog_record"
	// The fixture capture is not a recording, so the replay refuses it. What
	// matters here is that the worker hands the job to the clip executor.
	err := ws.runCaptureJob(context.Background(), job, func(capjobs.Progress) {})
	if err == nil || strings.Contains(err.Error(), "unknown capture job kind") {
		t.Fatalf("clip job was not dispatched to its executor: %v", err)
	}
	if left := clipAttempts(t, ws); len(left) != 0 {
		t.Fatalf("refused clip left its output behind: %v", left)
	}
}

// neverReplays fails the test if the job goes on to replay the capture.
func neverReplays(t *testing.T, ops *segmentClipOperations) {
	t.Helper()
	ops.run = func(replayeval.Config) (*replayeval.Result, error) {
		t.Fatal("the capture was replayed although its pack already exists")
		return nil, nil
	}
}

func TestClipRetryAdoptsThePackAnEarlierAttemptFinished(t *testing.T) {
	ws, job, _ := selectedSegmentJob(t)
	ws.udpPort = 2369
	store := sqlite.NewSegmentStore(ws.db)
	clip, _, err := store.ClipJob(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	// The process stopped after writing the pack and before recording it.
	// A second attempt got as far as a directory and no further.
	pack, digest := writeClipAttempt(t, ws, job.JobID, clip.SegmentID, "finished")
	unfinished := filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(job.JobID)+"unfinished")
	if err := os.MkdirAll(filepath.Join(unfinished, "vrlog"), 0755); err != nil {
		t.Fatal(err)
	}
	another := filepath.Join(ws.annotationPacksDir, clipAttemptPrefix("job-another")+"1")
	if err := os.MkdirAll(another, 0755); err != nil {
		t.Fatal(err)
	}
	ops := segmentClipTestOperations(t)
	neverReplays(t, &ops)
	var last capjobs.Progress
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(p capjobs.Progress) { last = p }, ops); err != nil {
		t.Fatal(err)
	}
	clip, _, err = store.ClipJob(job.JobID)
	if err != nil || ws.segmentPackPath(clip.PackDir) != pack || clip.PackDigest != digest {
		t.Fatalf("adopted pack: %+v %v, want %s %s", clip, err, pack, digest)
	}
	if last.Current != 3 || last.Total != 3 || !strings.Contains(last.Detail, pack) {
		t.Fatalf("progress after adoption: %+v", last)
	}
	if _, err := os.Stat(unfinished); !os.IsNotExist(err) {
		t.Fatalf("the job's unfinished attempt was left behind: %v", err)
	}
	if _, err := os.Stat(another); err != nil {
		t.Fatalf("another job's attempt was removed: %v", err)
	}
	// With the pack recorded, a further retry has nothing to read or cut.
	ops.readDir = func(string) ([]os.DirEntry, error) {
		t.Fatal("a recorded pack was looked for again")
		return nil, nil
	}
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
		t.Fatal(err)
	}
}

// recordedClip is a job whose pack is written and recorded.
func recordedClip(t *testing.T) (ws *Server, job capjobs.Job, pack, stored, digest string) {
	t.Helper()
	ws, job, _ = selectedSegmentJob(t)
	ws.udpPort = 2369
	store := sqlite.NewSegmentStore(ws.db)
	clip, _, err := store.ClipJob(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	pack, digest = writeClipAttempt(t, ws, job.JobID, clip.SegmentID, "recorded")
	if stored, err = ws.storedPackDir(pack); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkPack(job.JobID, stored, digest); err != nil {
		t.Fatal(err)
	}
	return ws, job, pack, stored, digest
}

// Before a retry adopted what an earlier attempt had finished, it cut the pack
// again, so a job can have two whole packs on disk and a person's review in
// either. A retry links one and removes neither.
func TestClipRetryNeverRemovesAWholePack(t *testing.T) {
	ws, job, _ := selectedSegmentJob(t)
	ws.udpPort = 2369
	store := sqlite.NewSegmentStore(ws.db)
	clip, _, err := store.ClipJob(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := writeClipAttempt(t, ws, job.JobID, clip.SegmentID, "1")
	reviewed, digest := writeClipAttempt(t, ws, job.JobID, clip.SegmentID, "2")
	review := filepath.Join(reviewed, "annotations.json")
	if err := os.WriteFile(review, []byte(`{"objects":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	unfinished := filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(job.JobID)+"3")
	if err := os.MkdirAll(unfinished, 0755); err != nil {
		t.Fatal(err)
	}
	ops := segmentClipTestOperations(t)
	neverReplays(t, &ops)
	removed := []string{}
	ops.removeAll = func(dir string) error {
		removed = append(removed, dir)
		return os.RemoveAll(dir)
	}
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != unfinished {
		t.Fatalf("removed %v, want only the attempt that left no pack", removed)
	}
	for _, kept := range []string{filepath.Join(plain, "manifest.json"), filepath.Join(reviewed, "manifest.json"), review} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("a whole pack or its review was removed: %v", err)
		}
	}
	clip, _, err = store.ClipJob(job.JobID)
	if err != nil || ws.segmentPackPath(clip.PackDir) != reviewed || clip.PackDigest != digest {
		t.Fatalf("linked pack: %+v %v, want the reviewed one %s", clip, err, reviewed)
	}
}

func TestClipRetryCutsAgainWhenItsRecordedPackIsGone(t *testing.T) {
	ws, job, pack, stored, _ := recordedClip(t)
	if err := os.RemoveAll(filepath.Dir(pack)); err != nil {
		t.Fatal(err)
	}
	replayed := false
	ops := segmentClipTestOperations(t)
	base := ops.run
	ops.run = func(cfg replayeval.Config) (*replayeval.Result, error) {
		replayed = true
		return base(cfg)
	}
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
		t.Fatal(err)
	}
	if !replayed {
		t.Fatal("a recorded pack that is no longer on disk was trusted")
	}
	clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(job.JobID)
	if err != nil || clip.PackDigest != clipTestDigest || clip.PackDir == stored || clip.PackDir == "" {
		t.Fatalf("row after cutting again: %+v %v", clip, err)
	}
}

// The row is a note of what is on disk. When the two disagree the pack on
// disk is the evidence, and the row is corrected without cutting it again.
func TestClipRetryCorrectsARowThatNamesAnotherDigest(t *testing.T) {
	ws, job, pack, stored, digest := recordedClip(t)
	if _, err := ws.db.Exec(`UPDATE lidar_segment_clip_jobs SET pack_digest=? WHERE job_id=?`, "sha256:"+strings.Repeat("c", 64), job.JobID); err != nil {
		t.Fatal(err)
	}
	ops := segmentClipTestOperations(t)
	neverReplays(t, &ops)
	if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
		t.Fatal(err)
	}
	clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(job.JobID)
	if err != nil || clip.PackDigest != digest || clip.PackDir != stored {
		t.Fatalf("row after the retry: %+v %v, want %s %s", clip, err, stored, digest)
	}
	if _, err := os.Stat(pack); err != nil {
		t.Fatalf("the pack on disk was disturbed: %v", err)
	}
}

func TestClipRecoveryReportsWhatItCouldNotDo(t *testing.T) {
	failure := errors.New("forced recovery failure")
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, ws *Server, ops *segmentClipOperations, jobID, segmentID string)
	}{
		{"packs directory unreadable", func(_ *testing.T, _ *Server, ops *segmentClipOperations, _, _ string) {
			ops.readDir = func(string) ([]os.DirEntry, error) { return nil, failure }
		}},
		{"unfinished attempt cannot be removed", func(t *testing.T, ws *Server, ops *segmentClipOperations, jobID, _ string) {
			if err := os.MkdirAll(filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(jobID)+"half"), 0755); err != nil {
				t.Fatal(err)
			}
			ops.removeAll = func(string) error { return failure }
		}},
		{"pack cannot be linked", func(t *testing.T, ws *Server, _ *segmentClipOperations, jobID, segmentID string) {
			writeClipAttempt(t, ws, jobID, segmentID, "finished")
			if _, err := ws.db.Exec(`CREATE TRIGGER refuse_pack_link BEFORE UPDATE ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced link failure'); END`); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, job, _ := selectedSegmentJob(t)
			ws.udpPort = 2369
			clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(job.JobID)
			if err != nil {
				t.Fatal(err)
			}
			ops := segmentClipTestOperations(t)
			neverReplays(t, &ops)
			tc.edit(t, ws, &ops, job.JobID, clip.SegmentID)
			err = ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops)
			if err == nil || !strings.Contains(err.Error(), "recover an earlier attempt") {
				t.Fatalf("recovery failure: %v", err)
			}
		})
	}
	t.Run("pack written outside the packs directory", func(t *testing.T) {
		ws, job, _ := selectedSegmentJob(t)
		ws.udpPort = 2369
		ops := segmentClipTestOperations(t)
		elsewhere := t.TempDir()
		ops.export = func(annotation.ExportConfig) (*annotation.Pack, error) {
			return &annotation.Pack{Dir: elsewhere, Manifest: annotation.Manifest{PackDigest: clipTestDigest}}, nil
		}
		err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops)
		if err == nil || !strings.Contains(err.Error(), "not inside the annotation packs directory") {
			t.Fatalf("pack outside the directory: %v", err)
		}
		if left := clipAttempts(t, ws); len(left) != 0 {
			t.Fatalf("refused clip left its output behind: %v", left)
		}
	})
	t.Run("packs directory does not exist yet", func(t *testing.T) {
		ws, job, _ := selectedSegmentJob(t)
		ws.udpPort = 2369
		ws.annotationPacksDir = filepath.Join(ws.annotationPacksDir, "made-by-the-job")
		ops := segmentClipTestOperations(t)
		// The first clip on a machine makes the directory it then looks in.
		if err := ws.runSegmentClipJobWith(context.Background(), job, func(capjobs.Progress) {}, ops); err != nil {
			t.Fatal(err)
		}
		if kept := clipAttempts(t, ws); len(kept) != 1 {
			t.Fatalf("first clip: %v", kept)
		}
	})
}
