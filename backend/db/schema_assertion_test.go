package db

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/testutil"
)

// allRequiredColumnKeys renders the production checklist in the "table.column"
// form the column read keys on.
func allRequiredColumnKeys() []string {
	keys := make([]string, 0, len(requiredSchemaColumns))
	for _, col := range requiredSchemaColumns {
		keys = append(keys, col.qualifiedName())
	}
	return keys
}

func TestAbsentUntypedColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		present    []string
		wantAbsent []string
	}{
		{
			// Derived from the production list rather than restated, so adding
			// a required column cannot silently break only this case.
			name:    "all required columns present",
			present: allRequiredColumnKeys(),
		},
		{
			name: "missing show time columns",
			present: []string{
				"user_bookmarks.scene_digest_sent_at",
				"user_preferences.notify_on_scene_digest",
			},
			wantAbsent: []string{"shows.doors_at", "shows.music_at"},
		},
		{
			name: "missing scene digest bookmark column",
			present: []string{
				"user_preferences.notify_on_scene_digest",
			},
			wantAbsent: []string{"user_bookmarks.scene_digest_sent_at"},
		},
		{
			name: "missing scene digest preference column",
			present: []string{
				"user_bookmarks.scene_digest_sent_at",
			},
			wantAbsent: []string{"user_preferences.notify_on_scene_digest"},
		},
		{
			name:       "missing all required columns",
			present:    nil,
			wantAbsent: []string{"user_bookmarks.scene_digest_sent_at", "user_preferences.notify_on_scene_digest"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			present := make(map[string]columnType, len(tc.present))
			for _, key := range tc.present {
				present[key] = columnType{}
			}

			absent := absentUntypedColumns(present)
			if tc.wantAbsent == nil {
				assert.Empty(t, absent)
				return
			}
			assert.Subset(t, absent, tc.wantAbsent)
			for _, key := range tc.present {
				assert.NotContains(t, absent, key)
			}
		})
	}

	t.Run("a typed entry is not checked for existence", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, absentUntypedColumns(passingColumnTypesWithout("webauthn_credentials.sign_count")))
	})
}

// A database that cannot be reached must refuse boot naming the failed read,
// not report the required columns as missing.
func TestAssertRequiredSchema_unreachableDatabase(t *testing.T) {
	t.Parallel()

	err := AssertRequiredSchema(testutil.UnavailableDB(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read required schema: ")
	assert.Contains(t, err.Error(), "refusing to boot")
	assert.NotContains(t, err.Error(), "missing")
}

func TestAssertRequiredSchema_nilDB(t *testing.T) {
	t.Parallel()

	err := AssertRequiredSchema(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection is nil")
}

type fakeColumnTypeReader struct {
	types map[string]columnType
	err   error
	reads [][]string
}

func (f *fakeColumnTypeReader) ColumnTypes(_ context.Context, tables []string) (map[string]columnType, error) {
	f.reads = append(f.reads, tables)
	return f.types, f.err
}

// listedColumnTypes renders every typed entry of the production checklist
// with its required type, keyed the way the reader keys them.
func listedColumnTypes() map[string]columnType {
	types := map[string]columnType{}
	for _, col := range requiredSchemaColumns {
		if col.Type != "" {
			types[col.qualifiedName()] = columnType{DataType: col.Type}
		}
	}
	return types
}

// passingColumnTypes renders every entry of the production checklist, the
// typed ones with their required type, so both checks pass on it.
func passingColumnTypes() map[string]columnType {
	types := listedColumnTypes()
	for _, key := range allRequiredColumnKeys() {
		if _, ok := types[key]; !ok {
			types[key] = columnType{}
		}
	}
	return types
}

// passingColumnTypesWithout is passingColumnTypes less the named columns.
func passingColumnTypesWithout(keys ...string) map[string]columnType {
	types := passingColumnTypes()
	for _, key := range keys {
		delete(types, key)
	}
	return types
}

// The column-types refusal, as a schema whose every untyped column exists gets
// it.
func TestColumnTypesRefusal(t *testing.T) {
	t.Parallel()

	t.Run("every listed column has its type", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, schemaRefusal(nil, typedColumnFaults(listedColumnTypes())))
	})

	t.Run("a listed column with another type fails with the column, actual and expected type", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.sign_count"] = columnType{DataType: "integer", UDTName: "int4"}
		err := schemaRefusal(nil, typedColumnFaults(types))
		require.EqualError(t, err,
			"required schema column types do not match: webauthn_credentials.sign_count is integer, "+
				"expected bigint (type change recorded but DDL absent?); refusing to boot")
	})

	t.Run("every wrong column is reported", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.sign_count"] = columnType{DataType: "integer", UDTName: "int4"}
		types["entity_edit_audit_logs.created_at"] = columnType{DataType: "timestamp without time zone", UDTName: "timestamp"}
		err := schemaRefusal(nil, typedColumnFaults(types))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webauthn_credentials.sign_count is integer, expected bigint")
		assert.Contains(t, err.Error(),
			"entity_edit_audit_logs.created_at is timestamp without time zone, expected timestamp with time zone")
	})

	t.Run("a listed column the reader did not return is reported absent", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		delete(types, "webauthn_credentials.sign_count")
		err := schemaRefusal(nil, typedColumnFaults(types))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webauthn_credentials.sign_count is absent, expected bigint")
		assert.Contains(t, err.Error(), "refusing to boot")
	})

	t.Run("an unlisted column's type is ignored", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.display_name"] = columnType{DataType: "integer", UDTName: "int4"}
		types["user_bookmarks.scene_digest_sent_at"] = columnType{DataType: "text", UDTName: "text"}
		require.NoError(t, schemaRefusal(nil, typedColumnFaults(types)))
	})
}

// unansweredColumnTypeReader never returns a result. Like the database driver,
// it stops waiting when its context ends.
type unansweredColumnTypeReader struct{}

func (unansweredColumnTypeReader) ColumnTypes(ctx context.Context, _ []string) (map[string]columnType, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAssertRequiredColumns(t *testing.T) {
	t.Parallel()

	t.Run("one read covers every listed table", func(t *testing.T) {
		t.Parallel()
		reader := &fakeColumnTypeReader{types: passingColumnTypes()}
		require.NoError(t, assertRequiredColumns(reader, bootSchemaReadTimeout))

		require.Len(t, reader.reads, 1)
		for _, col := range requiredSchemaColumns {
			assert.Contains(t, reader.reads[0], col.Table)
		}
	})

	// The existence check and the type check share the read, so its failure
	// has to surface as itself rather than as either check's verdict on an
	// empty result.
	t.Run("a read error refuses boot naming the read, not a missing or mistyped column", func(t *testing.T) {
		t.Parallel()
		err := assertRequiredColumns(&fakeColumnTypeReader{err: errors.New("connection refused")}, bootSchemaReadTimeout)
		require.EqualError(t, err, "could not read required schema: connection refused; refusing to boot")
	})

	t.Run("a read that does not answer refuses boot within the bound, naming it", func(t *testing.T) {
		t.Parallel()
		started := time.Now()
		err := assertRequiredColumns(unansweredColumnTypeReader{}, 50*time.Millisecond)
		assert.Less(t, time.Since(started), 2*time.Second)
		require.EqualError(t, err,
			"could not read required schema: no answer within 50ms: context deadline exceeded; refusing to boot")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("a missing untyped column alone gets the missing-columns refusal", func(t *testing.T) {
		t.Parallel()
		err := assertRequiredColumns(
			&fakeColumnTypeReader{types: passingColumnTypesWithout("shows.doors_at", "tags.website")}, bootSchemaReadTimeout)
		require.EqualError(t, err,
			"required schema columns missing: shows.doors_at, tags.website "+
				"(migration recorded but DDL absent?); refusing to boot")
	})

	t.Run("a mistyped column alone gets the column-types refusal", func(t *testing.T) {
		t.Parallel()
		types := passingColumnTypes()
		types["audit_logs.actor_id"] = columnType{DataType: "integer", UDTName: "int4"}
		err := assertRequiredColumns(&fakeColumnTypeReader{types: types}, bootSchemaReadTimeout)
		require.EqualError(t, err,
			"required schema column types do not match: audit_logs.actor_id is integer, "+
				"expected bigint (type change recorded but DDL absent?); refusing to boot")
	})

	t.Run("an absent typed column alone gets the column-types refusal", func(t *testing.T) {
		t.Parallel()
		err := assertRequiredColumns(
			&fakeColumnTypeReader{types: passingColumnTypesWithout("webauthn_credentials.sign_count")}, bootSchemaReadTimeout)
		require.EqualError(t, err,
			"required schema column types do not match: webauthn_credentials.sign_count is absent, "+
				"expected bigint (type change recorded but DDL absent?); refusing to boot")
	})

	t.Run("missing and mistyped columns are refused together in one message", func(t *testing.T) {
		t.Parallel()
		types := passingColumnTypesWithout("shows.doors_at", "webauthn_credentials.sign_count")
		types["audit_logs.actor_id"] = columnType{DataType: "integer", UDTName: "int4"}
		types["entity_edit_audit_logs.created_at"] = columnType{DataType: "timestamp without time zone", UDTName: "timestamp"}
		err := assertRequiredColumns(&fakeColumnTypeReader{types: types}, bootSchemaReadTimeout)
		require.EqualError(t, err,
			"required schema columns missing or mistyped: "+
				"missing: shows.doors_at, webauthn_credentials.sign_count; "+
				"mistyped: audit_logs.actor_id (want bigint, got integer), "+
				"entity_edit_audit_logs.created_at (want timestamp with time zone, got timestamp without time zone) "+
				"(migration recorded but DDL absent?); refusing to boot")
	})

	t.Run("missing untyped and absent typed columns are refused together as missing", func(t *testing.T) {
		t.Parallel()
		err := assertRequiredColumns(
			&fakeColumnTypeReader{types: passingColumnTypesWithout("shows.doors_at", "audit_logs.entity_id")},
			bootSchemaReadTimeout)
		require.EqualError(t, err,
			"required schema columns missing: shows.doors_at, audit_logs.entity_id "+
				"(migration recorded but DDL absent?); refusing to boot")
	})
}

// unansweringDB returns a GORM Postgres handle on a local listener that
// accepts connections and never writes to them, so every read waits until its
// context ends.
func unansweringDB(t *testing.T) *gorm.DB {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})

	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	sqlDB, err := sql.Open("pgx", "host=127.0.0.1 port="+port+" user=nobody dbname=nothing sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	return gormDB
}

// Through the real driver: a database that never answers refuses boot within
// the bound instead of holding it.
func TestAssertRequiredSchema_unansweringDatabase(t *testing.T) {
	t.Parallel()

	started := time.Now()
	err := assertRequiredSchemaWithin(unansweringDB(t), 200*time.Millisecond)
	assert.Less(t, time.Since(started), 5*time.Second)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "could not read required schema: no answer within 200ms: "), err.Error())
	assert.True(t, strings.HasSuffix(err.Error(), "; refusing to boot"), err.Error())
}

func TestColumnTypeMatches(t *testing.T) {
	t.Parallel()

	bigint := columnType{DataType: "bigint", UDTName: "int8"}
	timestamptz := columnType{DataType: "timestamp with time zone", UDTName: "timestamptz"}

	assert.True(t, bigint.matches("bigint"))
	assert.True(t, bigint.matches("int8"))
	assert.True(t, bigint.matches("BIGINT"))
	assert.False(t, bigint.matches("integer"))
	assert.False(t, bigint.matches("int4"))
	assert.True(t, timestamptz.matches("timestamptz"))
	assert.True(t, timestamptz.matches("timestamp with time zone"))
	assert.False(t, timestamptz.matches("timestamp"))
	assert.False(t, timestamptz.matches("timestamp without time zone"))
}
