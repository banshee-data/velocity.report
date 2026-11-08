package sqlite

import "testing"

// A restart fails the runs the previous process left running and leaves
// finished runs alone.
func TestRunStore_RecoverOrphanedRuns(t *testing.T) {
	db, cleanup := setupRunTestDB(t)
	defer cleanup()
	store := NewRunStore(db)

	for _, r := range []*Run{
		{RunID: "orphan", SourceType: "pcap", SensorID: "s1", Status: "running"},
		{RunID: "done", SourceType: "pcap", SensorID: "s1", Status: "completed"},
	} {
		if err := store.InsertRun(r); err != nil {
			t.Fatalf("InsertRun %s: %v", r.RunID, err)
		}
	}

	n, err := store.RecoverOrphanedRuns()
	if err != nil {
		t.Fatalf("RecoverOrphanedRuns: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered %d runs, want 1", n)
	}
	orphan, err := store.GetRun("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if orphan.Status != "failed" || orphan.ErrorMessage != OrphanedRunError {
		t.Errorf("orphan = status %q error %q, want failed with %q", orphan.Status, orphan.ErrorMessage, OrphanedRunError)
	}
	if done, _ := store.GetRun("done"); done.Status != "completed" || done.ErrorMessage != "" {
		t.Errorf("finished run = status %q error %q, want completed and untouched", done.Status, done.ErrorMessage)
	}
	if n, err := store.RecoverOrphanedRuns(); err != nil || n != 0 {
		t.Errorf("second recovery = %d, %v; want 0", n, err)
	}
}
