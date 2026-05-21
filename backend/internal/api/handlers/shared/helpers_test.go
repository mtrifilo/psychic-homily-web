package shared

import (
	"testing"

	authm "psychic-homily-backend/internal/models/auth"
)

func TestGetUserID_NilReturnsZero(t *testing.T) {
	if got := GetUserID(nil); got != 0 {
		t.Errorf("GetUserID(nil) = %d, want 0", got)
	}
}

func TestGetUserID_ReturnsUserID(t *testing.T) {
	user := &authm.User{}
	user.ID = 42
	if got := GetUserID(user); got != 42 {
		t.Errorf("GetUserID(user{ID:42}) = %d, want 42", got)
	}
}

func TestParseDate_Valid(t *testing.T) {
	got, err := ParseDate("2026-05-20")
	if err != nil {
		t.Fatalf("ParseDate(\"2026-05-20\") error = %v, want nil", err)
	}
	if got.Year() != 2026 || got.Month() != 5 || got.Day() != 20 {
		t.Errorf("ParseDate(\"2026-05-20\") = %v, want 2026-05-20", got)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	// Wrong format (slashes) and a non-date string both must error.
	for _, in := range []string{"05/20/2026", "not-a-date", "2026-13-40", ""} {
		if _, err := ParseDate(in); err == nil {
			t.Errorf("ParseDate(%q) error = nil, want non-nil", in)
		}
	}
}
