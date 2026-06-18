package internal

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

type createAccountRequest struct {
	Name             string `json:"name"`
	CookieString     string `json:"cookie_string"`
	CookieJSON       string `json:"cookie_json"`
	LocalStorageJSON string `json:"local_storage_json"`
	UserAgent        string `json:"user_agent"`
	ProxyURL         string `json:"proxy_url"`
}

type createSessionRequest struct {
	Name string `json:"name"`
}

type clickRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type inputRequest struct {
	Text string `json:"text"`
	Key  string `json:"key"`
}

func HandleAdminAPI(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) == 0 || parts[0] == "summary" {
		handleSummary(w, r)
		return
	}
	if len(parts) == 2 && parts[0] == "admin" && parts[1] == "summary" {
		handleSummary(w, r)
		return
	}
	switch parts[0] {
	case "accounts":
		handleAccountsAPI(w, r, parts[1:])
	case "login-sessions":
		handleLoginSessionsAPI(w, r, parts[1:])
	case "models":
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": klingModels()})
	case "tasks":
		handleTasksAPI(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown admin api path")
	}
}

func handleSummary(w http.ResponseWriter, r *http.Request) {
	accounts, _ := AppStore.ListAccounts()
	tasks, _ := AppStore.ListTasks(100)
	valid := 0
	for _, account := range accounts {
		if account.Status == "valid" {
			valid++
		}
	}
	payload := map[string]interface{}{
		"provider":      "KLING-CREATOR-01",
		"login_url":     Cfg.LoginURL,
		"database_path": Cfg.DatabasePath,
		"accounts":      len(accounts),
		"sessions":      sessionViews(LoginSessions.List()),
		"service": map[string]interface{}{
			"name":            "KLING-CREATOR-01",
			"host":            Cfg.Host,
			"port":            Cfg.Port,
			"login_url":       Cfg.LoginURL,
			"database_path":   Cfg.DatabasePath,
			"data_dir":        Cfg.DataDir,
			"public_base_url": "",
		},
		"account_stats": map[string]interface{}{"total": len(accounts), "valid": valid},
		"task_stats":    map[string]interface{}{"total": len(tasks)},
	}
	writeJSON(w, http.StatusOK, payload)
}

func handleAccountsAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			accounts, err := AppStore.ListAccounts()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "list_accounts_failed", err.Error())
				return
			}
			accounts = accountViews(accounts)
			writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": accounts, "data": accounts})
		case http.MethodPost:
			var req createAccountRequest
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, "bad_json", err.Error())
				return
			}
			if req.Name == "" {
				req.Name = "kling-account"
			}
			if req.CookieString == "" && req.CookieJSON == "" {
				session, err := LoginSessions.Create(req.Name)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "create_session_failed", err.Error())
					return
				}
				view := sessionView(session.publicCopy())
				writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
				return
			}
			account, err := AppStore.CreateAccount(AccountRecord{
				Name:             req.Name,
				Type:             "cookie",
				Status:           "imported",
				CookieJSON:       req.CookieJSON,
				CookieString:     req.CookieString,
				LocalStorageJSON: req.LocalStorageJSON,
				UserAgent:        req.UserAgent,
				ProxyURL:         req.ProxyURL,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "create_account_failed", err.Error())
				return
			}
			view := accountView(*account)
			writeJSON(w, http.StatusOK, map[string]interface{}{"account": view, "data": view})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "unsupported method")
		}
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := AppStore.DeleteAccount(id); err != nil {
			writeError(w, http.StatusInternalServerError, "delete_account_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost {
		account, err := AppStore.GetAccount(id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "account_not_found", "account not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "get_account_failed", err.Error())
			return
		}
		ok, message := testKlingAccount(account)
		_ = AppStore.SetAccountTestResult(id, ok, message)
		status := "invalid"
		if ok {
			status = "valid"
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": ok, "status": status, "message": message, "response_text": message})
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "unknown account api path")
}

func handleLoginSessionsAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			sessions := sessionViews(LoginSessions.List())
			writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": sessions, "data": sessions})
		case http.MethodPost:
			var req createSessionRequest
			_ = decodeJSON(r, &req)
			session, err := LoginSessions.Create(req.Name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "create_session_failed", err.Error())
				return
			}
			view := sessionView(session.publicCopy())
			writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "unsupported method")
		}
		return
	}
	session, ok := LoginSessions.Get(parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "login session not found")
		return
	}
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			view := sessionView(session.publicCopy())
			writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
			return
		}
		if r.Method == http.MethodDelete {
			LoginSessions.Delete(parts[0])
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
			return
		}
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found", "unknown login session path")
		return
	}
	switch parts[1] {
	case "screenshot", "qr-preview":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}
		writeSessionScreenshot(w, session)
	case "refresh":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		if err := session.Reload(); err != nil {
			writeError(w, http.StatusInternalServerError, "refresh_failed", err.Error())
			return
		}
		view := sessionView(session.publicCopy())
		writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
	case "click", "click-login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		if parts[1] == "click" {
			var req clickRequest
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, "bad_json", err.Error())
				return
			}
			if err := session.Click(req.X, req.Y); err != nil {
				writeError(w, http.StatusInternalServerError, "click_failed", err.Error())
				return
			}
		} else if err := session.Click(682, 450); err != nil {
			writeError(w, http.StatusInternalServerError, "click_login_failed", err.Error())
			return
		}
		view := sessionView(session.publicCopy())
		writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
	case "input", "type":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		var req inputRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		if err := session.Input(req.Text); err != nil {
			writeError(w, http.StatusInternalServerError, "input_failed", err.Error())
			return
		}
		view := sessionView(session.publicCopy())
		writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
	case "key":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		var req inputRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		if err := session.Input(keyText(req.Key)); err != nil {
			writeError(w, http.StatusInternalServerError, "key_failed", err.Error())
			return
		}
		view := sessionView(session.publicCopy())
		writeJSON(w, http.StatusOK, map[string]interface{}{"session": view, "data": view})
	case "capture":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		var req createSessionRequest
		_ = decodeJSON(r, &req)
		account, err := session.Capture(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "capture_failed", err.Error())
			return
		}
		view := accountView(*account)
		writeJSON(w, http.StatusOK, map[string]interface{}{"account": view, "session": sessionView(session.publicCopy()), "data": view})
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown login session action")
	}
}

func handleTasksAPI(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	tasks, err := AppStore.ListTasks(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_tasks_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": tasks})
}

func writeSessionScreenshot(w http.ResponseWriter, session *LoginSession) {
	image := session.Screenshot()
	if len(image) == 0 {
		_ = session.RefreshScreenshot()
		image = session.Screenshot()
	}
	if len(image) == 0 {
		writeError(w, http.StatusNotFound, "screenshot_not_ready", "screenshot is not ready")
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(image))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(image)))
	_, _ = w.Write(image)
}

func accountViews(accounts []AccountRecord) []AccountRecord {
	out := make([]AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, accountView(account))
	}
	return out
}

func accountView(account AccountRecord) AccountRecord {
	if account.Type == "" {
		account.Type = "kling-web-cookie"
	}
	return account
}

func sessionViews(sessions []*LoginSession) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, sessionView(session))
	}
	return out
}

func sessionView(session *LoginSession) map[string]interface{} {
	if session == nil {
		return map[string]interface{}{}
	}
	message := session.LastError
	if message == "" {
		message = "Open the screenshot, finish Kling login, then capture and test."
	}
	return map[string]interface{}{
		"id":             session.ID,
		"name":           session.Name,
		"status":         session.Status,
		"last_error":     session.LastError,
		"message":        message,
		"created_at":     session.CreatedAt,
		"updated_at":     session.UpdatedAt,
		"login_url":      session.LoginURL,
		"cookie_count":   0,
		"screenshot_url": "/api/login-sessions/" + session.ID + "/screenshot",
	}
}

func keyText(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter":
		return "\r"
	case "tab":
		return "\t"
	case "backspace":
		return "\b"
	case "escape", "esc":
		return "\x1b"
	default:
		return key
	}
}
