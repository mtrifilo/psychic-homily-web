package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// TagIntersectionIntegrationTestSuite exercises IntersectEntitiesByTags
// (PSY-995): the cross-entity multi-tag intersection grouped by entity type.
// Seeding mirrors TagFilterIntegrationTestSuite so the per-type gates +
// direct/transitive split are validated against real rows.
type TagIntersectionIntegrationTestSuite struct {
	suite.Suite
	testDB     *testutil.TestDatabase
	db         *gorm.DB
	tagService *TagService

	user *authm.User
	tags map[string]*catalogm.Tag
}

func (s *TagIntersectionIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.tagService = NewTagService(s.db)
}

func (s *TagIntersectionIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *TagIntersectionIntegrationTestSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, tbl := range []string{
		"tag_votes", "entity_tags", "tag_aliases",
		"collection_items", "collections",
		"artist_releases", "artist_labels", "release_labels",
		"festival_artists", "festival_venues",
		"show_artists", "show_venues", "shows",
		"artists", "venues", "festivals", "labels", "releases",
		"tags", "users",
	} {
		_, _ = sqlDB.Exec("DELETE FROM " + tbl)
	}

	email := fmt.Sprintf("ix-user-%d@test.com", time.Now().UnixNano())
	name := "Intersect"
	u := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	s.user = u

	s.tags = map[string]*catalogm.Tag{}
	for _, n := range []string{"post-punk", "shoegaze", "phoenix", "electronic"} {
		cat := "genre"
		if n == "phoenix" {
			cat = "locale"
		}
		t, err := s.tagService.CreateTag(n, nil, nil, cat, false, nil)
		s.Require().NoError(err)
		s.tags[t.Slug] = t
	}
}

func TestTagIntersectionIntegrationSuite(t *testing.T) {
	suite.Run(t, new(TagIntersectionIntegrationTestSuite))
}

// ── seeding helpers ───────────────────────────────────────────────

func (s *TagIntersectionIntegrationTestSuite) tag(entityType string, entityID uint, slug string) {
	t, ok := s.tags[slug]
	s.Require().Truef(ok, "unseeded tag %q", slug)
	s.Require().NoError(s.db.Create(&catalogm.EntityTag{
		TagID: t.ID, EntityType: entityType, EntityID: entityID, AddedByUserID: s.user.ID,
	}).Error)
}

func (s *TagIntersectionIntegrationTestSuite) seedArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *TagIntersectionIntegrationTestSuite) seedVerifiedVenue(name string) uint {
	v := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: true}
	s.Require().NoError(s.db.Create(v).Error)
	return v.ID
}

func (s *TagIntersectionIntegrationTestSuite) seedUnverifiedVenue(name string) uint {
	v := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(v).Error)
	return v.ID
}

func (s *TagIntersectionIntegrationTestSuite) seedRelease(title string) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	r := &catalogm.Release{Title: title, Slug: &slug}
	s.Require().NoError(s.db.Create(r).Error)
	return r.ID
}

func (s *TagIntersectionIntegrationTestSuite) seedLabel(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	l := &catalogm.Label{Name: name, Slug: &slug, Status: catalogm.LabelStatusActive}
	s.Require().NoError(s.db.Create(l).Error)
	return l.ID
}

func (s *TagIntersectionIntegrationTestSuite) seedFestival(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	f := &catalogm.Festival{
		Name: name, Slug: slug, SeriesSlug: slug + "-series",
		EditionYear: 2026, StartDate: "2026-06-01", EndDate: "2026-06-03",
		Status: catalogm.FestivalStatusConfirmed,
	}
	s.Require().NoError(s.db.Create(f).Error)
	return f.ID
}

// seedApprovedUpcomingShow creates an approved show in the future.
func (s *TagIntersectionIntegrationTestSuite) seedApprovedUpcomingShow(title string) uint {
	show := &catalogm.Show{
		Title: title, EventDate: time.Now().Add(48 * time.Hour).UTC(),
		Status: catalogm.ShowStatusApproved, SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show.ID
}

// seedPastApprovedShow creates an approved show in the past (excluded from the
// upcoming gate).
func (s *TagIntersectionIntegrationTestSuite) seedPastApprovedShow(title string) uint {
	show := &catalogm.Show{
		Title: title, EventDate: time.Now().Add(-72 * time.Hour).UTC(),
		Status: catalogm.ShowStatusApproved, SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show.ID
}

func (s *TagIntersectionIntegrationTestSuite) addArtistToShow(showID, artistID uint, position int) {
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: showID, ArtistID: artistID, Position: position}).Error)
}

func (s *TagIntersectionIntegrationTestSuite) addArtistToFestival(festivalID, artistID uint) {
	s.Require().NoError(s.db.Create(&catalogm.FestivalArtist{FestivalID: festivalID, ArtistID: artistID}).Error)
}

// seedCollection inserts a collection with an explicit is_public flag. GORM
// skips zero-value bools on Create (the DB default true wins), so we create
// then Update is_public to land a genuinely private row.
func (s *TagIntersectionIntegrationTestSuite) seedCollection(title string, isPublic bool) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	c := &communitym.Collection{
		Title: title, Slug: slug, CreatorID: s.user.ID, IsPublic: true, DisplayMode: "unranked",
	}
	s.Require().NoError(s.db.Create(c).Error)
	if !isPublic {
		s.Require().NoError(s.db.Model(&communitym.Collection{}).Where("id = ?", c.ID).Update("is_public", false).Error)
	}
	return c.ID
}

// ── tests ─────────────────────────────────────────────────────────

// TestIntersection_CompleteKeysetAndZeroCounts: all 7 valid types present in
// canonical order; types with no matches are zero-count groups.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_CompleteKeysetAndZeroCounts() {
	a := s.seedArtist("OnlyArtist")
	s.tag("artist", a, "shoegaze")
	s.tag("artist", a, "electronic")

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4)
	s.Require().NoError(err)

	s.Require().Len(resp.Groups, len(catalogm.TagEntityTypes))
	for i, et := range catalogm.TagEntityTypes {
		s.Equal(et, resp.Groups[i].EntityType, "groups in canonical order")
	}

	counts := map[string]int64{}
	for _, g := range resp.Groups {
		counts[g.EntityType] = g.Count
	}
	s.Equal(int64(1), counts["artist"])
	s.Equal(int64(0), counts["release"])
	s.Equal(int64(0), counts["show"])
	s.Equal(int64(0), counts["venue"])
	s.Equal(int64(0), counts["label"])
	s.Equal(int64(0), counts["festival"])
	s.Equal(int64(0), counts["collection"])
}

// TestIntersection_DirectReleaseAndLabel exercises the two direct types that no
// other test reaches — release and label — so their count, enrich hydration,
// and bespoke preview ORDER BY ("releases.release_year DESC NULLS LAST" and
// "labels.name ASC") run against real rows rather than only zero-count
// short-circuits.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_DirectReleaseAndLabel() {
	rBoth := s.seedRelease("RBoth")
	rOne := s.seedRelease("ROne")
	s.tag("release", rBoth, "shoegaze")
	s.tag("release", rBoth, "electronic")
	s.tag("release", rOne, "shoegaze")

	lBoth := s.seedLabel("LBoth")
	lOne := s.seedLabel("LOne")
	s.tag("label", lBoth, "shoegaze")
	s.tag("label", lBoth, "electronic")
	s.tag("label", lOne, "electronic")

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4)
	s.Require().NoError(err)
	groups := map[string]contracts.TagIntersectionGroup{}
	for _, g := range resp.Groups {
		groups[g.EntityType] = g
	}

	// AND ⇒ only the doubly-tagged row in each type; the preview is hydrated
	// (exercises the per-type ORDER BY + enrich helper).
	s.Equal(int64(1), groups["release"].Count, "AND release: only the doubly-tagged release")
	s.Require().Len(groups["release"].Preview, 1)
	s.Equal("release", groups["release"].Preview[0].EntityType)
	s.NotEmpty(groups["release"].Preview[0].Name, "release preview hydrated via enrichReleases")

	s.Equal(int64(1), groups["label"].Count, "AND label: only the doubly-tagged label")
	s.Require().Len(groups["label"].Preview, 1)
	s.Equal("label", groups["label"].Preview[0].EntityType)
	s.NotEmpty(groups["label"].Preview[0].Name, "label preview hydrated via enrichLabels")
}

// TestIntersection_AND_vs_OR_MixDirectAndTransitive: AND requires all tags;
// OR requires any. Mixes a direct type (artist) and a transitive type (show).
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_AND_vs_OR_MixDirectAndTransitive() {
	// Direct: artist tagged both vs only one.
	aBoth := s.seedArtist("ABoth")
	aOne := s.seedArtist("AOne")
	s.tag("artist", aBoth, "post-punk")
	s.tag("artist", aBoth, "shoegaze")
	s.tag("artist", aOne, "post-punk")

	// Transitive: show whose lineup collectively covers both vs only one.
	sBoth := s.seedApprovedUpcomingShow("ShowBoth")
	sOne := s.seedApprovedUpcomingShow("ShowOne")
	aPP := s.seedArtist("LineupPP")
	aSG := s.seedArtist("LineupSG")
	aPPOnly := s.seedArtist("LineupPPOnly")
	s.addArtistToShow(sBoth, aPP, 0)
	s.addArtistToShow(sBoth, aSG, 1)
	s.addArtistToShow(sOne, aPPOnly, 0)
	s.tag("artist", aPP, "post-punk")
	s.tag("artist", aSG, "shoegaze")
	s.tag("artist", aPPOnly, "post-punk")

	// Artist direct matches: aBoth(pp,sg), aOne(pp), and the lineup artists are
	// themselves directly-tagged artists too — aPP(pp), aSG(sg), aPPOnly(pp).
	// AND (both tags on the SAME artist) ⇒ only aBoth. OR (either tag) ⇒ all 5.
	and := s.counts(s.tagService.IntersectEntitiesByTags([]string{"post-punk", "shoegaze"}, false, 4))
	s.Equal(int64(1), and["artist"], "AND artist: only the doubly-tagged artist")
	s.Equal(int64(1), and["show"], "AND show: only the lineup that collectively covers both tags")

	or := s.counts(s.tagService.IntersectEntitiesByTags([]string{"post-punk", "shoegaze"}, true, 4))
	s.Equal(int64(5), or["artist"], "OR artist: every artist carrying either tag (incl. lineup artists)")
	s.Equal(int64(2), or["show"], "OR show: both (each lineup covers at least one tag)")
}

// TestIntersection_ShowFestivalTransitive: shows/festivals are counted via a
// billed artist's tags and appear in the preview.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_ShowFestivalTransitive() {
	// Show: lineup collectively covers shoegaze + electronic.
	show := s.seedApprovedUpcomingShow("TransShow")
	aSG := s.seedArtist("ShowSG")
	aEL := s.seedArtist("ShowEL")
	s.addArtistToShow(show, aSG, 0)
	s.addArtistToShow(show, aEL, 1)
	s.tag("artist", aSG, "shoegaze")
	s.tag("artist", aEL, "electronic")

	// Festival: lineup collectively covers both.
	fest := s.seedFestival("TransFest")
	fSG := s.seedArtist("FestSG")
	fEL := s.seedArtist("FestEL")
	s.addArtistToFestival(fest, fSG)
	s.addArtistToFestival(fest, fEL)
	s.tag("artist", fSG, "shoegaze")
	s.tag("artist", fEL, "electronic")

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4)
	s.Require().NoError(err)

	byType := map[string]int{}
	previewTitles := map[string][]string{}
	for i, g := range resp.Groups {
		byType[g.EntityType] = i
		for _, p := range g.Preview {
			previewTitles[g.EntityType] = append(previewTitles[g.EntityType], p.Name)
		}
	}
	s.Equal(int64(1), resp.Groups[byType["show"]].Count, "show counted transitively")
	s.Equal(int64(1), resp.Groups[byType["festival"]].Count, "festival counted transitively")
	s.Contains(previewTitles["show"], "TransShow")
	s.Contains(previewTitles["festival"], "TransFest")
}

// TestIntersection_CollectionExcludesPrivate: a private collection tagged with
// both tags must NOT appear in the collection group's count OR preview.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_CollectionExcludesPrivate() {
	pub := s.seedCollection("PublicCol", true)
	priv := s.seedCollection("PrivateCol", false)
	s.tag("collection", pub, "shoegaze")
	s.tag("collection", pub, "electronic")
	s.tag("collection", priv, "shoegaze")
	s.tag("collection", priv, "electronic")

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4)
	s.Require().NoError(err)

	var col *struct {
		count   int64
		preview []string
	}
	for _, g := range resp.Groups {
		if g.EntityType == "collection" {
			names := make([]string, 0, len(g.Preview))
			for _, p := range g.Preview {
				names = append(names, p.Name)
			}
			col = &struct {
				count   int64
				preview []string
			}{count: g.Count, preview: names}
		}
	}
	s.Require().NotNil(col)
	s.Equal(int64(1), col.count, "only the public collection is counted")
	s.Contains(col.preview, "PublicCol")
	s.NotContains(col.preview, "PrivateCol", "private collection must not leak into the preview")
}

// TestIntersection_VenueVerifiedGate: an unverified venue tagged with both
// tags is excluded (mirrors the public /venues list).
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_VenueVerifiedGate() {
	vVerified := s.seedVerifiedVenue("VerifiedV")
	vUnverified := s.seedUnverifiedVenue("UnverifiedV")
	for _, v := range []uint{vVerified, vUnverified} {
		s.tag("venue", v, "post-punk")
		s.tag("venue", v, "phoenix")
	}

	c := s.counts(s.tagService.IntersectEntitiesByTags([]string{"post-punk", "phoenix"}, false, 4))
	s.Equal(int64(1), c["venue"], "only the verified venue counts")
}

// TestIntersection_ShowUpcomingApprovedGate: a past (or non-approved) show whose
// lineup covers both tags is excluded from the show count so it agrees with
// /shows?tags= (which is upcoming + approved only).
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_ShowUpcomingApprovedGate() {
	upcoming := s.seedApprovedUpcomingShow("UpcomingShow")
	past := s.seedPastApprovedShow("PastShow")
	aSG := s.seedArtist("GateSG")
	aEL := s.seedArtist("GateEL")
	for _, sh := range []uint{upcoming, past} {
		s.addArtistToShow(sh, aSG, 0)
		s.addArtistToShow(sh, aEL, 1)
	}
	s.tag("artist", aSG, "shoegaze")
	s.tag("artist", aEL, "electronic")

	c := s.counts(s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4))
	s.Equal(int64(1), c["show"], "past show is excluded by the upcoming gate")
}

// TestIntersection_PreviewClampedToLimit: preview size is bounded by the limit
// while count reflects the full match set.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_PreviewClampedToLimit() {
	for i := 0; i < 5; i++ {
		a := s.seedArtist(fmt.Sprintf("Multi%d", i))
		s.tag("artist", a, "shoegaze")
		s.tag("artist", a, "electronic")
	}

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 2)
	s.Require().NoError(err)
	for _, g := range resp.Groups {
		if g.EntityType == "artist" {
			s.Equal(int64(5), g.Count, "count is the full match set")
			s.Len(g.Preview, 2, "preview clamped to the limit")
		}
	}
}

// TestIntersection_TagsEcho: the resolved input tags are echoed in request
// order with their summaries.
func (s *TagIntersectionIntegrationTestSuite) TestIntersection_TagsEcho() {
	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "post-punk"}, false, 4)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 2)
	s.Equal("shoegaze", resp.Tags[0].Slug)
	s.Equal("post-punk", resp.Tags[1].Slug)
	s.Equal("all", resp.TagMatch)
}

// counts collapses an IntersectEntitiesByTags result into entity_type → count,
// failing the test on error.
func (s *TagIntersectionIntegrationTestSuite) counts(resp *contracts.TagIntersectionResponse, err error) map[string]int64 {
	s.Require().NoError(err)
	out := map[string]int64{}
	for _, g := range resp.Groups {
		out[g.EntityType] = g.Count
	}
	return out
}
