package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// parseIntParam parses an integer query parameter from the request, returning
// defaultVal if the parameter is missing, unparseable, or out of [min, max].
func parseIntParam(r *http.Request, name string, defaultVal, min, max int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < min || v > max {
		return defaultVal
	}
	return v
}

// writeError sends a JSON error response with the given status code.
func writeError(w http.ResponseWriter, statusCode int, message string, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}

// writeJSON sends a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
