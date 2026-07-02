package enrich

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
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
	got, err := store.ReleaseLinkCandidates(0)
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
	got, err := store.ReleaseLinkCandidates(2)
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
