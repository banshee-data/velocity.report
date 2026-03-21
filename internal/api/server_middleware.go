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
	statusCode  int
	wroteHeader bool
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if !lrw.wroteHeader {
		lrw.statusCode = code
		lrw.wroteHeader = true
	}
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(p []byte) (int, error) {
	if !lrw.wroteHeader {
		lrw.WriteHeader(http.StatusOK)
	}
	return lrw.ResponseWriter.Write(p)
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func statusCodeColor(statusCode int) string {
	// Return plain status code without color codes
	return strconv.Itoa(statusCode)
}

// LoggingMiddleware logs method, path, query, status, and duration to debug log
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
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
