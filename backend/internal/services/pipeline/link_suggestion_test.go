package pipeline

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// ──────────────────────────────────────────────
// Integration tests — exercise the list ordering, accept (link write +
// Bandcamp resolver), reject, idempotency/conflict, and the unique constraint
// against a real Postgres database.
// ──────────────────────────────────────────────

// minimal page served for any Bandcamp profile fetch — a discography grid with a
// single /album link (the layout the resolver's primary extractor anchors on) so
// it picks a deterministic embed URL.
const bandcampFixtureHTML = `<!DOCTYPE html><html><head></head><body>
<ol id="music-grid">
<li><a href="/album/the-one">The One</a></li>
</ol>
</body></html>`

// fixtureResolvedEmbed is the URL the resolver produces for boris.bandcamp.com
// from the fixture above (profile host + the single /album path).
const fixtureResolvedEmbed = "https://boris.bandcamp.com/album/the-one"

// rewriteHostRoundTripper routes any outbound request to the test server while
// leaving the request URL's host (boris.bandcamp.com) intact so the resolver's
// SSRF host-anchor still sees a *.bandcamp.com host.
type rewriteHostRoundTripper struct {
	target *url.URL
	rt     http.RoundTripper
}

func (r *rewriteHostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	resp, err := r.rt.RoundTrip(clone)
	if resp != nil {
		// Restore the ORIGINAL *.bandcamp.com request URL so the resolver's
		// final-URL host anchor (buildEmbed) sees the bandcamp host it dialed
		// logically, not the 127.0.0.1 test-server host we physically routed to.
		resp.Request = req
	}
	return resp, err
}

type LinkSuggestionIntegrationSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	artistService *catalog.ArtistService
	service       *LinkSuggestionService
	server        *httptest.Server
}

func (s *LinkSuggestionIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bandcampFixtureHTML))
	}))
	target, _ := url.Parse(s.server.URL)

	// Real artist service with the resolver pointed at the fixture server, run
	// inline (SetSyncDispatch) so the accept assertions don't race a goroutine.
	s.artistService = catalog.NewArtistService(s.db)
	resolver := catalog.NewBandcampProfileResolverWithClient(&http.Client{
		Transport: &rewriteHostRoundTripper{target: target, rt: http.DefaultTransport},
	})
	s.artistService.SetBandcampResolver(resolver)
	s.artistService.SetSyncDispatch()

	s.service = NewLinkSuggestionService(s.db, s.artistService)
}

func (s *LinkSuggestionIntegrationSuite) TearDownSuite() {
	s.server.Close()
	s.testDB.Cleanup()
}

func (s *LinkSuggestionIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM artist_link_suggestions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestLinkSuggestionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(LinkSuggestionIntegrationSuite))
}

// ──────────────────────────────────────────────
// Seed helpers
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) seedArtist(name string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

// seedReviewer inserts a user with the given ID so the reviewed_by_user_id FK is
// satisfied. The accept/reject path stamps a real admin's ID in production; the
// FK guards against a stale/non-existent reviewer, so the test must seed one.
func (s *LinkSuggestionIntegrationSuite) seedReviewer(id uint) {
	email := "reviewer" + string(rune('a'+id%26)) + "@example.com"
	u := &authm.User{ID: id, Email: &email, IsAdmin: true}
	s.Require().NoError(s.db.Create(u).Error)
}

func (s *LinkSuggestionIntegrationSuite) seedSuggestion(artistID uint, platform, urlStr, confidence string) *catalogm.ArtistLinkSuggestion {
	sug := &catalogm.ArtistLinkSuggestion{
		ArtistID:   artistID,
		Platform:   platform,
		URL:        urlStr,
		Source:     catalogm.LinkSuggestionSourceMusicBrainz,
		Confidence: confidence,
		Status:     catalogm.LinkSuggestionStatusPending,
	}
	s.Require().NoError(s.db.Create(sug).Error)
	return sug
}

func (s *LinkSuggestionIntegrationSuite) reloadSuggestion(id uint) *catalogm.ArtistLinkSuggestion {
	var sug catalogm.ArtistLinkSuggestion
	s.Require().NoError(s.db.First(&sug, id).Error)
	return &sug
}

func (s *LinkSuggestionIntegrationSuite) reloadArtist(id uint) *catalogm.Artist {
	var a catalogm.Artist
	s.Require().NoError(s.db.First(&a, id).Error)
	return &a
}

// ──────────────────────────────────────────────
// List
// ──────────────────────────────────────────────

// High-confidence rows sort before review-tier rows; only pending rows appear.
func (s *LinkSuggestionIntegrationSuite) TestListOrdersHighConfidenceFirst() {
	a1 := s.seedArtist("Alpha")
	a2 := s.seedArtist("Beta")
	a3 := s.seedArtist("Gamma")

	// Insert a review-tier row FIRST (lower id) so a naive id-ASC order would put
	// it ahead of the high row — the CASE ordering must override that.
	reviewSug := s.seedSuggestion(a1.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/r1", contracts.MusicConfidenceReview)
	highSug := s.seedSuggestion(a2.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/h1", contracts.MusicConfidenceHigh)

	// A non-pending row must be excluded from the list entirely.
	accepted := s.seedSuggestion(a3.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/x1", contracts.MusicConfidenceHigh)
	s.Require().NoError(s.db.Model(&catalogm.ArtistLinkSuggestion{}).Where("id = ?", accepted.ID).Update("status", catalogm.LinkSuggestionStatusAccepted).Error)

	result, err := s.service.ListPendingSuggestions(50, 0)
	s.Require().NoError(err)
	s.Require().Len(result.Suggestions, 2)
	s.Equal(int64(2), result.Total)

	// High first, despite its higher id.
	s.Equal(highSug.ID, result.Suggestions[0].ID)
	s.Equal(contracts.MusicConfidenceHigh, result.Suggestions[0].Confidence)
	s.Equal(reviewSug.ID, result.Suggestions[1].ID)

	// Artist join populated.
	s.Equal("Beta", result.Suggestions[0].ArtistName)
	s.Require().NotNil(result.Suggestions[0].ArtistSlug)
	s.Equal("Beta", *result.Suggestions[0].ArtistSlug)
}

func (s *LinkSuggestionIntegrationSuite) TestListPagination() {
	a := s.seedArtist("Paginated")
	for i := 0; i < 3; i++ {
		s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/p"+string(rune('a'+i)), contracts.MusicConfidenceHigh)
	}

	page1, err := s.service.ListPendingSuggestions(2, 0)
	s.Require().NoError(err)
	s.Len(page1.Suggestions, 2)
	s.Equal(int64(3), page1.Total)

	page2, err := s.service.ListPendingSuggestions(2, 2)
	s.Require().NoError(err)
	s.Len(page2.Suggestions, 1)
	s.Equal(int64(3), page2.Total)
}

// ──────────────────────────────────────────────
// Accept — Spotify
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) TestAcceptSpotifyWritesLink() {
	s.seedReviewer(7)
	a := s.seedArtist("SpotifyArtist")
	spotifyURL := "https://open.spotify.com/artist/abc123"
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, spotifyURL, contracts.MusicConfidenceHigh)

	res, err := s.service.AcceptSuggestion(sug.ID, 7)
	s.Require().NoError(err)
	s.Equal(catalogm.LinkSuggestionStatusAccepted, res.Status)
	s.Require().NotNil(res.ReviewedByUserID)
	s.Equal(uint(7), *res.ReviewedByUserID)
	s.Require().NotNil(res.ReviewedAt)

	// Artist got the spotify social link.
	got := s.reloadArtist(a.ID)
	s.Require().NotNil(got.Social.Spotify)
	s.Equal(spotifyURL, *got.Social.Spotify)

	// Row marked accepted + stamped.
	row := s.reloadSuggestion(sug.ID)
	s.Equal(catalogm.LinkSuggestionStatusAccepted, row.Status)
	s.Require().NotNil(row.ReviewedByUserID)
	s.Equal(uint(7), *row.ReviewedByUserID)
	s.Require().NotNil(row.ReviewedAt)
}

// ──────────────────────────────────────────────
// Accept — Bandcamp (resolver fires)
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) TestAcceptBandcampWritesLinkAndResolvesEmbed() {
	s.seedReviewer(9)
	a := s.seedArtist("Boris")
	profileURL := "https://boris.bandcamp.com"
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformBandcamp, profileURL, contracts.MusicConfidenceHigh)

	res, err := s.service.AcceptSuggestion(sug.ID, 9)
	s.Require().NoError(err)
	s.Equal(catalogm.LinkSuggestionStatusAccepted, res.Status)

	got := s.reloadArtist(a.ID)
	// Social bandcamp profile stored.
	s.Require().NotNil(got.Social.Bandcamp)
	s.Equal(profileURL, *got.Social.Bandcamp)
	// PSY-1190 resolver filled the embed (inline dispatch) stamped profile_resolved.
	s.Require().NotNil(got.BandcampEmbedURL)
	s.Equal(fixtureResolvedEmbed, *got.BandcampEmbedURL)
	s.Require().NotNil(got.BandcampEmbedSource)
	s.Equal(catalogm.BandcampEmbedSourceProfileResolved, *got.BandcampEmbedSource)
}

// ──────────────────────────────────────────────
// Reject
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) TestRejectMarksRejectedWithoutWritingLink() {
	s.seedReviewer(3)
	a := s.seedArtist("RejectArtist")
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/rej", contracts.MusicConfidenceReview)

	res, err := s.service.RejectSuggestion(sug.ID, 3)
	s.Require().NoError(err)
	s.Equal(catalogm.LinkSuggestionStatusRejected, res.Status)
	s.Require().NotNil(res.ReviewedByUserID)
	s.Equal(uint(3), *res.ReviewedByUserID)

	// Artist link NOT written.
	got := s.reloadArtist(a.ID)
	s.Nil(got.Social.Spotify)

	row := s.reloadSuggestion(sug.ID)
	s.Equal(catalogm.LinkSuggestionStatusRejected, row.Status)
}

// ──────────────────────────────────────────────
// Idempotency + conflicting verdict
// ──────────────────────────────────────────────

// Accepting an already-accepted row returns the stored stamp WITHOUT re-writing.
func (s *LinkSuggestionIntegrationSuite) TestAcceptIsIdempotent() {
	s.seedReviewer(11)
	s.seedReviewer(99)
	a := s.seedArtist("IdemArtist")
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/idem", contracts.MusicConfidenceHigh)

	first, err := s.service.AcceptSuggestion(sug.ID, 11)
	s.Require().NoError(err)

	// Mutate the artist link out from under the row to prove the second accept
	// does NOT re-write it.
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", a.ID).Update("spotify", "https://open.spotify.com/artist/CHANGED").Error)

	second, err := s.service.AcceptSuggestion(sug.ID, 99)
	s.Require().NoError(err)
	s.Equal(catalogm.LinkSuggestionStatusAccepted, second.Status)
	// Reviewer preserved from the FIRST accept (not overwritten by 99).
	s.Require().NotNil(second.ReviewedByUserID)
	s.Equal(uint(11), *second.ReviewedByUserID)
	s.Equal(*first.ReviewedByUserID, *second.ReviewedByUserID)

	// Link was NOT re-written (still the CHANGED value).
	got := s.reloadArtist(a.ID)
	s.Require().NotNil(got.Social.Spotify)
	s.Equal("https://open.spotify.com/artist/CHANGED", *got.Social.Spotify)
}

func (s *LinkSuggestionIntegrationSuite) TestRejectIsIdempotent() {
	s.seedReviewer(5)
	s.seedReviewer(88)
	a := s.seedArtist("IdemReject")
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/idemr", contracts.MusicConfidenceHigh)

	first, err := s.service.RejectSuggestion(sug.ID, 5)
	s.Require().NoError(err)

	second, err := s.service.RejectSuggestion(sug.ID, 88)
	s.Require().NoError(err)
	s.Equal(catalogm.LinkSuggestionStatusRejected, second.Status)
	s.Equal(*first.ReviewedByUserID, *second.ReviewedByUserID)
}

// Accepting a rejected row (and vice versa) is a conflicting verdict → error.
func (s *LinkSuggestionIntegrationSuite) TestAcceptAfterRejectConflicts() {
	s.seedReviewer(1)
	s.seedReviewer(2)
	a := s.seedArtist("ConflictArtist")
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/conf", contracts.MusicConfidenceHigh)

	_, err := s.service.RejectSuggestion(sug.ID, 1)
	s.Require().NoError(err)

	_, err = s.service.AcceptSuggestion(sug.ID, 2)
	s.Require().ErrorIs(err, contracts.ErrLinkSuggestionAlreadyReviewed)

	// Still rejected; no link written.
	row := s.reloadSuggestion(sug.ID)
	s.Equal(catalogm.LinkSuggestionStatusRejected, row.Status)
	s.Nil(s.reloadArtist(a.ID).Social.Spotify)
}

func (s *LinkSuggestionIntegrationSuite) TestRejectAfterAcceptConflicts() {
	s.seedReviewer(1)
	s.seedReviewer(2)
	a := s.seedArtist("ConflictArtist2")
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/conf2", contracts.MusicConfidenceHigh)

	_, err := s.service.AcceptSuggestion(sug.ID, 1)
	s.Require().NoError(err)

	_, err = s.service.RejectSuggestion(sug.ID, 2)
	s.Require().ErrorIs(err, contracts.ErrLinkSuggestionAlreadyReviewed)
}

// ──────────────────────────────────────────────
// Not found
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) TestAcceptNotFound() {
	_, err := s.service.AcceptSuggestion(999999, 1)
	s.Require().ErrorIs(err, contracts.ErrLinkSuggestionNotFound)
}

func (s *LinkSuggestionIntegrationSuite) TestRejectNotFound() {
	_, err := s.service.RejectSuggestion(999999, 1)
	s.Require().ErrorIs(err, contracts.ErrLinkSuggestionNotFound)
}

// ──────────────────────────────────────────────
// Unique constraint — a re-sweep with the same (artist, platform, url) does
// not duplicate, and does NOT resurrect an already-reviewed row.
// ──────────────────────────────────────────────

func (s *LinkSuggestionIntegrationSuite) TestUniqueConstraintRejectsDuplicate() {
	s.seedReviewer(1)
	a := s.seedArtist("DupArtist")
	urlStr := "https://open.spotify.com/artist/dup"
	sug := s.seedSuggestion(a.ID, contracts.MusicPlatformSpotify, urlStr, contracts.MusicConfidenceHigh)

	// Reject it (terminal state).
	_, err := s.service.RejectSuggestion(sug.ID, 1)
	s.Require().NoError(err)

	// A re-sweep inserting the SAME (artist, platform, url) must violate the
	// unique constraint — proving the rejected row is not silently re-created as
	// a fresh pending row.
	dup := &catalogm.ArtistLinkSuggestion{
		ArtistID:   a.ID,
		Platform:   contracts.MusicPlatformSpotify,
		URL:        urlStr,
		Source:     catalogm.LinkSuggestionSourceMusicBrainz,
		Confidence: contracts.MusicConfidenceHigh,
		Status:     catalogm.LinkSuggestionStatusPending,
	}
	err = s.db.Create(dup).Error
	s.Require().Error(err, "re-inserting the same (artist, platform, url) must violate the unique constraint")

	// The sweep's idempotent insert (ON CONFLICT DO NOTHING) is a no-op: still
	// exactly one row, still rejected.
	upsertErr := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&catalogm.ArtistLinkSuggestion{
		ArtistID:   a.ID,
		Platform:   contracts.MusicPlatformSpotify,
		URL:        urlStr,
		Source:     catalogm.LinkSuggestionSourceMusicBrainz,
		Confidence: contracts.MusicConfidenceHigh,
		Status:     catalogm.LinkSuggestionStatusPending,
	}).Error
	s.Require().NoError(upsertErr)

	var count int64
	s.Require().NoError(s.db.Model(&catalogm.ArtistLinkSuggestion{}).Where("artist_id = ?", a.ID).Count(&count).Error)
	s.Equal(int64(1), count)
	s.Equal(catalogm.LinkSuggestionStatusRejected, s.reloadSuggestion(sug.ID).Status)
}
