package labelwork_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

func store(t *testing.T) labelwork.Store {
	t.Helper()
	d, err := db.NewDB(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return labelwork.Store{DB: d.DB}
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

func state() *labelset.State {
	reviewed, proposed := annotation.StatusReviewed, annotation.StatusProposed
	s := &labelset.State{
		Windows: []labelset.Window{{WindowID: 0}, {WindowID: 1}, {WindowID: 2}, {WindowID: 3}},
		Records: labelset.Records{Objects: []annotation.Object{
			{ObjectID: "a", Class: "car", Status: reviewed},
			{ObjectID: "b", Class: "truck", Status: proposed},
			{ObjectID: "c", Class: "", Status: proposed},
			{ObjectID: "r", Class: "car", Status: annotation.StatusRejected},
		}},
	}
	mask := func(obj string, w int, st annotation.ReviewStatus, vis annotation.Visibility) labelset.Mask {
		return labelset.Mask{ObjectID: obj, WindowID: w, Status: st, Completeness: annotation.MaskComplete, Visibility: vis}
	}
	s.Masks = []labelset.Mask{
		mask("a", 0, reviewed, annotation.VisiblePresent), mask("a", 1, reviewed, annotation.VisiblePartlyOccluded),
		mask("a", 2, reviewed, annotation.VisiblePresent),
		mask("b", 0, proposed, annotation.VisibleUnknown), mask("b", 3, reviewed, annotation.VisiblePresent),
	}
	return s
}

// Each layer is counted on its own, with its own denominator; spans do not
// make masks complete, and an unknown total stays unknown.
func TestMembershipProgressKeepsLayersApart(t *testing.T) {
	p := labelwork.MembershipProgress(state())
	if p.Objects != 3 || p.RejectedObjects != 1 || p.ReviewedObjects != 1 || p.ProposedObjects != 2 {
		t.Fatalf("%+v", p)
	}
	layers := map[string]labelwork.LayerProgress{}
	for _, l := range p.Layers {
		layers[l.Layer] = l
	}
	spans := layers["spans"]
	if spans.Counts[0].Total != nil || spans.Counts[0].Done != 3 {
		t.Fatalf("total road users is unknown: %+v", spans.Counts[0])
	}
	if spans.Counts[1].Done != 2 || *spans.Counts[1].Total != 3 || spans.Counts[2].Done != 2 {
		t.Fatalf("%+v", spans.Counts)
	}
	masks := layers["masks"].Counts[1]
	// a spans windows 0..2 with three masks; b spans 0..3 with two.
	if masks.Done != 5 || *masks.Total != 7 {
		t.Fatalf("%+v", masks)
	}
	review := layers["mask_review"]
	if review.Counts[0].Done != 4 || *review.Counts[0].Total != 5 || review.Counts[1].Done != 3 {
		t.Fatalf("truth masks are reviewed masks on reviewed objects: %+v", review.Counts)
	}
	if layers["object_review"].Status != labelwork.StatusInProgress || layers["spans"].Status == labelwork.StatusComplete {
		t.Fatalf("%+v", layers)
	}
	if occ := layers["occlusion"].Counts; occ[0].Done != 4 || occ[1].Done != 1 {
		t.Fatalf("%+v", occ)
	}
	if strings.Join(p.FreezeBlockers, ",") != "b,c" {
		t.Fatalf("%v", p.FreezeBlockers)
	}
}

// Reviewing an object moves object review only.
func TestReviewChangesOnlyItsLayer(t *testing.T) {
	s := state()
	before := labelwork.MembershipProgress(s)
	s.Records.Objects[1].Status = annotation.StatusReviewed
	after := labelwork.MembershipProgress(s)
	if after.ReviewedObjects != before.ReviewedObjects+1 || after.ReviewedMasks != before.ReviewedMasks || after.TruthMasks != before.TruthMasks+1 {
		t.Fatalf("before %+v after %+v", before, after)
	}
}

// A physical reference document's review is counted apart from the masks,
// and a pack without one says so rather than reporting zero work left.
func TestPhysicalProgress(t *testing.T) {
	none := labelwork.PhysicalReferenceProgress(nil, 4)
	if none.Exists || *none.Layers[0].Counts[0].Total != 4 || none.Layers[0].Counts[0].Done != 0 {
		t.Fatalf("%+v", none)
	}
	tasks := labelwork.Tasks(labelwork.RowFacts{Cohort: "development", HasLabelSet: true, Bound: true,
		Progress: ptr(labelwork.MembershipProgress(state())), Packs: []labelwork.PackFacts{{Handle: "c/packs/r", Physical: &none}}})
	var layers []string
	for _, t := range tasks {
		layers = append(layers, t.Layer+":"+t.Where)
	}
	got := strings.Join(layers, " ")
	for _, want := range []string{"spans:native", "object_review:browser", "masks:native", "mask_review:native", "physical:native", "freeze:browser"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s missing from %s", want, got)
		}
	}
	if strings.Contains(got, "purpose:") {
		t.Fatalf("an assigned, bound capture needs no purpose task: %s", got)
	}
}

// An unassigned capture is TBD with a reason, never ready; a protected one
// shows no per-item work.
func TestTasksForUnassignedAndProtected(t *testing.T) {
	tasks := labelwork.Tasks(labelwork.RowFacts{Cohort: "unassigned"})
	if len(tasks) < 2 || tasks[0].TBD != "unassigned_purpose" || !tasks[0].Blocked {
		t.Fatalf("%+v", tasks)
	}
	protected := labelwork.Tasks(labelwork.RowFacts{Cohort: "qualification", Protected: true, HasLabelSet: true,
		Progress: ptr(labelwork.MembershipProgress(state()))})
	if len(protected) != 1 || protected[0].Layer != "purpose" {
		t.Fatalf("%+v", protected)
	}
	unread := labelwork.Tasks(labelwork.RowFacts{Cohort: "development", HasLabelSet: true, ProgressError: "boom"})
	if unread[len(unread)-1].TBD != "unavailable_source" {
		t.Fatalf("%+v", unread)
	}
}

// The labelling plans are read from their specs, as planned work.
func TestReadPlans(t *testing.T) {
	plans := labelwork.ReadPlans(filepath.Join("..", "..", ".."))
	if !plans.Available {
		t.Fatal(plans.Error)
	}
	ids := map[string]bool{}
	for _, p := range plans.Plans {
		ids[p.ID] = p.Error == "" && len(p.Steps) > 0
	}
	for _, id := range []string{"label0001", "label0002", "label0003"} {
		if !ids[id] {
			t.Fatalf("%s not read: %+v", id, plans.Plans)
		}
	}
	if missing := labelwork.ReadPlans(t.TempDir()); missing.Available || missing.Error == "" {
		t.Fatalf("%+v", missing)
	}
}

func ptr[T any](v T) *T { return &v }
