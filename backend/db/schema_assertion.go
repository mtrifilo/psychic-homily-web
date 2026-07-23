package db

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// requiredColumn names a table column the running server depends on.
// Add entries here when a migration introduces columns that hot code paths
// assume exist — drift between schema_migrations and actual DDL then fails
// boot instead of surfacing as silent 422s at request time (PSY-1384).
type requiredColumn struct {
	Table  string
	Column string
}

// requiredSchemaColumns is the boot-time checklist. Keep it small: only
// columns whose absence breaks core request paths before any health probe
// would notice.
var requiredSchemaColumns = []requiredColumn{
	{Table: "user_bookmarks", Column: "scene_digest_sent_at"},
	{Table: "user_preferences", Column: "notify_on_scene_digest"},
}

type columnChecker interface {
	HasColumn(table interface{}, column string) bool
}

func assertRequiredSchema(checker columnChecker) error {
	var missing []string
	for _, col := range requiredSchemaColumns {
		if !checker.HasColumn(col.Table, col.Column) {
			missing = append(missing, col.Table+"."+col.Column)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"required schema columns missing: %s (migration recorded but DDL absent?); refusing to boot",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

// AssertRequiredSchema verifies that critical columns exist before the server
// accepts traffic. Call after db.Connect in cmd/server.
func AssertRequiredSchema(gormDB *gorm.DB) error {
	if gormDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	return assertRequiredSchema(gormDB.Migrator())
}
