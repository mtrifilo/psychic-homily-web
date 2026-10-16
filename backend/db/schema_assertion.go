package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
// one the mismatch message prints. An empty Type checks existence only. Which
// columns carry a Type is stated above the typed entries in
// requiredSchemaColumns.
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
// would notice, plus the typed entries described below.
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
	// GORM's Create enumerates every model column, so this one appears in
	// every radio play INSERT the playlist import runs. Absent DDL fails every
	// import at request time instead of at boot.
	{Table: "radio_plays", Column: "release_link_source"},
	// Typed entries: exactly the columns whose type a timestamp-named
	// migration changed. golang-migrate skips a timestamp-named migration
	// whose version is below the recorded pointer, which happens when branches
	// merge out of timestamp order, so the pointer does not prove its DDL ran.
	// A sequentially numbered migration cannot be skipped that way, so the
	// pointer alone proves its type changes and they are not typed here.
	//
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
	{Table: "entity_edit_audit_logs", Column: "created_at", Type: sqlTimestamptz},
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
// keyed by "table.column". It stops waiting when ctx ends.
type columnTypeReader interface {
	ColumnTypes(ctx context.Context, tables []string) (map[string]columnType, error)
}

// assertRequiredColumns reads every table the checklist names once, within
// bound, and runs both checks on that read, so a failed read refuses boot as
// a read error rather than as columns reported missing.
func assertRequiredColumns(reader columnTypeReader, bound time.Duration) error {
	var tables []string
	seenTable := map[string]bool{}
	for _, col := range requiredSchemaColumns {
		if !seenTable[col.Table] {
			seenTable[col.Table] = true
			tables = append(tables, col.Table)
		}
	}

	actual, err := readWithin(bound, func(ctx context.Context) (map[string]columnType, error) {
		return reader.ColumnTypes(ctx, tables)
	})
	if err != nil {
		return fmt.Errorf("could not read required schema: %w; refusing to boot", err)
	}
	return schemaRefusal(absentUntypedColumns(actual), typedColumnFaults(actual))
}

// absentUntypedColumns checks existence for the untyped entries only, and
// returns the ones the read did not return. typedColumnFaults reports a typed
// entry that is absent.
func absentUntypedColumns(actual map[string]columnType) []string {
	var absent []string
	for _, col := range requiredSchemaColumns {
		if col.Type != "" {
			continue
		}
		if _, ok := actual[col.qualifiedName()]; !ok {
			absent = append(absent, col.qualifiedName())
		}
	}
	return absent
}

// columnTypeFault is a typed entry the read did not return (absent) or
// returned with another type (gotType, its data_type spelling).
type columnTypeFault struct {
	column  requiredColumn
	absent  bool
	gotType string
}

// typedColumnFaults checks only the entries that declare a Type, in checklist
// order; no other column's type is compared.
func typedColumnFaults(actual map[string]columnType) []columnTypeFault {
	var faults []columnTypeFault
	for _, col := range requiredSchemaColumns {
		if col.Type == "" {
			continue
		}
		got, ok := actual[col.qualifiedName()]
		switch {
		case !ok:
			faults = append(faults, columnTypeFault{column: col, absent: true})
		case !got.matches(col.Type):
			faults = append(faults, columnTypeFault{column: col, gotType: got.DataType})
		}
	}
	return faults
}

// schemaRefusal is nil when neither check found a fault. Faults from one
// check alone get that check's refusal. Faults from both get one refusal
// naming every absent column, typed or not, and every mistyped one, so a
// single boot reports all of them; when every typed fault is an absence that
// refusal is the missing-columns one.
func schemaRefusal(absentUntyped []string, typedFaults []columnTypeFault) error {
	switch {
	case len(absentUntyped) == 0 && len(typedFaults) == 0:
		return nil
	case len(typedFaults) == 0:
		return missingColumnsRefusal(absentUntyped)
	case len(absentUntyped) == 0:
		return columnTypesRefusal(typedFaults)
	}

	missing := absentUntyped
	var mistyped []string
	for _, f := range typedFaults {
		name := f.column.qualifiedName()
		if f.absent {
			missing = append(missing, name)
		} else {
			mistyped = append(mistyped, fmt.Sprintf("%s (want %s, got %s)", name, f.column.Type, f.gotType))
		}
	}
	if len(mistyped) == 0 {
		return missingColumnsRefusal(missing)
	}
	return fmt.Errorf(
		"required schema columns missing or mistyped: missing: %s; mistyped: %s (migration recorded but DDL absent?); refusing to boot",
		strings.Join(missing, ", "),
		strings.Join(mistyped, ", "),
	)
}

func missingColumnsRefusal(missing []string) error {
	return fmt.Errorf(
		"required schema columns missing: %s (migration recorded but DDL absent?); refusing to boot",
		strings.Join(missing, ", "),
	)
}

func columnTypesRefusal(faults []columnTypeFault) error {
	wrong := make([]string, 0, len(faults))
	for _, f := range faults {
		name := f.column.qualifiedName()
		if f.absent {
			wrong = append(wrong, fmt.Sprintf("%s is absent, expected %s", name, f.column.Type))
		} else {
			wrong = append(wrong, fmt.Sprintf("%s is %s, expected %s", name, f.gotType, f.column.Type))
		}
	}
	return fmt.Errorf(
		"required schema column types do not match: %s (type change recorded but DDL absent?); refusing to boot",
		strings.Join(wrong, "; "),
	)
}

// informationSchemaColumnTypes reads column types from the connection's
// current schema in one query and returns the query's error. GORM's
// Migrator().HasColumn is not used: it reports a failed query as an absent
// column. Migrator().ColumnTypes is not used either: it runs several queries
// per table and reads a data row from each.
type informationSchemaColumnTypes struct {
	db *gorm.DB
}

func (r informationSchemaColumnTypes) ColumnTypes(ctx context.Context, tables []string) (map[string]columnType, error) {
	var rows []struct {
		TableName  string
		ColumnName string
		DataType   string
		UdtName    string
	}
	err := r.db.WithContext(ctx).Raw(
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

// bootSchemaReadTimeout bounds each boot-time catalog read: the schema
// assertion's two reads and FlagInvalidIndexes' one. A healthy read answers in
// milliseconds, so a read still waiting at the bound is on a connection that
// is not answering, and boot reports it instead of waiting with it.
const bootSchemaReadTimeout = 10 * time.Second

// readWithin runs read with a context that ends after bound. When a failed
// read ran past the bound, the error says so and names the bound.
func readWithin[T any](bound time.Duration, read func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	result, err := read(ctx)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, fmt.Errorf("no answer within %s: %w", bound, err)
	}
	return result, err
}

// AssertRequiredSchema verifies that critical columns exist, and have their
// required type where one is listed, before the server accepts traffic. Each
// read is bounded by bootSchemaReadTimeout, and a read that does not answer
// within it refuses boot. Call after db.Connect in cmd/server.
func AssertRequiredSchema(gormDB *gorm.DB) error {
	return assertRequiredSchemaWithin(gormDB, bootSchemaReadTimeout)
}

func assertRequiredSchemaWithin(gormDB *gorm.DB, bound time.Duration) error {
	if gormDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	if err := assertRequiredColumns(informationSchemaColumnTypes{db: gormDB}, bound); err != nil {
		return err
	}
	return assertTimezoneSnapshotPopulated(gormDB, bound)
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
func assertTimezoneSnapshotPopulated(gormDB *gorm.DB, bound time.Duration) error {
	zones, err := readWithin(bound, func(ctx context.Context) (int64, error) {
		var zones int64
		err := gormDB.WithContext(ctx).Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&zones).Error
		return zones, err
	})
	if err != nil {
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
