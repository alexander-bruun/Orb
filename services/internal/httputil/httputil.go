// Package httputil provides shared HTTP helpers for JSON responses
// and query-parameter parsing used across handler packages.
package httputil

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// WriteJSON writes v as JSON with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteOK writes v as JSON with HTTP 200.
func WriteOK(w http.ResponseWriter, v any) {
	WriteJSON(w, http.StatusOK, v)
}

// WriteErr writes a JSON error envelope: {"error": msg}.
func WriteErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// QueryInt reads a query parameter as an integer, returning def if
// the parameter is missing, empty, or not a valid non-negative integer.
func QueryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// Pagination reads "limit" and "offset" query parameters, clamping
// limit to [1, maxLimit]. If limit is missing or invalid, def is used.
func Pagination(r *http.Request, def, maxLimit int) (limit, offset int) {
	limit = QueryInt(r, "limit", def)
	if limit <= 0 || limit > maxLimit {
		limit = def
	}
	offset = QueryInt(r, "offset", 0)
	return
}
