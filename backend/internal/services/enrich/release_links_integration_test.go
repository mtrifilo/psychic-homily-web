package enrich

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// ReleaseLinksIntegrationTestSuite exercises the PSY-1307 gorm store against
// real Postgres — the per-platform NOT EXISTS candidate filter, the
// limit-caps-candidates property, and the pre-write re-check all live in SQL
// that unit fakes can't validate (mirrors ArtistLinksSweepIntegrationTestSuite).
type ReleaseLinksIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ReleaseLinksIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ReleaseLinksIntegrationTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ReleaseLinksIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM release_external_links")
	_, _ = sqlDB.Exec("DELETE FROM releases")
}

func TestReleaseLinksIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ReleaseLinksIntegrationTestSuite))
}

func (s *ReleaseLinksIntegrationTestSuite) seedRelease(title, slug string, rgMBID *string, links ...catalogm.ReleaseExternalLink) *catalogm.Release {
	r := &catalogm.Release{Title: title, Slug: &slug, ReleaseType: catalogm.ReleaseTypeLP, MusicBrainzReleaseGroupID: rgMBID}
	s.Require().NoError(s.db.Create(r).Error)
	for i := range links {
		links[i].ReleaseID = r.ID
		s.Require().NoError(s.db.Create(&links[i]).Error)
	}
	return r
}

func rgptr(i byte) *string {
	c := string('0' + i)
	mbid := c + "1111111-1111-1111-1111-111111111111"
	return &mbid
}

func (s *ReleaseLinksIntegrationTestSuite) TestCandidateSelection() {
	// eligible: RG-MBID + no links at all
	needsBoth := s.seedRelease("Needs Both", "needs-both", rgptr(1))
	// eligible: RG-MBID + only bandcamp — spotify still missing
	partial := s.seedRelease("Partial", "partial", rgptr(2),
		catalogm.ReleaseExternalLink{Platform: "bandcamp", URL: "https://x.bandcamp.com/album/a"})
	// NOT eligible: both platforms present — including a MIXED-CASE platform row
	// (the API stores platform strings verbatim; LOWER() in SQL must still match)
	s.seedRelease("Complete Mixed Case", "complete", rgptr(3),
		catalogm.ReleaseExternalLink{Platform: "Bandcamp", URL: "https://x.bandcamp.com/album/b"},
		catalogm.ReleaseExternalLink{Platform: "SPOTIFY", URL: "https://open.spotify.com/album/c"})
	// NOT eligible: no RG-MBID
	s.seedRelease("No MBID", "no-mbid", nil)
	// NOT eligible: whitespace-only RG-MBID (TRIM filter)
	blank := "   "
	s.seedRelease("Blank MBID", "blank-mbid", &blank)
	// eligible: a non-platform link doesn't satisfy the filter
	otherLink := s.seedRelease("Other Link Only", "other-link", rgptr(4),
		catalogm.ReleaseExternalLink{Platform: "discogs", URL: "https://www.discogs.com/release/1"})

	store := &gormReleaseLinkStore{db: s.db}
	got, err := store.ReleaseLinkCandidates(0, nil)
	s.Require().NoError(err)

	byID := map[uint]releaseLinkCandidate{}
	for _, c := range got {
		byID[c.release.ID] = c
	}
	s.Require().Len(got, 3, "needs-both, partial, other-link-only — nothing else")

	s.Require().Contains(byID, needsBoth.ID)
	s.False(byID[needsBoth.ID].hasBandcamp)
	s.False(byID[needsBoth.ID].hasSpotify)

	s.Require().Contains(byID, partial.ID)
	s.True(byID[partial.ID].hasBandcamp, "existing bandcamp link detected from the same snapshot")
	s.False(byID[partial.ID].hasSpotify)

	s.Require().Contains(byID, otherLink.ID)
	s.False(byID[otherLink.ID].hasBandcamp, "a discogs link is not a platform link")
}

// The round-1 HIGH fix: limit caps CANDIDATES, not raw RG-MBID rows — a fully
// linked release early in id-order must not consume the window.
func (s *ReleaseLinksIntegrationTestSuite) TestLimitCapsCandidatesNotRows() {
	// lowest ids: two COMPLETE releases (would exhaust a naive LIMIT 2)
	s.seedRelease("Complete 1", "c1", rgptr(1),
		catalogm.ReleaseExternalLink{Platform: "bandcamp", URL: "https://x.bandcamp.com/album/a"},
		catalogm.ReleaseExternalLink{Platform: "spotify", URL: "https://open.spotify.com/album/a"})
	s.seedRelease("Complete 2", "c2", rgptr(2),
		catalogm.ReleaseExternalLink{Platform: "bandcamp", URL: "https://x.bandcamp.com/album/b"},
		catalogm.ReleaseExternalLink{Platform: "spotify", URL: "https://open.spotify.com/album/b"})
	// higher ids: two real candidates
	cand1 := s.seedRelease("Candidate 1", "cand1", rgptr(3))
	cand2 := s.seedRelease("Candidate 2", "cand2", rgptr(4))

	store := &gormReleaseLinkStore{db: s.db}
	got, err := store.ReleaseLinkCandidates(2, nil)
	s.Require().NoError(err)
	s.Require().Len(got, 2, "limit window holds candidates, not already-complete rows")
	s.Equal(cand1.ID, got[0].release.ID)
	s.Equal(cand2.ID, got[1].release.ID)
}

func (s *ReleaseLinksIntegrationTestSuite) TestReleaseHasPlatformLink() {
	r := s.seedRelease("Recheck", "recheck", rgptr(1),
		catalogm.ReleaseExternalLink{Platform: "Bandcamp", URL: "https://x.bandcamp.com/album/a"})

	store := &gormReleaseLinkStore{db: s.db}

	has, err := store.ReleaseHasPlatformLink(r.ID, contracts.MusicPlatformBandcamp)
	s.Require().NoError(err)
	s.True(has, "mixed-case stored platform still counts (LOWER match)")

	has, err = store.ReleaseHasPlatformLink(r.ID, contracts.MusicPlatformSpotify)
	s.Require().NoError(err)
	s.False(has)
}

// PSY-1316: the sweep-mode memo filter — recently-attempted releases drop out,
// stale/never-attempted stay, ordered NULLs-first.
func (s *ReleaseLinksIntegrationTestSuite) TestCandidatesMemoFilter() {
	recent := time.Now().Add(-1 * time.Hour)
	stale := time.Now().Add(-100 * 24 * time.Hour)

	never := s.seedRelease("Never Tried", "never", rgptr(1))
	tried := s.seedRelease("Recently Tried", "recent", rgptr(2))
	s.Require().NoError(s.db.Table("releases").Where("id = ?", tried.ID).
		Update("links_enrich_attempted_at", recent).Error)
	staleTried := s.seedRelease("Stale Tried", "stale", rgptr(3))
	s.Require().NoError(s.db.Table("releases").Where("id = ?", staleTried.ID).
		Update("links_enrich_attempted_at", stale).Error)

	store := &gormReleaseLinkStore{db: s.db}
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	got, err := store.ReleaseLinkCandidates(0, &cutoff)
	s.Require().NoError(err)

	s.Require().Len(got, 2, "recently-attempted release excluded")
	s.Equal(never.ID, got[0].release.ID, "NULLs first")
	s.Equal(staleTried.ID, got[1].release.ID)
}

func (s *ReleaseLinksIntegrationTestSuite) TestStampLinksAttempted() {
	r := s.seedRelease("Stampable", "stampable", rgptr(1))
	store := &gormReleaseLinkStore{db: s.db}
	at := time.Now()
	s.Require().NoError(store.StampLinksAttempted([]uint{r.ID}, at))

	var got catalogm.Release
	s.Require().NoError(s.db.First(&got, r.ID).Error)
	s.Require().NotNil(got.LinksEnrichAttemptedAt)
	s.WithinDuration(at, *got.LinksEnrichAttemptedAt, time.Second)
}

// PSY-1316: enrichment writes carry source=mb_backfill; the partial unique index
// rejects a second backfill row for the same (release, platform) but leaves
// manual (NULL-source) rows unconstrained.
func (s *ReleaseLinksIntegrationTestSuite) TestSourceStampAndBackfillUniqueIndex() {
	r := s.seedRelease("Sourced", "sourced", rgptr(1))
	svc := catalog.NewReleaseService(s.db)

	_, err := svc.AddExternalLinkWithSource(r.ID, "bandcamp", "https://x.bandcamp.com/album/a", "mb_backfill")
	s.Require().NoError(err)
	var link catalogm.ReleaseExternalLink
	s.Require().NoError(s.db.Where("release_id = ?", r.ID).First(&link).Error)
	s.Require().NotNil(link.Source)
	s.Equal("mb_backfill", *link.Source)

	// second backfill row for the same platform → unique-index dup-key
	_, err = svc.AddExternalLinkWithSource(r.ID, "bandcamp", "https://x.bandcamp.com/album/b", "mb_backfill")
	s.Require().Error(err)
	s.True(shared.IsDuplicateKey(err), "backfill-scoped unique index fires as a duplicate key")

	// manual rows (NULL source) are NOT constrained — two same-platform manual adds succeed
	_, err = svc.AddExternalLink(r.ID, "bandcamp", "https://x.bandcamp.com/album/manual-1")
	s.Require().NoError(err)
	_, err = svc.AddExternalLink(r.ID, "bandcamp", "https://x.bandcamp.com/track/manual-2")
	s.Require().NoError(err, "manual entry stays unconstrained")
}
