package db

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/testutil"
)

// nullOAuthProviderTokensName is the migration's name without its version, so
// the lookup holds when the version is re-stamped.
const nullOAuthProviderTokensName = "null_oauth_account_provider_tokens"

// oauthRowsHoldingCredential counts the oauth_accounts rows that hold a
// provider token or expiry.
func oauthRowsHoldingCredential(t *testing.T, sqlDB *sql.DB) int64 {
	t.Helper()
	var holding int64
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM oauth_accounts
		  WHERE access_token IS NOT NULL OR refresh_token IS NOT NULL OR expires_at IS NOT NULL`).Scan(&holding))
	return holding
}

// The up migration leaves no oauth_accounts row holding a provider token or
// expiry and keeps each row's profile; the down migration runs clean and
// restores nothing.
func TestNullOAuthProviderTokensMigration_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	sqlDB, err := td.DB.DB()
	require.NoError(t, err)

	var userID, withTokens, onlyExpiry, withNone int64
	require.NoError(t, sqlDB.QueryRow(
		`INSERT INTO users (email, is_active, email_verified)
		 VALUES ('null-tokens@test.local', TRUE, TRUE) RETURNING id`).Scan(&userID))
	require.NoError(t, sqlDB.QueryRow(
		`INSERT INTO oauth_accounts (user_id, provider, provider_user_id, provider_email,
		                             access_token, refresh_token, expires_at)
		 VALUES ($1, 'google', 'null-tokens-google', 'google@test.local',
		         'stored-access', 'stored-refresh', now() + interval '1 hour')
		 RETURNING id`, userID).Scan(&withTokens))
	require.NoError(t, sqlDB.QueryRow(
		`INSERT INTO oauth_accounts (user_id, provider, provider_user_id, provider_email, expires_at)
		 VALUES ($1, 'github', 'null-tokens-github', 'github@test.local', now())
		 RETURNING id`, userID).Scan(&onlyExpiry))
	require.NoError(t, sqlDB.QueryRow(
		`INSERT INTO oauth_accounts (user_id, provider, provider_user_id, provider_email)
		 VALUES ($1, 'apple', 'null-tokens-apple', 'apple@test.local')
		 RETURNING id`, userID).Scan(&withNone))
	require.Equal(t, int64(2), oauthRowsHoldingCredential(t, sqlDB), "the fixture holds credentials before the migration")

	up := readMigrationByName(t, nullOAuthProviderTokensName, "up")
	_, err = sqlDB.Exec(up)
	require.NoError(t, err)
	assert.Zero(t, oauthRowsHoldingCredential(t, sqlDB), "no row holds a provider token or expiry")

	for id, email := range map[int64]string{
		withTokens: "google@test.local",
		onlyExpiry: "github@test.local",
		withNone:   "apple@test.local",
	} {
		var stored string
		require.NoError(t, sqlDB.QueryRow(`SELECT provider_email FROM oauth_accounts WHERE id = $1`, id).Scan(&stored))
		assert.Equal(t, email, stored, "row %d keeps its profile", id)
	}

	t.Run("applying it again changes nothing", func(t *testing.T) {
		_, err := sqlDB.Exec(up)
		require.NoError(t, err)
		assert.Zero(t, oauthRowsHoldingCredential(t, sqlDB))
	})

	t.Run("down runs clean and restores nothing", func(t *testing.T) {
		_, err := sqlDB.Exec(readMigrationByName(t, nullOAuthProviderTokensName, "down"))
		require.NoError(t, err)
		assert.Zero(t, oauthRowsHoldingCredential(t, sqlDB))
	})
}
