package l8analytics

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// gtConst builds a ground-truth track sitting at (x,y) for every timestamp.
func gtConst(id string, ts []int64, x, y float32) GroundTruthTrack {
	pts := make([]GroundTruthPoint, len(ts))
	for i, t := range ts {
		pts[i] = GroundTruthPoint{TimestampNanos: t, X: x, Y: y}
	}
	return GroundTruthTrack{ID: id, Points: pts}
}

// hypConst builds a hypothesis (tracker output) at (x,y) for every timestamp.
func hypConst(id string, ts []int64, x, y float32) *l5tracks.TrackedObject {
	h := &l5tracks.TrackedObject{TrackID: id}
	for _, t := range ts {
		h.History = append(h.History, l5tracks.TrackPoint{X: x, Y: y, Timestamp: t})
	}
	return h
}

func frames(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i+1) * int64(1e8) // 0.1s spacing
	}
	return out
}

func TestComputeCLEARMOT(t *testing.T) {
	f5 := frames(5)
	f4 := frames(4)
	f3 := frames(3)

	tests := []struct {
		name    string
		gt      []GroundTruthTrack
		hyp     []*l5tracks.TrackedObject
		fr      []int64
		maxDist float64
		want    CLEARMOTResult
	}{
		{
			name:    "perfect tracking",
			gt:      []GroundTruthTrack{gtConst("g1", f5, 10, 5)},
			hyp:     []*l5tracks.TrackedObject{hypConst("h1", f5, 10, 5)},
			fr:      f5,
			maxDist: 1.0,
			want:    CLEARMOTResult{MOTA: 1.0, MOTP: 0.0, NumFrames: 5, NumGT: 5, FN: 0, FP: 0, IDSwitches: 0, Matches: 5},
		},
		{
			name:    "no hypotheses",
			gt:      []GroundTruthTrack{gtConst("g1", f5, 10, 5)},
			hyp:     nil,
			fr:      f5,
			maxDist: 1.0,
			want:    CLEARMOTResult{MOTA: 0.0, MOTP: 0.0, NumFrames: 5, NumGT: 5, FN: 5, FP: 0, IDSwitches: 0, Matches: 0},
		},
		{
			name: "spurious hypothesis is a false positive",
			gt:   []GroundTruthTrack{gtConst("g1", f3, 10, 5)},
			hyp: []*l5tracks.TrackedObject{
				hypConst("h1", f3, 10, 5),     // matches g1
				hypConst("ghost", f3, 50, 50), // far away → FP each frame
			},
			fr:      f3,
			maxDist: 1.0,
			// FP=3, MOTA = 1 - 3/3 = 0
			want: CLEARMOTResult{MOTA: 0.0, MOTP: 0.0, NumFrames: 3, NumGT: 3, FN: 0, FP: 3, IDSwitches: 0, Matches: 3},
		},
		{
			name: "identity switch",
			gt:   []GroundTruthTrack{gtConst("g1", f4, 10, 5)},
			hyp: []*l5tracks.TrackedObject{
				hypConst("A", f4[:2], 10, 5), // frames 0,1
				hypConst("B", f4[2:], 10, 5), // frames 2,3
			},
			fr:      f4,
			maxDist: 1.0,
			// one switch A->B; MOTA = 1 - 1/4 = 0.75
			want: CLEARMOTResult{MOTA: 0.75, MOTP: 0.0, NumFrames: 4, NumGT: 4, FN: 0, FP: 0, IDSwitches: 1, Matches: 4},
		},
		{
			name:    "constant offset drives MOTP",
			gt:      []GroundTruthTrack{gtConst("g1", f5, 10, 5)},
			hyp:     []*l5tracks.TrackedObject{hypConst("h1", f5, 10.5, 5)}, // 0.5m offset
			fr:      f5,
			maxDist: 1.0,
			want:    CLEARMOTResult{MOTA: 1.0, MOTP: 0.5, NumFrames: 5, NumGT: 5, FN: 0, FP: 0, IDSwitches: 0, Matches: 5},
		},
		{
			name:    "empty inputs",
			gt:      nil,
			hyp:     nil,
			fr:      nil,
			maxDist: 1.0,
			want:    CLEARMOTResult{},
		},
		{
			name:    "gt present hyp absent then hyp present gt absent",
			gt:      []GroundTruthTrack{gtConst("g1", f3[:2], 10, 5)},         // frames 0,1 only
			hyp:     []*l5tracks.TrackedObject{hypConst("h1", f3[2:], 10, 5)}, // frame 2 only
			fr:      f3,
			maxDist: 1.0,
			// frames 0,1: GT present, no hyp -> FN=2; frame 2: hyp present, no GT -> FP=1
			want: CLEARMOTResult{MOTA: -0.5, MOTP: 0.0, NumFrames: 3, NumGT: 2, FN: 2, FP: 1, IDSwitches: 0, Matches: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCLEARMOT(tt.gt, tt.hyp, tt.fr, tt.maxDist)
			if got.NumFrames != tt.want.NumFrames || got.NumGT != tt.want.NumGT ||
				got.FN != tt.want.FN || got.FP != tt.want.FP ||
				got.IDSwitches != tt.want.IDSwitches || got.Matches != tt.want.Matches {
				t.Errorf("counts mismatch:\n got  %+v\n want %+v", got, tt.want)
			}
			if math.Abs(got.MOTA-tt.want.MOTA) > 1e-9 {
				t.Errorf("MOTA = %v, want %v", got.MOTA, tt.want.MOTA)
			}
			if math.Abs(got.MOTP-tt.want.MOTP) > 1e-6 {
				t.Errorf("MOTP = %v, want %v", got.MOTP, tt.want.MOTP)
			}
		})
	}
}
