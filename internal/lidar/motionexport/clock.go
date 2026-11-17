package motionexport

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// ClockFit is one rotation's azimuth clock: block time as a straight line in
// unwrapped block azimuth. The rotor turns far more regularly than UDP
// packets arrive, so the line removes both the PCAP arrival jitter and the
// missing per-block offset (a sawtooth of up to ten 55.56 µs block
// separations per packet) that point times carry today.
type ClockFit struct {
	Blocks   int     `json:"blocks"`
	Rejected int     `json:"rejected"`
	PeriodS  float64 `json:"period_s"` // seconds per 360°
	// RMSResidUS and MaxResidUS are the recorded block times' residual about
	// the line over the blocks the fit kept. MaxRejectedResidUS is the largest
	// residual among the blocks the robust pass dropped (zero if none).
	RMSResidUS         float64 `json:"rms_resid_us"`
	MaxResidUS         float64 `json:"max_resid_us"`
	MaxRejectedResidUS float64 `json:"max_rejected_resid_us"`
	SpanDeg            float64 `json:"span_deg"`
	OK                 bool    `json:"ok"`
	Reason             string  `json:"reason,omitempty"`
}

type blockKey struct {
	packet uint64
	seq    uint32
	block  int
}

type block struct {
	t     float64 // mean point time, seconds since epoch
	az    float64 // unwrapped block azimuth, degrees
	keep  bool
	count int
}

// MaxRejectedShare is the largest fraction of a rotation's blocks the robust
// pass may drop before the clock fit is refused. Rejected blocks are timed
// from the line like the rest, which is right for a late packet and wrong if
// the line does not describe them (as when the seam was unwrapped a turn off),
// so a fit that rejects many is not trusted.
const MaxRejectedShare = 0.10

// MaxKeptRMSResidUS is the largest RMS residual of the kept blocks about the
// line, in microseconds, for the fit to be trusted. The missing per-block
// offsets alone put about 160 µs under any rotation; the first hydra exports
// had a median of 176–188 µs.
const MaxKeptRMSResidUS = 500.0

// PointTimes returns per-point times (seconds since epoch) for one frame.
// With useClock the block reference time comes from the azimuth line and each
// point keeps its own channel offset within its block; otherwise it is the
// point's recorded time. The fit is reported either way, so the residual
// shows what the recorded times carry.
func PointTimes(points []l2frames.Point, epoch time.Time, useClock bool) ([]float64, ClockFit) {
	rec := make([]float64, len(points))
	for i, p := range points {
		rec[i] = p.Timestamp.Sub(epoch).Seconds()
	}
	idx := map[blockKey]int{}
	var blocks []block
	of := make([]int, len(points))
	for i, p := range points {
		k := blockKey{p.PacketKey, p.UDPSequence, p.BlockID}
		j, ok := idx[k]
		if !ok {
			j = len(blocks)
			idx[k] = j
			blocks = append(blocks, block{az: float64(p.RawBlockAzimuth) / 100, keep: true})
		}
		blocks[j].t += rec[i]
		blocks[j].count++
		of[i] = j
	}
	for j := range blocks {
		blocks[j].t /= float64(blocks[j].count)
	}
	fit := ClockFit{Blocks: len(blocks)}
	if len(blocks) < 20 {
		fit.Reason = "fewer than 20 blocks"
		return rec, fit
	}
	// Unwrap in firing order: blocks are first seen in packet and block order,
	// as the frame builder appends parsed points. Do not reorder by mean block
	// time. Within a packet that differs only by which channels returned, so it
	// shuffles the packet's blocks, and where a packet straddles the 0/360°
	// seam some unwrap a whole turn off. Dual-return pairs share an azimuth and
	// stay equal.
	offset := 0.0
	prev := blocks[0].az
	for j := range blocks {
		a := blocks[j].az + offset
		for a < prev-180 {
			offset += 360
			a += 360
		}
		blocks[j].az = a
		prev = a
	}
	fit.SpanDeg = blocks[len(blocks)-1].az - blocks[0].az

	// Robust passes: drop blocks far off the line (late or bunched packets),
	// refit, and repeat until the kept set settles. One pass is not enough:
	// enough late blocks drag the first line, inflate the median residual
	// and so widen the cut that would have dropped them.
	t0, k := lineFit(blocks)
	for pass := 0; pass < 8; pass++ {
		var kept []float64
		res := make([]float64, len(blocks))
		for j, b := range blocks {
			res[j] = math.Abs(b.t - (t0 + k*b.az))
			if b.keep {
				kept = append(kept, res[j])
			}
		}
		sort.Float64s(kept)
		cut := math.Max(4*1.4826*kept[len(kept)/2], 50e-6)
		changed := false
		for j := range blocks {
			keep := res[j] <= cut
			if keep != blocks[j].keep {
				blocks[j].keep = keep
				changed = true
			}
		}
		t0, k = lineFit(blocks)
		if !changed {
			break
		}
	}
	for _, b := range blocks {
		if !b.keep {
			fit.Rejected++
		}
	}
	ss, mx, mxRej := 0.0, 0.0, 0.0
	kept := 0
	for _, b := range blocks {
		r := math.Abs(b.t - (t0 + k*b.az))
		if !b.keep {
			mxRej = math.Max(mxRej, r)
			continue
		}
		kept++
		ss += r * r
		mx = math.Max(mx, r)
	}
	fit.PeriodS = k * 360
	if kept > 0 {
		fit.RMSResidUS = math.Sqrt(ss/float64(kept)) * 1e6
	}
	fit.MaxResidUS = mx * 1e6
	fit.MaxRejectedResidUS = mxRej * 1e6
	switch {
	case float64(fit.Rejected) > MaxRejectedShare*float64(len(blocks)):
		fit.Reason = fmt.Sprintf("robust pass rejected %d of %d blocks (over %.0f%%)", fit.Rejected, len(blocks), 100*MaxRejectedShare)
	case fit.RMSResidUS > MaxKeptRMSResidUS:
		fit.Reason = fmt.Sprintf("kept-block residual %.0f µs RMS exceeds %.0f µs", fit.RMSResidUS, MaxKeptRMSResidUS)
	case fit.SpanDeg < 300:
		fit.Reason = "block azimuth spans less than 300°"
	case fit.PeriodS < 0.04 || fit.PeriodS > 0.25:
		fit.Reason = "rotation period outside 0.04-0.25 s"
	default:
		fit.OK = true
	}
	if !useClock || !fit.OK {
		return rec, fit
	}
	out := make([]float64, len(points))
	for i := range points {
		b := blocks[of[i]]
		out[i] = t0 + k*b.az + (rec[i] - b.t)
	}
	return out, fit
}

func lineFit(blocks []block) (t0, k float64) {
	var n, sx, sy, sxx, sxy float64
	for _, b := range blocks {
		if !b.keep {
			continue
		}
		n++
		sx += b.az
		sy += b.t
		sxx += b.az * b.az
		sxy += b.az * b.t
	}
	den := n*sxx - sx*sx
	if n < 2 || den == 0 {
		return sy / math.Max(n, 1), 0
	}
	k = (n*sxy - sx*sy) / den
	return (sy - k*sx) / n, k
}
