package sqlite

import (
	"testing"
)

func insertListingSweep(t *testing.T, store *SweepStore, sweepID, mode, results string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO lidar_tuning_sweeps (sweep_id, sensor_id, mode, status, request, results, started_at)
		VALUES (?, 'hesai-pandar40p', ?, 'completed', '{}', ?, '2026-11-07T00:00:00Z')`, sweepID, mode, results); err != nil {
		t.Fatalf("insert sweep %s: %v", sweepID, err)
	}
}

func TestSweepsForRunsReadsEachCombinationsRun(t *testing.T) {
	db := setupClipFilesDB(t)
	sweeps := NewSweepStore(db)
	insertListingSweep(t, sweeps, "sweep-a", "sweep", `[{"run_id":"run-1"},{"run_id":"run-2"},{"param_values":{}}]`)
	// run-2 again in a later sweep: a combination whose replay failed keeps
	// the previous run's ID, so the earlier sweep is the one that ran it.
	insertListingSweep(t, sweeps, "sweep-b", "auto", `[{"run_id":"run-2","score":1},{"run_id":"run-3"}]`)
	insertListingSweep(t, sweeps, "sweep-c", "sweep", `not json`)
	insertListingSweep(t, sweeps, "sweep-d", "sweep", `{"run_id":"run-4"}`)

	got, err := SweepsForRuns(db, []string{"run-1", "run-2", "run-3", "run-4", "run-5"})
	if err != nil {
		t.Fatalf("SweepsForRuns: %v", err)
	}
	want := map[string]RunSweep{
		"run-1": {SweepID: "sweep-a", Mode: "sweep"},
		"run-2": {SweepID: "sweep-a", Mode: "sweep"},
		"run-3": {SweepID: "sweep-b", Mode: "auto"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d runs matched, want %d: %+v", len(got), len(want), got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: got %+v, want %+v", id, got[id], w)
		}
	}
}

// A checkpointed sweep stores its runs in checkpoint_results at each round
// and keeps them there when a restart suspends it; its runs still fold.
func TestSweepsForRunsReadsACheckpointedSweep(t *testing.T) {
	db := setupClipFilesDB(t)
	if _, err := db.Exec(`INSERT INTO lidar_tuning_sweeps (sweep_id, sensor_id, mode, status, request, checkpoint_round, checkpoint_results, started_at)
		VALUES ('sweep-s', 'hesai-pandar40p', 'auto', 'suspended', '{}', 2, '[{"run_id":"run-7","score":0.4},{"run_id":"run-8"}]', '2026-11-07T00:00:00Z')`); err != nil {
		t.Fatalf("insert suspended sweep: %v", err)
	}
	// A completed sweep whose final results repeat its checkpoint's runs.
	insertListingSweep(t, NewSweepStore(db), "sweep-c", "auto", `[{"run_id":"run-9"}]`)
	if _, err := db.Exec(`UPDATE lidar_tuning_sweeps SET checkpoint_results = '[{"run_id":"run-9"}]' WHERE sweep_id = 'sweep-c'`); err != nil {
		t.Fatalf("set checkpoint: %v", err)
	}

	got, err := SweepsForRuns(db, []string{"run-7", "run-8", "run-9"})
	if err != nil {
		t.Fatalf("SweepsForRuns: %v", err)
	}
	for id, want := range map[string]string{"run-7": "sweep-s", "run-8": "sweep-s", "run-9": "sweep-c"} {
		if got[id].SweepID != want || got[id].Mode != "auto" {
			t.Errorf("%s: got %+v, want %s/auto", id, got[id], want)
		}
	}
}

// HINT stores no results; each round names the reference run it replayed.
func TestSweepsForRunsReadsAHINTSweepsRounds(t *testing.T) {
	db := setupClipFilesDB(t)
	if _, err := db.Exec(`INSERT INTO lidar_tuning_sweeps (sweep_id, sensor_id, mode, status, request, round_results, started_at)
		VALUES ('sweep-h', 'hesai-pandar40p', 'hint', 'completed', '{}', '[{"round":1,"reference_run_id":"run-r1"},{"round":2,"reference_run_id":"run-r2"},{"round":3,"reference_run_id":""}]', '2026-11-07T00:00:00Z')`); err != nil {
		t.Fatalf("insert hint sweep: %v", err)
	}
	got, err := SweepsForRuns(db, []string{"run-r1", "run-r2", ""})
	if err != nil {
		t.Fatalf("SweepsForRuns: %v", err)
	}
	if len(got) != 2 || got["run-r1"].SweepID != "sweep-h" || got["run-r2"].Mode != "hint" {
		t.Errorf("got %+v, want run-r1 and run-r2 under sweep-h/hint", got)
	}
}

func TestAttachRunSweepsLeavesUnmatchedRunsAlone(t *testing.T) {
	db := setupClipFilesDB(t)
	insertListingSweep(t, NewSweepStore(db), "sweep-a", "sweep", `[{"run_id":"run-1"}]`)
	runs := []*Run{{RunID: "run-1"}, {RunID: "run-9"}, nil}
	if err := AttachRunSweeps(db, runs); err != nil {
		t.Fatalf("AttachRunSweeps: %v", err)
	}
	if runs[0].SweepID != "sweep-a" || runs[0].SweepMode != "sweep" {
		t.Errorf("run-1 = %+v, want sweep-a/sweep", runs[0])
	}
	if runs[1].SweepID != "" || runs[1].SweepMode != "" {
		t.Errorf("run-9 should name no sweep, got %+v", runs[1])
	}
}

func TestAttachRunSweepsWithNoRuns(t *testing.T) {
	if err := AttachRunSweeps(setupClipFilesDB(t), nil); err != nil {
		t.Fatalf("AttachRunSweeps(nil): %v", err)
	}
}

func TestAttachClipSitesNamesLocatedClips(t *testing.T) {
	db := setupGeoDB(t)
	store := NewClipStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	insertCase(t, store, "case-2", "b.pcap")
	insertCase(t, store, "case-3", "c.pcap")
	loc, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if _, err := NewSiteStore(db).SetCanonicalPose(loc.L16Token, broadwayLat, broadwayLng, "operator", "Broadway at Columbus"); err != nil {
		t.Fatalf("SetCanonicalPose: %v", err)
	}
	loc2, err := store.SetClipLocation("case-2", kirkhamLat, kirkhamLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	clips, err := store.ListClips("")
	if err != nil {
		t.Fatalf("ListClips: %v", err)
	}
	if err := AttachClipSites(db, clips); err != nil {
		t.Fatalf("AttachClipSites: %v", err)
	}
	byID := map[string]*Clip{}
	for _, c := range clips {
		byID[c.ClipID] = c
	}
	if c := byID["case-1"]; c.SiteID != loc.L16Token || c.SiteLabel != "Broadway at Columbus" {
		t.Errorf("case-1 site = %q %q, want %q with its label", c.SiteID, c.SiteLabel, loc.L16Token)
	}
	if c := byID["case-2"]; c.SiteID != loc2.L16Token || c.SiteLabel != "" {
		t.Errorf("case-2 site = %q %q, want %q and no label", c.SiteID, c.SiteLabel, loc2.L16Token)
	}
	if c := byID["case-3"]; c.SiteID != "" {
		t.Errorf("an unlocated clip has no site, got %q", c.SiteID)
	}
	if err := AttachClipSites(db, nil); err != nil {
		t.Errorf("AttachClipSites(nil): %v", err)
	}
}
