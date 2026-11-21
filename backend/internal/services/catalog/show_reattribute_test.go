package catalog

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

type ShowReattributeTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func TestShowReattributeTestSuite(t *testing.T) {
	suite.Run(t, new(ShowReattributeTestSuite))
}

func (s *ShowReattributeTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ShowReattributeTestSuite) TearDownSuite() {
	if s.testDB != nil {
		s.testDB.Cleanup()
	}
}

func (s *ShowReattributeTestSuite) TearDownTest() {
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

// --- fixtures ---

const denver = "America/Denver"

// albuquerqueEvening is 8pm Mountain on 2026-11-25, the date-only anchor.
var albuquerqueEvening = time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)

func (s *ShowReattributeTestSuite) room(slug, name, city, state string) *catalogm.Venue {
	tz := denver
	v := &catalogm.Venue{Name: name, Slug: &slug, City: city, State: state, Verified: true, Timezone: &tz}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

func (s *ShowReattributeTestSuite) artist(name string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *ShowReattributeTestSuite) user(email string) *authm.User {
	u := &authm.User{Email: stringPtr(email), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

// show inserts an approved show at one room with a bill in position order, the
// first act as headliner.
func (s *ShowReattributeTestSuite) show(slug string, at time.Time, roomID uint, acts ...uint) uint {
	var id uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO shows (title, slug, event_date, state, status, source, created_at, updated_at)
		VALUES (?, ?, ?, 'NM', 'approved', 'user', NOW(), NOW()) RETURNING id
	`, slug, slug, at).Row().Scan(&id))
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, roomID).Error)
	for i, a := range acts {
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

func (s *ShowReattributeTestSuite) roomsOf(showID uint) []uint {
	var ids []uint
	s.Require().NoError(s.db.Model(&catalogm.ShowVenue{}).Where("show_id = ?", showID).
		Order("venue_id").Pluck("venue_id", &ids).Error)
	return ids
}

func (s *ShowReattributeTestSuite) slugOf(showID uint) string {
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	return *show.Slug
}

func (s *ShowReattributeTestSuite) showExists(showID uint) bool {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).Count(&n).Error)
	return n == 1
}

func (s *ShowReattributeTestSuite) count(table, where string, args ...interface{}) int64 {
	var n int64
	s.Require().NoError(s.db.Table(table).Where(where, args...).Count(&n).Error)
	return n
}

// snapshot is every row the tool can touch, for "wrote nothing" assertions.
func (s *ShowReattributeTestSuite) snapshot() string {
	var out string
	for _, q := range []string{
		`SELECT string_agg(show_id || ':' || venue_id, ',' ORDER BY show_id, venue_id) FROM show_venues`,
		`SELECT string_agg(show_id || ':' || artist_id || ':' || COALESCE(venue_id, 0) || ':' || position || ':' ||
		        COALESCE(set_type, ''), ',' ORDER BY show_id, artist_id) FROM show_artists`,
		`SELECT string_agg(id || ':' || slug || ':' || updated_at, ',' ORDER BY id) FROM shows`,
		`SELECT string_agg(entity_id || ':' || user_id || ':' || action, ',' ORDER BY id) FROM user_bookmarks`,
		`SELECT string_agg(entity_id::text, ',' ORDER BY id) FROM comments`,
	} {
		var v *string
		s.Require().NoError(s.db.Raw(q).Row().Scan(&v))
		if v != nil {
			out += *v
		}
		out += "|"
	}
	return out
}

// errTestRollback ends a preview transaction.
var errTestRollback = errors.New("test: roll back")

// preview runs a row inside a transaction it then rolls back, which is how the
// CLI's dry run calls ReattributeShow.
func (s *ShowReattributeTestSuite) preview(row ShowReattribution) ShowReattributionResult {
	var res ShowReattributionResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res = ReattributeShow(tx, row)
		return errTestRollback
	})
	s.Require().ErrorIs(err, errTestRollback)
	return res
}

// --- plain move ---

func (s *ShowReattributeTestSuite) TestPlainMove_RepointsTheRoomAndKeepsTheSlug() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	act := s.artist("The Amity Affliction")
	id := s.show("2026-11-17-the-amity-affliction-at-launchpad", albuquerqueEvening, launchpad.ID, act.ID)

	res := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "2026-11-17-the-amity-affliction-at-launchpad", RoomSlug: "sunshine"})

	s.Require().Equal(ReattributionMoved, res.Outcome, res.Reason)
	s.Equal("Launchpad", res.CurrentRoom.Name)
	s.Equal("Sunshine Theater", res.TargetRoom.Name)
	s.Equal("2026-11-25", res.VenueLocalDate)
	s.Equal([]uint{sunshine.ID}, s.roomsOf(id))
	s.Equal("2026-11-17-the-amity-affliction-at-launchpad", s.slugOf(id), "a moved show keeps its slug")
	s.Equal(int64(1), s.count("show_artists", "show_id = ? AND venue_id = ?", id, sunshine.ID),
		"the denormalized primary room on the bill follows the move")
	s.Equal(int64(1), s.count("show_dedup_keys", "show_id = ? AND venue_id = ?", id, sunshine.ID),
		"the derived dedup key follows the move")
}

func (s *ShowReattributeTestSuite) TestPreview_RunsTheMoveAndWritesNothing() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	act := s.artist("Bodysnatcher")
	id := s.show("bodysnatcher", albuquerqueEvening, launchpad.ID, act.ID)
	before := s.snapshot()

	res := s.preview(ShowReattribution{ShowSlug: "bodysnatcher", RoomSlug: "sunshine"})

	s.Equal(ReattributionMoved, res.Outcome, res.Reason)
	s.Equal([]uint{launchpad.ID}, s.roomsOf(id))
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestPlainMove_SecondRunChangesNothing() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	act := s.artist("Story of the Year")
	id := s.show("soty", albuquerqueEvening, launchpad.ID, act.ID)
	row := ShowReattribution{ShowSlug: "soty", RoomSlug: "sunshine"}

	s.Require().Equal(ReattributionMoved, ReattributeShow(s.db, row).Outcome)
	after := s.snapshot()

	again := ReattributeShow(s.db, row)
	s.Equal(ReattributionUnchanged, again.Outcome)
	s.Equal(after, s.snapshot())
	s.Equal([]uint{sunshine.ID}, s.roomsOf(id))
}

func (s *ShowReattributeTestSuite) TestPlainMove_WarnsAboutAnotherShowAtTheTargetThatNight() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	cenit := s.artist("Cenit")
	cenitMezz := s.artist("Cenit / The Mezzanine")
	other := s.artist("Someone Else")
	opener := s.artist("Opener Act")
	mezzShow := s.show("cenit-mezz", albuquerqueEvening, elRey.ID, opener.ID)
	// The curated headliner sits at position 1, behind an uncurated opener, so
	// the reported headliner is the one the rank clause puts first.
	s.Require().NoError(s.db.Exec(`UPDATE show_artists SET set_type = 'opener' WHERE show_id = ?`, mezzShow).Error)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 1, 'headliner')`,
		mezzShow, cenitMezz.ID).Error)
	s.Require().NoError(syncShowArtistDedupColumns(s.db, mezzShow))
	// A different night at the same room, inside the SQL window: not reported.
	s.show("next-night", albuquerqueEvening.Add(24*time.Hour), elRey.ID, other.ID)
	s.show("cenit", albuquerqueEvening, launchpad.ID, cenit.ID)

	res := s.preview(ShowReattribution{ShowSlug: "cenit", RoomSlug: "el-rey"})

	s.Require().Equal(ReattributionMoved, res.Outcome, res.Reason)
	s.Require().Len(res.SameNightAtTarget, 1)
	s.Equal("cenit-mezz", res.SameNightAtTarget[0].Slug)
	s.Equal("Cenit / The Mezzanine", res.SameNightAtTarget[0].Headliner)
}

// The venue-local date, not the UTC date, decides "the same night": 11pm
// Mountain on the 25th is the 26th in UTC.
func (s *ShowReattributeTestSuite) TestPlainMove_SameNightIsVenueLocal() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	a := s.artist("Late Act")
	b := s.artist("Early Act")
	s.show("late", time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC), elRey.ID, a.ID)  // 11pm on the 25th
	s.show("early", time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC), elRey.ID, a.ID) // 1am on the 26th
	s.show("mover", time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC), launchpad.ID, b.ID)

	res := s.preview(ShowReattribution{ShowSlug: "mover", RoomSlug: "el-rey"})

	s.Require().Equal(ReattributionMoved, res.Outcome, res.Reason)
	s.Require().Len(res.SameNightAtTarget, 1)
	s.Equal("late", res.SameNightAtTarget[0].Slug)
}

// A plain move onto a room that already bills the same act at the same instant
// would break the derived dedup key, so it is refused before any write, naming
// the show it would collide with.
func (s *ShowReattributeTestSuite) TestPlainMove_OntoAnExactDuplicateIsRefusedNamingTheOtherShow() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	act := s.artist("As I Lay Dying")
	s.show("aild", albuquerqueEvening, sunshine.ID, act.ID)
	id := s.show("aild-2", albuquerqueEvening, launchpad.ID, act.ID)
	before := s.snapshot()

	row := ShowReattribution{ShowSlug: "aild-2", RoomSlug: "sunshine"}
	for name, res := range map[string]ShowReattributionResult{
		"preview": s.preview(row),
		"apply":   ReattributeShow(s.db, row),
	} {
		s.Equal(ReattributionRefused, res.Outcome, name)
		s.Contains(res.Reason, "aild (show ")
		s.Contains(res.Reason, "approved) at the target room bills one of this show's acts at the same time")
		s.Require().Len(res.SameNightAtTarget, 1)
		s.True(res.SameNightAtTarget[0].Collides)
	}
	s.Equal(before, s.snapshot())
	s.Equal([]uint{launchpad.ID}, s.roomsOf(id))
}

// --- merge ---

func (s *ShowReattributeTestSuite) TestMerge_FoldsTheShowIntoTheNamedCopyWithItsFollowersSavesAndComments() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	aild := s.artist("As I Lay Dying")
	support := s.artist("Kill The Lights")
	// The copy at the right room carries a slug naming the wrong room; the
	// merge must leave that slug alone.
	winner := s.show("2026-11-25-as-i-lay-dying-at-launchpad", albuquerqueEvening, sunshine.ID, aild.ID)
	loser := s.show("2026-11-25-as-i-lay-dying-at-launchpad-2", albuquerqueEvening, launchpad.ID, aild.ID, support.ID)

	saver := s.user("saver@test.local")
	follower := s.user("follower@test.local")
	both := s.user("both@test.local")
	bookmark := `INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, ?)`
	s.Require().NoError(s.db.Exec(bookmark, saver.ID, loser, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, follower.ID, loser, "follow").Error)
	s.Require().NoError(s.db.Exec(bookmark, both.ID, loser, "save").Error)
	s.Require().NoError(s.db.Exec(bookmark, both.ID, winner, "save").Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'see you there', '<p>see you there</p>')`, loser, saver.ID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO comment_subscriptions (user_id, entity_type, entity_id)
		VALUES (?, 'show', ?)`, follower.ID, loser).Error)

	row := ShowReattribution{
		ShowSlug:      "2026-11-25-as-i-lay-dying-at-launchpad-2",
		RoomSlug:      "sunshine",
		MergeIntoSlug: "2026-11-25-as-i-lay-dying-at-launchpad",
	}
	before := s.snapshot()
	preview := s.preview(row)
	s.Require().Equal(ReattributionMerged, preview.Outcome, preview.Reason)
	s.Equal([]ReattributionAct{{Name: "Kill The Lights", SetType: "performer", Position: 1}}, preview.ActsAddedToWinner)
	s.Require().NotNil(preview.MergeSummary)
	s.Equal(before, s.snapshot(), "the preview writes nothing")

	res := ReattributeShow(s.db, row)
	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.Equal(winner, res.MergeIntoID)
	s.Equal(1, res.MergeSummary.LosersMerged)
	s.Equal(int64(1), res.RoomRowsCleared, "the merged show's one room row is cleared")

	s.False(s.showExists(loser), "the merged show is deleted")
	s.Equal("2026-11-25-as-i-lay-dying-at-launchpad", s.slugOf(winner), "the winner keeps its slug")
	s.Equal([]uint{sunshine.ID}, s.roomsOf(winner), "the winner gains no second room")
	s.Equal(int64(1), s.count("show_artists", "show_id = ? AND artist_id = ?", winner, support.ID),
		"the support act on the merged show joins the winner's bill")

	s.Equal(int64(1), s.count("user_bookmarks", "user_id = ? AND entity_id = ? AND action = 'save'", saver.ID, winner))
	s.Equal(int64(1), s.count("user_bookmarks", "user_id = ? AND entity_id = ? AND action = 'follow'", follower.ID, winner))
	s.Equal(int64(1), s.count("user_bookmarks", "user_id = ? AND entity_id = ? AND action = 'save'", both.ID, winner),
		"a save held on both collapses to one")
	s.Equal(int64(1), s.count("comments", "entity_type = 'show' AND entity_id = ?", winner))
	s.Equal(int64(1), s.count("comment_subscriptions", "entity_type = 'show' AND entity_id = ?", winner))
	s.Zero(s.count("user_bookmarks", "entity_type = 'show' AND entity_id = ?", loser))
	s.Zero(s.count("comments", "entity_type = 'show' AND entity_id = ?", loser))

	after := s.snapshot()
	again := ReattributeShow(s.db, row)
	s.Equal(ReattributionRefused, again.Outcome)
	s.Contains(again.Reason, "may have been merged by an earlier run")
	s.Equal(after, s.snapshot(), "a second run changes nothing")
}

func (s *ShowReattributeTestSuite) TestMerge_RefusesATargetThatIsNotAtTheRoom() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	a := s.artist("Wolves at the Gate")
	s.show("demon-hunter", albuquerqueEvening, elRey.ID, a.ID)
	s.show("wolves", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	res := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "wolves", RoomSlug: "sunshine", MergeIntoSlug: "demon-hunter"})

	s.Equal(ReattributionRefused, res.Outcome)
	s.Equal("merge target demon-hunter must list exactly one room, sunshine; it lists [el-rey]", res.Reason)
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestMerge_RefusesAnotherVenueLocalDate() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	a := s.artist("Cenit")
	s.show("cenit-next-night", albuquerqueEvening.Add(24*time.Hour), elRey.ID, a.ID)
	s.show("cenit", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	res := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "cenit", RoomSlug: "el-rey", MergeIntoSlug: "cenit-next-night"})

	s.Equal(ReattributionRefused, res.Outcome)
	s.Contains(res.Reason, "2026-11-26")
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestMerge_RefusesUnknownTargetAndSelf() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	a := s.artist("FEA")
	s.show("fea", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	unknown := ReattributeShow(s.db, ShowReattribution{ShowSlug: "fea", RoomSlug: "sunshine", MergeIntoSlug: "nope"})
	s.Equal(ReattributionRefused, unknown.Outcome)
	s.Contains(unknown.Reason, "merge target nope not found")

	self := ReattributeShow(s.db, ShowReattribution{ShowSlug: "fea", RoomSlug: "sunshine", MergeIntoSlug: "fea"})
	s.Equal(ReattributionRefused, self.Outcome)
	s.Equal("merge target is the show itself", self.Reason)

	blank := ReattributeShow(s.db, ShowReattribution{ShowSlug: " ", RoomSlug: "sunshine"})
	s.Equal(ReattributionRefused, blank.Outcome)
	s.Equal("no show slug", blank.Reason)
	s.Equal(before, s.snapshot())
}

// The CLI's dry run: every row inside one outer transaction, each in its own
// savepoint, the lot rolled back. A merge into a show an earlier row moved sees
// that move, and a refused row in between does not poison the rows after it.
func (s *ShowReattributeTestSuite) TestRowsInsideAnOuterTransactionSeeEarlierRowsAndRollBackTogether() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	cenit := s.artist("Cenit")
	mezz := s.artist("Cenit / The Mezzanine")
	s.show("cenit", albuquerqueEvening, launchpad.ID, cenit.ID)
	s.show("cenit-mezz", albuquerqueEvening, launchpad.ID, mezz.ID)
	before := s.snapshot()

	var outcomes []ReattributionOutcome
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, row := range []ShowReattribution{
			{ShowSlug: "cenit", RoomSlug: "el-rey"},
			{ShowSlug: "missing", RoomSlug: "el-rey"},
			{ShowSlug: "cenit-mezz", RoomSlug: "el-rey", MergeIntoSlug: "cenit"},
		} {
			outcomes = append(outcomes, ReattributeShow(tx, row).Outcome)
		}
		return errTestRollback
	})

	s.ErrorIs(err, errTestRollback)
	s.Equal([]ReattributionOutcome{ReattributionMoved, ReattributionRefused, ReattributionMerged}, outcomes)
	s.Equal(before, s.snapshot(), "the outer rollback takes every row with it")
}

func (s *ShowReattributeTestSuite) TestMerge_KeepsTheWinnersHeadlinerAndBillOrder() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	// Created first, so a tie at position 0 would rank it ahead of Cenit.
	mezz := s.artist("Cenit / The Mezzanine")
	cenit := s.artist("Cenit")
	support := s.artist("Night Support")
	winner := s.show("cenit", albuquerqueEvening, elRey.ID, cenit.ID, support.ID)
	loserSupport := s.artist("Loser Support")
	s.show("cenit-mezz", albuquerqueEvening, launchpad.ID, mezz.ID, loserSupport.ID, support.ID)
	// A fixed past instant, so the assertion below compares two values the
	// test controls rather than the database clock with the Go one.
	stampedBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Require().NoError(s.db.Exec(`UPDATE shows SET updated_at = ? WHERE id = ?`, stampedBefore, winner).Error)

	res := ReattributeShow(s.db, ShowReattribution{ShowSlug: "cenit-mezz", RoomSlug: "el-rey", MergeIntoSlug: "cenit"})

	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.Equal("Cenit", res.MergeIntoHeadliner)
	s.Equal([]ReattributionAct{
		{Name: "Cenit / The Mezzanine", SetType: "performer", Position: 2},
		{Name: "Loser Support", SetType: "performer", Position: 3},
	}, res.ActsAddedToWinner)
	headliner, err := showHeadlinerName(s.db, winner)
	s.Require().NoError(err)
	s.Equal("Cenit", headliner, "the winner keeps its headliner")
	var bill []struct {
		Name     string
		Position int
		SetType  string
	}
	s.Require().NoError(s.db.Raw(`SELECT a.name, sa.position, sa.set_type FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id WHERE sa.show_id = ? ORDER BY sa.position`, winner).Scan(&bill).Error)
	s.Require().Len(bill, 4)
	s.Equal("Cenit", bill[0].Name)
	s.Equal("headliner", bill[0].SetType)
	s.Equal("Night Support", bill[1].Name, "an act both bills share keeps the winner's row")
	s.Equal(1, bill[1].Position)
	var stampedAfter time.Time
	s.Require().NoError(s.db.Raw(`SELECT updated_at FROM shows WHERE id = ?`, winner).Row().Scan(&stampedAfter))
	s.True(stampedAfter.After(stampedBefore), "the winner's page changed, so its updated_at moves")
}

func (s *ShowReattributeTestSuite) TestMerge_RefusesShowsThatAreNotBothApproved() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	a := s.artist("Status Act")
	b := s.artist("Other Status Act")
	s.show("approved-winner", albuquerqueEvening, sunshine.ID, a.ID)
	privateLoser := s.show("private-loser", albuquerqueEvening, launchpad.ID, a.ID)
	pendingWinner := s.show("pending-winner", albuquerqueEvening, sunshine.ID, b.ID)
	s.show("approved-loser", albuquerqueEvening, launchpad.ID, b.ID)
	s.Require().NoError(s.db.Exec(`UPDATE shows SET status = 'private' WHERE id = ?`, privateLoser).Error)
	s.Require().NoError(s.db.Exec(`UPDATE shows SET status = 'pending' WHERE id = ?`, pendingWinner).Error)
	u := s.user("private@test.local")
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'only mine', '<p>only mine</p>')`, privateLoser, u.ID).Error)
	before := s.snapshot()

	gatedLoser := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "private-loser", RoomSlug: "sunshine", MergeIntoSlug: "approved-winner"})
	s.Equal(ReattributionRefused, gatedLoser.Outcome)
	s.Contains(gatedLoser.Reason, "the show is private")

	gatedWinner := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "approved-loser", RoomSlug: "sunshine", MergeIntoSlug: "pending-winner"})
	s.Equal(ReattributionRefused, gatedWinner.Outcome)
	s.Contains(gatedWinner.Reason, "the merge target is pending")

	s.Equal(before, s.snapshot(), "a private show's comment stays where it was")
}

// A merge row whose show already sits at the target room is still a merge: the
// row names a copy to fold it into.
func (s *ShowReattributeTestSuite) TestMerge_ShowAlreadyAtTheTargetRoomIsStillMerged() {
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	a := s.artist("Same Room Act")
	b := s.artist("Same Room Twin")
	winner := s.show("same-room-winner", albuquerqueEvening, sunshine.ID, a.ID)
	loser := s.show("same-room-loser", albuquerqueEvening, sunshine.ID, b.ID)

	res := ReattributeShow(s.db, ShowReattribution{
		ShowSlug: "same-room-loser", RoomSlug: "sunshine", MergeIntoSlug: "same-room-winner"})

	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.False(s.showExists(loser))
	s.Equal([]uint{sunshine.ID}, s.roomsOf(winner))
}

// The CLI applies a worklist row by row; run twice, the second pass writes
// nothing, whatever mix of moves, merges and refusals it holds.
func (s *ShowReattributeTestSuite) TestAWorklistRunTwiceChangesNothingTheSecondTime() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	cenit := s.artist("Cenit")
	mezz := s.artist("Cenit / The Mezzanine")
	amity := s.artist("Amity")
	s.show("cenit", albuquerqueEvening, launchpad.ID, cenit.ID)
	s.show("cenit-mezz", albuquerqueEvening, launchpad.ID, mezz.ID)
	s.show("amity", albuquerqueEvening, launchpad.ID, amity.ID)
	worklist := []ShowReattribution{
		{ShowSlug: "cenit", RoomSlug: "el-rey"},
		{ShowSlug: "cenit-mezz", RoomSlug: "el-rey", MergeIntoSlug: "cenit"},
		{ShowSlug: "missing", RoomSlug: "el-rey"},
		{ShowSlug: "amity", RoomSlug: "sunshine"},
	}
	outcomes := func() []ReattributionOutcome {
		var out []ReattributionOutcome
		for _, row := range worklist {
			out = append(out, ReattributeShow(s.db, row).Outcome)
		}
		return out
	}

	s.Equal([]ReattributionOutcome{ReattributionMoved, ReattributionMerged, ReattributionRefused, ReattributionMoved}, outcomes())
	after := s.snapshot()
	s.Equal([]ReattributionOutcome{ReattributionUnchanged, ReattributionRefused, ReattributionRefused, ReattributionUnchanged}, outcomes())
	s.Equal(after, s.snapshot())
}

// An uncurated winner stays uncurated: an opener arriving from the merged show
// would otherwise make the bill read as curated, and a curated bill's headline
// slot is its headliner rows, of which this one has none.
func (s *ShowReattributeTestSuite) TestMerge_AnUncuratedWinnerKeepsItsHeadlineSlot() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	cenit := s.artist("Cenit")
	mezz := s.artist("Cenit / The Mezzanine")
	opener := s.artist("Early Opener")
	winner := s.show("cenit", albuquerqueEvening, elRey.ID, cenit.ID)
	s.Require().NoError(s.db.Exec(`UPDATE show_artists SET set_type = 'performer' WHERE show_id = ?`, winner).Error)
	loser := s.show("cenit-mezz", albuquerqueEvening, launchpad.ID, mezz.ID, opener.ID)
	s.Require().NoError(s.db.Exec(
		`UPDATE show_artists SET set_type = 'opener' WHERE show_id = ? AND artist_id = ?`, loser, opener.ID).Error)

	res := ReattributeShow(s.db, ShowReattribution{ShowSlug: "cenit-mezz", RoomSlug: "el-rey", MergeIntoSlug: "cenit"})

	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.Equal([]ReattributionAct{
		{Name: "Cenit / The Mezzanine", SetType: "performer", Position: 1},
		{Name: "Early Opener", SetType: "performer", Position: 2},
	}, res.ActsAddedToWinner)
	var inHeadlineSlot []string
	s.Require().NoError(s.db.Raw(`SELECT a.name FROM show_artists sa JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id = ? AND `+headlineSlotSQL("sa"), winner).Scan(&inHeadlineSlot).Error)
	s.Equal([]string{"Cenit"}, inHeadlineSlot)
}

// A merge the database rejects part way, after the merged show's rooms are
// cleared and its bill rewritten, leaves both shows exactly as they were.
func (s *ShowReattributeTestSuite) TestMerge_AFailureLeavesBothShowsAsTheyWere() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	aild := s.artist("As I Lay Dying")
	support := s.artist("Kill The Lights")
	s.show("aild", albuquerqueEvening, sunshine.ID, aild.ID)
	// The loser headlines with the act the winner lacks, so the bill rewrite
	// changes it (headliner at 0 to performer at 1) before the merge fails.
	loser := s.show("aild-2", albuquerqueEvening, launchpad.ID, support.ID, aild.ID)
	u := s.user("commenter@test.local")
	var commentID uint
	s.Require().NoError(s.db.Raw(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		VALUES ('show', ?, ?, 'hi', '<p>hi</p>') RETURNING id`, loser, u.ID).Row().Scan(&commentID))
	before := s.snapshot()

	// Another session holds the loser's comment, which the merge re-points
	// after the rooms and bill have already been rewritten.
	holder := s.db.Begin()
	s.Require().NoError(holder.Error)
	defer func() { _ = holder.Rollback() }()
	s.Require().NoError(holder.Exec(`SELECT id FROM comments WHERE id = ? FOR UPDATE`, commentID).Error)

	var res ShowReattributionResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		s.Require().NoError(tx.Exec(`SET LOCAL lock_timeout = '200ms'`).Error)
		res = ReattributeShow(tx, ShowReattribution{ShowSlug: "aild-2", RoomSlug: "sunshine", MergeIntoSlug: "aild"})
		return nil // commit the outer transaction, as --confirm does
	})
	s.Require().NoError(holder.Rollback().Error)
	s.Require().NoError(err)

	s.Equal(ReattributionFailed, res.Outcome)
	s.Contains(res.Reason, "lock")
	s.Equal(before, s.snapshot())
	s.Equal([]uint{launchpad.ID}, s.roomsOf(loser))
}

// A merge row also reports the other shows left at the target room that
// night, leaving out the show it merges into.
func (s *ShowReattributeTestSuite) TestMerge_ReportsOtherShowsAtTheTargetThatNight() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	a := s.artist("Merge Act")
	other := s.artist("Third Act")
	s.show("winner", albuquerqueEvening, sunshine.ID, a.ID)
	s.show("third", albuquerqueEvening.Add(time.Hour), sunshine.ID, other.ID)
	s.show("loser", albuquerqueEvening, launchpad.ID, a.ID)

	res := s.preview(ShowReattribution{ShowSlug: "loser", RoomSlug: "sunshine", MergeIntoSlug: "winner"})

	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.Require().Len(res.SameNightAtTarget, 1)
	s.Equal("third", res.SameNightAtTarget[0].Slug)
	s.False(res.SameNightAtTarget[0].Collides)
}

// --- refusals ---

func (s *ShowReattributeTestSuite) TestRefusesUnknownSlugs() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	a := s.artist("Dweezil Zappa")
	s.show("dweezil", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	noShow := ReattributeShow(s.db, ShowReattribution{ShowSlug: "missing", RoomSlug: "launchpad"})
	s.Equal(ReattributionRefused, noShow.Outcome)
	s.Equal("show not found", noShow.Reason)

	noRoom := ReattributeShow(s.db, ShowReattribution{ShowSlug: "dweezil", RoomSlug: "nhcc"})
	s.Equal(ReattributionRefused, noRoom.Outcome)
	s.Equal("room nhcc not found", noRoom.Reason)

	blank := ReattributeShow(s.db, ShowReattribution{ShowSlug: "dweezil", RoomSlug: "  "})
	s.Equal(ReattributionRefused, blank.Outcome)
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestRefusesACrossCityMove() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	s.room("meow-wolf", "Meow Wolf", "Santa Fe", "NM")
	s.room("launchpad-tx", "Launchpad Austin", "Albuquerque", "TX")
	a := s.artist("Sage Cornelius Band")
	s.show("sage", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	city := ReattributeShow(s.db, ShowReattribution{ShowSlug: "sage", RoomSlug: "meow-wolf"})
	s.Equal(ReattributionRefused, city.Outcome)
	s.Contains(city.Reason, "Santa Fe")

	state := ReattributeShow(s.db, ShowReattribution{ShowSlug: "sage", RoomSlug: "launchpad-tx"})
	s.Equal(ReattributionRefused, state.Outcome, "the same city name in another state is another city")
	s.Equal(before, s.snapshot())
}

// A merge target at a second room would take the show's acts there too, where
// the collision check does not look, so it is refused.
func (s *ShowReattributeTestSuite) TestMerge_RefusesATargetWithASecondRoom() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	elRey := s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	a := s.artist("Two Room Winner")
	winner := s.show("winner", albuquerqueEvening, sunshine.ID, a.ID)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, winner, elRey.ID).Error)
	s.show("loser", albuquerqueEvening, launchpad.ID, a.ID)
	before := s.snapshot()

	res := ReattributeShow(s.db, ShowReattribution{ShowSlug: "loser", RoomSlug: "sunshine", MergeIntoSlug: "winner"})

	s.Equal(ReattributionRefused, res.Outcome)
	s.Equal("merge target winner must list exactly one room, sunshine; it lists [sunshine, el-rey]", res.Reason)
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestRefusesAShowWithNoRoom() {
	s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	a := s.artist("Roomless Act")
	id := s.show("roomless", albuquerqueEvening, launchpad.ID, a.ID)
	s.Require().NoError(s.db.Exec(`DELETE FROM show_venues WHERE show_id = ?`, id).Error)
	before := s.snapshot()

	res := ReattributeShow(s.db, ShowReattribution{ShowSlug: "roomless", RoomSlug: "sunshine"})

	s.Equal(ReattributionRefused, res.Outcome)
	s.Contains(res.Reason, "0 rooms")
	s.Equal(before, s.snapshot())
}

func (s *ShowReattributeTestSuite) TestSameCityReadsCountryOnlyWhenBothRoomsStateOne() {
	us, ca, none := "US", "CA", ""
	at := func(country *string) *catalogm.Venue {
		return &catalogm.Venue{City: " Albuquerque ", State: "nm", Country: country}
	}
	s.True(sameCity(at(&us), &catalogm.Venue{City: "albuquerque", State: "NM", Country: &us}))
	s.False(sameCity(at(&us), at(&ca)), "same city and state in another country is another city")
	s.True(sameCity(at(&us), at(nil)), "one room with no country does not decide")
	s.True(sameCity(at(&none), at(&ca)))
}

func (s *ShowReattributeTestSuite) TestRefusesAShowWithTwoRooms() {
	launchpad := s.room("launchpad", "Launchpad", "Albuquerque", "NM")
	sunshine := s.room("sunshine", "Sunshine Theater", "Albuquerque", "NM")
	s.room("el-rey", "El Rey Theater", "Albuquerque", "NM")
	a := s.artist("Two Room Act")
	id := s.show("two-rooms", albuquerqueEvening, launchpad.ID, a.ID)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, sunshine.ID).Error)
	before := s.snapshot()

	res := ReattributeShow(s.db, ShowReattribution{ShowSlug: "two-rooms", RoomSlug: "el-rey"})

	s.Equal(ReattributionRefused, res.Outcome)
	s.Contains(res.Reason, "2 rooms")
	s.Equal(before, s.snapshot())
}
