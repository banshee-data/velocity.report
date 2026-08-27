package l9endpoints

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
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

	// Background snapshot management (M3.5).
	//
	// backgroundMgr is wired once by SetBackgroundManager during startup,
	// before Start() launches any goroutine, and is read-only thereafter.
	//
	// backgroundMu guards lastBackgroundSeq and lastBackgroundSent, which are
	// touched both by the pipeline goroutine (Publish → shouldSendBackground)
	// and by HTTP handlers (SendBackgroundSnapshot). It is held only across
	// those field accesses, never across GenerateBackgroundSnapshot: that walks
	// the whole grid, and blocking the publish hot path behind it would stall
	// frame delivery.
	backgroundMgr      BackgroundManagerInterface
	backgroundMu       sync.Mutex
	lastBackgroundSeq  uint64
	lastBackgroundSent time.Time
	// lastBackgroundFrame is the most recent background as broadcast, kept so a
	// client that subscribes afterwards can be given the scene rather than
	// rendering over nothing until the next refresh.
	lastBackgroundFrame     *FrameBundle
	lastForegroundTimestamp atomic.Int64 // most recent foreground frame's TimestampNanos

	// Frame recording
	recorder   FrameRecorder
	recorderMu sync.RWMutex

	// VRLOG replay state
	vrlogReader       FrameReader
	vrlogStopCh       chan struct{}
	vrlogMu           sync.RWMutex
	vrlogPaused       bool
	vrlogRate         float32
	vrlogSeekSignal   chan struct{}
	vrlogSendOneFrame bool // Send one frame after seek-while-paused
	vrlogActive       bool
	vrlogWg           sync.WaitGroup
	// vrlogEmittedBackground records whether the active replay published a
	// background frame of its own at startup.
	vrlogEmittedBackground bool
	// onReplayEnded is invoked once when a VRLOG replay reaches the end of its
	// recording, so the owner can return the pipeline to live input. Guarded by
	// vrlogMu; always invoked on its own goroutine (see notifyReplayEnded).
	onReplayEnded func()

	// Stats
	frameCount  atomic.Uint64
	clientCount atomic.Int32
	// droppedFrames counts frames lost at the publish stage: frameChan was full
	// so the frame never entered the pipeline at all.
	droppedFrames atomic.Uint64
	// clientDroppedFrames counts frames lost at the broadcast stage: the frame
	// was published successfully, then rejected because a client's own queue was
	// full. Kept separate from droppedFrames because the two measure different
	// stages and a frame can be counted at both — folding them into one ratio
	// made a client accepting nothing read as exactly 50%, never higher.
	clientDroppedFrames    atomic.Uint64
	lastStatsTime          time.Time
	lastFrameCount         uint64 // Frame count at last stats log
	lastDroppedCount       uint64 // Publish-stage dropped count at last stats log
	lastClientDroppedCount uint64 // Client-stage dropped count at last stats log
	lastStatsMu            sync.Mutex

	// Lifecycle
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// BackgroundManagerInterface defines the interface for background management.
// This avoids circular imports with the lidar package.
type BackgroundManagerInterface interface {
	GenerateBackgroundSnapshot() (interface{}, error) // Returns *l3grid.BackgroundSnapshotData
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

// SetOnReplayEnded registers a callback fired once when a VRLOG replay reaches
// the end of its recording. Without it a finished replay stays the pipeline's
// data source forever: nothing else observes the end, so live input is never
// restored and the replay slot is never released.
func (p *Publisher) SetOnReplayEnded(fn func()) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()
	p.onReplayEnded = fn
}

// notifyReplayEnded invokes the end-of-replay callback on its own goroutine.
//
// The goroutine is required, not incidental: this is called from
// vrlogReplayLoop, and the callback's natural implementation stops the replay,
// which waits on vrlogWg — i.e. on this very goroutine. Calling it inline would
// deadlock.
func (p *Publisher) notifyReplayEnded() {
	p.vrlogMu.RLock()
	fn := p.onReplayEnded
	p.vrlogMu.RUnlock()
	if fn != nil {
		go fn()
	}
}

// SetBackgroundManager sets the background manager for split streaming (M3.5).
func (p *Publisher) SetBackgroundManager(mgr BackgroundManagerInterface) {
	p.backgroundMgr = mgr
}

// SetRecorder sets the frame recorder for VRLOG recording.
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

// shouldSendBackground determines if a background snapshot should be sent.
func (p *Publisher) shouldSendBackground() bool {
	if p.backgroundMgr == nil {
		return false // No background manager configured
	}

	// Suppress background snapshots during VRLOG replay
	// The recorded frames already have background data embedded
	if p.IsVRLogActive() {
		return false
	}

	// Send if:
	// 1. Never sent before, OR
	// 2. Interval elapsed, OR
	// 3. Grid sequence changed (reset/sensor moved)

	currentSeq := p.backgroundMgr.GetBackgroundSequenceNumber()

	p.backgroundMu.Lock()
	lastSeq := p.lastBackgroundSeq
	lastSent := p.lastBackgroundSent
	p.backgroundMu.Unlock()

	if lastSent.IsZero() {
		diagf("[Visualiser] First background snapshot, sending now")
		return true // Never sent
	}

	// Any change of sequence means the grid the client is holding is no longer
	// the grid we have. This deliberately includes a change away from 0: the
	// sequence is the persisted snapshot ID, so an unsettled grid reports 0 and
	// takes a real ID the moment a settled snapshot is restored. That is exactly
	// the settle-before-recording handover — settling pass publishes at 0, the
	// restore moves it to the snapshot ID — and the old `lastSeq > 0` guard
	// suppressed the refresh for precisely that transition, leaving the client
	// on the unsettled grid until the 30s interval elapsed. The never-sent case
	// above already covers the startup reading the guard was there for.
	if currentSeq != lastSeq {
		lidar.Diagf("[Visualiser] Background sequence changed (%d → %d), sending refresh", lastSeq, currentSeq)
		return true // Grid was reset or a settled snapshot was restored
	}

	elapsed := time.Since(lastSent)
	if elapsed >= p.config.BackgroundInterval {
		lidar.Diagf("[Visualiser] Background interval elapsed (%.1fs), sending refresh", elapsed.Seconds())
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

	// Skip background emission until the first foreground frame has set a
	// canonical timestamp.  Before that, the only available timestamp is
	// time.Now() which would contaminate VRLOG recordings of PCAP replays.
	fgTs := p.lastForegroundTimestamp.Load()
	if fgTs == 0 {
		lidar.Diagf("[Visualiser] Background snapshot deferred: no foreground frame yet (seq=%d)", snapshot.SequenceNumber)
		return nil
	}
	ts := fgTs
	bundle := &FrameBundle{
		FrameID:        p.frameCount.Add(1),
		TimestampNanos: ts,
		SensorID:       p.config.SensorID,
		FrameType:      FrameTypeBackground,
		Background:     snapshot,
		BackgroundSeq:  snapshot.SequenceNumber,
	}

	// Record background snapshot if recorder is set
	p.recorderMu.RLock()
	rec := p.recorder
	p.recorderMu.RUnlock()
	if rec != nil {
		if err := rec.Record(bundle); err != nil {
			opsf("[Visualiser] Recording error (background): %v", err)
		}
	}

	// Send to all clients
	select {
	case p.frameChan <- bundle:
		p.backgroundMu.Lock()
		p.lastBackgroundSeq = snapshot.SequenceNumber
		p.lastBackgroundSent = time.Now()
		p.backgroundMu.Unlock()
		pointCount := len(snapshot.X)
		lidar.Diagf("[Visualiser] Background snapshot sent: %d points, seq=%d", pointCount, snapshot.SequenceNumber)
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

	diagf("[Visualiser] Attempting to bind to %s...", p.config.ListenAddr)
	lis, err := net.Listen("tcp", p.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	diagf("[Visualiser] Successfully bound to %s", p.config.ListenAddr)
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
		diagf("[Visualiser] gRPC server listening on %s", p.config.ListenAddr)
		diagf("[Visualiser] Waiting for client connections...")
		if err := p.server.Serve(lis); err != nil && p.running.Load() {
			opsf("[Visualiser] gRPC server error: %v", err)
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
	diagf("[Visualiser] gRPC server stopped")
}

// Publish sends a frame to all connected clients.
// Publish broadcasts a frame produced by the live pipeline.
//
// Frames are dropped while a VRLOG replay is active. Only background snapshots
// were suppressed before, so live foreground frames continued to reach both the
// stream and any attached recorder, interleaving with the replayed frames.
func (p *Publisher) Publish(frame interface{}) {
	p.publishInternal(frame, false)
}

// publishReplay broadcasts a frame read back from a VRLOG.
func (p *Publisher) publishReplay(frame interface{}) {
	p.publishInternal(frame, true)
}

func (p *Publisher) publishInternal(frame interface{}, fromReplay bool) {
	if !p.running.Load() {
		return
	}

	// Type assert to *FrameBundle
	frameBundle, ok := frame.(*FrameBundle)
	if !ok || frameBundle == nil {
		return
	}

	// A VRLOG replay owns the stream: live frames arriving alongside it would
	// interleave with the recorded ones and be written to any active recorder.
	if !fromReplay && p.IsVRLogActive() {
		return
	}

	// M3.5: Check if we should send a background snapshot first
	if p.shouldSendBackground() {
		if err := p.sendBackgroundSnapshot(); err != nil {
			opsf("[Visualiser] Failed to send background snapshot: %v", err)
		}
	}

	// Track the most recent foreground frame timestamp so background
	// snapshots can inherit it instead of using wall-clock time.
	if frameBundle.FrameType != FrameTypeBackground {
		p.lastForegroundTimestamp.Store(frameBundle.TimestampNanos)
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

	// Set background sequence number for client cache coherence.
	//
	// Replayed frames keep the sequence they were recorded with. At record time
	// each frame was stamped from the same grid that produced the background
	// recorded alongside it, so a recording is already self-consistent — and
	// stays so across a mid-recording grid reset, where later background frames
	// carry a new sequence and the frames after them match it. Restamping with
	// the live grid's sequence would point replayed frames at a background the
	// client is not holding.
	//
	// The exception is a recording that holds no background of its own: there
	// the client is showing the live grid that SendBackgroundSnapshot sent as a
	// fallback, so the live sequence is the correct one to advertise.
	if p.backgroundMgr != nil && !(fromReplay && p.VRLogEmittedBackground()) {
		frameBundle.BackgroundSeq = p.backgroundMgr.GetBackgroundSequenceNumber()
	}

	// Record frame if recorder is set
	p.recorderMu.RLock()
	rec := p.recorder
	p.recorderMu.RUnlock()
	if rec != nil {
		if err := rec.Record(frameBundle); err != nil {
			opsf("[Visualiser] Recording error: %v", err)
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
		lidar.Diagf("[Visualiser] WARNING: Frame queue depth high: %d/100", queueDepth)
	}

	select {
	case p.frameChan <- frameBundle:
		count := p.frameCount.Add(1)
		// Log stats periodically (every 100 frames or 5 seconds)
		p.logPeriodicStats(count, pointCount, trackCount, clusterCount, queueDepth)
	default:
		// Drop frame if channel is full
		dropped := p.droppedFrames.Add(1)
		lidar.Opsf("[Visualiser] DROPPED frame %d (total dropped: %d), channel full, points=%d tracks=%d",
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
		p.lastDroppedCount = p.droppedFrames.Load()
		p.lastClientDroppedCount = p.clientDroppedFrames.Load()
		return
	}

	elapsed := now.Sub(p.lastStatsTime)
	if elapsed >= 5*time.Second {
		// Calculate frames in this interval (not total frames)
		framesInInterval := frameCount - p.lastFrameCount
		fps := float64(framesInInterval) / elapsed.Seconds()
		dropped := p.droppedFrames.Load()
		droppedInInterval := dropped - p.lastDroppedCount
		clientDropped := p.clientDroppedFrames.Load()
		clientDroppedInInterval := clientDropped - p.lastClientDroppedCount
		clients := p.clientCount.Load()
		lidar.Tracef("[Visualiser] Stats: fps=%.1f frames=%d dropped=%d(%d total) client_dropped=%d(%d total) clients=%d queue=%d/100 last_frame: points=%d tracks=%d clusters=%d",
			fps, framesInInterval, droppedInInterval, dropped, clientDroppedInInterval, clientDropped, clients, queueDepth, pointCount, trackCount, clusterCount)

		// Publish-stage loss: frames offered to the pipeline that never entered
		// it. The denominator is everything offered, so this is a true rate.
		if droppedInInterval > 0 {
			offered := framesInInterval + droppedInInterval
			dropPct := float64(droppedInInterval) / float64(offered) * 100
			if dropPct > 10 {
				lidar.Opsf("[Visualiser] WARNING: high publish drop rate %.1f%% (%d/%d frames never entered the pipeline in %.0fs)",
					dropPct, droppedInInterval, offered, elapsed.Seconds())
			}
		}

		// Client-stage loss: frames that were published and then rejected
		// because a client could not take them. The denominator is what was
		// published, not published+rejected — a frame is counted once on each
		// side, so summing them capped the reported rate at 50% for a single
		// client and read "this client is receiving nothing" as "half the
		// frames are getting through".
		if clientDroppedInInterval > 0 && framesInInterval > 0 {
			clientDropPct := float64(clientDroppedInInterval) / float64(framesInInterval) * 100
			if clientDropPct > 10 {
				lidar.Opsf("[Visualiser] WARNING: high client drop rate %.1f%% (%d of %d published frames rejected by slow clients in %.0fs, clients=%d)",
					clientDropPct, clientDroppedInInterval, framesInInterval, elapsed.Seconds(), clients)
			}
		}

		p.lastStatsTime = now
		p.lastFrameCount = frameCount
		p.lastDroppedCount = dropped
		p.lastClientDroppedCount = clientDropped
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
			// Remember the scene so a client that connects later can be given
			// it. A background frame is published once and never repeated
			// until the next refresh, so a client whose stream started even a
			// few milliseconds afterwards would render nothing over it — which
			// is exactly what a replay load does, publishing the recording's
			// background and then restarting the stream to pick it up.
			p.rememberBackground(frame)

			p.clientsMu.RLock()
			clientCount := len(p.clients)
			for _, client := range p.clients {
				// Retain for this client (M7 reference counting).
				// Release is called in streamFromPublisher after protobuf conversion.
				if frame.PointCloud != nil {
					frame.PointCloud.Retain()
				}
				if !p.enqueueForClient(client, frame) {
					// Release the Retain we just did since frame wasn't sent.
					if frame.PointCloud != nil {
						frame.PointCloud.Release()
					}
					// Client-stage loss: the frame was published, this client
					// could not take it. Counted separately from publish-stage
					// drops so the two are not summed into one ratio.
					p.clientDroppedFrames.Add(1)
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

// enqueueForClient queues a frame for one client, reporting whether it was
// accepted.
//
// A slow client normally just loses the frame: another foreground frame is along
// shortly and supersedes it. Background frames are different — the client
// renders the last one it received until another arrives, so dropping one during
// a source change leaves the previous source's scene under the new source's
// foreground. For those, evict the oldest queued frame and retry once, which
// bounds the queue without blocking the broadcast loop on a slow client.
func (p *Publisher) enqueueForClient(client *clientStream, frame *FrameBundle) bool {
	select {
	case client.frameCh <- frame:
		return true
	default:
	}

	if frame.FrameType != FrameTypeBackground {
		return false
	}

	select {
	case evicted := <-client.frameCh:
		if evicted != nil && evicted.PointCloud != nil {
			evicted.PointCloud.Release()
		}
		p.clientDroppedFrames.Add(1)
	default:
	}

	select {
	case client.frameCh <- frame:
		return true
	default:
		return false
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
	diagf("[Visualiser] Client connected: %s (total: %d)", id, p.clientCount.Load())

	// Hand the new client the current scene. Without this it renders over
	// nothing until the next background is published — 30s on live, and on a
	// replay only whenever the recording happens to contain another one.
	if bg := p.latestBackground(); bg != nil {
		select {
		case client.frameCh <- bg:
			diagf("[Visualiser] Sent cached background to new client %s", id)
		default:
		}
	}

	return client
}

// rememberBackground caches the most recent background frame so a client that
// subscribes later can be sent the scene it would otherwise have missed.
func (p *Publisher) rememberBackground(frame *FrameBundle) {
	if frame == nil || frame.FrameType != FrameTypeBackground {
		return
	}
	p.backgroundMu.Lock()
	p.lastBackgroundFrame = frame
	p.backgroundMu.Unlock()
}

// latestBackground returns the cached background frame, or nil if none has been
// published yet.
func (p *Publisher) latestBackground() *FrameBundle {
	p.backgroundMu.Lock()
	defer p.backgroundMu.Unlock()
	return p.lastBackgroundFrame
}

// removeClient unregisters a streaming client.
func (p *Publisher) removeClient(id string) {
	p.clientsMu.Lock()
	if client, ok := p.clients[id]; ok {
		close(client.doneCh)
		delete(p.clients, id)
		p.clientsMu.Unlock()
		p.clientCount.Add(-1)
		diagf("[Visualiser] Client disconnected: %s (remaining: %d)", id, p.clientCount.Load())
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

// SendBackgroundSnapshot sends a background snapshot of the live grid to
// clients, bypassing the interval and sequence checks in shouldSendBackground.
//
// It is a no-op while a VRLOG replay owns the stream. Clients cache whichever
// background arrives last, so the live grid would replace the recorded scene
// the replayed foreground belongs to — and when the recording carries no
// background of its own, painting the live one under replayed foreground is
// exactly the stale composite this avoids. ClearBackground covers that case
// instead.
//
// sendBackgroundSnapshot builds its own bundle and pushes to frameChan
// directly, so the live-frame drop in publishInternal does not cover it.
func (p *Publisher) SendBackgroundSnapshot() error {
	if p.IsVRLogActive() {
		diagf("[Visualiser] Skipping live background snapshot: a replay owns the stream")
		return nil
	}
	return p.sendBackgroundSnapshot()
}

// ClearBackground tells clients to drop the background they are holding, by
// publishing a background frame carrying no points.
//
// Sent when the pipeline changes source. The client keeps the last background
// it received until another replaces it, so without this a new source's
// foreground is composited over the previous source's scene — a live settled
// grid sitting under replayed points, which reads as a real scene and is not
// obviously wrong until something moves through it.
//
// Clearing is unconditional rather than conditional on the new source having a
// background of its own: showing nothing is honest, and a recording that does
// carry one overwrites this within the same startup.
func (p *Publisher) ClearBackground() {
	if !p.running.Load() {
		return
	}
	bundle := &FrameBundle{
		FrameID:        p.frameCount.Add(1),
		TimestampNanos: p.lastForegroundTimestamp.Load(),
		SensorID:       p.config.SensorID,
		FrameType:      FrameTypeBackground,
		Background:     &BackgroundSnapshot{},
	}
	select {
	case p.frameChan <- bundle:
		diagf("[Visualiser] Cleared client background for source change")
	default:
		opsf("[Visualiser] Could not clear client background: frame channel full")
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
