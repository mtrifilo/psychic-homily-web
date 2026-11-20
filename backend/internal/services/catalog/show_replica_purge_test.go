package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

type ShowReplicaPurgeSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	backups []json.RawMessage
}

// keepBackup is the backup writer every purge in this suite uses: it records
// the snapshots so a test can read what a purge was handed before deleting.
func (s *ShowReplicaPurgeSuite) keepBackup(docs []json.RawMessage) error {
	s.backups = append(s.backups, docs...)
	return nil
}

func TestShowReplicaPurgeSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ShowReplicaPurgeSuite))
}

func (s *ShowReplicaPurgeSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ShowReplicaPurgeSuite) TearDownSuite() {
	if s.testDB != nil {
		s.testDB.Cleanup()
	}
}

func (s *ShowReplicaPurgeSuite) TearDownTest() {
	s.backups = nil
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{
		"comment_subscriptions", "comment_last_read", "comments", "notification_log",
		"show_notify_queue", "show_reports", "collection_items", "collections", "user_bookmarks",
		"show_artists", "show_venues", "shows", "artists", "venues", "users",
	} {
		_, _ = sqlDB.Exec(fmt.Sprintf("DELETE FROM %s", t))
	}
}

// replicaNow is the planning instant every test pins: venue-local
// 2026-11-20 in San Francisco, after the Oct 3 2026 show it seeds.
var replicaNow = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

const replicaTicket = "https://wl.seetickets.us/event/igorrr/691689?afflky=GreatAmericanMusicHall"

func (s *ShowReplicaPurgeSuite) venue(name string) uint {
	slug := name
	v := &catalogm.Venue{
		Name: name, Slug: &slug, City: "San Francisco", State: "CA", Verified: true,
		Timezone: stringPtr("America/Los_Angeles"),
	}
	s.Require().NoError(s.db.Create(v).Error)
	return v.ID
}

func (s *ShowReplicaPurgeSuite) artist(name string) uint {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *ShowReplicaPurgeSuite) user(email string) uint {
	u := &authm.User{Email: stringPtr(email), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u.ID
}

// evening is 20:00 Pacific on the given local day, which is 03:00Z the next
// UTC day in October: the shape the ingest CLI writes for a date-only show.
func evening(year int, month time.Month, day int) time.Time {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return time.Date(year, month, day, 20, 0, 0, 0, la).UTC()
}

// replicaCreated is when every seeded show was written unless a test says
// otherwise: one moment, the shape of one write fanning a date out.
var replicaCreated = replicaNow.AddDate(0, -2, 0)

// show inserts an approved show headlined by artistID at venueID, created at
// replicaCreated by no submitter. An empty ticket stores NULL.
func (s *ShowReplicaPurgeSuite) show(artistID, venueID uint, at time.Time, ticket string) uint {
	return s.showFrom(artistID, venueID, at, ticket, replicaCreated, nil)
}

// showFrom is show with the creation time and submitter stated.
func (s *ShowReplicaPurgeSuite) showFrom(artistID, venueID uint, at time.Time, ticket string, created time.Time, submittedBy *uint) uint {
	var ticketArg any
	if ticket != "" {
		ticketArg = ticket
	}
	slug := fmt.Sprintf("%s-a%d-v%d", at.Format("2006-01-02T15"), artistID, venueID)
	var id uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO shows (title, event_date, state, status, source, ticket_url, slug, submitted_by, created_at, updated_at)
		VALUES ('', ?, 'CA', 'approved', 'user', ?, ?, ?, ?, ?)
		RETURNING id`, at, ticketArg, slug, submittedBy, created, created).
		Row().Scan(&id))
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')`,
		id, artistID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, venueID).Error)
	return id
}

// replicate seeds one event on Oct 3 in each of the given years, all with the
// same ticket URL, and returns the ids in year order.
func (s *ShowReplicaPurgeSuite) replicate(artistID, venueID uint, years ...int) []uint {
	ids := make([]uint, 0, len(years))
	for _, y := range years {
		ids = append(ids, s.show(artistID, venueID, evening(y, time.October, 3), replicaTicket))
	}
	return ids
}

func (s *ShowReplicaPurgeSuite) plan() *ReplicatedShowPlan {
	plan, err := PlanReplicatedShowPurge(s.db, replicaNow, 18)
	s.Require().NoError(err)
	return plan
}

func (s *ShowReplicaPurgeSuite) existing(ids []uint) []uint {
	var got []uint
	s.Require().NoError(s.db.Raw(`SELECT id FROM shows WHERE id IN ? ORDER BY id`, ids).Scan(&got).Error)
	return got
}

func (s *ShowReplicaPurgeSuite) count(sql string, args ...any) int64 {
	var n int64
	s.Require().NoError(s.db.Raw(sql, args...).Scan(&n).Error)
	return n
}

// The production shape: an event on Oct 3 copied into five later years. The
// survivor is the current-year copy even though its date has passed, and the
// delete path drops the copies' bookmarks while keeping their comments.
func (s *ShowReplicaPurgeSuite) TestReplicatedGroup_KeepsCurrentYearCopyAndDeletesTheRest() {
	room := s.venue("Great American Music Hall")
	igorrr := s.artist("Igorrr")
	ids := s.replicate(igorrr, room, 2026, 2027, 2028, 2029, 2030, 2031)
	bystander := s.show(igorrr, room, evening(2027, time.March, 1), replicaTicket)

	fan := s.user("fan@test.local")
	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, 'save')`, fan, ids[0]).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, 'save')`, fan, ids[2]).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html) VALUES ('show', ?, ?, 'see you', '<p>see you</p>')`, ids[3], fan).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_reports (show_id, reported_by, report_type) VALUES (?, ?, 'inaccurate')`, ids[4], fan).Error)

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	g := plan.Groups[0]
	s.Empty(g.SkipReason)
	s.True(g.Actionable())
	s.Equal(2026, g.CurrentYear)
	s.Equal("10-03", g.MonthDay)
	s.Equal("Igorrr", g.HeadlinerName)
	s.Equal("Great American Music Hall", g.VenueName)
	s.Equal(ids[0], g.KeepID, "the passed 2026 copy survives: its YEAR is the current one")
	s.Equal(ids[1:], g.DeleteIDs)
	s.Require().Len(g.Copies, 6)
	s.Equal("2026-10-03", g.Copies[0].LocalDate, "grouped on the venue-local day, not the UTC one")
	s.Equal([]ReplicaRefCount{{Table: "user_bookmarks", Dropped: true, Rows: 1}}, g.Copies[2].Refs)
	s.Equal([]ReplicaRefCount{{Table: "comments", Dropped: false, Rows: 1}}, g.Copies[3].Refs)
	s.Equal([]ReplicaRefCount{{Table: "show_reports (cascade)", Dropped: true, Rows: 1}}, g.Copies[4].Refs)
	s.Empty(g.Copies[1].Refs)
	s.Empty(plan.Beyond, "every far-future copy belongs to the group")

	s.Require().NoError(PurgeReplicatedShowGroup(s.db, replicaNow, g, s.keepBackup))

	// Every deleted show was handed to the backup first, with its rows.
	s.Require().Len(s.backups, 5)
	var doc struct {
		Show struct {
			ID uint `json:"id"`
		} `json:"show"`
		ShowArtists []json.RawMessage `json:"show_artists"`
		ShowVenues  []json.RawMessage `json:"show_venues"`
		Bookmarks   []json.RawMessage `json:"user_bookmarks"`
		Reports     []json.RawMessage `json:"show_reports"`
		Comments    json.RawMessage   `json:"comments"`
	}
	s.Require().NoError(json.Unmarshal(s.backups[1], &doc))
	s.Equal(ids[2], doc.Show.ID)
	s.Len(doc.ShowArtists, 1)
	s.Len(doc.ShowVenues, 1)
	s.Len(doc.Bookmarks, 1)
	s.Require().NoError(json.Unmarshal(s.backups[3], &doc))
	s.Equal(ids[4], doc.Show.ID)
	s.Len(doc.Reports, 1)
	s.Require().NoError(json.Unmarshal(s.backups[2], &doc))
	s.Equal(ids[3], doc.Show.ID)
	s.Nil(doc.Comments, "comments stay in the database as tombstones, so the backup does not copy them")

	s.Equal([]uint{ids[0]}, s.existing(ids))
	s.Equal([]uint{bystander}, s.existing([]uint{bystander}), "a different month-day is a different group")
	for _, id := range ids[1:] {
		_, err := NewShowService(s.db).GetShow(id)
		var showErr *apperrors.ShowError
		s.Require().True(errors.As(err, &showErr), "show %d: %v", id, err)
		s.Equal(apperrors.CodeShowNotFound, showErr.Code)
	}
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type='show' AND entity_id = ?`, ids[0]))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type='show' AND entity_id = ?`, ids[2]))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM comments WHERE entity_type='show' AND entity_id = ?`, ids[3]),
		"comments are kept as tombstones by the delete path")
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM show_artists WHERE show_id IN ?`, ids[1:]))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM show_venues WHERE show_id IN ?`, ids[1:]))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM show_reports WHERE show_id = ?`, ids[4]))

	again := s.plan()
	s.Empty(again.Groups, "a second run finds nothing")
}

func (s *ShowReplicaPurgeSuite) TestSingleCopyIsNotAGroup() {
	room := s.venue("Great American Music Hall")
	id := s.show(s.artist("Beth Orton"), room, evening(2027, time.September, 30), replicaTicket)

	plan := s.plan()
	s.Empty(plan.Groups)
	s.Empty(plan.Beyond)
	s.Equal([]uint{id}, s.existing([]uint{id}))
}

// Same-date duplicates are another class of problem: they are not a
// replicated group, so this tool neither lists nor touches them.
func (s *ShowReplicaPurgeSuite) TestSameDateDuplicatesAreNotAGroup() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Beth Orton")
	var a, b uint
	seedLegacyShowDuplicate(s.T(), s.db, func() {
		a = s.show(act, room, evening(2027, time.September, 30), replicaTicket)
		b = s.show(act, room, evening(2027, time.September, 30).Add(time.Hour), replicaTicket)
	})

	s.Empty(s.plan().Groups)
	s.Equal([]uint{a, b}, s.existing([]uint{a, b}))
}

// Two copies in one year beside a copy in another year is ambiguous.
func (s *ShowReplicaPurgeSuite) TestTwoCopiesInOneYearAreUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Beth Orton")
	var ids []uint
	seedLegacyShowDuplicate(s.T(), s.db, func() {
		ids = append(ids, s.show(act, room, evening(2026, time.September, 30), replicaTicket))
		ids = append(ids, s.show(act, room, evening(2026, time.September, 30).Add(time.Hour), replicaTicket))
		ids = append(ids, s.show(act, room, evening(2027, time.September, 30), replicaTicket))
	})
	s.assertUntouched(replicaSkipSameYear, ids)
}

func (s *ShowReplicaPurgeSuite) TestNextYearCopySurvivesWhenNoCurrentYearCopy() {
	ids := s.replicate(s.artist("Groundation"), s.venue("Great American Music Hall"), 2027, 2028, 2029)

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Equal(ids[0], plan.Groups[0].KeepID)
	s.Equal(ids[1:], plan.Groups[0].DeleteIDs)
}

func (s *ShowReplicaPurgeSuite) assertUntouched(reason string, ids []uint) {
	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	g := plan.Groups[0]
	s.Equal(reason, g.SkipReason)
	s.False(g.Actionable())
	s.Zero(g.KeepID)
	s.Empty(g.DeleteIDs)
	s.Error(PurgeReplicatedShowGroup(s.db, replicaNow, g, s.keepBackup), "an untouched group is refused at the delete call too")
	s.Equal(ids, s.existing(ids))
}

func (s *ShowReplicaPurgeSuite) TestDifferentTicketURLsAreUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Chanel Beads")
	a := s.show(act, room, evening(2026, time.December, 31), "https://tix.example/nye-2026")
	b := s.show(act, room, evening(2027, time.December, 31), "https://tix.example/nye-2027")
	s.assertUntouched(replicaSkipTicketURLsDiffer, []uint{a, b})
}

func (s *ShowReplicaPurgeSuite) TestMissingTicketURLIsUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Chanel Beads")
	a := s.show(act, room, evening(2026, time.December, 31), "")
	b := s.show(act, room, evening(2027, time.December, 31), "")
	s.assertUntouched(replicaSkipNoTicketURL, []uint{a, b})
}

func (s *ShowReplicaPurgeSuite) TestNoCopyInCurrentOrNextYearIsUntouched() {
	ids := s.replicate(s.artist("The Menzingers"), s.venue("Great American Music Hall"), 2029, 2030)
	s.assertUntouched(replicaSkipNoNearCopy, ids)
}

func (s *ShowReplicaPurgeSuite) TestCopyInAPastYearIsUntouched() {
	ids := s.replicate(s.artist("John Craigie"), s.venue("Great American Music Hall"), 2025, 2026, 2027)
	s.assertUntouched(replicaSkipPastYear, ids)
}

func (s *ShowReplicaPurgeSuite) TestOtherRoomOrOtherHeadlinerDoesNotGroup() {
	gamh := s.venue("Great American Music Hall")
	other := s.venue("The Independent")
	act := s.artist("Phillip Phillips")
	a := s.show(act, gamh, evening(2026, time.October, 9), replicaTicket)
	b := s.show(act, other, evening(2027, time.October, 9), replicaTicket)
	c := s.show(s.artist("Someone Else"), gamh, evening(2028, time.October, 9), replicaTicket)

	plan := s.plan()
	s.Empty(plan.Groups)
	s.Equal([]uint{a, b, c}, s.existing([]uint{a, b, c}))
}

func (s *ShowReplicaPurgeSuite) TestBeyondListsOnlyUngroupedFarFutureShows() {
	s.replicate(s.artist("Igorrr"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	ace := s.show(s.artist("Testing Ace"), s.venue("Variety Playhouse"),
		time.Date(2083, 8, 15, 23, 0, 0, 0, time.UTC), "https://www.axs.com/events/1605473/testing-ace-tickets")

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Require().Len(plan.Beyond, 1)
	s.Equal(ace, plan.Beyond[0].ShowID)
	s.Equal("Testing Ace", plan.Beyond[0].HeadlinerName)
	s.Equal(replicaNow.AddDate(0, 18, 0), plan.BeyondCutoff)

	s.Require().NoError(PurgeReplicatedShowGroup(s.db, replicaNow, plan.Groups[0], s.keepBackup))
	s.Equal([]uint{ace}, s.existing([]uint{ace}), "the far-future report never deletes")
}

func (s *ShowReplicaPurgeSuite) TestGroupChangedSincePlanDeletesNothing() {
	ids := s.replicate(s.artist("Los Thuthanaka"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	plan := s.plan()
	s.Require().Len(plan.Groups, 1)

	s.Require().NoError(s.db.Exec(`UPDATE shows SET ticket_url = 'https://tix.example/fixed' WHERE id = ?`, ids[2]).Error)

	err := PurgeReplicatedShowGroup(s.db, replicaNow, plan.Groups[0], s.keepBackup)
	s.Require().ErrorIs(err, ErrReplicaGroupChanged)
	s.Equal(ids, s.existing(ids))
}

func (s *ShowReplicaPurgeSuite) TestGroupChangedSincePlan_UnapprovedOrRedatedCopy() {
	for name, change := range map[string]string{
		"unapproved": `UPDATE shows SET status = 'private' WHERE id = ?`,
		"re-dated":   `UPDATE shows SET event_date = event_date + interval '1 day' WHERE id = ?`,
		"deleted":    `DELETE FROM shows WHERE id = ?`,
	} {
		s.Run(name, func() {
			s.TearDownTest()
			ids := s.replicate(s.artist("Los Thuthanaka "+name), s.venue("Great American Music Hall "+name), 2026, 2027, 2028)
			plan := s.plan()
			s.Require().Len(plan.Groups, 1)

			s.Require().NoError(s.db.Exec(change, ids[1]).Error)

			err := PurgeReplicatedShowGroup(s.db, replicaNow, plan.Groups[0], s.keepBackup)
			s.Require().ErrorIs(err, ErrReplicaGroupChanged)
			s.Empty(s.backups, "nothing is handed to the backup for a changed group")
			survivors := []uint{ids[0], ids[2]}
			if name != "deleted" {
				survivors = ids
			}
			s.Equal(survivors, s.existing(ids))
		})
	}
}

func (s *ShowReplicaPurgeSuite) TestBackupFailureDeletesNothing() {
	ids := s.replicate(s.artist("Groundation"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	plan := s.plan()
	s.Require().Len(plan.Groups, 1)

	err := PurgeReplicatedShowGroup(s.db, replicaNow, plan.Groups[0], func([]json.RawMessage) error {
		return errors.New("disk full")
	})
	s.Require().ErrorContains(err, "disk full")
	s.Equal(ids, s.existing(ids))
}

// A submitter who adds a show in an earlier year cannot make it the survivor
// of another submitter's copies: mixed origin leaves the group alone.
func (s *ShowReplicaPurgeSuite) TestMixedSubmittersAreUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Igorrr")
	owner := s.user("owner@test.local")
	other := s.user("other@test.local")
	a := s.showFrom(act, room, evening(2026, time.October, 3), replicaTicket, replicaCreated, &other)
	b := s.showFrom(act, room, evening(2027, time.October, 3), replicaTicket, replicaCreated, &owner)
	c := s.showFrom(act, room, evening(2028, time.October, 3), replicaTicket, replicaCreated, &owner)
	s.assertUntouched(replicaSkipMixedOrigin, []uint{a, b, c})
}

// A show that really recurs was written at different times; copies written
// more than the window apart are left for a human.
func (s *ShowReplicaPurgeSuite) TestCopiesCreatedApartAreUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("NYE Band")
	a := s.showFrom(act, room, evening(2026, time.December, 31), replicaTicket, replicaCreated, nil)
	b := s.showFrom(act, room, evening(2027, time.December, 31), replicaTicket, replicaCreated, nil)
	c := s.showFrom(act, room, evening(2028, time.December, 31), replicaTicket, replicaCreated.Add(replicaCreatedWindow+time.Second), nil)
	s.assertUntouched(replicaSkipCreatedApart, []uint{a, b, c})
}

// The survivor may be written at any time; only the copies to delete must be
// close together.
func (s *ShowReplicaPurgeSuite) TestSurvivorCreatedEarlierStillPurges() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Igorrr")
	a := s.showFrom(act, room, evening(2026, time.October, 3), replicaTicket, replicaCreated.AddDate(0, -1, 0), nil)
	b := s.showFrom(act, room, evening(2027, time.October, 3), replicaTicket, replicaCreated, nil)
	c := s.showFrom(act, room, evening(2028, time.October, 3), replicaTicket, replicaCreated.Add(replicaCreatedWindow), nil)

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Equal(a, plan.Groups[0].KeepID)
	s.Equal([]uint{b, c}, plan.Groups[0].DeleteIDs)
}

// With one copy to delete there is no spread to measure, so that copy must have
// been written with the copy kept.
func (s *ShowReplicaPurgeSuite) TestPairCreatedApartIsUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Annual Act")
	a := s.showFrom(act, room, evening(2027, time.June, 14), replicaTicket, replicaCreated.AddDate(-1, 0, 0), nil)
	b := s.showFrom(act, room, evening(2028, time.June, 14), replicaTicket, replicaCreated, nil)
	s.assertUntouched(replicaSkipPairCreatedApart, []uint{a, b})
}

func (s *ShowReplicaPurgeSuite) TestPairCreatedTogetherPurges() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Igorrr")
	a := s.showFrom(act, room, evening(2027, time.June, 14), replicaTicket, replicaCreated, nil)
	b := s.showFrom(act, room, evening(2028, time.June, 14), replicaTicket, replicaCreated.Add(time.Minute), nil)

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Equal(a, plan.Groups[0].KeepID)
	s.Equal([]uint{b}, plan.Groups[0].DeleteIDs)
}

// Same submitter (none) but a different source is a different origin.
func (s *ShowReplicaPurgeSuite) TestMixedSourcesAreUntouched() {
	ids := s.replicate(s.artist("Groundation"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	s.Require().NoError(s.db.Exec(`UPDATE shows SET source = 'discovery' WHERE id = ?`, ids[1]).Error)
	s.assertUntouched(replicaSkipMixedOrigin, ids)
}

// Stored whitespace around a link is ignored, and a link ending in "v" keeps
// its "v": the trim set must not read \v as a letter.
func (s *ShowReplicaPurgeSuite) TestTicketURLTrimKeepsATrailingV() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Igorrr")
	nov := "https://dice.fm/event/abc-nov"
	a := s.show(act, room, evening(2026, time.October, 3), nov+"\n")
	b := s.show(act, room, evening(2027, time.October, 3), " "+nov)
	c := s.show(act, room, evening(2028, time.October, 3), nov)
	d := s.show(s.artist("Other"), room, evening(2028, time.October, 3), "https://dice.fm/event/abc-no")

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Equal(nov, plan.Groups[0].Copies[0].TicketURL)
	s.Equal(a, plan.Groups[0].KeepID)
	s.Equal([]uint{b, c}, plan.Groups[0].DeleteIDs)

	shows, total, err := NewShowService(s.db).GetShows(map[string]interface{}{"ticket_url": nov}, contracts.ShowsQuery{Limit: 50})
	s.Require().NoError(err)
	s.Equal(int64(3), total)
	for _, sh := range shows {
		s.NotEqual(d, sh.ID)
	}
}

// Two years inside the bound and nothing past it is not the replication
// signature; it is left for a human.
func (s *ShowReplicaPurgeSuite) TestNoCopyPastTheBoundIsUntouched() {
	ids := s.replicate(s.artist("Phillip Phillips"), s.venue("Great American Music Hall"), 2026, 2027)
	s.assertUntouched(replicaSkipNoFarCopy, ids)
}

// The ticket URL is a condition on the whole bucket: one stray link keeps
// every copy at that room, headliner and month-day untouched and listed.
func (s *ShowReplicaPurgeSuite) TestOneDifferentURLKeepsTheWholeBucketUntouched() {
	room := s.venue("Great American Music Hall")
	act := s.artist("Igorrr")
	a := s.show(act, room, evening(2026, time.October, 3), replicaTicket)
	b := s.show(act, room, evening(2027, time.October, 3), "https://tix.example/igorrr-2027")
	c := s.show(act, room, evening(2028, time.October, 3), replicaTicket)
	s.assertUntouched(replicaSkipTicketURLsDiffer, []uint{a, b, c})
}

// The group's headliner is the curated one, not the first-billed act, so a
// curated headliner billed second still groups with its copies.
func (s *ShowReplicaPurgeSuite) TestHeadlinerIsTheCuratedAct() {
	room := s.venue("Great American Music Hall")
	opener := s.artist("Opener")
	star := s.artist("Star")
	var ids []uint
	for _, y := range []int{2026, 2027, 2028} {
		var id uint
		s.Require().NoError(s.db.Raw(`
			INSERT INTO shows (title, event_date, state, status, source, ticket_url, slug, created_at, updated_at)
			VALUES ('', ?, 'CA', 'approved', 'user', ?, ?, ?, ?) RETURNING id`,
			evening(y, time.October, 3), replicaTicket, fmt.Sprintf("star-%d", y), replicaCreated, replicaCreated).Row().Scan(&id))
		// The opener sits at position 0 in odd years and 1 in even years, so a
		// key that read position alone would split the copies apart.
		openerPos, starPos := 1, 0
		if y%2 == 1 {
			openerPos, starPos = 0, 1
		}
		s.Require().NoError(s.db.Exec(`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, 'opener'), (?, ?, ?, 'headliner')`,
			id, opener, openerPos, id, star, starPos).Error)
		s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, room).Error)
		ids = append(ids, id)
	}

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	s.Equal("Star", plan.Groups[0].HeadlinerName)
	s.Equal(ids[0], plan.Groups[0].KeepID)
	s.Equal(ids[1:], plan.Groups[0].DeleteIDs)
}

// Every foreign key to shows is either counted and snapshotted (showFKRefs) or
// named here as handled another way, so a new cascade cannot be deleted
// without a count in the dry run or a copy in the backup.
func (s *ShowReplicaPurgeSuite) TestShowFKRefsCoverTheSchema() {
	var fks []struct {
		Tbl string
		Col string
	}
	s.Require().NoError(s.db.Raw(`
		SELECT c.conrelid::regclass::text AS tbl, a.attname AS col
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
		WHERE c.contype = 'f' AND c.confrelid = 'shows'::regclass`).Scan(&fks).Error)
	s.Require().NotEmpty(fks)

	known := map[string]bool{
		"show_artists.show_id":    true, // snapshotted directly
		"show_venues.show_id":     true, // snapshotted directly
		"show_dedup_keys.show_id": true, // derived by trigger from the two above
	}
	for _, fk := range showFKRefs {
		known[fk.table+"."+fk.column] = true
	}
	for _, fk := range fks {
		s.True(known[fk.Tbl+"."+fk.Col], "foreign key %s.%s to shows is not in showFKRefs or the exempt set", fk.Tbl, fk.Col)
	}
}

// The snapshot carries every kind of row the delete removes or rewrites, and
// the dry run counts each of them.
func (s *ShowReplicaPurgeSuite) TestSnapshotCarriesEveryRemovedRow() {
	ids := s.replicate(s.artist("Igorrr"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	doomed := ids[2]
	fan := s.user("fan@test.local")

	var collectionID uint
	s.Require().NoError(s.db.Raw(`INSERT INTO collections (title, slug, creator_id) VALUES ('Crate', 'crate', ?) RETURNING id`, fan).Row().Scan(&collectionID))
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id) VALUES (?, 'show', ?, 0, ?)`, []any{collectionID, doomed, fan}},
		{`INSERT INTO notification_log (user_id, entity_type, entity_id, channel) VALUES (?, 'show', ?, 'email')`, []any{fan, doomed}},
		{`INSERT INTO notification_log (user_id, entity_type, entity_id, channel) VALUES (?, ?, ?, 'email')`, []any{fan, notificationm.NotificationEntityArtistShowAlert, doomed}},
	} {
		s.Require().NoError(s.db.Exec(stmt.sql, stmt.args...).Error)
	}
	pointer := s.show(s.artist("Elsewhere"), s.venue("Elsewhere Room"), evening(2027, time.January, 9), "")
	s.Require().NoError(s.db.Exec(`UPDATE shows SET duplicate_of_show_id = ? WHERE id = ?`, doomed, pointer).Error)

	plan := s.plan()
	s.Require().Len(plan.Groups, 1)
	refs := map[string]int64{}
	for _, r := range plan.Groups[0].Copies[2].Refs {
		refs[r.Table] = r.Rows
	}
	s.Equal(map[string]int64{
		"collection_items":                         1,
		"notification_log":                         1,
		"notification_log (alert discriminators)":  1,
		"shows.duplicate_of_show_id (set to NULL)": 1,
	}, refs)

	s.Require().NoError(PurgeReplicatedShowGroup(s.db, replicaNow, plan.Groups[0], s.keepBackup))
	s.Require().Len(s.backups, 2)
	var doc struct {
		Show struct {
			ID uint `json:"id"`
		} `json:"show"`
		CollectionItems []json.RawMessage `json:"collection_items"`
		Notifications   []json.RawMessage `json:"notification_log"`
		Alerts          []json.RawMessage `json:"notification_log_alerts"`
		PointingHere    []uint            `json:"shows_pointing_here"`
	}
	s.Require().NoError(json.Unmarshal(s.backups[1], &doc))
	s.Equal(doomed, doc.Show.ID)
	s.Len(doc.CollectionItems, 1)
	s.Len(doc.Notifications, 1)
	s.Len(doc.Alerts, 1)
	s.Equal([]uint{pointer}, doc.PointingHere)
}

// An original that is pending, rejected or private shares the copies' key but
// is not approved; the group is left for a human and the sibling is named.
func (s *ShowReplicaPurgeSuite) TestNonApprovedSiblingLeavesTheGroupUntouched() {
	for _, status := range []string{"pending", "rejected", "private"} {
		s.Run(status, func() {
			s.TearDownTest()
			ids := s.replicate(s.artist("Igorrr "+status), s.venue("Great American Music Hall "+status), 2026, 2027, 2028)
			s.Require().NoError(s.db.Exec(`UPDATE shows SET status = ? WHERE id = ?`, status, ids[0]).Error)

			plan := s.plan()
			s.Require().Len(plan.Groups, 1)
			g := plan.Groups[0]
			s.Equal(replicaSkipSibling, g.SkipReason)
			s.False(g.Actionable())
			s.Require().Len(g.Siblings, 1)
			s.Equal(ids[0], g.Siblings[0].ShowID)
			s.Equal(status, g.Siblings[0].Status)
			s.Equal("2026-10-03", g.Siblings[0].LocalDate)
			s.Len(g.Copies, 2)
			s.Error(PurgeReplicatedShowGroup(s.db, replicaNow, g, s.keepBackup))
			s.Equal(ids, s.existing(ids))
			s.Empty(plan.Beyond, "a non-approved show is never in the far-future list")
		})
	}
}

// The digest moves when a row the delete would remove lands on a copy to
// delete, and does not move for rows the delete keeps or rows on the survivor.
func (s *ShowReplicaPurgeSuite) TestDigestCoversRowsTheDeleteRemoves() {
	ids := s.replicate(s.artist("Igorrr"), s.venue("Great American Music Hall"), 2026, 2027, 2028)
	fan := s.user("fan@test.local")
	before := s.plan().Digest()

	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, 'save')`, fan, ids[0]).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html) VALUES ('show', ?, ?, 'hi', '<p>hi</p>')`, ids[1], fan).Error)
	s.Equal(before, s.plan().Digest(), "a save on the survivor and a kept comment do not change what the delete removes")

	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, 'save')`, fan, ids[2]).Error)
	s.NotEqual(before, s.plan().Digest(), "a save on a copy to delete does")
}

func (s *ShowReplicaPurgeSuite) TestPlanDigestNamesTheDeletions() {
	room := s.venue("Great American Music Hall")
	s.replicate(s.artist("Igorrr"), room, 2026, 2027, 2028)
	first := s.plan().Digest()
	s.Equal(first, s.plan().Digest(), "the same rows give the same digest")

	s.replicate(s.artist("Beth Orton"), room, 2026, 2027, 2028)
	s.NotEqual(first, s.plan().Digest(), "a new actionable group changes the digest")
}

func (s *ShowReplicaPurgeSuite) TestUnapprovedCopyIsNotGrouped() {
	ids := s.replicate(s.artist("Misfit Cabaret"), s.venue("Great American Music Hall"), 2026, 2027)
	s.Require().NoError(s.db.Exec(`UPDATE shows SET status = 'private' WHERE id = ?`, ids[1]).Error)

	s.Empty(s.plan().Groups)
}

func (s *ShowReplicaPurgeSuite) TestCreateShow_EventDateBound() {
	room := s.venue("Great American Music Hall")
	svc := NewShowService(s.db)
	bound := showEventDateBound(time.Now())
	request := func(name string, at time.Time, admin, override bool) *contracts.CreateShowRequest {
		return &contracts.CreateShowRequest{
			Title:              name,
			EventDate:          at,
			State:              "CA",
			Venues:             []contracts.CreateShowVenue{{ID: &room}},
			Artists:            []contracts.CreateShowArtist{{Name: name}},
			SubmitterIsAdmin:   admin,
			AllowFarFutureDate: override,
		}
	}
	refused := func(err error) bool {
		var showErr *apperrors.ShowError
		return errors.As(err, &showErr) && showErr.Code == apperrors.CodeShowEventDateBeyondBound
	}

	_, err := svc.CreateShow(request("Inside", bound.Add(-24*time.Hour), true, false))
	s.NoError(err, "a day inside the bound is accepted")

	_, err = svc.CreateShow(request("Outside", bound.Add(24*time.Hour), true, false))
	s.True(refused(err), "a day past the bound is refused: %v", err)

	_, err = svc.CreateShow(request("Override", bound.Add(24*time.Hour), true, true))
	s.NoError(err, "an admin with the override is accepted")

	_, err = svc.CreateShow(request("Unprivileged", bound.Add(24*time.Hour), false, true))
	s.True(refused(err), "the override does nothing without admin: %v", err)

	s.Equal(int64(2), s.count(`SELECT COUNT(*) FROM shows`))
}
