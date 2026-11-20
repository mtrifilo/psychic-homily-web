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
		`SELECT string_agg(show_id || ':' || artist_id || ':' || COALESCE(venue_id, 0), ',' ORDER BY show_id, artist_id) FROM show_artists`,
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
	s.show("cenit-mezz", albuquerqueEvening, elRey.ID, cenitMezz.ID)
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
// is refused by the derived dedup key. The row fails, and its transaction takes
// the room re-point down with it.
func (s *ShowReattributeTestSuite) TestPlainMove_OntoAnExactDuplicateFailsAndRollsBack() {
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
		s.Equal(ReattributionFailed, res.Outcome, name)
		s.NotEmpty(res.Reason)
		s.Require().Len(res.SameNightAtTarget, 1, "the failure names the show it collided with")
		s.Equal("aild", res.SameNightAtTarget[0].Slug)
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
	s.Equal([]string{"Kill The Lights"}, preview.ActsAddedToWinner)
	s.Require().NotNil(preview.MergeSummary)
	s.Equal(before, s.snapshot(), "the preview writes nothing")

	res := ReattributeShow(s.db, row)
	s.Require().Equal(ReattributionMerged, res.Outcome, res.Reason)
	s.Equal(winner, res.MergeIntoID)
	s.Equal(1, res.MergeSummary.LosersMerged)

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

	again := ReattributeShow(s.db, row)
	s.Equal(ReattributionRefused, again.Outcome)
	s.Contains(again.Reason, "may have been merged by an earlier run")
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
	s.Contains(res.Reason, "is not at room sunshine")
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
