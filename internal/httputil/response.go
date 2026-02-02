package httputil

import (
	"encoding/json"
	"log"
	"net/http"
)

// WriteJSONError writes a JSON error response with the given status code and message.
// This helper reduces duplication across API handlers.
func WriteJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("failed to encode json error response: %v", err)
	}
}

// WriteJSON writes a JSON response with the given status code and data.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode json response: %v", err)
	}
}

// WriteJSONOK writes a successful JSON response (200 OK).
func WriteJSONOK(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusOK, data)
}

// MethodNotAllowed writes a 405 Method Not Allowed response.
func MethodNotAllowed(w http.ResponseWriter) {
	WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// BadRequest writes a 400 Bad Request response with the given message.
func BadRequest(w http.ResponseWriter, msg string) {
	WriteJSONError(w, http.StatusBadRequest, msg)
}

// InternalServerError writes a 500 Internal Server Error response.
func InternalServerError(w http.ResponseWriter, msg string) {
	WriteJSONError(w, http.StatusInternalServerError, msg)
}

// NotFound writes a 404 Not Found response.
func NotFound(w http.ResponseWriter, msg string) {
	WriteJSONError(w, http.StatusNotFound, msg)
}
