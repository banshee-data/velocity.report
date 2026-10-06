package l9endpoints

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

// TestCurrentSettlingWithoutProvider covers the unwired arm. The settling
// provider is injected by the composition root after the gRPC server is built,
// so every frame between construction and wiring goes through this path.
func TestCurrentSettlingWithoutProvider(t *testing.T) {
	s := NewServer(nil)

	settling, progress := s.currentSettling()

	if settling {
		t.Error("a server with no settling provider should not report settling")
	}
	if progress != 0 {
		t.Errorf("progress = %v, want 0 with no provider", progress)
	}
}

// TestCurrentSettlingUsesProvider covers the wired arm, so the nil case above
// cannot pass by the provider never being consulted at all.
func TestCurrentSettlingUsesProvider(t *testing.T) {
	s := NewServer(nil)
	s.SetSettlingProvider(func() (bool, float32) { return true, 0.42 })

	settling, progress := s.currentSettling()

	if !settling {
		t.Error("expected the provider's settling state to be reported")
	}
	if progress != 0.42 {
		t.Errorf("progress = %v, want 0.42", progress)
	}
}

// TestDecoratePlaybackInfoStampsReplayEpoch covers the epoch backfill. A frame
// that already carries playback info still needs the epoch: it is what lets a
// client discard frames queued before a seek, and a zero would make stale
// frames look current.
func TestDecoratePlaybackInfoStampsReplayEpoch(t *testing.T) {
	s := NewServer(nil)
	s.SetReplayMode(true)

	s.playbackMu.Lock()
	s.replayEpoch = 7
	s.playbackMu.Unlock()

	frame := &FrameBundle{PlaybackInfo: &PlaybackInfo{ReplayEpoch: 0}}
	s.decoratePlaybackInfo(frame)

	if frame.PlaybackInfo.ReplayEpoch != 7 {
		t.Errorf("ReplayEpoch = %d, want the server's epoch 7", frame.PlaybackInfo.ReplayEpoch)
	}
}

// TestDecoratePlaybackInfoKeepsSetEpoch checks the backfill does not overwrite
// an epoch the producer already stamped.
func TestDecoratePlaybackInfoKeepsSetEpoch(t *testing.T) {
	s := NewServer(nil)
	s.SetReplayMode(true)

	s.playbackMu.Lock()
	s.replayEpoch = 7
	s.playbackMu.Unlock()

	frame := &FrameBundle{PlaybackInfo: &PlaybackInfo{ReplayEpoch: 3}}
	s.decoratePlaybackInfo(frame)

	if frame.PlaybackInfo.ReplayEpoch != 3 {
		t.Errorf("ReplayEpoch = %d, want the producer's 3 left alone", frame.PlaybackInfo.ReplayEpoch)
	}
}

// TestFramePointCountFallsBackToBackground covers the background arm. A
// background frame keeps its points in the snapshot rather than a PointCloud,
// so counting only the cloud reported every background frame as empty — which
// is exactly the frame an operator is watching for during settling.
func TestFramePointCountFallsBackToBackground(t *testing.T) {
	tests := []struct {
		name  string
		frame *FrameBundle
		want  int
	}{
		{"nil frame", nil, 0},
		{"empty frame", &FrameBundle{}, 0},
		{
			name:  "background snapshot",
			frame: &FrameBundle{Background: &BackgroundSnapshot{X: []float32{1, 2, 3}}},
			want:  3,
		},
		{
			name:  "empty background snapshot",
			frame: &FrameBundle{Background: &BackgroundSnapshot{}},
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := framePointCount(tc.frame); got != tc.want {
				t.Errorf("framePointCount() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestPlaybackPositionDefaultsRate covers the whole accessor, including the
// rate floor. A rate of zero would read to a client as "stopped" rather than
// "playing normally", and the field is unset until a rate is chosen.
func TestPlaybackPositionDefaultsRate(t *testing.T) {
	s := NewServer(nil)

	got := s.PlaybackPosition()

	if got.Rate != 1.0 {
		t.Errorf("Rate = %v, want the 1.0 default", got.Rate)
	}
	if got.Paused {
		t.Error("expected a fresh server to report unpaused")
	}
	if got.Seekable {
		t.Error("expected a fresh server to report no seekable log")
	}
}

// TestPlaybackPositionReportsPCAPProgress covers the populated path, so the
// defaults above cannot pass by the fields never being read.
func TestPlaybackPositionReportsPCAPProgress(t *testing.T) {
	s := NewServer(nil)
	s.SetReplayMode(true)
	s.SetPCAPProgress(120, 800)
	s.SetPCAPTimestamps(1_000, 9_000)

	got := s.PlaybackPosition()

	if got.CurrentFrame != 120 {
		t.Errorf("CurrentFrame = %d, want 120", got.CurrentFrame)
	}
	if got.TotalFrames != 800 {
		t.Errorf("TotalFrames = %d, want 800", got.TotalFrames)
	}
	if got.LogStartNs != 1_000 {
		t.Errorf("LogStartNs = %d, want 1000", got.LogStartNs)
	}
	if got.LogEndNs != 9_000 {
		t.Errorf("LogEndNs = %d, want 9000", got.LogEndNs)
	}
}

// TestStreamSyntheticSkipsWhilePaused covers the pause arm of the synthetic
// loop. A paused stream must stop producing frames without tearing the stream
// down, so the visualiser holds the last frame rather than reconnecting.
func TestStreamSyntheticSkipsWhilePaused(t *testing.T) {
	s := NewServer(nil)
	s.EnableSyntheticMode("test-paused")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sends atomic.Int64
	stream := &mockSyntheticStream{
		ctx: ctx,
		send: func(*pb.FrameBundle) error {
			sends.Add(1)
			return nil
		},
	}

	s.playbackMu.Lock()
	s.paused = true
	s.playbackMu.Unlock()

	// Let the ticker fire several times while paused, then stop the stream.
	go func() {
		time.Sleep(120 * time.Millisecond)
		cancel()
	}()

	if err := s.streamSynthetic(ctx, &pb.StreamRequest{SensorId: "test-paused"}, stream); err != context.Canceled {
		t.Fatalf("streamSynthetic returned %v, want context.Canceled", err)
	}
	if n := sends.Load(); n != 0 {
		t.Errorf("sent %d frames while paused, want 0", n)
	}
}

// TestStreamSyntheticReturnsSendError covers the send-failure arm. A client
// that has gone away surfaces as a Send error, and the loop has to return it so
// the stream is torn down rather than spinning against a dead connection.
func TestStreamSyntheticReturnsSendError(t *testing.T) {
	s := NewServer(nil)
	s.EnableSyntheticMode("test-send-error")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sendErr := errors.New("client went away")
	stream := &mockSyntheticStream{
		ctx:  ctx,
		send: func(*pb.FrameBundle) error { return sendErr },
	}

	err := s.streamSynthetic(ctx, &pb.StreamRequest{SensorId: "test-send-error"}, stream)

	if !errors.Is(err, sendErr) {
		t.Errorf("streamSynthetic returned %v, want the send error", err)
	}
}
