package pipeline

import (
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// FragmentJoinStats counts experimental joining work over delivered clustered
// frames. Counts include warm-up; no physical correctness is implied.
type FragmentJoinStats struct {
	Frames           int `json:"frames"`
	FramesWithJoins  int `json:"frames_with_joins"`
	ComponentsJoined int `json:"components_joined"`
}

func (s *FragmentJoinStats) Observe(joined int) {
	s.Frames++
	if joined > 0 {
		s.FramesWithJoins++
		s.ComponentsJoined += joined
	}
}

// motionFragmentHints projects recent observed rigid vehicles to one common
// capture time. Stale, coasted, stopped, uncertain and unclassified bodies
// cannot join clusters. A footprint is only experimental support, not truth.
func motionFragmentHints(tracks []*l5tracks.TrackedObject, at time.Time, cfg config.L4FragmentJoin) []l4perception.FragmentHint {
	tracks = append([]*l5tracks.TrackedObject(nil), tracks...)
	sort.SliceStable(tracks, func(i, j int) bool { return tracks[i].CreationSequence < tracks[j].CreationSequence })
	var hints []l4perception.FragmentHint
	for _, tr := range tracks {
		if tr.TrackState != l5tracks.TrackConfirmed || tr.Misses != 0 || tr.ObservationCount < 8 || tr.LastSupport != l5tracks.SupportObserved {
			continue
		}
		switch tr.ObjectClass {
		case "vehicle", "car", "truck", "bus":
		default:
			continue
		}
		dt := float64(at.UnixNano()-tr.StateUnixNanos) / 1e9
		age := float64(at.UnixNano()-tr.LastObservedUnixNanos) / 1e9
		if tr.StateUnixNanos <= 0 || tr.LastObservedUnixNanos <= 0 || dt <= 0 || dt > cfg.MaxAgeSecs || age < 0 || age > cfg.MaxAgeSecs {
			continue
		}
		speed := math.Hypot(float64(tr.VX), float64(tr.VY))
		if !(speed >= cfg.MinSpeedMps) || math.IsInf(speed, 0) || tr.BoxLength < 3 || tr.BoxWidth < .5 || tr.BoxWidth > 4 {
			continue
		}
		// CV projection of the full position covariance to this frame. Do not
		// substitute a fixed isotropic certainty for a weak prior direction.
		p := tr.P
		xx := float64(p[0]) + dt*float64(p[2]+p[8]) + dt*dt*float64(p[10])
		yy := float64(p[5]) + dt*float64(p[7]+p[13]) + dt*dt*float64(p[15])
		xy := .5*float64(p[1]+p[4]) + dt*.5*float64(p[3]+p[9]+p[6]+p[12]) + dt*dt*.5*float64(p[11]+p[14])
		variance := .5 * (xx + yy + math.Hypot(xx-yy, 2*xy))
		if !(xx >= 0 && yy >= 0 && xx*yy >= xy*xy && variance <= cfg.MaxPositionStddevMetres*cfg.MaxPositionStddevMetres) {
			continue
		}
		hints = append(hints, l4perception.FragmentHint{ID: tr.TrackID, Rect: l4perception.Rect{
			X: float64(tr.BoxX) + dt*float64(tr.VX), Y: float64(tr.BoxY) + dt*float64(tr.VY),
			HeadingRad: float64(tr.BoxHeadingRad), Length: float64(tr.BoxLength), Width: float64(tr.BoxWidth),
		}})
	}
	return hints
}
