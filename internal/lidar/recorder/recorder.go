// Package recorder provides recording and replay of LiDAR frame data.
package recorder

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualizer"
)

// FileExtension is the extension for velocity.report log files.
const FileExtension = ".vrlog"

// ChunkSize is the number of frames per chunk file.
const ChunkSize = 1000

// LogHeader contains metadata about a recorded log.
type LogHeader struct {
	Version         string `json:"version"`
	CreatedNs       int64  `json:"created_ns"`
	SensorID        string `json:"sensor_id"`
	TotalFrames     uint64 `json:"total_frames"`
	StartNs         int64  `json:"start_ns"`
	EndNs           int64  `json:"end_ns"`
	CoordinateFrame struct {
		FrameID        string `json:"frame_id"`
		ReferenceFrame string `json:"reference_frame"`
	} `json:"coordinate_frame"`
}

// IndexEntry is an entry in the seek index.
type IndexEntry struct {
	FrameID     uint64
	TimestampNs int64
	ChunkID     uint32
	Offset      uint32
}

// Recorder writes FrameBundles to a log file.
type Recorder struct {
	basePath string
	sensorID string

	header       LogHeader
	index        []IndexEntry
	currentChunk int
	chunkFile    *os.File
	chunkOffset  uint32

	frameCount uint64
	startNs    int64
	endNs      int64

	mu     sync.Mutex
	closed bool
}

// NewRecorder creates a new Recorder that writes to the given directory.
// If path is empty, a timestamped directory is created in /tmp.
func NewRecorder(basePath, sensorID string) (*Recorder, error) {
	if basePath == "" {
		basePath = filepath.Join(os.TempDir(), fmt.Sprintf("vrlog_%s_%d", sensorID, time.Now().Unix()))
	}

	// Create directory structure
	if err := os.MkdirAll(filepath.Join(basePath, "frames"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	r := &Recorder{
		basePath:     basePath,
		sensorID:     sensorID,
		currentChunk: -1,
		index:        make([]IndexEntry, 0),
		header: LogHeader{
			Version:   "1.0",
			CreatedNs: time.Now().UnixNano(),
			SensorID:  sensorID,
		},
	}

	r.header.CoordinateFrame.FrameID = "site/" + sensorID
	r.header.CoordinateFrame.ReferenceFrame = "ENU"

	return r, nil
}

// Record writes a FrameBundle to the log.
func (r *Recorder) Record(frame *visualizer.FrameBundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return fmt.Errorf("recorder is closed")
	}

	// Track timestamps
	if r.startNs == 0 {
		r.startNs = frame.TimestampNanos
	}
	r.endNs = frame.TimestampNanos

	// Open new chunk if needed
	chunkIdx := int(r.frameCount / ChunkSize)
	if chunkIdx != r.currentChunk {
		if err := r.rotateChunk(chunkIdx); err != nil {
			return err
		}
	}

	// Serialize frame (placeholder - in production, use protobuf)
	data, err := serializeFrame(frame)
	if err != nil {
		return fmt.Errorf("failed to serialize frame: %w", err)
	}

	// Write length-prefixed frame
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := r.chunkFile.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write frame length: %w", err)
	}
	if _, err := r.chunkFile.Write(data); err != nil {
		return fmt.Errorf("failed to write frame data: %w", err)
	}

	// Add to index
	r.index = append(r.index, IndexEntry{
		FrameID:     frame.FrameID,
		TimestampNs: frame.TimestampNanos,
		ChunkID:     uint32(chunkIdx),
		Offset:      r.chunkOffset,
	})

	r.chunkOffset += uint32(4 + len(data))
	r.frameCount++

	return nil
}

// rotateChunk closes the current chunk and opens a new one.
func (r *Recorder) rotateChunk(chunkIdx int) error {
	if r.chunkFile != nil {
		if err := r.chunkFile.Close(); err != nil {
			return err
		}
	}

	chunkPath := filepath.Join(r.basePath, "frames", fmt.Sprintf("chunk_%04d.pb", chunkIdx))
	f, err := os.Create(chunkPath)
	if err != nil {
		return fmt.Errorf("failed to create chunk file: %w", err)
	}

	r.chunkFile = f
	r.currentChunk = chunkIdx
	r.chunkOffset = 0

	return nil
}

// Close finalises the log and writes the header and index.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	// Close current chunk
	if r.chunkFile != nil {
		r.chunkFile.Close()
	}

	// Write header
	r.header.TotalFrames = r.frameCount
	r.header.StartNs = r.startNs
	r.header.EndNs = r.endNs

	headerPath := filepath.Join(r.basePath, "header.json")
	headerData, err := json.MarshalIndent(r.header, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}
	if err := os.WriteFile(headerPath, headerData, 0644); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write index
	indexPath := filepath.Join(r.basePath, "index.bin")
	indexFile, err := os.Create(indexPath)
	if err != nil {
		return fmt.Errorf("failed to create index file: %w", err)
	}
	defer indexFile.Close()

	for _, entry := range r.index {
		if err := binary.Write(indexFile, binary.LittleEndian, entry.FrameID); err != nil {
			return err
		}
		if err := binary.Write(indexFile, binary.LittleEndian, entry.TimestampNs); err != nil {
			return err
		}
		if err := binary.Write(indexFile, binary.LittleEndian, entry.ChunkID); err != nil {
			return err
		}
		if err := binary.Write(indexFile, binary.LittleEndian, entry.Offset); err != nil {
			return err
		}
	}

	return nil
}

// Path returns the base path of the log.
func (r *Recorder) Path() string {
	return r.basePath
}

// FrameCount returns the number of frames recorded.
func (r *Recorder) FrameCount() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frameCount
}

// serializeFrame serializes a FrameBundle to bytes.
// TODO: Replace with proper protobuf serialization when generated.
func serializeFrame(frame *visualizer.FrameBundle) ([]byte, error) {
	// Placeholder: use JSON for now
	return json.Marshal(frame)
}

// deserializeFrame deserializes bytes to a FrameBundle.
// TODO: Replace with proper protobuf deserialization when generated.
func deserializeFrame(data []byte) (*visualizer.FrameBundle, error) {
	var frame visualizer.FrameBundle
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}

// Replayer reads FrameBundles from a log file.
type Replayer struct {
	basePath string
	header   LogHeader
	index    []IndexEntry

	// Playback state
	currentFrame uint64
	paused       bool
	rate         float32

	// Chunk cache
	currentChunk int
	chunkData    []byte
	chunkFile    *os.File

	mu sync.Mutex
}

// NewReplayer opens a log for replay.
func NewReplayer(basePath string) (*Replayer, error) {
	r := &Replayer{
		basePath:     basePath,
		currentChunk: -1,
		rate:         1.0,
	}

	// Read header
	headerPath := filepath.Join(basePath, "header.json")
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	if err := json.Unmarshal(headerData, &r.header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	// Read index
	indexPath := filepath.Join(basePath, "index.bin")
	indexFile, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open index: %w", err)
	}
	defer indexFile.Close()

	r.index = make([]IndexEntry, 0, r.header.TotalFrames)
	for {
		var entry IndexEntry
		if err := binary.Read(indexFile, binary.LittleEndian, &entry.FrameID); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if err := binary.Read(indexFile, binary.LittleEndian, &entry.TimestampNs); err != nil {
			return nil, err
		}
		if err := binary.Read(indexFile, binary.LittleEndian, &entry.ChunkID); err != nil {
			return nil, err
		}
		if err := binary.Read(indexFile, binary.LittleEndian, &entry.Offset); err != nil {
			return nil, err
		}
		r.index = append(r.index, entry)
	}

	return r, nil
}

// Header returns the log header.
func (r *Replayer) Header() LogHeader {
	return r.header
}

// TotalFrames returns the total number of frames in the log.
func (r *Replayer) TotalFrames() uint64 {
	return r.header.TotalFrames
}

// CurrentFrame returns the current frame index.
func (r *Replayer) CurrentFrame() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentFrame
}

// Seek seeks to a specific frame by index.
func (r *Replayer) Seek(frameIdx uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if frameIdx >= uint64(len(r.index)) {
		return fmt.Errorf("frame index out of range: %d >= %d", frameIdx, len(r.index))
	}

	r.currentFrame = frameIdx
	return nil
}

// SeekToTimestamp seeks to the frame closest to the given timestamp.
func (r *Replayer) SeekToTimestamp(timestampNs int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Binary search for closest frame
	// TODO: Implement binary search
	for i, entry := range r.index {
		if entry.TimestampNs >= timestampNs {
			r.currentFrame = uint64(i)
			return nil
		}
	}

	// Seek to end if timestamp is beyond log
	r.currentFrame = uint64(len(r.index) - 1)
	return nil
}

// ReadFrame reads the current frame and advances.
func (r *Replayer) ReadFrame() (*visualizer.FrameBundle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentFrame >= uint64(len(r.index)) {
		return nil, io.EOF
	}

	entry := r.index[r.currentFrame]

	// Load chunk if needed
	if int(entry.ChunkID) != r.currentChunk {
		if err := r.loadChunk(int(entry.ChunkID)); err != nil {
			return nil, err
		}
	}

	// Read frame from chunk
	offset := entry.Offset
	if offset+4 > uint32(len(r.chunkData)) {
		return nil, fmt.Errorf("invalid frame offset")
	}

	frameLen := binary.LittleEndian.Uint32(r.chunkData[offset:])
	offset += 4

	if offset+frameLen > uint32(len(r.chunkData)) {
		return nil, fmt.Errorf("invalid frame length")
	}

	frameData := r.chunkData[offset : offset+frameLen]
	frame, err := deserializeFrame(frameData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize frame: %w", err)
	}

	// Add playback info
	frame.PlaybackInfo = &visualizer.PlaybackInfo{
		IsLive:       false,
		LogStartNs:   r.header.StartNs,
		LogEndNs:     r.header.EndNs,
		PlaybackRate: r.rate,
		Paused:       r.paused,
	}

	r.currentFrame++
	return frame, nil
}

// loadChunk loads a chunk file into memory.
func (r *Replayer) loadChunk(chunkIdx int) error {
	if r.chunkFile != nil {
		r.chunkFile.Close()
	}

	chunkPath := filepath.Join(r.basePath, "frames", fmt.Sprintf("chunk_%04d.pb", chunkIdx))
	data, err := os.ReadFile(chunkPath)
	if err != nil {
		return fmt.Errorf("failed to read chunk: %w", err)
	}

	r.chunkData = data
	r.currentChunk = chunkIdx
	return nil
}

// SetPaused sets the paused state.
func (r *Replayer) SetPaused(paused bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = paused
}

// SetRate sets the playback rate.
func (r *Replayer) SetRate(rate float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rate = rate
}

// Close closes the replayer.
func (r *Replayer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.chunkFile != nil {
		return r.chunkFile.Close()
	}
	return nil
}
