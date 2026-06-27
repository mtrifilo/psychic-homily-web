package catalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// ImageEnrichSweepIntegrationTestSuite covers the sweep's new logic — the
// no-result memo selection + stamping (PSY-1246). The provider lookups
// (BackfillCommonsPhotos / BackfillCoverArt) are stubbed via the injectable
// enrich fields, so these tests exercise selection/stamping without external
// MusicBrainz/Wikidata/Commons/CAA traffic.
type ImageEnrichSweepIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ImageEnrichSweepIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ImageEnrichSweepIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ImageEnrichSweepIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestImageEnrichSweepIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ImageEnrichSweepIntegrationTestSuite))
}

// newTestSweep wires a sweep to the test DB with capturing fake enrichers, the
// given batch, and a 30-day re-attempt window. Returns the sweep plus pointers to
// the captured id slices each enricher received.
func (s *ImageEnrichSweepIntegrationTestSuite) newTestSweep(batch int) (*ImageEnrichmentSweep, *[]uint, *[]uint) {
	sw := NewImageEnrichmentSweep(s.db, nil, "")
	sw.batch = batch
	sw.reattempt = 30 * 24 * time.Hour

	var gotPhotoIDs, gotCoverIDs []uint
	sw.enrichPhotos = func(_ context.Context, ids []uint) error { gotPhotoIDs = append(gotPhotoIDs, ids...); return nil }
	sw.enrichCovers = func(_ context.Context, ids []uint) error { gotCoverIDs = append(gotCoverIDs, ids...); return nil }
	return sw, &gotPhotoIDs, &gotCoverIDs
}

func (s *ImageEnrichSweepIntegrationTestSuite) TestSelectsImagelessNotRecentlyAttempted() {
	recent := time.Now().Add(-1 * time.Hour)      // within window → skip
	stale := time.Now().Add(-60 * 24 * time.Hour) // beyond 30d window → eligible

	never := &catalogm.Artist{Name: "Never Tried"}                                              // eligible (NULL attempt)
	hasImage := &catalogm.Artist{Name: "Has Image", ImageURL: sweepStrPtr("https://img/x.jpg")} // skip (has image)
	recentlyTried := &catalogm.Artist{Name: "Recently Tried", ImageEnrichAttemptedAt: &recent}  // skip (recent)
	staleTried := &catalogm.Artist{Name: "Stale Tried", ImageEnrichAttemptedAt: &stale}         // eligible
	for _, a := range []*catalogm.Artist{never, hasImage, recentlyTried, staleTried} {
		s.Require().NoError(s.db.Create(a).Error)
	}

	sw, gotPhotos, _ := s.newTestSweep(50)
	sw.RunSweepNow(context.Background())

	// Only the image-less, not-recently-attempted rows, NULLS FIRST then oldest.
	s.Equal([]uint{never.ID, staleTried.ID}, *gotPhotos)

	// Selected rows got stamped; skipped rows untouched.
	s.NotNil(s.reloadArtist(never).ImageEnrichAttemptedAt)
	s.NotNil(s.reloadArtist(staleTried).ImageEnrichAttemptedAt)
	s.Nil(s.reloadArtist(hasImage).ImageEnrichAttemptedAt)
	s.WithinDuration(recent, *s.reloadArtist(recentlyTried).ImageEnrichAttemptedAt, time.Second)
}

func (s *ImageEnrichSweepIntegrationTestSuite) TestStampsBeforeEnrichEvenOnError() {
	a := &catalogm.Artist{Name: "Errors"}
	s.Require().NoError(s.db.Create(a).Error)

	sw, _, _ := s.newTestSweep(50)
	sw.enrichPhotos = func(_ context.Context, _ []uint) error { return errors.New("boom") }

	sw.RunSweepNow(context.Background())

	// Stamp-before-enrich: marked attempted despite the enrich error, so a poison
	// row can't be re-hammered every tick.
	s.NotNil(s.reloadArtist(a).ImageEnrichAttemptedAt)
}

func (s *ImageEnrichSweepIntegrationTestSuite) TestBatchLimit() {
	for i := 0; i < 5; i++ {
		s.Require().NoError(s.db.Create(&catalogm.Artist{Name: fmt.Sprintf("A%d", i)}).Error)
	}
	sw, gotPhotos, _ := s.newTestSweep(2)
	sw.RunSweepNow(context.Background())
	s.Len(*gotPhotos, 2)
}

func (s *ImageEnrichSweepIntegrationTestSuite) TestSelectsCoverlessReleases() {
	withCover := &catalogm.Release{Title: "Has Cover", CoverArtURL: sweepStrPtr("https://img/c.jpg")}
	without := &catalogm.Release{Title: "No Cover"}
	s.Require().NoError(s.db.Create(withCover).Error)
	s.Require().NoError(s.db.Create(without).Error)

	sw, _, gotCovers := s.newTestSweep(50)
	sw.RunSweepNow(context.Background())

	s.Equal([]uint{without.ID}, *gotCovers)
	s.NotNil(s.reloadRelease(without).ImageEnrichAttemptedAt)
	s.Nil(s.reloadRelease(withCover).ImageEnrichAttemptedAt)
}

func (s *ImageEnrichSweepIntegrationTestSuite) reloadArtist(a *catalogm.Artist) *catalogm.Artist {
	var out catalogm.Artist
	s.Require().NoError(s.db.First(&out, a.ID).Error)
	return &out
}

func (s *ImageEnrichSweepIntegrationTestSuite) reloadRelease(r *catalogm.Release) *catalogm.Release {
	var out catalogm.Release
	s.Require().NoError(s.db.First(&out, r.ID).Error)
	return &out
}

// TestRunCycleStopsOnCanceledContext: a canceled ctx fails the photo selection
// AND runCycle's ctx.Err() guard skips the covers sweep — neither enricher runs.
func (s *ImageEnrichSweepIntegrationTestSuite) TestRunCycleStopsOnCanceledContext() {
	s.Require().NoError(s.db.Create(&catalogm.Artist{Name: "A"}).Error)
	s.Require().NoError(s.db.Create(&catalogm.Release{Title: "R"}).Error)

	sw, gotPhotos, gotCovers := s.newTestSweep(50)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sw.RunSweepNow(ctx)

	s.Empty(*gotPhotos)
	s.Empty(*gotCovers, "covers sweep must be skipped after the ctx is canceled")
}

// TestStampsCoversBeforeEnrichEvenOnError mirrors the photo error-path test for
// the releases/covers side (the stamp logic is shared, but assert both halves).
func (s *ImageEnrichSweepIntegrationTestSuite) TestStampsCoversBeforeEnrichEvenOnError() {
	r := &catalogm.Release{Title: "Errs"}
	s.Require().NoError(s.db.Create(r).Error)

	sw, _, _ := s.newTestSweep(50)
	sw.enrichCovers = func(_ context.Context, _ []uint) error { return errors.New("boom") }

	sw.RunSweepNow(context.Background())
	s.NotNil(s.reloadRelease(r).ImageEnrichAttemptedAt)
}

// TestTreatsEmptyStringImageAsMissing: an empty-string image column (not just
// NULL) counts as image-less and is swept.
func (s *ImageEnrichSweepIntegrationTestSuite) TestTreatsEmptyStringImageAsMissing() {
	empty := &catalogm.Artist{Name: "Empty", ImageURL: sweepStrPtr("")}
	s.Require().NoError(s.db.Create(empty).Error)

	sw, gotPhotos, _ := s.newTestSweep(50)
	sw.RunSweepNow(context.Background())
	s.Equal([]uint{empty.ID}, *gotPhotos)
}

func sweepStrPtr(v string) *string { return &v }
