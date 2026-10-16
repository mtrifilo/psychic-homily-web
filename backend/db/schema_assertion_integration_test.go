package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// A column that exists with its pre-migration type has to fail the boot, not
// only a missing one: the existence check alone passes it.
func TestAssertRequiredSchema_columnTypeDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// A freshly migrated database has every listed type.
	require.NoError(t, AssertRequiredSchema(td.DB))

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(`ALTER TABLE webauthn_credentials ALTER COLUMN sign_count TYPE INTEGER`)
	require.NoError(t, err)

	err = AssertRequiredSchema(td.DB)
	require.EqualError(t, err,
		"required schema column types do not match: webauthn_credentials.sign_count is integer, "+
			"expected bigint (type change recorded but DDL absent?); refusing to boot")

	// The type read is also what reports a typed column that is absent.
	_, err = sqlDB.Exec(`ALTER TABLE webauthn_credentials DROP COLUMN sign_count`)
	require.NoError(t, err)
	err = AssertRequiredSchema(td.DB)
	require.Error(t, err)
	require.Contains(t, err.Error(), "webauthn_credentials.sign_count is absent, expected bigint")
}

func TestAssertRequiredSchema_unlistedColumnTypeIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	// Same table as a listed column, so the type read covers it.
	_, err = sqlDB.Exec(`ALTER TABLE webauthn_credentials ALTER COLUMN display_name TYPE TEXT`)
	require.NoError(t, err)

	require.NoError(t, AssertRequiredSchema(td.DB))
}

// A missing column and a mistyped one are refused together, in one boot.
func TestAssertRequiredSchema_missingAndMistypedColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(`ALTER TABLE shows DROP COLUMN doors_at`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`ALTER TABLE webauthn_credentials ALTER COLUMN sign_count TYPE INTEGER`)
	require.NoError(t, err)

	err = AssertRequiredSchema(td.DB)
	require.EqualError(t, err,
		"required schema columns missing or mistyped: missing: shows.doors_at; "+
			"mistyped: webauthn_credentials.sign_count (want bigint, got integer) "+
			"(migration recorded but DDL absent?); refusing to boot")
}

// A read the server holds open (here, behind another session's lock) refuses
// boot within the bound, and the connection it ran on stays usable.
func TestAssertRequiredSchema_readHeldPastBound(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	ctx := context.Background()
	sqlDB, err := td.DB.DB()
	require.NoError(t, err)
	locker, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = locker.ExecContext(ctx, `LOCK TABLE timezone_names_snapshot IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)

	started := time.Now()
	err = assertRequiredSchemaWithin(td.DB, 500*time.Millisecond)
	elapsed := time.Since(started)
	require.NoError(t, locker.Rollback())

	assert.Less(t, elapsed, 5*time.Second)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "could not read timezone_names_snapshot: no answer within 500ms: "), err.Error())
	assert.True(t, strings.HasSuffix(err.Error(), "; refusing to boot"), err.Error())

	require.NoError(t, AssertRequiredSchema(td.DB))
}
