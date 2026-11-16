package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// A slice is a byte-verifiable projection of the canonical export, never a
// second analysis. Sample anchors partition the recording in one-minute bins;
// original intervals and suppressed results are retained without clipping.
type sliceMetadata struct {
	CanonicalEvidenceSHA256 string `json:"canonical_evidence_sha256"`
	SampleStartNanos        int64  `json:"sample_start_nanos"`
	SampleEndExclusiveNanos int64  `json:"sample_end_exclusive_nanos"`
	RecordingHeaderSHA256   string `json:"recording_header_sha256"`
	RecordingBuildGitSHA    string `json:"recording_build_git_sha"`
	CalibrationID           string `json:"calibration_id"`
}

type timelineIdentity struct {
	FollowerTrackID string `json:"follower_track_id"`
	GeometryID      string `json:"geometry_id,omitempty"`
}

type reviewSlice struct {
	Schema      string                         `json:"schema"`
	Context     fieldrun.EvidenceContext       `json:"context"`
	Version     l8behaviour.InteractionVersion `json:"analysis_version"`
	ReviewSlice sliceMetadata                  `json:"review_slice"`
	Analysis    struct {
		Timelines []timelineIdentity `json:"timelines"`
	} `json:"analysis"`
	Rows []fieldrun.EvidenceRow `json:"rows"`
}

type sliceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Start  int64  `json:"sample_start_nanos"`
	End    int64  `json:"sample_end_exclusive_nanos"`
	Rows   int    `json:"rows"`
}

// Compact JSON avoids the pretty-printer's second multi-gigabyte buffer for
// full captures. The schema, derived content and exact-byte digest are retained.
func writeCanonical(output string, d fieldrun.EvidenceDocument) (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	path := filepath.Join(output, "canonical-following-evidence.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	err = json.NewEncoder(f).Encode(d)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, nil
}

func makeSlices(d fieldrun.EvidenceDocument, h recorder.LogHeader, canonicalSHA, headerSHA, calibrationID string) []reviewSlice {
	const minute = int64(60_000_000_000)
	bins := map[int64][]fieldrun.EvidenceRow{}
	for _, row := range d.Rows {
		bins[(row.CaptureUnixNanos-h.StartNs)/minute] = append(bins[(row.CaptureUnixNanos-h.StartNs)/minute], row)
	}
	keys := make([]int64, 0, len(bins))
	for k := range bins {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]reviewSlice, 0, len(keys))
	for _, k := range keys {
		end := min(h.StartNs+(k+1)*minute, h.EndNs+1)
		s := reviewSlice{Schema: "following_review_slice_v1", Context: d.Context, Version: d.Version,
			ReviewSlice: sliceMetadata{canonicalSHA, h.StartNs + k*minute, end, headerSHA, h.BuildGitSHA, calibrationID}, Rows: bins[k]}
		ids := map[string]bool{}
		for _, row := range s.Rows {
			ids[row.FollowerTrackID] = true
		}
		for _, tl := range d.Analysis.Timelines {
			if ids[tl.FollowerTrackID] {
				s.Analysis.Timelines = append(s.Analysis.Timelines, timelineIdentity{tl.FollowerTrackID, tl.GeometryID})
			}
		}
		out = append(out, s)
	}
	return out
}

func writeSlices(output, canonical, vrlog, calibrationID string, d fieldrun.EvidenceDocument, h recorder.LogHeader) ([]sliceFile, error) {
	canonicalSHA, err := eligibility.FileSHA256(canonical)
	if err != nil {
		return nil, err
	}
	headerSHA, err := eligibility.FileSHA256(filepath.Join(vrlog, "header.json"))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(output, "review-slices")
	if err = os.Mkdir(dir, 0755); err != nil {
		return nil, err
	}
	var files []sliceFile
	for i, s := range makeSlices(d, h, canonicalSHA, headerSHA, calibrationID) {
		path := filepath.Join(dir, fmt.Sprintf("minute-%03d.json", i))
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			return nil, e
		}
		e = json.NewEncoder(f).Encode(s)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		sha, e := eligibility.FileSHA256(path)
		if e != nil {
			return nil, e
		}
		files = append(files, sliceFile{path, sha, s.ReviewSlice.SampleStartNanos, s.ReviewSlice.SampleEndExclusiveNanos, len(s.Rows)})
	}
	return files, nil
}
