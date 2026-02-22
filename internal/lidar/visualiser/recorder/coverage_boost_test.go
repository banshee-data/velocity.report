package recorder

import (
	"encoding/binary"
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

// TestCloseWriteHeaderError covers the error path in Recorder.Close when the
// base directory has been removed so the header file cannot be written.
func TestCloseWriteHeaderError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))

	// Remove the base directory so Close fails to write header/index.
	os.RemoveAll(basePath)

	err = rec.Close()
	if err == nil {
		t.Error("expected error from Close when basePath is removed")
	}
}

// TestCloseIndexWriteError covers the error path in Recorder.Close when
// the index file cannot be created (index.bin is pre-created as a directory).
func TestCloseIndexWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))

	// Pre-create index.bin as a directory so os.Create fails.
	indexPath := filepath.Join(basePath, "index.bin")
	if err := os.MkdirAll(indexPath, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = rec.Close()
	if err == nil {
		t.Error("expected error from Close when index.bin is a directory")
	}
}

// TestReplayerLoadChunkTooLarge covers the error path in Replayer.loadChunk
// when the chunk file exceeds maxChunkSize.
func TestReplayerLoadChunkTooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Create a fake oversized chunk file. We can't write 200MB, but we can
	// use a sparse file by seeking past the limit. However, for simplicity
	// just write a small file and modify the stat result by replacing the
	// chunk with a symlink trick. Instead, we create a large file header.
	// Actually, the simplest approach: write the maxChunkSize + 1 byte
	// to a temp file and replace the chunk file.
	chunkPath := filepath.Join(basePath, "frames", "chunk_0000.pb")

	// Create a sparse file that reports > 200MB
	f, err := os.Create(chunkPath)
	if err != nil {
		t.Fatalf("os.Create error = %v", err)
	}
	// Seek to just past the limit and write one byte (sparse file)
	const maxChunkSize = 200 * 1024 * 1024
	f.Seek(int64(maxChunkSize), 0)
	f.Write([]byte{0})
	f.Close()

	// Reset replayer chunk cache so it reloads
	rp.currentChunk = -1

	_, err = rp.ReadFrame()
	if err == nil {
		t.Error("expected error for oversized chunk file")
	}
}

// TestReplayerCloseWithChunkFile covers Replayer.Close when a chunkFile is open.
func TestReplayerCloseWithOpenChunk(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}

	// Read a frame to trigger loadChunk (which sets rp.chunkFile... wait,
	// actually loadChunk in the current code doesn't set chunkFile — it
	// reads into chunkData. Let me set it manually.)
	// The Replayer.chunkFile field is set by loadChunk via os.Open? No — it
	// uses os.ReadFile, not os.Open. The chunkFile field is vestigial.
	// Let's open a file to simulate the field being set.
	f, err := os.Open(filepath.Join(basePath, "frames", "chunk_0000.pb"))
	if err != nil {
		t.Fatalf("os.Open error = %v", err)
	}
	rp.mu.Lock()
	rp.chunkFile = f
	rp.mu.Unlock()

	err = rp.Close()
	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// TestRecordFrameCountRotation verifies that chunk rotation happens at
// exactly ChunkSize frames with the framesInChunk counter.
func TestRecordFrameCountRotation(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ts := time.Now().UnixNano()

	// Write exactly ChunkSize frames — should all go in chunk 0.
	for i := 0; i < ChunkSize; i++ {
		if err := rec.Record(testFrameBundle(uint64(i), ts+int64(i)*1e9)); err != nil {
			t.Fatalf("Record(%d) error = %v", i, err)
		}
	}
	if rec.currentChunk != 0 {
		t.Errorf("after %d frames, currentChunk = %d, want 0", ChunkSize, rec.currentChunk)
	}
	if rec.framesInChunk != ChunkSize {
		t.Errorf("framesInChunk = %d, want %d", rec.framesInChunk, ChunkSize)
	}

	// One more frame triggers rotation to chunk 1.
	if err := rec.Record(testFrameBundle(uint64(ChunkSize), ts+int64(ChunkSize)*1e9)); err != nil {
		t.Fatalf("Record(%d) error = %v", ChunkSize, err)
	}
	if rec.currentChunk != 1 {
		t.Errorf("after %d+1 frames, currentChunk = %d, want 1", ChunkSize, rec.currentChunk)
	}
	if rec.framesInChunk != 1 {
		t.Errorf("framesInChunk = %d, want 1", rec.framesInChunk)
	}

	rec.Close()

	// Verify the index maps frames to correct chunks.
	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	if rp.TotalFrames() != uint64(ChunkSize+1) {
		t.Fatalf("TotalFrames = %d, want %d", rp.TotalFrames(), ChunkSize+1)
	}

	// Last frame should be in chunk 1.
	lastIdx := rp.index[ChunkSize]
	if lastIdx.ChunkID != 1 {
		t.Errorf("last frame ChunkID = %d, want 1", lastIdx.ChunkID)
	}
}

// TestReadFrameInvalidFrameLength covers the error path in ReadFrame when
// the length prefix points beyond chunk data.
func TestReadFrameInvalidFrameLength(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Read the first frame to force a chunk load (chunkData is populated lazily).
	_, err = rp.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	// Reset to frame 0 so the next ReadFrame re-reads the same entry.
	rp.currentFrame = 0

	// Corrupt the length prefix in chunk data to a very large value.
	offset := rp.index[0].Offset
	binary.LittleEndian.PutUint32(rp.chunkData[offset:], 0xFFFFFFFF)

	_, err = rp.ReadFrame()
	if err == nil {
		t.Error("expected error for invalid frame length")
	}
}

// TestReadFrameOffsetBeyondChunkData covers the error path in ReadFrame when
// the index entry's offset points beyond chunk data.
func TestReadFrameOffsetBeyondChunkData(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Load chunk data by reading a frame first.
	_, err = rp.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	// Reset to frame 0 and set offset beyond chunk data.
	rp.currentFrame = 0
	rp.index[0].Offset = uint32(len(rp.chunkData)) // offset+4 > len(chunkData)

	_, err = rp.ReadFrame()
	if err == nil {
		t.Error("expected error for invalid frame offset")
	}
}

// TestLoadChunkReadError covers the error path in Replayer.loadChunk
// when the chunk file cannot be read (replaced with a directory).
func TestLoadChunkReadError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Replace the chunk file with a directory to make os.ReadFile fail.
	chunkPath := filepath.Join(basePath, "frames", "chunk_0000.pb")
	os.Remove(chunkPath)
	os.MkdirAll(chunkPath, 0755)

	_, err = rp.ReadFrame()
	if err == nil {
		t.Error("expected error when chunk file is a directory")
	}
}

// TestRecordWriteDataError covers the write frame data error path
// by closing the chunk file before recording a frame.
func TestRecordWriteDataError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	// Record one frame successfully.
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))

	// Close the underlying chunk file to induce write errors.
	rec.mu.Lock()
	rec.chunkFile.Close()
	rec.mu.Unlock()

	// Next record should fail on the write path.
	err = rec.Record(testFrameBundle(1, ts+100))
	if err == nil {
		t.Error("expected error when chunk file is closed")
	}
}

// TestReadFrameDeserializeError covers the error path in ReadFrame when
// the frame data is valid-length but contains garbage protobuf bytes.
func TestReadFrameDeserializeError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Load chunk data.
	_, err = rp.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	rp.currentFrame = 0

	// Corrupt the frame data (after the 4-byte length prefix) with garbage.
	offset := rp.index[0].Offset
	frameLen := binary.LittleEndian.Uint32(rp.chunkData[offset:])
	dataStart := offset + 4
	for i := dataStart; i < dataStart+frameLen && int(i) < len(rp.chunkData); i++ {
		rp.chunkData[i] = 0xFF
	}

	_, err = rp.ReadFrame()
	if err == nil {
		t.Error("expected error for corrupted frame data")
	}
}

// TestLoadChunkCloseOldFile covers the chunkFile.Close() path in loadChunk
// when the replayer has an open chunk file and switches to another chunk.
func TestLoadChunkCloseOldFile(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))
	rec.Close()

	rp, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rp.Close()

	// Manually set chunkFile to a real file so loadChunk will close it.
	dummyPath := filepath.Join(tmpDir, "dummy-chunk")
	dummyFile, err := os.Create(dummyPath)
	if err != nil {
		t.Fatalf("create dummy: %v", err)
	}
	rp.chunkFile = dummyFile

	// ReadFrame loads chunk 0, which should close the old chunkFile.
	_, err = rp.ReadFrame()
	if err != nil {
		t.Errorf("ReadFrame() error = %v", err)
	}
}

// TestRotateChunkCloseErrorOnRecord covers the rotateChunk close-error path
// by making the chunk file already-closed before triggering frame-count rotation.
func TestRotateChunkCloseErrorOnRecord(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-log")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	rec.Record(testFrameBundle(0, ts))

	// Force frame count just below ChunkSize so next Record triggers rotation.
	rec.mu.Lock()
	rec.framesInChunk = ChunkSize
	// Close the chunk file so that rotateChunk's Close() returns an error.
	rec.chunkFile.Close()
	rec.mu.Unlock()

	err = rec.Record(testFrameBundle(1, ts+100))
	if err == nil {
		t.Error("expected error when chunk file close fails during rotation")
	}
}
