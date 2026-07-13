package internal

import "testing"

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
