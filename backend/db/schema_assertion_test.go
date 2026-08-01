package db

import (
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
