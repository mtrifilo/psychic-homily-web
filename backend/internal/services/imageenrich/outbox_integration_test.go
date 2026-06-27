package imageenrich

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// ImageEnrichOutboxTestSuite covers the outbox poller (PSY-1247): claim → enrich →
// finalize, retry/fail bounding, stale-processing reclaim, and concurrent-claim
// disjointness (FOR UPDATE SKIP LOCKED). The enrichers are stubbed via the engine's
// injectable fields, so no real MusicBrainz/Wikidata/Commons/CAA traffic.
type ImageEnrichOutboxTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ImageEnrichOutboxTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}
func (s *ImageEnrichOutboxTestSuite) TearDownSuite() { s.testDB.Cleanup() }
func (s *ImageEnrichOutboxTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM image_enrich_queue")
}
func TestImageEnrichOutboxTestSuite(t *testing.T) { suite.Run(t, new(ImageEnrichOutboxTestSuite)) }

// newPoller wires a poller to the test DB sharing a sweep "engine" (whose
// enrichers the caller overrides per-test). batch + a generous reclaim window.
func (s *ImageEnrichOutboxTestSuite) newPoller(batch int) (*ImageEnrichOutboxPoller, *ImageEnrichmentSweep) {
	engine := NewImageEnrichmentSweep(s.db, nil, "")
	p := NewImageEnrichOutboxPoller(s.db, engine)
	p.batch = batch
	p.staleReclaim = 30 * time.Minute
	return p, engine
}

func (s *ImageEnrichOutboxTestSuite) seedJob(entityType string, entityID uint) *catalogm.ImageEnrichQueueItem {
	job := &catalogm.ImageEnrichQueueItem{EntityType: entityType, EntityID: entityID, Status: catalogm.ImageEnrichStatusPending}
	s.Require().NoError(s.db.Create(job).Error)
	return job
}

func (s *ImageEnrichOutboxTestSuite) reload(id uint) catalogm.ImageEnrichQueueItem {
	var j catalogm.ImageEnrichQueueItem
	s.Require().NoError(s.db.First(&j, id).Error)
	return j
}

// TestClaimsPendingAndMarksDone: a pending artist job and a release job are routed
// to their respective enrichers and marked done with processed_at set.
func (s *ImageEnrichOutboxTestSuite) TestClaimsPendingAndMarksDone() {
	artistJob := s.seedJob(catalogm.ImageEnrichEntityArtist, 11)
	releaseJob := s.seedJob(catalogm.ImageEnrichEntityRelease, 22)

	var gotPhotos, gotCovers []uint
	p, engine := s.newPoller(50)
	engine.enrichPhotos = func(_ context.Context, ids []uint) error { gotPhotos = append(gotPhotos, ids...); return nil }
	engine.enrichCovers = func(_ context.Context, ids []uint) error { gotCovers = append(gotCovers, ids...); return nil }

	p.RunNow(context.Background())

	s.Equal([]uint{11}, gotPhotos)
	s.Equal([]uint{22}, gotCovers)

	a := s.reload(artistJob.ID)
	s.Equal(catalogm.ImageEnrichStatusDone, a.Status)
	s.Equal(1, a.Attempts)
	s.Require().NotNil(a.ProcessedAt)

	r := s.reload(releaseJob.ID)
	s.Equal(catalogm.ImageEnrichStatusDone, r.Status)
	s.Require().NotNil(r.ProcessedAt)
}

// TestEnricherErrorRequeuesThenFails: an enricher error requeues the job until
// attempts reach max_attempts, then marks it failed (mirrors EnrichmentService).
func (s *ImageEnrichOutboxTestSuite) TestEnricherErrorRequeuesThenFails() {
	job := s.seedJob(catalogm.ImageEnrichEntityArtist, 7)

	p, engine := s.newPoller(50)
	engine.enrichPhotos = func(_ context.Context, _ []uint) error { return errors.New("boom") }

	p.RunNow(context.Background())
	j1 := s.reload(job.ID)
	s.Equal(catalogm.ImageEnrichStatusPending, j1.Status, "first failure requeues")
	s.Equal(1, j1.Attempts)
	s.Require().NotNil(j1.LastError)
	s.Contains(*j1.LastError, "boom")

	p.RunNow(context.Background())
	s.Equal(catalogm.ImageEnrichStatusPending, s.reload(job.ID).Status)
	s.Equal(2, s.reload(job.ID).Attempts)

	p.RunNow(context.Background())
	j3 := s.reload(job.ID)
	s.Equal(catalogm.ImageEnrichStatusFailed, j3.Status, "exhausted attempts → failed")
	s.Equal(3, j3.Attempts)
}

// TestSkipsRowAtMaxAttempts: a row already at max_attempts is never claimed
// (attempts < max_attempts filter), so it is left untouched and no enricher runs.
func (s *ImageEnrichOutboxTestSuite) TestSkipsRowAtMaxAttempts() {
	exhausted := &catalogm.ImageEnrichQueueItem{
		EntityType: catalogm.ImageEnrichEntityArtist, EntityID: 9,
		Status: catalogm.ImageEnrichStatusPending, Attempts: 3, MaxAttempts: 3,
	}
	s.Require().NoError(s.db.Create(exhausted).Error)

	var gotPhotos []uint
	p, engine := s.newPoller(50)
	engine.enrichPhotos = func(_ context.Context, ids []uint) error { gotPhotos = append(gotPhotos, ids...); return nil }

	p.RunNow(context.Background())

	s.Empty(gotPhotos, "a maxed-out row must not be claimed")
	j := s.reload(exhausted.ID)
	s.Equal(catalogm.ImageEnrichStatusPending, j.Status)
	s.Equal(3, j.Attempts)
}

// TestReclaimsStaleProcessing: a row stranded in `processing` past the reclaim
// window is returned to pending, then claimed and processed in the same tick.
func (s *ImageEnrichOutboxTestSuite) TestReclaimsStaleProcessing() {
	job := s.seedJob(catalogm.ImageEnrichEntityArtist, 5)
	// Strand it: processing, last touched an hour ago.
	s.Require().NoError(s.db.Exec(
		"UPDATE image_enrich_queue SET status = 'processing', updated_at = NOW() - INTERVAL '1 hour' WHERE id = ?",
		job.ID).Error)

	var gotPhotos []uint
	p, engine := s.newPoller(50)
	p.staleReclaim = time.Minute // 1h-old processing row is well past this
	engine.enrichPhotos = func(_ context.Context, ids []uint) error { gotPhotos = append(gotPhotos, ids...); return nil }

	p.RunNow(context.Background())

	s.Equal([]uint{5}, gotPhotos, "reclaimed row should be enriched")
	s.Equal(catalogm.ImageEnrichStatusDone, s.reload(job.ID).Status)
}

// TestDoesNotReclaimFreshProcessing: a recently-touched `processing` row (another
// worker actively on it) is left alone — neither reclaimed nor claimed.
func (s *ImageEnrichOutboxTestSuite) TestDoesNotReclaimFreshProcessing() {
	job := s.seedJob(catalogm.ImageEnrichEntityArtist, 6)
	s.Require().NoError(s.db.Model(&catalogm.ImageEnrichQueueItem{}).Where("id = ?", job.ID).
		Update("status", catalogm.ImageEnrichStatusProcessing).Error) // updated_at = now

	var gotPhotos []uint
	p, engine := s.newPoller(50) // staleReclaim = 30m
	engine.enrichPhotos = func(_ context.Context, ids []uint) error { gotPhotos = append(gotPhotos, ids...); return nil }

	p.RunNow(context.Background())

	s.Empty(gotPhotos, "a fresh in-flight row must not be reclaimed/claimed")
	s.Equal(catalogm.ImageEnrichStatusProcessing, s.reload(job.ID).Status)
}

// TestSequentialClaimsDoNotReclaimInFlight: once a row is claimed (→ processing) a
// later claim does not re-grab it; it picks the next pending row instead.
func (s *ImageEnrichOutboxTestSuite) TestSequentialClaimsDoNotReclaimInFlight() {
	for i := uint(1); i <= 3; i++ {
		s.seedJob(catalogm.ImageEnrichEntityArtist, i)
	}
	p, _ := s.newPoller(1) // one row per claim

	ctx := context.Background()
	seen := map[uint]bool{}
	for i := 0; i < 3; i++ {
		items, err := p.claimBatch(ctx)
		s.Require().NoError(err)
		s.Require().Len(items, 1)
		s.False(seen[items[0].ID], "claim must not re-grab an in-flight row")
		seen[items[0].ID] = true
	}
	empty, err := p.claimBatch(ctx)
	s.Require().NoError(err)
	s.Empty(empty, "no pending rows remain")
}

// TestConcurrentClaimsDisjoint: many pollers claiming the same queue at once never
// double-claim a row (FOR UPDATE SKIP LOCKED). Every row ends up claimed exactly
// once across all workers.
func (s *ImageEnrichOutboxTestSuite) TestConcurrentClaimsDisjoint() {
	const total = 20
	for i := uint(1); i <= total; i++ {
		s.seedJob(catalogm.ImageEnrichEntityArtist, i)
	}
	p, _ := s.newPoller(total)

	var mu sync.Mutex
	claimed := map[uint]int{}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := p.claimBatch(context.Background())
			if err != nil {
				return
			}
			mu.Lock()
			for _, it := range items {
				claimed[it.ID]++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	s.Len(claimed, total, "every row must be claimed exactly once")
	for id, n := range claimed {
		s.Equal(1, n, "row %d claimed more than once — SKIP LOCKED failed", id)
	}
	var processing int64
	s.Require().NoError(s.db.Model(&catalogm.ImageEnrichQueueItem{}).
		Where("status = ?", catalogm.ImageEnrichStatusProcessing).Count(&processing).Error)
	s.Equal(int64(total), processing)
}
