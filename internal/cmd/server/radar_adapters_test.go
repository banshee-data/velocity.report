package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// These adapters exist purely to bridge package types across an import cycle
// (lidar <-> visualiser, sqlite <-> sweep). They are thin, but a wrong field
// mapping here silently corrupts what the visualiser and the HINT tuner see,
// so each one is checked for faithful translation rather than just for running.

func newTestBackgroundManager(t *testing.T) *l3grid.BackgroundManager {
	t.Helper()
	database, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)

	mgr := l3grid.NewBackgroundManager("test-sensor", 40, 1800,
		l3grid.DefaultBackgroundConfig().ToBackgroundParams(), database)
	if mgr == nil {
		t.Fatal("NewBackgroundManager returned nil")
	}
	return mgr
}

func TestBackgroundManagerBridgeSequenceNumber(t *testing.T) {
	mgr := newTestBackgroundManager(t)
	bridge := &backgroundManagerBridge{mgr: mgr}

	// The bridge must report the manager's own sequence, not a copy that can
	// drift: the visualiser uses it to decide whether to resend a snapshot.
	if got, want := bridge.GetBackgroundSequenceNumber(), mgr.GetBackgroundSequenceNumber(); got != want {
		t.Errorf("GetBackgroundSequenceNumber() = %d, want %d", got, want)
	}
}

func TestBackgroundManagerBridgeSnapshotPropagatesError(t *testing.T) {
	// A manager with no ring elevations configured cannot produce a snapshot.
	// The bridge must surface that error rather than returning a half-built
	// snapshot the publisher would send to the visualiser.
	mgr := newTestBackgroundManager(t)
	bridge := &backgroundManagerBridge{mgr: mgr}

	got, err := bridge.GenerateBackgroundSnapshot()
	if err == nil {
		t.Fatal("GenerateBackgroundSnapshot without ring elevations succeeded, want an error")
	}
	if got != nil {
		t.Errorf("snapshot = %#v, want nil alongside the error", got)
	}
}

func TestBackgroundManagerBridgeTranslatesSnapshotFields(t *testing.T) {
	mgr := newTestBackgroundManager(t)
	elevations := make([]float64, 40)
	for i := range elevations {
		elevations[i] = float64(i)*0.5 - 10
	}
	if err := mgr.SetRingElevations(elevations); err != nil {
		t.Fatalf("SetRingElevations: %v", err)
	}
	bridge := &backgroundManagerBridge{mgr: mgr}

	raw, err := bridge.GenerateBackgroundSnapshot()
	if err != nil {
		t.Fatalf("GenerateBackgroundSnapshot: %v", err)
	}
	got, ok := raw.(*l9endpoints.BackgroundSnapshot)
	if !ok {
		t.Fatalf("snapshot type = %T, want *l9endpoints.BackgroundSnapshot", raw)
	}

	// The bridge exists to convert l3grid's snapshot into the visualiser's
	// type. A mis-mapped grid dimension would silently distort the rendered
	// point cloud, so the geometry metadata is checked field by field.
	if got.GridMetadata.Rings != 40 {
		t.Errorf("Rings = %d, want 40", got.GridMetadata.Rings)
	}
	if got.GridMetadata.AzimuthBins != 1800 {
		t.Errorf("AzimuthBins = %d, want 1800", got.GridMetadata.AzimuthBins)
	}
	if len(got.GridMetadata.RingElevations) != len(elevations) {
		t.Fatalf("RingElevations length = %d, want %d",
			len(got.GridMetadata.RingElevations), len(elevations))
	}
	for i, want := range elevations {
		if float64(got.GridMetadata.RingElevations[i]) != want {
			t.Errorf("RingElevations[%d] = %v, want %v",
				i, got.GridMetadata.RingElevations[i], want)
		}
	}
	// A grid that has seen no frames is not settled, and that must be
	// reported honestly — the visualiser dims an unsettled background.
	if got.GridMetadata.SettlingComplete {
		t.Error("SettlingComplete = true on a grid that has processed no frames")
	}
	if got.TimestampNanos == 0 {
		t.Error("TimestampNanos = 0, want the snapshot time carried across")
	}
}

// erroringSweepBackend satisfies sweep.SweepBackend and fails every operation,
// so a sweep run aborts immediately instead of replaying a capture. It is
// enough to drive hintRunCreator's request-construction path.
type erroringSweepBackend struct{}

func (erroringSweepBackend) SensorID() string       { return "test-sensor" }
func (erroringSweepBackend) FetchBuckets() []string { return nil }
func (erroringSweepBackend) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	return nil, errors.New("backend unavailable")
}
func (erroringSweepBackend) ResetAcceptance() error { return errors.New("backend unavailable") }
func (erroringSweepBackend) FetchGridStatus() (map[string]interface{}, error) {
	return nil, errors.New("backend unavailable")
}
func (erroringSweepBackend) ResetGrid() error                { return errors.New("backend unavailable") }
func (erroringSweepBackend) WaitForGridSettle(time.Duration) {}
func (erroringSweepBackend) FetchTrackingMetrics() (map[string]interface{}, error) {
	return nil, errors.New("backend unavailable")
}
func (erroringSweepBackend) SetTuningParams(map[string]interface{}) error {
	return errors.New("backend unavailable")
}
func (erroringSweepBackend) StartPCAPReplayWithConfig(sweep.PCAPReplayConfig) error {
	return errors.New("backend unavailable")
}
func (erroringSweepBackend) StopPCAPReplay() error                   { return nil }
func (erroringSweepBackend) WaitForPCAPComplete(time.Duration) error { return nil }
func (erroringSweepBackend) GetLastAnalysisRunID() string            { return "" }

func TestHintRunCreatorRejectsMalformedParams(t *testing.T) {
	creator := &hintRunCreator{runner: sweep.NewRunner(erroringSweepBackend{})}

	// Malformed params are caught before any sweep is started, so this
	// returns without touching the backend.
	_, err := creator.CreateSweepRun("test-sensor", "scene.pcap",
		json.RawMessage("{not json"), 0, 1)
	if err == nil {
		t.Fatal("CreateSweepRun with malformed params succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "parsing paramsJSON") {
		t.Errorf("error = %v, want it to name the params parse failure", err)
	}
}

func TestHintRunCreatorInfersParamTypes(t *testing.T) {
	creator := &hintRunCreator{runner: sweep.NewRunner(erroringSweepBackend{})}

	// JSON carries no Go types, so CreateSweepRun infers one per value:
	// bool, string, and float64 for every number. A failing backend aborts
	// the run right after the request is built, which is all this needs.
	params := json.RawMessage(`{"seed_from_first":true,"mode":"fast","noise_relative":0.01}`)

	_, err := creator.CreateSweepRun("test-sensor", "scene.pcap", params, 0, 1)
	if err == nil {
		t.Fatal("CreateSweepRun succeeded against a failing backend, want an error")
	}
	// It must get past parsing — a parse failure here would mean the type
	// inference never ran.
	if strings.Contains(err.Error(), "parsing paramsJSON") {
		t.Errorf("error = %v, want a failure after the params were parsed", err)
	}
}

func TestHintRunCreatorAcceptsNullParams(t *testing.T) {
	creator := &hintRunCreator{runner: sweep.NewRunner(erroringSweepBackend{})}

	// A literal JSON null means "no parameter overrides" and must be treated
	// as an empty sweep rather than a parse error.
	_, err := creator.CreateSweepRun("test-sensor", "scene.pcap", json.RawMessage("null"), 0, 1)
	if err == nil {
		t.Fatal("CreateSweepRun succeeded against a failing backend, want an error")
	}
	if strings.Contains(err.Error(), "parsing paramsJSON") {
		t.Errorf("error = %v, want null params to be accepted, not parsed as invalid", err)
	}
}

func TestHintSceneAdapterGetSceneMissing(t *testing.T) {
	database, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	adapter := &hintSceneAdapter{store: sqlite.NewReplayCaseStore(database)}

	if _, err := adapter.GetScene("no-such-scene"); err == nil {
		t.Fatal("GetScene for an unknown scene succeeded, want an error")
	}
}

func TestHintSceneAdapterSetReferenceRunMissing(t *testing.T) {
	database, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	adapter := &hintSceneAdapter{store: sqlite.NewReplayCaseStore(database)}

	// Delegates straight to the store; an unknown scene must not silently
	// succeed, or HINT would believe it had pinned a reference run.
	if err := adapter.SetReferenceRun("no-such-scene", "run-1"); err == nil {
		t.Fatal("SetReferenceRun for an unknown scene succeeded, want an error")
	}
}

func TestHintLabelAdapterGetLabelingProgressOnEmptyRun(t *testing.T) {
	database, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	adapter := &hintLabelAdapter{store: sqlite.NewAnalysisRunStore(database)}

	total, labelled, byClass, err := adapter.GetLabelingProgress("no-such-run")
	if err != nil {
		t.Fatalf("GetLabelingProgress: %v", err)
	}
	if total != 0 || labelled != 0 {
		t.Errorf("progress = %d/%d, want 0/0 for an unknown run", labelled, total)
	}
	if len(byClass) != 0 {
		t.Errorf("byClass = %v, want empty", byClass)
	}
}

func TestHintLabelAdapterGetRunTracksOnEmptyRun(t *testing.T) {
	database, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	adapter := &hintLabelAdapter{store: sqlite.NewAnalysisRunStore(database)}

	got, err := adapter.GetRunTracks("no-such-run")
	if err != nil {
		t.Fatalf("GetRunTracks: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d tracks, want 0 for an unknown run", len(got))
	}
}

func TestHintLabelAdapterUpdateTrackLabelUnknownTrack(t *testing.T) {
	database, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	adapter := &hintLabelAdapter{store: sqlite.NewAnalysisRunStore(database)}

	// Pure delegation; whatever the store decides about an unknown track is
	// what the caller must see.
	err := adapter.UpdateTrackLabel("no-such-run", "no-such-track", "vehicle", "good", 0.9, "tester", "manual")
	wantErr := sqlite.NewAnalysisRunStore(database).UpdateTrackLabel(
		"no-such-run", "no-such-track", "vehicle", "good", 0.9, "tester", "manual")

	if (err == nil) != (wantErr == nil) {
		t.Errorf("adapter error = %v, store error = %v; want them to agree", err, wantErr)
	}
}
