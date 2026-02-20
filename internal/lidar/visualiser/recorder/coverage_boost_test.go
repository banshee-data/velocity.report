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

	// Force chunk rotation: set frameCount so next Record triggers chunkIdx=1
	// while currentChunk is still 0
	rec.mu.Lock()
	rec.frameCount = ChunkSize
	rec.mu.Unlock()

	err = rec.Record(testFrameBundle(1, ts+1e9))
	if err == nil {
		t.Error("expected error when frames dir is non-writable")
	}
}
