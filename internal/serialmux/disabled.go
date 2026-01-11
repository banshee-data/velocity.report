package serialmux

import (
	"context"
	"net/http"
	"sync"
)

// DisabledSerialMux is a no-op SerialMux implementation used when the radar
// hardware is absent (for --disable-radar). It allows the server and admin
// routes to run without a real device. Unlike the previous simple
// implementation, this version tracks subscribers so their channels can be
// deterministically closed on Unsubscribe() or Close(), allowing readers to
// unblock predictably during shutdown.
type DisabledSerialMux struct {
	mu          sync.Mutex
	subscribers map[string]chan string
	closing     bool
}

func NewDisabledSerialMux() *DisabledSerialMux {
	return &DisabledSerialMux{
		subscribers: make(map[string]chan string),
	}
}

func (d *DisabledSerialMux) Subscribe() (string, chan string) {
	id := randomID()
	ch := make(chan string)

	d.mu.Lock()
	if d.closing {
		// If already closing, return a closed channel so callers don't block.
		close(ch)
		d.mu.Unlock()
		return id, ch
	}
	d.subscribers[id] = ch
	d.mu.Unlock()
	return id, ch
}

func (d *DisabledSerialMux) Unsubscribe(id string) {
	d.mu.Lock()
	if ch, ok := d.subscribers[id]; ok {
		close(ch)
		delete(d.subscribers, id)
	}
	d.mu.Unlock()
}

func (d *DisabledSerialMux) SendCommand(string) error { return nil }

func (d *DisabledSerialMux) Monitor(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func (d *DisabledSerialMux) Close() error {
	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		return nil
	}
	d.closing = true
	// Close all subscriber channels
	for id, ch := range d.subscribers {
		close(ch)
		delete(d.subscribers, id)
	}
	d.mu.Unlock()
	return nil
}

func (d *DisabledSerialMux) Initialise() error { return nil }

func (d *DisabledSerialMux) AttachAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/debug/serial-disabled", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("serial disabled"))
	})
}
