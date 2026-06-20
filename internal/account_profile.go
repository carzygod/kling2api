package internal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var accountChromeProfileLocks sync.Map

func lockAccountChromeProfile(accountID string) func() {
	if accountID == "" {
		return func() {}
	}
	value, _ := accountChromeProfileLocks.LoadOrStore(accountID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func accountChromeProfilePath(accountID string) string {
	return filepath.Join(Cfg.DataDir, "account-chrome-profiles", safePathSegment(accountID))
}

func accountChromeProfileExists(accountID string) bool {
	if accountID == "" {
		return false
	}
	profile := accountChromeProfilePath(accountID)
	for _, rel := range []string{
		filepath.Join("Default", "Cookies"),
		"Local State",
	} {
		if _, err := os.Stat(filepath.Join(profile, rel)); err == nil {
			return true
		}
	}
	return false
}

func persistAccountChromeProfile(accountID, source string) error {
	if accountID == "" || source == "" {
		return nil
	}
	target := accountChromeProfilePath(accountID)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return copyChromeProfile(source, target)
}

func removeAccountChromeProfile(accountID string) error {
	if accountID == "" {
		return nil
	}
	return os.RemoveAll(accountChromeProfilePath(accountID))
}

func CleanupTransientChromeProfiles() {
	for _, name := range []string{"chrome-api-profiles", "chrome-profiles"} {
		path := filepath.Join(Cfg.DataDir, name)
		if err := os.RemoveAll(path); err != nil {
			LogError("failed to clean transient Kling chrome profile dir %s: %v", path, err)
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			LogError("failed to recreate transient Kling chrome profile dir %s: %v", path, err)
		}
	}
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
