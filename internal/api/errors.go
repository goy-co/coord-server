package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ErrorResponse represents the standardized error format returned by the API.
type ErrorResponse struct {
	Error    string `json:"error"`
	Details  string `json:"details,omitempty"`
	Resource string `json:"resource,omitempty"`
	ID       string `json:"id,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// WriteJSONError sends a JSON-formatted HTTP error response.
func WriteJSONError(w http.ResponseWriter, statusCode int, resp ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteBadRequest sends a 400 Bad Request error.
func WriteBadRequest(w http.ResponseWriter, details string) {
	WriteJSONError(w, http.StatusBadRequest, ErrorResponse{
		Error:   "invalid request",
		Details: details,
	})
}

// WriteUnauthorized sends a 401 Unauthorized error.
func WriteUnauthorized(w http.ResponseWriter, details string) {
	if details == "" {
		details = "valid API key required"
	}
	WriteJSONError(w, http.StatusUnauthorized, ErrorResponse{
		Error:   "unauthorized",
		Details: details,
	})
}

// WriteNotFound sends a 404 Not Found error.
func WriteNotFound(w http.ResponseWriter, resource string, id string) {
	WriteJSONError(w, http.StatusNotFound, ErrorResponse{
		Error:    "not found",
		Resource: resource,
		ID:       id,
	})
}

// WriteTooManyRequests sends a 429 Too Many Requests error with the Retry-After header.
func WriteTooManyRequests(w http.ResponseWriter, retryAfterSeconds int) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	WriteJSONError(w, http.StatusTooManyRequests, ErrorResponse{
		Error:  "rate limit exceeded",
		Reason: "HTTP request limit exceeded",
	})
}

// WriteInternalServerError sends a 500 Internal Server Error (without exposing internal details).
func WriteInternalServerError(w http.ResponseWriter) {
	WriteJSONError(w, http.StatusInternalServerError, ErrorResponse{
		Error: "internal server error",
	})
}

// WriteServiceUnavailable sends a 503 Service Unavailable error.
func WriteServiceUnavailable(w http.ResponseWriter, reason string) {
	WriteJSONError(w, http.StatusServiceUnavailable, ErrorResponse{
		Error:  "service unavailable",
		Reason: reason,
	})
}
