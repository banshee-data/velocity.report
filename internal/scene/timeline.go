package scene

import (
	"math"
	"path/filepath"
)

// DefaultBucketSeconds is the timeline summary resolution.
//
// Five seconds is fine enough that a single vehicle passing is visible and
// coarse enough that an 11-minute scene is ~130 buckets, which draws as a
// readable strip on a phone rather than a smear.
const DefaultBucketSeconds = 5.0

// modeOf groups a classifier label into the three things a street is made of.
//
// The split matters because a road busy with people reads nothing like one busy
// with cars, and a single "objects" count hides exactly that. Labels the
// classifier is unsure about land in "other" rather than being guessed into a
// mode they might not belong to.
func modeOf(class string) string {
	switch class {
	case "car", "bus", "truck":
		return "vehicle"
	case "pedestrian":
		return "person"
	case "cyclist", "motorcyclist":
		return "cycle"
	default:
		return "other"
	}
}

// timelineAccumulator builds the summary while the exporter is already walking
// every frame, so annotating the timeline costs one pass rather than two.
type timelineAccumulator struct {
	bucketSeconds float64
	buckets       map[int]*bucketTally
	maxOffsetUs   int64
}

type bucketTally struct {
	vehicle, person, cycle, other int
	frames                        int
	maxSpeed                      float64
}

func newTimelineAccumulator(bucketSeconds float64) *timelineAccumulator {
	if bucketSeconds <= 0 {
		bucketSeconds = DefaultBucketSeconds
	}
	return &timelineAccumulator{
		bucketSeconds: bucketSeconds,
		buckets:       map[int]*bucketTally{},
	}
}

// observe folds one retained frame into its bucket.
func (a *timelineAccumulator) observe(f Frame) {
	if a == nil {
		return
	}
	if f.TimeUs > a.maxOffsetUs {
		a.maxOffsetUs = f.TimeUs
	}
	idx := int(float64(f.TimeUs) / 1e6 / a.bucketSeconds)
	b := a.buckets[idx]
	if b == nil {
		b = &bucketTally{}
		a.buckets[idx] = b
	}
	b.frames++
	for _, t := range f.Tracks {
		switch modeOf(t.Class) {
		case "vehicle":
			b.vehicle++
		case "person":
			b.person++
		case "cycle":
			b.cycle++
		default:
			b.other++
		}
		if t.Speed > b.maxSpeed {
			b.maxSpeed = t.Speed
		}
	}
}

// summary converts the tallies into mean concurrent counts per bucket.
//
// Means rather than totals, because a total depends on how many frames landed
// in the bucket, which depends on stride and on a rotation rate that drifts. A
// mean answers "how busy was the street", which is the question the strip is
// there to answer.
func (a *timelineAccumulator) summary() TimelineSummary {
	out := TimelineSummary{
		Version:       FormatVersion,
		BucketSeconds: a.bucketSeconds,
		DurationSec:   float64(a.maxOffsetUs) / 1e6,
		Buckets:       []TimelineBucket{},
	}
	if len(a.buckets) == 0 {
		return out
	}

	highest := 0
	for idx := range a.buckets {
		if idx > highest {
			highest = idx
		}
	}

	round2f := func(v float64) float64 { return math.Round(v*100) / 100 }

	for idx := 0; idx <= highest; idx++ {
		b := a.buckets[idx]
		entry := TimelineBucket{StartSec: round2f(float64(idx) * a.bucketSeconds)}
		if b != nil && b.frames > 0 {
			f := float64(b.frames)
			entry.Vehicle = round2f(float64(b.vehicle) / f)
			entry.Person = round2f(float64(b.person) / f)
			entry.Cycle = round2f(float64(b.cycle) / f)
			entry.Other = round2f(float64(b.other) / f)
			entry.MaxSpeed = round2f(b.maxSpeed)
			entry.Frames = b.frames
		}
		total := entry.Vehicle + entry.Person + entry.Cycle + entry.Other
		if total > out.MaxTotal {
			out.MaxTotal = round2f(total)
		}
		if entry.MaxSpeed > out.MaxSpeed {
			out.MaxSpeed = entry.MaxSpeed
		}
		out.Buckets = append(out.Buckets, entry)
	}
	return out
}

// write emits timeline.json beside the frames it summarises.
func (a *timelineAccumulator) write(outDir string) error {
	return writeJSON(filepath.Join(outDir, "timeline.json"), a.summary())
}
