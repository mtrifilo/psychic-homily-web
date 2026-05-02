package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
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

	user *authm.User
	// Pre-seeded tags keyed by slug.
	tags map[string]*catalogm.Tag
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
	u := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	s.user = u

	s.tags = map[string]*catalogm.Tag{}
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
	s.Require().NoError(s.db.Create(&catalogm.EntityTag{
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
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)

	v := &catalogm.Venue{Name: fmt.Sprintf("V-%s", slug), City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(v).Error)

	show := &catalogm.Show{
		Title:       fmt.Sprintf("Show-%s", slug),
		EventDate:   time.Now().Add(7 * 24 * time.Hour).UTC(),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: a.ID, Position: 0}).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
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
//
// PSY-499: Shows are filtered transitively via billed artist tags — direct
// `entity_type='show'` tags are no longer honored by the filter because
// nobody manually tags shows with genres. Each test seeds an artist (tagged
// on the `artist` entity type) and a show that includes that artist on the
// bill via `show_artists`, then asserts the filter finds the show.
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedShow(title string, eventDate time.Time) uint {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show.ID
}

// seedArtist creates an artist with a unique slug. Used to build a show
// lineup whose tags drive the transitive filter.
func (s *TagFilterIntegrationTestSuite) seedArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

// addArtistToShow attaches an artist to a show's lineup with the given bill
// position (0 = headliner).
func (s *TagFilterIntegrationTestSuite) addArtistToShow(showID, artistID uint, position int) {
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
		ShowID: showID, ArtistID: artistID, Position: position,
	}).Error)
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetShows_AND_Transitive() {
	// Show A: lineup collectively covers both tags (post-punk + phoenix)
	// Show B: lineup only covers post-punk
	sA := s.seedShow("A", time.Now().Add(24*time.Hour).UTC())
	sB := s.seedShow("B", time.Now().Add(24*time.Hour).UTC())
	aPP := s.seedArtist("ArtistPP")
	aPhx := s.seedArtist("ArtistPhx")
	aPPOnly := s.seedArtist("ArtistPPOnly")
	s.addArtistToShow(sA, aPP, 0)
	s.addArtistToShow(sA, aPhx, 1)
	s.addArtistToShow(sB, aPPOnly, 0)
	s.tag("artist", aPP, "post-punk")
	s.tag("artist", aPhx, "phoenix")
	s.tag("artist", aPPOnly, "post-punk")

	resp, err := s.showService.GetShows(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"post-punk", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("A", resp[0].Title)
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetUpcomingShows_AND_Transitive() {
	// Show Upcoming-A: lineup covers both post-punk + shoegaze
	// Show Upcoming-B: lineup only covers post-punk
	sA := s.seedShow("Upcoming-A", time.Now().Add(2*24*time.Hour).UTC())
	sB := s.seedShow("Upcoming-B", time.Now().Add(2*24*time.Hour).UTC())
	aPP := s.seedArtist("UPP")
	aSG := s.seedArtist("USG")
	aPPOnly := s.seedArtist("UPPOnly")
	s.addArtistToShow(sA, aPP, 0)
	s.addArtistToShow(sA, aSG, 1)
	s.addArtistToShow(sB, aPPOnly, 0)
	s.tag("artist", aPP, "post-punk")
	s.tag("artist", aSG, "shoegaze")
	s.tag("artist", aPPOnly, "post-punk")

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs: []string{"post-punk", "shoegaze"},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("Upcoming-A", resp[0].Title)
}

func (s *TagFilterIntegrationTestSuite) TestShows_GetUpcomingShows_OR_Transitive() {
	// Shows A + B both have one matching artist each; C has none.
	sA := s.seedShow("OR-A", time.Now().Add(2*24*time.Hour).UTC())
	sB := s.seedShow("OR-B", time.Now().Add(2*24*time.Hour).UTC())
	sC := s.seedShow("OR-C", time.Now().Add(2*24*time.Hour).UTC())
	aPP := s.seedArtist("ORPP")
	aSG := s.seedArtist("ORSG")
	aNone := s.seedArtist("ORNone")
	s.addArtistToShow(sA, aPP, 0)
	s.addArtistToShow(sB, aSG, 0)
	s.addArtistToShow(sC, aNone, 0)
	s.tag("artist", aPP, "post-punk")
	s.tag("artist", aSG, "shoegaze")

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs:    []string{"post-punk", "shoegaze"},
		TagMatchAny: true,
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 2)
}

// TestShows_SingleTag_Transitive is the canonical PSY-499 scenario: a single
// genre tag on an artist surfaces every show that artist is billed on, even
// when the show itself has no direct `entity_type='show'` tag. This mirrors
// the dogfood repro: `/shows?tags=shoegaze` should return the 3 Faetooth
// shows when Faetooth (an artist) is tagged `shoegaze`.
func (s *TagFilterIntegrationTestSuite) TestShows_SingleTag_Transitive() {
	sA := s.seedShow("Shoegaze-A", time.Now().Add(2*24*time.Hour).UTC())
	sB := s.seedShow("Shoegaze-B", time.Now().Add(3*24*time.Hour).UTC())
	sC := s.seedShow("Shoegaze-C", time.Now().Add(4*24*time.Hour).UTC())
	sX := s.seedShow("Other-X", time.Now().Add(2*24*time.Hour).UTC())
	faetooth := s.seedArtist("Faetooth")
	other := s.seedArtist("Other")
	s.addArtistToShow(sA, faetooth, 0)
	s.addArtistToShow(sB, faetooth, 0)
	s.addArtistToShow(sC, faetooth, 0)
	s.addArtistToShow(sX, other, 0)
	s.tag("artist", faetooth, "shoegaze")
	// Intentionally do NOT tag show X directly with anything; filter should
	// still exclude it because the transitive filter ignores show-level tags.

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs: []string{"shoegaze"},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 3)
	titles := map[string]bool{}
	for _, r := range resp {
		titles[r.Title] = true
	}
	s.True(titles["Shoegaze-A"])
	s.True(titles["Shoegaze-B"])
	s.True(titles["Shoegaze-C"])
}

// TestShows_DirectTagNotSufficient verifies the semantics flip: directly
// tagging a show with `entity_type='show'` is intentionally a no-op for the
// filter (PSY-499). Direct show-level tags coexist harmlessly but don't
// drive discovery — only lineup artist tags do.
func (s *TagFilterIntegrationTestSuite) TestShows_DirectTagNotSufficient() {
	sA := s.seedShow("Direct-Only", time.Now().Add(2*24*time.Hour).UTC())
	// Tag the show directly but give it no tagged artists on the lineup.
	aUntagged := s.seedArtist("Untagged")
	s.addArtistToShow(sA, aUntagged, 0)
	s.tag("show", sA, "shoegaze")

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs: []string{"shoegaze"},
	})
	s.Require().NoError(err)
	s.Len(resp, 0)
}

// TestShows_DistinctShowIDs verifies the DISTINCT dedup when a show has
// multiple matching lineup artists. Without DISTINCT the subquery would
// return duplicates, but the outer `IN (?)` clause ignores them — this test
// still asserts a single response row to catch any future refactor that
// accidentally joins duplicates back in via e.g. a LEFT JOIN.
func (s *TagFilterIntegrationTestSuite) TestShows_DistinctShowIDs() {
	sA := s.seedShow("Multi-Shoegaze", time.Now().Add(2*24*time.Hour).UTC())
	a1 := s.seedArtist("SG1")
	a2 := s.seedArtist("SG2")
	a3 := s.seedArtist("SG3")
	s.addArtistToShow(sA, a1, 0)
	s.addArtistToShow(sA, a2, 1)
	s.addArtistToShow(sA, a3, 2)
	s.tag("artist", a1, "shoegaze")
	s.tag("artist", a2, "shoegaze")
	s.tag("artist", a3, "shoegaze")

	resp, _, err := s.showService.GetUpcomingShows("UTC", "", 50, false, &contracts.UpcomingShowsFilter{
		TagSlugs: []string{"shoegaze"},
	})
	s.Require().NoError(err)
	s.Require().Len(resp, 1)
	s.Equal("Multi-Shoegaze", resp[0].Title)
}

// ──────────────────────────────────────────────
// Venue tests (GetVenuesWithShowCounts is the /venues browse)
// ──────────────────────────────────────────────

func (s *TagFilterIntegrationTestSuite) seedVerifiedVenue(name string) uint {
	v := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: true}
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
	r := &catalogm.Release{Title: title, Slug: &slug}
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
	l := &catalogm.Label{Name: name, Slug: &slug, Status: catalogm.LabelStatusActive}
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
	f := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug + "-series",
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
		Status:      catalogm.FestivalStatusConfirmed,
	}
	s.Require().NoError(s.db.Create(f).Error)
	return f.ID
}

// addArtistToFestival attaches an artist to a festival's lineup.
func (s *TagFilterIntegrationTestSuite) addArtistToFestival(festivalID, artistID uint) {
	s.Require().NoError(s.db.Create(&catalogm.FestivalArtist{
		FestivalID: festivalID, ArtistID: artistID,
	}).Error)
}

// PSY-499: Festivals filter transitively through `festival_artists`, mirroring
// the show↔artist pattern.
func (s *TagFilterIntegrationTestSuite) TestFestivals_AND_Transitive() {
	f1 := s.seedFestival("Fest1")
	f2 := s.seedFestival("Fest2")
	aElec := s.seedArtist("FElec")
	aPhx := s.seedArtist("FPhx")
	aElecOnly := s.seedArtist("FElecOnly")
	s.addArtistToFestival(f1, aElec)
	s.addArtistToFestival(f1, aPhx)
	s.addArtistToFestival(f2, aElecOnly)
	s.tag("artist", aElec, "electronic")
	s.tag("artist", aPhx, "phoenix")
	s.tag("artist", aElecOnly, "electronic")

	out, err := s.festivalService.ListFestivals(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"electronic", "phoenix"}},
	})
	s.Require().NoError(err)
	s.Require().Len(out, 1)
	s.Equal("Fest1", out[0].Name)
}

func (s *TagFilterIntegrationTestSuite) TestFestivals_OR_Transitive() {
	f1 := s.seedFestival("Fest1")
	f2 := s.seedFestival("Fest2")
	f3 := s.seedFestival("Fest3")
	aElec := s.seedArtist("OElec")
	aSG := s.seedArtist("OSG")
	aNone := s.seedArtist("ONone")
	s.addArtistToFestival(f1, aElec)
	s.addArtistToFestival(f2, aSG)
	s.addArtistToFestival(f3, aNone)
	s.tag("artist", aElec, "electronic")
	s.tag("artist", aSG, "shoegaze")

	out, err := s.festivalService.ListFestivals(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"electronic", "shoegaze"}, MatchAny: true},
	})
	s.Require().NoError(err)
	s.Len(out, 2)
}

// TestFestivals_SingleTag_Transitive is the canonical PSY-499 scenario for
// festivals: an artist tagged `shoegaze` surfaces every festival that artist
// is on the lineup of, even when no festival is directly tagged.
func (s *TagFilterIntegrationTestSuite) TestFestivals_SingleTag_Transitive() {
	f1 := s.seedFestival("SG-Fest1")
	f2 := s.seedFestival("SG-Fest2")
	f3 := s.seedFestival("Other-Fest")
	headliner := s.seedArtist("SGHeadliner")
	other := s.seedArtist("Unrelated")
	s.addArtistToFestival(f1, headliner)
	s.addArtistToFestival(f2, headliner)
	s.addArtistToFestival(f3, other)
	s.tag("artist", headliner, "shoegaze")

	out, err := s.festivalService.ListFestivals(map[string]interface{}{
		"tag_filter": TagFilter{TagSlugs: []string{"shoegaze"}},
	})
	s.Require().NoError(err)
	s.Require().Len(out, 2)
}

// ──────────────────────────────────────────────
// Tag facet count (PSY-499): `/tags?entity_type=show|festival` counts must
// reflect transitive usage — "3 shows have a shoegaze-tagged artist" — not
// the direct-tag count which is always zero at our data volumes.
// ──────────────────────────────────────────────

// TestListTags_ShowEntityType_Transitive verifies that when the tags
// endpoint is scoped to `entity_type=show`, each tag's usage_count reflects
// the number of distinct shows whose lineup includes an artist with that
// tag. Shows with a direct `entity_type='show'` tag row do not contribute.
func (s *TagFilterIntegrationTestSuite) TestListTags_ShowEntityType_Transitive() {
	// 2 shoegaze shows (same artist, 2 shows), 1 post-punk show,
	// 1 "other" show that only has a direct show-level tag (must NOT count).
	sA := s.seedShow("SG-A", time.Now().Add(24*time.Hour).UTC())
	sB := s.seedShow("SG-B", time.Now().Add(48*time.Hour).UTC())
	sC := s.seedShow("PP-C", time.Now().Add(72*time.Hour).UTC())
	sD := s.seedShow("Direct-Only", time.Now().Add(96*time.Hour).UTC())

	sgArtist := s.seedArtist("SGArtist")
	ppArtist := s.seedArtist("PPArtist")
	untagged := s.seedArtist("Untagged")
	s.addArtistToShow(sA, sgArtist, 0)
	s.addArtistToShow(sB, sgArtist, 0)
	s.addArtistToShow(sC, ppArtist, 0)
	s.addArtistToShow(sD, untagged, 0)

	s.tag("artist", sgArtist, "shoegaze")
	s.tag("artist", ppArtist, "post-punk")
	// Direct show-level tag: legacy/possible admin action. Must be ignored.
	s.tag("show", sD, "shoegaze")

	tags, _, err := s.tagService.ListTags("", "", nil, "name", 50, 0, catalogm.TagEntityShow)
	s.Require().NoError(err)
	counts := map[string]int{}
	for _, t := range tags {
		counts[t.Slug] = t.UsageCount
	}
	s.Equal(2, counts["shoegaze"], "shoegaze shows count (transitive via lineup)")
	s.Equal(1, counts["post-punk"], "post-punk shows count (transitive via lineup)")
	s.Equal(0, counts["phoenix"], "unused tag should be 0")
	s.Equal(0, counts["electronic"], "unused tag should be 0")
}

// TestListTags_ShowEntityType_MultipleArtistsSameShow verifies that a show
// with multiple artists sharing the same tag still counts once for that
// tag — DISTINCT dedup is preserved in the facet count.
func (s *TagFilterIntegrationTestSuite) TestListTags_ShowEntityType_MultipleArtistsSameShow() {
	sA := s.seedShow("Multi", time.Now().Add(24*time.Hour).UTC())
	a1 := s.seedArtist("a1")
	a2 := s.seedArtist("a2")
	a3 := s.seedArtist("a3")
	s.addArtistToShow(sA, a1, 0)
	s.addArtistToShow(sA, a2, 1)
	s.addArtistToShow(sA, a3, 2)
	s.tag("artist", a1, "shoegaze")
	s.tag("artist", a2, "shoegaze")
	s.tag("artist", a3, "shoegaze")

	tags, _, err := s.tagService.ListTags("", "", nil, "name", 50, 0, catalogm.TagEntityShow)
	s.Require().NoError(err)
	var shoegazeCount int
	for _, t := range tags {
		if t.Slug == "shoegaze" {
			shoegazeCount = t.UsageCount
		}
	}
	s.Equal(1, shoegazeCount, "show with 3 shoegaze artists should count once")
}

// TestListTags_FestivalEntityType_Transitive mirrors the show test for
// festivals: `/tags?entity_type=festival` facet counts reflect lineup-based
// transitive usage.
func (s *TagFilterIntegrationTestSuite) TestListTags_FestivalEntityType_Transitive() {
	f1 := s.seedFestival("FacetFest1")
	f2 := s.seedFestival("FacetFest2")
	f3 := s.seedFestival("DirectOnlyFest")
	elecArtist := s.seedArtist("ElecArtist")
	sgArtist := s.seedArtist("FSGArtist")
	untagged := s.seedArtist("FUntagged")
	s.addArtistToFestival(f1, elecArtist)
	s.addArtistToFestival(f2, elecArtist)
	s.addArtistToFestival(f3, untagged)
	s.tag("artist", elecArtist, "electronic")
	s.tag("artist", sgArtist, "shoegaze") // unattached — inflates nothing
	// Direct festival tag, should NOT count.
	s.tag("festival", f3, "electronic")

	tags, _, err := s.tagService.ListTags("", "", nil, "name", 50, 0, catalogm.TagEntityFestival)
	s.Require().NoError(err)
	counts := map[string]int{}
	for _, t := range tags {
		counts[t.Slug] = t.UsageCount
	}
	s.Equal(2, counts["electronic"], "electronic festivals (transitive via lineup)")
	s.Equal(0, counts["shoegaze"], "shoegaze artist not on any festival lineup")
}

// TestListTags_ArtistEntityType_DirectCount verifies that non-show/festival
// entity types still use the direct `entity_tags` count — only show/festival
// are transitive.
func (s *TagFilterIntegrationTestSuite) TestListTags_ArtistEntityType_DirectCount() {
	a1 := s.seedArtist("A1")
	a2 := s.seedArtist("A2")
	s.tag("artist", a1, "post-punk")
	s.tag("artist", a2, "post-punk")

	tags, _, err := s.tagService.ListTags("", "", nil, "name", 50, 0, catalogm.TagEntityArtist)
	s.Require().NoError(err)
	var ppCount int
	for _, t := range tags {
		if t.Slug == "post-punk" {
			ppCount = t.UsageCount
		}
	}
	s.Equal(2, ppCount, "artist count is direct, not transitive")
}
