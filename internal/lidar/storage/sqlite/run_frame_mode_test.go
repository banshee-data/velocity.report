package sqlite

import (
	"encoding/json"
	"strings"
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
)

// A run records what its VRLOG frames hold, decided at its start; a run that
// says nothing holds foreground frames, as every VRLOG before the column did.
func TestRunFrameModeIsStoredAndRead(t *testing.T) {
	db, cleanup := setupRunDB(t)
	defer cleanup()
	cfg := cfgpkg.MustLoadDefaultConfig()
	store := NewRunStore(db)

	ids := map[string]string{}
	for _, mode := range []string{RunFrameModeFull, "", "bogus"} {
		id, err := NewRunManager(db, "frame-mode-"+mode).StartRunWithConfig(RunStartOptions{
			SourcePath: "/tmp/" + mode + ".pcap", EffectiveConfig: cfg, FrameMode: mode,
		})
		if err != nil {
			t.Fatalf("start %q: %v", mode, err)
		}
		ids[mode] = id
	}
	want := map[string]string{RunFrameModeFull: "full", "": "foreground", "bogus": "foreground"}
	for mode, id := range ids {
		run, err := store.GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if run.FrameMode != want[mode] {
			t.Errorf("started with %q: frame mode %q, want %q", mode, run.FrameMode, want[mode])
		}
	}

	// The JSON always names the mode.
	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		body, _ := json.Marshal(run)
		if !strings.Contains(string(body), `"frame_mode":"`+run.FrameMode+`"`) {
			t.Errorf("run JSON %s does not name its frame mode", body)
		}
	}
	if _, err := db.Exec(`UPDATE lidar_runs SET frame_mode = 'partial' WHERE run_id = ?`, ids[RunFrameModeFull]); err == nil {
		t.Error("an unknown frame mode was accepted")
	}
}
