// Package shared provides cross-service helpers usable from any service
// or handler. db_errors.go centralizes detection of GORM/driver errors
// so callers do not duplicate fragile substring matches on the raw
// Postgres message.
package shared

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
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

// userEmailUniqueConstraints are the indexes that make an address unique: the
// functional index the identity rule is built on, and the byte-exact index the
// column's UNIQUE constraint creates.
var userEmailUniqueConstraints = map[string]bool{
	"users_lower_email_uniq": true,
	"users_email_key":        true,
}

// UserExistsIfDuplicate returns the canonical USER_EXISTS refusal when err is a
// unique violation against one of the users-table ADDRESS indexes, and nil for
// anything else, so callers keep their own wrapping for a genuine failure.
//
// A violation of an address index means the caller's own GetUserByEmail
// pre-check lost a race with a concurrent signup for the same identity. The
// index, not the pre-check, is the authority. Whether that reaches the client
// as USER_EXISTS is the CALLER's decision: password registration renders it,
// while the passkey, Apple and goth callbacks collapse every create failure
// into their own generic shape.
//
// It matches on the constraint name rather than on "some unique index was
// violated" because users.username is unique too, and rendering a username
// collision as "an account with this email already exists" would be a wrong
// answer on an auth surface. No signup path populates username today, which is
// why the guard belongs here rather than in a comment asserting it never will.
func UserExistsIfDuplicate(email string, err error) error {
	if !IsDuplicateKey(err) {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && !userEmailUniqueConstraints[pgErr.ConstraintName] {
		return nil
	}
	return apperrors.ErrUserExists(email)
}

// IsCheckConstraintViolation reports whether err is a Postgres check-constraint
// violation (SQLSTATE 23514).
//
// Like IsDuplicateKey this keys on a translated GORM sentinel rather than the
// driver message, which names the constraint and is not part of any contract a
// caller should read or repeat back to a client.
//
// The caller it exists for is a write that cannot pre-validate what it is
// writing: the revision rollback restores a stored OldValue that no forward gate
// ever saw, so the column is where an out-of-range value is caught.
func IsCheckConstraintViolation(err error) bool {
	return errors.Is(err, gorm.ErrCheckConstraintViolated)
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
