package db

import (
	"testing"

	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/testutil"
)

func TestAssertRequiredSchema_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	require.NoError(t, AssertRequiredSchema(td.DB))

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(`ALTER TABLE user_bookmarks DROP COLUMN scene_digest_sent_at`)
	require.NoError(t, err)

	err = AssertRequiredSchema(td.DB)
	require.Error(t, err)
	require.Contains(t, err.Error(), "user_bookmarks.scene_digest_sent_at")
}

// An EMPTY timezone_names_snapshot has to fail the boot, not just a missing one
// (PSY-1761). It is the failure mode that raises nothing and logs nothing:
// every venue's zone fails the venue-local guard's membership test, so every
// show silently re-dates onto the state map, and the drift detector cannot see
// it because it compares venues against the live catalog rather than against
// this table.
func TestAssertRequiredSchema_emptyTimezoneSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// The migration seeds it, so a freshly migrated database passes.
	require.NoError(t, AssertRequiredSchema(td.DB))

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(`TRUNCATE timezone_names_snapshot`)
	require.NoError(t, err)

	err = AssertRequiredSchema(td.DB)
	require.Error(t, err, "an empty snapshot must not be allowed to serve traffic")
	require.Contains(t, err.Error(), "timezone_names_snapshot is empty")
}
