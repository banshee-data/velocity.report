package scene

import (
	"math"
	"path/filepath"
	"sort"
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
	carTrackMax   map[string]float64
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
		carTrackMax:   map[string]float64{},
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
		// A classification may settle after a track has appeared. MaxSpeed is
		// the running track maximum, so the first frame labelled car still
		// carries the whole track's history up to that point.
		maxSpeed := math.Max(t.Speed, t.MaxSpeed)
		if t.Class == "car" {
			if maxSpeed > a.carTrackMax[t.ID] {
				a.carTrackMax[t.ID] = maxSpeed
			}
		} else if previous, ok := a.carTrackMax[t.ID]; ok && maxSpeed > previous {
			a.carTrackMax[t.ID] = maxSpeed
		}
	}
}

const (
	mphPerMPS              = 2.2369362920544
	vehicleSpeedBucketMPH  = 5
	vehicleMinimumSpeedMPH = 5
)

// vehicleSpeedSummary follows the radar report's empirical nearest-rank
// convention: ceil(p*n)-1 in the sorted population.
func (a *timelineAccumulator) vehicleSpeedSummary() *VehicleSpeedSummary {
	if len(a.carTrackMax) == 0 {
		return nil
	}
	speeds := make([]float64, 0, len(a.carTrackMax))
	for _, speedMPS := range a.carTrackMax {
		if speedMPH := speedMPS * mphPerMPS; speedMPH > vehicleMinimumSpeedMPH {
			speeds = append(speeds, speedMPH)
		}
	}
	if len(speeds) == 0 {
		return nil
	}
	sort.Float64s(speeds)
	percentile := func(p float64) *float64 {
		idx := int(math.Ceil(float64(len(speeds))*p)) - 1
		if idx < 0 {
			idx = 0
		}
		v := math.Round(speeds[idx]*100) / 100
		return &v
	}

	maxMPH := speeds[len(speeds)-1]
	lastBucket := int(math.Floor(maxMPH/vehicleSpeedBucketMPH)) * vehicleSpeedBucketMPH
	counts := make(map[int]int, lastBucket/vehicleSpeedBucketMPH+1)
	for _, speed := range speeds {
		start := int(math.Floor(speed/vehicleSpeedBucketMPH)) * vehicleSpeedBucketMPH
		counts[start]++
	}
	histogram := make([]VehicleSpeedBucket, 0, len(counts))
	for start := vehicleMinimumSpeedMPH; start <= lastBucket; start += vehicleSpeedBucketMPH {
		histogram = append(histogram, VehicleSpeedBucket{StartMPH: start, Count: counts[start]})
	}

	out := &VehicleSpeedSummary{
		Units:        "mph",
		BucketSize:   vehicleSpeedBucketMPH,
		MinimumSpeed: vehicleMinimumSpeedMPH,
		TrackCount:   len(speeds),
		Max:          math.Round(maxMPH*100) / 100,
		Histogram:    histogram,
	}
	out.P50 = percentile(0.50)
	out.P85 = percentile(0.85)
	out.P98 = percentile(0.98)
	return out
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
		VehicleSpeed:  a.vehicleSpeedSummary(),
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
