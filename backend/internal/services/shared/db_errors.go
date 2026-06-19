// Package shared provides cross-service helpers usable from any service
// or handler. db_errors.go centralizes detection of GORM/driver errors
// so callers do not duplicate fragile substring matches on the raw
// Postgres message.
package shared

import (
	"errors"

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
