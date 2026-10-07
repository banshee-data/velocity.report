package server

import (
	"net/http"
	"testing"

	"github.com/banshee-data/velocity.report/internal/access"
)

func TestControlRoutesRecordOperationPermissions(t *testing.T) {
	ws := NewServer(Config{Address: "127.0.0.1:0", Stats: NewPacketStats(), SensorID: "test-policy"})
	type operations struct{ read, write access.Operation }
	registered := map[string]operations{}
	ws.RegisterRoutes(http.NewServeMux(), func(pattern string, read, write access.Operation) { registered[pattern] = operations{read, write} })
	for _, tc := range []struct {
		pattern     string
		read, write access.Operation
	}{
		{"GET /api/lidar/status", access.ReadConfiguration, access.Configure},
		{"/api/lidar/params", access.ReadConfiguration, access.Configure},
		{"POST /api/lidar/persist", access.ExportData, access.Configure},
		{"POST /api/lidar/playback/play", access.ExportData, access.Configure},
		{"GET /api/lidar/snapshot", access.ExportData, access.Configure},
		{"POST /api/lidar/snapshots/cleanup", access.Maintenance, access.Maintenance},
		{"/debug/lidar", access.Maintenance, access.Maintenance},
	} {
		if got := registered[tc.pattern]; got != (operations{tc.read, tc.write}) {
			t.Errorf("%s: %+v", tc.pattern, got)
		}
	}
	if len(registered) < 50 {
		t.Fatalf("incomplete control-route inventory: %d", len(registered))
	}
}
