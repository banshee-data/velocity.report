package sqlite

import (
	"database/sql"
	"testing"
	"time"
)

// The pipeline offers RecordTrack every confirmed track on every frame. These
// tests offer a track the way the pipeline does: first as it looks when it is
// confirmed, a few observations old and not yet classified, then again as it
// grows. The older RecordTrack test offered the same state twice, so a manager
// that kept only the first offer passed it.

const trackBirthNanos int64 = 1_700_000_000_000_000_000

// trackAtAge is a track as the tracker reports it `obs` observations into its
// life at 10 Hz: older means longer-lived, faster, larger and, once it has
// enough history, classified.
func trackAtAge(id string, obs int) *TrackedObject {
	m := TrackMeasurement{
		SensorID:             "test-sensor",
		TrackState:           TrackConfirmed,
		StartUnixNanos:       trackBirthNanos,
		EndUnixNanos:         trackBirthNanos + int64(obs)*100_000_000,
		ObservationCount:     obs,
		AvgSpeedMps:          float32(obs) / 10,
		MaxSpeedMps:          float32(obs) / 5,
		BoundingBoxLengthAvg: 1 + float32(obs)/20,
	}
	if obs >= 20 {
		m.ObjectClass = "car"
		m.ObjectConfidence = 0.9
		m.ClassificationModel = "rule-based-v1"
	}
	return &TrackedObject{TrackID: id, TrackMeasurement: m}
}

type storedRunTrack struct {
	obs        int
	endNanos   int64
	maxSpeed   float64
	length     float64
	class      sql.NullString
	confidence sql.NullFloat64
}

func readStoredRunTrack(t *testing.T, db *sql.DB, runID, trackID string) storedRunTrack {
	t.Helper()
	var got storedRunTrack
	err := db.QueryRow(`
		SELECT observation_count, end_unix_nanos, max_speed_mps, bounding_box_length_avg,
		       object_class, object_confidence
		FROM lidar_run_tracks WHERE run_id = ? AND track_id = ?`, runID, trackID).
		Scan(&got.obs, &got.endNanos, &got.maxSpeed, &got.length, &got.class, &got.confidence)
	if err != nil {
		t.Fatalf("read run track %s: %v", trackID, err)
	}
	return got
}

func startTestRun(t *testing.T, db *sql.DB) (*AnalysisRunManager, string) {
	t.Helper()
	manager := NewAnalysisRunManagerDI(db, "test-sensor")
	runID, err := manager.StartRun("/path/to/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	return manager, runID
}

func TestCompleteRunStoresTheFinishedTrackNotItsFirstSighting(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID := startTestRun(t, db)

	for _, obs := range []int{3, 4, 5, 20, 40, 60} {
		manager.RecordTrack(trackAtAge("track-001", obs))
	}
	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got := readStoredRunTrack(t, db, runID, "track-001")
	if got.obs != 60 {
		t.Errorf("observation_count = %d, want 60: the row still describes the track's first sighting", got.obs)
	}
	if want := trackBirthNanos + 60*100_000_000; got.endNanos != want {
		t.Errorf("end_unix_nanos = %d, want %d: a six-second track was stored as %.1f s long",
			got.endNanos, want, float64(got.endNanos-trackBirthNanos)/1e9)
	}
	if got.maxSpeed != 12 {
		t.Errorf("max_speed_mps = %v, want 12", got.maxSpeed)
	}
	if got.length != 4 {
		t.Errorf("bounding_box_length_avg = %v, want 4", got.length)
	}
	if got.class.String != "car" {
		t.Errorf("object_class = %q, want car: classification arrives after the first sighting and was never stored", got.class.String)
	}
}

// A track that dies mid-run stops being offered. What was last offered is the
// best description of it there will ever be.
func TestATrackThatDiesMidRunKeepsItsLastOfferedState(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID := startTestRun(t, db)

	for obs := 3; obs <= 30; obs++ {
		manager.RecordTrack(trackAtAge("short-lived", obs))
	}
	for obs := 3; obs <= 80; obs++ {
		manager.RecordTrack(trackAtAge("long-lived", obs))
	}
	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	if got := readStoredRunTrack(t, db, runID, "short-lived"); got.obs != 30 {
		t.Errorf("short-lived observation_count = %d, want 30", got.obs)
	}
	if got := readStoredRunTrack(t, db, runID, "long-lived"); got.obs != 80 {
		t.Errorf("long-lived observation_count = %d, want 80", got.obs)
	}
}

// Rows exist from the first sighting so a track can be labelled while its run
// is still going. Bringing the measurements up to date must not take the
// label with it: a label is a person's work, and a measurement is not.
func TestUpdatingMeasurementsLeavesHumanLabelsAlone(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID := startTestRun(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))

	if _, err := db.Exec(`
		UPDATE lidar_run_tracks
		SET user_label = 'bus', label_confidence = 0.75, labeler_id = 'david',
		    labeled_at = 1700000123, quality_label = 'good', label_source = 'human_manual',
		    linked_track_ids = '["track-009"]', is_split_candidate = 1, is_merge_candidate = 1
		WHERE run_id = ? AND track_id = 'track-001'`, runID); err != nil {
		t.Fatalf("label the track: %v", err)
	}

	for _, obs := range []int{20, 60} {
		manager.RecordTrack(trackAtAge("track-001", obs))
	}
	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	var (
		userLabel, labelerID, qualityLabel, labelSource, linked string
		labelConfidence                                         float64
		labeledAt                                               int64
		split, merge                                            int
	)
	if err := db.QueryRow(`
		SELECT user_label, label_confidence, labeler_id, labeled_at, quality_label,
		       label_source, linked_track_ids, is_split_candidate, is_merge_candidate
		FROM lidar_run_tracks WHERE run_id = ? AND track_id = 'track-001'`, runID).
		Scan(&userLabel, &labelConfidence, &labelerID, &labeledAt, &qualityLabel,
			&labelSource, &linked, &split, &merge); err != nil {
		t.Fatalf("read labels: %v", err)
	}
	if userLabel != "bus" || labelConfidence != 0.75 || labelerID != "david" ||
		labeledAt != 1700000123 || qualityLabel != "good" || labelSource != "human_manual" ||
		linked != `["track-009"]` || split != 1 || merge != 1 {
		t.Errorf("labels changed: user_label=%q confidence=%v labeler=%q labeled_at=%d quality=%q source=%q linked=%q split=%d merge=%d",
			userLabel, labelConfidence, labelerID, labeledAt, qualityLabel, labelSource, linked, split, merge)
	}

	// The operator called it a bus; the classifier's "car" sits beside that
	// in its own column, as it always has.
	got := readStoredRunTrack(t, db, runID, "track-001")
	if got.obs != 60 || got.class.String != "car" {
		t.Errorf("measurements not updated beside the label: observation_count=%d object_class=%q", got.obs, got.class.String)
	}
}

func TestAFailedRunStillStoresWhatItsTracksBecame(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID := startTestRun(t, db)

	for _, obs := range []int{3, 45} {
		manager.RecordTrack(trackAtAge("track-001", obs))
	}
	if err := manager.FailRun("replay aborted"); err != nil {
		t.Fatalf("FailRun: %v", err)
	}

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 45 {
		t.Errorf("observation_count = %d, want 45: a run that fails late has still measured its tracks", got.obs)
	}
}

// manualClock is a clock the test moves, so the flush interval is crossed by
// saying so rather than by sleeping through it.
type manualClock struct{ t time.Time }

func (c *manualClock) now() time.Time          { return c.t }
func (c *manualClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func startTestRunWithClock(t *testing.T, db *sql.DB) (*AnalysisRunManager, string, *manualClock) {
	t.Helper()
	clock := &manualClock{t: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)}
	manager := NewAnalysisRunManagerDI(db, "test-sensor")
	manager.now = clock.now
	runID, err := manager.StartRun("/path/to/test.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	return manager, runID, clock
}

// A live run can last for days and end with the power. If rows were only
// brought up to date when a run completes, every row of such a run would be
// left describing a first sighting.
func TestMeasurementsReachTheDatabaseWhileTheRunIsStillGoing(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID, clock := startTestRunWithClock(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))
	manager.RecordTrack(trackAtAge("track-001", 40))
	clock.advance(runTrackFlushInterval)
	manager.RecordTrack(trackAtAge("track-001", 41))

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 41 {
		t.Errorf("observation_count = %d mid-run, want 41: nothing was written until the run ended", got.obs)
	}
	if !manager.IsRunActive() {
		t.Error("the run should still be active: this is a mid-run flush, not a completion")
	}
}

// The other half of the interval: measurements are batched, not written on
// every frame. At 10 Hz with dozens of tracks that would be hundreds of
// writes a second for values that are about to be replaced.
func TestMeasurementsAreNotWrittenOnEveryFrame(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID, clock := startTestRunWithClock(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))
	clock.advance(runTrackFlushInterval - time.Millisecond)
	manager.RecordTrack(trackAtAge("track-001", 40))

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 3 {
		t.Errorf("observation_count = %d before the interval had passed, want the inserted 3", got.obs)
	}
}

// When the last tracks in view die, nothing offers a track again. Frames keep
// arriving, and they have to be enough to get the final measurements written.
func TestFramesAloneFlushTheLastTracksOfAQuietScene(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID, clock := startTestRunWithClock(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))
	manager.RecordTrack(trackAtAge("track-001", 55))
	clock.advance(runTrackFlushInterval)
	manager.RecordFrame(trackBirthNanos + 9_000_000_000)

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 55 {
		t.Errorf("observation_count = %d, want 55: an empty scene left the last track unwritten", got.obs)
	}
}

func TestAFailedWriteIsRetriedWithNothingLost(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, runID, clock := startTestRunWithClock(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))
	manager.RecordTrack(trackAtAge("track-001", 30))

	// Take the table away so the flush fails, then put it back.
	if _, err := db.Exec(`ALTER TABLE lidar_run_tracks RENAME TO lidar_run_tracks_away`); err != nil {
		t.Fatalf("rename table away: %v", err)
	}
	clock.advance(runTrackFlushInterval)
	manager.RecordFrame(trackBirthNanos + 3_000_000_000)
	if _, err := db.Exec(`ALTER TABLE lidar_run_tracks_away RENAME TO lidar_run_tracks`); err != nil {
		t.Fatalf("rename table back: %v", err)
	}

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 3 {
		t.Fatalf("observation_count = %d after a failed flush, want the inserted 3", got.obs)
	}

	clock.advance(runTrackFlushInterval)
	manager.RecordFrame(trackBirthNanos + 4_000_000_000)

	if got := readStoredRunTrack(t, db, runID, "track-001"); got.obs != 30 {
		t.Errorf("observation_count = %d, want 30: measurements pending at a failed flush were dropped instead of retried", got.obs)
	}
}

func TestANewRunDoesNotInheritTheLastRunsPendingTracks(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	manager, firstRun, _ := startTestRunWithClock(t, db)

	manager.RecordTrack(trackAtAge("track-001", 3))
	manager.RecordTrack(trackAtAge("track-001", 30))
	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	secondRun, err := manager.StartRun("/path/to/other.pcap", DefaultRunParams())
	if err != nil {
		t.Fatalf("second StartRun: %v", err)
	}
	// The same track ID in a new run is a new track: it is inserted afresh.
	if isNew := manager.RecordTrack(trackAtAge("track-001", 4)); !isNew {
		t.Error("a track ID seen in the previous run was not treated as new in this one")
	}
	if err := manager.CompleteRun(); err != nil {
		t.Fatalf("second CompleteRun: %v", err)
	}

	if got := readStoredRunTrack(t, db, firstRun, "track-001"); got.obs != 30 {
		t.Errorf("first run observation_count = %d, want 30", got.obs)
	}
	if got := readStoredRunTrack(t, db, secondRun, "track-001"); got.obs != 4 {
		t.Errorf("second run observation_count = %d, want 4", got.obs)
	}
}

func TestUpdateRunTrackMeasurementsNeverInsertsARow(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()
	_, runID := startTestRun(t, db)
	store := NewAnalysisRunStore(db)

	if err := store.UpdateRunTrackMeasurements(runID, nil); err != nil {
		t.Errorf("an empty update should be a no-op, got %v", err)
	}
	err := store.UpdateRunTrackMeasurements(runID, map[string]TrackMeasurement{
		"never-inserted": trackAtAge("never-inserted", 50).TrackMeasurement,
	})
	if err != nil {
		t.Fatalf("updating a track with no row: %v", err)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_run_tracks WHERE run_id = ?`, runID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d row(s) appeared: inserting is InsertRunTrack's job, with the label defaults it owns", rows)
	}
}
