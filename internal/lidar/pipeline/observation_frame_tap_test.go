package pipeline

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

type memoryFrameSink struct {
	records  []l4bobserve.FrameRecord
	failures []error
	err      error
}

func (s *memoryFrameSink) ObserveFrame(record l4bobserve.FrameRecord) error {
	s.records = append(s.records, record)
	return s.err
}

func (s *memoryFrameSink) RecordObservationFrameFailure(err error) {
	s.failures = append(s.failures, err)
}

// clusterCapture records the clusters the pipeline hands to publication, so
// runs with and without the tap can be compared on L4's actual output.
type clusterCapture struct{ frames [][]l4perception.WorldCluster }

func (c *clusterCapture) AdaptFrame(_ *l2frames.LiDARFrame, _ []bool, clusters []l4perception.WorldCluster, _ l5tracks.TrackerInterface, _ interface{}) interface{} {
	c.frames = append(c.frames, append([]l4perception.WorldCluster(nil), clusters...))
	return nil
}
func (c *clusterCapture) AdaptEmptyFrame(*l2frames.LiDARFrame) interface{} { return nil }

type discardPublisher struct{}

func (discardPublisher) Publish(interface{}) {}

func tapConfig(t *testing.T, sink ObservationFrameSink) *TrackingPipelineConfig {
	t.Helper()
	sensorID := "tap-" + t.Name()
	return &TrackingPipelineConfig{
		SensorID:                 sensorID,
		BackgroundManager:        makeTestBgManager(t, sensorID),
		RemoveGround:             false,
		ObservationFrameSink:     sink,
		ObservationSourceID:      "source/v1/tap-test",
		ObservationCalibrationID: "calibration/v1/tap-test",
	}
}

// ended gives a fixture frame the capture end L2 always declares.
func ended(frame *l2frames.LiDARFrame) *l2frames.LiDARFrame {
	frame.EndTimestamp = frame.StartTimestamp
	return frame
}

func driveSeedAndForeground(cb func(*l2frames.LiDARFrame), foregroundFrames int) {
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		cb(ended(makeStableFrame("seed", now.Add(time.Duration(i)*100*time.Millisecond), 20.0)))
	}
	for i := 0; i < foregroundFrames; i++ {
		cb(ended(makeForegroundFrame("fg", now.Add(time.Duration(600+i*100)*time.Millisecond), 20.0, 5.0)))
	}
}

func TestObservationFrameTapRecordsEveryFrameBeforeL5(t *testing.T) {
	sink := &memoryFrameSink{}
	cfg := tapConfig(t, sink)
	// No tracker: the callback returns right after clustering, so a record for
	// a clustered frame proves the tap does not depend on L5.
	cb := cfg.NewFrameCallback()
	cb(nil) // a nil frame has no identity and gets no record
	cb(&l2frames.LiDARFrame{FrameID: "empty"})
	driveSeedAndForeground(cb, 3)

	if len(sink.records) != 9 || len(sink.failures) != 0 {
		t.Fatalf("records = %d, failures = %v; want 9 and none", len(sink.records), sink.failures)
	}
	stream := l4bobserve.NewStreamValidator(l4bobserve.ForegroundComplete())
	for i, record := range sink.records {
		if record.Sequence != uint64(i) {
			t.Fatalf("record %d has sequence %d", i, record.Sequence)
		}
		if err := stream.AddFrame(record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	empty := sink.records[0]
	if empty.Disposition.Kind != l4bobserve.DispositionObserved || empty.Points.Len() != 0 || !empty.Payload.Has(l4bobserve.PayloadMembership) {
		t.Fatalf("empty L2 frame = %+v", empty)
	}
	foreground := sink.records[len(sink.records)-1]
	if len(foreground.Clusters) != 1 || foreground.Disposition.Kind != l4bobserve.DispositionObserved {
		t.Fatalf("foreground frame = %d clusters, %+v", len(foreground.Clusters), foreground.Disposition)
	}
	l3, _ := foreground.Stage(l4bobserve.StageL3Foreground)
	if int(l3.Output.Value) != foreground.Points.Len() || foreground.Points.Len() < len(foreground.Clusters[0].Members) {
		t.Fatalf("retained %d of %s foreground returns", foreground.Points.Len(), l3.Output)
	}
	// Foreground returns in makeForegroundFrame follow 40 background returns.
	for _, member := range foreground.Clusters[0].Members {
		if ordinal := foreground.Points.SourceOrdinal[member]; ordinal < 40 {
			t.Fatalf("cluster member has background ordinal %d", ordinal)
		}
	}
}

func TestObservationFrameTapDoesNotChangeClusters(t *testing.T) {
	run := func(sink ObservationFrameSink) [][]l4perception.WorldCluster {
		capture := &clusterCapture{}
		cfg := tapConfig(t, sink)
		cfg.RemoveGround = true
		cfg.Tracker = l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
		cfg.VisualiserAdapter, cfg.VisualiserPublisher = capture, discardPublisher{}
		driveSeedAndForeground(cfg.NewFrameCallback(), 4)
		return capture.frames
	}
	sink := &memoryFrameSink{}
	without, with := run(nil), run(sink)
	if len(without) == 0 || !reflect.DeepEqual(without, with) {
		t.Fatalf("the tap changed L4 output:\n%+v\n%+v", without, with)
	}
	if len(sink.records) != 9 {
		t.Fatalf("records = %d, want 9", len(sink.records))
	}
}

func TestObservationFrameTapRecordsSuppressedAndFailedFrames(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	t.Run("throttled", func(t *testing.T) {
		sink := &memoryFrameSink{}
		cfg := tapConfig(t, sink)
		replay := &atomic.Bool{}
		replay.Store(true)
		cfg.ReplayActive, cfg.MaxFrameRate = replay, 0.001
		driveSeedAndForeground(cfg.NewFrameCallback(), 2)
		last := sink.records[len(sink.records)-1]
		if last.Disposition != (l4bobserve.Disposition{Kind: l4bobserve.DispositionSuppressed, Stage: l4bobserve.StageL4Transform, Reason: "replay frame-rate throttle"}) || last.Payload != 0 {
			t.Fatalf("throttled frame = %+v payload %b", last.Disposition, last.Payload)
		}
	})
	t.Run("l3-only profile", func(t *testing.T) {
		sink := &memoryFrameSink{}
		cfg := tapConfig(t, sink)
		cfg.Profile = config.ProfileL3Only
		driveSeedAndForeground(cfg.NewFrameCallback(), 1)
		last := sink.records[len(sink.records)-1]
		if last.Disposition.Kind != l4bobserve.DispositionSuppressed || !strings.Contains(last.Disposition.Reason, "L3") {
			t.Fatalf("l3-only frame = %+v", last.Disposition)
		}
		if l3, ok := last.Stage(l4bobserve.StageL3Foreground); !ok || l3.Output.Value == 0 {
			t.Fatalf("suppressed frame lost its L3 count: %+v", last.Stages)
		}
	})
	t.Run("no background model", func(t *testing.T) {
		sink := &memoryFrameSink{}
		cfg := tapConfig(t, sink)
		cfg.BackgroundManager = nil
		cfg.NewFrameCallback()(ended(makeStableFrame("f", now, 20)))
		if len(sink.records) != 1 || sink.records[0].Disposition.Kind != l4bobserve.DispositionFailed || sink.records[0].Payload != 0 {
			t.Fatalf("records = %+v", sink.records)
		}
	})
}

// A live background model suppresses foreground while it settles, so those
// frames are unsettled and empty; replay mode exposes foreground from frame
// zero, so the same frames are unsettled but carry clusters.
func TestObservationFrameTapMarksUnsettledFrames(t *testing.T) {
	for _, replay := range []bool{false, true} {
		sink := &memoryFrameSink{}
		cfg := tapConfig(t, sink)
		cfg.BackgroundManager = l3grid.NewBackgroundManagerDI(cfg.SensorID, 16, 360, l3grid.BackgroundParams{
			SeedFromFirstObservation: true, BackgroundUpdateFraction: 0.5, ClosenessSensitivityMultiplier: 2.0,
			SafetyMarginMetres: 0.5, NoiseRelativeFraction: 0.01, WarmupMinFrames: 1000,
		}, nil)
		cfg.BackgroundManager.SetReplayMode(replay)
		driveSeedAndForeground(cfg.NewFrameCallback(), 2)
		last := sink.records[len(sink.records)-1]
		if last.Disposition.Kind != l4bobserve.DispositionUnsettled || last.Background != l4bobserve.BackgroundSettling {
			t.Fatalf("replay=%v: disposition %+v background %d", replay, last.Disposition, last.Background)
		}
		if got := last.Points.Len() > 0; got != replay {
			t.Fatalf("replay=%v retained %d points", replay, last.Points.Len())
		}
	}
}

func TestObservationFrameTapRefusesMissingIdentityAndSurvivesSinkErrors(t *testing.T) {
	sink := &memoryFrameSink{}
	cfg := tapConfig(t, sink)
	cfg.ObservationCalibrationID = ""
	driveSeedAndForeground(cfg.NewFrameCallback(), 1)
	if len(sink.records) != 0 || len(sink.failures) != 1 || !strings.Contains(sink.failures[0].Error(), "calibration") {
		t.Fatalf("records = %d, failures = %v", len(sink.records), sink.failures)
	}

	failing := &memoryFrameSink{err: errors.New("disk full")}
	cfg = tapConfig(t, failing)
	driveSeedAndForeground(cfg.NewFrameCallback(), 1)
	if len(failing.records) != 6 {
		t.Fatalf("a failing sink stopped the tap after %d records", len(failing.records))
	}
}

// gatedTracker holds L5 inside Update until released, recording how many
// observation records the writer had accepted when L5 was entered.
type gatedTracker struct {
	l5tracks.TrackerInterface
	writer   *vrlog.Writer
	entered  chan uint64
	release  chan struct{}
	gateOnce sync.Once
}

func (g *gatedTracker) Update(clusters []l5tracks.WorldCluster, timestamp time.Time) {
	g.gateOnce.Do(func() {
		g.entered <- g.writer.Frontier().AcceptedRecords
		<-g.release
	})
	g.TrackerInterface.Update(clusters, timestamp)
}

// The L4 capture commit is independent of L5: with the durable writer as
// the tap's sink and the tracker held inside Update, the frame L5 is working
// on is committed and announced while L5 is still blocked. The callback
// only waits for acceptance; durability happens on the writer's committer.
func TestObservationCommitNeverWaitsForL5(t *testing.T) {
	sensorID := "tap-" + t.Name()
	calibration := l4bobserve.Calibration{SensorID: sensorID, FromFrame: "sensor", ToFrame: "site/" + sensorID,
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "tap.vrlog")
	writer, err := vrlog.Create(dir, vrlog.Manifest{
		Capture: vrlog.CaptureIdentity{SensorID: sensorID, SourceType: "synthetic"},
		Extraction: vrlog.ExtractionIdentity{SourceID: "source/v1/tap-test", CalibrationID: calibrationID,
			CoordinateFrame: "site/" + sensorID, ExtractorID: "l4.test/tap"},
		Calibration: calibration,
		Commit:      vrlog.CommitPolicy{MaxBatchAge: 5 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abandon()
	cfg := tapConfig(t, writer)
	gate := &gatedTracker{TrackerInterface: l5tracks.NewTracker(l5tracks.DefaultTrackerConfig()), writer: writer,
		entered: make(chan uint64, 1), release: make(chan struct{})}
	cfg.Tracker = gate
	done := make(chan struct{})
	go func() {
		defer close(done)
		driveSeedAndForeground(cfg.NewFrameCallback(), 3)
	}()
	var accepted uint64
	select {
	case accepted = <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("L5 was never entered")
	}
	if accepted == 0 {
		t.Fatal("L5 ran before any frame was accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := writer.Frontier()
	for f.Records < accepted {
		if f, err = writer.WaitFrontier(ctx, f.Generation); err != nil {
			t.Fatalf("the frame L5 holds was not committed while L5 was blocked: %+v, %v", f, err)
		}
	}
	close(gate.release)
	<-done
	summary, err := writer.Close()
	if err != nil || summary.Frames != 8 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
}
