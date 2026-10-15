package labelset

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// testState is two windows of a four-packet table with one domain and two
// reviewed objects.
func testState() State {
	s := State{
		Packets: []PacketKey{0x1111, 0x2222, 0x3333, 0x4444},
		Records: Records{Objects: []annotation.Object{
			{ObjectID: "obj_a", Class: "car", Confidence: 1, Status: annotation.StatusReviewed},
			{ObjectID: "obj_b", Class: "pedestrian", Confidence: 0.75, Status: annotation.StatusProposed},
		}},
		Domains: []Domain{{DomainID: "fg-1", Kind: DomainForeground, RunID: "run-1"}},
		Windows: []Window{{WindowID: 0, Start: 0, End: 800}, {WindowID: 1, Start: 800, End: 1600}},
		DomainReturns: []DomainReturns{
			{DomainID: "fg-1", WindowID: 0, Returns: Ranges{{10, 50}, {400, 20}}},
			{DomainID: "fg-1", WindowID: 1, Returns: Ranges{{0, 100}}},
		},
		Masks: []Mask{
			{ObjectID: "obj_a", WindowID: 0, DomainID: "fg-1", Members: Ranges{{10, 20}}, Uncertain: Ranges{{30, 2}},
				Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed,
				Provenance: annotation.Provenance{Author: "dd", CreatedUTC: "2026-10-12T00:00:00Z"}},
			{ObjectID: "obj_b", WindowID: 0, DomainID: "fg-1", Members: Ranges{{400, 5}},
				Completeness: annotation.MaskUnreviewed, Visibility: annotation.VisibleUnknown, Status: annotation.StatusProposed,
				Provenance: annotation.Provenance{Author: "proposer", Algorithm: "cluster_chain", AlgorithmVersion: "1"}},
			{ObjectID: "obj_a", WindowID: 1, DomainID: "fg-1", Members: Ranges{{0, 3}},
				Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePartlyOccluded, Status: annotation.StatusReviewed,
				Pose: &annotation.Pose{CenterX: 1.5, Length: 4.2, Width: 1.8, Height: 1.5, YawRad: 0.1, Confidence: 0.5, Status: annotation.StatusProposed}},
		},
	}
	s.Canonicalise()
	return s
}

func TestValidateAcceptsTestState(t *testing.T) {
	s := testState()
	if err := s.Validate(Pandar40P); err != nil {
		t.Fatal(err)
	}
	if got := len(s.ReviewedMasks()); got != 2 {
		t.Fatalf("%d reviewed masks, want 2", got)
	}
}

func TestValidateRefuses(t *testing.T) {
	cases := map[string]func(s *State){
		"duplicate packet":   func(s *State) { s.Packets[1] = s.Packets[0] },
		"overlapping window": func(s *State) { s.Windows[1].Start = 700 },
		"window past table":  func(s *State) { s.Windows[1].End = 1601 },
		"member outside domain": func(s *State) {
			s.Masks[0].Members = Ranges{{60, 1}}
		},
		"two objects claim a return": func(s *State) {
			s.Masks[1].Members = Ranges{{15, 1}}
		},
		"member and uncertain": func(s *State) { s.Masks[0].Uncertain = Ranges{{12, 1}} },
		"unknown object":       func(s *State) { s.Masks[0].ObjectID = "ghost" },
		"unknown domain":       func(s *State) { s.Masks[0].DomainID = "fg-2" },
		"non-foreground domain": func(s *State) {
			s.Domains[0].Kind = "all_returns"
		},
		"bad status":          func(s *State) { s.Masks[0].Status = "maybe" },
		"duplicate dismissed": func(s *State) { s.Records.DismissedProposals = []string{"p", "p"} },
		"non-canonical set":   func(s *State) { s.Masks[0].Members = Ranges{{10, 2}, {12, 1}} },
	}
	for name, mutate := range cases {
		s := testState()
		mutate(&s)
		if err := s.Validate(Pandar40P); err == nil {
			t.Errorf("%s: validated", name)
		}
	}
}

func TestCanonicalEncoding(t *testing.T) {
	type sample struct {
		Zeta  float64 `json:"zeta"`
		Alpha string  `json:"alpha"`
		Mid   []int   `json:"mid"`
	}
	b, err := Canonical(sample{Zeta: 1, Alpha: "a/<b>&\"\\\n\u0001é", Mid: []int{3, 1}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"alpha":"a/<b>&\"\\\n\u0001é","mid":[3,1],"zeta":1}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
	for v, want := range map[float64]string{0.1: "0.1", 1e-7: "1e-7", 1e21: "1e+21", 123456789: "123456789", -0.5: "-0.5", 0.75: "0.75"} {
		b, err := Canonical(map[string]float64{"v": v})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSuffix(strings.TrimPrefix(string(b), `{"v":`), "}"); got != want {
			t.Errorf("%v encodes as %s, want %s", v, got, want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	s := testState()
	files, err := EncodeState(&s)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeState(files)
	if err != nil {
		t.Fatal(err)
	}
	back.Canonicalise()
	if !reflect.DeepEqual(*back, s) {
		t.Fatalf("round trip changed the state:\n%+v\n%+v", *back, s)
	}
	again, _ := EncodeState(back)
	if StateDigest(again) != StateDigest(files) {
		t.Fatal("re-encoding changed the digest")
	}
	if !strings.HasSuffix(string(files[MasksFile]), "\n") || strings.Count(string(files[MasksFile]), "\n") != len(s.Masks) {
		t.Fatalf("masks.ndjson is not one line per mask:\n%s", files[MasksFile])
	}
}

func TestDecodeRefusesUnknownFields(t *testing.T) {
	s := testState()
	files, _ := EncodeState(&s)
	files[MasksFile] = []byte(strings.Replace(string(files[MasksFile]), `{"completeness"`, `{"colour":"red","completeness"`, 1))
	if _, err := DecodeState(files); err == nil {
		t.Fatal("unknown field accepted")
	}
}

// mutate makes a random, valid edit.
func mutate(r *rand.Rand, s *State) {
	switch r.Intn(7) {
	case 0: // move a member
		m := &s.Masks[r.Intn(len(s.Masks))]
		dom := domainSet(s, m.DomainID, m.WindowID)
		free := Difference(Difference(dom, m.Uncertain), claimedBy(s, m.WindowID))
		if len(free) > 0 && r.Intn(2) == 0 {
			m.Members = Union(m.Members, Ranges{{free[0].Start, 1}})
		} else if m.Members.Count() > 0 {
			m.Members = Difference(m.Members, Ranges{{m.Members[0].Start, 1}})
		}
	case 1:
		m := &s.Masks[r.Intn(len(s.Masks))]
		m.Status = []annotation.ReviewStatus{annotation.StatusProposed, annotation.StatusReviewed, annotation.StatusRejected}[r.Intn(3)]
	case 2:
		id := fmt.Sprintf("obj_%d", r.Intn(1000))
		for _, o := range s.Records.Objects {
			if o.ObjectID == id {
				return
			}
		}
		s.Records.Objects = append(s.Records.Objects, annotation.Object{ObjectID: id, Class: "car", Status: annotation.StatusProposed})
	case 3:
		s.Records.DismissedProposals = append(s.Records.DismissedProposals, fmt.Sprintf("p%d", r.Int()))
	case 4:
		if len(s.Masks) > 1 {
			i := r.Intn(len(s.Masks))
			s.Masks = append(s.Masks[:i], s.Masks[i+1:]...)
		}
	case 5:
		s.Packets = append(s.Packets, PacketKey(r.Uint64()))
	case 6:
		m := &s.Masks[r.Intn(len(s.Masks))]
		m.Pose = &annotation.Pose{CenterX: r.Float64(), Length: 4, Status: annotation.StatusProposed}
	}
	s.Canonicalise()
}

func domainSet(s *State, domain string, window int) Ranges {
	for _, d := range s.DomainReturns {
		if d.DomainID == domain && d.WindowID == window {
			return d.Returns
		}
	}
	return nil
}

func claimedBy(s *State, window int) Ranges {
	var all Ranges
	for _, m := range s.Masks {
		if m.WindowID == window {
			all = Union(all, m.Members)
		}
	}
	return all
}

func TestDiffApplyRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	base := testState()
	for i := 0; i < 300; i++ {
		next := base.Clone()
		for j := 0; j <= r.Intn(4); j++ {
			mutate(r, &next)
		}
		if err := next.Validate(Pandar40P); err != nil {
			t.Fatalf("step %d made an invalid state: %v", i, err)
		}
		d, err := Diff(&base, &next)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Apply(&base, d)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		gd, _ := DigestOf(got)
		nd, _ := DigestOf(&next)
		if gd != nd {
			t.Fatalf("step %d: apply(diff) differs from the edit", i)
		}
		base = next
	}
	same := base.Clone()
	d, _ := Diff(&base, &same)
	if !d.Empty() {
		t.Fatalf("diff of identical states is not empty: %+v", d)
	}
}

func TestApplyRefusesMisfit(t *testing.T) {
	s := testState()
	d := &Delta{Masks: []MaskDelta{{ObjectID: "obj_a", WindowID: 0, MembersRemove: Ranges{{500, 1}}}}}
	if _, err := Apply(&s, d); err == nil {
		t.Fatal("removing an absent member applied")
	}
	d = &Delta{Masks: []MaskDelta{{ObjectID: "obj_a", WindowID: 0, MembersAdd: Ranges{{10, 1}}}}}
	if _, err := Apply(&s, d); err == nil {
		t.Fatal("adding a present member applied")
	}
	d = &Delta{Masks: []MaskDelta{{ObjectID: "obj_c", WindowID: 0, MembersAdd: Ranges{{10, 1}}}}}
	if _, err := Apply(&s, d); err == nil {
		t.Fatal("a mask created without fields applied")
	}
	shrunk := s.Clone()
	shrunk.Packets = shrunk.Packets[:2]
	if _, err := Diff(&s, &shrunk); err == nil {
		t.Fatal("a shrinking packet table diffed")
	}
}
