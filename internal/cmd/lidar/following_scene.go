//go:build pcap

package lidar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/banshee-data/velocity.report/internal/scene"
)

// exportFollowingScene keeps the normal scene exporter and its per-export
// actor keys. Canonical/raw track IDs are never published in these frames.
func exportFollowingScene(s *followingreview.Session, out string) error {
	var rows []fieldrun.EvidenceRow
	var samples map[int64]map[string]scene.FollowingJSON
	var first, last int64
	provider := func(at int64) (map[string]scene.FollowingJSON, error) {
		if at < first || at >= last || rows == nil {
			found := false
			for _, slice := range s.Metadata.Slices {
				if at >= slice.Start && at < slice.End {
					first, last = slice.Start, slice.End
					found = true
					break
				}
			}
			if !found {
				return nil, nil
			}
			var err error
			_, rows, err = s.Evidence(at)
			if err != nil {
				return nil, err
			}
			samples = map[int64]map[string]scene.FollowingJSON{}
			for _, r := range rows {
				v := scene.FollowingJSON{Leader: r.Decision.LeaderTrackID, Bucket: r.Bucket}
				if r.Bucket == "following_eligible" && r.IntervalNanos > 0 && r.Point != nil {
					if r.Point.Gap != nil {
						gap := r.Point.Gap.ValueM
						v.GapM = &gap
					}
					if r.Point.TimeGap != nil {
						v.TimeGapS = r.Point.TimeGap.ValueS
					}
					if r.DRAC.Status == "supported" && r.DRAC.Calculation != nil {
						v.DRAC = r.DRAC.Calculation.ValueMPS2
					}
				}
				if samples[r.CaptureUnixNanos] == nil {
					samples[r.CaptureUnixNanos] = map[string]scene.FollowingJSON{}
				}
				samples[r.CaptureUnixNanos][r.FollowerTrackID] = v
			}
		}
		return samples[at], nil
	}
	_, err := scene.Export(scene.Options{VRLOGPath: s.Metadata.RecordingPath, OutDir: filepath.Join(out, "scene"), Kind: scene.KindTracks, Stride: 1, Site: s.Metadata.Binding.SiteID, Title: s.Metadata.Binding.SiteID,
		FollowingAt: provider, Following: &scene.FollowingProvenance{EvidenceSHA256: s.Metadata.EvidenceSHA256, CalibrationID: s.Metadata.CalibrationID, AnalysisBuild: s.Metadata.Binding.AnalysisBuildGitSHA, MethodID: s.Metadata.Binding.MethodID, Status: "provisional_estimates"}})
	if err != nil {
		return fmt.Errorf("following scene: %w", err)
	}
	manifest := scene.Manifest{Version: scene.FormatVersion, Site: scene.ManifestSite{ID: s.Metadata.Binding.SiteID, Title: s.Metadata.Binding.SiteID}, Parts: []scene.ManifestPart{{URL: "scene", StartSeconds: 0}}}
	// This closed manifest contains strings and literal finite numbers only.
	b, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(b, '\n'), 0644)
}
