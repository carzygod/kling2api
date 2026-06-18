package internal

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"provider": "KLING-CREATOR-01",
		"loginUrl": Cfg.LoginURL,
	})
}
