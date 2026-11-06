package sqlite

import (
	"errors"
	"testing"
)

func TestLaneProposalStoreKeepsOneActiveJobAndReadsTheNewest(t *testing.T) {
	clips := setupExtractStore(t)
	insertCase(t, clips, "clip-1", "kirk/kirk2.pcapng")
	s := NewLaneProposalStore(clips.db)

	first, created, err := s.EnqueueLaneProposals("clip-1", "queued")
	if err != nil || !created || first.Kind != JobKindLaneProposals {
		t.Fatalf("first enqueue: %+v created=%v err=%v", first, created, err)
	}
	again, created, err := s.EnqueueLaneProposals("clip-1", "queued")
	if err != nil || created || again.JobID != first.JobID {
		t.Fatalf("second enqueue made %+v (created=%v, err=%v), want the active %s", again, created, err, first.JobID)
	}
	if _, _, err := s.EnqueueLaneProposals("missing", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown clip: %v", err)
	}

	latest, err := s.LatestByClip()
	if err != nil {
		t.Fatal(err)
	}
	if cl := latest["clip-1"]; cl.Latest != nil || cl.Job == nil || cl.Job.JobID != first.JobID || !cl.Job.Active() {
		t.Fatalf("queued clip lanes = %+v", cl)
	}

	row, err := s.ForJob(first.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(LaneProposalRun{JobID: row.JobID}); err == nil {
		t.Fatal("a completion with no record or fields was accepted")
	}
	row.RecordID, row.FieldIDs, row.High, row.Medium, row.Outstanding = "gms:1", []string{"sha256:a", "sha256:b", "sha256:c"}, 4, 2, 6
	row.PipelineBuild, row.TuningFingerprint = "0.5.2+abc", "ab0c4035780d"
	if _, err := s.Complete(row); err != nil {
		t.Fatal(err)
	}
	finishJob(t, clips, first.JobID, JobCompleted)

	latest, err = s.LatestByClip()
	if err != nil {
		t.Fatal(err)
	}
	cl := latest["clip-1"]
	if cl.Latest == nil || cl.Latest.High != 4 || len(cl.Latest.FieldIDs) != 3 || cl.Latest.TuningFingerprint != "ab0c4035780d" {
		t.Fatalf("completed clip lanes = %+v", cl.Latest)
	}

	second, created, err := s.EnqueueLaneProposals("clip-1", "")
	if err != nil || !created || second.JobID == first.JobID {
		t.Fatalf("a completed job blocks a re-run: %+v %v %v", second, created, err)
	}
	latest, _ = s.LatestByClip()
	if cl := latest["clip-1"]; cl.Latest == nil || cl.Latest.JobID != first.JobID || cl.Job.JobID != second.JobID {
		t.Fatalf("after a re-run is queued: latest %+v job %+v", cl.Latest, cl.Job)
	}

	// The run form writes a row of its own, without a job.
	byRun, err := s.Complete(LaneProposalRun{ClipID: "clip-1", RunID: "run-9", RecordID: "gms:1", FieldIDs: []string{"sha256:d"}, High: 7})
	if err != nil || byRun.ProposalRunID == "" {
		t.Fatalf("run form: %+v %v", byRun, err)
	}
	latest, _ = s.LatestByClip()
	if cl := latest["clip-1"]; cl.Latest == nil || cl.Latest.RunID != "run-9" || cl.Latest.High != 7 {
		t.Fatalf("run-form row is not the newest: %+v", cl.Latest)
	}
}

func TestLaneProposalJobsNeedTheirClipRow(t *testing.T) {
	clips := setupExtractStore(t)
	if _, err := clips.db.Exec(`INSERT INTO lidar_capture_jobs (job_id, kind, session_id, state, detail, queued_at_ns)
		VALUES ('j', 'lane_proposals', 'sess', 'queued', '', 1)`); err == nil {
		t.Fatal("a lane proposals job naming a session was admitted")
	}
	if _, err := clips.db.Exec(`INSERT INTO lidar_capture_jobs (job_id, kind, state, detail, queued_at_ns)
		VALUES ('j', 'lane_proposals', 'queued', '', 1)`); err != nil {
		t.Fatalf("a lane proposals job was refused: %v", err)
	}
}
