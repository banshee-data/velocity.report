package serialmux

import (
	"context"
	"net/http"
)

// DisabledSerialMux is a no-op SerialMux implementation used when the radar
// hardware is absent (for --disable-radar). It allows the server and admin
// routes to run without a real device.
type DisabledSerialMux struct{}

func NewDisabledSerialMux() *DisabledSerialMux { return &DisabledSerialMux{} }

func (d *DisabledSerialMux) Subscribe() (string, chan string) {
	// Return a channel that never receives (and is not closed) so
	// subscribers block on reads instead of receiving the zero value
	// repeatedly. This prevents tight-loop logging when the serial
	// device is disabled.
	ch := make(chan string)
	return "disabled", ch
}

func (d *DisabledSerialMux) Unsubscribe(string) {}

func (d *DisabledSerialMux) SendCommand(string) error { return nil }

func (d *DisabledSerialMux) Monitor(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func (d *DisabledSerialMux) Close() error { return nil }

func (d *DisabledSerialMux) Initialize() error { return nil }

func (d *DisabledSerialMux) AttachAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/debug/serial-disabled", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("serial disabled"))
	})
}
