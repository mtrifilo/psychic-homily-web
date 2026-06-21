package sourceregistry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type SourceConfigIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *SourceConfigService
}

func (s *SourceConfigIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewSourceConfigService(s.testDB.DB)
}

func (s *SourceConfigIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *SourceConfigIntegrationTestSuite) TearDownTest() {
	s.db.Exec("DELETE FROM source_configs")
}

func TestSourceConfigIntegrationSuite(t *testing.T) {
	suite.Run(t, new(SourceConfigIntegrationTestSuite))
}

// The registry is polymorphic with no FK on entity_id, so arbitrary IDs are fine.
func ptr(s string) *string { return &s }

// --- CreateOrUpdate ---

func (s *SourceConfigIntegrationTestSuite) TestCreateOrUpdate_NewConfig() {
	cfg := &adminm.SourceConfig{
		EntityType: adminm.SourceEntityLabel,
		EntityID:   1,
		SourceURL:  ptr("https://sacredbonesrecords.com/pages/artists"),
	}
	out, err := s.svc.CreateOrUpdate(cfg)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Equal(adminm.SourceEntityLabel, out.EntityType)
	s.Equal(uint(1), out.EntityID)
	s.Equal("https://sacredbonesrecords.com/pages/artists", *out.SourceURL)
	s.Equal(0, out.ConsecutiveFailures)
	s.Nil(out.LastRefreshedAt)
}

func (s *SourceConfigIntegrationTestSuite) TestCreateOrUpdate_UpsertsExisting() {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{
		EntityType: adminm.SourceEntityVenue, EntityID: 7, SourceURL: ptr("https://old.example.com"),
	})
	s.Require().NoError(err)

	out, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{
		EntityType: adminm.SourceEntityVenue, EntityID: 7, SourceURL: ptr("https://new.example.com"),
	})
	s.NoError(err)
	s.Require().NotNil(out)
	s.Equal("https://new.example.com", *out.SourceURL)

	// Exactly one row for that (entity_type, entity_id).
	var count int64
	s.db.Model(&adminm.SourceConfig{}).Where("entity_type = ? AND entity_id = ?", adminm.SourceEntityVenue, 7).Count(&count)
	s.Equal(int64(1), count)
}

func (s *SourceConfigIntegrationTestSuite) TestCreateOrUpdate_RejectsInvalidEntityType() {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: "artist", EntityID: 1})
	s.Error(err)
}

func (s *SourceConfigIntegrationTestSuite) TestCreateOrUpdate_RejectsZeroEntityID() {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: adminm.SourceEntityLabel, EntityID: 0})
	s.Error(err)
}

// --- GetByEntity ---

func (s *SourceConfigIntegrationTestSuite) TestGetByEntity_NilWhenMissing() {
	out, err := s.svc.GetByEntity(adminm.SourceEntityLabel, 999)
	s.NoError(err)
	s.Nil(out)
}

// --- RecordRefresh ---

func (s *SourceConfigIntegrationTestSuite) TestRecordRefresh_StampsAndResetsFailures() {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: adminm.SourceEntityLabel, EntityID: 2, SourceURL: ptr("https://x.com")})
	s.Require().NoError(err)
	s.Require().NoError(s.svc.IncrementFailures(adminm.SourceEntityLabel, 2))

	err = s.svc.RecordRefresh(adminm.SourceEntityLabel, 2, ptr("deadbeef"))
	s.NoError(err)

	out, err := s.svc.GetByEntity(adminm.SourceEntityLabel, 2)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Require().NotNil(out.LastRefreshedAt)
	s.Equal(0, out.ConsecutiveFailures)
	s.Require().NotNil(out.LastContentHash)
	s.Equal("deadbeef", *out.LastContentHash)
}

func (s *SourceConfigIntegrationTestSuite) TestRecordRefresh_ErrorsWhenMissing() {
	err := s.svc.RecordRefresh(adminm.SourceEntityLabel, 12345, nil)
	s.Error(err)
}

// --- IncrementFailures ---

func (s *SourceConfigIntegrationTestSuite) TestIncrementFailures_Bumps() {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: adminm.SourceEntityVenue, EntityID: 3})
	s.Require().NoError(err)
	s.Require().NoError(s.svc.IncrementFailures(adminm.SourceEntityVenue, 3))
	s.Require().NoError(s.svc.IncrementFailures(adminm.SourceEntityVenue, 3))

	out, err := s.svc.GetByEntity(adminm.SourceEntityVenue, 3)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Equal(2, out.ConsecutiveFailures)
}

func (s *SourceConfigIntegrationTestSuite) TestIncrementFailures_ErrorsWhenMissing() {
	err := s.svc.IncrementFailures(adminm.SourceEntityVenue, 999)
	s.Error(err)
}

// --- ListStale ---

func (s *SourceConfigIntegrationTestSuite) seedRefreshedAt(entityType string, id uint, t *time.Time) {
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: entityType, EntityID: id})
	s.Require().NoError(err)
	if t != nil {
		s.Require().NoError(s.db.Model(&adminm.SourceConfig{}).
			Where("entity_type = ? AND entity_id = ?", entityType, id).
			Update("last_refreshed_at", *t).Error)
	}
}

func (s *SourceConfigIntegrationTestSuite) TestListStale_OrdersNeverRefreshedFirstThenOldest() {
	old := time.Now().Add(-72 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	s.seedRefreshedAt(adminm.SourceEntityVenue, 100, &recent) // recent
	s.seedRefreshedAt(adminm.SourceEntityLabel, 200, nil)     // never (stalest)
	s.seedRefreshedAt(adminm.SourceEntityVenue, 300, &old)    // old

	out, err := s.svc.ListStale(0, 0)
	s.NoError(err)
	s.Require().Len(out, 3)
	s.Equal(uint(200), out[0].EntityID) // never-refreshed first
	s.Equal(uint(300), out[1].EntityID) // then oldest
	s.Equal(uint(100), out[2].EntityID) // then most recent
}

func (s *SourceConfigIntegrationTestSuite) TestListStale_RespectsLimit() {
	s.seedRefreshedAt(adminm.SourceEntityLabel, 1, nil)
	s.seedRefreshedAt(adminm.SourceEntityLabel, 2, nil)
	s.seedRefreshedAt(adminm.SourceEntityLabel, 3, nil)

	out, err := s.svc.ListStale(2, 0)
	s.NoError(err)
	s.Len(out, 2)
}

func (s *SourceConfigIntegrationTestSuite) TestListStale_ExcludesCircuitBroken() {
	s.seedRefreshedAt(adminm.SourceEntityLabel, 1, nil)
	// id 2 has hit the failure threshold.
	_, err := s.svc.CreateOrUpdate(&adminm.SourceConfig{EntityType: adminm.SourceEntityLabel, EntityID: 2})
	s.Require().NoError(err)
	for i := 0; i < 5; i++ {
		s.Require().NoError(s.svc.IncrementFailures(adminm.SourceEntityLabel, 2))
	}

	out, err := s.svc.ListStale(0, 5)
	s.NoError(err)
	s.Require().Len(out, 1)
	s.Equal(uint(1), out[0].EntityID)
}
