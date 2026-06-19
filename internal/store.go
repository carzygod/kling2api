package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type AccountRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type,omitempty"`
	Status           string `json:"status"`
	Enabled          bool   `json:"enabled"`
	CookieJSON       string `json:"cookie_json,omitempty"`
	CookieString     string `json:"cookie_string,omitempty"`
	LocalStorageJSON string `json:"local_storage_json,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ProxyURL         string `json:"proxy_url,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	LastTestAt       string `json:"last_test_at,omitempty"`
	LastSuccessAt    string `json:"last_success_at,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type TaskRecord struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Status            string `json:"status"`
	Model             string `json:"model,omitempty"`
	ProviderAccountID string `json:"provider_account_id,omitempty"`
	UpstreamTaskID    string `json:"upstream_task_id,omitempty"`
	RequestJSON       string `json:"request_json,omitempty"`
	ResponseJSON      string `json:"response_json,omitempty"`
	ResultJSON        string `json:"result_json,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
	ErrorMessage      string `json:"error_message,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

var AppStore *Store

func InitStore() error {
	db, err := sql.Open("sqlite", Cfg.DatabasePath)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return err
	}
	AppStore = store
	return nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS kling_accounts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'unknown',
  enabled INTEGER NOT NULL DEFAULT 1,
  cookie_json TEXT,
  cookie_string TEXT,
  local_storage_json TEXT,
  user_agent TEXT,
  proxy_url TEXT,
  last_error TEXT,
  last_test_at TEXT,
  last_success_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS kling_account_events (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  message TEXT,
  metadata_json TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS kling_tasks (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  model TEXT,
  provider_account_id TEXT,
  upstream_task_id TEXT,
  request_json TEXT,
  response_json TEXT,
  result_json TEXT,
  error_code TEXT,
  error_message TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT
);
`)
	return err
}

func (s *Store) CreateAccount(input AccountRecord) (*AccountRecord, error) {
	now := nowISO()
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Name == "" {
		input.Name = "kling-account"
	}
	if input.Status == "" {
		input.Status = "imported"
	}
	input.Enabled = true
	input.CreatedAt = now
	input.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO kling_accounts
    (id, name, status, enabled, cookie_json, cookie_string, local_storage_json, user_agent, proxy_url, last_error, last_test_at, last_success_at, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.Status, boolToInt(input.Enabled), input.CookieJSON, input.CookieString, input.LocalStorageJSON, input.UserAgent, input.ProxyURL, input.LastError, input.LastTestAt, input.LastSuccessAt, input.CreatedAt, input.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = s.AddEvent(input.ID, "created", "account created", nil)
	return &input, nil
}

func (s *Store) ListAccounts() ([]AccountRecord, error) {
	rows, err := s.db.Query(`SELECT id, name, status, enabled, cookie_json, cookie_string, local_storage_json, user_agent, proxy_url, last_error, last_test_at, last_success_at, created_at, updated_at FROM kling_accounts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []AccountRecord
	for rows.Next() {
		var a AccountRecord
		var enabled int
		if err := rows.Scan(&a.ID, &a.Name, &a.Status, &enabled, &a.CookieJSON, &a.CookieString, &a.LocalStorageJSON, &a.UserAgent, &a.ProxyURL, &a.LastError, &a.LastTestAt, &a.LastSuccessAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Enabled = enabled == 1
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetAccount(id string) (*AccountRecord, error) {
	var a AccountRecord
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, status, enabled, cookie_json, cookie_string, local_storage_json, user_agent, proxy_url, last_error, last_test_at, last_success_at, created_at, updated_at FROM kling_accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Name, &a.Status, &enabled, &a.CookieJSON, &a.CookieString, &a.LocalStorageJSON, &a.UserAgent, &a.ProxyURL, &a.LastError, &a.LastTestAt, &a.LastSuccessAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Enabled = enabled == 1
	return &a, nil
}

func (s *Store) SelectRunnableAccount(accountID string) (*AccountRecord, error) {
	if accountID != "" {
		return s.GetAccount(accountID)
	}
	var a AccountRecord
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, status, enabled, cookie_json, cookie_string, local_storage_json, user_agent, proxy_url, last_error, last_test_at, last_success_at, created_at, updated_at
		FROM kling_accounts
		WHERE enabled=1 AND COALESCE(cookie_string, '') <> ''
		ORDER BY CASE status WHEN 'valid' THEN 0 WHEN 'captured' THEN 1 WHEN 'imported' THEN 2 ELSE 3 END, updated_at DESC
		LIMIT 1`).
		Scan(&a.ID, &a.Name, &a.Status, &enabled, &a.CookieJSON, &a.CookieString, &a.LocalStorageJSON, &a.UserAgent, &a.ProxyURL, &a.LastError, &a.LastTestAt, &a.LastSuccessAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Enabled = enabled == 1
	return &a, nil
}

func (s *Store) DeleteAccount(id string) error {
	_, err := s.db.Exec(`DELETE FROM kling_accounts WHERE id=?`, id)
	if err == nil {
		_ = s.AddEvent(id, "deleted", "account deleted", nil)
	}
	return err
}

func (s *Store) CreateTask(input TaskRecord) (*TaskRecord, error) {
	now := nowISO()
	if input.ID == "" {
		input.ID = "task_" + uuid.NewString()
	}
	if input.Status == "" {
		input.Status = "queued"
	}
	input.CreatedAt = now
	input.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO kling_tasks
		(id, type, status, model, provider_account_id, upstream_task_id, request_json, response_json, result_json, error_code, error_message, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Type, input.Status, input.Model, input.ProviderAccountID, input.UpstreamTaskID, input.RequestJSON, input.ResponseJSON, input.ResultJSON, input.ErrorCode, input.ErrorMessage, input.CreatedAt, input.UpdatedAt, input.CompletedAt)
	if err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *Store) SetTaskSubmitted(id, upstreamTaskID string, response interface{}) error {
	_, err := s.db.Exec(`UPDATE kling_tasks SET status='submitted', upstream_task_id=?, response_json=?, updated_at=? WHERE id=?`,
		upstreamTaskID, mustJSON(response), nowISO(), id)
	return err
}

func (s *Store) SetTaskResult(id, status string, result interface{}, errorCode, errorMessage string) error {
	completedAt := ""
	if status == "succeeded" || status == "failed" {
		completedAt = nowISO()
	}
	_, err := s.db.Exec(`UPDATE kling_tasks SET status=?, result_json=?, error_code=?, error_message=?, updated_at=?, completed_at=COALESCE(NULLIF(?, ''), completed_at) WHERE id=?`,
		status, mustJSON(result), errorCode, errorMessage, nowISO(), completedAt, id)
	return err
}

func (s *Store) CancelTask(id string) error {
	now := nowISO()
	res, err := s.db.Exec(`UPDATE kling_tasks
		SET status='cancelled', error_code='cancelled', error_message='Task was cancelled locally.', updated_at=?, completed_at=COALESCE(NULLIF(completed_at, ''), ?)
		WHERE id=? AND status NOT IN ('succeeded', 'failed', 'cancelled')`,
		now, now, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM kling_tasks WHERE id=?`, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		return err
	}
	return nil
}

func (s *Store) GetTask(id string) (*TaskRecord, error) {
	var t TaskRecord
	err := s.db.QueryRow(`SELECT id, type, status, model, provider_account_id, upstream_task_id, request_json, response_json, result_json, error_code, error_message, created_at, updated_at, COALESCE(completed_at, '')
		FROM kling_tasks WHERE id=?`, id).
		Scan(&t.ID, &t.Type, &t.Status, &t.Model, &t.ProviderAccountID, &t.UpstreamTaskID, &t.RequestJSON, &t.ResponseJSON, &t.ResultJSON, &t.ErrorCode, &t.ErrorMessage, &t.CreatedAt, &t.UpdatedAt, &t.CompletedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTasks(limit int) ([]TaskRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, type, status, model, provider_account_id, upstream_task_id, request_json, response_json, result_json, error_code, error_message, created_at, updated_at, COALESCE(completed_at, '')
		FROM kling_tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []TaskRecord{}
	for rows.Next() {
		var t TaskRecord
		if err := rows.Scan(&t.ID, &t.Type, &t.Status, &t.Model, &t.ProviderAccountID, &t.UpstreamTaskID, &t.RequestJSON, &t.ResponseJSON, &t.ResultJSON, &t.ErrorCode, &t.ErrorMessage, &t.CreatedAt, &t.UpdatedAt, &t.CompletedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) SetAccountTestResult(id string, ok bool, message string) error {
	status := "invalid"
	successAt := ""
	if ok {
		status = "valid"
		successAt = nowISO()
	}
	_, err := s.db.Exec(`UPDATE kling_accounts SET status=?, last_error=?, last_test_at=?, last_success_at=COALESCE(NULLIF(?, ''), last_success_at), updated_at=? WHERE id=?`,
		status, message, nowISO(), successAt, nowISO(), id)
	return err
}

func (s *Store) AddEvent(accountID, eventType, message string, metadata interface{}) error {
	meta := ""
	if metadata != nil {
		b, _ := json.Marshal(metadata)
		meta = string(b)
	}
	_, err := s.db.Exec(`INSERT INTO kling_account_events (id, account_id, event_type, message, metadata_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), accountID, eventType, message, meta, nowISO())
	return err
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func mustJSON(value interface{}) string {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
