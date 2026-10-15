package db

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockColumnChecker struct {
	present map[string]bool
}

func (m *mockColumnChecker) HasColumn(table interface{}, column string) bool {
	tableName, ok := table.(string)
	if !ok {
		return false
	}
	return m.present[tableName+"."+column]
}

// allRequiredColumnKeys renders the production checklist in the "table.column"
// form the mock checker keys on.
func allRequiredColumnKeys() []string {
	keys := make([]string, 0, len(requiredSchemaColumns))
	for _, col := range requiredSchemaColumns {
		keys = append(keys, col.Table+"."+col.Column)
	}
	return keys
}

func TestAssertRequiredSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		present []string
		wantErr string
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
			wantErr: "shows.doors_at, shows.music_at",
		},
		{
			name: "missing scene digest bookmark column",
			present: []string{
				"user_preferences.notify_on_scene_digest",
			},
			wantErr: "user_bookmarks.scene_digest_sent_at",
		},
		{
			name: "missing scene digest preference column",
			present: []string{
				"user_bookmarks.scene_digest_sent_at",
			},
			wantErr: "user_preferences.notify_on_scene_digest",
		},
		{
			name:    "missing all required columns",
			present: nil,
			wantErr: "user_bookmarks.scene_digest_sent_at, user_preferences.notify_on_scene_digest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			present := make(map[string]bool, len(tc.present))
			for _, key := range tc.present {
				present[key] = true
			}

			err := assertRequiredSchema(&mockColumnChecker{present: present})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Contains(t, err.Error(), "refusing to boot")
		})
	}
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
}

func (f *fakeColumnTypeReader) ColumnTypes([]string) (map[string]columnType, error) {
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

func TestAssertRequiredColumnTypes(t *testing.T) {
	t.Parallel()

	t.Run("every listed column has its type", func(t *testing.T) {
		t.Parallel()
		reader := &fakeColumnTypeReader{types: listedColumnTypes()}
		require.NoError(t, assertRequiredColumnTypes(reader))
	})

	t.Run("a listed column with another type fails with the column, actual and expected type", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.sign_count"] = columnType{DataType: "integer", UDTName: "int4"}
		err := assertRequiredColumnTypes(&fakeColumnTypeReader{types: types})
		require.EqualError(t, err,
			"required schema column types do not match: webauthn_credentials.sign_count is integer, "+
				"expected bigint (type change recorded but DDL absent?); refusing to boot")
	})

	t.Run("every wrong column is reported", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.sign_count"] = columnType{DataType: "integer", UDTName: "int4"}
		types["entity_edit_audit_logs.created_at"] = columnType{DataType: "timestamp without time zone", UDTName: "timestamp"}
		err := assertRequiredColumnTypes(&fakeColumnTypeReader{types: types})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webauthn_credentials.sign_count is integer, expected bigint")
		assert.Contains(t, err.Error(),
			"entity_edit_audit_logs.created_at is timestamp without time zone, expected timestamp with time zone")
	})

	t.Run("a listed column the reader did not return is reported absent", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		delete(types, "webauthn_credentials.sign_count")
		err := assertRequiredColumnTypes(&fakeColumnTypeReader{types: types})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webauthn_credentials.sign_count is absent, expected bigint")
		assert.Contains(t, err.Error(), "refusing to boot")
	})

	t.Run("an unlisted column's type is ignored", func(t *testing.T) {
		t.Parallel()
		types := listedColumnTypes()
		types["webauthn_credentials.display_name"] = columnType{DataType: "integer", UDTName: "int4"}
		types["user_bookmarks.scene_digest_sent_at"] = columnType{DataType: "text", UDTName: "text"}
		require.NoError(t, assertRequiredColumnTypes(&fakeColumnTypeReader{types: types}))
	})

	t.Run("a read error refuses boot", func(t *testing.T) {
		t.Parallel()
		err := assertRequiredColumnTypes(&fakeColumnTypeReader{err: errors.New("connection reset")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not read required column types: connection reset")
		assert.Contains(t, err.Error(), "refusing to boot")
	})
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
