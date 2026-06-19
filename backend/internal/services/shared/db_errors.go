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

// DuplicateKeyConstraint returns the name of the unique constraint or index a
// duplicate-key error violated, or "" if err is not a recognizable duplicate-key
// violation. Lets a caller distinguish WHICH unique key collided (e.g. a name
// index vs a slug index) so it can return a precise, non-misleading error rather
// than attributing every unique violation to one constraint. Relies on the
// underlying *pgconn.PgError surviving in the error chain (it does — gorm's
// TranslateError wraps, it does not replace).
func DuplicateKeyConstraint(err error) string {
	if !IsDuplicateKey(err) {
		return ""
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}
