package l9endpoints

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// nilSnapshotBackgroundManager reports a healthy grid but hands back no
// snapshot. This is the shape a background manager takes when its grid has been
// reset between the sequence check and the snapshot call.
type nilSnapshotBackgroundManager struct{}

func (nilSnapshotBackgroundManager) IsSettlingComplete() bool { return true }
func (nilSnapshotBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return nil, nil
}
func (nilSnapshotBackgroundManager) GetBackgroundSequenceNumber() uint64 { return 1 }

// TestSendBackgroundSnapshotRejectsNilSnapshot covers the nil-snapshot guard.
// The manager returns interface{}, so a nil with no error is representable and
// would otherwise reach the type assertion below it as a typed nil.
func TestSendBackgroundSnapshotRejectsNilSnapshot(t *testing.T) {
	pub := NewPublisher(Config{ListenAddr: "127.0.0.1:0"})
	pub.SetBackgroundManager(nilSnapshotBackgroundManager{})

	err := pub.sendBackgroundSnapshot()

	if err == nil {
		t.Fatal("expected a nil snapshot to be reported as an error")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error %q should say the snapshot was nil", err)
	}
}

// TestSendBackgroundSnapshotWithoutManagerIsNoOp covers the unconfigured arm.
// Split background streaming is optional, so a publisher without a manager must
// stay quiet rather than error on every frame.
func TestSendBackgroundSnapshotWithoutManagerIsNoOp(t *testing.T) {
	pub := NewPublisher(Config{ListenAddr: "127.0.0.1:0"})

	if err := pub.sendBackgroundSnapshot(); err != nil {
		t.Errorf("expected a no-op without a background manager, got %v", err)
	}
}

// TestStartReportsListenFailure covers the bind-failure path. A port already in
// use is the ordinary way this fails in the field — two velocity processes, or
// a stale one that has not exited — and the error has to name it rather than
// leaving a publisher that looks started but is not.
func TestStartReportsListenFailure(t *testing.T) {
	// Hold a port so the publisher cannot bind it.
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a port: %v", err)
	}
	defer held.Close()

	pub := NewPublisher(Config{ListenAddr: held.Addr().String()})

	err = pub.Start()

	if err == nil {
		pub.Stop()
		t.Fatal("expected Start to fail on an address already in use")
	}
	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("error %q should name the listen failure", err)
	}
	if pub.running.Load() {
		t.Error("a publisher that failed to bind must not report itself running")
	}
}

// TestStartRejectsSecondCall covers the already-running guard. Start is reached
// from both the serve path and the reconciler, so a double call is reachable
// rather than hypothetical.
func TestStartRejectsSecondCall(t *testing.T) {
	pub := NewPublisher(Config{ListenAddr: "127.0.0.1:0"})
	if err := pub.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer pub.Stop()

	if err := pub.Start(); err == nil {
		t.Error("expected the second Start to be rejected")
	}
}

// erroringRecorder fails every write, standing in for a VRLOG whose disk has
// filled or whose file has been removed underneath the recorder.
type erroringRecorder struct{ calls int }

func (r *erroringRecorder) Record(*FrameBundle) error {
	r.calls++
	return errors.New("recorder is closed")
}

// TestPublishSurvivesRecorderError covers the recording error path. A failing
// recorder must not take the live stream down with it: the visualiser keeps
// receiving frames whether or not they are also being written to disk.
func TestPublishSurvivesRecorderError(t *testing.T) {
	pub := NewPublisher(Config{ListenAddr: "127.0.0.1:0"})
	rec := &erroringRecorder{}
	pub.SetRecorder(rec)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer pub.Stop()

	pub.Publish(&FrameBundle{TimestampNanos: 1})

	if rec.calls == 0 {
		t.Fatal("the recorder was never called")
	}
}

// TestBackgroundSnapshotSurvivesRecorderError covers the same failure on the
// background path, which records through a separate call site.
func TestBackgroundSnapshotSurvivesRecorderError(t *testing.T) {
	pub := NewPublisher(Config{ListenAddr: "127.0.0.1:0"})
	rec := &erroringRecorder{}
	pub.SetRecorder(rec)
	pub.SetBackgroundManager(staticBackgroundManager{})

	if err := pub.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer pub.Stop()

	// Background emission is deferred until a foreground frame has set the
	// canonical timestamp, so publish one first.
	pub.Publish(&FrameBundle{TimestampNanos: 1})
	rec.calls = 0

	if err := pub.sendBackgroundSnapshot(); err != nil {
		t.Fatalf("a recorder failure should not fail the snapshot send: %v", err)
	}
	if rec.calls == 0 {
		t.Error("the recorder was never called on the background path")
	}
}

// staticBackgroundManager returns a minimal well-formed snapshot.
type staticBackgroundManager struct{}

func (staticBackgroundManager) IsSettlingComplete() bool { return true }
func (staticBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return &BackgroundSnapshot{}, nil
}
func (staticBackgroundManager) GetBackgroundSequenceNumber() uint64 { return 1 }
