package labelworkdb_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
)

func store(t *testing.T) labelworkdb.WorkflowStore {
	t.Helper()
	d, err := db.NewDB(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return labelworkdb.WorkflowStore{DB: d.DB}
}

// Workflow items persist, keep their history, and refuse a write based on a
// revision another reviewer has already replaced.
func TestWorkflowStoreRevisionsAndStaleWrites(t *testing.T) {
	s := store(t)
	item := labelwork.Item{ItemKey: "label-set:abc", Layer: "spans", State: "in_progress", Assignee: "ana", UpdatedBy: "ana",
		SourceRevision: "label-set:3:sha256:x"}
	first, err := s.Put(item, 0)
	if err != nil || first.Revision != 1 {
		t.Fatal(err, first)
	}
	item.State, item.Note, item.UpdatedBy = "blocked", "", "bo"
	if _, err := s.Put(item, 1); !errors.Is(err, labelwork.ErrInvalid) {
		t.Fatalf("blocked without a blocker: %v", err)
	}
	item.Blocker = "waiting on the capture"
	second, err := s.Put(item, 1)
	if err != nil || second.Revision != 2 || second.CreatedAtNs != first.CreatedAtNs {
		t.Fatal(err, second)
	}
	item.State, item.Blocker = "done", ""
	if _, err := s.Put(item, 1); !errors.Is(err, labelwork.ErrStale) {
		t.Fatalf("a write from revision 1 after revision 2: %v", err)
	}
	if _, err := s.Put(labelwork.Item{ItemKey: "label-set:abc", Layer: "spans", State: "open", UpdatedBy: "c"}, 0); !errors.Is(err, labelwork.ErrStale) {
		t.Fatalf("a second create: %v", err)
	}
	got, err := s.Items("label-set:abc")
	if err != nil || len(got) != 1 || got[0].State != "blocked" || got[0].Revision != 2 {
		t.Fatal(err, got)
	}
	history, err := s.History("label-set:abc", 0)
	if err != nil || len(history) != 2 || history[0].Revision != 2 {
		t.Fatal(err, history)
	}
	if _, err := s.DB.Exec(`DELETE FROM lidar_workflow_events`); err == nil {
		t.Fatal("history is append-only")
	}
}

// TBD carries a reason, and "other" carries a note.
func TestTBDNeedsAReason(t *testing.T) {
	s := store(t)
	base := labelwork.Item{ItemKey: "capture:sha256:aa", Layer: "purpose", State: "tbd", UpdatedBy: "x"}
	if _, err := s.Put(base, 0); !errors.Is(err, labelwork.ErrInvalid) {
		t.Fatal(err)
	}
	base.TBDReason = "other"
	if _, err := s.Put(base, 0); !errors.Is(err, labelwork.ErrInvalid) {
		t.Fatal(err)
	}
	base.TBDReason = "unassigned_purpose"
	if _, err := s.Put(base, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(labelwork.Item{ItemKey: "nonsense", Layer: "spans", State: "open", UpdatedBy: "x"}, 0); !errors.Is(err, labelwork.ErrInvalid) {
		t.Fatal(err)
	}
}
