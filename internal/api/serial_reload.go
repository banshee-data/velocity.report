package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

// SerialMuxFactory defines a factory function for constructing a
// serialmux.SerialMuxInterface using the provided port path and options. It is
// injected so the manager can be tested and so different runtime modes (real,
// mock, disabled) can supply their own constructors.
type SerialMuxFactory func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error)

// SerialConfigSnapshot describes the configuration that is currently applied to
// the running serial mux. It mirrors the user-facing serial configuration model
// so that API responses can report the active settings.
type SerialConfigSnapshot struct {
	ConfigID int                   `json:"config_id,omitempty"`
	Name     string                `json:"name,omitempty"`
	PortPath string                `json:"port_path"`
	Source   string                `json:"source"`
	Options  serialmux.PortOptions `json:"options"`
}

// SerialReloadResult is returned to API clients when a reload request is
// processed.
type SerialReloadResult struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Config  *SerialConfigSnapshot `json:"config,omitempty"`
}

// SerialPortManager wraps a SerialMuxInterface and enables hot-reloading of the
// underlying serial configuration. It implements the SerialMuxInterface itself
// so that existing call sites (API handlers, admin routes, monitor routines)
// automatically delegate to the active mux without additional wiring.
//
// Event Fanout:
// To preserve subscriptions across mux reloads, SerialPortManager maintains an
// internal event fanout system. When Subscribe() is called, clients receive
// channels from the fanout, not directly from the mux. A background goroutine
// subscribes to the current mux and forwards all events to the fanout. When the
// mux is reloaded, the goroutine automatically reconnects to the new mux, ensuring
// no event data is lost and all existing subscriptions remain valid.
//
// Thread-safety:
//   - CurrentMux(), SendCommand(), and Initialize() safely read/delegate to the
//     current mux using RWMutex protection.
//   - Subscribe() and Unsubscribe() work with persistent fanout channels that
//     survive mux reloads.
//   - Close() should only be called during shutdown; it signals the fanout loop
//     to exit and closes all subscriber channels. After Close(), subsequent calls
//     to SendCommand() and Initialize() will return an error, and Subscribe() will
//     return a closed channel.
//   - ReloadConfig() safely swaps the mux; the fanout loop automatically reconnects.
type SerialPortManager struct {
	mu       sync.RWMutex
	current  serialmux.SerialMuxInterface
	snapshot *SerialConfigSnapshot
	closed   bool // Tracks whether Close() has been called

	db      *db.DB
	factory SerialMuxFactory

	reloadMu sync.Mutex

	// Event fanout: bridges subscriptions across mux reloads
	eventFanoutCh chan string            // Input from mux subscription (internal use)
	fanoutMu      sync.RWMutex           // Protects subscribers map
	subscribers   map[string]chan string // Maps subscriber ID -> channel
}

// NewSerialPortManager constructs a SerialPortManager using the provided
// dependencies. The initial snapshot is optional; if the port path is empty the
// manager assumes no configuration has been applied yet.
//
// This constructor also starts an internal event fanout goroutine that bridges
// subscriptions across mux reloads. The goroutine will run until Close() is called.
func NewSerialPortManager(database *db.DB, initial serialmux.SerialMuxInterface, snapshot SerialConfigSnapshot, factory SerialMuxFactory) *SerialPortManager {
	mgr := &SerialPortManager{
		current:       initial,
		db:            database,
		factory:       factory,
		eventFanoutCh: make(chan string, 100), // Buffered to prevent fanout from blocking
		subscribers:   make(map[string]chan string),
	}

	if snapshot.PortPath != "" {
		snap := snapshot
		mgr.snapshot = &snap
	}

	// Start the event fanout goroutine that bridges subscriptions across reloads
	go mgr.runEventFanout()

	return mgr
}

// CurrentMux returns the underlying serial mux currently in use. Callers must
// treat the returned value as read-only; any reconfiguration should occur via
// ReloadConfig.
func (m *SerialPortManager) CurrentMux() serialmux.SerialMuxInterface {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Snapshot returns a copy of the active configuration snapshot.
func (m *SerialPortManager) Snapshot() SerialConfigSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.snapshot == nil {
		return SerialConfigSnapshot{}
	}
	snap := *m.snapshot
	return snap
}

// runEventFanout is an internal goroutine that bridges subscriptions across mux
// reloads. It continuously subscribes to the current mux and forwards all events
// to persistent subscriber channels. When the mux is reloaded, it automatically
// reconnects to the new mux.
//
// This loop runs until Close() is called (signaled via eventFanoutCh being closed).
func (m *SerialPortManager) runEventFanout() {
	var currentSubID string
	var currentSubCh chan string

	defer func() {
		// Cleanup: unsubscribe from current mux if active
		if currentSubID != "" {
			m.mu.RLock()
			mux := m.current
			m.mu.RUnlock()
			if mux != nil {
				mux.Unsubscribe(currentSubID)
			}
		}

		// Close all subscriber channels to signal shutdown
		m.fanoutMu.Lock()
		for _, ch := range m.subscribers {
			close(ch)
		}
		m.subscribers = make(map[string]chan string)
		m.fanoutMu.Unlock()

		log.Printf("Event fanout loop terminated")
	}()

	for {
		// Ensure we're subscribed to the current mux
		if currentSubID == "" {
			m.mu.RLock()
			mux := m.current
			closed := m.closed
			m.mu.RUnlock()

			if closed {
				return // Exit on shutdown
			}

			if mux != nil {
				currentSubID, currentSubCh = mux.Subscribe()
				if currentSubID == "" {
					// Likely nil mux, wait and retry
					time.Sleep(250 * time.Millisecond)
					continue
				}
			} else {
				time.Sleep(250 * time.Millisecond)
				continue
			}
		}

		// Wait for an event or shutdown signal
		select {
		case <-m.eventFanoutCh:
			// Shutdown signal (channel closed)
			return

		case payload, ok := <-currentSubCh:
			if !ok {
				// Mux subscription closed (likely due to reload), reconnect on next loop
				currentSubID = ""
				currentSubCh = nil
				log.Printf("Event fanout: mux subscription closed, reconnecting on next iteration")
				time.Sleep(50 * time.Millisecond)
				continue
			}

			// Forward event to all subscribers
			m.fanoutMu.RLock()
			subscribers := make([]chan string, 0, len(m.subscribers))
			for _, ch := range m.subscribers {
				subscribers = append(subscribers, ch)
			}
			m.fanoutMu.RUnlock()

			// Send to all subscribers without blocking
			for _, ch := range subscribers {
				select {
				case ch <- payload:
				default:
					log.Printf("Event fanout: subscriber channel full, dropping event")
				}
			}
		}
	}
}

// Subscribe returns a persistent channel from the internal fanout system. This
// channel will remain valid even if the underlying mux is reloaded. Events from
// the current mux are automatically forwarded to all subscriber channels.
//
// If the manager is closed, Subscribe returns a closed channel.
func (m *SerialPortManager) Subscribe() (string, chan string) {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		ch := make(chan string)
		close(ch)
		return "", ch
	}

	// Generate a unique subscriber ID
	id := fmt.Sprintf("subscriber-%d", time.Now().UnixNano())

	// Create a buffered channel for this subscriber
	ch := make(chan string, 10)

	// Register the subscriber
	m.fanoutMu.Lock()
	m.subscribers[id] = ch
	m.fanoutMu.Unlock()

	return id, ch
}

// Unsubscribe removes a subscriber from the fanout system and closes its channel.
func (m *SerialPortManager) Unsubscribe(id string) {
	m.fanoutMu.Lock()
	defer m.fanoutMu.Unlock()

	if ch, ok := m.subscribers[id]; ok {
		close(ch)
		delete(m.subscribers, id)
	}
}

// SendCommand delegates to the current serial mux. Returns an error if the mux
// is unavailable or has been closed.
func (m *SerialPortManager) SendCommand(command string) error {
	m.mu.RLock()
	mux := m.current
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return errors.New("serial manager is closed")
	}
	if mux == nil {
		return errors.New("serial mux unavailable")
	}
	return mux.SendCommand(command)
}

// Monitor proxies Monitor calls to the active mux. When the underlying mux is
// swapped out due to a reload this loop will attach to the new mux automatically.
func (m *SerialPortManager) Monitor(ctx context.Context) error {
	for {
		mux := m.CurrentMux()
		if mux == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}

		err := mux.Monitor(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("serial monitor terminated with error: %v", err)
			time.Sleep(500 * time.Millisecond)
		} else if err == nil {
			// Monitor exited cleanly (likely due to a reload). Loop back to
			// pick up the new mux.
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Close closes the currently active mux and marks the manager as closed.
// It also signals the internal event fanout loop to shut down. This method
// should only be called during shutdown; callers must ensure that no concurrent
// operations (SendCommand, Initialize, Subscribe, etc.) are in progress.
//
// After Close() is called:
//   - SendCommand() and Initialize() will return an error
//   - Subscribe() will return a closed channel
//   - All existing subscriber channels will be closed
func (m *SerialPortManager) Close() error {
	m.mu.Lock()
	m.closed = true
	if m.current != nil {
		if err := m.current.Close(); err != nil {
			log.Printf("Warning: failed to close current mux during shutdown: %v", err)
		}
	}
	m.current = nil
	m.mu.Unlock()

	// Signal the event fanout loop to exit
	close(m.eventFanoutCh)

	return nil
}

// Initialize delegates to the active mux. Returns an error if the mux is
// unavailable or has been closed.
func (m *SerialPortManager) Initialize() error {
	m.mu.RLock()
	mux := m.current
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return errors.New("serial manager is closed")
	}
	if mux == nil {
		return errors.New("serial mux unavailable")
	}
	return mux.Initialize()
}

// AttachAdminRoutes reuses the generic helper so debug routes call through the
// manager.
func (m *SerialPortManager) AttachAdminRoutes(mux *http.ServeMux) {
	serialmux.AttachAdminRoutesForMux(mux, m)
}

// ReloadConfig reloads the serial configuration from the database and swaps the
// active mux in a thread-safe manner.
//
// Note: The Monitor() method (and routines using it) will automatically reconnect
// after a reload. Subscriber channels obtained via Subscribe() remain valid across
// reloads: the fanout goroutine reconnects to the new mux, and subscriber channels
// persist in the manager's subscribers map. Only the internal mux channel is closed
// and replaced; subscriber-facing channels continue to receive events after reload.
func (m *SerialPortManager) ReloadConfig(ctx context.Context) (*SerialReloadResult, error) {
	if m.factory == nil {
		return nil, errors.New("serial mux factory not configured")
	}
	if m.db == nil {
		return nil, errors.New("database not configured")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	configs, err := m.db.GetEnabledSerialConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load serial configurations: %w", err)
	}
	if len(configs) == 0 {
		return nil, errors.New("no enabled serial configurations found")
	}

	cfg := configs[0]
	opts := serialmux.PortOptions{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		StopBits: cfg.StopBits,
		Parity:   cfg.Parity,
	}
	normalized, err := opts.Normalize()
	if err != nil {
		return nil, fmt.Errorf("invalid serial configuration: %w", err)
	}

	currentSnap := m.Snapshot()
	optionsEqual, err := currentSnap.Options.Equal(normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to compare serial options: %w", err)
	}
	if currentSnap.PortPath == cfg.PortPath && optionsEqual {
		return &SerialReloadResult{
			Success: true,
			Message: fmt.Sprintf("Serial configuration %q already active", cfg.Name),
			Config: &SerialConfigSnapshot{
				ConfigID: cfg.ID,
				Name:     cfg.Name,
				PortPath: cfg.PortPath,
				Source:   "database",
				Options:  normalized,
			},
		}, nil
	}

	// Close the old mux BEFORE opening the new one.
	// This is necessary because serial ports cannot be opened twice, and if the
	// new configuration uses the same port as the current one (with different
	// settings), we must release the port first.
	m.mu.Lock()
	oldMux := m.current
	m.current = nil // Clear current mux while we're switching
	m.mu.Unlock()

	if oldMux != nil {
		log.Printf("Closing current serial mux before reload...")
		if err := oldMux.Close(); err != nil {
			log.Printf("warning: failed to close previous serial mux: %v", err)
		}
	}

	// Now open the new mux
	newMux, err := m.factory(cfg.PortPath, normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", cfg.PortPath, err)
	}

	if err := newMux.Initialize(); err != nil {
		newMux.Close()
		return nil, fmt.Errorf("failed to initialize serial port: %w", err)
	}

	// Update the current mux and snapshot
	m.mu.Lock()
	snap := SerialConfigSnapshot{
		ConfigID: cfg.ID,
		Name:     cfg.Name,
		PortPath: cfg.PortPath,
		Source:   "database",
		Options:  normalized,
	}
	m.current = newMux
	m.snapshot = &snap
	// Note: We do NOT set m.closed = true here because this is not a shutdown.
	// m.closed is only set during Close() to signal shutdown to all methods.
	m.mu.Unlock()

	return &SerialReloadResult{
		Success: true,
		Message: fmt.Sprintf("Reloaded serial configuration %q", cfg.Name),
		Config:  &snap,
	}, nil
}
