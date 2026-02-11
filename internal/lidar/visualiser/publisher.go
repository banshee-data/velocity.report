// Package visualiser provides gRPC streaming of LiDAR perception data
// to the macOS visualiser application.
//
// This package implements Track B of the visualiser project:
// - Canonical internal model (FrameBundle)
// - gRPC publisher for live streaming
// - Adapter layer for existing LidarView forwarding
package visualiser

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/grpc"
)

// Config holds configuration for the visualiser gRPC server.
type Config struct {
	// ListenAddr is the address to listen on (e.g., "localhost:50051")
	ListenAddr string

	// SensorID is the default sensor ID for streaming
	SensorID string

	// EnableDebug enables debug overlay emission
	EnableDebug bool

	// MaxClients is the maximum number of concurrent streaming clients
	MaxClients int

	// BackgroundInterval is how often to send background snapshots (default: 30s)
	BackgroundInterval time.Duration
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		ListenAddr:         "localhost:50051",
		SensorID:           "hesai-01",
		EnableDebug:        false,
		MaxClients:         5,
		BackgroundInterval: 30 * time.Second,
	}
}

// Publisher manages the gRPC server and frame streaming.
type Publisher struct {
	config   Config
	server   *grpc.Server
	listener net.Listener

	// Frame broadcasting
	frameChan chan *FrameBundle
	clients   map[string]*clientStream
	clientsMu sync.RWMutex

	// Background snapshot management (M3.5)
	backgroundMgr      BackgroundManagerInterface
	lastBackgroundSeq  uint64
	lastBackgroundSent time.Time

	// Frame recording (Phase 1.1)
	recorder   FrameRecorder
	recorderMu sync.RWMutex

	// VRLOG replay state (Phase 2.1)
	vrlogReader     FrameReader
	vrlogStopCh     chan struct{}
	vrlogMu         sync.RWMutex
	vrlogPaused     bool
	vrlogRate       float32
	vrlogSeekSignal chan struct{}
	vrlogActive     bool
	vrlogWg         sync.WaitGroup

	// Stats
	frameCount     atomic.Uint64
	clientCount    atomic.Int32
	droppedFrames  atomic.Uint64
	lastStatsTime  time.Time
	lastFrameCount uint64 // Frame count at last stats log
	lastStatsMu    sync.Mutex

	// Lifecycle
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// BackgroundManagerInterface defines the interface for background management.
// This avoids circular imports with the lidar package.
type BackgroundManagerInterface interface {
	GenerateBackgroundSnapshot() (interface{}, error) // Returns *lidar.BackgroundSnapshotData
	GetBackgroundSequenceNumber() uint64
}

// FrameRecorder is an interface for recording frames.
// This avoids circular imports with the recorder package.
type FrameRecorder interface {
	Record(frame *FrameBundle) error
}

// clientStream represents a connected streaming client.
type clientStream struct {
	id          string
	request     *pb.StreamRequest
	frameCh     chan *FrameBundle
	doneCh      chan struct{}
	preferences overlayPreferences
}

// NewPublisher creates a new Publisher with the given configuration.
func NewPublisher(cfg Config) *Publisher {
	return &Publisher{
		config:    cfg,
		frameChan: make(chan *FrameBundle, 100),
		clients:   make(map[string]*clientStream),
		stopCh:    make(chan struct{}),
	}
}

// SetBackgroundManager sets the background manager for split streaming (M3.5).
func (p *Publisher) SetBackgroundManager(mgr BackgroundManagerInterface) {
	p.backgroundMgr = mgr
}

// SetRecorder sets the frame recorder for VRLOG recording (Phase 1.1).
// The recorder will receive all frames published via Publish().
func (p *Publisher) SetRecorder(rec FrameRecorder) {
	p.recorderMu.Lock()
	defer p.recorderMu.Unlock()
	p.recorder = rec
}

// ClearRecorder removes the current frame recorder.
func (p *Publisher) ClearRecorder() {
	p.recorderMu.Lock()
	defer p.recorderMu.Unlock()
	p.recorder = nil
}

// StartVRLogReplay starts VRLOG replay from a FrameReader.
// Frames are published to all connected clients at the specified rate.
func (p *Publisher) StartVRLogReplay(reader FrameReader) error {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()

	if p.vrlogActive {
		return fmt.Errorf("VRLOG replay already active")
	}

	p.vrlogReader = reader
	p.vrlogStopCh = make(chan struct{})
	p.vrlogSeekSignal = make(chan struct{}, 1)
	p.vrlogPaused = false
	p.vrlogRate = 1.0
	p.vrlogActive = true

	p.vrlogWg.Add(1)
	go p.vrlogReplayLoop()

	log.Printf("[Visualiser] Started VRLOG replay: %d total frames", reader.TotalFrames())
	return nil
}

// StopVRLogReplay stops the current VRLOG replay.
func (p *Publisher) StopVRLogReplay() {
	p.vrlogMu.Lock()
	if !p.vrlogActive {
		p.vrlogMu.Unlock()
		return
	}
	close(p.vrlogStopCh)
	p.vrlogActive = false
	p.vrlogMu.Unlock()

	p.vrlogWg.Wait()

	p.vrlogMu.Lock()
	if p.vrlogReader != nil {
		p.vrlogReader.Close()
		p.vrlogReader = nil
	}
	p.vrlogMu.Unlock()

	log.Printf("[Visualiser] Stopped VRLOG replay")
}

// IsVRLogActive returns true if VRLOG replay is currently active.
func (p *Publisher) IsVRLogActive() bool {
	p.vrlogMu.RLock()
	defer p.vrlogMu.RUnlock()
	return p.vrlogActive
}

// VRLogReader returns the current VRLOG reader (nil if not active).
func (p *Publisher) VRLogReader() FrameReader {
	p.vrlogMu.RLock()
	defer p.vrlogMu.RUnlock()
	return p.vrlogReader
}

// SetVRLogPaused sets the paused state for VRLOG replay.
func (p *Publisher) SetVRLogPaused(paused bool) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()
	p.vrlogPaused = paused
	if p.vrlogReader != nil {
		p.vrlogReader.SetPaused(paused)
	}
}

// SetVRLogRate sets the playback rate for VRLOG replay.
func (p *Publisher) SetVRLogRate(rate float32) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()
	p.vrlogRate = rate
	if p.vrlogReader != nil {
		p.vrlogReader.SetRate(rate)
	}
}

// SeekVRLog seeks to a specific frame index in VRLOG replay.
func (p *Publisher) SeekVRLog(frameIdx uint64) error {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()

	if p.vrlogReader == nil {
		return fmt.Errorf("VRLOG replay not active")
	}

	if err := p.vrlogReader.Seek(frameIdx); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	// Signal the replay loop to reset timing
	select {
	case p.vrlogSeekSignal <- struct{}{}:
	default:
	}

	return nil
}

// SeekVRLogTimestamp seeks to a specific timestamp in VRLOG replay.
func (p *Publisher) SeekVRLogTimestamp(timestampNs int64) error {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()

	if p.vrlogReader == nil {
		return fmt.Errorf("VRLOG replay not active")
	}

	if err := p.vrlogReader.SeekToTimestamp(timestampNs); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	// Signal the replay loop to reset timing
	select {
	case p.vrlogSeekSignal <- struct{}{}:
	default:
	}

	return nil
}

// vrlogReplayLoop reads frames from the VRLOG reader and publishes them.
func (p *Publisher) vrlogReplayLoop() {
	defer p.vrlogWg.Done()

	var lastFrameTime int64
	var lastWallTime time.Time

	for {
		select {
		case <-p.vrlogStopCh:
			return
		case <-p.vrlogSeekSignal:
			// Reset timing after seek
			lastFrameTime = 0
			lastWallTime = time.Time{}
			continue
		default:
		}

		p.vrlogMu.RLock()
		isPaused := p.vrlogPaused
		rate := p.vrlogRate
		reader := p.vrlogReader
		p.vrlogMu.RUnlock()

		if isPaused || reader == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		frame, err := reader.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("[Visualiser] VRLOG replay complete")
			} else {
				log.Printf("[Visualiser] VRLOG replay error: %v", err)
			}
			// Clean up replay state on exit to prevent resource leaks
			p.StopVRLogReplay()
			return
		}

		// Rate control: sleep to match playback rate
		if lastFrameTime > 0 && rate > 0 {
			frameDelta := time.Duration(float64(frame.TimestampNanos-lastFrameTime) / float64(rate))
			wallDelta := time.Since(lastWallTime)
			if frameDelta > wallDelta {
				sleepTime := frameDelta - wallDelta
				// Cap sleep to avoid long waits
				if sleepTime > 500*time.Millisecond {
					sleepTime = 500 * time.Millisecond
				}
				time.Sleep(sleepTime)
			}
		}

		lastFrameTime = frame.TimestampNanos
		lastWallTime = time.Now()

		// Mark frame as seekable replay
		if frame.PlaybackInfo == nil {
			frame.PlaybackInfo = &PlaybackInfo{}
		}
		frame.PlaybackInfo.IsLive = false
		frame.PlaybackInfo.Seekable = true

		// Publish to all clients
		p.Publish(frame)
	}
}

// shouldSendBackground determines if a background snapshot should be sent.
func (p *Publisher) shouldSendBackground() bool {
	if p.backgroundMgr == nil {
		return false // No background manager configured
	}

	// Phase 2.1: Suppress background snapshots during VRLOG replay
	// The recorded frames already have background data embedded
	if p.IsVRLogActive() {
		return false
	}

	// Send if:
	// 1. Never sent before, OR
	// 2. Interval elapsed, OR
	// 3. Grid sequence changed (reset/sensor moved)

	currentSeq := p.backgroundMgr.GetBackgroundSequenceNumber()
	if currentSeq != p.lastBackgroundSeq && p.lastBackgroundSeq > 0 {
		lidar.Debugf("[Visualiser] Background sequence changed (%d → %d), sending refresh", p.lastBackgroundSeq, currentSeq)
		return true // Grid was reset
	}

	if p.lastBackgroundSent.IsZero() {
		log.Printf("[Visualiser] First background snapshot, sending now")
		return true // Never sent
	}

	elapsed := time.Since(p.lastBackgroundSent)
	if elapsed >= p.config.BackgroundInterval {
		lidar.Debugf("[Visualiser] Background interval elapsed (%.1fs), sending refresh", elapsed.Seconds())
		return true // Periodic refresh
	}

	return false
}

// sendBackgroundSnapshot generates and broadcasts a background snapshot.
func (p *Publisher) sendBackgroundSnapshot() error {
	if p.backgroundMgr == nil {
		return nil // No-op if not configured
	}

	snapshotDataRaw, err := p.backgroundMgr.GenerateBackgroundSnapshot()
	if err != nil {
		return fmt.Errorf("failed to generate background snapshot: %w", err)
	}

	if snapshotDataRaw == nil {
		return fmt.Errorf("background snapshot is nil")
	}

	// The interface returns interface{}, so we type assert to BackgroundSnapshot
	snapshot, ok := snapshotDataRaw.(*BackgroundSnapshot)
	if !ok {
		return fmt.Errorf("background snapshot has incorrect type: %T", snapshotDataRaw)
	}

	// Create a frame bundle with background type
	bundle := &FrameBundle{
		FrameID:        p.frameCount.Add(1),
		TimestampNanos: snapshot.TimestampNanos,
		SensorID:       p.config.SensorID,
		FrameType:      FrameTypeBackground,
		Background:     snapshot,
		BackgroundSeq:  snapshot.SequenceNumber,
	}

	// Send to all clients
	select {
	case p.frameChan <- bundle:
		p.lastBackgroundSeq = snapshot.SequenceNumber
		p.lastBackgroundSent = time.Now()
		pointCount := len(snapshot.X)
		lidar.Debugf("[Visualiser] Background snapshot sent: %d points, seq=%d", pointCount, snapshot.SequenceNumber)
	default:
		return fmt.Errorf("frame channel full, background snapshot dropped")
	}

	return nil
}

// Start starts the gRPC server.
func (p *Publisher) Start() error {
	if p.running.Load() {
		return fmt.Errorf("publisher already running")
	}

	log.Printf("[Visualiser] Attempting to bind to %s...", p.config.ListenAddr)
	lis, err := net.Listen("tcp", p.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	log.Printf("[Visualiser] Successfully bound to %s", p.config.ListenAddr)
	p.listener = lis

	// Configure max message size for large point clouds (64k+ points).
	// Default 4MB is insufficient; use 16MB to handle full-resolution frames.
	const maxMsgSize = 16 * 1024 * 1024 // 16 MB
	p.server = grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
	// Service registration is done by caller via RegisterService method

	p.running.Store(true)

	// Start broadcast goroutine
	p.wg.Add(1)
	go p.broadcastLoop()

	// Start server in background
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("[Visualiser] gRPC server listening on %s", p.config.ListenAddr)
		log.Printf("[Visualiser] Waiting for client connections...")
		if err := p.server.Serve(lis); err != nil && p.running.Load() {
			log.Printf("[Visualiser] gRPC server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server.
func (p *Publisher) Stop() {
	if !p.running.Load() {
		return
	}
	p.running.Store(false)
	close(p.stopCh)

	if p.server != nil {
		p.server.GracefulStop()
	}
	if p.listener != nil {
		p.listener.Close()
	}

	p.wg.Wait()
	log.Printf("[Visualiser] gRPC server stopped")
}

// Publish sends a frame to all connected clients.
func (p *Publisher) Publish(frame interface{}) {
	if !p.running.Load() {
		return
	}

	// Type assert to *FrameBundle
	frameBundle, ok := frame.(*FrameBundle)
	if !ok || frameBundle == nil {
		return
	}

	// M3.5: Check if we should send a background snapshot first
	if p.shouldSendBackground() {
		if err := p.sendBackgroundSnapshot(); err != nil {
			log.Printf("[Visualiser] Failed to send background snapshot: %v", err)
		}
	}

	// Determine frame type — only set if not already specified.
	// With split streaming (M3.5), foreground frames carry only perception
	// data; the client composites them over a cached background snapshot.
	if frameBundle.FrameType == 0 && frameBundle.PointCloud != nil {
		if p.backgroundMgr != nil {
			frameBundle.FrameType = FrameTypeForeground
			// Strip background points — keep only classification==1 (foreground).
			// This reduces per-frame size from ~970KB (69k pts) to ~30KB (~2k pts).
			frameBundle.PointCloud.ApplyDecimation(DecimationForegroundOnly, 0)
		} else {
			frameBundle.FrameType = FrameTypeFull
		}
	}

	// Set background sequence number for client cache coherence
	if p.backgroundMgr != nil {
		frameBundle.BackgroundSeq = p.backgroundMgr.GetBackgroundSequenceNumber()
	}

	// Phase 1.1: Record frame if recorder is set
	p.recorderMu.RLock()
	rec := p.recorder
	p.recorderMu.RUnlock()
	if rec != nil {
		if err := rec.Record(frameBundle); err != nil {
			log.Printf("[Visualiser] Recording error: %v", err)
		}
	}

	// Calculate frame size for diagnostics
	pointCount := 0
	if frameBundle.PointCloud != nil {
		pointCount = frameBundle.PointCloud.PointCount
	}
	trackCount := 0
	if frameBundle.Tracks != nil {
		trackCount = len(frameBundle.Tracks.Tracks)
	}
	clusterCount := 0
	if frameBundle.Clusters != nil {
		clusterCount = len(frameBundle.Clusters.Clusters)
	}

	// Check channel depth before sending
	queueDepth := len(p.frameChan)
	if queueDepth > 50 {
		log.Printf("[Visualiser] WARNING: Frame queue depth high: %d/100", queueDepth)
	}

	select {
	case p.frameChan <- frameBundle:
		count := p.frameCount.Add(1)
		// Log stats periodically (every 100 frames or 5 seconds)
		p.logPeriodicStats(count, pointCount, trackCount, clusterCount, queueDepth)
	default:
		// Drop frame if channel is full
		dropped := p.droppedFrames.Add(1)
		log.Printf("[Visualiser] DROPPED frame %d (total dropped: %d), channel full, points=%d tracks=%d",
			frameBundle.FrameID, dropped, pointCount, trackCount)
	}
}

// logPeriodicStats logs performance stats every 5 seconds.
func (p *Publisher) logPeriodicStats(frameCount uint64, pointCount, trackCount, clusterCount, queueDepth int) {
	p.lastStatsMu.Lock()
	defer p.lastStatsMu.Unlock()

	now := time.Now()
	if p.lastStatsTime.IsZero() {
		p.lastStatsTime = now
		p.lastFrameCount = frameCount
		return
	}

	elapsed := now.Sub(p.lastStatsTime)
	if elapsed >= 5*time.Second {
		// Calculate frames in this interval (not total frames)
		framesInInterval := frameCount - p.lastFrameCount
		fps := float64(framesInInterval) / elapsed.Seconds()
		dropped := p.droppedFrames.Load()
		clients := p.clientCount.Load()
		lidar.Debugf("[Visualiser] Stats: fps=%.1f frames=%d dropped=%d clients=%d queue=%d/100 last_frame: points=%d tracks=%d clusters=%d",
			fps, framesInInterval, dropped, clients, queueDepth, pointCount, trackCount, clusterCount)
		p.lastStatsTime = now
		p.lastFrameCount = frameCount
	}
}

// broadcastLoop distributes frames to all connected clients.
// Uses reference counting (M7) to enable safe pool reuse: each client
// that receives a frame calls Retain() before use and Release() after
// protobuf conversion. The pool reclaims slices when all clients are done.
func (p *Publisher) broadcastLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		case frame := <-p.frameChan:
			p.clientsMu.RLock()
			clientCount := len(p.clients)
			for _, client := range p.clients {
				// Retain for this client (M7 reference counting).
				// Release is called in streamFromPublisher after protobuf conversion.
				if frame.PointCloud != nil {
					frame.PointCloud.Retain()
				}
				select {
				case client.frameCh <- frame:
					// Successfully sent
				default:
					// Client is slow, drop frame for this client.
					// Release the Retain we just did since frame wasn't sent.
					if frame.PointCloud != nil {
						frame.PointCloud.Release()
					}
					// Count this so gRPC stats reflect the full picture.
					p.droppedFrames.Add(1)
				}
			}
			p.clientsMu.RUnlock()

			// If no clients are connected, release the frame immediately
			// so pooled slices aren't leaked.
			if clientCount == 0 && frame.PointCloud != nil {
				frame.PointCloud.Release()
			}
		}
	}
}

// addClient registers a new streaming client.
func (p *Publisher) addClient(id string, req *pb.StreamRequest) *clientStream {
	client := &clientStream{
		id:      id,
		request: req,
		frameCh: make(chan *FrameBundle, 10),
		doneCh:  make(chan struct{}),
	}

	p.clientsMu.Lock()
	p.clients[id] = client
	p.clientsMu.Unlock()

	p.clientCount.Add(1)
	log.Printf("[Visualiser] Client connected: %s (total: %d)", id, p.clientCount.Load())

	return client
}

// removeClient unregisters a streaming client.
func (p *Publisher) removeClient(id string) {
	p.clientsMu.Lock()
	if client, ok := p.clients[id]; ok {
		close(client.doneCh)
		delete(p.clients, id)
		p.clientsMu.Unlock()
		p.clientCount.Add(-1)
		log.Printf("[Visualiser] Client disconnected: %s (remaining: %d)", id, p.clientCount.Load())
	} else {
		p.clientsMu.Unlock()
	}
}

// Stats returns current publisher statistics.
func (p *Publisher) Stats() PublisherStats {
	return PublisherStats{
		FrameCount:  p.frameCount.Load(),
		ClientCount: p.clientCount.Load(),
		Running:     p.running.Load(),
	}
}

// PublisherStats contains publisher statistics.
type PublisherStats struct {
	FrameCount  uint64
	ClientCount int32
	Running     bool
}

// StreamRequest mirrors the proto StreamRequest for pre-generation use.
type StreamRequest struct {
	SensorID        string
	IncludePoints   bool
	IncludeClusters bool
	IncludeTracks   bool
	IncludeDebug    bool
	PointDecimation int // DecimationMode enum
	DecimationRatio float32
}

// GRPCServer returns the underlying gRPC server for service registration.
func (p *Publisher) GRPCServer() *grpc.Server {
	return p.server
}
