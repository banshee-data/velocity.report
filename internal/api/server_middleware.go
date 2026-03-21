package api

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func statusCodeColor(statusCode int) string {
	// Return plain status code without color codes
	return strconv.Itoa(statusCode)
}

// LoggingMiddleware logs method, path, query, status, and duration to debug log
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{w, http.StatusOK}
		next.ServeHTTP(lrw, r)

		// Include the listener port (if present) ahead of the path for clarity across multiple servers.
		portPrefix := ""
		if host := r.Host; host != "" {
			if h, p, err := net.SplitHostPort(host); err == nil {
				_ = h // host not used currently
				portPrefix = ":" + p
			}
		}
		if portPrefix == "" {
			if p := r.URL.Port(); p != "" {
				portPrefix = ":" + p
			}
		}
		requestTarget := fmt.Sprintf("%s%s", portPrefix, r.RequestURI)
		log.Printf(
			"[%s] %s %s %.3fms",
			statusCodeColor(lrw.statusCode), r.Method,
			requestTarget,
			float64(time.Since(start).Nanoseconds())/1e6,
		)
	})
}
