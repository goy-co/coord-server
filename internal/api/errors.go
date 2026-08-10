package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ErrorResponse representa o formato padronizado de erro retornado pela API.
type ErrorResponse struct {
	Error    string `json:"error"`
	Details  string `json:"details,omitempty"`
	Resource string `json:"resource,omitempty"`
	ID       string `json:"id,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// WriteJSONError envia uma resposta HTTP de erro formatada em JSON.
func WriteJSONError(w http.ResponseWriter, statusCode int, resp ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteBadRequest envia um erro 400 Bad Request.
func WriteBadRequest(w http.ResponseWriter, details string) {
	WriteJSONError(w, http.StatusBadRequest, ErrorResponse{
		Error:   "invalid request",
		Details: details,
	})
}

// WriteUnauthorized envia um erro 401 Unauthorized.
func WriteUnauthorized(w http.ResponseWriter, details string) {
	if details == "" {
		details = "valid API key required"
	}
	WriteJSONError(w, http.StatusUnauthorized, ErrorResponse{
		Error:   "unauthorized",
		Details: details,
	})
}

// WriteNotFound envia um erro 404 Not Found.
func WriteNotFound(w http.ResponseWriter, resource string, id string) {
	WriteJSONError(w, http.StatusNotFound, ErrorResponse{
		Error:    "not found",
		Resource: resource,
		ID:       id,
	})
}

// WriteTooManyRequests envia um erro 429 Too Many Requests com o header Retry-After.
func WriteTooManyRequests(w http.ResponseWriter, retryAfterSeconds int) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	WriteJSONError(w, http.StatusTooManyRequests, ErrorResponse{
		Error:  "rate limit exceeded",
		Reason: "limite de pedidos HTTP excedido",
	})
}

// WriteInternalServerError envia um erro 500 Internal Server Error (sem expor detalhes internos).
func WriteInternalServerError(w http.ResponseWriter) {
	WriteJSONError(w, http.StatusInternalServerError, ErrorResponse{
		Error: "internal server error",
	})
}

// WriteServiceUnavailable envia um erro 503 Service Unavailable.
func WriteServiceUnavailable(w http.ResponseWriter, reason string) {
	WriteJSONError(w, http.StatusServiceUnavailable, ErrorResponse{
		Error:  "service unavailable",
		Reason: reason,
	})
}
