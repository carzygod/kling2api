package internal

import (
	"encoding/json"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func decodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if Cfg.AdminKey == "" {
		return true
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.Header.Get("X-Admin-Key")
	}
	if key == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			key = strings.TrimSpace(auth[7:])
		}
	}
	if key != Cfg.AdminKey {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid admin key")
		return false
	}
	return true
}

func requireAPIKey(w http.ResponseWriter, r *http.Request) bool {
	if Cfg.APIKey == "" {
		return true
	}
	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	if key == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			key = strings.TrimSpace(auth[7:])
		}
	}
	if key != Cfg.APIKey {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid api key")
		return false
	}
	return true
}
