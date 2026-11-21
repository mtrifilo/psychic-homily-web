package catalog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

type SameLocalDateTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func TestSameLocalDateTestSuite(t *testing.T) {
	suite.Run(t, new(SameLocalDateTestSuite))
}

func (s *SameLocalDateTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *SameLocalDateTestSuite) TearDownSuite() {
	if s.testDB != nil {
		s.testDB.Cleanup()
	}
}

func (s *SameLocalDateTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{
		"comment_subscriptions", "comment_votes", "comment_edits", "comment_last_read", "comments",
		"notification_log", "user_bookmarks", "show_reports",
		"show_artists", "show_venues", "shows", "artists", "venues", "users",
	} {
		_, _ = sqlDB.Exec(fmt.Sprintf("DELETE FROM %s", t))
	}
}

const losAngeles = "America/Los_Angeles"

// room inserts a San Diego room. A nil zone stores none, so the state decides.
func (s *SameLocalDateTestSuite) room(slug, name string, tz *string) *catalogm.Venue {
	v := &catalogm.Venue{Name: name, Slug: &slug, City: "San Diego", State: "CA", Verified: true, Timezone: tz}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

func (s *SameLocalDateTestSuite) artist(name string) uint {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

type showSpec struct {
	slug string
	// title defaults to fixtureTitle, so copies of one show share a title.
	title     string
	at        time.Time
	musicAt   *time.Time
	doorsAt   *time.Time
	cancelled bool
	created   time.Time
	status    string
	ticket    string
	rooms     []uint
	acts      []uint
}

const fixtureTitle = "A Night Out"

// show inserts a show with a bill in position order, the first act headliner.
func (s *SameLocalDateTestSuite) show(sp showSpec) uint {
	status := sp.status
	if status == "" {
		status = "approved"
	}
	created := sp.created
	if created.IsZero() {
		created = time.Date(2026, 8, 30, 1, 11, 0, 0, time.UTC)
	}
	var ticket *string
	if sp.ticket != "" {
		ticket = &sp.ticket
	}
	title := sp.title
	if title == "" {
		title = fixtureTitle
	}
	var id uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO shows (title, slug, event_date, music_at, doors_at, is_cancelled, state, status, source, ticket_url,
		                   created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'CA', ?, 'user', ?, ?, ?) RETURNING id
	`, title, sp.slug, sp.at, sp.musicAt, sp.doorsAt, sp.cancelled, status, ticket, created, created).Row().Scan(&id))
	for _, r := range sp.rooms {
		s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, r).Error)
	}
	for i, a := range sp.acts {
		setType := "performer"
		if i == 0 {
			setType = "headliner"
		}
		s.Require().NoError(s.db.Exec(
			`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, ?)`,
			id, a, i, setType).Error)
	}
	s.Require().NoError(syncShowArtistDedupColumns(s.db, id))
	return id
}

func (s *SameLocalDateTestSuite) user(email string) uint {
	u := &authm.User{Email: stringPtr(email), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u.ID
}

func (s *SameLocalDateTestSuite) groups() []SameLocalDateGroup {
	g, err := FindSameLocalDateGroups(s.db)
	s.Require().NoError(err)
	return g
}

func groupSlugs(g SameLocalDateGroup) []string {
	var out []string
	for _, c := range g.Copies {
		out = append(out, c.Slug)
	}
	return out
}

func zonePtr(tz string) *string { return &tz }

func (s *SameLocalDateTestSuite) TestAPairThatDiffersByClockTimeIsOneGroup() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	maris := s.artist("MARIS")
	// 8pm and 8:30pm Pacific on 2026-10-09.
	s.show(showSpec{slug: "2026-10-09-maris-at-the-casbah", at: time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{maris}})
	music := time.Date(2026, 10, 10, 3, 30, 0, 0, time.UTC)
	s.show(showSpec{slug: "2026-10-09-maris-at-casbah", at: music, musicAt: &music,
		created: time.Date(2026, 9, 13, 23, 54, 0, 0, time.UTC), rooms: []uint{casbah.ID}, acts: []uint{maris}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	g := groups[0]
	s.Equal("2026-10-09", g.LocalDate)
	s.Equal("MARIS", g.HeadlinerName)
	s.Equal("the-casbah", g.Room.Slug)
	s.Equal([]string{"2026-10-09-maris-at-the-casbah", "2026-10-09-maris-at-casbah"}, groupSlugs(g))
	s.False(g.Copies[0].StatesStart, "8pm with no music time states no start")
	s.Equal("stored start 20:00 PDT", g.Copies[0].ClockText())
	s.True(g.Copies[1].StatesStart)
	s.Equal("stored start 20:30 PDT, music 20:30 PDT", g.Copies[1].ClockText())

	// The Casbah shape: 30 minutes apart, nothing else differs. A merge, into
	// the copy stating its music time.
	keep, rows := SuggestSameLocalDateKeep(g)
	s.Require().NotNil(keep)
	s.Equal("2026-10-09-maris-at-casbah", keep.Slug)
	s.Require().Len(rows, 1)
	s.Equal(SameLocalDateMerge, rows[0].Action, rows[0].Reason)
	s.Equal(30*time.Minute, rows[0].Span)
}

func (s *SameLocalDateTestSuite) TestAPairAcrossMidnightUTCOnOneVenueLocalDayIsOneGroup() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("The Beat Farmers")
	// In December, 3pm Pacific is 23:00 UTC on the 6th and 8pm Pacific is 04:00
	// UTC on the 7th: two UTC dates, one venue-local date.
	afternoon := time.Date(2026, 12, 6, 23, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 12, 7, 4, 0, 0, 0, time.UTC)
	s.Require().NotEqual(afternoon.Format("2006-01-02"), evening.Format("2006-01-02"))
	s.show(showSpec{slug: "beat-farmers-evening", at: evening, rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "beat-farmers-afternoon", at: afternoon, rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	s.Equal("2026-12-06", groups[0].LocalDate)
	s.Equal("stored start 15:00 PST", groups[0].Copies[0].ClockText())
	s.Equal([]string{"beat-farmers-afternoon", "beat-farmers-evening"}, groupSlugs(groups[0]))
}

func (s *SameLocalDateTestSuite) TestTwoShowsOnOneUTCDateOnDifferentVenueLocalDaysAreNotAPair() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Friko")
	// Both on 2026-09-20 UTC: 6pm Pacific on the 19th, and 1pm Pacific on the 20th.
	s.show(showSpec{slug: "friko-19th", at: time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "friko-20th", at: time.Date(2026, 9, 20, 20, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})

	s.Empty(s.groups())
}

func (s *SameLocalDateTestSuite) TestAVenueWithNoStoredTimezoneIsDatedByItsState() {
	casbah := s.room("the-casbah", "The Casbah", nil)
	act := s.artist("L7")
	// The room stores no zone, so California's decides: 03:00 UTC on the 8th is
	// 8pm Pacific on the 7th, and 23:30 UTC on the 7th is 4:30pm Pacific on the
	// 7th. Read in UTC the two would fall on different dates.
	s.show(showSpec{slug: "l7-evening", at: time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "l7-afternoon", at: time.Date(2026, 10, 7, 23, 30, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	s.Equal("2026-10-07", groups[0].LocalDate)
	s.Equal("stored start 16:30 PDT", groups[0].Copies[0].ClockText())
	s.False(groups[0].Copies[1].StatesStart, "the state's zone reads the 03:00 UTC copy as 8pm, the date-only hour")
}

func (s *SameLocalDateTestSuite) TestOtherHeadlinersOtherRoomsAndUnapprovedShowsAreNotGrouped() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	soda := s.room("soda-bar", "Soda Bar", zonePtr(losAngeles))
	a := s.artist("Slow Magic")
	b := s.artist("Opener Band")
	evening := time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)
	s.show(showSpec{slug: "slow-magic", at: evening, rooms: []uint{casbah.ID}, acts: []uint{a, b}})
	// The same night, the support act headlining its own bill.
	s.show(showSpec{slug: "opener-headlines", at: evening.Add(30 * time.Minute), rooms: []uint{casbah.ID}, acts: []uint{b}})
	// The headliner at another room that night.
	s.show(showSpec{slug: "slow-magic-soda", at: evening.Add(time.Hour), rooms: []uint{soda.ID}, acts: []uint{a}})
	// A pending copy at the same room.
	s.show(showSpec{slug: "slow-magic-pending", at: evening.Add(30 * time.Minute), status: "pending",
		rooms: []uint{casbah.ID}, acts: []uint{a}})

	s.Empty(s.groups())
}

func (s *SameLocalDateTestSuite) TestTheHeadlinerIsTheCuratedActNotTheFirstBilled() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	opener := s.artist("First Billed Opener")
	top := s.artist("Curated Headliner")
	evening := time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)
	first := s.show(showSpec{slug: "curated-1", at: evening, rooms: []uint{casbah.ID}, acts: []uint{opener, top}})
	second := s.show(showSpec{slug: "curated-2", at: evening.Add(time.Hour), rooms: []uint{casbah.ID}, acts: []uint{opener, top}})
	for _, id := range []uint{first, second} {
		s.Require().NoError(s.db.Exec(`UPDATE show_artists SET set_type = CASE WHEN artist_id = ? THEN 'headliner' ELSE 'opener' END
			WHERE show_id = ?`, top, id).Error)
	}

	groups := s.groups()
	s.Require().Len(groups, 1)
	s.Equal("Curated Headliner", groups[0].HeadlinerName)
}

func (s *SameLocalDateTestSuite) TestCountsSavesAndComments() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Dillinger Four")
	evening := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	older := s.show(showSpec{slug: "d4-older", at: evening, rooms: []uint{casbah.ID}, acts: []uint{act}})
	newer := s.show(showSpec{slug: "d4-newer", at: evening.Add(30 * time.Minute),
		created: time.Date(2026, 9, 13, 23, 54, 0, 0, time.UTC), rooms: []uint{casbah.ID}, acts: []uint{act}})
	users := []uint{s.user("a@test.local"), s.user("b@test.local")}
	bookmark := `INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, ?)`
	s.Require().NoError(s.db.Exec(bookmark, users[0], older, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, users[1], older, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, users[0], newer, "follow").Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'see you', '<p>see you</p>')`, older, users[1]).Error)

	groups := s.groups()
	s.Require().Len(groups, 1)
	byID := map[uint]SameLocalDateCopy{}
	for _, c := range groups[0].Copies {
		byID[c.ShowID] = c
	}
	s.Equal(int64(2), byID[older].Saves)
	s.Equal(int64(1), byID[older].Comments)
	s.Zero(byID[newer].Saves, "a follow is not a save")
	s.Zero(byID[newer].Comments)

	// Saves and comments move with a merge, so they do not decide which copy is
	// kept: the 8:30 copy states a time and the saved 8pm copy may not.
	keep, rows := SuggestSameLocalDateKeep(groups[0])
	s.Require().NotNil(keep)
	s.Equal(newer, keep.ShowID)
	s.Require().Len(rows, 1)
	s.Equal(SameLocalDateMerge, rows[0].Action)
	s.Contains(rows[0].Reason, "it states a start (stored start 20:30 PDT)")
	s.Contains(rows[0].Reason, "this copy's 2 saves and 1 visible comments move to the kept copy")
}

// A doors-only matinee (stored at the date-only hour, doors 3pm) and a 9pm show:
// the clocks span six hours, so the pair is review.
func (s *SameLocalDateTestSuite) TestADoorsOnlyMatineeAndAnEveningShowAreReview() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Doors Only Act")
	doors := time.Date(2026, 10, 9, 22, 0, 0, 0, time.UTC) // 3pm Pacific
	music := time.Date(2026, 10, 10, 4, 0, 0, 0, time.UTC) // 9pm Pacific
	s.show(showSpec{slug: "doors-only-matinee", at: time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC), doorsAt: &doors,
		rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "evening", at: music, musicAt: &music, rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	matinee := groups[0].Copies[0]
	s.Equal("doors-only-matinee", matinee.Slug)
	s.False(matinee.StatesStart)
	s.Equal("stored start 20:00 PDT, doors 15:00 PDT", matinee.ClockText())
	keep, rows := SuggestSameLocalDateKeep(groups[0])
	s.Nil(keep)
	s.Require().Len(rows, 2)
	s.Equal(SameLocalDateReview, rows[0].Action)
	s.Contains(rows[0].Reason, "the clocks span 6h; these may be two shows")
	s.Contains(rows[0].Reason, "(6) the clocks span 360 minutes")
}

// The columns that only the rule's notes and conditions read come back from
// the database under the names the query selects.
func (s *SameLocalDateTestSuite) TestReadsTitleScraperEventSubmitterAndMusicTime() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Read Back")
	evening := time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC) // 9pm Pacific
	music := evening.Add(30 * time.Minute)
	scraped := s.show(showSpec{slug: "scraped", title: "Early Show", at: evening, musicAt: &music,
		rooms: []uint{casbah.ID}, acts: []uint{act}})
	submitter := s.user("submitter@test.local")
	s.Require().NoError(s.db.Exec(`UPDATE shows SET source_event_id = 'evt-1', submitted_by = ? WHERE id = ?`,
		submitter, scraped).Error)
	s.show(showSpec{slug: "other", at: evening.Add(20 * time.Minute), rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	var got SameLocalDateCopy
	for _, c := range groups[0].Copies {
		if c.Slug == "scraped" {
			got = c
		}
	}
	s.Equal("Early Show", got.Title)
	s.Equal("evt-1", got.SourceEventID)
	s.Equal(submitter, got.SubmittedBy)
	s.Equal("stored start 21:00 PDT, music 21:30 PDT", got.ClockText())
	_, rows := SuggestSameLocalDateKeep(groups[0])
	s.Contains(rows[0].Reason, `(3) the titles differ ("Early Show" and "A Night Out")`)
}

// Every column the finder reads into DroppedFields and IsCancelled is read
// from the database under the name the query selects.
func (s *SameLocalDateTestSuite) TestReadsEveryDroppedFieldFromTheRow() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Full Record")
	evening := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	doors := evening.Add(-time.Hour)
	full := s.show(showSpec{slug: "full", at: evening, doorsAt: &doors, cancelled: true,
		ticket: "https://tickets.example/full", rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.Require().NoError(s.db.Exec(`UPDATE shows SET price = 10, door_price = 12, age_requirement = '21+',
		description = 'a night', image_url = 'https://img.example/x.jpg', is_sold_out = true WHERE id = ?`, full).Error)
	s.show(showSpec{slug: "empty", at: evening.Add(30 * time.Minute), rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	var got SameLocalDateCopy
	for _, c := range groups[0].Copies {
		if c.Slug == "full" {
			got = c
		}
	}
	s.Equal(DroppedByMergeFields, got.DroppedFields)
	s.True(got.IsCancelled)
}

func (s *SameLocalDateTestSuite) TestOnlyVisibleCommentsAreCounted() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("Spam Target")
	evening := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	s.show(showSpec{slug: "spam-a", at: evening, rooms: []uint{casbah.ID}, acts: []uint{act}})
	b := s.show(showSpec{slug: "spam-b", at: evening.Add(30 * time.Minute), rooms: []uint{casbah.ID}, acts: []uint{act}})
	u := s.user("spammer@test.local")
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html, visibility)
		VALUES ('show', ?, ?, 'buy now', '<p>buy now</p>', 'hidden_by_mod')`, b, u).Error)

	groups := s.groups()
	s.Require().Len(groups, 1)
	for _, c := range groups[0].Copies {
		s.Zero(c.Comments, "a hidden comment is not counted on %s", c.Slug)
	}
}

// A show listed at two rooms is in both rooms' groups, and its suggestion says
// reattribute-shows will not merge it.
func (s *SameLocalDateTestSuite) TestAShowAtTwoRoomsIsInBothRoomsGroups() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	soda := s.room("soda-bar", "Soda Bar", zonePtr(losAngeles))
	act := s.artist("Two Room Act")
	evening := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	s.show(showSpec{slug: "both-rooms", at: evening, rooms: []uint{casbah.ID, soda.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "casbah-only", at: evening.Add(30 * time.Minute), rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "soda-only", at: evening.Add(45 * time.Minute), rooms: []uint{soda.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 2)
	for _, g := range groups {
		s.Contains(groupSlugs(g), "both-rooms", "room %s", g.Room.Slug)
		_, rows := SuggestSameLocalDateKeep(g)
		s.Require().Len(rows, 1)
		s.Contains(rows[0].Reason, "a copy lists more than one room")
	}
}

// A listed pair merged with ReattributeShow, the tool the worklist feeds: the
// row naming a copy to keep merges, saves and comments land on the kept copy,
// and a row naming no copy writes nothing.
func (s *SameLocalDateTestSuite) TestAListedPairMergesThroughReattributeShowOnlyWhenMarked() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	juice := s.artist("JUICE")
	ruby := s.artist("Ruby Haunt")
	music := time.Date(2026, 10, 18, 3, 30, 0, 0, time.UTC)
	keep := s.show(showSpec{slug: "juice-kept", at: music, musicAt: &music, rooms: []uint{casbah.ID}, acts: []uint{juice}})
	gone := s.show(showSpec{slug: "juice-merged", at: time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{juice}})
	rubyA := s.show(showSpec{slug: "ruby-a", at: time.Date(2026, 12, 10, 4, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{ruby}})
	rubyB := s.show(showSpec{slug: "ruby-b", at: time.Date(2026, 12, 10, 4, 30, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{ruby}})
	user := s.user("saver@test.local")
	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action)
		VALUES (?, 'show', ?, 'save')`, user, gone).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'tickets?', '<p>tickets?</p>')`, gone, user).Error)

	s.Require().Len(s.groups(), 2)

	marked := ReattributeShow(s.db, ShowReattribution{ShowSlug: "juice-merged", RoomSlug: "the-casbah", MergeIntoSlug: "juice-kept"})
	s.Require().Equal(ReattributionMerged, marked.Outcome, marked.Reason)
	unmarked := ReattributeShow(s.db, ShowReattribution{ShowSlug: "ruby-b", RoomSlug: "the-casbah"})
	s.Equal(ReattributionUnchanged, unmarked.Outcome, "a row naming no copy to keep writes nothing")

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", gone).Count(&n).Error)
	s.Zero(n, "the merged copy is gone")
	s.Require().NoError(s.db.Table("user_bookmarks").Where("entity_id = ? AND action = 'save'", keep).Count(&n).Error)
	s.Equal(int64(1), n, "the save lands on the kept copy")
	s.Require().NoError(s.db.Table("comments").Where("entity_type = 'show' AND entity_id = ?", keep).Count(&n).Error)
	s.Equal(int64(1), n, "the comment lands on the kept copy")
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id IN ?", []uint{rubyA, rubyB}).Count(&n).Error)
	s.Equal(int64(2), n, "the unmarked pair is untouched")

	groups := s.groups()
	s.Require().Len(groups, 1)
	s.Equal("Ruby Haunt", groups[0].HeadlinerName)
}

// --- the suggestion rule, without the database ---

type copyOpt func(*SameLocalDateCopy)

func parseLocal(s string) time.Time {
	at, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return at
}

// withMusic adds a stored music time, which states a start.
func withMusic(local string) copyOpt {
	return func(c *SameLocalDateCopy) {
		at := parseLocal(local)
		c.Clocks = append(c.Clocks, SameLocalDateClock{Label: "music", At: at, Local: at.Format("15:04")})
		c.StatesStart = true
	}
}

func withDoors(local string) copyOpt {
	return func(c *SameLocalDateCopy) {
		at := parseLocal(local)
		c.Clocks = append(c.Clocks, SameLocalDateClock{Label: "doors", At: at, Local: at.Format("15:04")})
	}
}

func withField(name, value string) copyOpt {
	return func(c *SameLocalDateCopy) {
		c.DroppedFields = append(c.DroppedFields, name)
		c.FieldValues[name] = value
	}
}

// copyAt builds a copy stored at a local clock. A stored start other than
// 20:00 states a start, as copyFromRow reads it.
func copyAt(id uint, stored string, created time.Time, opts ...copyOpt) SameLocalDateCopy {
	at := parseLocal(stored)
	c := SameLocalDateCopy{ShowID: id, Slug: fmt.Sprintf("show-%d", id), Title: "A Night Out", EventDate: at,
		Clocks:      []SameLocalDateClock{{Label: "stored start", At: at, Local: at.Format("15:04")}},
		StatesStart: at.Hour() != 20 || at.Minute() != 0, CreatedAt: created, RoomCount: 1,
		FieldValues: map[string]string{}}
	for _, o := range opts {
		o(&c)
	}
	return c
}

var (
	aug30 = time.Date(2026, 8, 30, 1, 11, 0, 0, time.UTC)
	sep13 = time.Date(2026, 9, 13, 23, 54, 0, 0, time.UTC)
)

func suggest(t *testing.T, copies ...SameLocalDateCopy) (*SameLocalDateCopy, []SameLocalDateSuggestion) {
	t.Helper()
	return SuggestSameLocalDateKeep(SameLocalDateGroup{Copies: copies})
}

func requireReason(t *testing.T, reason string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
}

func requireMerge(t *testing.T, wantKeep uint, copies ...SameLocalDateCopy) SameLocalDateSuggestion {
	t.Helper()
	keep, rows := suggest(t, copies...)
	if keep == nil || keep.ShowID != wantKeep || len(rows) != 1 || rows[0].Action != SameLocalDateMerge {
		t.Fatalf("want merge keeping %d, got keep=%v rows=%+v", wantKeep, keep, rows)
	}
	return rows[0]
}

func requireReview(t *testing.T, copies ...SameLocalDateCopy) SameLocalDateSuggestion {
	t.Helper()
	keep, rows := suggest(t, copies...)
	if keep != nil || len(rows) != len(copies) {
		t.Fatalf("want review with a row per copy, got keep=%v rows=%+v", keep, rows)
	}
	for _, r := range rows {
		if r.Action != SameLocalDateReview {
			t.Fatalf("want review, got %+v", r)
		}
	}
	return rows[0]
}

// The Casbah shape: an 8pm untimed copy and an 8:30 copy stating its music.
func TestRule_ThirtyMinutesApartIsAMergeIntoTheStatedStart(t *testing.T) {
	row := requireMerge(t, 2,
		copyAt(1, "2026-10-09 20:00", aug30),
		copyAt(2, "2026-10-09 20:30", sep13, withMusic("2026-10-09 20:30")))
	requireReason(t, row.Reason, "all 6 conditions hold", "it states a start")
	if row.Span != 30*time.Minute {
		t.Errorf("span %v", row.Span)
	}
}

// A doors time is a clock in the span: doors 19:00 on an 8pm row against a 21:00
// start spans two hours, which is review, not a merge into either.
func TestRule_DoorsAreAClockInTheSpan(t *testing.T) {
	row := requireReview(t,
		copyAt(1, "2026-10-09 20:00", aug30, withDoors("2026-10-09 19:00")),
		copyAt(2, "2026-10-09 21:00", sep13, withMusic("2026-10-09 21:00")))
	requireReason(t, row.Reason, "(6) the clocks span 120 minutes")
}

// A doors-only evening copy against a 17:00 matinee, both orders of doors: the
// span covers every clock, so neither reads as the other.
func TestRule_ADoorsOnlyCopyAgainstAStartInBothOrders(t *testing.T) {
	for _, doors := range []string{"2026-10-09 16:00", "2026-10-09 19:00"} {
		row := requireReview(t,
			copyAt(1, "2026-10-09 20:00", aug30, withDoors(doors)),
			copyAt(2, "2026-10-09 17:00", sep13))
		requireReason(t, row.Reason, "(6) the clocks span")
	}
}

// Doors within the span of one event merge, and the doors are named for
// copying over.
func TestRule_DoorsAnHourBeforeTheStartMerge(t *testing.T) {
	row := requireMerge(t, 2,
		copyAt(1, "2026-10-09 20:00", aug30, withDoors("2026-10-09 19:30")),
		copyAt(2, "2026-10-09 20:30", sep13, withMusic("2026-10-09 20:30")))
	requireReason(t, row.Reason, "this copy fills doors time 19:30, which the kept copy lacks")
}

// A 20:00 row whose music time is 15:00, next to a 15:00 matinee.
func TestRule_ARowWithAFarMusicTimeIsReview(t *testing.T) {
	row := requireReview(t,
		copyAt(1, "2026-10-09 20:00", aug30, withMusic("2026-10-09 15:00")),
		copyAt(2, "2026-10-09 15:00", sep13))
	requireReason(t, row.Reason, "the clocks span 5h; these may be two shows", "(6) the clocks span 300 minutes")
}

// A stated 4pm copy and an untimed 8pm copy.
func TestRule_AWidePairWithOneUntimedCopyIsReview(t *testing.T) {
	row := requireReview(t,
		copyAt(1, "2026-12-06 16:00", sep13),
		copyAt(2, "2026-12-06 20:00", aug30))
	requireReason(t, row.Reason, "these may be two shows")
}

// Early and late sets 150 minutes apart.
func TestRule_EarlyAndLateSetsAreReview(t *testing.T) {
	row := requireReview(t,
		copyAt(1, "2026-12-06 19:00", sep13),
		copyAt(2, "2026-12-06 21:30", aug30))
	requireReason(t, row.Reason, "(6) the clocks span 150 minutes")
	if strings.Contains(row.Reason, "these may be two shows") {
		t.Errorf("under three hours the two-shows phrase is not used: %q", row.Reason)
	}
}

func TestRule_AMatineeAndAnEveningShowAreReview(t *testing.T) {
	row := requireReview(t,
		copyAt(1, "2026-12-26 15:00", sep13, withMusic("2026-12-26 15:00")),
		copyAt(2, "2026-12-26 19:30", sep13, withMusic("2026-12-26 19:30")))
	requireReason(t, row.Reason, "the clocks span 4h30m; these may be two shows")
}

// Each of conditions 1 to 5 failing alone, on a pair whose clocks agree.
func TestRule_EachConditionFailingAloneIsReview(t *testing.T) {
	base := func() (SameLocalDateCopy, SameLocalDateCopy) {
		return copyAt(1, "2026-10-09 20:30", aug30), copyAt(2, "2026-10-09 20:45", sep13)
	}
	cases := map[string]func(a, b *SameLocalDateCopy) []SameLocalDateCopy{
		"(1) the group holds 3 copies": func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			return []SameLocalDateCopy{*a, *b, copyAt(3, "2026-10-09 20:50", sep13)}
		},
		"(1) show 2 has no slug": func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			b.Slug = ""
			return []SameLocalDateCopy{*a, *b}
		},
		"(2) one copy is cancelled and the other is not": func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			b.IsCancelled = true
			return []SameLocalDateCopy{*a, *b}
		},
		`(3) the titles differ ("A Night Out" and "Late Show")`: func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			b.Title = "Late Show"
			return []SameLocalDateCopy{*a, *b}
		},
		"(4) the scraper listed them as two events (evt-1 and evt-2)": func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			a.SourceEventID, b.SourceEventID = "evt-1", "evt-2"
			return []SameLocalDateCopy{*a, *b}
		},
		"(5) fields differ: age requirement 21+ and All Ages": func(a, b *SameLocalDateCopy) []SameLocalDateCopy {
			withField("age requirement", "21+")(a)
			withField("age requirement", "All Ages")(b)
			return []SameLocalDateCopy{*a, *b}
		},
	}
	for want, mutate := range cases {
		a, b := base()
		row := requireReview(t, mutate(&a, &b)...)
		requireReason(t, row.Reason, want)
	}
}

// Conditions that do not fail a pair: titles equal ignoring case and space,
// one scraper event id, both cancelled, a field filled on one side only.
func TestRule_ConditionsThatHold(t *testing.T) {
	a := copyAt(1, "2026-10-09 20:30", aug30, withField("price", "12.00"))
	b := copyAt(2, "2026-10-09 20:45", sep13)
	b.Title = " a night OUT "
	a.SourceEventID = "evt-1"
	a.IsCancelled, b.IsCancelled = true, true
	row := requireMerge(t, 1, a, b)
	for _, want := range []string{"(1) holds", "(2) holds", "(3) holds", "(4) holds", "(5) holds", "(6) holds"} {
		if !strings.Contains(strings.Join(row.Conditions, ", "), want) {
			t.Errorf("conditions %v lack %q", row.Conditions, want)
		}
	}
}

func TestRule_TwoTicketLinksToOneSalePageAreEqual(t *testing.T) {
	if ticketLinkKey("https://Tickets.example/e/123/?utm=x") != ticketLinkKey("https://tickets.example/e/123") {
		t.Error("host case, trailing slash and query should not tell two links apart")
	}
	if ticketLinkKey("https://tickets.example/e/123") == ticketLinkKey("https://tickets.example/e/124") {
		t.Error("two paths are two links")
	}
}

func TestRule_SavesAndCommentsDoNotDecideAndMove(t *testing.T) {
	saved := copyAt(1, "2026-10-09 20:00", aug30, withField("ticket link", "tickets.example/maris"))
	saved.Saves, saved.Comments = 4, 1
	row := requireMerge(t, 2, saved, copyAt(2, "2026-10-09 20:30", sep13, withMusic("2026-10-09 20:30")))
	requireReason(t, row.Reason, "this copy's 4 saves and 1 visible comments move to the kept copy",
		"this copy fills ticket link, which the kept copy lacks and the merge drops: copy it over by hand")
}

func TestRule_MoreDroppedFieldsOutrankTheOlderRow(t *testing.T) {
	row := requireMerge(t, 2,
		copyAt(1, "2026-10-09 20:30", aug30),
		copyAt(2, "2026-10-09 20:45", sep13, withField("ticket link", "x/1"), withField("price", "10.00")))
	requireReason(t, row.Reason, "it fills more of the fields a merge drops (ticket link, price) than this copy (none)")
}

func TestRule_ATieOnEverythingKeepsTheLowerID(t *testing.T) {
	row := requireMerge(t, 4, copyAt(9, "2026-12-13 20:45", sep13), copyAt(4, "2026-12-13 20:30", sep13))
	requireReason(t, row.Reason, "it has the lower id")
}

func TestRule_NamesALostSubmitterAndAScrapedCopy(t *testing.T) {
	gone := copyAt(2, "2026-10-09 20:45", sep13)
	gone.SubmittedBy = 7
	gone.Source = "discovery"
	row := requireMerge(t, 1, copyAt(1, "2026-10-09 20:30", aug30), gone)
	requireReason(t, row.Reason, "submitted by user 7, whose credit for it the merge drops", "discovery scraper")
}

// A spread of exactly SameShowClockSpan passes; one minute more fails.
func TestRule_TheClockSpanBoundary(t *testing.T) {
	requireMerge(t, 1, copyAt(1, "2026-10-09 18:30", aug30), copyAt(2, "2026-10-09 20:00", sep13, withMusic("2026-10-09 20:00")))
	row := requireReview(t, copyAt(1, "2026-10-09 18:29", aug30), copyAt(2, "2026-10-09 20:00", sep13, withMusic("2026-10-09 20:00")))
	requireReason(t, row.Reason, "(6) the clocks span 91 minutes")
}

// Two copies both stating a start, 15 minutes apart: the keep falls to field
// count, and the merged-away copy's start is named.
func TestRule_TwoStatedStartsNameTheDroppedOne(t *testing.T) {
	row := requireMerge(t, 2,
		copyAt(1, "2026-10-09 20:30", aug30),
		copyAt(2, "2026-10-09 20:45", sep13, withField("ticket link", "x/1")))
	requireReason(t, row.Reason, "it fills more of the fields",
		"the two state different starts (kept copy stored start 20:45; this copy stored start 20:30)")
}

func TestRule_TextIsBuiltFromTheConditionsAndConstants(t *testing.T) {
	for i, c := range mergeConditions {
		if !strings.Contains(SameLocalDateRuleText, fmt.Sprintf("(%d) %s", i+1, c.text)) {
			t.Errorf("rule text lacks condition %d", i+1)
		}
	}
	if !strings.Contains(SameLocalDateRuleText, "within 90 minutes") {
		t.Error("rule text does not state the clock span")
	}
	for _, f := range droppedFields {
		if !strings.Contains(SameLocalDateRuleText, f.name) {
			t.Errorf("rule text does not name %q", f.name)
		}
	}
}
