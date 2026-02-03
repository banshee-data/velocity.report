// Package visualiser provides gRPC streaming of LiDAR perception data
// to the macOS visualiser application.
//
// This package implements Track B of the visualiser project:
// - Canonical internal model (FrameBundle)
// - gRPC publisher for live streaming
// - Adapter layer for existing LidarView forwarding
package visualiser

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
)

// Config holds configuration for the visualizer gRPC server.
type Config struct {
	// ListenAddr is the address to listen on (e.g., "localhost:50051")
	ListenAddr string

	// SensorID is the default sensor ID for streaming
	SensorID string

	// EnableDebug enables debug overlay emission
	EnableDebug bool

	// MaxClients is the maximum number of concurrent streaming clients
	MaxClients int
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		ListenAddr:  "localhost:50051",
		SensorID:    "hesai-01",
		EnableDebug: false,
		MaxClients:  5,
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

	// Stats
	frameCount  atomic.Uint64
	clientCount atomic.Int32

	// Lifecycle
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// clientStream represents a connected streaming client.
type clientStream struct {
	id      string
	request *StreamRequest
	frameCh chan *FrameBundle
	doneCh  chan struct{}
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

// Start starts the gRPC server.
func (p *Publisher) Start() error {
	if p.running.Load() {
		return fmt.Errorf("publisher already running")
	}

	lis, err := net.Listen("tcp", p.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	p.listener = lis

	p.server = grpc.NewServer()
	// TODO: Register VisualizerService when proto is generated
	// pb.RegisterVisualizerServiceServer(p.server, p)

	p.running.Store(true)

	// Start broadcast goroutine
	p.wg.Add(1)
	go p.broadcastLoop()

	// Start server in background
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("[Visualiser] gRPC server listening on %s", p.config.ListenAddr)
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
func (p *Publisher) Publish(frame *FrameBundle) {
	if !p.running.Load() {
		return
	}

	select {
	case p.frameChan <- frame:
		p.frameCount.Add(1)
	default:
		// Drop frame if channel is full
		log.Printf("[Visualiser] Dropping frame %d, channel full", frame.FrameID)
	}
}

// broadcastLoop distributes frames to all connected clients.
func (p *Publisher) broadcastLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		case frame := <-p.frameChan:
			p.clientsMu.RLock()
			for _, client := range p.clients {
				select {
				case client.frameCh <- frame:
				default:
					// Client is slow, drop frame for this client
				}
			}
			p.clientsMu.RUnlock()
		}
	}
}

// addClient registers a new streaming client.
func (p *Publisher) addClient(id string, req *StreamRequest) *clientStream {
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

// StreamFrames implements the VisualizerService.StreamFrames RPC.
// TODO: Implement when proto is generated
func (p *Publisher) StreamFrames(ctx context.Context, req *StreamRequest) error {
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := p.addClient(clientID, req)
	defer p.removeClient(clientID)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stopCh:
			return nil
		case frame := <-client.frameCh:
			// TODO: Send frame via gRPC stream
			_ = frame
		}
	}
}
