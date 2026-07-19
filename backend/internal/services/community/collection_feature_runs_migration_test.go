package community

// PSY-1500: migration-level coverage for collection_feature_runs — the backfill
// (both reconstruction paths) and a clean down-migration. Uses its own
// testcontainer suite so it can drop + re-run the real up.sql against seeded
// data (the suite-wide migration run sees an empty collections table, so the
// migration-time backfill is a no-op; re-running it against a seed is how we
// exercise the actual SQL rather than a hand-rewritten copy).

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testutil"
)

const featureRunsMigrationVersion = "20260719020000_create_collection_feature_runs"

type FeatureRunsMigrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	sqlDB  *sql.DB
}

func (suite *FeatureRunsMigrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	db, err := suite.testDB.DB.DB()
	suite.Require().NoError(err)
	suite.sqlDB = db
}

func (suite *FeatureRunsMigrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FeatureRunsMigrationTestSuite) TearDownTest() {
	_, _ = suite.sqlDB.Exec("DELETE FROM collection_feature_runs")
	_, _ = suite.sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = suite.sqlDB.Exec("DELETE FROM collections")
	_, _ = suite.sqlDB.Exec("DELETE FROM users")
	// A down-migration test may have dropped the table; restore it so suite
	// ordering can't leave a later test without the schema.
	suite.ensureTableExists()
}

func TestFeatureRunsMigrationTestSuite(t *testing.T) {
	suite.Run(t, new(FeatureRunsMigrationTestSuite))
}

func migrationPath(suffix string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "db", "migrations", featureRunsMigrationVersion+suffix)
}

func (suite *FeatureRunsMigrationTestSuite) readMigration(suffix string) string {
	content, err := os.ReadFile(migrationPath(suffix))
	suite.Require().NoError(err)
	return string(content)
}

func (suite *FeatureRunsMigrationTestSuite) tableExists() bool {
	var exists bool
	err := suite.sqlDB.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'collection_feature_runs')`,
	).Scan(&exists)
	suite.Require().NoError(err)
	return exists
}

func (suite *FeatureRunsMigrationTestSuite) ensureTableExists() {
	if !suite.tableExists() {
		_, err := suite.sqlDB.Exec(suite.readMigration(".up.sql"))
		suite.Require().NoError(err)
	}
}

// reapplyUp drops the (empty) table and re-runs the real up.sql so the backfill
// executes against whatever collections/audit_logs the test just seeded.
func (suite *FeatureRunsMigrationTestSuite) reapplyUp() {
	_, err := suite.sqlDB.Exec("DROP TABLE IF EXISTS collection_feature_runs CASCADE")
	suite.Require().NoError(err)
	_, err = suite.sqlDB.Exec(suite.readMigration(".up.sql"))
	suite.Require().NoError(err)
}

func (suite *FeatureRunsMigrationTestSuite) seedUser() *authm.User {
	email := "mig-" + time.Now().Format("150405.000000000") + "@test.com"
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	suite.Require().NoError(suite.testDB.DB.Create(user).Error)
	return user
}

func (suite *FeatureRunsMigrationTestSuite) seedCollection(creatorID uint, title, slug string, featured bool, createdAt time.Time) *communitym.Collection {
	c := &communitym.Collection{
		Title:      title,
		Slug:       slug,
		CreatorID:  creatorID,
		IsFeatured: featured,
		IsPublic:   true,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
	suite.Require().NoError(suite.testDB.DB.Create(c).Error)
	return c
}

func (suite *FeatureRunsMigrationTestSuite) seedAuditFeatured(slug string, featured bool, createdAt time.Time) {
	meta := `{"slug":"` + slug + `","featured":` + boolStr(featured) + `}`
	_, err := suite.sqlDB.Exec(
		`INSERT INTO audit_logs (action, entity_type, entity_id, metadata, created_at)
		 VALUES ('set_collection_featured', 'collection', 0, $1::jsonb, $2)`,
		meta, createdAt,
	)
	suite.Require().NoError(err)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (suite *FeatureRunsMigrationTestSuite) runsFor(collectionID uint) []communitym.CollectionFeatureRun {
	var runs []communitym.CollectionFeatureRun
	suite.Require().NoError(suite.testDB.DB.Where("collection_id = ?", collectionID).Find(&runs).Error)
	return runs
}

// Backfill opens exactly one open run per currently-featured collection:
//   - path 1 (audit event present) → featured_at = the newest matching event's
//     time, featured_at_estimated = false;
//   - path 2 (no audit event) → featured_at = collections.created_at,
//     featured_at_estimated = true;
//   - non-featured collections get no run.
func (suite *FeatureRunsMigrationTestSuite) TestBackfill_BothReconstructionPaths() {
	user := suite.seedUser()
	base := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)

	// Path 1: featured + a matching audit event (with an older stale event to
	// prove the NEWEST true event wins).
	auditColl := suite.seedCollection(user.ID, "Audit Path", "audit-path", true, base)
	suite.seedAuditFeatured("audit-path", true, base.Add(1*time.Hour))
	auditWinner := base.Add(10 * time.Hour)
	suite.seedAuditFeatured("audit-path", true, auditWinner)

	// Path 2: featured, no audit event → estimated from created_at.
	createdColl := suite.seedCollection(user.ID, "Created Path", "created-path", true, base.Add(2*time.Hour))

	// Not featured → no run, even with a stale 'false' audit row.
	plainColl := suite.seedCollection(user.ID, "Plain", "plain", false, base)
	suite.seedAuditFeatured("plain", false, base)

	suite.reapplyUp()

	// Path 1
	auditRuns := suite.runsFor(auditColl.ID)
	suite.Require().Len(auditRuns, 1)
	suite.False(auditRuns[0].FeaturedAtEstimated)
	suite.Nil(auditRuns[0].UnfeaturedAt)
	suite.WithinDuration(auditWinner, auditRuns[0].FeaturedAt, time.Second)

	// Path 2
	createdRuns := suite.runsFor(createdColl.ID)
	suite.Require().Len(createdRuns, 1)
	suite.True(createdRuns[0].FeaturedAtEstimated)
	suite.Nil(createdRuns[0].UnfeaturedAt)
	suite.WithinDuration(createdColl.CreatedAt, createdRuns[0].FeaturedAt, time.Second)

	// Not featured
	suite.Empty(suite.runsFor(plainColl.ID))
}

// The down-migration drops the table (and with it its indexes and FKs) cleanly,
// leaving collections.is_featured untouched.
func (suite *FeatureRunsMigrationTestSuite) TestDownMigration_DropsCleanly() {
	user := suite.seedUser()
	coll := suite.seedCollection(user.ID, "Survives Down", "survives-down", true, time.Now().UTC())

	suite.Require().True(suite.tableExists())

	_, err := suite.sqlDB.Exec(suite.readMigration(".down.sql"))
	suite.Require().NoError(err)
	suite.False(suite.tableExists())

	// is_featured is untouched by the down migration.
	var isFeatured bool
	suite.Require().NoError(suite.sqlDB.QueryRow(
		`SELECT is_featured FROM collections WHERE id = $1`, coll.ID,
	).Scan(&isFeatured))
	suite.True(isFeatured)

	// Restore for the rest of the suite.
	suite.ensureTableExists()
}
