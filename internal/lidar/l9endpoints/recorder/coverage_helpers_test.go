package recorder

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

type failAfterNWrites struct {
	remaining int
	err       error
}

func (w *failAfterNWrites) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestWriteFramedPayload_WriteErrors(t *testing.T) {
	t.Run("frame length", func(t *testing.T) {
		err := writeFramedPayload(&failAfterNWrites{remaining: 0, err: errors.New("len fail")}, []byte("abc"))
		if err == nil || !strings.Contains(err.Error(), "failed to write frame length") {
			t.Fatalf("expected frame length error, got %v", err)
		}
	})

	t.Run("frame data", func(t *testing.T) {
		err := writeFramedPayload(&failAfterNWrites{remaining: 1, err: errors.New("data fail")}, []byte("abc"))
		if err == nil || !strings.Contains(err.Error(), "failed to write frame data") {
			t.Fatalf("expected frame data error, got %v", err)
		}
	})
}

type indexFieldErrorWriter struct {
	failOnWrite int
	writes      int
}

func (w *indexFieldErrorWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failOnWrite {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func TestWriteIndexEntry_FieldErrors(t *testing.T) {
	entry := IndexEntry{FrameID: 1, TimestampNs: 2, ChunkID: 3, Offset: 4}
	for i := 1; i <= 4; i++ {
		err := writeIndexEntry(&indexFieldErrorWriter{failOnWrite: i}, entry)
		if err == nil {
			t.Fatalf("expected write error on field %d", i)
		}
	}
}

func TestRecord_SerializeError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "serialize-error")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	orig := frameSerializer
	frameSerializer = func(*l9endpoints.FrameBundle) ([]byte, error) {
		return nil, errors.New("boom")
	}
	defer func() { frameSerializer = orig }()

	err = rec.Record(testFrameBundle(0, time.Now().UnixNano()))
	if err == nil || !strings.Contains(err.Error(), "failed to serialize frame") {
		t.Fatalf("expected serialize error, got %v", err)
	}
}

func TestRecorderClose_MarshalHeaderError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "marshal-error")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.SetProvenance("pcap", "test.pcap", "hash", math.NaN())

	if err := rec.Close(); err == nil || !strings.Contains(err.Error(), "failed to marshal header") {
		t.Fatalf("expected marshal header error, got %v", err)
	}
}

func TestRecorderClose_ExecutionConfigWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "execution-config-error")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	rec.SetDeterministicConfig("run-1", "param-1", "cfg", "params", "v1", "effective", "build", "sha", []byte(`{"ok":true}`))

	executionConfigPath := filepath.Join(basePath, "execution_config.json")
	if err := os.MkdirAll(executionConfigPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(execution_config.json): %v", err)
	}

	if err := rec.Close(); err == nil || !strings.Contains(err.Error(), "failed to write execution_config.json") {
		t.Fatalf("expected execution_config write error, got %v", err)
	}
}

func TestRecorderClose_IndexWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "index-write-error")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	if err := rec.Record(testFrameBundle(0, time.Now().UnixNano())); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	orig := writeRecorderIndexEntry
	writeRecorderIndexEntry = func(io.Writer, IndexEntry) error {
		return errors.New("index write failed")
	}
	defer func() { writeRecorderIndexEntry = orig }()

	if err := rec.Close(); err == nil || !strings.Contains(err.Error(), "index write failed") {
		t.Fatalf("expected index write error, got %v", err)
	}
}

func TestLooksLikeFrameBundle_Nil(t *testing.T) {
	if looksLikeFrameBundle(nil) {
		t.Fatal("nil frame should not look like a frame bundle")
	}
}

func TestDetectReplayerFrameDecoder_EmptyIndex(t *testing.T) {
	encoding, decoder, err := detectReplayerFrameDecoder(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("detectReplayerFrameDecoder() error = %v", err)
	}
	if encoding != FrameEncodingUnknown {
		t.Fatalf("encoding = %q, want %q", encoding, FrameEncodingUnknown)
	}
	if decoder == nil {
		t.Fatal("expected default decoder for empty index")
	}
}

func TestDetectReplayerFrameDecoder_InvalidPayload(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "invalid-decoder")
	frame := testFrameBundle(7, time.Now().UnixNano())
	writeSingleFrameLog(t, basePath, frame, []byte("not a frame"))

	_, _, err := detectReplayerFrameDecoder(basePath, []IndexEntry{{
		FrameID:     frame.FrameID,
		TimestampNs: frame.TimestampNanos,
		ChunkID:     0,
		Offset:      0,
	}})
	if err == nil {
		t.Fatal("expected invalid payload error")
	}
}

func TestDetectReplayerFrameDecoder_ReadPayloadError(t *testing.T) {
	_, _, err := detectReplayerFrameDecoder(t.TempDir(), []IndexEntry{{ChunkID: 0}})
	if err == nil || !strings.Contains(err.Error(), "failed to open chunk") {
		t.Fatalf("expected read payload error, got %v", err)
	}
}

func TestReadIndexedFramePayload_ErrorPaths(t *testing.T) {
	t.Run("missing chunk", func(t *testing.T) {
		_, err := readIndexedFramePayload(t.TempDir(), IndexEntry{ChunkID: 0})
		if err == nil || !strings.Contains(err.Error(), "failed to open chunk") {
			t.Fatalf("expected open chunk error, got %v", err)
		}
	})

	t.Run("oversized chunk", func(t *testing.T) {
		basePath := filepath.Join(t.TempDir(), "oversized")
		if err := os.MkdirAll(filepath.Join(basePath, "frames"), 0o755); err != nil {
			t.Fatalf("MkdirAll(): %v", err)
		}
		chunkPath := filepath.Join(basePath, "frames", "chunk_0000.pb")
		f, err := os.Create(chunkPath)
		if err != nil {
			t.Fatalf("Create(): %v", err)
		}
		if _, err := f.Seek(int64(maxChunkSize), io.SeekStart); err != nil {
			t.Fatalf("Seek(): %v", err)
		}
		if _, err := f.Write([]byte{0}); err != nil {
			t.Fatalf("Write(): %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close(): %v", err)
		}

		_, err = readIndexedFramePayload(basePath, IndexEntry{ChunkID: 0})
		if err == nil || !strings.Contains(err.Error(), "chunk file too large") {
			t.Fatalf("expected chunk too large error, got %v", err)
		}
	})

	t.Run("invalid offset", func(t *testing.T) {
		basePath := filepath.Join(t.TempDir(), "bad-offset")
		frame := testFrameBundle(1, time.Now().UnixNano())
		payload, err := serializeFrame(frame)
		if err != nil {
			t.Fatalf("serializeFrame(): %v", err)
		}
		writeSingleFrameLog(t, basePath, frame, payload)

		_, err = readIndexedFramePayload(basePath, IndexEntry{ChunkID: 0, Offset: 1 << 20})
		if err == nil || !strings.Contains(err.Error(), "invalid frame offset") {
			t.Fatalf("expected invalid frame offset error, got %v", err)
		}
	})

	t.Run("read frame length", func(t *testing.T) {
		basePath := filepath.Join(t.TempDir(), "dir-chunk")
		chunkDir := filepath.Join(basePath, "frames", "chunk_0000.pb")
		if err := os.MkdirAll(chunkDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(chunk dir): %v", err)
		}

		_, err := readIndexedFramePayload(basePath, IndexEntry{ChunkID: 0, Offset: 0})
		if err == nil || !strings.Contains(err.Error(), "failed to read frame length") {
			t.Fatalf("expected read frame length error, got %v", err)
		}
	})

	t.Run("invalid frame length", func(t *testing.T) {
		basePath := filepath.Join(t.TempDir(), "bad-len")
		if err := os.MkdirAll(filepath.Join(basePath, "frames"), 0o755); err != nil {
			t.Fatalf("MkdirAll(): %v", err)
		}
		chunkPath := filepath.Join(basePath, "frames", "chunk_0000.pb")
		if err := os.WriteFile(chunkPath, []byte{10, 0, 0, 0, 1, 2}, 0o644); err != nil {
			t.Fatalf("WriteFile(): %v", err)
		}

		_, err := readIndexedFramePayload(basePath, IndexEntry{ChunkID: 0, Offset: 0})
		if err == nil || !strings.Contains(err.Error(), "invalid frame length") {
			t.Fatalf("expected invalid frame length error, got %v", err)
		}
	})
}

func TestReadFrame_NilDecoder(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "nil-decoder")

	rec, err := NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	ts := time.Now().UnixNano()
	if err := rec.Record(testFrameBundle(0, ts)); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	rep, err := NewReplayer(basePath)
	if err != nil {
		t.Fatalf("NewReplayer() error = %v", err)
	}
	defer rep.Close()

	rep.frameDecoder = nil
	_, err = rep.ReadFrame()
	if err == nil || !strings.Contains(err.Error(), "frame decoder not initialised") {
		t.Fatalf("expected nil decoder error, got %v", err)
	}
}
