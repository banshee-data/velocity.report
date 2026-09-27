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
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// queuedClip chooses the fixture's following window and queues its clip.
func queuedClip(t *testing.T, ws *Server) (jobID, segmentID string) {
	t.Helper()
	chosen := firstFollowingSegment(t, ws, "tuning")
	created := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run"}, ws.handleSegmentByID)
	var selected struct {
		ReplayCaseID string `json:"replay_case_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &selected); err != nil || created.Code != 201 {
		t.Fatalf("case: %d %s", created.Code, created.Body.String())
	}
	queued := callSegment(t, ws, "POST", "/api/lidar/scenes/"+selected.ReplayCaseID+"/clip", nil, func(w http.ResponseWriter, r *http.Request) {
		ws.handleSceneClip(w, r, selected.ReplayCaseID)
	})
	var response struct {
		Job struct {
			ID string `json:"job_id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &response); err != nil || queued.Code != 202 {
		t.Fatalf("clip: %d %s", queued.Code, queued.Body.String())
	}
	return response.Job.ID, chosen.ID
}

// writeClipAttempt leaves what an attempt of the job would: a directory named
// for the job, holding a pack cut for the segment and its segment record.
func writeClipAttempt(t *testing.T, ws *Server, jobID, segmentID, suffix string) (pack string, digest string) {
	t.Helper()
	selection, err := sqlite.NewSegmentStore(ws.db).Selection(segmentID)
	if err != nil {
		t.Fatal(err)
	}
	var params segments.Params
	var chosen segments.Window
	if err := json.Unmarshal([]byte(selection.ParametersJSON), &params); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(selection.WindowJSON), &chosen); err != nil {
		t.Fatal(err)
	}
	attempt := filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(jobID)+suffix)
	if err := os.MkdirAll(attempt, 0755); err != nil {
		t.Fatal(err)
	}
	exported, err := annotation.Export(annotation.ExportConfig{VRLOGPath: writeTestVRLOG(t, 2), OutDir: filepath.Join(attempt, "pack"), Coverage: annotation.CoverageForegroundOnly})
	if err != nil {
		t.Fatal(err)
	}
	record := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: exported.Manifest.PackDigest, Role: selection.Role, Finder: selection.Finder, FinderVersion: chosen.Version, Parameters: params, Segment: chosen}
	if err := segments.WriteRecord(exported.Dir, record); err != nil {
		t.Fatal(err)
	}
	return exported.Dir, exported.Manifest.PackDigest
}

func finishClip(t *testing.T, ws *Server, jobID string) {
	t.Helper()
	if err := sqlite.NewCaptureStore(ws.db).FinishJob(jobID, sqlite.JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
}

func TestStoredPackDirectoryIsInsideThePacksDirectory(t *testing.T) {
	ws, _ := segmentServer(t)
	inside := filepath.Join(ws.annotationPacksDir, "clip-job-1", "pack")
	stored, err := ws.storedPackDir(inside)
	if err != nil || stored != "clip-job-1/pack" {
		t.Fatalf("stored %q: %v", stored, err)
	}
	if got := ws.segmentPackPath(stored); got != inside {
		t.Fatalf("path from the stored directory: %q, want %q", got, inside)
	}
	for name, dir := range map[string]string{
		"the packs directory itself": ws.annotationPacksDir,
		"its parent":                 filepath.Dir(ws.annotationPacksDir),
		"a sibling":                  filepath.Join(filepath.Dir(ws.annotationPacksDir), "elsewhere", "pack"),
		"a relative path":            "clip-job-1/pack",
	} {
		if stored, err := ws.storedPackDir(dir); err == nil {
			t.Fatalf("%s was stored as %q", name, stored)
		}
	}
}

func TestOnlyAWholePackForTheSegmentIsRecognised(t *testing.T) {
	ws, _ := segmentServer(t)
	jobID, segmentID := queuedClip(t, ws)
	pack, digest := writeClipAttempt(t, ws, jobID, segmentID, "whole")
	if got, whole := segmentPackDigest(pack, segmentID); !whole || got != digest {
		t.Fatalf("whole pack: %q %t", got, whole)
	}
	if _, whole := segmentPackDigest(pack, "seg-another"); whole {
		t.Fatal("a pack cut for another segment was recognised")
	}
	if _, whole := segmentPackDigest(filepath.Join(ws.annotationPacksDir, "absent"), segmentID); whole {
		t.Fatal("a pack that does not exist was recognised")
	}
	record := filepath.Join(pack, "segment.json")
	original, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"not JSON":              []byte("{"),
		"bound to another pack": []byte(strings.Replace(string(original), digest, "sha256:"+strings.Repeat("b", 64), 1)),
		"an invalid record":     []byte(strings.Replace(string(original), `"schema_version": 1`, `"schema_version": 2`, 1)),
	} {
		if err := os.WriteFile(record, content, 0644); err != nil {
			t.Fatal(err)
		}
		if _, whole := segmentPackDigest(pack, segmentID); whole {
			t.Fatalf("a pack whose record is %s was recognised", name)
		}
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if _, whole := segmentPackDigest(pack, segmentID); whole {
		t.Fatal("a pack without a segment record was recognised")
	}
}

func TestFinishedClipGetsItsPackBackAtStart(t *testing.T) {
	ws, _ := segmentServer(t)
	store := sqlite.NewSegmentStore(ws.db)
	if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
		t.Fatalf("nothing to link: %d %v", linked, err)
	}
	jobID, segmentID := queuedClip(t, ws)
	pack, digest := writeClipAttempt(t, ws, jobID, segmentID, "1")
	// A clip that is still queued is the worker's to finish.
	if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
		t.Fatalf("queued clip was linked: %d %v", linked, err)
	}
	finishClip(t, ws, jobID)
	linked, err := ws.relinkSegmentPacks()
	if err != nil || linked != 1 {
		t.Fatalf("finished clip: %d %v", linked, err)
	}
	clip, _, err := store.ClipJob(jobID)
	if err != nil || clip.PackDigest != digest || ws.segmentPackPath(clip.PackDir) != pack {
		t.Fatalf("linked pack: %+v %v, want %s %s", clip, err, pack, digest)
	}
	windows, _, err := ws.findRunSegments(segmentRequest{RunID: "run"})
	if err != nil || len(windows) != 1 || windows[0].Status != "packed" || windows[0].PackDir != pack {
		t.Fatalf("ranking after the link: %+v %v", windows, err)
	}
	// Once linked there is nothing left to do, and nothing is done twice.
	if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
		t.Fatalf("second pass: %d %v", linked, err)
	}
	if _, err := os.Stat(pack); err != nil {
		t.Fatalf("linking must only read: %v", err)
	}
}

func TestRelinkLeavesWhatItCannotRecognise(t *testing.T) {
	t.Run("no pack on disk", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, _ := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("clip with no pack: %d %v", linked, err)
		}
		ws.annotationPacksDir = filepath.Join(ws.annotationPacksDir, "never-made")
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("packs directory that does not exist: %d %v", linked, err)
		}
	})
	t.Run("unfinished attempt", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, _ := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		attempt := filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(jobID)+"half", "vrlog")
		if err := os.MkdirAll(attempt, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ws.annotationPacksDir, clipAttemptPrefix(jobID)+"note"), []byte("a file, not an attempt"), 0644); err != nil {
			t.Fatal(err)
		}
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("unfinished attempt: %d %v", linked, err)
		}
		if _, err := os.Stat(attempt); err != nil {
			t.Fatalf("relinking removed an attempt it does not own: %v", err)
		}
	})
	t.Run("another job's pack", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, segmentID := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		writeClipAttempt(t, ws, "job-another", segmentID, "1")
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("pack of another job: %d %v", linked, err)
		}
	})
	t.Run("two whole packs", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, segmentID := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		first, _ := writeClipAttempt(t, ws, jobID, segmentID, "1")
		writeClipAttempt(t, ws, jobID, segmentID, "2")
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 1 {
			t.Fatalf("two attempts: %d %v", linked, err)
		}
		clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(jobID)
		if err != nil || ws.segmentPackPath(clip.PackDir) != first {
			t.Fatalf("the first whole pack in name order is the one linked: %+v %v", clip, err)
		}
	})
	t.Run("not configured", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, segmentID := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		writeClipAttempt(t, ws, jobID, segmentID, "1")
		packs := ws.annotationPacksDir
		ws.annotationPacksDir = ""
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("no packs directory: %d %v", linked, err)
		}
		ws.annotationPacksDir = packs
		database := ws.db
		ws.db = nil
		if linked, err := ws.relinkSegmentPacks(); err != nil || linked != 0 {
			t.Fatalf("no database: %d %v", linked, err)
		}
		ws.db = database
	})
}

func TestRelinkReportsWhatItCouldNotRead(t *testing.T) {
	t.Run("clip jobs", func(t *testing.T) {
		ws, _ := segmentServer(t)
		if _, err := ws.db.Exec(`ALTER TABLE lidar_segment_clip_jobs RENAME COLUMN pack_dir TO broken_pack_dir`); err != nil {
			t.Fatal(err)
		}
		if _, err := ws.relinkSegmentPacks(); err == nil {
			t.Fatal("clip jobs that could not be listed went unreported")
		}
	})
	t.Run("packs directory", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, _ := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		// A file where the directory should be cannot be listed.
		file := filepath.Join(t.TempDir(), "packs")
		if err := os.WriteFile(file, []byte("not a directory"), 0644); err != nil {
			t.Fatal(err)
		}
		ws.annotationPacksDir = file
		if _, err := ws.relinkSegmentPacks(); err == nil {
			t.Fatal("a packs directory that could not be listed went unreported")
		}
	})
	t.Run("link refused", func(t *testing.T) {
		ws, _ := segmentServer(t)
		jobID, segmentID := queuedClip(t, ws)
		finishClip(t, ws, jobID)
		writeClipAttempt(t, ws, jobID, segmentID, "1")
		if _, err := ws.db.Exec(`CREATE TRIGGER refuse_pack_link BEFORE UPDATE ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced link failure'); END`); err != nil {
			t.Fatal(err)
		}
		if linked, err := ws.relinkSegmentPacks(); err == nil || linked != 0 {
			t.Fatalf("refused link: %d %v", linked, err)
		}
	})
}

// An attempt's pack is named from the packs directory, however that
// directory is written, and found again from the name.
func TestFoundClipPackIsNamedFromThePacksDirectory(t *testing.T) {
	ws, _ := segmentServer(t)
	jobID, segmentID := queuedClip(t, ws)
	dir, digest := writeClipAttempt(t, ws, jobID, segmentID, "1")
	pack, unfinished, err := ws.findClipPack(jobID, segmentID, os.ReadDir)
	if err != nil || pack == nil || len(unfinished) != 0 {
		t.Fatalf("attempt: %+v %v %v", pack, unfinished, err)
	}
	if pack.stored != clipAttemptPrefix(jobID)+"1/pack" || pack.dir != dir || pack.digest != digest || ws.segmentPackPath(pack.stored) != dir {
		t.Fatalf("found pack: %+v, want %s %s", pack, dir, digest)
	}
	if stored, err := ws.storedPackDir(dir); err != nil || stored != pack.stored {
		t.Fatalf("the two names of one pack differ: %q and %q: %v", stored, pack.stored, err)
	}
}

func TestCaptureQueueStartsWhetherOrNotPacksRelink(t *testing.T) {
	for name, prepare := range map[string]func(*testing.T, *Server){
		"a pack to link": func(t *testing.T, ws *Server) {
			jobID, segmentID := queuedClip(t, ws)
			finishClip(t, ws, jobID)
			writeClipAttempt(t, ws, jobID, segmentID, "1")
		},
		"a failure to link": func(t *testing.T, ws *Server) {
			jobID, segmentID := queuedClip(t, ws)
			finishClip(t, ws, jobID)
			writeClipAttempt(t, ws, jobID, segmentID, "1")
			if _, err := ws.db.Exec(`CREATE TRIGGER refuse_pack_link BEFORE UPDATE ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced link failure'); END`); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ws, _ := segmentServer(t)
			prepare(t, ws)
			ctx, cancel := context.WithCancel(context.Background())
			if err := ws.StartCaptureJobs(ctx); err != nil {
				t.Fatalf("the queue did not start: %v", err)
			}
			cancel()
			ws.captureRunner.Wait()
			clip, _, err := sqlite.NewSegmentStore(ws.db).ClipJob(firstClipJob(t, ws))
			if err != nil {
				t.Fatal(err)
			}
			if linked := clip.PackDir != ""; linked != (name == "a pack to link") {
				t.Fatalf("pack link after start: %+v", clip)
			}
		})
	}
}

func firstClipJob(t *testing.T, ws *Server) string {
	t.Helper()
	var jobID string
	if err := ws.db.QueryRow(`SELECT job_id FROM lidar_segment_clip_jobs ORDER BY job_id LIMIT 1`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func TestFindClipPackReportsAnUnreadableDirectory(t *testing.T) {
	ws, _ := segmentServer(t)
	failure := errors.New("directory could not be read")
	if _, _, err := ws.findClipPack("job-1", "seg-1", func(string) ([]os.DirEntry, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatalf("unreadable directory: %v", err)
	}
}
