package labelset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// migrationBase is two windows of 400 returns each with every offset in the
// domain, and sample i's point j at offset j.
func migrationBase() (State, [][]uint64) {
	s := State{Packets: []PacketKey{1, 2}, Domains: []Domain{{DomainID: "fg", Kind: DomainForeground}},
		Windows: []Window{{WindowID: 0, Start: 0, End: 400}, {WindowID: 1, Start: 400, End: 800}}}
	offsets := make([][]uint64, 2)
	for i := range offsets {
		offsets[i] = make([]uint64, 50)
		for j := range offsets[i] {
			offsets[i][j] = uint64(j)
		}
		s.DomainReturns = append(s.DomainReturns, DomainReturns{DomainID: "fg", WindowID: i, Returns: Ranges{{0, 50}}})
	}
	return s, offsets
}

func v1Mask(object string, sample int, status annotation.ReviewStatus, idx ...int) annotation.FrameMask {
	return annotation.FrameMask{ObjectID: object, SampleID: sample, PointIndices: idx, Status: status,
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent}
}

func TestConvertSidecarResolvesOverlaps(t *testing.T) {
	base, offsets := migrationBase()
	sc := &annotation.Sidecar{
		Objects: []annotation.Object{
			{ObjectID: "rev", Status: annotation.StatusReviewed}, {ObjectID: "p1", Status: annotation.StatusProposed},
			{ObjectID: "p2", Status: annotation.StatusProposed},
		},
		Masks: []annotation.FrameMask{
			v1Mask("rev", 0, annotation.StatusReviewed, 1, 2, 3),
			v1Mask("p1", 0, annotation.StatusProposed, 3, 4, 5),
			v1Mask("p2", 0, annotation.StatusProposed, 5, 6),
			v1Mask("p1", 1, annotation.StatusProposed, 7),
		},
	}
	s, counts, err := convertSidecar(base, offsets, sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(Pandar40P); err != nil {
		t.Fatalf("converted state invalid: %v", err)
	}
	if counts.ProposedVsReviewed != 1 || counts.ProposedVsProposed != 1 || counts.MasksTouched != 2 {
		t.Fatalf("counts %+v", counts)
	}
	find := func(o string, w int) Mask {
		for _, m := range s.Masks {
			if m.ObjectID == o && m.WindowID == w {
				return m
			}
		}
		t.Fatalf("no mask %s %d", o, w)
		return Mask{}
	}
	if m := find("rev", 0); !Equal(m.Members, Ranges{{1, 3}}) || len(m.Uncertain) != 0 {
		t.Fatalf("reviewed mask %+v", m)
	}
	if m := find("p1", 0); !Equal(m.Members, Ranges{{4, 1}}) || !Equal(m.Uncertain, Ranges{{3, 1}, {5, 1}}) {
		t.Fatalf("p1 %v / %v", m.Members, m.Uncertain)
	}
	if m := find("p2", 0); !Equal(m.Members, Ranges{{6, 1}}) || !Equal(m.Uncertain, Ranges{{5, 1}}) {
		t.Fatalf("p2 %v / %v", m.Members, m.Uncertain)
	}
	// Two reviewed claims are refused, not resolved.
	sc.Objects[1].Status = annotation.StatusReviewed
	sc.Masks[1].Status = annotation.StatusReviewed
	if _, _, err := convertSidecar(base, offsets, sc); err == nil || !strings.Contains(err.Error(), "two reviewed") {
		t.Fatalf("two reviewed claims: %v", err)
	}
}

func TestPacketTableJoinsSplitPackets(t *testing.T) {
	maps := []sampleMap{{packets: []PacketKey{1, 2, 3}}, {packets: []PacketKey{3, 4}}, {packets: []PacketKey{5}}}
	table, index, err := packetTable(maps)
	if err != nil {
		t.Fatal(err)
	}
	if len(table) != 5 || index[3] != 2 || index[5] != 4 {
		t.Fatalf("table %v", table)
	}
	maps = append(maps, sampleMap{packets: []PacketKey{2}})
	if _, _, err := packetTable(maps); err == nil {
		t.Fatal("a packet reappearing non-adjacently was accepted")
	}
}

func TestHistoryWriterKeepsNumberingAndTimes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "set")
	hw, err := NewHistory(dir, Manifest{Name: "h", Layout: Pandar40P})
	if err != nil {
		t.Fatal(err)
	}
	s := testState()
	if _, err := hw.Add(s, Change{Author: "dd", Operation: "save_mask", CreatedUTC: "2026-09-29T17:48:00Z"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := hw.Add(s, Change{Author: "dd", Operation: "save_mask", CreatedUTC: "2026-09-29T17:49:00Z"}, 0); err != nil {
		t.Fatal(err) // an unchanged revision still advances the numbering
	}
	s2 := s.Clone()
	s2.Masks[0].Status = annotation.StatusRejected
	if _, err := hw.Add(s2, Change{Author: "dd", Operation: "restore", CreatedUTC: "2026-09-29T17:50:00Z"}, 1); err != nil {
		t.Fatal(err)
	}
	ls, err := hw.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if ls.Revision() != 3 {
		t.Fatalf("head revision %d", ls.Revision())
	}
	if n, err := Verify(dir); err != nil || n != 3 {
		t.Fatalf("verify %d %v", n, err)
	}
	journal, _ := os.ReadFile(filepath.Join(dir, JournalFile))
	if !strings.Contains(string(journal), `"created_utc":"2026-09-29T17:49:00Z"`) || !strings.Contains(string(journal), `"restored_from":1`) {
		t.Fatalf("journal lost times or restore marks:\n%s", journal)
	}

	// A history may start from a base at a later revision.
	dir2 := filepath.Join(t.TempDir(), "based")
	hw, _ = NewHistory(dir2, Manifest{Layout: Pandar40P})
	if _, err := hw.Base(2232, s, Change{Author: "dd", Operation: "migrate"}); err != nil {
		t.Fatal(err)
	}
	ls, err = hw.Finish()
	if err != nil || ls.Revision() != 2232 {
		t.Fatalf("based history at %d: %v", ls.Revision(), err)
	}
	if _, err := LoadRevision(dir2, 2232); err != nil {
		t.Fatalf("base revision: %v", err)
	}
}
