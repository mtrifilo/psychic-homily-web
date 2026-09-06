package shared

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
)

// TestUserExistsIfDuplicate pins both halves of the mapping: a unique
// violation on a users insert becomes the same USER_EXISTS refusal the serial
// pre-check produces, and every other error passes through untouched so a
// genuine backend failure still reads as one.
func TestUserExistsIfDuplicate(t *testing.T) {
	t.Run("duplicate becomes USER_EXISTS", func(t *testing.T) {
		wrapped := fmt.Errorf("insert users: %w", gorm.ErrDuplicatedKey)

		got := UserExistsIfDuplicate("Sym.Case@Example.com", wrapped)

		var authErr *apperrors.AuthError
		if !errors.As(got, &authErr) {
			t.Fatalf("expected an *apperrors.AuthError, got %T: %v", got, got)
		}
		if authErr.Code != apperrors.CodeUserExists {
			t.Errorf("code = %q, want %q", authErr.Code, apperrors.CodeUserExists)
		}
	})

	t.Run("other errors yield nil so the caller keeps its own wrapping", func(t *testing.T) {
		for _, err := range []error{
			nil,
			errors.New("connection refused"),
			fmt.Errorf("insert users: %w", gorm.ErrInvalidField),
		} {
			if got := UserExistsIfDuplicate("user@example.com", err); got != nil {
				t.Errorf("UserExistsIfDuplicate(%v) = %v, want nil", err, got)
			}
		}
	})
}
