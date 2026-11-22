package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// EntityFormerSlugsSuite exercises entity_former_slugs end to end against a
// fully migrated database: the slug triggers, the merges that write it, the
// inventory re-point and delete sweep that maintain it, and the probe reads.
type EntityFormerSlugsSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	probe  *EntityExistenceService
	seq    int
}

func TestEntityFormerSlugsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(EntityFormerSlugsSuite))
}

func (s *EntityFormerSlugsSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.probe = NewEntityExistenceService(s.db, NewSceneService(s.db))
}

func (s *EntityFormerSlugsSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *EntityFormerSlugsSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, stmt := range []string{
		"DELETE FROM audit_logs",
		"DELETE FROM entity_former_slugs",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM artist_aliases",
		"DELETE FROM artists",
		"DELETE FROM venues",
	} {
		_, err := sqlDB.Exec(stmt)
		s.Require().NoError(err, stmt)
	}
}

// ──────────────────────────────────────────────
// Fixtures
// ──────────────────────────────────────────────

// slug returns a slug unique to this run, so no fixture can collide with a row
// another test left behind.
func (s *EntityFormerSlugsSuite) slug(label string) string {
	s.seq++
	return fmt.Sprintf("%s-%d-%d", label, s.seq, time.Now().UnixNano())
}

func (s *EntityFormerSlugsSuite) createVenue(slug string) *catalogm.Venue {
	v := &catalogm.Venue{Name: "Room " + s.slug("name"), Slug: &slug, City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

func (s *EntityFormerSlugsSuite) createArtist(slug string) *catalogm.Artist {
	a := &catalogm.Artist{Name: "Band " + s.slug("name"), Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *EntityFormerSlugsSuite) createShow(slug string) *catalogm.Show {
	show := &catalogm.Show{
		Title:     "Show " + slug,
		Slug:      &slug,
		EventDate: time.Now().UTC().Add(72 * time.Hour).Truncate(time.Microsecond),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show
}

// createBilledShow is createShow at venue with artist billed, so the show has
// the dedup keys a venue merge reads.
func (s *EntityFormerSlugsSuite) createBilledShow(slug string, venue *catalogm.Venue, artist *catalogm.Artist, date time.Time) *catalogm.Show {
	show := &catalogm.Show{Title: "Show " + slug, Slug: &slug, EventDate: date, Status: catalogm.ShowStatusApproved}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)
	d, vid := date, venue.ID
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: artist.ID, Position: 0, SetType: "performer", EventDate: &d, VenueID: &vid,
	}).Error)
	return show
}

// family is one entity family with a way to seed a row and change its slug,
// so the trigger cases run against all three tables.
type family struct {
	prefix string
	table  string
	create func(slug string) uint
}

func (s *EntityFormerSlugsSuite) families() []family {
	return []family{
		{prefix: "shows", table: "shows", create: func(slug string) uint { return s.createShow(slug).ID }},
		{prefix: "venues", table: "venues", create: func(slug string) uint { return s.createVenue(slug).ID }},
		{prefix: "artists", table: "artists", create: func(slug string) uint { return s.createArtist(slug).ID }},
	}
}

func (s *EntityFormerSlugsSuite) setSlug(table string, id uint, slug any) {
	// #nosec G201 -- table is one of the three literals in families().
	s.Require().NoError(s.db.Exec(fmt.Sprintf("UPDATE %s SET slug = ? WHERE id = ?", table), slug, id).Error)
}

// formerRow is the entity id a former slug names, or 0 when no row holds it.
func (s *EntityFormerSlugsSuite) formerRow(entityType, slug string) uint {
	var id uint
	s.Require().NoError(s.db.Raw(
		"SELECT entity_id FROM entity_former_slugs WHERE entity_type = ? AND former_slug = ?",
		entityType, slug).Scan(&id).Error)
	return id
}

// served is what the probe answers for a segment: whether the live lookup hits,
// and the current slug a miss resolves to.
func (s *EntityFormerSlugsSuite) served(prefix, segment string) (live bool, current string) {
	live, err := s.probe.Exists(prefix, segment)
	s.Require().NoError(err)
	if live {
		return true, ""
	}
	current, err = s.probe.CurrentSlugForFormerSlug(prefix, segment)
	s.Require().NoError(err)
	return false, current
}

var entityTypeOfPrefix = map[string]string{"shows": "show", "venues": "venue", "artists": "artist"}

// ──────────────────────────────────────────────
// Slug changes (the triggers)
// ──────────────────────────────────────────────

func (s *EntityFormerSlugsSuite) TestSlugChangeIsRecordedAndResolves() {
	for _, f := range s.families() {
		s.Run(f.prefix, func() {
			oldSlug, newSlug := s.slug("old"), s.slug("new")
			id := f.create(oldSlug)
			s.setSlug(f.table, id, newSlug)

			s.Equal(id, s.formerRow(entityTypeOfPrefix[f.prefix], oldSlug))
			live, current := s.served(f.prefix, oldSlug)
			s.False(live, "the old slug is no longer live")
			s.Equal(newSlug, current)

			live, _ = s.served(f.prefix, newSlug)
			s.True(live, "the new slug is live")
		})
	}
}

func (s *EntityFormerSlugsSuite) TestRenameChainResolvesInOneHop() {
	for _, f := range s.families() {
		s.Run(f.prefix, func() {
			a, b, c := s.slug("a"), s.slug("b"), s.slug("c")
			id := f.create(a)
			s.setSlug(f.table, id, b)
			s.setSlug(f.table, id, c)

			for _, former := range []string{a, b} {
				_, current := s.served(f.prefix, former)
				s.Equalf(c, current, "%s resolves to the live slug", former)
			}
		})
	}
}

func (s *EntityFormerSlugsSuite) TestChangingBackRemovesTheRow() {
	for _, f := range s.families() {
		s.Run(f.prefix, func() {
			a, b := s.slug("a"), s.slug("b")
			id := f.create(a)
			s.setSlug(f.table, id, b)
			s.setSlug(f.table, id, a)

			s.Zero(s.formerRow(entityTypeOfPrefix[f.prefix], a), "the live entity owns its slug again")
			live, _ := s.served(f.prefix, a)
			s.True(live)
			_, current := s.served(f.prefix, b)
			s.Equal(a, current, "the slug it left in between redirects back")
		})
	}
}

// Only a change to a different non-empty value is a URL change.
func (s *EntityFormerSlugsSuite) TestOnlyAChangeToANonEmptySlugIsRecorded() {
	for _, f := range s.families() {
		s.Run(f.prefix, func() {
			a := s.slug("a")
			id := f.create(a)
			s.setSlug(f.table, id, a)
			s.Zero(s.formerRow(entityTypeOfPrefix[f.prefix], a), "an unchanged slug records nothing")
			s.setSlug(f.table, id, nil)
			s.Zero(s.formerRow(entityTypeOfPrefix[f.prefix], a), "a cleared slug records nothing")
		})
	}
}

// A live entity always wins a slug: taking one that is a former slug, by
// insert or by update, removes the row, so the slug answers for the new
// entity now and does not resurface when that entity goes.
func (s *EntityFormerSlugsSuite) TestAFreedSlugTakenByANewEntityBelongsToIt() {
	for _, f := range s.families() {
		s.Run(f.prefix, func() {
			entityType := entityTypeOfPrefix[f.prefix]
			a, b := s.slug("a"), s.slug("b")
			first := f.create(a)
			s.setSlug(f.table, first, b)
			s.Require().Equal(first, s.formerRow(entityType, a))

			second := f.create(a)
			s.Zero(s.formerRow(entityType, a), "an insert taking the slug removes the row")
			live, _ := s.served(f.prefix, a)
			s.True(live)

			// The same through an update.
			c, d := s.slug("c"), s.slug("d")
			s.setSlug(f.table, first, c)
			s.Require().Equal(first, s.formerRow(entityType, b))
			s.setSlug(f.table, second, b)
			s.Zero(s.formerRow(entityType, b), "an update taking the slug removes the row")
			s.Equal(second, s.formerRow(entityType, a), "and records the slug it left")

			s.setSlug(f.table, second, d)
			_, current := s.served(f.prefix, b)
			s.Equal(d, current, "the slug now follows its latest owner")
		})
	}
}

// The show trigger sits beside show_dedup_keys' own trigger on shows; one
// statement that moves the date and the slug keeps both tables right.
func (s *EntityFormerSlugsSuite) TestShowSlugTriggerCoexistsWithDedupKeys() {
	venue := s.createVenue(s.slug("room"))
	artist := s.createArtist(s.slug("band"))
	date := time.Now().UTC().Add(96 * time.Hour).Truncate(time.Microsecond)
	oldSlug, newSlug := s.slug("old-show"), s.slug("new-show")
	show := s.createBilledShow(oldSlug, venue, artist, date)

	moved := date.Add(24 * time.Hour)
	s.Require().NoError(s.db.Exec("UPDATE shows SET event_date = ?, slug = ? WHERE id = ?", moved, newSlug, show.ID).Error)

	var keyDate time.Time
	s.Require().NoError(s.db.Raw("SELECT event_date FROM show_dedup_keys WHERE show_id = ?", show.ID).Scan(&keyDate).Error)
	s.True(keyDate.Equal(moved), "dedup key date %v, want %v", keyDate, moved)
	s.Equal(show.ID, s.formerRow("show", oldSlug))
}

// A former show slug resolves only to a show the probe serves, so a header can
// never name a private show's slug.
func (s *EntityFormerSlugsSuite) TestShowFormerSlugResolvesOnlyToAnApprovedShow() {
	oldSlug, newSlug := s.slug("old"), s.slug("new")
	show := s.createShow(oldSlug)
	s.setSlug("shows", show.ID, newSlug)
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", show.ID).Update("status", catalogm.ShowStatusPrivate).Error)

	live, current := s.served("shows", oldSlug)
	s.False(live)
	s.Empty(current)
}

// An id segment is never a former slug, so it costs no lookup and names
// nothing, and a family that keeps no former slugs answers without a query.
func (s *EntityFormerSlugsSuite) TestIDSegmentsAndOtherFamiliesNameNothing() {
	venue := s.createVenue(s.slug("room"))
	current, err := s.probe.CurrentSlugForFormerSlug("venues", fmt.Sprintf("%d", venue.ID))
	s.Require().NoError(err)
	s.Empty(current)
	current, err = s.probe.CurrentSlugForFormerSlug("labels", "anything")
	s.Require().NoError(err)
	s.Empty(current)
}

// ──────────────────────────────────────────────
// Merges
// ──────────────────────────────────────────────

// A venue merge points the merged room's slug, and the slugs it had before, at
// the room that survives; a show it deletes as a duplicate points at the show
// that survives at the kept room; a show it moves keeps its slug.
func (s *EntityFormerSlugsSuite) TestMergeVenuesPointsTheLoserAndItsDuplicateShowsAtTheWinners() {
	kept := s.createVenue(s.slug("kept-room"))
	mergedOld, mergedSlug := s.slug("merged-room-old"), s.slug("merged-room")
	merged := s.createVenue(mergedOld)
	s.setSlug("venues", merged.ID, mergedSlug)

	artist := s.createArtist(s.slug("band"))
	date := time.Now().UTC().Add(120 * time.Hour).Truncate(time.Microsecond)
	survivor := s.createBilledShow(s.slug("survivor"), kept, artist, date)
	dupOld, dupSlug := s.slug("dup-old"), s.slug("dup")
	duplicate := s.createBilledShow(dupOld, merged, artist, date)
	s.setSlug("shows", duplicate.ID, dupSlug)
	movedSlug := s.slug("moved")
	s.createBilledShow(movedSlug, merged, artist, date.Add(48*time.Hour))

	result, err := NewVenueService(s.db).MergeVenues(kept.ID, merged.ID, 0)
	s.Require().NoError(err)
	s.Require().EqualValues(1, result.DuplicateShows)

	for _, former := range []string{mergedSlug, mergedOld} {
		_, current := s.served("venues", former)
		s.Equalf(*kept.Slug, current, "venue %s resolves to the kept room", former)
	}
	for _, former := range []string{dupSlug, dupOld} {
		_, current := s.served("shows", former)
		s.Equalf(*survivor.Slug, current, "show %s resolves to the surviving show", former)
	}
	live, _ := s.served("shows", movedSlug)
	s.True(live, "a moved show keeps its slug")
	s.Zero(s.formerRow("show", movedSlug))

	id, current, err := NewVenueService(s.db).VenueForFormerSlug(mergedSlug)
	s.Require().NoError(err)
	s.Equal(kept.ID, id)
	s.Equal(*kept.Slug, current)
}

// A preview rolls back, and its former-slug writes with it.
func (s *EntityFormerSlugsSuite) TestVenueMergePreviewRecordsNothing() {
	kept := s.createVenue(s.slug("kept-room"))
	merged := s.createVenue(s.slug("merged-room"))
	_, err := NewVenueService(s.db).PreviewMergeVenues(kept.ID, merged.ID)
	s.Require().NoError(err)
	s.Zero(s.formerRow("venue", *merged.Slug))
}

func (s *EntityFormerSlugsSuite) TestMergeArtistsPointsTheLoserAtTheWinner() {
	kept := s.createArtist(s.slug("kept-band"))
	mergedOld, mergedSlug := s.slug("merged-band-old"), s.slug("merged-band")
	merged := s.createArtist(mergedOld)
	s.setSlug("artists", merged.ID, mergedSlug)

	_, err := NewArtistService(s.db).MergeArtists(kept.ID, merged.ID)
	s.Require().NoError(err)

	for _, former := range []string{mergedSlug, mergedOld} {
		_, current := s.served("artists", former)
		s.Equalf(*kept.Slug, current, "artist %s resolves to the kept artist", former)
	}
}

func (s *EntityFormerSlugsSuite) TestMergeDuplicateShowPointsTheLoserAtTheWinner() {
	winner := s.createShow(s.slug("winner"))
	loserOld, loserSlug := s.slug("loser-old"), s.slug("loser")
	loser := s.createShow(loserOld)
	s.setSlug("shows", loser.ID, loserSlug)

	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner.ID, loser.ID, &ShowDedupSummary{})
	}))

	for _, former := range []string{loserSlug, loserOld} {
		_, current := s.served("shows", former)
		s.Equalf(*winner.Slug, current, "show %s resolves to the winner", former)
	}
}

// A into B, then B into C: both resolve to C in one hop, and so does every
// slug after C is renamed.
func (s *EntityFormerSlugsSuite) TestMergeChainsStayOneHop() {
	svc := NewArtistService(s.db)
	a := s.createArtist(s.slug("a"))
	b := s.createArtist(s.slug("b"))
	c := s.createArtist(s.slug("c"))
	_, err := svc.MergeArtists(b.ID, a.ID)
	s.Require().NoError(err)
	_, err = svc.MergeArtists(c.ID, b.ID)
	s.Require().NoError(err)

	for _, former := range []string{*a.Slug, *b.Slug} {
		_, current := s.served("artists", former)
		s.Equalf(*c.Slug, current, "%s resolves to the last survivor", former)
	}

	renamed := s.slug("c-renamed")
	s.setSlug("artists", c.ID, renamed)
	for _, former := range []string{*a.Slug, *b.Slug, *c.Slug} {
		_, current := s.served("artists", former)
		s.Equalf(renamed, current, "%s resolves to the renamed survivor", former)
	}
}

// ──────────────────────────────────────────────
// Deletes with no successor
// ──────────────────────────────────────────────

func (s *EntityFormerSlugsSuite) TestDeletedEntitiesLoseTheirFormerSlugs() {
	venueOld := s.slug("venue-old")
	venue := s.createVenue(venueOld)
	s.setSlug("venues", venue.ID, s.slug("venue-new"))
	s.Require().NoError(NewVenueService(s.db).DeleteVenue(venue.ID))

	artistOld := s.slug("artist-old")
	artist := s.createArtist(artistOld)
	s.setSlug("artists", artist.ID, s.slug("artist-new"))
	s.Require().NoError(NewArtistService(s.db).DeleteArtist(artist.ID, contracts.EntityDeleteActor{UserID: 1, IsAdmin: true}))

	showOld := s.slug("show-old")
	show := s.createShow(showOld)
	s.setSlug("shows", show.ID, s.slug("show-new"))
	s.Require().NoError(NewShowService(s.db).DeleteShow(show.ID))

	for prefix, former := range map[string]string{"venues": venueOld, "artists": artistOld, "shows": showOld} {
		s.Zerof(s.formerRow(entityTypeOfPrefix[prefix], former), "%s %s keeps no row", prefix, former)
		live, current := s.served(prefix, former)
		s.False(live)
		s.Emptyf(current, "%s %s answers 404", prefix, former)
	}
}

// A row whose entity is gone without the sweep (a bare delete) names nothing:
// the lookup joins the live entity.
func (s *EntityFormerSlugsSuite) TestAFormerSlugOfAMissingEntityNamesNothing() {
	former := s.slug("orphan")
	s.Require().NoError(s.db.Exec(
		"INSERT INTO entity_former_slugs (entity_type, former_slug, entity_id) VALUES ('venue', ?, 2147483000)", former).Error)
	_, current := s.served("venues", former)
	s.Empty(current)
}
