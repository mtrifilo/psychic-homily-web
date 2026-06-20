// Package shared provides cross-service helpers usable from any service
// or handler. db_errors.go centralizes detection of GORM/driver errors
// so callers do not duplicate fragile substring matches on the raw
// Postgres message.
package shared

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// pgSerializationFailure is the Postgres SQLSTATE for a serialization failure —
// a concurrent-transaction conflict that is safe to retry. Unlike 23505
// (duplicate key), GORM's postgres driver does NOT translate this code into a
// sentinel: its errCodes map only special-cases 23505 (unique) / 23503 (fk) /
// 42703 (invalid field) / 23514 (check), so the original *pgconn.PgError
// survives TranslateError untouched and errors.As can recover it.
const pgSerializationFailure = "40001"

// pgDeadlockDetected is the Postgres SQLSTATE for a detected deadlock — the other
// half of the canonical "safe to retry after a transient conflict" pair (with
// 40001). Like 40001 it is absent from GORM's TranslateError code map, so the
// original *pgconn.PgError survives untouched.
const pgDeadlockDetected = "40P01"

// IsDuplicateKey reports whether err is a GORM duplicate-key (unique
// constraint) violation.
//
// Requires gorm.Config.TranslateError = true so the underlying
// pgconn.PgError (SQLSTATE 23505) is translated to gorm.ErrDuplicatedKey
// before it reaches callers. db.Connect and the testcontainer setup both
// enable that option.
//
// Use this instead of strings.Contains(err.Error(), "duplicate key"). The
// driver message ("duplicate key value violates unique constraint ...") is
// not part of any public contract and can change between postgres/pgx
// versions.
func IsDuplicateKey(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// IsSerializationFailure reports whether err is a Postgres serialization_failure
// (SQLSTATE 40001) — a transient concurrency conflict that is safe to retry.
//
// Unlike IsDuplicateKey (which keys on a translated gorm sentinel), this keys on
// the typed *pgconn.PgError directly, because 40001 is NOT in GORM's
// TranslateError code map (see pgSerializationFailure) — so it reaches callers
// unchanged even with gorm.Config.TranslateError = true. errors.As unwraps any
// fmt.Errorf-wrapped chain. A nil or non-pg error returns false.
func IsSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgSerializationFailure
	}
	return false
}

// IsDeadlock reports whether err is a Postgres deadlock_detected (SQLSTATE 40P01)
// — a transient conflict that is safe to retry. Unlike a serialization failure
// (which only arises at REPEATABLE READ / SERIALIZABLE), a deadlock can occur at
// any isolation level including READ COMMITTED, so a retry-on-conflict guard that
// omits it is incomplete. Keys on the typed *pgconn.PgError directly (40P01 is
// not in GORM's TranslateError map either); see IsSerializationFailure for the
// TranslateError rationale.
func IsDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgDeadlockDetected
	}
	return false
}
