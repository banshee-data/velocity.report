package sqlite

import (
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSceneStore_ListScenes_All(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	// Insert two scenes with different sensors
	s1 := &Scene{SceneID: "s1", SensorID: "sensor-a", PCAPFile: "a.pcap"}
	s2 := &Scene{SceneID: "s2", SensorID: "sensor-b", PCAPFile: "b.pcap"}
	if err := store.InsertScene(s1); err != nil {
		t.Fatalf("insert s1: %v", err)
	}
	if err := store.InsertScene(s2); err != nil {
		t.Fatalf("insert s2: %v", err)
	}

	// List all
	scenes, err := store.ListScenes("")
	if err != nil {
		t.Fatalf("ListScenes all: %v", err)
	}
	if len(scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(scenes))
	}

	// List by sensor
	scenes, err = store.ListScenes("sensor-a")
	if err != nil {
		t.Fatalf("ListScenes filtered: %v", err)
	}
	if len(scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(scenes))
	}
	if scenes[0].SensorID != "sensor-a" {
		t.Fatalf("expected sensor-a, got %s", scenes[0].SensorID)
	}
}

func TestSceneStore_ListScenes_WithOptionalFields(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	// Insert a scene with all optional fields
	startSecs := 10.0
	durSecs := 30.0
	s := &Scene{
		SceneID:           "full",
		SensorID:          "sensor-c",
		PCAPFile:          "c.pcap",
		Description:       "test scene",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"key":"value"}`),
	}
	if err := store.InsertScene(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Insert a reference run for FK
	db.Exec("INSERT INTO lidar_analysis_runs (run_id) VALUES ('run-1')")
	store.SetReferenceRun("full", "run-1")

	scenes, err := store.ListScenes("sensor-c")
	if err != nil {
		t.Fatalf("ListScenes: %v", err)
	}
	if len(scenes) != 1 {
		t.Fatalf("expected 1, got %d", len(scenes))
	}
	if scenes[0].Description != "test scene" {
		t.Fatalf("description mismatch: %s", scenes[0].Description)
	}
	if scenes[0].ReferenceRunID != "run-1" {
		t.Fatalf("reference run mismatch: %s", scenes[0].ReferenceRunID)
	}
}

func TestSceneStore_UpdateScene_Success(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	s := &Scene{SceneID: "upd-1", SensorID: "sensor-a", PCAPFile: "a.pcap"}
	if err := store.InsertScene(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	startSecs := 5.0
	durSecs := 20.0
	update := &Scene{
		SceneID:           "upd-1",
		Description:       "updated",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"updated":true}`),
	}
	if err := store.UpdateScene(update); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetScene("upd-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "updated" {
		t.Fatalf("description not updated: %s", got.Description)
	}
}

func TestSceneStore_UpdateScene_NotFound(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	err := store.UpdateScene(&Scene{SceneID: "nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "scene not found") {
		t.Fatalf("expected 'scene not found', got: %v", err)
	}
}

func TestSceneStore_DeleteScene_Success(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	s := &Scene{SceneID: "del-1", SensorID: "sensor-a", PCAPFile: "a.pcap"}
	if err := store.InsertScene(s); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := store.DeleteScene("del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.GetScene("del-1")
	if err == nil || !strings.Contains(err.Error(), "scene not found") {
		t.Fatalf("expected not found after delete, got: %v", err)
	}
}

func TestSceneStore_DeleteScene_NotFound(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	err := store.DeleteScene("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "scene not found") {
		t.Fatalf("expected 'scene not found', got: %v", err)
	}
}

func TestSceneStore_SetReferenceRun_Success(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	s := &Scene{SceneID: "ref-1", SensorID: "sensor-a", PCAPFile: "a.pcap"}
	if err := store.InsertScene(s); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Exec("INSERT INTO lidar_analysis_runs (run_id) VALUES ('run-ref')")

	if err := store.SetReferenceRun("ref-1", "run-ref"); err != nil {
		t.Fatalf("set reference run: %v", err)
	}
	got, _ := store.GetScene("ref-1")
	if got.ReferenceRunID != "run-ref" {
		t.Fatalf("expected run-ref, got %s", got.ReferenceRunID)
	}

	// Clear reference run
	if err := store.SetReferenceRun("ref-1", ""); err != nil {
		t.Fatalf("clear reference run: %v", err)
	}
}

func TestSceneStore_SetReferenceRun_NotFound(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	err := store.SetReferenceRun("nonexistent", "run-1")
	if err == nil || !strings.Contains(err.Error(), "scene not found") {
		t.Fatalf("expected 'scene not found', got: %v", err)
	}
}

func TestSceneStore_SetOptimalParams_Success(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	s := &Scene{SceneID: "opt-1", SensorID: "sensor-a", PCAPFile: "a.pcap"}
	if err := store.InsertScene(s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	params := json.RawMessage(`{"param1": 0.5, "param2": 10}`)
	if err := store.SetOptimalParams("opt-1", params); err != nil {
		t.Fatalf("set optimal params: %v", err)
	}
	got, _ := store.GetScene("opt-1")
	if string(got.OptimalParamsJSON) != string(params) {
		t.Fatalf("params mismatch: %s", string(got.OptimalParamsJSON))
	}
}

func TestSceneStore_SetOptimalParams_NotFound(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()
	store := NewSceneStore(db)

	err := store.SetOptimalParams("nonexistent", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "scene not found") {
		t.Fatalf("expected 'scene not found', got: %v", err)
	}
}

func TestSceneStore_InsertScene_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	err := store.InsertScene(&Scene{SceneID: "x", SensorID: "s", PCAPFile: "f"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_GetScene_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	_, err := store.GetScene("x")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_ListScenes_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	_, err := store.ListScenes("")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_UpdateScene_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	err := store.UpdateScene(&Scene{SceneID: "x"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_DeleteScene_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	err := store.DeleteScene("x")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_SetReferenceRun_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	err := store.SetReferenceRun("x", "y")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

func TestSceneStore_SetOptimalParams_DBClosed(t *testing.T) {
	db := setupTestSceneDB(t)
	store := NewSceneStore(db)
	db.Close()

	err := store.SetOptimalParams("x", nil)
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}
