// Package httpx holds small HTTP helpers: JSON envelope and key middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// ErrBadJSON is returned when a request body is not valid JSON for the target.
var ErrBadJSON = errors.New("bad json")

// DecodeJSON decodes the request body (max 1 MiB) into dst. Unknown fields are allowed.
func DecodeJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	return nil
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the error envelope {"error": code, ...extra}.
func WriteError(w http.ResponseWriter, status int, code string, extra map[string]any) {
	body := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		body[k] = v
	}
	body["error"] = code
	WriteJSON(w, status, body)
}
