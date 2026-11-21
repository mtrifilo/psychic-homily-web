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
	slug      string
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
		INSERT INTO shows (title, slug, event_date, music_at, doors_at, is_cancelled, state, status, source, ticket_url,
		                   created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'CA', ?, 'user', ?, ?, ?) RETURNING id
	`, sp.slug, sp.slug, sp.at, sp.musicAt, sp.doorsAt, sp.cancelled, status, ticket, created, created).Row().Scan(&id))
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
	s.False(g.Copies[0].StatesStart(), "8pm with no music time states no start")
	s.Equal("20:00 PDT with no music time stored, so it may be the date-only anchor", g.Copies[0].Clock)
	s.True(g.Copies[1].StatesStart())
	s.Equal("music 20:30 PDT", g.Copies[1].Clock)
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
	s.Equal("15:00 PST", groups[0].Copies[0].LocalTime)
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
	s.False(groups[0].Copies[1].StatesStart(), "the state's zone reads the 03:00 UTC copy as 8pm, the date-only hour")
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
	s.Contains(rows[0].Reason, "states a start time (start 20:30 PDT)")
	s.Contains(rows[0].Reason, "this copy's 2 saves and 1 visible comments move to the kept copy")
}

// A listing with doors and no music time is stored at the date-only hour with
// its doors time. It states no start, and its doors time is what the matinee
// rule compares, so a doors-3pm copy and a 9pm copy are a matinee and an
// evening show.
func (s *SameLocalDateTestSuite) TestADoorsOnlyCopyStatesItsDoorsTime() {
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
	s.False(matinee.StatesStart())
	s.Equal("20:00 PDT with no music time stored, so it may be the date-only anchor, doors 15:00 PDT", matinee.Clock)
	s.Contains(matinee.DroppedFields, "doors time")
	_, rows := SuggestSameLocalDateKeep(groups[0])
	s.Require().Len(rows, 1)
	s.Equal(SameLocalDateKeepBoth, rows[0].Action, rows[0].Reason)
	s.Contains(rows[0].Reason, "doors 15:00 PDT")
	s.Contains(rows[0].Reason, "music 21:00 PDT")
	s.Contains(rows[0].Reason, "6h apart")
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

// copyAt builds a copy at a local clock. start says whether that clock is a
// stated start; doors, when set, is a doors clock on the same day.
func copyAt(id uint, local string, start bool, created time.Time, doors ...string) SameLocalDateCopy {
	parse := func(s string) time.Time {
		at, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			panic(err)
		}
		return at
	}
	at := parse(local)
	c := SameLocalDateCopy{ShowID: id, Slug: fmt.Sprintf("show-%d", id), EventDate: at,
		LocalTime: at.Format("15:04"), Clock: at.Format("15:04"), CreatedAt: created, RoomCount: 1}
	if start {
		c.StartAt = at
		c.Clock = "start " + at.Format("15:04")
	}
	if len(doors) > 0 {
		c.DoorsAt = parse(doors[0])
		c.DroppedFields = append(c.DroppedFields, "doors time")
		c.Clock += ", doors " + c.DoorsAt.Format("15:04")
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

func TestSuggestSameLocalDateKeep_MatineeAndEveningAreKeptBoth(t *testing.T) {
	keep, rows := suggest(t, copyAt(1, "2026-12-26 15:00", true, sep13), copyAt(2, "2026-12-26 19:30", true, sep13))
	if keep == nil || keep.ShowID != 1 || len(rows) != 1 || rows[0].Action != SameLocalDateKeepBoth {
		t.Fatalf("want keep both, got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "this copy start 19:30; the other start 15:00", "4h30m apart")
}

func TestSuggestSameLocalDateKeep_ADoorsOnlyMatineeIsKeptBothAndPrintsItsDoors(t *testing.T) {
	matinee := copyAt(1, "2026-10-09 20:00", false, sep13, "2026-10-09 15:00")
	evening := copyAt(2, "2026-10-09 21:00", true, sep13)
	_, rows := suggest(t, matinee, evening)
	if len(rows) != 1 || rows[0].Action != SameLocalDateKeepBoth {
		t.Fatalf("got %+v", rows)
	}
	requireReason(t, rows[0].Reason, "doors 15:00", "start 21:00", "6h apart")
}

// Doors and a real start an hour or two apart are one show; the copy stating
// the start is kept, whatever the doors-only copy holds.
func TestSuggestSameLocalDateKeep_AStartOutranksADoorsOnlyCopy(t *testing.T) {
	doorsOnly := copyAt(1, "2026-10-09 20:00", false, aug30, "2026-10-09 19:00")
	doorsOnly.TicketURL = "https://tickets.example/a"
	doorsOnly.DroppedFields = []string{"ticket link", "doors time"}
	start := copyAt(2, "2026-10-09 21:00", true, sep13)
	keep, rows := suggest(t, doorsOnly, start)
	if keep == nil || keep.ShowID != 2 || rows[0].Action != SameLocalDateMerge {
		t.Fatalf("want the stated start kept, got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "it states a start time (start 21:00)",
		"this copy holds ticket link, doors time, which the kept copy lacks")
}

// A pair a matinee gap apart where only one copy states a time may be a
// matinee and a real 8pm show entered with the form's default time.
func TestSuggestSameLocalDateKeep_AWidePairWithOneUntimedCopyIsReview(t *testing.T) {
	keep, rows := suggest(t, copyAt(1, "2026-12-06 16:00", true, sep13), copyAt(2, "2026-12-06 20:00", false, aug30))
	if keep != nil || len(rows) != 2 {
		t.Fatalf("want review rows for both copies, got keep=%v rows=%+v", keep, rows)
	}
	for _, r := range rows {
		if r.Action != SameLocalDateReview {
			t.Errorf("got %+v", r)
		}
		requireReason(t, r.Reason, "4h apart and only one states a time")
	}
}

func TestSuggestSameLocalDateKeep_ANarrowPairWithOneUntimedCopyMergesIntoTheStatedOne(t *testing.T) {
	keep, rows := suggest(t, copyAt(1, "2026-10-09 20:00", false, aug30), copyAt(2, "2026-10-09 20:30", true, sep13))
	if keep == nil || keep.ShowID != 2 || rows[0].Action != SameLocalDateMerge {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "it states a start time (start 20:30), and this copy does not (20:00)")
}

func TestSuggestSameLocalDateKeep_StatedStartsUnderThreeHoursApartMergeAndNameBothTimes(t *testing.T) {
	keep, rows := suggest(t, copyAt(1, "2026-12-06 19:00", true, sep13), copyAt(2, "2026-12-06 21:30", true, aug30))
	if keep == nil || keep.ShowID != 2 || rows[0].Action != SameLocalDateMerge {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "older row",
		"the two state different times (kept copy start 21:30; this copy start 19:00)")
}

func TestSuggestSameLocalDateKeep_SavesAndCommentsDoNotDecide(t *testing.T) {
	stated := copyAt(1, "2026-10-09 20:30", true, sep13)
	anchored := copyAt(2, "2026-10-09 20:00", false, aug30)
	anchored.TicketURL = "https://tickets.example/maris"
	anchored.DroppedFields = []string{"ticket link"}
	anchored.Saves = 4
	anchored.Comments = 1
	keep, rows := suggest(t, stated, anchored)
	if keep == nil || keep.ShowID != 1 {
		t.Fatalf("want the copy stating a start kept, got %v", keep)
	}
	requireReason(t, rows[0].Reason, "this copy's 4 saves and 1 visible comments move to the kept copy",
		"this copy holds ticket link, which the kept copy lacks and the merge drops: copy it over by hand")
}

func TestSuggestSameLocalDateKeep_MoreDroppedFieldsOutrankTheOlderRow(t *testing.T) {
	older := copyAt(1, "2026-10-09 20:30", true, aug30)
	fuller := copyAt(2, "2026-10-09 20:30", true, sep13)
	fuller.DroppedFields = []string{"ticket link", "price"}
	keep, rows := suggest(t, older, fuller)
	if keep == nil || keep.ShowID != 2 {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "holds more of the fields a merge drops (ticket link, price) than this copy (none)")
}

func TestSuggestSameLocalDateKeep_ATieOnEverythingKeepsTheLowerID(t *testing.T) {
	keep, rows := suggest(t, copyAt(9, "2026-12-13 20:00", false, sep13), copyAt(4, "2026-12-13 20:30", false, sep13))
	if keep == nil || keep.ShowID != 4 {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "lower id")
}

func TestSuggestSameLocalDateKeep_MoreThanTwoCopiesAreReviewWithARowEach(t *testing.T) {
	keep, rows := suggest(t,
		copyAt(1, "2026-12-26 15:00", true, aug30),
		copyAt(2, "2026-12-26 19:30", true, sep13),
		copyAt(3, "2026-12-26 19:45", true, sep13))
	if keep != nil || len(rows) != 3 {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	for _, r := range rows {
		if r.Action != SameLocalDateReview {
			t.Errorf("got %+v", r)
		}
		requireReason(t, r.Reason, "show-1, show-2, show-3")
	}
}

func TestSuggestSameLocalDateKeep_ACancelledCopyIsReview(t *testing.T) {
	cancelled := copyAt(2, "2026-10-09 20:00", false, aug30)
	cancelled.IsCancelled = true
	keep, rows := suggest(t, copyAt(1, "2026-10-09 20:30", true, sep13), cancelled)
	if keep != nil || len(rows) != 2 || rows[0].Action != SameLocalDateReview {
		t.Fatalf("got keep=%v rows=%+v", keep, rows)
	}
	requireReason(t, rows[0].Reason, "cancelled")
}

func TestSuggestSameLocalDateKeep_WarnsAboutAnyScrapedCopy(t *testing.T) {
	kept := copyAt(1, "2026-10-09 20:30", true, aug30)
	gone := copyAt(2, "2026-10-09 20:30", true, sep13)
	kept.Source, gone.Source = "discovery", "discovery"
	_, rows := suggest(t, kept, gone)
	requireReason(t, rows[0].Reason, "discovery scraper")
}

func TestSameLocalDateRuleTextNamesEveryDroppedField(t *testing.T) {
	for _, f := range droppedFields {
		if !strings.Contains(SameLocalDateRuleText, f.name) {
			t.Errorf("rule text does not name %q", f.name)
		}
	}
}
