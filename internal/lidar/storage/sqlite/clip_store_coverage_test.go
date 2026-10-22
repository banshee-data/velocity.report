package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestClipStore_ListClips_All(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	// Insert two clips with different sensors
	s1 := &Clip{ClipID: "s1", SensorID: "sensor-a", Files: []ClipCapture{{PCAPFile: "a.pcap"}}}
	s2 := &Clip{ClipID: "s2", SensorID: "sensor-b", Files: []ClipCapture{{PCAPFile: "b.pcap"}}}
	if err := store.InsertClip(s1); err != nil {
		t.Fatalf("insert s1: %v", err)
	}
	if err := store.InsertClip(s2); err != nil {
		t.Fatalf("insert s2: %v", err)
	}

	// List all
	clips, err := store.ListClips("")
	if err != nil {
		t.Fatalf("ListClips all: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("expected 2 clips, got %d", len(clips))
	}

	// List by sensor
	clips, err = store.ListClips("sensor-a")
	if err != nil {
		t.Fatalf("ListClips filtered: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("expected 1 clip, got %d", len(clips))
	}
	if clips[0].SensorID != "sensor-a" {
		t.Fatalf("expected sensor-a, got %s", clips[0].SensorID)
	}
}

func TestClipStore_ListClips_WithOptionalFields(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	// Insert a clip with all optional fields
	startSecs := 10.0
	durSecs := 30.0
	s := &Clip{
		ClipID:   "full",
		SensorID: "sensor-c", Files: []ClipCapture{{PCAPFile: "c.pcap"}}, Description: "test clip",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"key":"value"}`),
	}
	if err := store.InsertClip(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Insert a reference run for FK
	db.Exec("INSERT INTO lidar_runs (run_id) VALUES ('run-1')")
	store.SetReferenceRun("full", "run-1")

	clips, err := store.ListClips("sensor-c")
	if err != nil {
		t.Fatalf("ListClips: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("expected 1, got %d", len(clips))
	}
	if clips[0].Description != "test clip" {
		t.Fatalf("description mismatch: %s", clips[0].Description)
	}
	if clips[0].ReferenceRunID != "run-1" {
		t.Fatalf("reference run mismatch: %s", clips[0].ReferenceRunID)
	}
}

func TestClipStore_UpdateClip_Success(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	s := &Clip{ClipID: "upd-1", SensorID: "sensor-a", Files: []ClipCapture{{PCAPFile: "a.pcap"}}}
	if err := store.InsertClip(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	startSecs := 5.0
	durSecs := 20.0
	update := &Clip{
		ClipID:            "upd-1",
		Description:       "updated",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"updated":true}`),
	}
	if err := store.UpdateClip(update); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetClip("upd-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "updated" {
		t.Fatalf("description not updated: %s", got.Description)
	}
}

func TestClipStore_UpdateClip_NotFound(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	err := store.UpdateClip(&Clip{ClipID: "nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "clip not found") {
		t.Fatalf("expected 'clip not found', got: %v", err)
	}
}

func TestClipStore_DeleteClip_Success(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	s := &Clip{ClipID: "del-1", SensorID: "sensor-a", Files: []ClipCapture{{PCAPFile: "a.pcap"}}}
	if err := store.InsertClip(s); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := store.DeleteClip("del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.GetClip("del-1")
	if err == nil || !strings.Contains(err.Error(), "clip not found") {
		t.Fatalf("expected not found after delete, got: %v", err)
	}
}

func TestClipStore_DeleteClip_NotFound(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	err := store.DeleteClip("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "clip not found") {
		t.Fatalf("expected 'clip not found', got: %v", err)
	}
}

func TestClipStore_SetReferenceRun_Success(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	s := &Clip{ClipID: "ref-1", SensorID: "sensor-a", Files: []ClipCapture{{PCAPFile: "a.pcap"}}}
	if err := store.InsertClip(s); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Exec("INSERT INTO lidar_runs (run_id) VALUES ('run-ref')")

	if err := store.SetReferenceRun("ref-1", "run-ref"); err != nil {
		t.Fatalf("set reference run: %v", err)
	}
	got, _ := store.GetClip("ref-1")
	if got.ReferenceRunID != "run-ref" {
		t.Fatalf("expected run-ref, got %s", got.ReferenceRunID)
	}

	// Clear reference run
	if err := store.SetReferenceRun("ref-1", ""); err != nil {
		t.Fatalf("clear reference run: %v", err)
	}
}

func TestClipStore_SetReferenceRun_NotFound(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	err := store.SetReferenceRun("nonexistent", "run-1")
	if err == nil || !strings.Contains(err.Error(), "clip not found") {
		t.Fatalf("expected 'clip not found', got: %v", err)
	}
}

func TestClipStore_SetOptimalParams_Success(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	s := &Clip{ClipID: "opt-1", SensorID: "sensor-a", Files: []ClipCapture{{PCAPFile: "a.pcap"}}}
	if err := store.InsertClip(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	params := json.RawMessage(`{"param1": 0.5, "param2": 10}`)
	if err := store.SetOptimalParams("opt-1", params); err != nil {
		t.Fatalf("set optimal params: %v", err)
	}
	got, _ := store.GetClip("opt-1")
	if got.RecommendedParamSetID == "" {
		t.Fatalf("expected recommended_param_set_id to be set after SetOptimalParams")
	}
}

func TestClipStore_SetOptimalParams_NotFound(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)

	err := store.SetOptimalParams("nonexistent", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "clip not found") {
		t.Fatalf("expected 'clip not found', got: %v", err)
	}
}

func TestClipStore_InsertClip_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	err := store.InsertClip(&Clip{ClipID: "x", SensorID: "s", Files: []ClipCapture{{PCAPFile: "f"}}})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_GetClip_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	_, err := store.GetClip("x")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_ListClips_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	_, err := store.ListClips("")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_UpdateClip_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	err := store.UpdateClip(&Clip{ClipID: "x"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_DeleteClip_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	err := store.DeleteClip("x")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_SetReferenceRun_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	err := store.SetReferenceRun("x", "y")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestClipStore_SetOptimalParams_DBClosed(t *testing.T) {
	db := setupTestClipDB(t)
	store := NewClipStore(db)
	db.Close()

	err := store.SetOptimalParams("x", nil)
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// --- collectClips: rows.Err() error path ---

// ccErrorRows is a mock clipRows that yields no rows but returns an
// error from Err() to exercise the rows.Err() branch in collectClips.
type ccErrorRows struct{ err error }

func (r *ccErrorRows) Next() bool          { return false }
func (r *ccErrorRows) Scan(_ ...any) error { return nil }
func (r *ccErrorRows) Err() error          { return r.err }

func TestCollectClips_RowsErr(t *testing.T) {
	sentinel := errors.New("iteration failed")
	rows := &ccErrorRows{err: sentinel}

	_, err := collectClips(rows, func(*Clip) {})
	if err == nil {
		t.Fatal("expected error from rows.Err()")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
}

func TestClipStoreCaptureFailuresDoNotHideOrLeaveRows(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()
	store := NewClipStore(db)
	// Invalid capture rows cannot leave a newly inserted clip behind.
	if err := store.InsertClip(&Clip{ClipID: "invalid-capture", SensorID: "sensor", Files: []ClipCapture{{PCAPFile: " "}}}); err == nil {
		t.Fatal("invalid capture accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_clips WHERE clip_id='invalid-capture'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed insert left %d rows: %v", count, err)
	}
	if err := store.InsertClip(&Clip{ClipID: "read-failure", SensorID: "sensor", Files: []ClipCapture{{PCAPFile: "valid.pcap"}}}); err != nil {
		t.Fatal(err)
	}
	broken := NewClipStore(&interceptSQLiteDB{db: db, queryFn: func(query string, args []any) (*sql.Rows, error) {
		if strings.Contains(query, "SELECT ordinal") {
			return nil, errors.New("capture rows unavailable")
		}
		return db.Query(query, args...)
	}})
	if _, err := broken.GetClip("read-failure"); err == nil || !strings.Contains(err.Error(), "capture rows unavailable") {
		t.Fatalf("GetClip hid capture read error: %v", err)
	}
	if _, err := broken.ListClips(""); err == nil || !strings.Contains(err.Error(), "capture rows unavailable") {
		t.Fatalf("ListClips hid capture read error: %v", err)
	}
}
