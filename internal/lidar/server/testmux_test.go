package server

import "net/http"

// testMux mounts the LiDAR API on a fresh mux, as the main server does.
func (ws *Server) testMux() *http.ServeMux {
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	return mux
}
