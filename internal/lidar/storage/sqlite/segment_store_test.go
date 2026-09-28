package sqlite

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

const segmentStoreDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// segmentWindow is the document a finder returns for a chosen window, with
// the fields the database derives its columns from.
func segmentWindow(id, finder, role, source string, edit func(map[string]any)) []byte {
	doc := map[string]any{
		"id": id, "finder": finder, "version": 1, "source": source, "role": role,
		"window_start_unix_nanos": int64(1_788_466_680_000_000_000),
		"window_end_unix_nanos":   int64(1_788_466_690_000_000_000),
		"peak_timestamp_ns":       int64(1_788_466_684_000_000_000),
		"score":                   12.5, "capture": "/captures/a.pcap", "offset_seconds": 45.3,
		"status": "candidate",
	}
	if edit != nil {
		edit(doc)
	}
	b, _ := json.Marshal(doc)
	return b
}

var segmentParameters = []byte(`{"window_seconds":10,"min_speed":3,"random_seed":1}`)

// segmentFixture is a database with two runs and the replay cases named.
func segmentFixture(t *testing.T, cases ...string) (*SegmentStore, DBClient) {
	t.Helper()
	db, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	for _, run := range []string{"run-a", "run-b"} {
		if _, err := db.Exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,sensor_id,status) VALUES(?,1,'pcap','sensor','completed')`, run); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range cases {
		if _, err := db.Exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES(?,'sensor','a.pcap',1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	return NewSegmentStore(db), db
}

func TestSegmentSelectionIsReadFromItsDocument(t *testing.T) {
	store, _ := segmentFixture(t, "case-1")
	window := segmentWindow("seg-1", "following", "tuning", "run-a", nil)
	if err := store.InsertSelection("seg-1", "run-a", "case-1", segmentParameters, window); err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func() (SegmentSelection, error){
		"by segment": func() (SegmentSelection, error) { return store.Selection("seg-1") },
		"by case":    func() (SegmentSelection, error) { return store.SelectionForCase("case-1") },
	} {
		got, err := read()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := SegmentSelection{
			SegmentID: "seg-1", RunID: "run-a", Source: "run-a", ReplayCaseID: "case-1",
			Role: "tuning", Finder: "following", FinderVersion: 1, Capture: "/captures/a.pcap",
			WindowStartNs: 1_788_466_680_000_000_000, WindowEndNs: 1_788_466_690_000_000_000,
			ParametersJSON: string(segmentParameters), WindowJSON: string(window), CreatedAtNs: got.CreatedAtNs,
		}
		if got != want || got.CreatedAtNs <= 0 {
			t.Fatalf("%s:\n got %+v\nwant %+v", name, got, want)
		}
	}
	for name, err := range map[string]error{
		"unknown segment": func() error { _, err := store.Selection("seg-none"); return err }(),
		"freehand case":   func() error { _, err := store.SelectionForCase("case-none"); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, args := range map[string][3]string{
		"no segment": {"", "run-a", "case-1"},
		"no run":     {"seg-2", "", "case-1"},
		"no case":    {"seg-2", "run-a", ""},
	} {
		if err := store.InsertSelection(args[0], args[1], args[2], segmentParameters, window); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

// The database holds the rules, so a writer that skips the Go checks is
// refused as well. Each case breaks one rule and nothing else.
func TestSegmentSelectionRulesAreHeldByTheDatabase(t *testing.T) {
	for _, tc := range []struct {
		name       string
		segment    string
		run        string
		parameters []byte
		window     []byte
	}{
		{"window is not JSON", "seg-1", "run-a", segmentParameters, []byte(`{not json`)},
		{"window is not an object", "seg-1", "run-a", segmentParameters, []byte(`["seg-1"]`)},
		{"parameters are not JSON", "seg-1", "run-a", []byte(`{`), segmentWindow("seg-1", "following", "tuning", "run-a", nil)},
		{"parameters are not an object", "seg-1", "run-a", []byte(`10`), segmentWindow("seg-1", "following", "tuning", "run-a", nil)},
		{"window names another segment", "seg-1", "run-a", segmentParameters, segmentWindow("seg-2", "following", "tuning", "run-a", nil)},
		{"window names another source", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-b", nil)},
		{"run does not exist", "seg-1", "run-none", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-none", nil)},
		{"role outside the vocabulary", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "validation", "run-a", nil)},
		{"finder outside the catalogue", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "manual", "tuning", "run-a", nil)},
		{"finder left empty", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "", "tuning", "run-a", nil)},
		{"held-out window from a failure finder", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "leader_changes", "held_out", "run-a", nil)},
		{"no finder version", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { delete(d, "version") })},
		{"finder version zero", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { d["version"] = 0 })},
		{"finder version is text", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { d["version"] = "one" })},
		{"window ends where it starts", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { d["window_end_unix_nanos"] = d["window_start_unix_nanos"] })},
		{"window has no end", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { delete(d, "window_end_unix_nanos") })},
		{"window bounds are text", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { d["window_start_unix_nanos"] = "early" })},
		{"no capture", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { delete(d, "capture") })},
		{"empty capture", "seg-1", "run-a", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", func(d map[string]any) { d["capture"] = "" })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := segmentFixture(t, "case-1")
			if err := store.InsertSelection(tc.segment, tc.run, "case-1", tc.parameters, tc.window); err == nil {
				t.Fatal("the database accepted it")
			}
			var rows int
			if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_segment_selections`).Scan(&rows); err != nil || rows != 0 {
				t.Fatalf("refused selection left %d rows: %v", rows, err)
			}
		})
	}
	t.Run("replay case does not exist", func(t *testing.T) {
		store, _ := segmentFixture(t)
		if err := store.InsertSelection("seg-1", "run-a", "case-none", segmentParameters, segmentWindow("seg-1", "following", "tuning", "run-a", nil)); err == nil {
			t.Fatal("the database accepted a selection for no case")
		}
	})
	t.Run("one case, one segment", func(t *testing.T) {
		store, _ := segmentFixture(t, "case-1", "case-2")
		first := segmentWindow("seg-1", "following", "tuning", "run-a", nil)
		if err := store.InsertSelection("seg-1", "run-a", "case-1", segmentParameters, first); err != nil {
			t.Fatal(err)
		}
		if err := store.InsertSelection("seg-1", "run-a", "case-2", segmentParameters, first); err == nil {
			t.Fatal("a segment was chosen twice")
		}
		if err := store.InsertSelection("seg-2", "run-a", "case-1", segmentParameters, segmentWindow("seg-2", "following", "tuning", "run-a", nil)); err == nil {
			t.Fatal("a case was given a second segment")
		}
	})
	t.Run("a stored document cannot be edited out of the rules", func(t *testing.T) {
		store, db := segmentFixture(t, "case-1")
		if err := store.InsertSelection("seg-1", "run-a", "case-1", segmentParameters, segmentWindow("seg-1", "following", "held_out", "run-a", nil)); err != nil {
			t.Fatal(err)
		}
		for name, update := range map[string]string{
			"finder":   `UPDATE lidar_segment_selections SET window_json = json_set(window_json, '$.finder', 'lateral_jump')`,
			"document": `UPDATE lidar_segment_selections SET window_json = '{'`,
			"version":  `UPDATE lidar_segment_selections SET document_version = 2`,
			"role":     `UPDATE lidar_segment_selections SET role = 'tuning'`,
		} {
			if _, err := db.Exec(update); err == nil {
				t.Fatalf("%s was edited past its rule", name)
			}
		}
	})
}

func TestDeletingARunKeepsItsSelections(t *testing.T) {
	store, db := segmentFixture(t, "case-1")
	if err := store.InsertSelection("seg-1", "run-a", "case-1", segmentParameters, segmentWindow("seg-1", "exposure", "held_out", "run-a", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM lidar_run_records WHERE run_id='run-a'`); err != nil {
		t.Fatalf("a selection must not stop its run from being deleted: %v", err)
	}
	got, err := store.Selection("seg-1")
	if err != nil {
		t.Fatalf("selection did not outlive its run: %v", err)
	}
	// The run is gone; what it was called, and what was chosen from it, stay.
	if got.RunID != "" || got.Source != "run-a" || got.Role != "held_out" || got.Finder != "exposure" || got.ReplayCaseID != "case-1" {
		t.Fatalf("selection after its run was deleted: %+v", got)
	}
	// Deleting the replay case is a decision about the selection itself.
	if _, err := db.Exec(`DELETE FROM lidar_replay_cases WHERE replay_case_id='case-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Selection("seg-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("selection outlived its replay case: %v", err)
	}
}

func TestHeldOutGuardIsALookup(t *testing.T) {
	store, db := segmentFixture(t, "case-1", "case-2", "case-3")
	for _, s := range []struct{ id, caseID, finder, role, capture string }{
		{"seg-1", "case-1", "random", "tuning", "/captures/a.pcap"},
		{"seg-2", "case-2", "following", "held_out", "/captures/a.pcap"},
		{"seg-3", "case-3", "random", "held_out", "/captures/b.pcap"},
	} {
		window := segmentWindow(s.id, s.finder, s.role, "run-a", func(d map[string]any) { d["capture"] = s.capture })
		if err := store.InsertSelection(s.id, "run-a", s.caseID, segmentParameters, window); err != nil {
			t.Fatal(err)
		}
	}
	for capture, want := range map[string]bool{
		// A tuning random window and a held-out traffic window are not a
		// held-out random window.
		"/captures/a.pcap": false,
		"/captures/b.pcap": true,
		"/captures/c.pcap": false,
	} {
		got, err := store.HasRandomHeldOut(capture)
		if err != nil || got != want {
			t.Fatalf("%s: %t %v, want %t", capture, got, err, want)
		}
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT 1 FROM lidar_segment_selections WHERE role = 'held_out' AND finder = 'random' AND capture = ?`, "/captures/a.pcap")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if !strings.Contains(plan, "idx_lidar_segment_selections_guard") || strings.Contains(plan, "SCAN") {
		t.Fatalf("the guard reads every selection:\n%s", plan)
	}
	if _, err := db.Exec(`DROP TABLE lidar_segment_clip_jobs`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE lidar_segment_selections`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HasRandomHeldOut("/captures/a.pcap"); err == nil {
		t.Fatal("a guard that could not be read was taken for an answer")
	}
}

func clipFixture(t *testing.T) (*SegmentStore, DBClient) {
	t.Helper()
	store, db := segmentFixture(t, "case-1", "case-2")
	for _, s := range []struct{ id, caseID string }{{"seg-1", "case-1"}, {"seg-2", "case-2"}} {
		if err := store.InsertSelection(s.id, "run-a", s.caseID, segmentParameters, segmentWindow(s.id, "following", "tuning", "run-a", nil)); err != nil {
			t.Fatal(err)
		}
	}
	return store, db
}

func TestClipIsQueuedWithItsSegment(t *testing.T) {
	store, db := clipFixture(t)
	if _, err := store.Status("seg-none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status of a window that was not chosen: %v", err)
	}
	if status, err := store.Status("seg-1"); err != nil || status != (SegmentStatus{ReplayCaseID: "case-1"}) {
		t.Fatalf("status before a clip: %+v %v", status, err)
	}
	job, err := store.EnqueueClip("seg-1", "clip for case-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobID == "" || job.Kind != JobKindSegmentClip || job.State != JobQueued || job.SessionID != "" || job.Detail != "clip for case-1" {
		t.Fatalf("queued clip: %+v", job)
	}
	// The queue's session column names capture sessions. A segment is named
	// by the link, and the two are written together.
	var session any
	var linked string
	if err := db.QueryRow(`SELECT j.session_id, c.segment_id FROM lidar_capture_jobs j JOIN lidar_segment_clip_jobs c ON c.job_id=j.job_id WHERE j.job_id=?`, job.JobID).Scan(&session, &linked); err != nil || session != nil || linked != "seg-1" {
		t.Fatalf("job row session %v, linked segment %q: %v", session, linked, err)
	}
	again, err := store.EnqueueClip("seg-1", "clip for case-1, asked twice")
	if err != nil || again.JobID != job.JobID || again.Detail != "clip for case-1" {
		t.Fatalf("second request while queued: %+v %v", again, err)
	}
	other, err := store.EnqueueClip("seg-2", "clip for case-2")
	if err != nil || other.JobID == job.JobID {
		t.Fatalf("another segment's clip: %+v %v", other, err)
	}
	claimed, err := NewCaptureStore(db).ClaimNextJob()
	if err != nil || claimed.JobID != job.JobID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if running, err := store.EnqueueClip("seg-1", "asked while running"); err != nil || running.JobID != job.JobID || running.State != JobRunning {
		t.Fatalf("request while running: %+v %v", running, err)
	}
	clip, selection, err := store.ClipJob(job.JobID)
	if err != nil || clip != (SegmentClipJob{JobID: job.JobID, SegmentID: "seg-1", State: JobRunning, QueuedAtNs: job.QueuedAtNs}) || selection.ReplayCaseID != "case-1" {
		t.Fatalf("clip job: %+v %+v %v", clip, selection, err)
	}
	if status, err := store.Status("seg-1"); err != nil || status != (SegmentStatus{ReplayCaseID: "case-1", JobID: job.JobID, JobState: JobRunning}) {
		t.Fatalf("status while clipping: %+v %v", status, err)
	}
	if err := store.LinkPack(job.JobID, "clip-"+job.JobID+"-1/pack", segmentStoreDigest); err != nil {
		t.Fatal(err)
	}
	if err := NewCaptureStore(db).FinishJob(job.JobID, JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
	clip, _, err = store.ClipJob(job.JobID)
	if err != nil || clip.PackDir != "clip-"+job.JobID+"-1/pack" || clip.PackDigest != segmentStoreDigest || clip.State != JobCompleted {
		t.Fatalf("clip job after its pack: %+v %v", clip, err)
	}
	// A finished clip does not stand in the way of cutting the segment again,
	// and the ranking shows the newest job.
	next, err := store.EnqueueClip("seg-1", "second clip")
	if err != nil || next.JobID == job.JobID {
		t.Fatalf("clip after a finished one: %+v %v", next, err)
	}
	if status, err := store.Status("seg-1"); err != nil || status.JobID != next.JobID || status.PackDir != "" {
		t.Fatalf("status shows an older job: %+v %v", status, err)
	}
	if _, _, err := store.ClipJob("job-none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown clip job: %v", err)
	}
	if _, err := store.EnqueueClip("", "no segment"); err == nil {
		t.Fatal("a clip of no segment was queued")
	}
	if _, err := store.EnqueueClip("seg-none", "unknown segment"); err == nil {
		t.Fatal("a clip of an unknown segment was queued")
	}
	var orphans int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_capture_jobs j WHERE j.kind=? AND NOT EXISTS (SELECT 1 FROM lidar_segment_clip_jobs c WHERE c.job_id=j.job_id)`, JobKindSegmentClip).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("a refused clip left %d job(s) that name no segment: %v", orphans, err)
	}
}

// Two requests arriving together must agree on one job.
func TestConcurrentClipRequestsShareOneJob(t *testing.T) {
	store, db := clipFixture(t)
	const requests = 8
	ids := make([]string, requests)
	errs := make([]error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			job, err := store.EnqueueClip("seg-1", "asked together")
			ids[i], errs[i] = job.JobID, err
		}(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] == "" || ids[i] != ids[0] {
			t.Fatalf("request %d: job %q, error %v; first job %q", i, ids[i], errs[i], ids[0])
		}
	}
	var jobs, links int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM lidar_capture_jobs), (SELECT COUNT(*) FROM lidar_segment_clip_jobs)`).Scan(&jobs, &links); err != nil || jobs != 1 || links != 1 {
		t.Fatalf("%d jobs and %d links for one segment: %v", jobs, links, err)
	}
}

func TestPackLinkRules(t *testing.T) {
	store, db := clipFixture(t)
	job, err := store.EnqueueClip("seg-1", "clip")
	if err != nil {
		t.Fatal(err)
	}
	for name, pack := range map[string][2]string{
		"no directory":       {"", segmentStoreDigest},
		"no digest":          {"clip-1/pack", ""},
		"absolute directory": {"/packs/clip-1/pack", segmentStoreDigest},
		"parent directory":   {"../clip-1/pack", segmentStoreDigest},
		"parent inside":      {"clip-1/../../pack", segmentStoreDigest},
		"backslash":          {`clip-1\pack`, segmentStoreDigest},
		"unnamed digest":     {"clip-1/pack", "0123456789abcdef"},
		"empty digest value": {"clip-1/pack", "sha256:"},
		// A pack digest is sha256: and 64 lower-case hex digits, as the
		// exporter writes it; anything else is not a digest of a pack.
		"short digest":      {"clip-1/pack", segmentStoreDigest[:70]},
		"long digest":       {"clip-1/pack", segmentStoreDigest + "0"},
		"upper-case digest": {"clip-1/pack", strings.ToUpper(segmentStoreDigest[:7]) + segmentStoreDigest[7:]},
		"upper-case hex":    {"clip-1/pack", "sha256:" + strings.ToUpper(segmentStoreDigest[7:])},
		"non-hex digest":    {"clip-1/pack", "sha256:" + strings.Repeat("g", 64)},
	} {
		if err := store.LinkPack(job.JobID, pack[0], pack[1]); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if err := store.LinkPack("job-none", "clip-1/pack", segmentStoreDigest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pack for an unknown job: %v", err)
	}
	// The database refuses half a link, whoever writes it.
	for name, update := range map[string]string{
		"directory alone": `UPDATE lidar_segment_clip_jobs SET pack_dir='clip-1/pack'`,
		"digest alone":    `UPDATE lidar_segment_clip_jobs SET pack_digest='` + segmentStoreDigest + `'`,
		"empty directory": `UPDATE lidar_segment_clip_jobs SET pack_dir='', pack_digest='` + segmentStoreDigest + `'`,
	} {
		if _, err := db.Exec(update); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if unlinked, err := store.UnlinkedFinishedClips(); err != nil || len(unlinked) != 0 {
		t.Fatalf("a queued clip is not a finished one: %+v %v", unlinked, err)
	}
	if err := NewCaptureStore(db).FinishJob(job.JobID, JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
	failed, err := store.EnqueueClip("seg-2", "clip that fails")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCaptureStore(db).FinishJob(failed.JobID, JobFailed, "replay refused"); err != nil {
		t.Fatal(err)
	}
	unlinked, err := store.UnlinkedFinishedClips()
	if err != nil || len(unlinked) != 1 || unlinked[0] != (SegmentClipJob{JobID: job.JobID, SegmentID: "seg-1", State: JobCompleted, QueuedAtNs: job.QueuedAtNs}) {
		t.Fatalf("finished clips without a pack: %+v %v", unlinked, err)
	}
	if err := store.LinkPack(job.JobID, "clip-1/pack", segmentStoreDigest); err != nil {
		t.Fatal(err)
	}
	if unlinked, err = store.UnlinkedFinishedClips(); err != nil || len(unlinked) != 0 {
		t.Fatalf("a linked clip is still listed: %+v %v", unlinked, err)
	}
	// Deleting the job takes its link with it; deleting the selection does too.
	if _, err := db.Exec(`DELETE FROM lidar_capture_jobs WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClipJob(job.JobID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("link outlived its job: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM lidar_replay_cases WHERE replay_case_id='case-2'`); err != nil {
		t.Fatal(err)
	}
	var links int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_segment_clip_jobs`).Scan(&links); err != nil || links != 0 {
		t.Fatalf("%d link(s) outlived their selections: %v", links, err)
	}
}

func TestSegmentStoreReportsAnUnreadableDatabase(t *testing.T) {
	t.Run("closed", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		store := NewSegmentStore(db)
		cleanup()
		if _, err := store.EnqueueClip("seg-1", "clip"); err == nil {
			t.Fatal("queued a clip in a closed database")
		}
		if _, err := store.UnlinkedFinishedClips(); err == nil {
			t.Fatal("listed clips from a closed database")
		}
		if err := store.LinkPack("job-1", "clip-1/pack", segmentStoreDigest); err == nil {
			t.Fatal("linked a pack in a closed database")
		}
	})
	t.Run("link refused", func(t *testing.T) {
		store, db := clipFixture(t)
		if _, err := db.Exec(`CREATE TRIGGER refuse_clip_link BEFORE INSERT ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced link failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.EnqueueClip("seg-1", "clip"); err == nil || !strings.Contains(err.Error(), "link clip to segment") {
			t.Fatalf("refused link: %v", err)
		}
		// The job went with the link it could not have.
		var jobs int
		if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_capture_jobs`).Scan(&jobs); err != nil || jobs != 0 {
			t.Fatalf("%d job(s) left without a segment: %v", jobs, err)
		}
	})
	t.Run("queue unreadable", func(t *testing.T) {
		store, db := clipFixture(t)
		if _, err := db.Exec(`ALTER TABLE lidar_capture_jobs RENAME COLUMN queued_at_ns TO broken_queued_at`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.EnqueueClip("seg-1", "clip"); err == nil || !strings.Contains(err.Error(), "enqueue clip") {
			t.Fatalf("queue that cannot be written: %v", err)
		}
	})
	t.Run("active clip unreadable", func(t *testing.T) {
		store, db := clipFixture(t)
		if _, err := store.EnqueueClip("seg-1", "clip"); err != nil {
			t.Fatal(err)
		}
		// SQLite stores text in an integer column; a reader must not take
		// it for progress.
		if _, err := db.Exec(`UPDATE lidar_capture_jobs SET progress_current = 'half way'`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.EnqueueClip("seg-1", "asked again"); err == nil || !strings.Contains(err.Error(), "read active clip") {
			t.Fatalf("active clip that cannot be read: %v", err)
		}
	})
	t.Run("unlinked clip unreadable", func(t *testing.T) {
		store, db := clipFixture(t)
		job, err := store.EnqueueClip("seg-1", "clip")
		if err != nil {
			t.Fatal(err)
		}
		if err := NewCaptureStore(db).FinishJob(job.JobID, JobCompleted, ""); err != nil {
			t.Fatal(err)
		}
		// SQLite stores text in an integer column; a reader must not take
		// it for a time.
		if _, err := db.Exec(`UPDATE lidar_capture_jobs SET queued_at_ns = 'some time ago'`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.UnlinkedFinishedClips(); err == nil || !strings.Contains(err.Error(), "scan unlinked clip") {
			t.Fatalf("link that cannot be read: %v", err)
		}
	})
	t.Run("selection removed under its job", func(t *testing.T) {
		store, db := clipFixture(t)
		job, err := store.EnqueueClip("seg-1", "clip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM lidar_segment_selections WHERE segment_id='seg-1'`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ClipJob(job.JobID); err == nil || !strings.Contains(err.Error(), "clip job's selection") {
			t.Fatalf("job whose selection is gone: %v", err)
		}
	})
	t.Run("unreadable clip row", func(t *testing.T) {
		store, db := clipFixture(t)
		job, err := store.EnqueueClip("seg-1", "clip")
		if err != nil {
			t.Fatal(err)
		}
		if err := NewCaptureStore(db).FinishJob(job.JobID, JobCompleted, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE lidar_segment_clip_jobs SET segment_id = NULL`); err == nil {
			t.Fatal("a link to no segment was accepted")
		}
		if _, err := db.Exec(`ALTER TABLE lidar_segment_clip_jobs RENAME COLUMN pack_digest TO broken_digest`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.EnqueueClip("seg-2", "clip"); err != nil {
			t.Fatalf("queueing does not read the digest: %v", err)
		}
		if _, _, err := store.ClipJob(job.JobID); err == nil {
			t.Fatal("a clip row that could not be read was accepted")
		}
	})
}
