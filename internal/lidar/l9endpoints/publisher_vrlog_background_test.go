package l9endpoints

import (
	"io"
	"sync"
	"testing"
	"time"
)

// Sequence numbers chosen to be distinct so a background frame can be traced
// back to its source: the VRLOG's own recording, or the live background grid.
const (
	recordedBackgroundSeq uint64 = 7
	liveBackgroundSeq     uint64 = 3
)

// scanOnlyReader serves frames to emitFirstBackground's opening scan and then
// reports EOF, so the replay goroutine publishes nothing of its own. Tests then
// observe exactly the frames the replay-startup path emits.
type scanOnlyReader struct {
	mu       sync.Mutex
	frames   []*FrameBundle
	pos      int
	scanDone bool
}

func newScanOnlyReader(frames []*FrameBundle) *scanOnlyReader {
	return &scanOnlyReader{frames: frames}
}

func (r *scanOnlyReader) ReadFrame() (*FrameBundle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scanDone || r.pos >= len(r.frames) {
		return nil, io.EOF
	}
	frame := r.frames[r.pos]
	r.pos++
	return frame, nil
}

// Seek marks the opening scan complete: emitFirstBackground rewinds to frame 0
// once it has finished looking for a background frame.
func (r *scanOnlyReader) Seek(uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanDone = true
	r.pos = 0
	return nil
}

func (r *scanOnlyReader) SeekToTimestamp(int64) error { return nil }

func (r *scanOnlyReader) CurrentFrame() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return uint64(r.pos)
}

func (r *scanOnlyReader) TotalFrames() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return uint64(len(r.frames))
}

func (r *scanOnlyReader) SetPaused(bool)  {}
func (r *scanOnlyReader) SetRate(float32) {}
func (r *scanOnlyReader) Close() error    { return nil }

// liveBackgroundManager stands in for the live L3 background grid.
type liveBackgroundManager struct{}

func (m *liveBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return &BackgroundSnapshot{
		SequenceNumber: liveBackgroundSeq,
		TimestampNanos: time.Now().UnixNano(),
		X:              []float32{10},
		Y:              []float32{11},
		Z:              []float32{12},
	}, nil
}

func (m *liveBackgroundManager) GetBackgroundSequenceNumber() uint64 { return liveBackgroundSeq }

// newVRLogBackgroundTestPublisher returns a running publisher wired to a live
// background grid, with a foreground timestamp already recorded so background
// snapshots are not deferred for want of one.
func newVRLogBackgroundTestPublisher(t *testing.T) *Publisher {
	t.Helper()
	p := NewPublisher(Config{SensorID: "test-sensor"})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })
	p.SetBackgroundManager(&liveBackgroundManager{})
	p.lastForegroundTimestamp.Store(time.Now().UnixNano())
	return p
}

// drainVRLogBackgrounds returns the background frames queued for clients, in
// the order clients would receive them.
func drainVRLogBackgrounds(p *Publisher) []*FrameBundle {
	var out []*FrameBundle
	for {
		select {
		case f := <-p.frameChan:
			if f.FrameType == FrameTypeBackground {
				out = append(out, f)
			}
		default:
			return out
		}
	}
}

func recordedBackgroundFrame() *FrameBundle {
	return &FrameBundle{
		FrameID:       2,
		FrameType:     FrameTypeBackground,
		BackgroundSeq: recordedBackgroundSeq,
		Background: &BackgroundSnapshot{
			SequenceNumber: recordedBackgroundSeq,
			X:              []float32{1},
			Y:              []float32{2},
			Z:              []float32{3},
		},
	}
}

// TestVRLogStartup_RecordedBackgroundIsNotOverwritten covers the OnVRLogLoad
// sequence in internal/cmd/server/radar.go: StartVRLogReplay emits the VRLOG's
// own background, then SendBackgroundSnapshot is called. When the recording
// carries a background of its own, the live grid must not be pushed on top of
// it — the client caches whichever background arrives last.
func TestVRLogStartup_RecordedBackgroundIsNotOverwritten(t *testing.T) {
	pub := newVRLogBackgroundTestPublisher(t)

	reader := newScanOnlyReader([]*FrameBundle{
		{FrameID: 1, FrameType: FrameTypeForeground, TimestampNanos: 1000},
		recordedBackgroundFrame(),
	})

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	t.Cleanup(pub.StopVRLogReplay)

	if err := pub.SendBackgroundSnapshot(); err != nil {
		t.Fatalf("SendBackgroundSnapshot failed: %v", err)
	}

	if !pub.VRLogEmittedBackground() {
		t.Error("VRLogEmittedBackground = false, want true: the recording holds a background frame")
	}

	backgrounds := drainVRLogBackgrounds(pub)
	if len(backgrounds) != 1 {
		t.Fatalf("got %d background frames, want 1 (the VRLOG's own)", len(backgrounds))
	}
	if got := backgrounds[0].Background.SequenceNumber; got != recordedBackgroundSeq {
		t.Errorf("background seq = %d, want %d (recorded); the live grid overwrote the replay's background",
			got, recordedBackgroundSeq)
	}

	pub.StopVRLogReplay()
	if pub.VRLogEmittedBackground() {
		t.Error("VRLogEmittedBackground = true after StopVRLogReplay, want false")
	}
}

// TestVRLogStartup_LiveBackgroundFallsBackWhenUnrecorded covers the other half:
// a VRLOG with no background frame of its own leaves the client with nothing to
// composite against, so the live grid is still sent as a fallback.
func TestVRLogStartup_LiveBackgroundFallsBackWhenUnrecorded(t *testing.T) {
	pub := newVRLogBackgroundTestPublisher(t)

	reader := newScanOnlyReader([]*FrameBundle{
		{FrameID: 1, FrameType: FrameTypeForeground, TimestampNanos: 1000},
		{FrameID: 2, FrameType: FrameTypeForeground, TimestampNanos: 2000},
	})

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	t.Cleanup(pub.StopVRLogReplay)

	if err := pub.SendBackgroundSnapshot(); err != nil {
		t.Fatalf("SendBackgroundSnapshot failed: %v", err)
	}

	if pub.VRLogEmittedBackground() {
		t.Error("VRLogEmittedBackground = true, want false: the recording holds no background frame")
	}

	backgrounds := drainVRLogBackgrounds(pub)
	if len(backgrounds) != 1 {
		t.Fatalf("got %d background frames, want 1 (the live fallback)", len(backgrounds))
	}
	if got := backgrounds[0].Background.SequenceNumber; got != liveBackgroundSeq {
		t.Errorf("background seq = %d, want %d (live fallback)", got, liveBackgroundSeq)
	}
}
