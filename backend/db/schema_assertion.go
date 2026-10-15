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
//
// Type, when set, is the column's required type in either spelling Postgres
// reports (information_schema data_type or udt_name, so "bigint" and "int8"
// are equivalent); the entries use the data_type spelling, which is also the
// one the mismatch message prints. An empty Type checks existence only. Set
// it when a migration changes a column's type, so a recorded-but-absent type
// change fails boot too.
type requiredColumn struct {
	Table  string
	Column string
	Type   string
}

const (
	sqlBigint      = "bigint"
	sqlTimestamptz = "timestamp with time zone"
)

func (c requiredColumn) qualifiedName() string {
	return c.Table + "." + c.Column
}

// requiredSchemaColumns is the boot-time checklist. Keep it small: only
// columns whose absence breaks core request paths before any health probe
// would notice, and typed columns whose old type fails or silently alters
// the writes the code makes.
var requiredSchemaColumns = []requiredColumn{
	{Table: "user_bookmarks", Column: "scene_digest_sent_at"},
	{Table: "user_preferences", Column: "notify_on_scene_digest"},
	// PSY-1907: every follow's alert subscription resolves against this column,
	// so recorded-migration-but-absent-DDL fails the Library following page and
	// both follow-alert endpoints at once rather than degrading.
	{Table: "user_preferences", Column: "alert_defaults"},
	// Same table, same reason: GORM enumerates every model column, so an
	// absent home_layout DDL fails EVERY preferences write at request time,
	// not just the home-layout one.
	{Table: "user_preferences", Column: "home_layout"},
	// Same table, same reason as home_layout.
	{Table: "user_preferences", Column: "first_save_hint_dismissed_at"},
	// GORM's Create builds an explicit column list from the model, so these
	// three appear in every show INSERT. Absent DDL breaks show submission at
	// request time rather than at boot, which is exactly what this list is for.
	{Table: "shows", Column: "doors_at"},
	{Table: "shows", Column: "music_at"},
	{Table: "shows", Column: "door_price"},
	// GORM's Create enumerates every model column, so these three appear in
	// every tag INSERT, including the one an entity page's inline tag-add runs.
	// Absent DDL fails that write at request time, which is what this list
	// exists to turn into a boot refusal.
	{Table: "tags", Column: "website"},
	{Table: "tags", Column: "instagram"},
	{Table: "tags", Column: "bandcamp"},
	// PSY-1761: shared.VenueTZJoin reads this table on EVERY show-listing
	// query, so recorded-migration-but-absent-DDL here does not degrade, it
	// fails every listing surface at once with "relation does not exist" —
	// a worse outage than the unknown-zone raise the guard exists to remove.
	// Checked by a generated column so the assertion also catches a table
	// that exists but predates the shape the guard queries.
	{Table: "timezone_names_snapshot", Column: "name_lower"},
	// The model stores the authenticator's signature counter as a uint32; an
	// INTEGER column cannot hold the upper half of that range, so the
	// credential update after such a sign-in fails.
	{Table: "webauthn_credentials", Column: "sign_count", Type: sqlBigint},
	// The audit models type both ids as uint.
	{Table: "audit_logs", Column: "actor_id", Type: sqlBigint},
	{Table: "audit_logs", Column: "entity_id", Type: sqlBigint},
	{Table: "entity_edit_audit_logs", Column: "actor_id", Type: sqlBigint},
	{Table: "entity_edit_audit_logs", Column: "entity_id", Type: sqlBigint},
	// A TIMESTAMP (without time zone) column discards the zone of the instant
	// the code writes, and no error says so.
	{Table: "shows", Column: "event_date", Type: sqlTimestamptz},
	{Table: "shows", Column: "created_at", Type: sqlTimestamptz},
	{Table: "shows", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "artists", Column: "created_at", Type: sqlTimestamptz},
	{Table: "artists", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "venues", Column: "created_at", Type: sqlTimestamptz},
	{Table: "venues", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "users", Column: "created_at", Type: sqlTimestamptz},
	{Table: "users", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "oauth_accounts", Column: "expires_at", Type: sqlTimestamptz},
	{Table: "oauth_accounts", Column: "created_at", Type: sqlTimestamptz},
	{Table: "oauth_accounts", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "user_preferences", Column: "created_at", Type: sqlTimestamptz},
	{Table: "user_preferences", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "show_reports", Column: "reviewed_at", Type: sqlTimestamptz},
	{Table: "show_reports", Column: "created_at", Type: sqlTimestamptz},
	{Table: "show_reports", Column: "updated_at", Type: sqlTimestamptz},
	{Table: "audit_logs", Column: "created_at", Type: sqlTimestamptz},
	{Table: "entity_edit_audit_logs", Column: "created_at", Type: sqlTimestamptz},
}

type columnChecker interface {
	HasColumn(table interface{}, column string) bool
}

// assertRequiredSchema checks existence for the untyped entries only; the
// type read in assertRequiredColumnTypes reports a typed entry that is absent.
func assertRequiredSchema(checker columnChecker) error {
	var missing []string
	for _, col := range requiredSchemaColumns {
		if col.Type != "" {
			continue
		}
		if !checker.HasColumn(col.Table, col.Column) {
			missing = append(missing, col.qualifiedName())
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

// columnType is a column's type as information_schema.columns reports it.
type columnType struct {
	DataType string // SQL-standard name, e.g. "bigint", "timestamp with time zone"
	UDTName  string // Postgres internal name, e.g. "int8", "timestamptz"
}

func (c columnType) matches(want string) bool {
	return strings.EqualFold(want, c.DataType) || strings.EqualFold(want, c.UDTName)
}

// columnTypeReader returns the type of every column of the named tables,
// keyed by "table.column".
type columnTypeReader interface {
	ColumnTypes(tables []string) (map[string]columnType, error)
}

// assertRequiredColumnTypes checks only the entries that declare a Type; no
// other column's type is compared.
func assertRequiredColumnTypes(reader columnTypeReader) error {
	var typed []requiredColumn
	var tables []string
	seenTable := map[string]bool{}
	for _, col := range requiredSchemaColumns {
		if col.Type == "" {
			continue
		}
		typed = append(typed, col)
		if !seenTable[col.Table] {
			seenTable[col.Table] = true
			tables = append(tables, col.Table)
		}
	}
	if len(typed) == 0 {
		return nil
	}

	actual, err := reader.ColumnTypes(tables)
	if err != nil {
		return fmt.Errorf("could not read required column types: %w; refusing to boot", err)
	}

	var wrong []string
	for _, col := range typed {
		got, ok := actual[col.qualifiedName()]
		switch {
		case !ok:
			wrong = append(wrong, fmt.Sprintf("%s is absent, expected %s", col.qualifiedName(), col.Type))
		case !got.matches(col.Type):
			wrong = append(wrong, fmt.Sprintf("%s is %s, expected %s", col.qualifiedName(), got.DataType, col.Type))
		}
	}
	if len(wrong) > 0 {
		return fmt.Errorf(
			"required schema column types do not match: %s (type change recorded but DDL absent?); refusing to boot",
			strings.Join(wrong, "; "),
		)
	}
	return nil
}

// informationSchemaColumnTypes reads column types from the connection's
// current schema, the schema GORM's Migrator().HasColumn consults, in one
// query. Migrator().ColumnTypes is not used: it runs several queries per table
// and reads a data row from each.
type informationSchemaColumnTypes struct {
	db *gorm.DB
}

func (r informationSchemaColumnTypes) ColumnTypes(tables []string) (map[string]columnType, error) {
	var rows []struct {
		TableName  string
		ColumnName string
		DataType   string
		UdtName    string
	}
	err := r.db.Raw(
		`SELECT table_name, column_name, data_type, udt_name
		   FROM information_schema.columns
		  WHERE table_schema = current_schema() AND table_name IN ?`,
		tables,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	types := make(map[string]columnType, len(rows))
	for _, row := range rows {
		key := requiredColumn{Table: row.TableName, Column: row.ColumnName}.qualifiedName()
		types[key] = columnType{DataType: row.DataType, UDTName: row.UdtName}
	}
	return types, nil
}

// AssertRequiredSchema verifies that critical columns exist, and have their
// required type where one is listed, before the server accepts traffic. Call
// after db.Connect in cmd/server.
func AssertRequiredSchema(gormDB *gorm.DB) error {
	if gormDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	if err := assertRequiredSchema(gormDB.Migrator()); err != nil {
		return err
	}
	if err := assertRequiredColumnTypes(informationSchemaColumnTypes{db: gormDB}); err != nil {
		return err
	}
	return assertTimezoneSnapshotPopulated(gormDB)
}

// assertTimezoneSnapshotPopulated refuses to boot against an EMPTY
// timezone_names_snapshot (PSY-1761).
//
// This is the only column in this file's remit whose emptiness matters as much
// as its absence, and it is checked here because it is the one failure mode
// that produces no error anywhere. shared.VenueTZJoin tests each venue's stored
// zone for membership in this table; against an empty one every test fails, so
// every venue silently falls back to the US state map and every non-US venue to
// UTC — up to fourteen hours off, enough to move a show to the wrong calendar
// day, with no exception raised and nothing logged. The drift detector cannot
// see it either: it compares venues against the LIVE catalog, which still
// agrees with them.
//
// An out-of-band TRUNCATE or a selective restore is what would produce it (a
// stage-to-production data refresh restores table data, and the refresher's own
// emptiness guard only covers the catalog read returning nothing). Failing the
// boot turns a silent, sitewide, wrong-date condition into an obvious one that
// `migrate up` on the next deploy repairs.
func assertTimezoneSnapshotPopulated(gormDB *gorm.DB) error {
	var zones int64
	if err := gormDB.Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&zones).Error; err != nil {
		return fmt.Errorf("could not read timezone_names_snapshot: %w; refusing to boot", err)
	}
	if zones == 0 {
		return fmt.Errorf(
			"timezone_names_snapshot is empty; every venue timezone would fail the venue-local " +
				"zone guard and every show would be dated by the state map instead. Re-seed it " +
				"(the create migration's INSERT ... SELECT FROM pg_timezone_names); refusing to boot")
	}
	return nil
}
