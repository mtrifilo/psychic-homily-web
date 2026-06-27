package catalog

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// ImageEnrichEnqueueTestSuite covers the transactional-outbox enqueue (PSY-1247):
// the create funnel enqueues a job in the SAME tx (atomic), but an enqueue failure
// must never fail the create (best-effort via a savepoint), and re-enqueue is a
// no-op against the one-active-job-per-entity index.
type ImageEnrichEnqueueTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ImageEnrichEnqueueTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}
func (s *ImageEnrichEnqueueTestSuite) TearDownSuite() { s.testDB.Cleanup() }
func (s *ImageEnrichEnqueueTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM image_enrich_queue")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}
func TestImageEnrichEnqueueTestSuite(t *testing.T) { suite.Run(t, new(ImageEnrichEnqueueTestSuite)) }

func (s *ImageEnrichEnqueueTestSuite) queueCount() int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.ImageEnrichQueueItem{}).Count(&n).Error)
	return n
}

// TestFunnelEnqueuesOnCreate: creating an artist via the funnel writes exactly one
// pending job for it.
func (s *ImageEnrichEnqueueTestSuite) TestFunnelEnqueuesOnCreate() {
	a, created, err := FindOrCreateArtistTx(s.db, "Enqueued", nil)
	s.Require().NoError(err)
	s.Require().True(created)

	var item catalogm.ImageEnrichQueueItem
	err = s.db.Where("entity_type = ? AND entity_id = ?", catalogm.ImageEnrichEntityArtist, a.ID).First(&item).Error
	s.Require().NoError(err)
	s.Equal(catalogm.ImageEnrichStatusPending, item.Status)
	s.Equal(int64(1), s.queueCount())
}

// TestNoEnqueueOnFound: a find (not create) must NOT enqueue again — the entity was
// already enqueued at its own creation, so re-referencing it from a show is churn.
func (s *ImageEnrichEnqueueTestSuite) TestNoEnqueueOnFound() {
	_, created, err := FindOrCreateArtistTx(s.db, "Dup", nil)
	s.Require().NoError(err)
	s.Require().True(created)

	_, created2, err := FindOrCreateArtistTx(s.db, "dup", nil) // case-insensitive find
	s.Require().NoError(err)
	s.Require().False(created2)

	s.Equal(int64(1), s.queueCount(), "found path must not enqueue a second job")
}

// TestEnqueueIsAtomicWithCreate: if the surrounding tx rolls back, NEITHER the
// artist NOR the queue row survives (AC1 — no orphan job).
func (s *ImageEnrichEnqueueTestSuite) TestEnqueueIsAtomicWithCreate() {
	rollback := errors.New("force rollback")
	err := s.db.Transaction(func(tx *gorm.DB) error {
		a, created, ferr := FindOrCreateArtistTx(tx, "RolledBack", nil)
		s.Require().NoError(ferr)
		s.Require().True(created)
		s.Require().NotZero(a.ID)
		return rollback
	})
	s.Require().ErrorIs(err, rollback)

	var artistCount int64
	s.db.Model(&catalogm.Artist{}).Where("name = ?", "RolledBack").Count(&artistCount)
	s.Zero(artistCount)
	s.Zero(s.queueCount(), "queue row must roll back with the create")
}

// TestEnqueueFailureDoesNotFailCreate is the crux of the savepoint design (AC5).
// A failing enqueue (here forced by an invalid entity_type that trips the CHECK
// constraint) must NOT poison the outer transaction: the artist still commits, and
// no queue row is left behind. Without the savepoint, the aborted statement would
// turn the outer COMMIT into a ROLLBACK and the artist would silently vanish.
func (s *ImageEnrichEnqueueTestSuite) TestEnqueueFailureDoesNotFailCreate() {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		a := &catalogm.Artist{Name: "Survivor"}
		if cerr := tx.Create(a).Error; cerr != nil {
			return cerr
		}
		// Invalid entity_type → INSERT violates the CHECK → nested-tx savepoint
		// rolls back ONLY the enqueue; the helper swallows the error.
		enqueueImageEnrich(tx, "bogus_type", a.ID)
		return nil
	})
	s.Require().NoError(err, "outer tx must commit despite the failed enqueue")

	var artistCount int64
	s.db.Model(&catalogm.Artist{}).Where("name = ?", "Survivor").Count(&artistCount)
	s.Equal(int64(1), artistCount, "artist must survive a poisoned enqueue (savepoint isolation)")
	s.Zero(s.queueCount(), "no queue row should exist after the rejected insert")
}

// TestReEnqueueIsIdempotent: a second enqueue for the same entity is a no-op
// against the one-active-job-per-entity partial unique index (ON CONFLICT DO
// NOTHING), not an error.
func (s *ImageEnrichEnqueueTestSuite) TestReEnqueueIsIdempotent() {
	enqueueImageEnrich(s.db, catalogm.ImageEnrichEntityArtist, 4242)
	enqueueImageEnrich(s.db, catalogm.ImageEnrichEntityArtist, 4242)
	s.Equal(int64(1), s.queueCount(), "duplicate active enqueue must be a no-op")
}
