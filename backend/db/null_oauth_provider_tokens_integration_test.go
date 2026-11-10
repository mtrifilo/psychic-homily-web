package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/testutil"
)

// nullOAuthProviderTokensName is the migration's name without its version, so
// the lookup holds when the version is re-stamped.
const nullOAuthProviderTokensName = "null_oauth_account_provider_tokens"

func readNullOAuthProviderTokens(t *testing.T, direction string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("migrations", "*_"+nullOAuthProviderTokensName+"."+direction+".sql"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "exactly one %s migration named %s", direction, nullOAuthProviderTokensName)
	body, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	return string(body)
}

// oauthAccountRow is one oauth_accounts row as the migration test reads it.
type oauthAccountRow struct {
	providerEmail sql.NullString
	accessToken   sql.NullString
	refreshToken  sql.NullString
	expiresAt     sql.NullTime
}

func readOAuthAccountRow(t *testing.T, sqlDB *sql.DB, id int64) oauthAccountRow {
	t.Helper()
	var row oauthAccountRow
	require.NoError(t, sqlDB.QueryRow(
		`SELECT provider_email, access_token, refresh_token, expires_at
		   FROM oauth_accounts WHERE id = $1`, id).
		Scan(&row.providerEmail, &row.accessToken, &row.refreshToken, &row.expiresAt))
	return row
}

// The up migration leaves no oauth_accounts row holding a provider token or
// expiry and changes nothing else on the row; the down migration runs clean
// and restores nothing.
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
	require.True(t, readOAuthAccountRow(t, sqlDB, withTokens).accessToken.Valid, "the fixture holds a token before the migration")

	_, err = sqlDB.Exec(readNullOAuthProviderTokens(t, "up"))
	require.NoError(t, err)

	var holding int64
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM oauth_accounts
		  WHERE access_token IS NOT NULL OR refresh_token IS NOT NULL OR expires_at IS NOT NULL`).Scan(&holding))
	assert.Zero(t, holding, "no row holds a provider token or expiry")

	for id, email := range map[int64]string{
		withTokens: "google@test.local",
		onlyExpiry: "github@test.local",
		withNone:   "apple@test.local",
	} {
		row := readOAuthAccountRow(t, sqlDB, id)
		assert.False(t, row.accessToken.Valid, "row %d access_token", id)
		assert.False(t, row.refreshToken.Valid, "row %d refresh_token", id)
		assert.False(t, row.expiresAt.Valid, "row %d expires_at", id)
		assert.Equal(t, email, row.providerEmail.String, "row %d keeps its profile", id)
	}

	t.Run("applying it again changes nothing", func(t *testing.T) {
		_, err := sqlDB.Exec(readNullOAuthProviderTokens(t, "up"))
		require.NoError(t, err)
		assert.False(t, readOAuthAccountRow(t, sqlDB, withTokens).accessToken.Valid)
	})

	t.Run("down runs clean and restores nothing", func(t *testing.T) {
		_, err := sqlDB.Exec(readNullOAuthProviderTokens(t, "down"))
		require.NoError(t, err)
		row := readOAuthAccountRow(t, sqlDB, withTokens)
		assert.False(t, row.accessToken.Valid)
		assert.False(t, row.refreshToken.Valid)
		assert.False(t, row.expiresAt.Valid)
	})
}
