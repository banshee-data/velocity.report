// Package recorder provides recording and replay of LiDAR frame data.
package recorder

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
)

// testFrameBundle creates a FrameBundle for testing.
func testFrameBundle(frameID uint64, timestampNanos int64) *visualiser.FrameBundle {
	return &visualiser.FrameBundle{
		FrameID:        frameID,
		TimestampNanos: timestampNanos,
		SensorID:       "test-sensor",
		CoordinateFrame: visualiser.CoordinateFrameInfo{
			FrameID:        "test/sensor-01",
			ReferenceFrame: "ENU",
			OriginLat:      51.5074,
			OriginLon:      -0.1278,
		},
		PointCloud: &visualiser.PointCloudFrame{
			FrameID:        frameID,
			TimestampNanos: timestampNanos,
			SensorID:       "test-sensor",
			X:              []float32{1.0, 2.0, 3.0},
			Y:              []float32{1.1, 2.1, 3.1},
			Z:              []float32{1.2, 2.2, 3.2},
			Intensity:      []uint8{100, 150, 200},
			Classification: []uint8{0, 1, 2},
			PointCount:     3,
		},
		Clusters: &visualiser.ClusterSet{
			FrameID:        frameID,
			TimestampNanos: timestampNanos,
			Clusters: []visualiser.Cluster{
				{
					ClusterID:      1,
					SensorID:       "test-sensor",
					TimestampNanos: timestampNanos,
					CentroidX:      2.0,
					CentroidY:      2.1,
					CentroidZ:      2.2,
				},
			},
		},
	}
}

func TestNewRecorder(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	if rec.Path() != basePath {
		t.Errorf("Path() = %q, want %q", rec.Path(), basePath)
	}

	if rec.FrameCount() != 0 {
		t.Errorf("FrameCount() = %d, want 0", rec.FrameCount())
	}

	framesDir := filepath.Join(basePath, "frames")
	if _, err := os.Stat(framesDir); os.IsNotExist(err) {
		t.Error("frames directory not created")
	}
}

func TestNewRecorderInvalidPath(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping test as root user")
	}

	_, err := NewRecorder("/dev/null/invalid", "test-sensor")
	if err == nil {
		t.Error("NewRecorder() expected error for invalid path, got nil")
	}
}

func TestRecorderRecord(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	baseTime := time.Now().UnixNano()

	for i := 0; i < 5; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record() frame %d error = %v", i, err)
		}
	}

	if rec.FrameCount() != 5 {
		t.Errorf("FrameCount() = %d, want 5", rec.FrameCount())
	}
}

func TestRecorderRecordNilFrame(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	err = rec.Record(nil)
	if err == nil {
		t.Error("Record(nil) expected error, got nil")
	}
}

func TestRecorderClose(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := time.Now().UnixNano()

	for i := 0; i < 3; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	headerPath := filepath.Join(basePath, "header.json")
	if _, err := os.Stat(headerPath); os.IsNotExist(err) {
		t.Error("header.json not created after Close()")
	}

	indexPath := filepath.Join(basePath, "index.bin")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.bin not created after Close()")
	}
}

func TestRecorderChunkRotation(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	baseTime := time.Now().UnixNano()
	numFrames := ChunkSize + 10

	for i := 0; i < numFrames; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*1000000))
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record() frame %d error = %v", i, err)
		}
	}

	if rec.FrameCount() != uint64(numFrames) {
		t.Errorf("FrameCount() = %d, want %d", rec.FrameCount(), numFrames)
	}

	chunk0 := filepath.Join(basePath, "frames", "chunk_0000.pb")
	chunk1 := filepath.Join(basePath, "frames", "chunk_0001.pb")

	if _, err := os.Stat(chunk0); os.IsNotExist(err) {
		t.Error("chunk_0000.pb not created")
	}
	if _, err := os.Stat(chunk1); os.IsNotExist(err) {
		t.Error("chunk_0001.pb not created after rotation")
	}
}

func TestSerializeDeserializeFrame(t *testing.T) {
	frame := testFrameBundle(42, time.Now().UnixNano())

	data, err := serializeFrame(frame)
	if err != nil {
		t.Fatalf("serializeFrame() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("serializeFrame() returned empty data")
	}

	restored, err := deserializeFrame(data)
	if err != nil {
		t.Fatalf("deserializeFrame() error = %v", err)
	}

	if restored.FrameID != frame.FrameID {
		t.Errorf("FrameID = %d, want %d", restored.FrameID, frame.FrameID)
	}
	if restored.SensorID != frame.SensorID {
		t.Errorf("SensorID = %q, want %q", restored.SensorID, frame.SensorID)
	}
	if restored.TimestampNanos != frame.TimestampNanos {
		t.Errorf("TimestampNanos = %d, want %d", restored.TimestampNanos, frame.TimestampNanos)
	}
}

func TestDeserializeFrameInvalidData(t *testing.T) {
	_, err := deserializeFrame([]byte("invalid json"))
	if err == nil {
		t.Error("deserializeFrame() expected error for invalid data, got nil")
	}
}

func TestNewReplayer(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor-01")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	if rep.TotalFrames() != 10 {
		t.Errorf("TotalFrames() = %d, want 10", rep.TotalFrames())
	}

	if rep.CurrentFrame() != 0 {
		t.Errorf("CurrentFrame() = %d, want 0", rep.CurrentFrame())
	}
}

func TestNewReplayerInvalidPath(t *testing.T) {
	_, err := NewReplayer("/nonexistent/path")
	if err == nil {
		t.Error("NewReplayer() expected error for invalid path, got nil")
	}
}

func TestReplayerHeader(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "sensor-xyz")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	frame := testFrameBundle(1, time.Now().UnixNano())
	rec.Record(frame)
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	header := rep.Header()
	if header.SensorID != "sensor-xyz" {
		t.Errorf("Header().SensorID = %q, want %q", header.SensorID, "sensor-xyz")
	}
	if header.Version == "" {
		t.Error("Header().Version should not be empty")
	}
}

func TestReplayerSeek(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := time.Now().UnixNano()
	for i := 0; i < 20; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		rec.Record(frame)
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	if err := rep.Seek(10); err != nil {
		t.Fatalf("Seek(10) error = %v", err)
	}
	if rep.CurrentFrame() != 10 {
		t.Errorf("CurrentFrame() = %d, want 10", rep.CurrentFrame())
	}

	err = rep.Seek(100)
	if err == nil {
		t.Error("Seek(100) expected error for out of range, got nil")
	}
}

func TestReplayerSeekToTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	for i := 0; i < 10; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		rec.Record(frame)
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	targetTs := baseTime + 5*100000000
	if err := rep.SeekToTimestamp(targetTs); err != nil {
		t.Fatalf("SeekToTimestamp() error = %v", err)
	}

	if rep.CurrentFrame() != 5 {
		t.Errorf("CurrentFrame() = %d, want 5", rep.CurrentFrame())
	}

	if err := rep.SeekToTimestamp(baseTime + 1000000000000); err != nil {
		t.Fatalf("SeekToTimestamp() error = %v", err)
	}
	if rep.CurrentFrame() != 9 {
		t.Errorf("CurrentFrame() after seeking beyond = %d, want 9", rep.CurrentFrame())
	}
}

func TestReplayerReadFrame(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := time.Now().UnixNano()
	for i := 0; i < 5; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*100000000))
		rec.Record(frame)
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	for i := 0; i < 5; i++ {
		frame, err := rep.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() frame %d error = %v", i, err)
		}
		if frame.FrameID != uint64(i) {
			t.Errorf("ReadFrame() FrameID = %d, want %d", frame.FrameID, i)
		}

		if frame.PlaybackInfo == nil {
			t.Errorf("ReadFrame() frame %d PlaybackInfo is nil", i)
		} else if frame.PlaybackInfo.IsLive {
			t.Errorf("ReadFrame() PlaybackInfo.IsLive = true, want false")
		}
	}

	_, err = rep.ReadFrame()
	if err != io.EOF {
		t.Errorf("ReadFrame() at end = %v, want io.EOF", err)
	}
}

func TestReplayerSetPausedAndRate(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	rep.SetPaused(true)
	frame, _ := rep.ReadFrame()
	if frame.PlaybackInfo != nil && !frame.PlaybackInfo.Paused {
		t.Error("PlaybackInfo.Paused should be true after SetPaused(true)")
	}
}

func TestReplayerSetRate(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	rep.SetRate(2.0)
	frame, _ := rep.ReadFrame()
	if frame.PlaybackInfo != nil && frame.PlaybackInfo.PlaybackRate != 2.0 {
		t.Errorf("PlaybackInfo.PlaybackRate = %f, want 2.0", frame.PlaybackInfo.PlaybackRate)
	}
}

func TestReplayerClose(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}

	rep.ReadFrame()

	if err := rep.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if err := rep.Close(); err != nil {
		t.Errorf("double Close() error = %v", err)
	}
}

func TestRecordAndReplayRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "roundtrip-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	originalFrames := make([]*visualiser.FrameBundle, 10)
	for i := 0; i < 10; i++ {
		originalFrames[i] = testFrameBundle(uint64(i+100), baseTime+int64(i*50000000))
		if err := rec.Record(originalFrames[i]); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	for i := 0; i < 10; i++ {
		frame, err := rep.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}

		orig := originalFrames[i]
		if frame.FrameID != orig.FrameID {
			t.Errorf("frame %d: FrameID = %d, want %d", i, frame.FrameID, orig.FrameID)
		}
		if frame.TimestampNanos != orig.TimestampNanos {
			t.Errorf("frame %d: TimestampNanos = %d, want %d", i, frame.TimestampNanos, orig.TimestampNanos)
		}
		if frame.SensorID != orig.SensorID {
			t.Errorf("frame %d: SensorID = %q, want %q", i, frame.SensorID, orig.SensorID)
		}

		if frame.PointCloud == nil {
			t.Errorf("frame %d: PointCloud is nil", i)
		} else if len(frame.PointCloud.X) != len(orig.PointCloud.X) {
			t.Errorf("frame %d: PointCloud.X len = %d, want %d",
				i, len(frame.PointCloud.X), len(orig.PointCloud.X))
		}

		if frame.Clusters == nil {
			t.Errorf("frame %d: Clusters is nil", i)
		} else if len(frame.Clusters.Clusters) != len(orig.Clusters.Clusters) {
			t.Errorf("frame %d: Clusters len = %d, want %d",
				i, len(frame.Clusters.Clusters), len(orig.Clusters.Clusters))
		}
	}
}

func TestMultiChunkRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-chunk test in short mode")
	}

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "multi-chunk-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	numFrames := ChunkSize + 500
	baseTime := int64(1000000000000)

	for i := 0; i < numFrames; i++ {
		frame := testFrameBundle(uint64(i), baseTime+int64(i*1000000))
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record() frame %d error = %v", i, err)
		}
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	if rep.TotalFrames() != uint64(numFrames) {
		t.Errorf("TotalFrames() = %d, want %d", rep.TotalFrames(), numFrames)
	}

	frameCount := 0
	for {
		frame, err := rep.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ReadFrame() frame %d error = %v", frameCount, err)
		}
		if frame.FrameID != uint64(frameCount) {
			t.Errorf("frame %d: FrameID = %d, want %d", frameCount, frame.FrameID, frameCount)
		}
		frameCount++
	}

	if frameCount != numFrames {
		t.Errorf("read %d frames, want %d", frameCount, numFrames)
	}
}

func TestRecorderRecordAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Close()

	err = rec.Record(testFrameBundle(1, time.Now().UnixNano()))
	if err == nil {
		t.Error("Record() after Close() expected error, got nil")
	}
}

func TestRecorderDoubleClose(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	rec.Record(testFrameBundle(0, time.Now().UnixNano()))

	if err := rec.Close(); err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestReplayerSeekAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	for i := 0; i < 20; i++ {
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*100000000)))
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Seek to frame 15 and read
	rep.Seek(15)
	frame, err := rep.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() after Seek() error = %v", err)
	}
	if frame.FrameID != 15 {
		t.Errorf("FrameID after Seek(15) = %d, want 15", frame.FrameID)
	}

	// Read next frame should be 16
	frame, err = rep.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if frame.FrameID != 16 {
		t.Errorf("FrameID after reading = %d, want 16", frame.FrameID)
	}
}

func TestReplayerChunkCrossing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chunk crossing test in short mode")
	}

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	numFrames := ChunkSize + 100

	for i := 0; i < numFrames; i++ {
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*1000000)))
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Seek to frame just before chunk boundary
	rep.Seek(uint64(ChunkSize - 1))
	frame, _ := rep.ReadFrame()
	if frame.FrameID != uint64(ChunkSize-1) {
		t.Errorf("FrameID = %d, want %d", frame.FrameID, ChunkSize-1)
	}

	// Read frame at chunk boundary (should load new chunk)
	frame, _ = rep.ReadFrame()
	if frame.FrameID != uint64(ChunkSize) {
		t.Errorf("FrameID after chunk crossing = %d, want %d", frame.FrameID, ChunkSize)
	}
}

func TestRecorderPath(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "my-custom-log-path")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	if rec.Path() != basePath {
		t.Errorf("Path() = %q, want %q", rec.Path(), basePath)
	}
}

func TestRecorderFrameCountDuringRecording(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	baseTime := time.Now().UnixNano()

	for i := 0; i < 10; i++ {
		if rec.FrameCount() != uint64(i) {
			t.Errorf("FrameCount() before recording frame %d = %d, want %d", i, rec.FrameCount(), i)
		}
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*1000000)))
		if rec.FrameCount() != uint64(i+1) {
			t.Errorf("FrameCount() after recording frame %d = %d, want %d", i, rec.FrameCount(), i+1)
		}
	}
}

func TestReplayerSeekToTimestampEarly(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	for i := 0; i < 10; i++ {
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*100000000)))
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Seek to timestamp before log start (should go to frame 0)
	rep.SeekToTimestamp(baseTime - 1000000000)
	if rep.CurrentFrame() != 0 {
		t.Errorf("CurrentFrame() after seeking before start = %d, want 0", rep.CurrentFrame())
	}
}

func TestRecorderWithEmptyLog(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "empty-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	// Close without recording any frames
	rec.Close()

	// Verify header was created
	headerPath := filepath.Join(basePath, "header.json")
	if _, err := os.Stat(headerPath); os.IsNotExist(err) {
		t.Error("header.json not created for empty log")
	}

	// Verify index was created
	indexPath := filepath.Join(basePath, "index.bin")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.bin not created for empty log")
	}
}

func TestNewRecorderWithEmptyBasePath(t *testing.T) {
	// When basePath is empty, it generates a temp path
	rec, err := NewRecorder("", "auto-path-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer rec.Close()

	// Path should not be empty
	if rec.Path() == "" {
		t.Error("Path() should not be empty when basePath was empty")
	}

	// Should contain the sensor ID
	if !filepath.IsAbs(rec.Path()) {
		t.Error("Path() should be absolute")
	}
}

func TestReplayerReadFrameEOF(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	// Record only 2 frames
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Record(testFrameBundle(1, time.Now().UnixNano()))
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Read all frames
	rep.ReadFrame()
	rep.ReadFrame()

	// Third read should be EOF
	_, err = rep.ReadFrame()
	if err != io.EOF {
		t.Errorf("ReadFrame() after all frames = %v, want io.EOF", err)
	}

	// Fourth read should also be EOF
	_, err = rep.ReadFrame()
	if err != io.EOF {
		t.Errorf("ReadFrame() repeated after EOF = %v, want io.EOF", err)
	}
}

func TestReplayerMultipleSeeks(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1000000000000)
	for i := 0; i < 30; i++ {
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*100000000)))
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Multiple seeks
	for _, seekTo := range []uint64{5, 20, 10, 0, 29} {
		if err := rep.Seek(seekTo); err != nil {
			t.Fatalf("Seek(%d) error = %v", seekTo, err)
		}
		if rep.CurrentFrame() != seekTo {
			t.Errorf("CurrentFrame() after Seek(%d) = %d", seekTo, rep.CurrentFrame())
		}
	}
}

func TestReplayerCorruptedChunk(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	// Create a valid recording first
	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Record(testFrameBundle(1, time.Now().UnixNano()))
	rec.Close()

	// Corrupt the chunk file by truncating it
	chunkPath := filepath.Join(basePath, "frames", "chunk_0000.pb")
	if err := os.Truncate(chunkPath, 10); err != nil {
		t.Fatalf("failed to truncate chunk: %v", err)
	}

	// Try to replay
	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Reading should fail due to corrupted data
	_, err = rep.ReadFrame()
	if err == nil {
		t.Error("ReadFrame() on corrupted data expected error, got nil")
	}
}

func TestReplayerMissingChunk(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	// Create a recording with frames in multiple chunks
	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := time.Now().UnixNano()
	for i := 0; i < ChunkSize+10; i++ {
		rec.Record(testFrameBundle(uint64(i), baseTime+int64(i*1000000)))
	}
	rec.Close()

	// Delete the second chunk file
	chunk1Path := filepath.Join(basePath, "frames", "chunk_0001.pb")
	if err := os.Remove(chunk1Path); err != nil {
		t.Fatalf("failed to remove chunk: %v", err)
	}

	// Try to replay
	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Seek to a frame in the missing chunk
	rep.Seek(uint64(ChunkSize + 5))

	// Reading should fail due to missing chunk
	_, err = rep.ReadFrame()
	if err == nil {
		t.Error("ReadFrame() on missing chunk expected error, got nil")
	}
}

func TestReplayerMissingIndex(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "missing-index")

	// Create directory structure manually
	os.MkdirAll(basePath, 0755)

	// Create header but no index
	header := `{"version": "1.0", "sensor_id": "test", "total_frames": 10}`
	os.WriteFile(filepath.Join(basePath, "header.json"), []byte(header), 0644)

	// NewReplayer should fail due to missing index
	_, err := NewReplayer(basePath)
	if err == nil {
		t.Error("NewReplayer() with missing index expected error, got nil")
	}
}

func TestReplayerMissingHeader(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "missing-header")

	// Create directory but no files
	os.MkdirAll(basePath, 0755)

	// NewReplayer should fail due to missing header
	_, err := NewReplayer(basePath)
	if err == nil {
		t.Error("NewReplayer() with missing header expected error, got nil")
	}
}

func TestReplayerInvalidHeader(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "invalid-header")

	// Create directory structure
	os.MkdirAll(basePath, 0755)

	// Create invalid header JSON
	os.WriteFile(filepath.Join(basePath, "header.json"), []byte("not valid json"), 0644)

	// NewReplayer should fail due to invalid header
	_, err := NewReplayer(basePath)
	if err == nil {
		t.Error("NewReplayer() with invalid header expected error, got nil")
	}
}

func TestReplayerTruncatedIndex(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "truncated-index")

	// Create a valid recording first
	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	rec.Record(testFrameBundle(1, time.Now().UnixNano()))
	rec.Close()

	// Truncate the index file to an incomplete entry
	indexPath := filepath.Join(basePath, "index.bin")
	// Each entry is 24 bytes (uint64 + int64 + uint32 + uint32 = 8+8+4+4)
	// Truncate to leave a partial entry
	if err := os.Truncate(indexPath, 30); err != nil {
		t.Fatalf("failed to truncate index: %v", err)
	}

	// NewReplayer should fail due to truncated index
	_, err = NewReplayer(basePath)
	if err == nil {
		t.Error("NewReplayer() with truncated index expected error, got nil")
	}
}

func TestReplayerWithChunkFileOpen(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		rec.Record(testFrameBundle(uint64(i), time.Now().UnixNano()))
	}
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}

	// Read multiple frames to ensure chunk is loaded
	for i := 0; i < 3; i++ {
		_, err := rep.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() %d error = %v", i, err)
		}
	}

	// Close replayer (should close chunk file)
	if err := rep.Close(); err != nil {
		t.Errorf("Close() with open chunk error = %v", err)
	}
}
