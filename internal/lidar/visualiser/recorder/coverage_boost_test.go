package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewReplayerTruncatedIndexTimestampNs covers the error path when the
// second index entry's TimestampNs field is truncated.
func TestNewReplayerTruncatedIndexTimestampNs(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Record(testFrameBundle(1, ts+1e9))
	rec.Close()

	// 1 full entry (24 bytes) + FrameID of 2nd entry (8 bytes) = 32 bytes
	if err := os.Truncate(filepath.Join(basePath, "index.bin"), 32); err != nil {
		t.Fatalf("Truncate error = %v", err)
	}

	_, err = NewReplayer(basePath)
	if err == nil {
		t.Error("expected error for truncated TimestampNs")
	}
}

// TestNewReplayerTruncatedIndexChunkID covers the error path when the second
// index entry's ChunkID field is truncated.
func TestNewReplayerTruncatedIndexChunkID(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Record(testFrameBundle(1, ts+1e9))
	rec.Close()

	// 1 full entry (24) + FrameID (8) + TimestampNs (8) = 40 bytes
	if err := os.Truncate(filepath.Join(basePath, "index.bin"), 40); err != nil {
		t.Fatalf("Truncate error = %v", err)
	}

	_, err = NewReplayer(basePath)
	if err == nil {
		t.Error("expected error for truncated ChunkID")
	}
}

// TestNewReplayerTruncatedIndexOffset covers the error path when the second
// index entry's Offset field is truncated.
func TestNewReplayerTruncatedIndexOffset(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Record(testFrameBundle(1, ts+1e9))
	rec.Close()

	// 1 full entry (24) + FrameID (8) + TimestampNs (8) + ChunkID (4) = 44 bytes
	if err := os.Truncate(filepath.Join(basePath, "index.bin"), 44); err != nil {
		t.Fatalf("Truncate error = %v", err)
	}

	_, err = NewReplayer(basePath)
	if err == nil {
		t.Error("expected error for truncated Offset")
	}
}

// TestRecordWriteFailure covers the write-error path when the underlying chunk
// file is closed externally before Record writes to it.
func TestRecordWriteFailure(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ts := time.Now().UnixNano()
	if err := rec.Record(testFrameBundle(0, ts)); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	// Close the chunk file to cause subsequent writes to fail
	rec.mu.Lock()
	rec.chunkFile.Close()
	rec.mu.Unlock()

	err = rec.Record(testFrameBundle(1, ts+1e9))
	if err == nil {
		t.Error("expected write error after chunk file closed")
	}
}

// TestReadFrameInvalidOffset covers the error path in ReadFrame when the
// index points beyond chunk data.
func TestReadFrameInvalidOffset(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	// Corrupt index to point beyond chunk data
	rep.index[0].Offset = 999999

	_, err = rep.ReadFrame()
	if err == nil {
		t.Error("expected error for invalid offset")
	}
}

// TestRotateChunkCreateError covers the error path when os.Create fails
// during chunk rotation (due to a non-writable directory).
func TestRotateChunkCreateError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ts := time.Now().UnixNano()
	if err := rec.Record(testFrameBundle(0, ts)); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	// Make the frames directory non-writable to trigger os.Create error
	framesDir := filepath.Join(basePath, "frames")
	os.Chmod(framesDir, 0444)
	defer os.Chmod(framesDir, 0755)

	// Force chunk rotation: set framesInChunk so next Record triggers rotation
	// while currentChunk is still 0
	rec.mu.Lock()
	rec.framesInChunk = ChunkSize
	rec.mu.Unlock()

	err = rec.Record(testFrameBundle(1, ts+1e9))
	if err == nil {
		t.Error("expected error when frames dir is non-writable")
	}
}

// TestSizeBasedChunkRotation verifies that the recorder rotates to a new
// chunk when the current chunk's byte size exceeds maxChunkWriteBytes,
// even if the frame-count boundary has not been reached.
func TestSizeBasedChunkRotation(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ts := time.Now().UnixNano()

	// Write one frame, then fake a very large chunkOffset to trigger
	// size-based rotation on the next Record call.
	if err := rec.Record(testFrameBundle(0, ts)); err != nil {
		t.Fatalf("Record(0) error = %v", err)
	}
	if rec.currentChunk != 0 {
		t.Fatalf("expected chunk 0, got %d", rec.currentChunk)
	}

	rec.mu.Lock()
	rec.chunkOffset = maxChunkWriteBytes // simulate a full chunk
	rec.mu.Unlock()

	// Next frame should trigger rotation to chunk 1.
	if err := rec.Record(testFrameBundle(1, ts+1e9)); err != nil {
		t.Fatalf("Record(1) error = %v", err)
	}
	if rec.currentChunk != 1 {
		t.Fatalf("expected chunk 1 after size rotation, got %d", rec.currentChunk)
	}

	rec.Close()

	// Verify replay works across the two chunks.
	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	if rp.TotalFrames() != 2 {
		t.Fatalf("expected 2 frames, got %d", rp.TotalFrames())
	}

	// Confirm frames are in different chunks in the index.
	if rp.index[0].ChunkID == rp.index[1].ChunkID {
		t.Error("expected frames in different chunks after size-based rotation")
	}

	f0, err := rp.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(0) error = %v", err)
	}
	if f0.FrameID != 0 {
		t.Errorf("frame 0 ID = %d, want 0", f0.FrameID)
	}

	f1, err := rp.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(1) error = %v", err)
	}
	if f1.FrameID != 1 {
		t.Errorf("frame 1 ID = %d, want 1", f1.FrameID)
	}
}
