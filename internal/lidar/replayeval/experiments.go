package replayeval

import (
	"fmt"
	"sort"
	"strings"
)

// Experiments are the default-off, Go-level options a replay can switch on.
//
// They are deliberately not tuning keys: TuningConfig.Fingerprint hashes the
// whole resolved config, so a new key would move the fingerprint and make the
// committed perf baselines refuse to compare before anyone had measured
// whether the option helps. The offline harness is where that measurement
// happens, so this is the one place the options are reachable by name.
//
// A replay with experiments is a different estimator from one without, so the
// sorted list is folded into the parameter hash and written to the run
// metadata. An empty list leaves both exactly as they were.
const (
	// ExperimentAdaptiveUncertainty: l5tracks.TrackerConfig.AdaptiveMeasurementNoise,
	// state-estimation plan Phase 3: anisotropic R along and across the line
	// of sight. Uncalibrated unless Config.UncertaintyCalibrationFile supplies
	// a fitted table; see G-UNC-1.
	ExperimentAdaptiveUncertainty = "adaptive_uncertainty"
	// ExperimentLikelihoodCost: l5tracks.TrackerConfig.LikelihoodAssociationCost (gap analysis S3).
	ExperimentLikelihoodCost = "likelihood_cost"
	// ExperimentCascade: l5tracks.TrackerConfig.CascadedAssociation (S2).
	ExperimentCascade = "cascade"
	// ExperimentDensityCap: pipeline DensityPreservingCap (D6).
	ExperimentDensityCap = "density_cap"
	// ExperimentFlipRule: l5tracks.TrackerConfig.OBBHeadingFlipRule (P3).
	ExperimentFlipRule = "flip_rule"
	// ExperimentNoRegionOverrides: l3grid.BackgroundParams.DisableRegionOverrides (B8).
	ExperimentNoRegionOverrides = "no_region_overrides"
	// ExperimentMeasurementTime: l5tracks.TrackerConfig.MeasurementTimePrediction,
	// state-estimation plan question Q3: update each track at its cluster's
	// own acquisition time rather than the frame start.
	ExperimentMeasurementTime = "measurement_time"
	// ExperimentCaptureGapPredict: l5tracks.TrackerConfig.CaptureGapPrediction,
	// predict across a whole capture-time gap instead of clamping it to
	// max_predict_dt.
	ExperimentCaptureGapPredict = "capture_gap_predict"

	// Occlusion continuity (state-estimation plan, Sprint 0.5.2.2): the
	// switches in l5tracks.OcclusionContinuityConfig, each alone so a change
	// can be attributed, and all four together. Every one runs with
	// l5tracks.DefaultOcclusionContinuity's starting values.
	//
	// ExperimentCoastSupport: ExplainAbsence alone. Diagnostic: the tracks
	// are the default replay's exactly, and only the manifest's continuity
	// support counts split coasted into occluded_inferred, missed_unknown and
	// out_of_fov. It is the arm to read the shipped tracker's absences with.
	ExperimentCoastSupport = "coast_support"
	// ExperimentCoastTimeInflation: CaptureTimeInflation, uncertainty per
	// coast second rather than per missed frame.
	ExperimentCoastTimeInflation = "coast_time_inflation"
	// ExperimentClassCoastBounds: ClassCoastBounds, per-class capture-time
	// coast bounds in place of the miss count (implies absence explanation).
	ExperimentClassCoastBounds = "class_coast_bounds"
	// ExperimentReacquisitionGuard: ReacquisitionGuard.
	ExperimentReacquisitionGuard = "reacquisition_guard"
	// ExperimentOcclusionContinuity: all four. It does not imply
	// capture_gap_predict; name both for prediction in capture time too.
	ExperimentOcclusionContinuity = "occlusion_continuity"

	// ExperimentFixedLagRTS attaches the fixed-assignment RTS smoother at the
	// comparison horizons (three frames; 0.5, 1 and 2 s; the whole track) and
	// writes refinement_report.json; see refinement.go. It observes the
	// tracker and changes none of its decisions, but it is hashed like every
	// experiment, so the online rows of such a run carry their own hash.
	ExperimentFixedLagRTS = "fixed_lag_rts"

	// ExperimentSolidBody: l5tracks.TrackerConfig.SolidBody, the near-edge
	// solid-body estimate as a shadow of the tracked filter, written to
	// lidar_track_solid_bodies beside each point estimate. It never changes
	// the tracks, so its recording and tracking baseline are the default
	// replay's. It is useful only with ObservationDBPath, which is where its
	// rows go and what retains the cluster points the near-edge model reads;
	// without one it runs, says so, and writes nothing.
	ExperimentSolidBody = "solid_body"

	// ExperimentSolidBodyFaceHysteresis and ExperimentSolidBodyFaceConsider
	// are the near-edge tracked-state plan's face-transition remedies T1 and
	// T2 on the solid body (SolidBodyOptions.FaceHysteresis and
	// FaceEntryConsider). Each qualifies solid_body and is refused without it,
	// so a remedy can never run as an arm that has no solid body to change.
	ExperimentSolidBodyFaceHysteresis = "solid_body_face_hysteresis"
	ExperimentSolidBodyFaceConsider   = "solid_body_face_consider"
	// ExperimentSolidBodyCourseFaces is remedy T3
	// (SolidBodyOptions.CourseAlignedFaces): faces and spans along the solid
	// body's own course while it moves, not the lagging tracked heading.
	ExperimentSolidBodyCourseFaces = "solid_body_course_faces"
	// ExperimentSolidBodyFullMembers hands the tracker every cluster member
	// for the frame (pipeline KeepClusterMembers), so the solid body measures
	// faces from the members rather than the retained evidence sample. It
	// qualifies solid_body like the remedies, and changes no tracked decision.
	ExperimentSolidBodyFullMembers = "solid_body_full_members"
	// ExperimentSolidBodyReferenceTranslation qualifies solid_body with
	// SolidBodyOptions.ReferenceTranslation: the first fix translates the
	// medoid-referenced position to the body centre before the faces update
	// it (the near-edge plan's invariant 3). near_edge_track always does.
	ExperimentSolidBodyReferenceTranslation = "solid_body_reference_translation"
	// ExperimentSolidBodyRankOneMedoid is remedy T5 on the solid body's state
	// machine (SolidBodyOptions.RankOneMedoidScale at one): a fix by a front
	// or rear face alone, with no side face found, also takes the medoid
	// across the body, with A2's loose noise, R plus the believed half-width
	// squared. It applies to the shadow and, with near_edge_track, to the
	// tracked filter.
	// ExperimentSolidBodyRankOneMedoidTight is the same at a quarter of the
	// half-extent term, trusting the medoid to within half the half-extent.
	// They are two settings of one option, so a replay may name only one.
	ExperimentSolidBodyRankOneMedoid      = "solid_body_rank_one_medoid"
	ExperimentSolidBodyRankOneMedoidTight = "solid_body_rank_one_medoid_tight"
	// The solid body's extent and heading from one face
	// (lidar-solid-body-physical-alignment-plan.md), each qualifying
	// solid_body like the remedies:
	//   - ExperimentSolidBodyCourseHeading: the orientation is the course
	//     while the body moves (SolidBodyOptions.CourseHeading);
	//   - ExperimentSolidBodyExtentPriorFloor: a dimension shorter than the
	//     class prior keeps the prior (ExtentPriorFloor);
	//   - ExperimentSolidBodyFacePlaneSpans: a face gives the span along its
	//     own plane, not its depth (FacePlaneSpans);
	//   - ExperimentSolidBodyExtentGrowth: a merge candidate no wider than the
	//     body still gives extents (ExtentGrowthAdmission).
	ExperimentSolidBodyCourseHeading    = "solid_body_course_heading"
	ExperimentSolidBodyExtentPriorFloor = "solid_body_extent_prior_floor"
	ExperimentSolidBodyFacePlaneSpans   = "solid_body_face_plane_spans"
	ExperimentSolidBodyExtentGrowth     = "solid_body_extent_growth"
	// ExperimentSolidBodyVehicleExtentFloor: a rigid vehicle's accumulated
	// length and width are at least the smallest road car's
	// (SolidBodyOptions.VehicleExtentFloor).
	ExperimentSolidBodyVehicleExtentFloor = "solid_body_vehicle_extent_floor"
	// ExperimentNearEdgeTrack is l5tracks.TrackerConfig.NearEdgeTracking,
	// S2.2 of the near-edge plan: the solid body's state machine runs on the
	// tracked filter, with A2 face-residual association, so unlike the
	// shadow it changes the tracks. It names the solid body it runs, so it
	// needs solid_body, and solid_body_full_members, because the tracked
	// decisions then depend on the face geometry and the determinism repeat
	// runs without the observation database that holds the retained sample.
	ExperimentNearEdgeTrack = "near_edge_track"
	// ExperimentNearEdgeTrackA1 is S2.3's ablation arm
	// (l5tracks.TrackerConfig.NearEdgeMedoidGate): near_edge_track with its
	// body-centre tracks gated on the medoid against the predicted centre, as
	// a medoid-referenced track is, instead of A2's face residual. It
	// qualifies near_edge_track and is refused without it.
	ExperimentNearEdgeTrackA1 = "near_edge_track_a1"
)

var knownExperiments = map[string]bool{
	ExperimentAdaptiveUncertainty: true,
	ExperimentLikelihoodCost:      true,
	ExperimentCascade:             true,
	ExperimentDensityCap:          true,
	ExperimentFlipRule:            true,
	ExperimentNoRegionOverrides:   true,
	ExperimentMeasurementTime:     true,
	ExperimentCaptureGapPredict:   true,
	ExperimentCoastSupport:        true,
	ExperimentCoastTimeInflation:  true,
	ExperimentClassCoastBounds:    true,
	ExperimentReacquisitionGuard:  true,
	ExperimentOcclusionContinuity: true,
	ExperimentFixedLagRTS:         true,
	ExperimentSolidBody:           true,

	ExperimentSolidBodyFaceHysteresis: true,
	ExperimentSolidBodyFaceConsider:   true,
	ExperimentSolidBodyCourseFaces:    true,
	ExperimentSolidBodyFullMembers:    true,

	ExperimentSolidBodyReferenceTranslation: true,
	ExperimentSolidBodyRankOneMedoid:        true,
	ExperimentSolidBodyRankOneMedoidTight:   true,
	ExperimentSolidBodyCourseHeading:        true,
	ExperimentSolidBodyExtentPriorFloor:     true,
	ExperimentSolidBodyFacePlaneSpans:       true,
	ExperimentSolidBodyExtentGrowth:         true,
	ExperimentSolidBodyVehicleExtentFloor:   true,
	ExperimentNearEdgeTrack:                 true,
	ExperimentNearEdgeTrackA1:               true,
}

// KnownExperiments returns every accepted experiment name, sorted.
func KnownExperiments() []string {
	names := make([]string, 0, len(knownExperiments))
	for name := range knownExperiments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NormaliseExperiments validates names against the closed set and returns
// them sorted and de-duplicated, so the same selection always produces the
// same parameter hash whatever order it was written in. Blank entries are
// dropped; an unknown name is an error, never ignored, because a misspelt
// option would otherwise run as the baseline and be reported as the option.
func NormaliseExperiments(names []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !knownExperiments[name] {
			return nil, fmt.Errorf("unknown experiment %q (known: %s)", name, strings.Join(KnownExperiments(), ", "))
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ParseExperiments splits a comma-separated flag value and normalises it.
func ParseExperiments(flagValue string) ([]string, error) {
	if strings.TrimSpace(flagValue) == "" {
		return []string{}, nil
	}
	return NormaliseExperiments(strings.Split(flagValue, ","))
}

func hasExperiment(experiments []string, name string) bool {
	for _, e := range experiments {
		if e == name {
			return true
		}
	}
	return false
}

// experimentsHashSuffix is appended to the marshalled tuning config before it
// is hashed. Empty for no experiments, so existing parameter hashes stand.
func experimentsHashSuffix(experiments []string) []byte {
	if len(experiments) == 0 {
		return nil
	}
	return []byte("\nexperiments:" + strings.Join(experiments, ","))
}
