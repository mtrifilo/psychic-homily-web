package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// TagFilterIntegrationTestSuite exercises the multi-tag filter
// (PSY-309) against every browse-list method — one suite covers all
// six entity types to keep setup cost (testcontainer boot) amortized.
type TagFilterIntegrationTestSuite struct {
	suite.Suite
	testDB          *testutil.TestDatabase
	db              *gorm.DB
	artistService   *ArtistService
	showService     *ShowService
	venueService    *VenueService
	releaseService  *ReleaseService
	labelService    *LabelService
	festivalService *FestivalService
	tagService      *TagService

	user *models.User
	// Pre-seeded tags keyed by slug.
	tags map[string]*models.Tag
}

func (s *TagFilterIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.artistService = &ArtistService{db: s.db}
	s.showService = &ShowService{db: s.db}
	s.venueService = &VenueService{db: s.db}
	s.releaseService = NewReleaseService(s.db)
	s.labelService = NewLabelService(s.db)
	s.festivalService = NewFestivalService(s.db)
	s.tagService = NewTagService(s.db)
}

func (s *TagFilterIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// SetupTest truncates everything and re-seeds a user and the canonical
// tag set used across the tag-filter tests (post-punk, shoegaze,
// phoenix, electronic). Each per-entity test then applies these tags
// to its own rows.
func (s *TagFilterIntegrationTestSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// FK-safe truncate; order mirrors existing suites.
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")

	email := fmt.Sprintf("tf-user-%d@test.com", time.Now().UnixNano())
	name := "TagFilter"
	u := &models.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	s.user = u

	s.tags = map[string]*models.Tag{}
	for _, name := range []string{"post-punk", "shoegaze", "phoenix", "electronic"} {
		cat := "genre"
		if name == "phoenix" {
			cat = "locale"
		}
		t, err := s.tagService.CreateTag(name, nil, nil, cat, false, nil)
		s.Require().NoError(err)
		s.tags[t.Slug] = t
	}
}

func TestTagFilterIntegrationSuite(t *testing.T) {
	suite.Run(t, new(TagFilterIntegrationTestSuite))
}

// tag applies a tag (by slug) to an entity directly via the raw
// junction table so we avoid the inline-create / permission flow
// and stay focused on the filter logic.
func (s *TagFilterIntegrationTestSuite) tag(entityType string, entityID uint, slug string) {
	tag, ok := s.tags[slug]
	s.Require().Truef(ok, "unseeded tag %q", slug)
	s.Require().NoError(s.db.Create(&models.EntityTag{
		TagID:         tag.ID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: s.user.ID,
	}).Error)
}

// ──────────────────────────────────────────────
// Artist tests (uses GetArtistsWithShowCounts because that's the /artists browse)
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedArtistWithUpcoming(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &models.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)

	v := &models.Venue{Name: fmt.Sprintf("V-%s", slug), City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(v).Error)

	show := &models.Show{
		Title:       fmt.Sprintf("Show-%s", slug),
		EventDate:   time.Now().Add(7 * 24 * time.Hour).UTC(),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: a.ID, Position: 0}).Error)
	s.Require().NoError(s.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
	return a.ID
}

func (s *TagFilterIntegrationTestSuite) TestArtists_SingleTag() {
	a1 := s.seedArtistWithUpcoming("The Tagged One")
	a2 := s.seedArtistWithUpcoming("Not Tagged")
	s.tag("artist", a1, "post-punk")
	_ = a2

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk"}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("The Tagged One", resp[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestArtists_TwoTagAND() {
	a1 := s.seedArtistWithUpcoming("Both")
	a2 := s.seedArtistWithUpcoming("OnlyOne")
	a3 := s.seedArtistWithUpcoming("None")
	s.tag("artist", a1, "post-punk")
	s.tag("artist", a1, "phoenix")
	s.tag("artist", a2, "post-punk")
	_ = a3

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("Both", resp[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestArtists_ThreeTagAND() {
	a1 := s.seedArtistWithUpcoming("All Three")
	a2 := s.seedArtistWithUpcoming("Two Only")
	s.tag("artist", a1, "post-punk")
	s.tag("artist", a1, "shoegaze")
	s.tag("artist", a1, "phoenix")
	s.tag("artist", a2, "post-punk")
	s.tag("artist", a2, "shoegaze")

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "shoegaze", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("All Three", resp[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestArtists_OR() {
	a1 := s.seedArtistWithUpcoming("PP")
	a2 := s.seedArtistWithUpcoming("SG")
	a3 := s.seedArtistWithUpcoming("Neither")
	s.tag("artist", a1, "post-punk")
	s.tag("artist", a2, "shoegaze")
	_ = a3

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "shoegaze"}, MatchAny: true},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 2)
}

func (s *TagFilterIntegrationTestSuite) TestArtists_EmptyIsNoop() {
	s.seedArtistWithUpcoming("One")
	s.seedArtistWithUpcoming("Two")

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{})
	s.Require().NoError(err)
	s.Len(resp, 2)
}

func (s *TagFilterIntegrationTestSuite) TestArtists_UnknownTagReturnsEmpty() {
	s.seedArtistWithUpcoming("Any")

	resp, err := s.artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"does-not-exist"}},
	})
	s.Require().NoError(err)
	s.Len(resp, 0)
}

// ──────────────────────────────────────────────
// Show tests (GetShows / GetUpcomingShows)
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedShow(title string, eventDate time.Time) uint {
	show := &models.Show{
		Title:       title,
		EventDate:   eventDate,
		Status:      models.ShowStatusApproved,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show.ID
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetShows_AND() {
	sA := s.seedShow("A", time.Now().Add(24*time.Hour).UTC())
	sB := s.seedShow("B", time.Now().Add(24*time.Hour).UTC())
	s.tag("show", sA, "post-punk")
	s.tag("show", sA, "phoenix")
	s.tag("show", sB, "post-punk")

	resp, err := s.showService.GetShows(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("A", resp[0].Title)
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetUpcomingShows_AND() {
	sA := s.seedShow("Upcoming-A", time.Now().Add(2*24*time.Hour).UTC())
	sB := s.seedShow("Upcoming-B", time.Now().Add(2*24*time.Hour).UTC())
	s.tag("show", sA, "post-punk")
	s.tag("show", sA, "shoegaze")
	s.tag("show", sB, "post-punk")

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs: []string{"post-punk", "shoegaze"},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("Upcoming-A", resp[0].Title)
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetUpcomingShows_OR() {
	sA := s.seedShow("OR-A", time.Now().Add(2*24*time.Hour).UTC())
	sB := s.seedShow("OR-B", time.Now().Add(2*24*time.Hour).UTC())
	sC := s.seedShow("OR-C", time.Now().Add(2*24*time.Hour).UTC())
	s.tag("show", sA, "post-punk")
	s.tag("show", sB, "shoegaze")
	_ = sC

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs:    []string{"post-punk", "shoegaze"},
		TagMatchAny: true,
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 2)
}

// ──────────────────────────────────────────────
// Venue tests (GetVenuesWithShowCounts is the /venues browse)
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedVerifiedVenue(name string) uint {
	v := &models.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: true}
	s.Require().NoError(s.db.Create(v).Error)
	return v.ID
}

func (s *TagFilterIntegrationTestSuite) TestVenues_AND() {
	v1 := s.seedVerifiedVenue("V1")
	v2 := s.seedVerifiedVenue("V2")
	s.tag("venue", v1, "post-punk")
	s.tag("venue", v1, "phoenix")
	s.tag("venue", v2, "post-punk")

	resp, total, err := s.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{
		TagSlugs: []string{"post-punk", "phoenix"},
	}, 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(resp, 1)
	s.Equal("V1", resp[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestVenues_OR() {
	v1 := s.seedVerifiedVenue("V1")
	v2 := s.seedVerifiedVenue("V2")
	v3 := s.seedVerifiedVenue("V3")
	s.tag("venue", v1, "post-punk")
	s.tag("venue", v2, "shoegaze")
	_ = v3

	resp, total, err := s.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{
		TagSlugs:    []string{"post-punk", "shoegaze"},
		TagMatchAny: true,
	}, 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(2), total)
	s.Require().Len(resp, 2)
}

// ──────────────────────────────────────────────
// Release tests
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedRelease(title string) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	r := &models.Release{Title: title, Slug: &slug}
	s.Require().NoError(s.db.Create(r).Error)
	return r.ID
}

func (s *TagFilterIntegrationTestSuite) TestReleases_AND() {
	r1 := s.seedRelease("Rel1")
	r2 := s.seedRelease("Rel2")
	s.tag("release", r1, "shoegaze")
	s.tag("release", r1, "electronic")
	s.tag("release", r2, "shoegaze")

	out, total, err := s.releaseService.ListReleases(contracts.ReleaseListFilters{
		TagSlugs: []string{"shoegaze", "electronic"},
	})
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(out, 1)
	s.Equal("Rel1", out[0].Title)
}

func (s *TagFilterIntegrationTestSuite) TestReleases_OR() {
	r1 := s.seedRelease("Rel1")
	r2 := s.seedRelease("Rel2")
	r3 := s.seedRelease("Rel3")
	s.tag("release", r1, "shoegaze")
	s.tag("release", r2, "electronic")
	_ = r3

	out, total, err := s.releaseService.ListReleases(contracts.ReleaseListFilters{
		TagSlugs:    []string{"shoegaze", "electronic"},
		TagMatchAny: true,
	})
	s.Require().NoError(err)
	s.Equal(int64(2), total)
	s.Len(out, 2)
}

// ──────────────────────────────────────────────
// Label tests
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedLabel(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	l := &models.Label{Name: name, Slug: &slug, Status: models.LabelStatusActive}
	s.Require().NoError(s.db.Create(l).Error)
	return l.ID
}

func (s *TagFilterIntegrationTestSuite) TestLabels_AND() {
	l1 := s.seedLabel("Lab1")
	l2 := s.seedLabel("Lab2")
	s.tag("label", l1, "post-punk")
	s.tag("label", l1, "phoenix")
	s.tag("label", l2, "post-punk")

	out, err := s.labelService.ListLabels(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.Equal("Lab1", out[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestLabels_OR() {
	l1 := s.seedLabel("Lab1")
	l2 := s.seedLabel("Lab2")
	l3 := s.seedLabel("Lab3")
	s.tag("label", l1, "post-punk")
	s.tag("label", l2, "shoegaze")
	_ = l3

	out, err := s.labelService.ListLabels(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "shoegaze"}, MatchAny: true},
	})
	s.Require().NoError(err)
	s.Len(out, 2)
}

// ──────────────────────────────────────────────
// Festival tests
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedFestival(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	f := &models.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug + "-series",
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
		Status:      models.FestivalStatusConfirmed,
	}
	s.Require().NoError(s.db.Create(f).Error)
	return f.ID
}

func (s *TagFilterIntegrationTestSuite) TestFestivals_AND() {
	f1 := s.seedFestival("Fest1")
	f2 := s.seedFestival("Fest2")
	s.tag("festival", f1, "electronic")
	s.tag("festival", f1, "phoenix")
	s.tag("festival", f2, "electronic")

	out, err := s.festivalService.ListFestivals(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"electronic", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.Equal("Fest1", out[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestFestivals_OR() {
	f1 := s.seedFestival("Fest1")
	f2 := s.seedFestival("Fest2")
	f3 := s.seedFestival("Fest3")
	s.tag("festival", f1, "electronic")
	s.tag("festival", f2, "shoegaze")
	_ = f3

	out, err := s.festivalService.ListFestivals(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"electronic", "shoegaze"}, MatchAny: true},
	})
	s.Require().NoError(err)
	s.Len(out, 2)
}
