package lidar

import (
	"fmt"
	"strings"
)

// Ground Truth Evaluation Engine
//
// This module implements track comparison for ground truth evaluation:
// - Match candidate tracks against labelled reference tracks using temporal IoU
// - Compute detection rates, fragmentation, false positives, and quality metrics
// - Support label-aware auto-tuning with composite scoring

// isPositiveLabel returns true if the classification label represents a real
// detection (car, ped) rather than noise. These are the reference-worthy labels.
func isPositiveLabel(label string) bool {
	return label == "car" || label == "ped"
}

// hasQualityFlag returns true if the comma-separated quality label string
// contains the specified flag.
func hasQualityFlag(qualityLabel, flag string) bool {
	for _, f := range strings.Split(qualityLabel, ",") {
		if strings.TrimSpace(f) == flag {
			return true
		}
	}
	return false
}

// GroundTruthWeights holds the weights for computing composite ground truth scores.
// These weights control the relative importance of each metric in the overall score.
type GroundTruthWeights struct {
	DetectionRate     float64 `json:"detection_rate"`      // w1: Weight for detection rate (matched good tracks)
	Fragmentation     float64 `json:"fragmentation"`       // w2: Penalty for track splits
	FalsePositives    float64 `json:"false_positives"`     // w3: Penalty for unmatched candidate tracks
	VelocityCoverage  float64 `json:"velocity_coverage"`   // w4: Bonus for tracks with velocity data
	QualityPremium    float64 `json:"quality_premium"`     // w5: Bonus for "good" quality tracks
	TruncationRate    float64 `json:"truncation_rate"`     // w6: Penalty for truncated tracks
	VelocityNoiseRate float64 `json:"velocity_noise_rate"` // w7: Penalty for noisy velocity tracks
	StoppedRecovery   float64 `json:"stopped_recovery"`    // w8: Bonus for stopped vehicle recovery
}

// DefaultGroundTruthWeights returns the default weights from the design doc.
// Composite score = w1*detection - w2*fragmentation - w3*FP + w4*velocity_coverage
//   - w5*quality_premium - w6*truncation - w7*velocity_noise + w8*stopped_recovery
func DefaultGroundTruthWeights() GroundTruthWeights {
	return GroundTruthWeights{
		DetectionRate:     1.0,
		Fragmentation:     5.0,
		FalsePositives:    2.0,
		VelocityCoverage:  0.5,
		QualityPremium:    0.3,
		TruncationRate:    0.4,
		VelocityNoiseRate: 0.4,
		StoppedRecovery:   0.2,
	}
}

// GroundTruthScore holds the evaluation results for a candidate run against reference ground truth.
type GroundTruthScore struct {
	DetectionRate        float64            `json:"detection_rate"`          // Fraction of reference good tracks matched
	DetectionRateByClass map[string]float64 `json:"detection_rate_by_class"` // Per-class detection rates
	Fragmentation        float64            `json:"fragmentation"`           // Fraction of reference tracks split into multiple candidates
	FalsePositiveRate    float64            `json:"false_positive_rate"`     // Fraction of candidate tracks not matching any reference
	VelocityCoverage     float64            `json:"velocity_coverage"`       // Fraction of matched tracks with velocity data
	QualityPremium       float64            `json:"quality_premium"`         // Fraction of matched tracks with "good" quality
	TruncationRate       float64            `json:"truncation_rate"`         // Fraction of matched tracks with "truncated" quality
	VelocityNoiseRate    float64            `json:"velocity_noise_rate"`     // Fraction of matched tracks with "jitter_velocity" quality
	StoppedRecoveryRate  float64            `json:"stopped_recovery_rate"`   // Fraction of stopped tracks with "disconnected" quality
	CompositeScore       float64            `json:"composite_score"`         // Weighted composite score
	MatchedCount         int                `json:"matched_count"`           // Number of reference tracks matched
	ReferenceCount       int                `json:"reference_count"`         // Total number of reference good tracks
	CandidateCount       int                `json:"candidate_count"`         // Total number of candidate tracks
	Matches              []TrackMatchResult `json:"matches,omitempty"`       // Individual track matches
}

// TrackMatchResult represents a single matched track pair with matching metrics.
type TrackMatchResult struct {
	ReferenceTrackID string  `json:"reference_track_id"` // Track ID from reference run
	CandidateTrackID string  `json:"candidate_track_id"` // Track ID from candidate run
	TemporalIoU      float64 `json:"temporal_iou"`       // Time overlap IoU (0-1)
	SpatialDistance  float64 `json:"spatial_distance"`   // Approximate spatial distance (not yet implemented)
}

// computeTemporalIoU calculates the temporal intersection-over-union for two tracks.
// IoU = intersection / union of time ranges [StartUnixNanos, EndUnixNanos].
// Returns a value in [0, 1] where 1 means perfect temporal alignment.
func computeTemporalIoU(ref, cand *RunTrack) float64 {
	// Calculate intersection: max(starts) to min(ends)
	intersectionStart := ref.StartUnixNanos
	if cand.StartUnixNanos > intersectionStart {
		intersectionStart = cand.StartUnixNanos
	}

	intersectionEnd := ref.EndUnixNanos
	if cand.EndUnixNanos < intersectionEnd {
		intersectionEnd = cand.EndUnixNanos
	}

	// If no overlap, IoU is 0
	if intersectionStart >= intersectionEnd {
		return 0.0
	}

	intersection := float64(intersectionEnd - intersectionStart)

	// Calculate union: min(starts) to max(ends)
	unionStart := ref.StartUnixNanos
	if cand.StartUnixNanos < unionStart {
		unionStart = cand.StartUnixNanos
	}

	unionEnd := ref.EndUnixNanos
	if cand.EndUnixNanos > unionEnd {
		unionEnd = cand.EndUnixNanos
	}

	union := float64(unionEnd - unionStart)

	if union <= 0 {
		return 0.0
	}

	return intersection / union
}

// matchTracks performs optimal bipartite matching between reference and candidate tracks
// using temporal IoU as the matching criterion. Returns a list of matched track pairs.
//
// Matching criterion: IoU > 0.3 (design doc requirement)
// Uses the Hungarian algorithm for optimal assignment to avoid greedy matching issues.
func matchTracks(reference, candidate []*RunTrack) []TrackMatchResult {
	if len(reference) == 0 || len(candidate) == 0 {
		return nil
	}

	// Build cost matrix: cost[i][j] = 1.0 - IoU between reference[i] and candidate[j]
	// Forbidden pairs (IoU <= 0.3) are set to hungarianlnf (1e18)
	const iouThreshold = 0.3
	const forbiddenCost = 1e18

	costMatrix := make([][]float32, len(reference))
	iouMatrix := make([][]float64, len(reference))

	for i, ref := range reference {
		costMatrix[i] = make([]float32, len(candidate))
		iouMatrix[i] = make([]float64, len(candidate))

		for j, cand := range candidate {
			iou := computeTemporalIoU(ref, cand)
			iouMatrix[i][j] = iou

			if iou > iouThreshold {
				// Valid match: cost = 1.0 - IoU (lower is better)
				costMatrix[i][j] = float32(1.0 - iou)
			} else {
				// Forbidden match
				costMatrix[i][j] = forbiddenCost
			}
		}
	}

	// Use Hungarian algorithm for optimal assignment
	assignments := HungarianAssign(costMatrix)

	// Extract matches
	var matches []TrackMatchResult
	for i, candIdx := range assignments {
		if candIdx >= 0 && candIdx < len(candidate) {
			// Check that this is a valid match (not forbidden)
			if costMatrix[i][candIdx] < forbiddenCost {
				matches = append(matches, TrackMatchResult{
					ReferenceTrackID: reference[i].TrackID,
					CandidateTrackID: candidate[candIdx].TrackID,
					TemporalIoU:      iouMatrix[i][candIdx],
					SpatialDistance:  0.0, // Not yet implemented (would need observation data)
				})
			}
		}
	}

	return matches
}

// EvaluateGroundTruth compares candidate tracks against reference ground truth tracks
// and computes a comprehensive score with multiple quality metrics.
//
// Only reference tracks with classification label "car" or "ped" are considered ground truth.
// Tracks labelled "noise" are filtered out of the reference set.
// All candidate tracks are evaluated to detect false positives.
func EvaluateGroundTruth(reference, candidate []*RunTrack, weights GroundTruthWeights) *GroundTruthScore {
	// Filter reference tracks to only those labelled as positive detections (car, ped)
	var goodReferenceTracks []*RunTrack
	for _, track := range reference {
		if isPositiveLabel(track.UserLabel) {
			goodReferenceTracks = append(goodReferenceTracks, track)
		}
	}

	score := &GroundTruthScore{
		DetectionRateByClass: make(map[string]float64),
		ReferenceCount:       len(goodReferenceTracks),
		CandidateCount:       len(candidate),
	}

	// If no reference tracks, return zero score
	if len(goodReferenceTracks) == 0 {
		return score
	}

	// Perform optimal matching
	matches := matchTracks(goodReferenceTracks, candidate)
	score.Matches = matches
	score.MatchedCount = len(matches)

	// Build set of matched candidate IDs for false positive calculation
	matchedCandidateIDs := make(map[string]bool)
	for _, match := range matches {
		matchedCandidateIDs[match.CandidateTrackID] = true
	}

	// Build map from reference track ID to match for quality metrics
	refToMatch := make(map[string]TrackMatchResult)
	for _, match := range matches {
		refToMatch[match.ReferenceTrackID] = match
	}

	// Compute detection rate: matched / total good tracks
	if len(goodReferenceTracks) > 0 {
		score.DetectionRate = float64(len(matches)) / float64(len(goodReferenceTracks))
	}

	// Compute detection rate by class (car, ped, noise)
	classCounts := make(map[string]int)
	classMatched := make(map[string]int)

	for _, ref := range goodReferenceTracks {
		classCounts[ref.UserLabel]++
		if _, matched := refToMatch[ref.TrackID]; matched {
			classMatched[ref.UserLabel]++
		}
	}

	for class, total := range classCounts {
		if total > 0 {
			score.DetectionRateByClass[class] = float64(classMatched[class]) / float64(total)
		}
	}

	// Compute false positive rate: unmatched candidates / total candidates
	if len(candidate) > 0 {
		unmatchedCount := len(candidate) - len(matchedCandidateIDs)
		score.FalsePositiveRate = float64(unmatchedCount) / float64(len(candidate))
	}

	// Compute quality metrics for matched tracks
	// These require checking the candidate track's quality_label field
	var (
		tracksWithVelocity    int
		perfectQualityTracks  int
		truncatedTracks       int
		noisyVelocityTracks   int
		stoppedRecoveryTracks int
	)

	// Build candidate track map for quick lookup
	candTrackMap := make(map[string]*RunTrack)
	for _, cand := range candidate {
		candTrackMap[cand.TrackID] = cand
	}

	for _, ref := range goodReferenceTracks {
		match, matched := refToMatch[ref.TrackID]
		if !matched {
			continue
		}

		cand, found := candTrackMap[match.CandidateTrackID]
		if !found {
			continue
		}

		// Velocity coverage: check if candidate has non-zero speed
		if cand.AvgSpeedMps > 0 {
			tracksWithVelocity++
		}

		// Quality metrics based on quality_label (may contain comma-separated flags)
		if hasQualityFlag(cand.QualityLabel, "good") {
			perfectQualityTracks++
		}
		if hasQualityFlag(cand.QualityLabel, "truncated") {
			truncatedTracks++
		}
		if hasQualityFlag(cand.QualityLabel, "jitter_velocity") {
			noisyVelocityTracks++
		}
		if hasQualityFlag(cand.QualityLabel, "disconnected") {
			stoppedRecoveryTracks++
		}
	}

	matchedCount := len(matches)
	if matchedCount > 0 {
		score.VelocityCoverage = float64(tracksWithVelocity) / float64(matchedCount)
		score.QualityPremium = float64(perfectQualityTracks) / float64(matchedCount)
		score.TruncationRate = float64(truncatedTracks) / float64(matchedCount)
		score.VelocityNoiseRate = float64(noisyVelocityTracks) / float64(matchedCount)
		score.StoppedRecoveryRate = float64(stoppedRecoveryTracks) / float64(matchedCount)
	}

	// Compute fragmentation: reference tracks matched by >1 candidate
	// For now, with simple matching, this is 0 (each reference matched to at most one candidate)
	// Future enhancement: detect splits by checking for multiple candidates with IoU > threshold
	score.Fragmentation = 0.0

	// Compute composite score using weights
	// Positive contributions: detection, velocity coverage, quality premium, stopped recovery
	positiveScore := weights.DetectionRate*score.DetectionRate +
		weights.VelocityCoverage*score.VelocityCoverage +
		weights.QualityPremium*score.QualityPremium +
		weights.StoppedRecovery*score.StoppedRecoveryRate

	// Negative contributions: fragmentation, false positives, truncation, velocity noise
	negativeScore := weights.Fragmentation*score.Fragmentation +
		weights.FalsePositives*score.FalsePositiveRate +
		weights.TruncationRate*score.TruncationRate +
		weights.VelocityNoiseRate*score.VelocityNoiseRate

	score.CompositeScore = positiveScore - negativeScore

	return score
}

// GroundTruthEvaluator provides ground truth evaluation services using stored analysis runs.
type GroundTruthEvaluator struct {
	store   *AnalysisRunStore
	weights GroundTruthWeights
}

// NewGroundTruthEvaluator creates a new evaluator with the specified weights.
func NewGroundTruthEvaluator(store *AnalysisRunStore, weights GroundTruthWeights) *GroundTruthEvaluator {
	return &GroundTruthEvaluator{
		store:   store,
		weights: weights,
	}
}

// Evaluate compares a candidate run against a reference run and returns a ground truth score.
// Returns an error if either run cannot be loaded.
func (e *GroundTruthEvaluator) Evaluate(referenceRunID, candidateRunID string) (*GroundTruthScore, error) {
	// Fetch reference tracks
	referenceTracks, err := e.store.GetRunTracks(referenceRunID)
	if err != nil {
		return nil, fmt.Errorf("failed to load reference tracks: %w", err)
	}

	// Fetch candidate tracks
	candidateTracks, err := e.store.GetRunTracks(candidateRunID)
	if err != nil {
		return nil, fmt.Errorf("failed to load candidate tracks: %w", err)
	}

	// Evaluate
	score := EvaluateGroundTruth(referenceTracks, candidateTracks, e.weights)
	return score, nil
}
