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
type SerialPortManager struct {
	mu       sync.RWMutex
	current  serialmux.SerialMuxInterface
	snapshot *SerialConfigSnapshot

	db      *db.DB
	factory SerialMuxFactory

	reloadMu sync.Mutex
}

// NewSerialPortManager constructs a SerialPortManager using the provided
// dependencies. The initial snapshot is optional; if the port path is empty the
// manager assumes no configuration has been applied yet.
func NewSerialPortManager(database *db.DB, initial serialmux.SerialMuxInterface, snapshot SerialConfigSnapshot, factory SerialMuxFactory) *SerialPortManager {
	mgr := &SerialPortManager{
		current: initial,
		db:      database,
		factory: factory,
	}

	if snapshot.PortPath != "" {
		snap := snapshot
		mgr.snapshot = &snap
	}

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

// Subscribe delegates to the current serial mux. If the mux is unavailable a
// closed channel is returned so callers can exit gracefully.
func (m *SerialPortManager) Subscribe() (string, chan string) {
	mux := m.CurrentMux()
	if mux == nil {
		ch := make(chan string)
		close(ch)
		return "", ch
	}
	return mux.Subscribe()
}

// Unsubscribe delegates to the current serial mux when available.
func (m *SerialPortManager) Unsubscribe(id string) {
	if mux := m.CurrentMux(); mux != nil {
		mux.Unsubscribe(id)
	}
}

// SendCommand delegates to the current serial mux.
func (m *SerialPortManager) SendCommand(command string) error {
	mux := m.CurrentMux()
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

// Close closes the currently active mux.
func (m *SerialPortManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	err := m.current.Close()
	m.current = nil
	return err
}

// Initialize delegates to the active mux.
func (m *SerialPortManager) Initialize() error {
	mux := m.CurrentMux()
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
// active mux in a thread-safe manner. Any existing monitor/subscriber routines
// automatically reconnect via the SerialMuxInterface methods.
func (m *SerialPortManager) ReloadConfig(ctx context.Context) (*SerialReloadResult, error) {
	if m.factory == nil {
		return nil, errors.New("serial mux factory not configured")
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
	if currentSnap.PortPath == cfg.PortPath && currentSnap.Options.Equal(normalized) {
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

	newMux, err := m.factory(cfg.PortPath, normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", cfg.PortPath, err)
	}

	if err := newMux.Initialize(); err != nil {
		newMux.Close()
		return nil, fmt.Errorf("failed to initialise serial port: %w", err)
	}

	m.mu.Lock()
	oldMux := m.current
	snap := SerialConfigSnapshot{
		ConfigID: cfg.ID,
		Name:     cfg.Name,
		PortPath: cfg.PortPath,
		Source:   "database",
		Options:  normalized,
	}
	m.current = newMux
	m.snapshot = &snap
	m.mu.Unlock()

	if oldMux != nil {
		if err := oldMux.Close(); err != nil {
			log.Printf("warning: failed to close previous serial mux: %v", err)
		}
	}

	return &SerialReloadResult{
		Success: true,
		Message: fmt.Sprintf("Reloaded serial configuration %q", cfg.Name),
		Config:  &snap,
	}, nil
}
