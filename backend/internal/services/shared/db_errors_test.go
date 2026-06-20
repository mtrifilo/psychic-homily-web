package shared

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
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

func TestIsSerializationFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("connection refused"), false},
		// Typed-only, like IsDuplicateKey: a raw driver message must NOT match, so
		// a caller relying on substring matching gets a loud false rather than a
		// brittle hit.
		{"raw driver string", errors.New("could not serialize access due to concurrent update"), false},
		{"pg 40001 direct", &pgconn.PgError{Code: "40001"}, true},
		{"pg 40001 wrapped", fmt.Errorf("batch inserting plays: %w", &pgconn.PgError{Code: "40001"}), true},
		{"pg 40P01 deadlock", &pgconn.PgError{Code: "40P01"}, false},
		{"pg 23505 duplicate", &pgconn.PgError{Code: "23505"}, false},
		{"gorm dup sentinel", gorm.ErrDuplicatedKey, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSerializationFailure(tc.err); got != tc.want {
				t.Errorf("IsSerializationFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsDeadlock(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("connection refused"), false},
		{"raw driver string", errors.New("deadlock detected"), false},
		{"pg 40P01 direct", &pgconn.PgError{Code: "40P01"}, true},
		{"pg 40P01 wrapped", fmt.Errorf("batch inserting plays: %w", &pgconn.PgError{Code: "40P01"}), true},
		{"pg 40001 serialization", &pgconn.PgError{Code: "40001"}, false},
		{"pg 23505 duplicate", &pgconn.PgError{Code: "23505"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDeadlock(tc.err); got != tc.want {
				t.Errorf("IsDeadlock(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsSerializationFailure_SurvivesTranslateError is the explicit guard that the
// retry-on-serialization path (PSY-1143) depends on: a 40001 must survive GORM's
// TranslateError as a *pgconn.PgError (pattern_gorm_translate_error.md), unlike
// 23505 which is replaced by the bare gorm.ErrDuplicatedKey sentinel. Asserted
// against the REAL postgres dialector rather than a reimplementation of its map.
func TestIsSerializationFailure_SurvivesTranslateError(t *testing.T) {
	dialector := postgres.Dialector{}

	// 40001 is not in the driver's errCodes map → passes through untranslated.
	serErr := dialector.Translate(&pgconn.PgError{Code: "40001"})
	if !IsSerializationFailure(serErr) {
		t.Errorf("40001 must survive TranslateError and remain detectable; got %T: %v", serErr, serErr)
	}

	// 40P01 likewise is not translated → survives as a *pgconn.PgError.
	deadlockErr := dialector.Translate(&pgconn.PgError{Code: "40P01"})
	if !IsDeadlock(deadlockErr) {
		t.Errorf("40P01 must survive TranslateError and remain detectable; got %T: %v", deadlockErr, deadlockErr)
	}

	// Control: 23505 IS translated to the sentinel, so it is a duplicate-key error,
	// NOT a serialization failure. Proves the two helpers don't cross-classify.
	dupErr := dialector.Translate(&pgconn.PgError{Code: "23505"})
	if IsSerializationFailure(dupErr) {
		t.Errorf("23505 must not be classified as a serialization failure; got %T: %v", dupErr, dupErr)
	}
	if !IsDuplicateKey(dupErr) {
		t.Errorf("23505 must translate to the duplicate-key sentinel; got %T: %v", dupErr, dupErr)
	}
}
