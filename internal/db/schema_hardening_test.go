package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func insertTestRunRecord(t *testing.T, db *DB, runID, sensorID, status string, parentRunID *string) {
	t.Helper()

	_, err := db.Exec(
		`INSERT INTO lidar_run_records (
			run_id, created_at, source_type, sensor_id, params_json, status, parent_run_id
		) VALUES (?, ?, 'pcap', ?, '{}', ?, ?)`,
		runID,
		time.Now().UnixNano(),
		sensorID,
		status,
		parentRunID,
	)
	if err != nil {
		t.Fatalf("failed to insert run %s: %v", runID, err)
	}
}

func TestCreateSiteReport_WithNoSiteStoresNullSiteID(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	report := &SiteReport{
		SiteID:    0,
		StartDate: "2026-03-01",
		EndDate:   "2026-03-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-site-null",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}

	if err := db.CreateSiteReport(context.Background(), report); err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	var siteIDIsNull int
	if err := db.QueryRow("SELECT site_id IS NULL FROM site_reports WHERE id = ?", report.ID).Scan(&siteIDIsNull); err != nil {
		t.Fatalf("failed to inspect stored site_id: %v", err)
	}
	if siteIDIsNull != 1 {
		t.Fatalf("expected site_id to be stored as NULL, got %d", siteIDIsNull)
	}

	retrieved, err := db.GetSiteReport(context.Background(), report.ID)
	if err != nil {
		t.Fatalf("GetSiteReport failed: %v", err)
	}
	if retrieved.SiteID != 0 {
		t.Fatalf("expected SiteID to round-trip as 0, got %d", retrieved.SiteID)
	}
}

func TestReplayCaseDeleteCascadesReplayAnnotationsAndEvaluations(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	insertTestRunRecord(t, db, "ref-run", "sensor-1", "completed", nil)
	insertTestRunRecord(t, db, "cand-run", "sensor-1", "completed", nil)

	if _, err := db.Exec(`INSERT INTO lidar_replay_cases (
		replay_case_id, sensor_id, pcap_file, reference_run_id, created_at_ns
	) VALUES ('case-1', 'sensor-1', '/tmp/test.pcap', 'ref-run', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert replay case: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_run_tracks (
		run_id, track_id, sensor_id, track_state, start_unix_nanos
	) VALUES ('ref-run', 'track-1', 'sensor-1', 'confirmed', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert run track: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_replay_annotations (
		annotation_id, replay_case_id, run_id, track_id, class_label, start_timestamp_ns, created_at_ns
	) VALUES ('ann-1', 'case-1', 'ref-run', 'track-1', 'car', ?, ?)`, time.Now().UnixNano(), time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert replay annotation: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_replay_evaluations (
		evaluation_id, replay_case_id, reference_run_id, candidate_run_id, created_at
	) VALUES ('eval-1', 'case-1', 'ref-run', 'cand-run', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert evaluation: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM lidar_replay_cases WHERE replay_case_id = 'case-1'`); err != nil {
		t.Fatalf("failed to delete replay case: %v", err)
	}

	var annotationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_replay_annotations WHERE annotation_id = 'ann-1'`).Scan(&annotationCount); err != nil {
		t.Fatalf("failed to query replay annotations: %v", err)
	}
	if annotationCount != 0 {
		t.Fatalf("expected replay annotation to be deleted, found %d", annotationCount)
	}

	var evaluationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_replay_evaluations WHERE evaluation_id = 'eval-1'`).Scan(&evaluationCount); err != nil {
		t.Fatalf("failed to query replay evaluations: %v", err)
	}
	if evaluationCount != 0 {
		t.Fatalf("expected replay evaluation to be deleted, found %d", evaluationCount)
	}
}

func TestRunDeleteNullsParentAndReferenceAndDropsRunScopedEvaluationLinks(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	insertTestRunRecord(t, db, "parent-run", "sensor-1", "completed", nil)
	parentID := "parent-run"
	insertTestRunRecord(t, db, "child-run", "sensor-1", "completed", &parentID)
	insertTestRunRecord(t, db, "cand-run", "sensor-1", "completed", nil)

	if _, err := db.Exec(`INSERT INTO lidar_replay_cases (
		replay_case_id, sensor_id, pcap_file, reference_run_id, created_at_ns
	) VALUES ('case-2', 'sensor-1', '/tmp/test.pcap', 'parent-run', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert replay case: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_run_tracks (
		run_id, track_id, sensor_id, track_state, start_unix_nanos
	) VALUES ('parent-run', 'track-1', 'sensor-1', 'confirmed', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert run track: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_replay_annotations (
		annotation_id, replay_case_id, run_id, track_id, class_label, start_timestamp_ns, created_at_ns
	) VALUES ('ann-2', 'case-2', 'parent-run', 'track-1', 'car', ?, ?)`, time.Now().UnixNano(), time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert replay annotation: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_replay_evaluations (
		evaluation_id, replay_case_id, reference_run_id, candidate_run_id, created_at
	) VALUES ('eval-2', 'case-2', 'parent-run', 'cand-run', ?)`, time.Now().UnixNano()); err != nil {
		t.Fatalf("failed to insert evaluation: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM lidar_run_records WHERE run_id = 'parent-run'`); err != nil {
		t.Fatalf("failed to delete parent run: %v", err)
	}

	var childParent sql.NullString
	if err := db.QueryRow(`SELECT parent_run_id FROM lidar_run_records WHERE run_id = 'child-run'`).Scan(&childParent); err != nil {
		t.Fatalf("failed to query child run: %v", err)
	}
	if childParent.Valid {
		t.Fatalf("expected child parent_run_id to be NULL, got %q", childParent.String)
	}

	var replayRef sql.NullString
	if err := db.QueryRow(`SELECT reference_run_id FROM lidar_replay_cases WHERE replay_case_id = 'case-2'`).Scan(&replayRef); err != nil {
		t.Fatalf("failed to query replay case: %v", err)
	}
	if replayRef.Valid {
		t.Fatalf("expected replay case reference_run_id to be NULL, got %q", replayRef.String)
	}

	var annotationRunID, annotationTrackID sql.NullString
	if err := db.QueryRow(`SELECT run_id, track_id FROM lidar_replay_annotations WHERE annotation_id = 'ann-2'`).Scan(&annotationRunID, &annotationTrackID); err != nil {
		t.Fatalf("failed to query replay annotation: %v", err)
	}
	if annotationRunID.Valid || annotationTrackID.Valid {
		t.Fatalf("expected replay annotation run/track linkage to be cleared, got run=%q track=%q", annotationRunID.String, annotationTrackID.String)
	}

	var evaluationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_replay_evaluations WHERE evaluation_id = 'eval-2'`).Scan(&evaluationCount); err != nil {
		t.Fatalf("failed to query replay evaluations: %v", err)
	}
	if evaluationCount != 0 {
		t.Fatalf("expected replay evaluation to be deleted, found %d", evaluationCount)
	}
}

func TestTransitWorkerPersistsLinksWithForeignKeysEnabled(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	now := float64(time.Now().Unix())
	for _, ts := range []float64{now, now + 1, now + 2} {
		if _, err := db.Exec(
			`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
			ts,
			`{"speed": 12.5, "magnitude": 42}`,
		); err != nil {
			t.Fatalf("failed to insert radar data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 5, "test-model")
	if err := worker.RunRange(context.Background(), now-1, now+10); err != nil {
		t.Fatalf("RunRange failed: %v", err)
	}

	var transitCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits`).Scan(&transitCount); err != nil {
		t.Fatalf("failed to query transits: %v", err)
	}
	if transitCount == 0 {
		t.Fatal("expected at least one transit")
	}

	var linkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_transit_links`).Scan(&linkCount); err != nil {
		t.Fatalf("failed to query transit links: %v", err)
	}
	if linkCount == 0 {
		t.Fatal("expected at least one transit link")
	}
}
