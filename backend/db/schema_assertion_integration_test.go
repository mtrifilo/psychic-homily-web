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
