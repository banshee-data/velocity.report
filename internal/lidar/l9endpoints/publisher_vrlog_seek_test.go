package l9endpoints

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gatedFrameReader holds the replay loop inside ReadFrame, once armed, until
// released, so a test can ask for a seek while a read is in progress.
type gatedFrameReader struct {
	*mockFrameReader
	armed   atomic.Bool
	entered chan struct{}
	once    sync.Once
	release chan struct{}
}

func (g *gatedFrameReader) ReadFrame() (*FrameBundle, error) {
	if g.armed.Load() {
		g.once.Do(func() { close(g.entered) })
		<-g.release
	}
	return g.mockFrameReader.ReadFrame()
}

// A seek asked for while the replay loop is reading a frame waits for the
// read, then lands where it was asked: the frame read before it is never
// published, and the next frame published is the one sought. Before, the
// loop read without the seek's lock, so it could advance the reader between
// a seek and the frame index it reported (TestPublisher_VRLogReplay_Seek
// failed in CI with "expected frame 3, got 4"), and could publish the frame
// it had read after the seek drained the queue.
func TestPublisher_VRLogSeekWaitsForAReadInProgress(t *testing.T) {
	pub := newVRLogBackgroundTestPublisher(t)
	frames := make([]*FrameBundle, 10)
	base := time.Now().UnixNano()
	for i := range frames {
		frames[i] = &FrameBundle{FrameID: uint64(i + 1), FrameType: FrameTypeForeground,
			TimestampNanos: base + int64(i)*int64(10*time.Millisecond), SensorID: "test-sensor"}
	}
	g := &gatedFrameReader{mockFrameReader: newMockFrameReader(frames),
		entered: make(chan struct{}), release: make(chan struct{})}
	if err := pub.StartVRLogReplay(g); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pub.StopVRLogReplay)
	var released sync.Once
	release := func() { released.Do(func() { close(g.release) }) }
	t.Cleanup(release)

	g.armed.Store(true)
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the replay loop never read a frame")
	}
	type landed struct {
		frame uint64
		err   error
	}
	done := make(chan landed, 1)
	go func() {
		f, err := pub.SeekVRLog(7)
		done <- landed{f, err}
	}()
	select {
	case got := <-done:
		t.Fatalf("the seek returned (frame %d, %v) while the loop was still reading", got.frame, got.err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	got := <-done
	if got.err != nil || got.frame != 7 {
		t.Fatalf("seek landed on %d (%v), want 7", got.frame, got.err)
	}
	// The first foreground frame published after the seek is frame index 7.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case f := <-pub.frameChan:
			if f.FrameType != FrameTypeForeground {
				continue
			}
			if f.FrameID != 8 {
				t.Fatalf("after seeking to index 7 the first frame published was %d, want 8", f.FrameID)
			}
			return
		case <-deadline:
			t.Fatal("no frame was published after the seek")
		}
	}
}
