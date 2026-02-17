package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestEvaluationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create referenced tables (FK targets)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
			run_id TEXT PRIMARY KEY,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_analysis_runs table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_scenes (
			scene_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_file TEXT NOT NULL,
			created_at_ns INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_scenes table: %v", err)
	}

	// Create lidar_evaluations table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_evaluations (
			evaluation_id       TEXT PRIMARY KEY,
			scene_id            TEXT NOT NULL,
			reference_run_id    TEXT NOT NULL,
			candidate_run_id    TEXT NOT NULL,
			detection_rate      REAL,
			fragmentation       REAL,
			false_positive_rate REAL,
			velocity_coverage   REAL,
			quality_premium     REAL,
			truncation_rate     REAL,
			velocity_noise_rate REAL,
			stopped_recovery_rate REAL,
			composite_score     REAL,
			matched_count       INTEGER,
			reference_count     INTEGER,
			candidate_count     INTEGER,
			params_json         TEXT,
			created_at          INTEGER NOT NULL,
			FOREIGN KEY (scene_id) REFERENCES lidar_scenes(scene_id) ON DELETE CASCADE,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id),
			FOREIGN KEY (candidate_run_id) REFERENCES lidar_analysis_runs(run_id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_pair ON lidar_evaluations(reference_run_id, candidate_run_id);
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_evaluations table: %v", err)
	}

	return db
}

func seedEvaluationTestData(t *testing.T, db *sql.DB) {
	t.Helper()
	// Insert referenced rows
	_, err := db.Exec(`INSERT INTO lidar_scenes (scene_id, sensor_id, pcap_file, created_at_ns) VALUES ('scene-1', 'sensor-1', 'test.pcap', 1000)`)
	if err != nil {
		t.Fatalf("failed to seed scene: %v", err)
	}
	for _, runID := range []string{"ref-run-1", "cand-run-1", "cand-run-2"} {
		_, err := db.Exec(`INSERT INTO lidar_analysis_runs (run_id) VALUES (?)`, runID)
		if err != nil {
			t.Fatalf("failed to seed run %s: %v", runID, err)
		}
	}
}

func TestEvaluationStore_InsertAndGet(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()
	seedEvaluationTestData(t, db)

	store := NewEvaluationStore(db)

	eval := &Evaluation{
		SceneID:             "scene-1",
		ReferenceRunID:      "ref-run-1",
		CandidateRunID:      "cand-run-1",
		DetectionRate:       0.92,
		Fragmentation:       0.05,
		FalsePositiveRate:   0.03,
		VelocityCoverage:    0.87,
		QualityPremium:      0.82,
		TruncationRate:      0.1,
		VelocityNoiseRate:   0.08,
		StoppedRecoveryRate: 0.6,
		CompositeScore:      0.874,
		MatchedCount:        12,
		ReferenceCount:      13,
		CandidateCount:      15,
		ParamsJSON:          json.RawMessage(`{"dist": 2.5}`),
	}

	err := store.Insert(eval)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if eval.EvaluationID == "" {
		t.Error("expected evaluation_id to be generated")
	}
	if eval.CreatedAt == 0 {
		t.Error("expected created_at to be set")
	}

	// Retrieve
	retrieved, err := store.Get(eval.EvaluationID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.SceneID != "scene-1" {
		t.Errorf("scene_id mismatch: got %s, want scene-1", retrieved.SceneID)
	}
	if retrieved.DetectionRate != 0.92 {
		t.Errorf("detection_rate mismatch: got %f, want 0.92", retrieved.DetectionRate)
	}
	if retrieved.CompositeScore != 0.874 {
		t.Errorf("composite_score mismatch: got %f, want 0.874", retrieved.CompositeScore)
	}
	if retrieved.MatchedCount != 12 {
		t.Errorf("matched_count mismatch: got %d, want 12", retrieved.MatchedCount)
	}
	if string(retrieved.ParamsJSON) != `{"dist": 2.5}` {
		t.Errorf("params_json mismatch: got %s", string(retrieved.ParamsJSON))
	}
}

func TestEvaluationStore_ListByScene(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()
	seedEvaluationTestData(t, db)

	store := NewEvaluationStore(db)

	// Insert two evaluations for the same scene
	eval1 := &Evaluation{
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run-1",
		CandidateRunID: "cand-run-1",
		CompositeScore: 0.874,
		MatchedCount:   12,
		ReferenceCount: 13,
		CandidateCount: 15,
	}
	eval2 := &Evaluation{
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run-1",
		CandidateRunID: "cand-run-2",
		CompositeScore: 0.841,
		MatchedCount:   11,
		ReferenceCount: 13,
		CandidateCount: 14,
	}

	if err := store.Insert(eval1); err != nil {
		t.Fatalf("Insert eval1 failed: %v", err)
	}
	if err := store.Insert(eval2); err != nil {
		t.Fatalf("Insert eval2 failed: %v", err)
	}

	evals, err := store.ListByScene("scene-1")
	if err != nil {
		t.Fatalf("ListByScene failed: %v", err)
	}
	if len(evals) != 2 {
		t.Fatalf("expected 2 evaluations, got %d", len(evals))
	}

	// Empty scene should return nil
	empty, err := store.ListByScene("nonexistent")
	if err != nil {
		t.Fatalf("ListByScene for nonexistent failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 evaluations for nonexistent scene, got %d", len(empty))
	}
}

func TestEvaluationStore_Delete(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()
	seedEvaluationTestData(t, db)

	store := NewEvaluationStore(db)

	eval := &Evaluation{
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run-1",
		CandidateRunID: "cand-run-1",
		CompositeScore: 0.5,
	}
	if err := store.Insert(eval); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if err := store.Delete(eval.EvaluationID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Get(eval.EvaluationID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestEvaluationStore_GetNotFound(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()

	store := NewEvaluationStore(db)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent evaluation, got nil")
	}
}

func TestEvaluationStore_DeleteNotFound(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()

	store := NewEvaluationStore(db)

	err := store.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent evaluation, got nil")
	}
}

func TestEvaluationStore_UniqueConstraint(t *testing.T) {
	db := setupTestEvaluationDB(t)
	defer db.Close()
	seedEvaluationTestData(t, db)

	store := NewEvaluationStore(db)

	eval1 := &Evaluation{
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run-1",
		CandidateRunID: "cand-run-1",
		CompositeScore: 0.5,
	}
	if err := store.Insert(eval1); err != nil {
		t.Fatalf("first Insert failed: %v", err)
	}

	// Same (reference_run_id, candidate_run_id) pair should fail
	eval2 := &Evaluation{
		SceneID:        "scene-1",
		ReferenceRunID: "ref-run-1",
		CandidateRunID: "cand-run-1",
		CompositeScore: 0.6,
	}
	err := store.Insert(eval2)
	if err == nil {
		t.Error("expected unique constraint violation, got nil")
	}
}
