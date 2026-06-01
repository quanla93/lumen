// Package publicapi serves the versioned, key-authenticated Public Read
// API at /api/v1/*. It owns the wire envelope, Bearer-token middleware,
// per-key rate limiting, and the v1 endpoint handlers themselves.
//
// The envelope shape is deliberately different from the internal
// /api/* surface used by the web UI — terse error JSON for internal,
// rich success/data/error/request_id for public so integrators get the
// stability + introspection a stable API should have.
package publicapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// Envelope is the standard shape for every /api/v1/* response.
type Envelope struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Error     *APIError   `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

// APIError carries a stable code (uppercase snake) + human message.
// Code is what integrators should switch on; message is for humans.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes — kept short + grep-able. Any new code must be added here
// so we don't end up with three spellings of the same failure.
const (
	CodeMissingAuth   = "MISSING_AUTH"
	CodeInvalidAuth   = "INVALID_AUTH"
	CodeInsufficient  = "INSUFFICIENT_SCOPE"
	CodeRateLimit     = "RATE_LIMITED"
	CodeNotFound      = "NOT_FOUND"
	CodeBadRequest    = "BAD_REQUEST"
	CodeInternalError = "INTERNAL_ERROR"
)

// WriteSuccess emits a 200 envelope with data. status is configurable
// so handlers can return 201/204 if they ever need to (write endpoints
// land post-v0.5.0).
func WriteSuccess(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success:   true,
		Data:      data,
		RequestID: middleware.GetReqID(r.Context()),
	})
}

// WriteError emits a non-2xx envelope. status is the HTTP code; code +
// message form the body's `error` field.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success:   false,
		Error:     &APIError{Code: code, Message: message},
		RequestID: middleware.GetReqID(r.Context()),
	})
}
