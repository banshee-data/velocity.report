package l3grid

import (
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// Border traits of one return, read from its neighbours in the same channel's azimuth order.
// They are candidates, not physical labels: a veil bit says the range steps through the return
// monotonically between a nearer and a farther neighbour, which is what a mixed return looks
// like, and equally what a thin sloping surface looks like (l3bg0028, after the obstacle,
// shadow and veil classes of PCL's RangeImageBorderExtractor).
const (
	BorderObstacleLeft uint8 = 1 << iota // the left neighbour is farther: this return is the near edge
	BorderObstacleRight
	BorderShadowLeft // the left neighbour is nearer: this return is the far side of an occlusion
	BorderShadowRight
	BorderVeil // nearer on one side and farther on the other, both steps over the jump
)

// BorderParams sets the range step that counts as a border and the widest azimuth gap across
// which two returns are neighbours. A wider gap is an unknown, never a border.
type BorderParams struct {
	// JumpMetres and JumpFraction: a step is a border when it exceeds
	// max(JumpMetres, JumpFraction x range).
	JumpMetres   float64
	JumpFraction float64
	// MaxGapDegrees is the largest azimuth separation of neighbours.
	MaxGapDegrees float64
}

// DefaultBorderParams are fixed before any replay: 0.3 m or 4 % of range, and the 0.2 degree
// azimuth step of the Pandar40P at 10 Hz allowed to double.
func DefaultBorderParams() BorderParams {
	return BorderParams{JumpMetres: 0.3, JumpFraction: 0.04, MaxGapDegrees: 0.45}
}

// BorderTraits returns one trait byte per return, parallel to polar. Neighbours are the
// adjacent returns of the same channel in azimuth order (wrapping at 360), so the result does
// not depend on the order polar arrives in; ties in azimuth break on distance. Non-positive
// distances are missing returns and take part in nothing.
func BorderTraits(polar []l2frames.PointPolar, p BorderParams) []uint8 {
	traits := make([]uint8, len(polar))
	byChannel := make(map[int][]int)
	for i := range polar {
		if polar[i].Distance > 0 {
			byChannel[polar[i].Channel] = append(byChannel[polar[i].Channel], i)
		}
	}
	for _, idx := range byChannel {
		sort.Slice(idx, func(a, b int) bool {
			pa, pb := polar[idx[a]], polar[idx[b]]
			if pa.Azimuth != pb.Azimuth {
				return pa.Azimuth < pb.Azimuth
			}
			return pa.Distance < pb.Distance
		})
		n := len(idx)
		if n < 3 {
			continue
		}
		for k := 0; k < n; k++ {
			cur := polar[idx[k]]
			left, right := polar[idx[(k+n-1)%n]], polar[idx[(k+1)%n]]
			jump := math.Max(p.JumpMetres, p.JumpFraction*cur.Distance)
			var t uint8
			lKnown := azGap(left.Azimuth, cur.Azimuth) <= p.MaxGapDegrees
			rKnown := azGap(cur.Azimuth, right.Azimuth) <= p.MaxGapDegrees
			var lStep, rStep float64 // positive: the neighbour is farther
			if lKnown {
				lStep = left.Distance - cur.Distance
				switch {
				case lStep > jump:
					t |= BorderObstacleLeft
				case lStep < -jump:
					t |= BorderShadowLeft
				}
			}
			if rKnown {
				rStep = right.Distance - cur.Distance
				switch {
				case rStep > jump:
					t |= BorderObstacleRight
				case rStep < -jump:
					t |= BorderShadowRight
				}
			}
			if lKnown && rKnown && ((lStep < -jump && rStep > jump) || (lStep > jump && rStep < -jump)) {
				t |= BorderVeil
			}
			traits[idx[k]] = t
		}
	}
	return traits
}

// WithholdVeil returns a copy of mask with every foreground return that carries the veil trait
// cleared. A nil or short mask is returned unchanged. The second result counts the returns
// withheld.
func WithholdVeil(polar []l2frames.PointPolar, mask []bool, p BorderParams) ([]bool, int) {
	if len(mask) != len(polar) {
		return mask, 0
	}
	traits := BorderTraits(polar, p)
	out := make([]bool, len(mask))
	copy(out, mask)
	withheld := 0
	for i, t := range traits {
		if out[i] && t&BorderVeil != 0 {
			out[i] = false
			withheld++
		}
	}
	return out, withheld
}

func azGap(from, to float64) float64 {
	d := math.Mod(to-from, 360)
	if d < 0 {
		d += 360
	}
	return d
}
