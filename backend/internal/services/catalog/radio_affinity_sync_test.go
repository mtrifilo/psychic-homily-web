package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestRadioService_NilDB_SyncAffinityToRelationships(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error {
		_, err := svc.SyncAffinityToRelationships()
		return err
	})
}

// =============================================================================
// INTEGRATION TESTS (Require Database)
// =============================================================================

type RadioAffinitySyncSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RadioService
}

func TestRadioAffinitySyncSuite(t *testing.T) {
	suite.Run(t, new(RadioAffinitySyncSuite))
}

func (s *RadioAffinitySyncSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &RadioService{db: s.db}
}

func (s *RadioAffinitySyncSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioAffinitySyncSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM radio_artist_affinity")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

// createArtist creates an artist for testing.
func (s *RadioAffinitySyncSuite) createArtist(name, slug string) *models.Artist {
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

// insertAffinity directly inserts a radio_artist_affinity row.
func (s *RadioAffinitySyncSuite) insertAffinity(artistAID, artistBID uint, coOccurrenceCount, showCount, stationCount int) {
	aff := &models.RadioArtistAffinity{
		ArtistAID:         artistAID,
		ArtistBID:         artistBID,
		CoOccurrenceCount: coOccurrenceCount,
		ShowCount:         showCount,
		StationCount:      stationCount,
	}
	s.Require().NoError(s.db.Create(aff).Error)
}

// ── SyncAffinityToRelationships ──

func (s *RadioAffinitySyncSuite) TestSync_CreatesNewRelationships() {
	artist1 := s.createArtist("Artist A", "artist-a-sync1")
	artist2 := s.createArtist("Artist B", "artist-b-sync1")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	s.insertAffinity(lowID, highID, 5, 3, 1)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)
	s.Equal(0, result.Updated)
	s.Equal(0, result.Deleted)

	// Verify the relationship was created
	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.True(rel.AutoDerived)
	s.Equal(models.RelationshipTypeRadioCooccurrence, rel.RelationshipType)
}

func (s *RadioAffinitySyncSuite) TestSync_UpdatesExistingRelationships() {
	artist1 := s.createArtist("Artist A", "artist-a-sync2")
	artist2 := s.createArtist("Artist B", "artist-b-sync2")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// Insert an existing radio_cooccurrence relationship
	existingRel := &models.ArtistRelationship{
		SourceArtistID:   lowID,
		TargetArtistID:   highID,
		RelationshipType: models.RelationshipTypeRadioCooccurrence,
		Score:            0.1,
		AutoDerived:      true,
	}
	s.Require().NoError(s.db.Create(existingRel).Error)

	// Now insert an affinity with higher counts
	s.insertAffinity(lowID, highID, 25, 10, 2)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(0, result.Created)
	s.Equal(1, result.Updated)
	s.Equal(0, result.Deleted)

	// Verify the score was updated
	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	// 25/50 = 0.5 * 1.5 (cross-station) = 0.75
	s.InDelta(0.75, float64(rel.Score), 0.01)
}

func (s *RadioAffinitySyncSuite) TestSync_DeletesStaleRelationships() {
	artist1 := s.createArtist("Artist A", "artist-a-sync3")
	artist2 := s.createArtist("Artist B", "artist-b-sync3")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// Insert an existing radio_cooccurrence relationship but NO affinity data
	staleRel := &models.ArtistRelationship{
		SourceArtistID:   lowID,
		TargetArtistID:   highID,
		RelationshipType: models.RelationshipTypeRadioCooccurrence,
		Score:            0.5,
		AutoDerived:      true,
	}
	s.Require().NoError(s.db.Create(staleRel).Error)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(0, result.Created)
	s.Equal(0, result.Updated)
	s.Equal(1, result.Deleted)

	// Verify the relationship was deleted
	var count int64
	s.db.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			lowID, highID, models.RelationshipTypeRadioCooccurrence).
		Count(&count)
	s.Equal(int64(0), count)
}

func (s *RadioAffinitySyncSuite) TestSync_ScoreNormalization_CappedAt1() {
	artist1 := s.createArtist("Artist A", "artist-a-sync4")
	artist2 := s.createArtist("Artist B", "artist-b-sync4")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// 100 co-occurrences => 100/50 = 2.0 => capped at 1.0
	s.insertAffinity(lowID, highID, 100, 50, 1)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.InDelta(1.0, float64(rel.Score), 0.01)
}

func (s *RadioAffinitySyncSuite) TestSync_CrossStationMultiplier() {
	artist1 := s.createArtist("Artist A", "artist-a-sync5")
	artist2 := s.createArtist("Artist B", "artist-b-sync5")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// 10 co-occurrences, 1 station => 10/50 = 0.2 (no multiplier)
	s.insertAffinity(lowID, highID, 10, 5, 1)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.InDelta(0.2, float64(rel.Score), 0.01)
}

func (s *RadioAffinitySyncSuite) TestSync_CrossStationMultiplier_Applied() {
	artist1 := s.createArtist("Artist A", "artist-a-sync6")
	artist2 := s.createArtist("Artist B", "artist-b-sync6")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// 10 co-occurrences, 2 stations => 10/50 = 0.2 * 1.5 = 0.3
	s.insertAffinity(lowID, highID, 10, 5, 2)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.InDelta(0.3, float64(rel.Score), 0.01)
}

func (s *RadioAffinitySyncSuite) TestSync_CrossStationMultiplier_CappedAt1() {
	artist1 := s.createArtist("Artist A", "artist-a-sync7")
	artist2 := s.createArtist("Artist B", "artist-b-sync7")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// 40 co-occurrences, 3 stations => 40/50 = 0.8 * 1.5 = 1.2 => capped at 1.0
	s.insertAffinity(lowID, highID, 40, 20, 3)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.InDelta(1.0, float64(rel.Score), 0.01)
}

func (s *RadioAffinitySyncSuite) TestSync_SkipsBelowMinimumThreshold() {
	artist1 := s.createArtist("Artist A", "artist-a-sync8")
	artist2 := s.createArtist("Artist B", "artist-b-sync8")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// 1 co-occurrence (below threshold of 2, but inserted directly — ComputeAffinity
	// already filters >= 2. SyncAffinityToRelationships also checks >= 2.)
	aff := &models.RadioArtistAffinity{
		ArtistAID:         lowID,
		ArtistBID:         highID,
		CoOccurrenceCount: 1,
		ShowCount:         1,
		StationCount:      1,
	}
	s.Require().NoError(s.db.Create(aff).Error)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(0, result.Created)

	// No relationship should exist
	var count int64
	s.db.Model(&models.ArtistRelationship{}).
		Where("relationship_type = ?", models.RelationshipTypeRadioCooccurrence).
		Count(&count)
	s.Equal(int64(0), count)
}

func (s *RadioAffinitySyncSuite) TestSync_CanonicalOrderPreserved() {
	artist1 := s.createArtist("Artist A", "artist-a-sync9")
	artist2 := s.createArtist("Artist B", "artist-b-sync9")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// The affinity table already stores canonical order (artist_a_id < artist_b_id)
	s.insertAffinity(lowID, highID, 5, 3, 1)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	// Verify the relationship preserves canonical ordering
	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.Equal(lowID, rel.SourceArtistID)
	s.Equal(highID, rel.TargetArtistID)
}

func (s *RadioAffinitySyncSuite) TestSync_JSONBDetailContent() {
	artist1 := s.createArtist("Artist A", "artist-a-sync10")
	artist2 := s.createArtist("Artist B", "artist-b-sync10")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	s.insertAffinity(lowID, highID, 12, 8, 3)

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(1, result.Created)

	var rel models.ArtistRelationship
	err = s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		lowID, highID, models.RelationshipTypeRadioCooccurrence).First(&rel).Error
	s.Require().NoError(err)
	s.Require().NotNil(rel.Detail)

	var detail map[string]interface{}
	err = json.Unmarshal(*rel.Detail, &detail)
	s.Require().NoError(err)
	s.Equal(float64(12), detail["co_occurrence_count"])
	s.Equal(float64(3), detail["station_count"])
	s.Equal(float64(8), detail["show_count"])
}

func (s *RadioAffinitySyncSuite) TestSync_MultiplePairs() {
	artist1 := s.createArtist("Artist A", "artist-a-sync11")
	artist2 := s.createArtist("Artist B", "artist-b-sync11")
	artist3 := s.createArtist("Artist C", "artist-c-sync11")

	// Sort pairs for canonical ordering
	ids := []uint{artist1.ID, artist2.ID, artist3.ID}
	// Pairs: (1,2), (1,3), (2,3)
	pairs := [][2]uint{}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			low, high := ids[i], ids[j]
			if low > high {
				low, high = high, low
			}
			pairs = append(pairs, [2]uint{low, high})
		}
	}

	for i, pair := range pairs {
		s.insertAffinity(pair[0], pair[1], (i+1)*5, (i+1)*3, 1)
	}

	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(3, result.Created)
	s.Equal(0, result.Updated)
	s.Equal(0, result.Deleted)

	// Verify all 3 relationships exist
	var count int64
	s.db.Model(&models.ArtistRelationship{}).
		Where("relationship_type = ?", models.RelationshipTypeRadioCooccurrence).
		Count(&count)
	s.Equal(int64(3), count)
}

func (s *RadioAffinitySyncSuite) TestSync_DoesNotDeleteOtherRelationshipTypes() {
	artist1 := s.createArtist("Artist A", "artist-a-sync12")
	artist2 := s.createArtist("Artist B", "artist-b-sync12")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// Create a shared_bills relationship (should NOT be deleted)
	sharedBillsRel := &models.ArtistRelationship{
		SourceArtistID:   lowID,
		TargetArtistID:   highID,
		RelationshipType: models.RelationshipTypeSharedBills,
		Score:            0.5,
		AutoDerived:      true,
	}
	s.Require().NoError(s.db.Create(sharedBillsRel).Error)

	// No affinity data — sync should not touch the shared_bills relationship
	result, err := s.svc.SyncAffinityToRelationships()
	s.Require().NoError(err)
	s.Equal(0, result.Created)
	s.Equal(0, result.Updated)
	s.Equal(0, result.Deleted)

	// Verify shared_bills relationship still exists
	var count int64
	s.db.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			lowID, highID, models.RelationshipTypeSharedBills).
		Count(&count)
	s.Equal(int64(1), count)
}
