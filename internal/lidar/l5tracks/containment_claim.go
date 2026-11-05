package l5tracks

// Containment claim (W5, experiment l5id0011).
//
// The association cost is the squared Mahalanobis distance, and a coasting
// track's covariance is inflated on purpose, so a coasting track outbids a
// fresh one for the same cluster (gap analysis S3). The likelihood cost
// charges the coasting track for its vagueness; this rule asks the body
// instead. A confirmed track whose contained box, at its predicted pose,
// holds less than containmentClaimLevel of a cluster's points is not
// allowed that cluster when another bidder's box holds it, or when another
// bidder is not subject to the test. Where every bidder fails, nothing is
// refused. This preserves a bidder in each row, but not the cardinality of
// the global assignment: multiple clusters may need the same remaining track.
// The unguarded rule can therefore leave clusters unmatched and cause new births.
// l5id0018 optionally restores all refusals when maximum matching size falls.

// containmentClaimLevel is G5's containment level: a box holds a cluster
// when at least this share of the cluster's points lie inside it.
const containmentClaimLevel = 0.98

// applyContainmentClaim forbids, in place, the pairings the rule refuses.
// Each cluster's row is decided from the matrix as it stood on entry, so the
// order of the rows does not matter.
func (t *Tracker) applyContainmentClaim(costMatrix [][]float32, clusters []WorldCluster, clusterIdx []int, trackIDs []string) {
	if !t.Config.SolidBody.Enabled || !t.Config.SolidBody.Containment {
		return
	}
	forbidden := float32(hungarianlnf)
	var original [][]float32
	var refusalCount int64
	for row, ci := range clusterIdx {
		var refused []int
		others := 0
		for tj, trackID := range trackIDs {
			if hungarianForbidden(costMatrix[row][tj]) {
				continue
			}
			if t.boxCannotHold(t.Tracks[trackID], clusters[ci]) {
				refused = append(refused, tj)
			} else {
				others++
			}
		}
		if len(refused) == 0 || others == 0 {
			continue
		}
		if t.Config.ContainmentClaimCardinalityGuard && original == nil {
			original = make([][]float32, len(costMatrix))
			for i := range costMatrix {
				original[i] = append([]float32(nil), costMatrix[i]...)
			}
		}
		for _, tj := range refused {
			costMatrix[row][tj] = forbidden
			refusalCount++
		}
	}
	if original != nil && assignmentCardinality(costMatrix) < assignmentCardinality(original) {
		for i := range costMatrix {
			copy(costMatrix[i], original[i])
		}
		t.continuity.ContainmentClaimGuardFallbacks++
		return
	}
	t.continuity.ContainmentClaimRefusals += refusalCount
}

// HungarianAssign maximises cardinality before minimising cost. The guard
// compares that objective, not a row-wise count of remaining bidders.
func assignmentCardinality(cost [][]float32) int {
	count := 0
	for _, col := range HungarianAssign(cost) {
		if col >= 0 {
			count++
		}
	}
	return count
}

// boxCannotHold reports whether the rule applies to the track and its box
// fails to hold the cluster. A track is subject to the test only when it is
// confirmed, its solid body has an orientation and is referenced to the body
// centre, so its box is a body's, and the cluster carries enough points to
// judge, DefaultMinFaceSupport, as Containment's own measure requires.
func (t *Tracker) boxCannotHold(track *TrackedObject, cluster WorldCluster) bool {
	if track == nil || track.TrackState != TrackConfirmed {
		return false
	}
	sb := &track.solidBody
	if sb.orientation.Provenance == ProvenanceNone || sb.reference != ReferenceBodyCentre {
		return false
	}
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return false
	}
	e := t.assembleSolidBody(track, solidBodyClass(track))
	inside := boxContainment(points, float64(e.X), float64(e.Y), float64(e.Orientation.PsiRad),
		float64(e.Length.Metres)/2+DefaultFaceToleranceMetres, float64(e.Width.Metres)/2+DefaultFaceToleranceMetres)
	return float64(inside) < containmentClaimLevel*float64(len(points))
}
