//go:build pcap

package lidar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/banshee-data/velocity.report/internal/scene"
)

// exportFollowingScene keeps the normal scene exporter and its per-export
// actor keys. Canonical/raw track IDs are never published in these frames.
func exportFollowingScene(s *followingreview.Session, out string, analysis followingreview.Analysis) error {
	filter, err := s.ReviewFilter(analysis.ReviewScope)
	if err != nil {
		return err
	}
	minima := map[string]float64{}
	for _, m := range analysis.Minima {
		minima[m.Follower] = m.TimeGapS
	}
	summary := &scene.FollowingSummary{Population: analysis.Population, Tracks: analysis.Tracks, WithoutTimeGap: analysis.WithoutTimeGap, Histogram: []scene.FollowingBin{}}
	for _, b := range analysis.Histogram {
		summary.Histogram = append(summary.Histogram, scene.FollowingBin{Label: b.Label, Count: b.Count})
	}
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
			geometry, err := s.Geometry(at, "")
			if err != nil {
				return nil, err
			}
			paths := map[string]*l8behaviour.LocalPath{}
			if geometry.RecordingCoordinates {
				for _, p := range geometry.Paths {
					paths[p.FollowerTrackID] = p.Path
				}
			}
			for _, r := range rows {
				if reason := filter(r); reason != "" {
					if samples[r.CaptureUnixNanos] == nil {
						samples[r.CaptureUnixNanos] = map[string]scene.FollowingJSON{}
					}
					samples[r.CaptureUnixNanos][r.FollowerTrackID] = scene.FollowingJSON{Bucket: reason, DRACStatus: "outside_static_review"}
					continue
				}
				v := scene.FollowingJSON{Leader: r.Decision.LeaderTrackID, Bucket: r.Bucket}
				v.FollowerSpeed, v.LeaderSpeed, v.DRACStatus = r.FollowerSpeedMPS, r.LeaderSpeedMPS, r.DRAC.Status
				if r.DRAC.Calculation != nil {
					v.DRACStatus = string(r.DRAC.Calculation.Status)
				}
				if r.Bucket == "following_eligible" && r.IntervalNanos > 0 && r.Point != nil {
					if r.Point.Gap != nil {
						gap := r.Point.Gap.ValueM
						v.GapM = &gap
					}
					if r.Point.TimeGap != nil {
						v.TimeGapS = r.Point.TimeGap.ValueS
					}
				}
				// DRAC admission is independent of headway's speed floor.
				if r.IntervalNanos > 0 && r.Point != nil {
					if r.DRAC.Status == "supported" && r.DRAC.Calculation != nil {
						v.DRAC = r.DRAC.Calculation.ValueMPS2
						if r.Point.Gap != nil {
							gap := r.Point.Gap.ValueM
							v.GapM = &gap
						}
					}
					if v.GapM != nil && r.Point.Follower != nil && r.Point.Leader != nil {
						v.GapPath = projectedGapPath(paths[r.FollowerTrackID], r.Point.Follower.Leading.ArcM, r.Point.Leader.Trailing.ArcM)
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
	_, err = scene.Export(scene.Options{VRLOGPath: s.Metadata.RecordingPath, OutDir: filepath.Join(out, "scene"), Kind: scene.KindTracks, Stride: 1, Site: s.Metadata.Binding.SiteID, Title: s.Metadata.Binding.SiteID,
		FollowingAt: provider, MinimumTimeGapByTrack: minima, Following: &scene.FollowingProvenance{EvidenceSHA256: s.Metadata.EvidenceSHA256, CalibrationID: s.Metadata.CalibrationID, AnalysisBuild: s.Metadata.Binding.AnalysisBuildGitSHA, MethodID: s.Metadata.Binding.MethodID, Status: "provisional_estimates", Summary: summary, ReviewScope: analysis.ReviewScope}})
	if err != nil {
		return fmt.Errorf("following scene: %w", err)
	}
	manifest := scene.Manifest{Version: scene.FormatVersion, Site: scene.ManifestSite{ID: s.Metadata.Binding.SiteID, Title: s.Metadata.Binding.SiteID}, Parts: []scene.ManifestPart{{URL: "scene", StartSeconds: 0}}}
	// This closed manifest contains strings and literal finite numbers only.
	b, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(b, '\n'), 0644)
}

// The measured quantity is an arc-length projection, not centre-to-centre
// distance. Draw that exact polyline; never extend a path beyond its support.
func projectedGapPath(p *l8behaviour.LocalPath, first, last float64) [][2]float64 {
	if p == nil || len(p.Vertices) < 2 || !(last > first) || first < p.Vertices[0].ArcM || last > p.Vertices[len(p.Vertices)-1].ArcM {
		return nil
	}
	point := func(at float64) [2]float64 {
		for i := 1; i < len(p.Vertices); i++ {
			a, b := p.Vertices[i-1], p.Vertices[i]
			if at <= b.ArcM {
				f := (at - a.ArcM) / (b.ArcM - a.ArcM)
				return [2]float64{a.X + f*(b.X-a.X), a.Y + f*(b.Y-a.Y)}
			}
		}
		b := p.Vertices[len(p.Vertices)-1]
		return [2]float64{b.X, b.Y}
	}
	out := [][2]float64{point(first)}
	for _, v := range p.Vertices {
		if v.ArcM > first && v.ArcM < last {
			out = append(out, [2]float64{v.X, v.Y})
		}
	}
	return append(out, point(last))
}
