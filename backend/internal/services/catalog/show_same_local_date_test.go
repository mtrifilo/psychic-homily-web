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
	slug    string
	at      time.Time
	musicAt *time.Time
	created time.Time
	status  string
	ticket  string
	rooms   []uint
	acts    []uint
}

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
	var id uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO shows (title, slug, event_date, music_at, state, status, source, ticket_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'CA', ?, 'user', ?, ?, ?) RETURNING id
	`, sp.slug, sp.slug, sp.at, sp.musicAt, status, ticket, created, created).Row().Scan(&id))
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
	s.True(g.Copies[0].DateOnlyAnchor, "8pm with no music time reads as the date-only anchor")
	s.False(g.Copies[1].DateOnlyAnchor)
	s.Equal("20:30 PDT", g.Copies[1].LocalTime)
}

func (s *SameLocalDateTestSuite) TestAPairAcrossMidnightUTCOnOneVenueLocalDayIsOneGroup() {
	casbah := s.room("the-casbah", "The Casbah", zonePtr(losAngeles))
	act := s.artist("The Beat Farmers")
	// 4pm Pacific is 23:00 UTC on the 6th; 8pm Pacific is 04:00 UTC on the 7th.
	s.show(showSpec{slug: "beat-farmers-evening", at: time.Date(2026, 12, 7, 4, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})
	s.show(showSpec{slug: "beat-farmers-afternoon", at: time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC),
		rooms: []uint{casbah.ID}, acts: []uint{act}})

	groups := s.groups()
	s.Require().Len(groups, 1)
	s.Equal("2026-12-06", groups[0].LocalDate)
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
	s.Equal("16:30 PDT", groups[0].Copies[0].LocalTime)
	s.True(groups[0].Copies[1].DateOnlyAnchor, "the state's zone reads the 03:00 UTC copy as 8pm")
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
	s.Require().NoError(s.db.Exec(bookmark, users[0], newer, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, users[1], newer, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, users[0], older, "follow").Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'see you', '<p>see you</p>')`, newer, users[1]).Error)

	groups := s.groups()
	s.Require().Len(groups, 1)
	byID := map[uint]SameLocalDateCopy{}
	for _, c := range groups[0].Copies {
		byID[c.ShowID] = c
	}
	s.Equal(int64(2), byID[newer].Saves)
	s.Equal(int64(1), byID[newer].Comments)
	s.Zero(byID[older].Saves, "a follow is not a save")
	s.Zero(byID[older].Comments)

	keep, rows := SuggestSameLocalDateKeep(groups[0])
	s.Equal(newer, keep.ShowID, "the copy with saves and comments is kept over the older row")
	s.Require().Len(rows, 1)
	s.Equal(SameLocalDateMerge, rows[0].Action)
	s.Contains(rows[0].Reason, "it has 2 saves and 1 comments")
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

func copyAt(id uint, local string, anchor bool, created time.Time) SameLocalDateCopy {
	at, err := time.Parse("2006-01-02 15:04", local)
	if err != nil {
		panic(err)
	}
	return SameLocalDateCopy{ShowID: id, Slug: fmt.Sprintf("show-%d", id), EventDate: at,
		LocalTime: at.Format("15:04"), DateOnlyAnchor: anchor, CreatedAt: created, RoomCount: 1}
}

var (
	aug30 = time.Date(2026, 8, 30, 1, 11, 0, 0, time.UTC)
	sep13 = time.Date(2026, 9, 13, 23, 54, 0, 0, time.UTC)
)

func TestSuggestSameLocalDateKeep_MatineeAndEveningAreKeptBoth(t *testing.T) {
	g := SameLocalDateGroup{Copies: []SameLocalDateCopy{
		copyAt(1, "2026-12-26 15:00", false, sep13),
		copyAt(2, "2026-12-26 19:30", false, sep13),
	}}
	keep, rows := SuggestSameLocalDateKeep(g)
	if keep.ShowID != 1 || len(rows) != 1 || rows[0].Action != SameLocalDateKeepBoth {
		t.Fatalf("want keep both, got keep=%d rows=%+v", keep.ShowID, rows)
	}
	if want := "4h30m apart"; !strings.Contains(rows[0].Reason, want) {
		t.Errorf("reason %q lacks %q", rows[0].Reason, want)
	}
}

func TestSuggestSameLocalDateKeep_AnAnchorIsNotAStatedTime(t *testing.T) {
	// 4pm stated and the 8pm anchor, four hours apart: not a matinee pair,
	// because only one copy states a time.
	g := SameLocalDateGroup{Copies: []SameLocalDateCopy{
		copyAt(1, "2026-12-06 16:00", false, sep13),
		copyAt(2, "2026-12-06 20:00", true, aug30),
	}}
	keep, rows := SuggestSameLocalDateKeep(g)
	if keep.ShowID != 1 || rows[0].Action != SameLocalDateMerge {
		t.Fatalf("want merge into the stated copy, got keep=%d rows=%+v", keep.ShowID, rows)
	}
	if !strings.Contains(rows[0].Reason, "states a time (16:00)") {
		t.Errorf("reason %q", rows[0].Reason)
	}
}

func TestSuggestSameLocalDateKeep_StatedTimesUnderThreeHoursApartMerge(t *testing.T) {
	g := SameLocalDateGroup{Copies: []SameLocalDateCopy{
		copyAt(1, "2026-12-06 19:00", false, sep13),
		copyAt(2, "2026-12-06 21:30", false, aug30),
	}}
	keep, rows := SuggestSameLocalDateKeep(g)
	if keep.ShowID != 2 || rows[0].Action != SameLocalDateMerge || !strings.Contains(rows[0].Reason, "older row") {
		t.Fatalf("want merge into the older row, got keep=%d rows=%+v", keep.ShowID, rows)
	}
}

func TestSuggestSameLocalDateKeep_EngagementOutranksAStatedTimeAndSaysWhatIsLost(t *testing.T) {
	stated := copyAt(1, "2026-10-09 20:30", false, sep13)
	stated.TicketURL = "https://tickets.example/maris"
	anchored := copyAt(2, "2026-10-09 20:00", true, sep13)
	anchored.Comments = 1
	keep, rows := SuggestSameLocalDateKeep(SameLocalDateGroup{Copies: []SameLocalDateCopy{stated, anchored}})
	if keep.ShowID != 2 {
		t.Fatalf("want the commented copy kept, got %d", keep.ShowID)
	}
	for _, want := range []string{"0 saves and 1 comments", "time (20:30) is not carried", "ticket link is not carried"} {
		if !strings.Contains(rows[0].Reason, want) {
			t.Errorf("reason %q lacks %q", rows[0].Reason, want)
		}
	}
}

func TestSuggestSameLocalDateKeep_ATieOnEverythingKeepsTheLowerID(t *testing.T) {
	g := SameLocalDateGroup{Copies: []SameLocalDateCopy{
		copyAt(9, "2026-12-13 20:00", true, sep13),
		copyAt(4, "2026-12-13 20:30", true, sep13),
	}}
	keep, rows := SuggestSameLocalDateKeep(g)
	if keep.ShowID != 4 || !strings.Contains(rows[0].Reason, "lower id") {
		t.Fatalf("got keep=%d rows=%+v", keep.ShowID, rows)
	}
}

func TestSuggestSameLocalDateKeep_ThreeCopiesEachGetARow(t *testing.T) {
	g := SameLocalDateGroup{Copies: []SameLocalDateCopy{
		copyAt(1, "2026-12-26 15:00", false, aug30),
		copyAt(2, "2026-12-26 15:30", false, sep13),
		copyAt(3, "2026-12-26 19:30", false, sep13),
	}}
	keep, rows := SuggestSameLocalDateKeep(g)
	if keep.ShowID != 1 || len(rows) != 2 {
		t.Fatalf("got keep=%d rows=%+v", keep.ShowID, rows)
	}
	if rows[0].Copy.ShowID != 2 || rows[0].Action != SameLocalDateMerge {
		t.Errorf("the 15:30 copy merges into the 15:00 one: %+v", rows[0])
	}
	if rows[1].Copy.ShowID != 3 || rows[1].Action != SameLocalDateKeepBoth {
		t.Errorf("the 19:30 copy is a second show: %+v", rows[1])
	}
}
