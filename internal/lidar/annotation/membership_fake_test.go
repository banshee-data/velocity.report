package annotation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// This package's tests cannot import package labelset, which imports this
// one, so they stand in for it: testMemberships keeps each pack's membership
// revisions in memory and projects them as the label set projector does,
// with a token per revision. Tests in other packages use the real label set
// (labelset.SaveMembership).
type testMembershipStore struct {
	mu   sync.Mutex
	revs map[string][][]byte // pack directory -> revision bytes, revision 1 first
}

var testMemberships = &testMembershipStore{revs: map[string][][]byte{}}

func init() {
	RegisterMembershipProjector(func(p *Pack, _ LabelSetLink, revision int) ([]byte, string, error) {
		testMemberships.mu.Lock()
		defer testMemberships.mu.Unlock()
		revs := testMemberships.revs[p.Dir]
		if len(revs) == 0 {
			return nil, "", fmt.Errorf("test pack %s has no membership", p.Dir)
		}
		if revision == 0 {
			revision = len(revs)
		}
		if revision < 1 || revision > len(revs) {
			return nil, "", fmt.Errorf("test pack %s has no membership revision %d", p.Dir, revision)
		}
		b := revs[revision-1]
		if b == nil {
			return nil, "", fmt.Errorf("membership revision %d is not retained", revision)
		}
		return b, sha256Hex(b), nil
	})
}

// saveMembership records s as the pack's next membership revision, as a save
// through the pack's label set would: the edit must be of the current
// revision, it is validated and canonicalised, and s comes back with its new
// revision and token.
func saveMembership(p *Pack, s *Sidecar) error {
	next := *s
	next.Objects = append([]Object(nil), s.Objects...)
	next.Masks = append([]FrameMask(nil), s.Masks...)
	next.Correspondences = append([]TrackCorrespondence(nil), s.Correspondences...)
	for i := range next.Masks {
		next.Masks[i].PointIndices = append([]int(nil), s.Masks[i].PointIndices...)
		next.Masks[i].UncertainIndices = append([]int(nil), s.Masks[i].UncertainIndices...)
		sort.Ints(next.Masks[i].PointIndices)
		sort.Ints(next.Masks[i].UncertainIndices)
	}
	if err := next.Validate(p); err != nil {
		return fmt.Errorf("invalid annotation edit: %w", err)
	}
	next.Canonicalise()

	testMemberships.mu.Lock()
	defer testMemberships.mu.Unlock()
	revs := testMemberships.revs[p.Dir]
	parent := len(revs)
	if parent > 0 && s.baseDigest != sha256Hex(revs[parent-1]) {
		return ErrSidecarConflict
	}
	if parent == 0 && s.baseDigest != "" {
		return ErrSidecarConflict
	}
	next.Revision = parent + 1
	next.UpdatedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	next.Change.ParentRev, next.Change.Revision, next.Change.CreatedUTC = parent, next.Revision, next.UpdatedUTC
	if next.Change.Operation == "" {
		next.Change.Operation = "save"
	}
	b, err := json.MarshalIndent(&next, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if parent == 0 {
		if err := linkTestMembership(p.Dir); err != nil {
			return err
		}
	}
	testMemberships.revs[p.Dir] = append(revs, b)
	next.baseDigest = sha256Hex(b)
	*s = next
	return nil
}

// broken marks packs whose membership cannot be read, as a label set that
// fails to open.
var brokenMemberships sync.Map

func init() {
	inner := projector
	RegisterMembershipProjector(func(p *Pack, link LabelSetLink, revision int) ([]byte, string, error) {
		if _, broken := brokenMemberships.Load(p.Dir); broken {
			return nil, "", fmt.Errorf("label set %s cannot be read", link.LabelSetID)
		}
		return inner(p, link, revision)
	})
}

// breakMembership makes the pack's membership unreadable.
func breakMembership(p *Pack) { brokenMemberships.Store(p.Dir, true) }

// dropMembership leaves the pack linked to a label set that holds nothing.
func dropMembership(p *Pack) {
	testMemberships.mu.Lock()
	defer testMemberships.mu.Unlock()
	testMemberships.revs[p.Dir] = nil
}

// forgetMembershipHistory keeps only the head, as a published label set does.
func forgetMembershipHistory(p *Pack) {
	testMemberships.mu.Lock()
	defer testMemberships.mu.Unlock()
	revs := testMemberships.revs[p.Dir]
	for i := range revs[:len(revs)-1] {
		revs[i] = nil
	}
}

// rewriteMembershipHead replaces the head revision's content in place,
// which a label set's journal and digests exist to catch.
func rewriteMembershipHead(p *Pack, s *Sidecar) {
	b, _ := json.MarshalIndent(s, "", "  ")
	testMemberships.mu.Lock()
	defer testMemberships.mu.Unlock()
	revs := testMemberships.revs[p.Dir]
	revs[len(revs)-1] = append(b, '\n')
}

// membershipHead returns the head revision's bytes, nil when there are none.
func membershipHead(p *Pack) []byte {
	testMemberships.mu.Lock()
	defer testMemberships.mu.Unlock()
	revs := testMemberships.revs[p.Dir]
	if len(revs) == 0 {
		return nil
	}
	return append([]byte(nil), revs[len(revs)-1]...)
}

// copyMembership gives to the membership saved for from, unchanged.
func copyMembership(from, to *Pack) {
	testMemberships.mu.Lock()
	revs := append([][]byte(nil), testMemberships.revs[from.Dir]...)
	testMemberships.revs[to.Dir] = revs
	testMemberships.mu.Unlock()
	linkTestMembership(to.Dir)
}

// linkTestMembership links a pack to a stand-in label set with no verified
// captures: the link file, and the label set manifest split freezing reads.
func linkTestMembership(packDir string) error {
	link := LabelSetLink{LabelSetID: "test-membership", Path: "labels/test-membership"}
	lb, _ := json.Marshal(link)
	if err := os.WriteFile(filepath.Join(packDir, LabelSetLinkFile), lb, 0o644); err != nil {
		return err
	}
	dir := link.LabelSetDir(packDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "labels.json"), []byte(`{"label_set_id":"test-membership","applies_to":[]}`), 0o644)
}

// readSaved returns the pack's current membership bytes.
func readSaved(t *testing.T, p *Pack) []byte {
	t.Helper()
	b := membershipHead(p)
	if b == nil {
		t.Fatal("the pack has no saved membership")
	}
	return b
}
