// Package followingreview opens immutable, run-bound preparation receipts for
// operator review. It does not rerun analysis or infer evidence from playback.
package followingreview

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/google/uuid"
)

// Binding uses the native audit document's existing camelCase wire format.
type Binding struct {
	EvidenceSHA256      string                    `json:"evidenceSHA256"`
	SiteNamespace       string                    `json:"siteNamespace"`
	SiteID              string                    `json:"siteID"`
	SourceID            string                    `json:"sourceID"`
	BindingID           string                    `json:"bindingID"`
	PolicyID            string                    `json:"policyID"`
	AnalysisBuildGitSHA string                    `json:"analysisBuildGitSHA"`
	EstimateStage       l8behaviour.EstimateStage `json:"estimateStage"`
	EstimatorID         string                    `json:"estimatorID"`
	ObservationModelID  string                    `json:"observationModelID"`
	MethodID            string                    `json:"methodID"`
	ParameterHash       string                    `json:"parameterHash"`
}

func binding(c fieldrun.EvidenceContext, v l8behaviour.InteractionVersion, hash string) Binding {
	return Binding{hash, c.SiteNamespace, c.SiteID, c.SourceID, c.BindingID, c.PolicyID, c.BuildGitSHA, v.EstimateStage, v.EstimatorID, v.ObsModelID, v.MethodID, v.ParamHash}
}

type Playback struct {
	RunID                      string   `json:"runID,omitempty"`
	SourcePath                 string   `json:"sourcePath,omitempty"`
	RecordingPath              string   `json:"recordingPath,omitempty"`
	CaptureNanos               int64    `json:"captureNanos"`
	StartNanos                 int64    `json:"startNanos"`
	EndNanos                   int64    `json:"endNanos"`
	CoordinateFrameID          string   `json:"coordinateFrameID,omitempty"`
	FrameSourceIDs             []string `json:"frameSourceIDs"`
	FrameRecordingPath         string   `json:"frameRecordingPath,omitempty"`
	FrameRecordingHeaderSHA256 string   `json:"frameRecordingHeaderSHA256,omitempty"`
}

type Record struct {
	ID                    string    `json:"id"`
	Reviewer              string    `json:"reviewer"`
	SavedAt               time.Time `json:"savedAt"`
	FollowerTrackID       string    `json:"followerTrackID"`
	ProposedLeaderTrackID *string   `json:"proposedLeaderTrackID,omitempty"`
	StartNanos            int64     `json:"startNanos"`
	EndExclusiveNanos     int64     `json:"endExclusiveNanos"`
	IdentityContinuity    string    `json:"identityContinuity"`
	LeaderAndPath         string    `json:"leaderAndPath"`
	VisibleEndpoints      string    `json:"visibleEndpoints"`
	SpeedEvidence         string    `json:"speedEvidence"`
	Conclusion            string    `json:"conclusion"`
	Notes                 string    `json:"notes"`
	Playback              Playback  `json:"playback"`
}

type Audit struct {
	Schema  string            `json:"schema"`
	Binding Binding           `json:"binding"`
	Records []json.RawMessage `json:"records"`
}

type Slice struct {
	ID    string `json:"id"`
	Start int64  `json:"start_nanos"`
	End   int64  `json:"end_exclusive_nanos"`
	Rows  int    `json:"rows"`
}

type Metadata struct {
	Schema         string  `json:"schema"`
	RunID          string  `json:"run_id"`
	EvidenceSHA256 string  `json:"evidence_sha256"`
	HeaderSHA256   string  `json:"recording_header_sha256"`
	RecordingPath  string  `json:"recording_path"`
	SourceID       string  `json:"source_id"`
	CalibrationID  string  `json:"calibration_id"`
	Start          int64   `json:"start_nanos"`
	End            int64   `json:"end_exclusive_nanos"`
	Slices         []Slice `json:"slices"`
	Binding        Binding `json:"binding"`
}

type sliceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Start  int64  `json:"sample_start_nanos"`
	End    int64  `json:"sample_end_exclusive_nanos"`
	Rows   int    `json:"rows"`
}

type receipt struct {
	Schema      string                   `json:"schema"`
	Context     fieldrun.EvidenceContext `json:"context"`
	Calibration l4bobserve.Calibration   `json:"calibration"`
	Header      recorder.LogHeader       `json:"recording_header"`
	VRLOG       string                   `json:"vrlog"`
	Evidence    string                   `json:"evidence"`
	Rows        int                      `json:"rows"`
	Purpose     string                   `json:"purpose"`
	Slices      []sliceFile              `json:"slices"`
}

type sliceMeta struct {
	CanonicalSHA  string `json:"canonical_evidence_sha256"`
	Start         int64  `json:"sample_start_nanos"`
	End           int64  `json:"sample_end_exclusive_nanos"`
	HeaderSHA     string `json:"recording_header_sha256"`
	Build         string `json:"recording_build_git_sha"`
	CalibrationID string `json:"calibration_id"`
}

type reviewSlice struct {
	Schema  string                         `json:"schema"`
	Context fieldrun.EvidenceContext       `json:"context"`
	Version l8behaviour.InteractionVersion `json:"analysis_version"`
	Review  sliceMeta                      `json:"review_slice"`
	Rows    []fieldrun.EvidenceRow         `json:"rows"`
}

// ValidateRecord admits observations only at an exact retained follower sample.
// The caller supplies the slice containing StartNanos; review cannot silently
// cross a slice boundary whose evidence has not been loaded and verified.
func ValidateRecord(r Record, rows []fieldrun.EvidenceRow, runID string, start, end int64) error {
	if _, err := uuid.Parse(r.ID); err != nil {
		return fmt.Errorf("audit id must be a UUID")
	}
	if strings.TrimSpace(r.Reviewer) == "" || len(r.Reviewer) > 256 || strings.TrimSpace(r.Notes) == "" || len(r.Notes) > 16384 || r.SavedAt.IsZero() || r.SavedAt.Location() != time.UTC {
		return fmt.Errorf("audit requires bounded reviewer, notes and UTC savedAt")
	}
	if r.StartNanos < start || r.StartNanos >= end || r.EndExclusiveNanos < r.StartNanos || r.EndExclusiveNanos > end {
		return fmt.Errorf("audit period is outside verified slice")
	}
	anchor, limit := false, int64(math.MinInt64)
	for _, row := range rows {
		if row.FollowerTrackID != r.FollowerTrackID {
			continue
		}
		if row.CaptureUnixNanos == r.StartNanos {
			anchor = true
			if r.ProposedLeaderTrackID != nil {
				candidate := false
				for _, c := range row.Decision.Candidates {
					if *r.ProposedLeaderTrackID != "" && c.TrackID == *r.ProposedLeaderTrackID && c.TrackID != r.FollowerTrackID {
						candidate = true
					}
				}
				if !candidate {
					return fmt.Errorf("proposed leader must be an exact-row candidate distinct from follower")
				}
			}
		}
		if row.IntervalNanos < 0 || row.CaptureUnixNanos > math.MaxInt64-row.IntervalNanos {
			return fmt.Errorf("invalid evidence interval")
		}
		limit = max(limit, row.CaptureUnixNanos+row.IntervalNanos)
	}
	if !anchor || r.EndExclusiveNanos > limit {
		return fmt.Errorf("audit must start at exact follower sample and stay within evidence")
	}
	consistent := true
	for _, v := range []string{r.IdentityContinuity, r.LeaderAndPath, r.VisibleEndpoints, r.SpeedEvidence} {
		switch v {
		case "consistent":
		case "unknown", "disputed", "notVisible":
			consistent = false
		default:
			return fmt.Errorf("invalid observation")
		}
	}
	switch r.Conclusion {
	case "consistent":
		if !consistent {
			return fmt.Errorf("consistent conclusion requires four consistent observations")
		}
	case "insufficientEvidence", "disagreement":
	default:
		return fmt.Errorf("invalid conclusion")
	}
	if r.Playback.RunID != "" && r.Playback.RunID != runID {
		return fmt.Errorf("playback run differs from review run")
	}
	if len(r.Playback.RunID) > 4096 || len(r.Playback.SourcePath) > 4096 || len(r.Playback.RecordingPath) > 4096 || len(r.Playback.CoordinateFrameID) > 256 || len(r.Playback.FrameSourceIDs) > 64 {
		return fmt.Errorf("playback context exceeds bounded review fields")
	}
	for _, id := range r.Playback.FrameSourceIDs {
		if len(id) > 512 {
			return fmt.Errorf("playback source identity exceeds review limit")
		}
	}
	if len(r.Playback.FrameRecordingPath) > 4096 || len(r.Playback.FrameRecordingHeaderSHA256) > 71 {
		return fmt.Errorf("frame recording context exceeds review limit")
	}
	return nil
}
