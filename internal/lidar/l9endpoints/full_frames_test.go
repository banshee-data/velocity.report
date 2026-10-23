package l9endpoints

import (
	"slices"
	"testing"
	"time"
)

// fullFramesMask marks points 2, 4 and 5 of keyedFrame as foreground.
var fullFramesMask = []bool{false, false, true, false, true, true}

// samePoints reports where two clouds differ in what a reader of either sees.
func samePoints(t *testing.T, what string, got, want *PointCloudFrame) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("%s: cloud %v, want %v", what, got, want)
	}
	if !slices.Equal(got.X, want.X) || !slices.Equal(got.Y, want.Y) || !slices.Equal(got.Z, want.Z) ||
		!slices.Equal(got.Intensity, want.Intensity) || !slices.Equal(got.Classification, want.Classification) {
		t.Errorf("%s: points %v/%v class %v, want %v/%v class %v", what,
			got.X, got.Y, got.Classification, want.X, want.Y, want.Classification)
	}
	if got.PointCount != want.PointCount || got.DecimationMode != want.DecimationMode ||
		got.DecimationRatio != want.DecimationRatio {
		t.Errorf("%s: %d points mode %v ratio %v, want %d mode %v ratio %v", what,
			got.PointCount, got.DecimationMode, got.DecimationRatio,
			want.PointCount, want.DecimationMode, want.DecimationRatio)
	}
	if !slices.Equal(got.PacketKeys, want.PacketKeys) || !slices.Equal(got.ReturnRefs, want.ReturnRefs) ||
		got.ReturnStart != want.ReturnStart || got.ReturnEnd != want.ReturnEnd || got.ReturnModel != want.ReturnModel {
		t.Errorf("%s: identity %v %v [%d,%d), want %v %v [%d,%d)", what,
			got.PacketKeys, got.ReturnRefs, got.ReturnStart, got.ReturnEnd,
			want.PacketKeys, want.ReturnRefs, want.ReturnStart, want.ReturnEnd)
	}
}

// Full frames suspend split streaming for as long as they are on, and only
// then: the adapter's split path comes back unchanged when they go off.
func TestAdapterEmitsFullFramesOnlyWhileOn(t *testing.T) {
	frame, _, _ := keyedFrame()
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	split := toFrameBundle(t, adapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	if split.FrameType != FrameTypeForeground || split.fullCloud || split.PointCloud.PointCount != 3 {
		t.Fatalf("split: type %v full %v points %d", split.FrameType, split.fullCloud, split.PointCloud.PointCount)
	}

	adapter.SetFullFrames(true)
	if !adapter.FullFrames() {
		t.Fatal("FullFrames() = false after SetFullFrames(true)")
	}
	full := toFrameBundle(t, adapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	if full.FrameType != FrameTypeFull || !full.fullCloud || full.recordEmpty {
		t.Fatalf("full: type %v full %v recordEmpty %v", full.FrameType, full.fullCloud, full.recordEmpty)
	}
	pc := full.PointCloud
	if pc.PointCount != 6 || pc.DecimationMode != DecimationNone {
		t.Fatalf("full: %d points mode %v, want 6 undecimated", pc.PointCount, pc.DecimationMode)
	}
	// Background is class 0, foreground class 1; nothing is ground here.
	if want := []uint8{0, 0, 1, 0, 1, 1}; !slices.Equal(pc.Classification, want) {
		t.Errorf("classes %v, want %v", pc.Classification, want)
	}
	if !pc.HasReturnIdentity() {
		t.Error("a full frame lost its return identity")
	}

	adapter.SetFullFrames(false)
	again := toFrameBundle(t, adapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	if again.FrameType != FrameTypeForeground || again.fullCloud || again.PointCloud.PointCount != 3 {
		t.Fatalf("after off: type %v full %v points %d", again.FrameType, again.fullCloud, again.PointCloud.PointCount)
	}
}

// A frame split streaming would publish empty carries its whole cloud to
// clients in full-frames mode, and is the plain empty frame otherwise.
func TestAdapterEmptyFrameCarriesTheCloudOnlyInFullFrames(t *testing.T) {
	frame, _, _ := keyedFrame()
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true
	quiet := make([]bool, len(frame.Points))

	off := toFrameBundle(t, adapter.AdaptEmptyFrameWithMask(frame, quiet))
	if off.FrameType != FrameTypeEmpty || off.PointCloud != nil || off.fullCloud {
		t.Fatalf("off: type %v cloud %v full %v", off.FrameType, off.PointCloud, off.fullCloud)
	}

	adapter.SetFullFrames(true)
	on := toFrameBundle(t, adapter.AdaptEmptyFrameWithMask(frame, quiet))
	if on.FrameType != FrameTypeFull || !on.fullCloud || !on.recordEmpty {
		t.Fatalf("on: type %v full %v recordEmpty %v", on.FrameType, on.fullCloud, on.recordEmpty)
	}
	if on.PointCloud == nil || on.PointCloud.PointCount != 6 || slices.Contains(on.PointCloud.Classification, 1) {
		t.Fatalf("on: cloud %+v, want all six points as background", on.PointCloud)
	}
	if on.FrameID != off.FrameID+1 {
		t.Errorf("frame ids %d then %d: an empty frame must still advance the id", off.FrameID, on.FrameID)
	}

	// Without a mask there is nothing to classify by: the plain empty frame.
	if b := toFrameBundle(t, adapter.AdaptEmptyFrameWithMask(frame, nil)); b.PointCloud != nil || b.FrameType != FrameTypeEmpty {
		t.Errorf("no mask: type %v cloud %v", b.FrameType, b.PointCloud)
	}
}

// fullFramesPublisher is a running publisher with split streaming's
// background manager, a recorder, and no background snapshot due.
func fullFramesPublisher(t *testing.T) (*Publisher, *capturingRecorder) {
	t.Helper()
	pub := NewPublisher(Config{SensorID: "hesai-01", BackgroundEveryFrames: 100000})
	pub.SetBackgroundManager(&mockBackgroundManager{sequenceNumber: 7})
	pub.lastBackgroundSeq = 7
	pub.lastBackgroundSent = time.Now()
	pub.running.Store(true)
	rec := &capturingRecorder{}
	pub.SetRecorder(rec)
	return pub, rec
}

// published takes the frame the publisher queued for clients.
func published(t *testing.T, pub *Publisher) *FrameBundle {
	t.Helper()
	select {
	case f := <-pub.frameChan:
		return f
	default:
		t.Fatal("nothing was published")
		return nil
	}
}

// While full frames are on, clients get the whole cloud and the recorder
// gets exactly the frame split streaming would have produced, so a VRLOG
// recorded meanwhile neither grows nor changes.
func TestPublisherSendsFullFramesButRecordsTheForegroundOnes(t *testing.T) {
	frame, _, _ := keyedFrame()
	pub, rec := fullFramesPublisher(t)
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true
	adapter.SetFullFrames(true)

	pub.Publish(adapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))

	sent := published(t, pub)
	if sent.FrameType != FrameTypeFull || sent.PointCloud.PointCount != 6 {
		t.Fatalf("clients got type %v with %d points, want the full six", sent.FrameType, sent.PointCloud.PointCount)
	}
	if sent.BackgroundSeq != 7 {
		t.Errorf("background seq %d, want 7", sent.BackgroundSeq)
	}
	if len(rec.frames) != 1 {
		t.Fatalf("recorded %d frames, want 1", len(rec.frames))
	}
	recorded := rec.frames[0]
	if recorded == sent || recorded.PointCloud == sent.PointCloud {
		t.Fatal("the recorder was handed the frame sent to clients")
	}

	// The same frame through split streaming, from an adapter at the same id.
	splitAdapter := NewFrameAdapter("hesai-01")
	splitAdapter.SplitStreaming = true
	want := toFrameBundle(t, splitAdapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	if recorded.FrameType != FrameTypeForeground || recorded.fullCloud || recorded.FrameID != want.FrameID ||
		recorded.TimestampNanos != want.TimestampNanos || recorded.BackgroundSeq != 7 {
		t.Errorf("recorded type %v full %v id %d ts %d seq %d", recorded.FrameType, recorded.fullCloud,
			recorded.FrameID, recorded.TimestampNanos, recorded.BackgroundSeq)
	}
	samePoints(t, "recorded", recorded.PointCloud, want.PointCloud)

	// The published frame is untouched by the copy.
	if want := []uint8{0, 0, 1, 0, 1, 1}; !slices.Equal(sent.PointCloud.Classification, want) {
		t.Errorf("the published frame changed: classes %v", sent.PointCloud.Classification)
	}
}

// A quiet frame goes to clients whole and to the recorder as the empty frame
// split streaming would have written.
func TestPublisherRecordsAQuietFullFrameAsEmpty(t *testing.T) {
	frame, _, _ := keyedFrame()
	pub, rec := fullFramesPublisher(t)
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true
	adapter.SetFullFrames(true)

	pub.Publish(adapter.AdaptEmptyFrameWithMask(frame, make([]bool, len(frame.Points))))

	if sent := published(t, pub); sent.PointCloud == nil || sent.PointCloud.PointCount != 6 {
		t.Fatalf("clients got %+v, want the six points", sent.PointCloud)
	}
	if len(rec.frames) != 1 || rec.frames[0].FrameType != FrameTypeEmpty || rec.frames[0].PointCloud != nil {
		t.Fatalf("recorded %+v, want one empty frame", rec.frames)
	}
}

// Off, nothing changes: the split frame is published and recorded as one,
// and an unmarked full-type frame is still stripped as before.
func TestPublisherLeavesSplitStreamingAloneWhenFullFramesAreOff(t *testing.T) {
	frame, _, _ := keyedFrame()
	pub, rec := fullFramesPublisher(t)
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	pub.Publish(adapter.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	sent := published(t, pub)
	if sent.FrameType != FrameTypeForeground || sent.PointCloud.PointCount != 3 {
		t.Fatalf("split frame published as type %v with %d points", sent.FrameType, sent.PointCloud.PointCount)
	}
	if len(rec.frames) != 1 || rec.frames[0] != sent {
		t.Fatal("a split frame should be recorded as published")
	}

	unsplit := NewFrameAdapter("hesai-01")
	pub.Publish(unsplit.AdaptFrame(frame, fullFramesMask, nil, nil, nil))
	stripped := published(t, pub)
	if stripped.FrameType != FrameTypeForeground || stripped.PointCloud.PointCount != 3 {
		t.Fatalf("an unmarked frame went out with %d points as type %v; the publisher must strip it",
			stripped.PointCloud.PointCount, stripped.FrameType)
	}
}
