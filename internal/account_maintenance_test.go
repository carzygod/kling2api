package internal

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newMaintenanceTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "kling-maintenance.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate() error = %v", err)
	}
	if _, err := store.CreateAccount(AccountRecord{
		ID:           "account-01",
		Name:         "maintenance-test",
		Status:       "valid",
		CookieString: "session=test",
	}); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store
}

func TestSafePathSegmentForAccountProfile(t *testing.T) {
	tests := map[string]string{
		"account-01":       "account-01",
		" account_02 ":     "account_02",
		"../../other-user": "______other-user",
		"账号 03":            "___03",
		"":                 "unknown",
	}
	for input, expected := range tests {
		if actual := safePathSegment(input); actual != expected {
			t.Fatalf("safePathSegment(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestMaintenanceLeaseBlocksSchedulingAndRequiresOwner(t *testing.T) {
	store := newMaintenanceTestStore(t)
	started, err := store.BeginAccountMaintenance("account-01", "owner-a", time.Minute)
	if err != nil {
		t.Fatalf("BeginAccountMaintenance() error = %v", err)
	}
	if started.State != "maintenance" || started.LeaseOwner != "owner-a" {
		t.Fatalf("maintenance = %+v", started)
	}
	if _, err := store.SelectRunnableAccount("account-01"); err == nil {
		t.Fatal("SelectRunnableAccount() succeeded during maintenance")
	}
	if _, err := store.HeartbeatAccountMaintenance("account-01", "owner-b", time.Minute); err == nil {
		t.Fatal("heartbeat with foreign owner succeeded")
	}
	if err := store.EndAccountMaintenance("account-01", "owner-b", ""); err == nil {
		t.Fatal("end with foreign owner succeeded")
	}
	if err := store.EndAccountMaintenance("account-01", "owner-a", ""); err != nil {
		t.Fatalf("EndAccountMaintenance() error = %v", err)
	}
	if _, err := store.SelectRunnableAccount("account-01"); err != nil {
		t.Fatalf("SelectRunnableAccount() after maintenance error = %v", err)
	}
}

func TestExpiredMaintenanceLeaseCanBeTakenOver(t *testing.T) {
	store := newMaintenanceTestStore(t)
	if _, err := store.BeginAccountMaintenance("account-01", "owner-a", time.Minute); err != nil {
		t.Fatalf("BeginAccountMaintenance() error = %v", err)
	}
	if _, err := store.db.Exec(
		"UPDATE kling_account_maintenance SET lease_expires_at=? WHERE account_id=?",
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		"account-01",
	); err != nil {
		t.Fatalf("expire lease error = %v", err)
	}
	started, err := store.BeginAccountMaintenance("account-01", "owner-b", time.Minute)
	if err != nil {
		t.Fatalf("take over expired lease error = %v", err)
	}
	if started.LeaseOwner != "owner-b" {
		t.Fatalf("lease owner = %q, want owner-b", started.LeaseOwner)
	}
}
