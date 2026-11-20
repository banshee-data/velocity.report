package annotation

import "fmt"

// FollowingShellComparison is a read-only diagnostic, never a truth import or
// an acceptance score. It holds leader decisions and the sample cohort fixed.
type FollowingShellComparison struct {
	Schema                   string              `json:"schema"`
	Purpose                  string              `json:"purpose"`
	PackDigest               string              `json:"pack_digest"`
	BaselineRevision         int                 `json:"baseline_revision"`
	CandidateRevision        int                 `json:"candidate_revision"`
	BaselineDigest           string              `json:"baseline_digest"`
	CandidateDigest          string              `json:"candidate_digest"`
	MembershipDigest         string              `json:"membership_digest"`
	BaselineUnpinnedReviews  int                 `json:"baseline_unpinned_reviews"`
	CandidateUnpinnedReviews int                 `json:"candidate_unpinned_reviews"`
	Rows                     []FollowingShellRow `json:"rows"`
}

// FollowingShellRow retains missing endpoints instead of comparing only the
// frames where refinement succeeded. Delta is candidate minus frozen reference.
// Both gaps use the follower's axis in their own respective version.
type FollowingShellRow struct {
	FollowingID         string         `json:"following_id"`
	FollowerObjectID    string         `json:"follower_object_id"`
	LeaderObjectID      string         `json:"leader_object_id"`
	SampleID            int            `json:"sample_id"`
	TimestampNs         int64          `json:"timestamp_ns"`
	BaselineGap         *FollowingGap  `json:"baseline_gap,omitempty"`
	CandidateGap        *FollowingGap  `json:"candidate_gap,omitempty"`
	FollowerFront       *PlanarBound   `json:"follower_front,omitempty"`
	LeaderRear          *PlanarBound   `json:"leader_rear,omitempty"`
	FollowerFrontStatus EvidenceStatus `json:"follower_front_status"`
	LeaderRearStatus    EvidenceStatus `json:"leader_rear_status"`
	Unavailable         string         `json:"unavailable,omitempty"`
	DeltaM              *float64       `json:"delta_m,omitempty"`
}

// CompareFollowingShells compares validated documents from the same pack and
// membership. Callers must validate links and review pins before invoking it.
// Proposed geometry is explicitly inspected without promoting review status.
func CompareFollowingShells(p *Pack, baseline, candidate *PhysicalReferenceSet) (*FollowingShellComparison, error) {
	if err := baseline.Validate(p); err != nil {
		return nil, err
	}
	if err := candidate.Validate(p); err != nil {
		return nil, err
	}
	if baseline.PackDigest != p.Manifest.PackDigest || candidate.PackDigest != baseline.PackDigest ||
		baseline.DatasetID != candidate.DatasetID || baseline.Source != candidate.Source {
		return nil, fmt.Errorf("shell comparison requires identical pack, dataset and source identities")
	}
	r := &FollowingShellComparison{Schema: "velocity.report/following-shell-comparison/v1", Purpose: "proposal_diagnostic_not_accuracy_score",
		PackDigest: baseline.PackDigest, BaselineRevision: baseline.Revision, CandidateRevision: candidate.Revision,
		BaselineDigest: baseline.Digest(), CandidateDigest: candidate.Digest(),
		BaselineUnpinnedReviews: baseline.UnpinnedReviews(), CandidateUnpinnedReviews: candidate.UnpinnedReviews(), Rows: []FollowingShellRow{}}
	for _, f := range baseline.Following {
		if f.Decision != FollowingLeader || !f.Review.ScoredAsTruth() {
			continue
		}
		fo, _ := candidate.Object(f.FollowerObjectID)
		lo, _ := candidate.Object(f.LeaderObjectID)
		for _, s := range p.Samples {
			if !f.Interval.Contains(s.SampleID) {
				continue
			}
			r.Rows = append(r.Rows, compareShellRow(s, f, fo, lo))
		}
	}
	return r, nil
}

func compareShellRow(s Sample, f FollowingReference, fo, lo PhysicalObject) FollowingShellRow {
	row := FollowingShellRow{FollowingID: f.FollowingID, FollowerObjectID: f.FollowerObjectID,
		LeaderObjectID: f.LeaderObjectID, SampleID: s.SampleID, TimestampNs: s.TimestampNs,
		FollowerFrontStatus: EvidenceUnknown, LeaderRearStatus: EvidenceUnknown}
	for _, g := range f.Gaps {
		if g.SampleID == s.SampleID && g.Status.Scorable() {
			copy := g
			row.BaselineGap = &copy
			break
		}
	}
	fk, fok := keyframeAt(fo, s.SampleID)
	lk, lok := keyframeAt(lo, s.SampleID)
	switch {
	case !fok:
		row.Unavailable = "follower_no_keyframe"
	case !lok:
		row.Unavailable = "leader_no_keyframe"
	case fk.TimestampNs != s.TimestampNs || lk.TimestampNs != s.TimestampNs:
		row.Unavailable = "timestamp_mismatch"
	case fk.Review.Status == StatusRejected || lk.Review.Status == StatusRejected:
		row.Unavailable = "rejected_keyframe"
	default:
		fg, lg := fo.ProposalGeometry(fk), lo.ProposalGeometry(lk)
		row.FollowerFront, row.LeaderRear = fg.Front, lg.Rear
		row.FollowerFrontStatus, row.LeaderRearStatus = fk.Front.Status, lk.Rear.Status
		switch {
		case fg.Front == nil:
			row.Unavailable = "follower_front:" + fg.FrontUnavailable
		case lg.Rear == nil:
			row.Unavailable = "leader_rear:" + lg.RearUnavailable
		default:
			gap, reason := gapFromGeometry(fk, lk, fg, lg)
			if reason != "" {
				row.Unavailable = reason
				break
			}
			row.CandidateGap = &gap
			if row.BaselineGap != nil && row.BaselineGap.ValueM != nil {
				delta := *gap.ValueM - *row.BaselineGap.ValueM
				row.DeltaM = &delta
			}
		}
	}
	return row
}
