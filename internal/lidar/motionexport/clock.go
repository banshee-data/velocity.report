package motionexport

import (
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
	Blocks     int     `json:"blocks"`
	Rejected   int     `json:"rejected"`
	PeriodS    float64 `json:"period_s"`     // seconds per 360°
	RMSResidUS float64 `json:"rms_resid_us"` // packet-time residual about the line
	MaxResidUS float64 `json:"max_resid_us"`
	SpanDeg    float64 `json:"span_deg"`
	OK         bool    `json:"ok"`
	Reason     string  `json:"reason,omitempty"`
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

	t0, k := lineFit(blocks)
	// One robust pass: drop blocks far off the line (late or bunched packets).
	res := make([]float64, 0, len(blocks))
	for _, b := range blocks {
		res = append(res, math.Abs(b.t-(t0+k*b.az)))
	}
	sorted := append([]float64(nil), res...)
	sort.Float64s(sorted)
	cut := math.Max(4*1.4826*sorted[len(sorted)/2], 50e-6)
	for j := range blocks {
		if res[j] > cut {
			blocks[j].keep = false
			fit.Rejected++
		}
	}
	t0, k = lineFit(blocks)
	ss, mx := 0.0, 0.0
	for _, b := range blocks {
		r := math.Abs(b.t - (t0 + k*b.az))
		ss += r * r
		mx = math.Max(mx, r)
	}
	fit.PeriodS = k * 360
	fit.RMSResidUS = math.Sqrt(ss/float64(len(blocks))) * 1e6
	fit.MaxResidUS = mx * 1e6
	switch {
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
