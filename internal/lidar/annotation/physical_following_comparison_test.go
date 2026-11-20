package annotation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestFollowingShellComparisonPreservesTruthAndCohort(t *testing.T) {
	p := physPack(t)
	b := validPhysical(p)
	b.RecordOrigins = b.currentOrigins()
	bytes, _ := json.Marshal(b)
	var c PhysicalReferenceSet
	if err := json.Unmarshal(bytes, &c); err != nil {
		t.Fatal(err)
	}
	c.Following = nil // The baseline's decisions, not a candidate's choices.
	for i := range c.Objects {
		c.Objects[i].Body.Review.Status = StatusProposed
		for j := range c.Objects[i].Keyframes {
			c.Objects[i].Keyframes[j].Review.Status = StatusProposed
		}
	}
	*c.Objects[1].Keyframes[0].Position.XM += 1
	r, err := CompareFollowingShells(p, b, &c)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != len(p.Samples) || r.Rows[0].DeltaM == nil || math.Abs(*r.Rows[0].DeltaM-1) > 1e-9 {
		t.Fatalf("comparison %+v", r)
	}
	if r.Rows[1].CandidateGap != nil || r.Rows[1].Unavailable != "follower_no_keyframe" {
		t.Fatalf("missing row %+v", r.Rows[1])
	}
	g := c.Objects[0].ProposalGeometry(c.Objects[0].Keyframes[0])
	if g.Truth || g.BodyTruth || g.Front == nil || g.Height != nil {
		t.Fatalf("proposal %+v", g)
	}
	if c.Objects[0].Geometry(c.Objects[0].Keyframes[0]).Front != nil {
		t.Fatal("truth gate accepted proposed body")
	}
	after, _ := json.Marshal(b)
	if string(bytes) != string(after) {
		t.Fatal("baseline mutated")
	}
	if c.Objects[0].Body.Review.Status != StatusProposed {
		t.Fatal("candidate promoted")
	}
	for _, mutate := range []func(*PhysicalReferenceSet){
		func(d *PhysicalReferenceSet) { d.Schema = "invalid" },
		func(d *PhysicalReferenceSet) { d.Source.CalibrationID = "different" },
	} {
		var bad PhysicalReferenceSet
		_ = json.Unmarshal(bytes, &bad)
		mutate(&bad)
		if _, err := CompareFollowingShells(p, b, &bad); err == nil {
			t.Fatal("invalid candidate accepted")
		}
	}
	bad := *b
	bad.Schema = "invalid"
	if _, err := CompareFollowingShells(p, &bad, &c); err == nil {
		t.Fatal("invalid baseline accepted")
	}
	b.Following[0].Interval.LastSample = 0
	r, err = CompareFollowingShells(p, b, &c)
	if err != nil || len(r.Rows) != 1 {
		t.Fatalf("interval cohort %+v %v", r, err)
	}
	b.Following[0].Review.Status = StatusProposed
	r, err = CompareFollowingShells(p, b, &c)
	if err != nil || len(r.Rows) != 0 {
		t.Fatalf("proposed decision %+v %v", r, err)
	}
}

func TestShellRowAbstentionsAndBounds(t *testing.T) {
	p := physPack(t)
	for _, tc := range []struct {
		name, reason string
		change       func(*PhysicalObject, *PhysicalObject, *FollowingReference)
	}{
		{"missing follower", "follower_no_keyframe", func(f, l *PhysicalObject, r *FollowingReference) { f.Keyframes = nil }},
		{"missing leader", "leader_no_keyframe", func(f, l *PhysicalObject, r *FollowingReference) { l.Keyframes = nil }},
		{"wrong time", "timestamp_mismatch", func(f, l *PhysicalObject, r *FollowingReference) { l.Keyframes[0].TimestampNs++ }},
		{"rejected", "rejected_keyframe", func(f, l *PhysicalObject, r *FollowingReference) { f.Keyframes[0].Review.Status = StatusRejected }},
		{"unknown front", "follower_front:unknown", func(f, l *PhysicalObject, r *FollowingReference) {
			f.Keyframes[0].Front = EndpointEvidence{Status: EvidenceUnknown}
		}},
		{"unknown rear", "leader_rear:unknown", func(f, l *PhysicalObject, r *FollowingReference) {
			l.Keyframes[0].Rear = EndpointEvidence{Status: EvidenceUnknown}
		}},
		{"behind", "behind the follower", func(f, l *PhysicalObject, r *FollowingReference) { *l.Keyframes[0].Position.XM = 1 }},
		{"no baseline value", "", func(f, l *PhysicalObject, r *FollowingReference) { r.Gaps = nil }},
		{"inferred rear", "", func(f, l *PhysicalObject, r *FollowingReference) { l.Keyframes[0].Rear.Status = EvidenceInferred }},
		{"normal", "", func(f, l *PhysicalObject, r *FollowingReference) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validPhysical(p)
			f, l, r := d.Objects[0], d.Objects[1], d.Following[0]
			tc.change(&f, &l, &r)
			row := compareShellRow(p.Samples[0], r, f, l)
			if tc.reason != "" {
				if !strings.Contains(row.Unavailable, tc.reason) || row.CandidateGap != nil {
					t.Fatalf("row %+v", row)
				}
				return
			}
			if row.CandidateGap == nil || row.Unavailable != "" {
				t.Fatalf("row %+v", row)
			}
			if tc.name == "no baseline value" {
				if row.DeltaM != nil {
					t.Fatal("invented baseline")
				}
				return
			}
			gap, reason := draftGap(f, l, carOneFollowsCarTwo(), 0)
			if reason != "" || !reflect.DeepEqual(gap, *row.CandidateGap) {
				t.Fatalf("shared propagation disagrees: %+v vs %+v (%s)", gap, row.CandidateGap, reason)
			}
		})
	}
	// Rejected and absent bodies never provide inferred lengths in preview.
	d := validPhysical(p)
	o := d.Objects[0]
	o.Body.Review.Status = StatusRejected
	if o.ProposalGeometry(o.Keyframes[0]).Length != nil {
		t.Fatal("rejected body used")
	}
	o.Body = nil
	if o.ProposalGeometry(o.Keyframes[0]).Length != nil {
		t.Fatal("missing body invented")
	}
}
