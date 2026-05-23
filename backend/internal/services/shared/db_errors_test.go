package shared

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"
)

func TestIsDuplicateKey(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("connection refused"), false},
		// Pre-TranslateError driver message must NOT be treated as a hit —
		// the helper is intentionally typed-only so callers that forget to
		// enable TranslateError get a loud false-negative rather than a
		// silent substring match.
		{"raw driver string", errors.New("duplicate key value violates unique constraint \"users_username_key\""), false},
		{"gorm sentinel direct", gorm.ErrDuplicatedKey, true},
		{"gorm sentinel wrapped", fmt.Errorf("create failed: %w", gorm.ErrDuplicatedKey), true},
		{"unrelated gorm error", gorm.ErrRecordNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDuplicateKey(tc.err); got != tc.want {
				t.Errorf("IsDuplicateKey(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
