package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeLidarServer stands in for the LiDAR server: it reports ready and
// serves until ctx ends, or fails to start with startErr.
type fakeLidarServer struct {
	onReady  func()
	startErr error
	serving  chan struct{}
}

func (f *fakeLidarServer) SetOnReady(fn func()) { f.onReady = fn }

func (f *fakeLidarServer) Start(ctx context.Context) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.onReady()
	close(f.serving)
	<-ctx.Done()
	return nil
}

func lidarStatus(cp *capabilitiesProvider) (string, bool) {
	st, ok := cp.Capabilities().Lidar["default"]
	return st.Status, ok && st.Sweep
}

// Enabled and serving: starting until the server is up, then ready with
// sweeps, and it stays ready while it serves.
func TestRunLidarServer_ReportsReadyOnceServing(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarStarting()
	if status, _ := lidarStatus(cp); status != "starting" {
		t.Fatalf("before start: %q, want starting", status)
	}

	srv := &fakeLidarServer{serving: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runLidarServer(ctx, srv, cp) }()

	select {
	case <-srv.serving:
	case <-time.After(5 * time.Second):
		t.Fatal("server never started serving")
	}
	if status, sweep := lidarStatus(cp); status != "ready" || !sweep {
		t.Fatalf("serving: %q, sweep %v; want ready with sweep", status, sweep)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runLidarServer returned %v after shutdown", err)
	}
}

// Enabled but unable to start (a port in use, say): error, not starting.
func TestRunLidarServer_ReportsErrorWhenStartFails(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarStarting()
	bindErr := errors.New("listen tcp :8081: bind: address already in use")

	err := runLidarServer(context.Background(), &fakeLidarServer{startErr: bindErr}, cp)
	if !errors.Is(err, bindErr) {
		t.Fatalf("runLidarServer returned %v, want the start error", err)
	}
	if status, sweep := lidarStatus(cp); status != "error" || sweep {
		t.Fatalf("after failed start: %q, sweep %v; want error without sweep", status, sweep)
	}
}

// Disabled: no LiDAR server runs and the LiDAR map stays empty.
func TestRunLidarServer_DisabledReportsNoLidar(t *testing.T) {
	cp := newCapabilitiesProvider()
	if _, ok := cp.Capabilities().Lidar["default"]; ok {
		t.Fatal("LiDAR reported while disabled")
	}
}
