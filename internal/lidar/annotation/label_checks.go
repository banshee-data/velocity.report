package annotation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Label checks: mistakes a labeller would want pointed out before a reviewer
// finds them. They warn and never refuse: a save that trips one is still
// saved. The macOS window runs the same rules (LabelChecks.swift) on the
// same inputs, held to these by testdata/label_checks_fixture.json, so the
// window's Checks panel and 'velocity lidar labels check' agree.
//
// The rules read a pack's membership in the pack's own terms: samples in
// order, the points of each, and masks by sample and point index.

// Label check rules, in the order warnings are listed.
const (
	CheckSingleFrame      = "single-frame"
	CheckGap              = "gap"
	CheckImplausibleSpeed = "implausible-speed"
	CheckSharedReturn     = "shared-return"
	CheckTinyMask         = "tiny-mask"
	CheckTwin             = "twin"
)

var checkOrder = map[string]int{CheckSingleFrame: 0, CheckGap: 1, CheckImplausibleSpeed: 2, CheckSharedReturn: 3, CheckTinyMask: 4, CheckTwin: 5}

// The checks' limits. Each warning names the limit it crossed.
const (
	// CheckMaxGapFrames is the most frames an object may go without a mask
	// between two of its masks.
	CheckMaxGapFrames = 10
	// CheckMinMaskReturns is the fewest member returns a mask should have.
	CheckMinMaskReturns = 3
	// CheckSpeedFrames is how far apart, in frames, two masks may be for
	// their centroids' speed to be judged.
	CheckSpeedFrames = 3
	// CheckTwinDistanceM and CheckTwinFrames: two objects whose centroids are
	// within this distance in plan in this many shared frames look like one
	// thing labelled twice.
	CheckTwinDistanceM = 0.5
	CheckTwinFrames    = 3
)

// CheckSpeedLimitMPS is the fastest an object of a class is taken to move:
// past it, two masks' centroids are probably not the same object.
func CheckSpeedLimitMPS(class string) float64 {
	switch strings.ToLower(class) {
	case "pedestrian", "person":
		return 4
	case "cyclist", "bicycle", "bike", "scooter":
		return 15
	}
	return 45
}

// CheckSample is one sample a check reads: its id, time and points.
type CheckSample struct {
	SampleID    int       `json:"sample_id"`
	TimestampNs int64     `json:"timestamp_ns"`
	X           []float32 `json:"x"`
	Y           []float32 `json:"y"`
}

// CheckInput is a membership in a pack's terms, with the pack's samples in
// order. Samples need points only where a mask has members.
type CheckInput struct {
	Samples []CheckSample `json:"samples"`
	Objects []Object      `json:"objects"`
	Masks   []FrameMask   `json:"masks"`
}

// LabelWarning is one thing a check found.
type LabelWarning struct {
	Rule     string `json:"rule"`
	ObjectID string `json:"object_id"`
	// SampleID is the sample to look at.
	SampleID int `json:"sample_id"`
	// OtherID is the second object, for shared-return and twin.
	OtherID string `json:"other_id,omitempty"`
	Message string `json:"message"`
}

// RunLabelChecks runs every rule over a membership, warnings in rule order,
// then by object, sample and other object.
func RunLabelChecks(in CheckInput) []LabelWarning {
	position := map[int]int{}
	samples := map[int]CheckSample{}
	for i, s := range in.Samples {
		position[s.SampleID] = i
		samples[s.SampleID] = s
	}
	class := map[string]string{}
	for _, o := range in.Objects {
		class[o.ObjectID] = o.Class
	}
	// An object's masks with members, in sample order.
	type held struct {
		mask     FrameMask
		pos      int
		cx, cy   float64
		centroid bool
	}
	byObject := map[string][]held{}
	for _, m := range in.Masks {
		pos, ok := position[m.SampleID]
		if !ok || len(m.PointIndices) == 0 {
			continue
		}
		h := held{mask: m, pos: pos}
		h.cx, h.cy, h.centroid = centroid(samples[m.SampleID], m.PointIndices)
		byObject[m.ObjectID] = append(byObject[m.ObjectID], h)
	}
	var out []LabelWarning
	warn := func(rule, object string, sample int, other, format string, args ...any) {
		out = append(out, LabelWarning{Rule: rule, ObjectID: object, SampleID: sample, OtherID: other, Message: fmt.Sprintf(format, args...)})
	}
	for object, masks := range byObject {
		sort.Slice(masks, func(i, j int) bool { return masks[i].pos < masks[j].pos })
		if len(masks) == 1 {
			warn(CheckSingleFrame, object, masks[0].mask.SampleID, "", "%s has a mask in one frame only", object)
		}
		limit := CheckSpeedLimitMPS(class[object])
		for i := 1; i < len(masks); i++ {
			a, b := masks[i-1], masks[i]
			if missing := b.pos - a.pos - 1; missing > CheckMaxGapFrames {
				warn(CheckGap, object, b.mask.SampleID, "", "%s has no mask for %d frames between samples %d and %d (more than %d)",
					object, missing, a.mask.SampleID, b.mask.SampleID, CheckMaxGapFrames)
			}
			if b.pos-a.pos > CheckSpeedFrames || !a.centroid || !b.centroid {
				continue
			}
			dt := float64(samples[b.mask.SampleID].TimestampNs-samples[a.mask.SampleID].TimestampNs) / 1e9
			if dt <= 0 {
				continue
			}
			if speed := math.Hypot(b.cx-a.cx, b.cy-a.cy) / dt; speed > limit {
				warn(CheckImplausibleSpeed, object, b.mask.SampleID, "", "%s moves %.1f m/s between samples %d and %d, faster than a %s's %.0f m/s",
					object, speed, a.mask.SampleID, b.mask.SampleID, classOr(class[object]), limit)
			}
		}
	}
	// Per sample: returns two objects claim, tiny masks, and twins.
	bySample := map[int][]FrameMask{}
	for _, m := range in.Masks {
		if _, ok := position[m.SampleID]; ok {
			bySample[m.SampleID] = append(bySample[m.SampleID], m)
		}
	}
	type pair struct{ a, b string }
	near := map[pair][]int{}
	for sample, masks := range bySample {
		sort.Slice(masks, func(i, j int) bool { return masks[i].ObjectID < masks[j].ObjectID })
		owner := map[int]string{}
		shared := map[pair]int{}
		for _, m := range masks {
			// An empty mask says the object is not seen there; a tiny one
			// is usually a stray click.
			if n := len(m.PointIndices); n > 0 && n < CheckMinMaskReturns {
				warn(CheckTinyMask, m.ObjectID, sample, "", "%s's mask in sample %d has %d return(s) (fewer than %d)", m.ObjectID, sample, n, CheckMinMaskReturns)
			}
			for _, idx := range m.PointIndices {
				if first, ok := owner[idx]; ok && first != m.ObjectID {
					shared[pair{first, m.ObjectID}]++
				} else {
					owner[idx] = m.ObjectID
				}
			}
		}
		for p, n := range shared {
			warn(CheckSharedReturn, p.a, sample, p.b, "%s and %s share %d return(s) in sample %d", p.a, p.b, n, sample)
		}
		for i := range masks {
			for j := i + 1; j < len(masks); j++ {
				ax, ay, aok := centroid(samples[sample], masks[i].PointIndices)
				bx, by, bok := centroid(samples[sample], masks[j].PointIndices)
				if aok && bok && math.Hypot(ax-bx, ay-by) < CheckTwinDistanceM {
					p := pair{masks[i].ObjectID, masks[j].ObjectID}
					near[p] = append(near[p], sample)
				}
			}
		}
	}
	for p, frames := range near {
		if len(frames) < CheckTwinFrames {
			continue
		}
		sort.Slice(frames, func(i, j int) bool { return position[frames[i]] < position[frames[j]] })
		warn(CheckTwin, p.a, frames[0], p.b, "%s and %s are within %.1f m of each other in %d frames: one object labelled twice?",
			p.a, p.b, CheckTwinDistanceM, len(frames))
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if checkOrder[a.Rule] != checkOrder[b.Rule] {
			return checkOrder[a.Rule] < checkOrder[b.Rule]
		}
		if a.ObjectID != b.ObjectID {
			return a.ObjectID < b.ObjectID
		}
		if position[a.SampleID] != position[b.SampleID] {
			return position[a.SampleID] < position[b.SampleID]
		}
		return a.OtherID < b.OtherID
	})
	return out
}

// centroid is the mean plan position of a sample's points at indices, summed
// in index order; ok is false when none is in the sample.
func centroid(s CheckSample, indices []int) (x, y float64, ok bool) {
	sorted := append([]int(nil), indices...)
	sort.Ints(sorted)
	n := 0
	for _, i := range sorted {
		if i < 0 || i >= len(s.X) || i >= len(s.Y) {
			continue
		}
		x += float64(s.X[i])
		y += float64(s.Y[i])
		n++
	}
	if n == 0 {
		return 0, 0, false
	}
	return x / float64(n), y / float64(n), true
}

func classOr(c string) string {
	if c == "" {
		return "object"
	}
	return c
}

// CheckLabels runs the checks over a pack's membership as s holds it.
func CheckLabels(p *Pack, s *Sidecar) ([]LabelWarning, error) {
	in := CheckInput{Objects: s.Objects, Masks: s.Masks}
	masked := map[int]bool{}
	for _, m := range s.Masks {
		if len(m.PointIndices) > 0 {
			masked[m.SampleID] = true
		}
	}
	for _, sample := range p.Samples {
		cs := CheckSample{SampleID: sample.SampleID, TimestampNs: sample.TimestampNs}
		if masked[sample.SampleID] {
			pts, err := p.PointsAt(sample.SampleID)
			if err != nil {
				return nil, fmt.Errorf("sample %d: %w", sample.SampleID, err)
			}
			cs.X, cs.Y = pts.X, pts.Y
		}
		in.Samples = append(in.Samples, cs)
	}
	return RunLabelChecks(in), nil
}
