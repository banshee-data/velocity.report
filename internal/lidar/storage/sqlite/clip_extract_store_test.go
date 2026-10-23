package sqlite

import (
	"errors"
	"testing"
)

func setupExtractStore(t *testing.T) *ClipStore {
	t.Helper()
	db := setupClipFilesDB(t)
	// One connection: an in-memory database is per connection.
	db.SetMaxOpenConns(1)
	return NewClipStore(db)
}

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func finishJob(t *testing.T, s *ClipStore, jobID, state string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE lidar_capture_jobs SET state = ? WHERE job_id = ?`, state, jobID); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueClipExtractKeepsOneActiveJobPerClip(t *testing.T) {
	s := setupExtractStore(t)
	insertCase(t, s, "clip-1", "kirk/kirk2.pcapng")

	first, created, err := s.EnqueueClipExtract("clip-1", 1, "queued")
	if err != nil || !created || first.Kind != JobKindCaptureExport {
		t.Fatalf("first enqueue: %+v created=%v err=%v", first, created, err)
	}
	again, created, err := s.EnqueueClipExtract("clip-1", 1, "queued")
	if err != nil || created || again.JobID != first.JobID {
		t.Fatalf("second enqueue made %+v (created=%v, err=%v), want the active %s", again, created, err, first.JobID)
	}
	if _, _, err := s.EnqueueClipExtract("missing", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown clip: %v", err)
	}

	finishJob(t, s, first.JobID, JobFailed)
	next, created, err := s.EnqueueClipExtract("clip-1", 1, "")
	if err != nil || !created || next.JobID == first.JobID {
		t.Fatalf("a failed export blocks a retry: %+v %v %v", next, created, err)
	}
	extracts, err := s.ClipExtracts("clip-1")
	if err != nil || len(extracts) != 2 {
		t.Fatalf("extracts: %v %v", extracts, err)
	}
}

func TestCompletedExtractBecomesReplayable(t *testing.T) {
	s := setupExtractStore(t)
	insertCase(t, s, "clip-1", "kirk/kirk2.pcapng")

	if err := s.SetClipPublishing("clip-1", ClipPublishing{Storage: ClipStorageExtract}); !errors.Is(err, ErrNoExtract) {
		t.Fatalf("extract storage without an extract: %v", err)
	}
	job, _, err := s.EnqueueClipExtract("clip-1", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LatestClipExtract("clip-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a queued extract counts as complete: %v", err)
	}
	if err := s.CompleteClipExtract(ClipExtract{
		JobID: job.JobID, RelPath: "clips/k3.pcapng", SHA256: testDigest, SizeBytes: 10,
		PacketCount: 4, LidarPacketCount: 3, FrameCount: 1, FirstPacketNs: 5, LastPacketNs: 9,
		ClosedBy: "window", RubricVersion: 1, Sidecar: []byte(`{"schema_version":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	finishJob(t, s, job.JobID, JobCompleted)

	latest, err := s.LatestClipExtract("clip-1")
	if err != nil || !latest.Completed() || latest.RelPath != "clips/k3.pcapng" || latest.FrameCount != 1 {
		t.Fatalf("latest extract %+v %v", latest, err)
	}
	if string(latest.Sidecar) != `{"schema_version":1}` {
		t.Fatalf("sidecar %s", latest.Sidecar)
	}
	if err := s.SetClipPublishing("clip-1", ClipPublishing{Storage: ClipStorageExtract}); err != nil {
		t.Fatal(err)
	}
	clip, err := s.GetClip("clip-1")
	if err != nil || clip.Storage != ClipStorageExtract {
		t.Fatalf("storage %q %v", clip.Storage, err)
	}
	if err := s.CompleteClipExtract(ClipExtract{JobID: "nope", RubricVersion: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("completing an unknown job: %v", err)
	}
}

func TestSetClipPublishingValidates(t *testing.T) {
	s := setupExtractStore(t)
	insertCase(t, s, "clip-1", "a.pcapng")
	insertCase(t, s, "clip-2", "b.pcapng")

	clip, err := s.GetClip("clip-1")
	if err != nil || clip.Storage != ClipStorageSources || clip.DatasetID != "" {
		t.Fatalf("a new clip: %+v %v", clip, err)
	}

	az := 35.0
	if err := s.SetClipPublishing("clip-1", ClipPublishing{DatasetID: "kirkham-9th-k2", NorthAzimuthDeg: &az, Storage: ClipStorageSources}); err != nil {
		t.Fatal(err)
	}
	clip, _ = s.GetClip("clip-1")
	if clip.DatasetID != "kirkham-9th-k2" || clip.NorthAzimuthDeg == nil || *clip.NorthAzimuthDeg != 35 {
		t.Fatalf("publishing not stored: %+v", clip)
	}
	if err := s.SetClipPublishing("clip-2", ClipPublishing{DatasetID: "kirkham-9th-k2", Storage: ClipStorageSources}); !errors.Is(err, ErrDatasetIDTaken) {
		t.Fatalf("duplicate dataset id: %v", err)
	}
	bad := 360.0
	for name, p := range map[string]ClipPublishing{
		"upper case": {DatasetID: "Kirkham", Storage: ClipStorageSources},
		"underscore": {DatasetID: "kirk_2", Storage: ClipStorageSources},
		"azimuth":    {NorthAzimuthDeg: &bad, Storage: ClipStorageSources},
		"storage":    {Storage: "copy"},
	} {
		if err := s.SetClipPublishing("clip-2", p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := s.SetClipPublishing("missing", ClipPublishing{Storage: ClipStorageSources}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown clip: %v", err)
	}

	clips, err := s.PublishingClips()
	if err != nil || len(clips) != 1 || clips[0].ClipID != "clip-1" {
		t.Fatalf("publishing clips: %v %v", clips, err)
	}
	// Clearing the id takes the clip off the list.
	if err := s.SetClipPublishing("clip-1", ClipPublishing{Storage: ClipStorageSources}); err != nil {
		t.Fatal(err)
	}
	if clips, _ := s.PublishingClips(); len(clips) != 0 {
		t.Fatalf("cleared clip still listed: %v", clips)
	}
}

func TestPeriodClipsPrefersALocatedClip(t *testing.T) {
	s := setupExtractStore(t)
	insertCase(t, s, "older-located", "a.pcapng")
	insertCase(t, s, "newer-unlocated", "a.pcapng")
	insertCase(t, s, "other-period", "b.pcapng")
	for clip, period := range map[string]string{"older-located": "per-1", "newer-unlocated": "per-1", "other-period": "per-2"} {
		if err := s.SetClipSession(clip, "ses-1", period); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`UPDATE lidar_clips SET created_at_ns = created_at_ns + 1000 WHERE clip_id = 'newer-unlocated'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipLocation("older-located", kirkhamLat, kirkhamLng, ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.PeriodClips([]string{"per-1", "per-2", "per-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["per-1"].ClipID != "older-located" || !got["per-1"].Located || *got["per-1"].Lat != kirkhamLat {
		t.Fatalf("per-1: %+v", got)
	}
	if got["per-2"].ClipID != "other-period" || got["per-2"].Located {
		t.Fatalf("per-2: %+v", got["per-2"])
	}
	if empty, err := s.PeriodClips(nil); err != nil || len(empty) != 0 {
		t.Fatalf("no periods: %v %v", empty, err)
	}
}
