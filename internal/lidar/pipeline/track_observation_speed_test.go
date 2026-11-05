package pipeline

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// An observation row's speed is the filter's speed at that instant, the
// magnitude of the velocity the same row carries, not the track's running
// mean (figure0002): a track that has just sped up reads its new speed.
func TestTrackObservationRecordsTheInstantSpeed(t *testing.T) {
	track := &l5tracks.TrackedObject{TrackID: "t", VX: 3, VY: 4, AvgSpeedMps: 1.5}
	obs := trackObservation(track, 1_000, "frame")
	if math.Abs(float64(obs.SpeedMps)-5) > 1e-6 {
		t.Fatalf("speed %v, want 5 from the velocity (3, 4), not the running mean 1.5", obs.SpeedMps)
	}
	if obs.VelocityX != 3 || obs.VelocityY != 4 {
		t.Fatalf("velocity (%v, %v)", obs.VelocityX, obs.VelocityY)
	}
}
