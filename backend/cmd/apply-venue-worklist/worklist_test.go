package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

func TestParseWorklist(t *testing.T) {
	t.Run("reads the reviewed columns, trims, lowercases the action and ignores extra columns", func(t *testing.T) {
		rows, err := parseWorklist(strings.NewReader(
			"\ufeffroom_slug,action,target,note,name\n" +
				" a-tucson-tucson-az , MERGE , a-tucson-az ,twin,A Tucson\n" +
				",,,,\n" +
				"b-houston-houston-tx,rename,B,,B Houston\n"))
		require.NoError(t, err)
		assert.Equal(t, []worklistRow{
			{Line: 2, RoomSlug: "a-tucson-tucson-az", Action: "merge", Target: "a-tucson-az", Note: "twin"},
			{Line: 4, RoomSlug: "b-houston-houston-tx", Action: "rename", Target: "B"},
		}, rows)
	})

	t.Run("note is optional", func(t *testing.T) {
		rows, err := parseWorklist(strings.NewReader("action,target,room_slug\nkeep,,c-chicago-il\n"))
		require.NoError(t, err)
		assert.Equal(t, []worklistRow{{Line: 2, RoomSlug: "c-chicago-il", Action: "keep"}}, rows)
	})

	for name, tc := range map[string]struct{ in, wantErr string }{
		"an unreviewed suggestions file": {
			in:      "room_slug,suggested_action,suggested_target,note\na,merge,b,\n",
			wantErr: `no "action" column`,
		},
		"a missing target column": {in: "room_slug,action\na,keep\n", wantErr: `no "target" column`},
		"a repeated column":       {in: "room_slug,action,target,action\na,keep,,merge\n", wantErr: "twice"},
		"an empty file":           {in: "", wantErr: "empty"},
		"a header and no rows":    {in: "room_slug,action,target\n", wantErr: "no rows"},
		"a ragged row":            {in: "room_slug,action,target\na,keep\n", wantErr: "read worklist"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			_, err := parseWorklist(strings.NewReader(tc.in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// The suite runs the tool against a real, fully migrated Postgres: the merge it
// drives deletes rows and relies on cascades and unique indexes that only the
// real schema has.
type WorklistSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	actor  uint
}

func TestWorklistSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(WorklistSuite))
}

func (s *WorklistSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *WorklistSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *WorklistSuite) SetupTest() {
	for _, stmt := range []string{
		"DELETE FROM audit_logs",
		"DELETE FROM revisions",
		"DELETE FROM user_bookmarks",
		"DELETE FROM source_configs",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM venues",
		"DELETE FROM users",
	} {
		s.Require().NoError(s.db.Exec(stmt).Error, stmt)
	}
	s.actor = s.user()
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", s.actor).Update("is_admin", true).Error)
}

func (s *WorklistSuite) venue(name, city, state string) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%s-%s", strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		strings.ToLower(strings.ReplaceAll(city, " ", "-")), strings.ToLower(state))
	v := &catalogm.Venue{Name: name, Slug: &slug, City: city, State: state}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

func (s *WorklistSuite) show(title string, v *catalogm.Venue) uint {
	slug := fmt.Sprintf("%s-%d", strings.ReplaceAll(title, " ", "-"), time.Now().UnixNano())
	sh := &catalogm.Show{Title: title, Slug: &slug, EventDate: time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)}
	s.Require().NoError(s.db.Create(sh).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: sh.ID, VenueID: v.ID}).Error)
	return sh.ID
}

func (s *WorklistSuite) user() uint {
	email := fmt.Sprintf("fan-%d@test.local", time.Now().UnixNano())
	u := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u.ID
}

func (s *WorklistSuite) run(csv string, confirm bool) (string, runSummary) {
	out, report := s.runReport(csv, confirm)
	return out, report.Summary
}

func (s *WorklistSuite) runReport(csv string, confirm bool) (string, runReport) {
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	var out bytes.Buffer
	report := runWorklist(s.db, rows, s.actor, confirm, &out)
	return out.String(), report
}

func (s *WorklistSuite) venueByID(id uint) *catalogm.Venue {
	var v catalogm.Venue
	err := s.db.First(&v, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	s.Require().NoError(err)
	return &v
}

func (s *WorklistSuite) count(sql string, args ...any) int64 {
	var n int64
	s.Require().NoError(s.db.Raw(sql, args...).Scan(&n).Error)
	return n
}

// waitForMergeAudit waits for the background audit write of a merge into id.
func (s *WorklistSuite) waitForMergeAudit(into uint) {
	for i := 0; i < 200; i++ {
		if s.count(`SELECT COUNT(*) FROM audit_logs WHERE action = ? AND entity_id = ?`,
			catalog.AuditActionMergeVenues, into) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.FailNow("merge audit row never arrived")
}

// A twin merge moves the suffixed room's show, follow and calendar registry
// row onto the kept room and deletes the suffixed room. The dry run before it
// writes nothing and prints both rooms so a wrong target is visible.
func (s *WorklistSuite) TestMergeMovesShowsFollowsAndRegistryRows() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	showID := s.show("twin show", twin)
	fan := s.user()
	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'venue', ?, 'follow', NOW())`, fan, twin.ID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO source_configs (entity_type, entity_id, source_url)
		VALUES ('venue', ?, 'https://example.test/calendar')`, twin.ID).Error)
	csv := "room_slug,action,target,note\n" + *twin.Slug + ",merge," + *kept.Slug + ",same-city twin\n"

	out, summary := s.run(csv, false)

	s.False(summary.Refused())
	s.Equal(1, summary[statusWouldApply])
	s.Contains(out, `from: "Rialto Theatre Tucson" (Tucson, AZ)`)
	s.Contains(out, `into: "Rialto Theatre" (Tucson, AZ)`)
	s.Contains(out, "shows=1")
	s.Contains(out, "show_venues_moved=1")
	s.Contains(out, "/venues/"+*twin.Slug+" answers 404")
	s.Contains(out, "note: same-city twin")
	s.NotNil(s.venueByID(twin.ID), "the dry run deletes nothing")
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM show_venues WHERE venue_id = ?`, twin.ID))

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusApplied])
	s.Nil(s.venueByID(twin.ID), "the merged room is deleted")
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM show_venues WHERE show_id = ? AND venue_id = ?`, showID, kept.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'venue' AND entity_id = ? AND user_id = ?`, kept.ID, fan))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM source_configs WHERE entity_type = 'venue' AND entity_id = ?`, kept.ID))
	s.Equal("Rialto Theatre", s.venueByID(kept.ID).Name, "the kept room keeps its name")
}

// A rename changes the name and nothing else: the slug stays as it was, so the
// room's URL keeps answering.
func (s *WorklistSuite) TestRenameChangesTheNameAndKeepsTheSlug() {
	room := s.venue("Scout Bar Houston", "Houston", "TX")
	csv := "room_slug,action,target\n" + *room.Slug + ",rename,Scout Bar\n"

	out, summary := s.run(csv, false)

	s.False(summary.Refused(), out)
	s.Contains(out, `name: "Scout Bar Houston" -> "Scout Bar"`)
	s.Contains(out, "slug: scout-bar-houston-houston-tx (unchanged)")
	s.Equal("Scout Bar Houston", s.venueByID(room.ID).Name, "the dry run renames nothing")

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	got := s.venueByID(room.ID)
	s.Equal("Scout Bar", got.Name)
	s.Equal("scout-bar-houston-houston-tx", *got.Slug)
}

func (s *WorklistSuite) TestKeepChangesNothing() {
	room := s.venue("House of Blues Chicago", "Chicago", "IL")
	csv := "room_slug,action,target,note\n" + *room.Slug + ",keep,,brand names each room by city\n"

	out, summary := s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusKept])
	s.Equal("House of Blues Chicago", s.venueByID(room.ID).Name)
}

// Every refusal is reported, writes nothing for its row, and does not stop the
// rows around it.
func (s *WorklistSuite) TestRefusedRowsAreReportedAndTheRunContinues() {
	tucson := s.venue("Rialto Theatre", "Tucson", "AZ")
	tucsonTwin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	phoenix := s.venue("Rialto Theatre", "Phoenix", "AZ")
	portlandME := s.venue("State Theatre Portland", "Portland", "ME")
	portlandOR := s.venue("State Theatre", "Portland", "OR")
	cobalt := s.venue("Cobalt", "Vancouver", "BC")
	cobaltTwin := s.venue("The Cobalt Vancouver", "Vancouver", "BC")
	dup := s.venue("Twice Listed Denver", "Denver", "CO")
	chainA := s.venue("Chain A Boise", "Boise", "ID")
	chainB := s.venue("Chain B Boise", "Boise", "ID")
	chainC := s.venue("Chain C", "Boise", "ID")
	sameA := s.venue("Same Name Reno", "Reno", "NV")
	sameB := s.venue("Same Name Reno NV", "Reno", "NV")
	scout := s.venue("Scout Bar Houston", "Houston", "TX")
	keepRoom := s.venue("Keep Room Austin", "Austin", "TX")
	lonely := s.venue("Lonely Room", "Tucson", "AZ")

	rows := [][]string{
		{"no-such-room", "merge", *lonely.Slug},         // unknown room
		{*tucsonTwin.Slug, "merge", "no-such-target"},   // unknown target
		{*tucson.Slug, "merge", *tucson.Slug},           // the room itself
		{*phoenix.Slug, "merge", *tucson.Slug},          // another city
		{*portlandME.Slug, "merge", *portlandOR.Slug},   // same city name, another state
		{*cobaltTwin.Slug, "rename", "The Cobalt"},      // name already in the place
		{*dup.Slug, "keep", ""},                         // on two rows
		{*dup.Slug, "rename", "Twice Listed"},           // on two rows
		{*chainA.Slug, "merge", *chainB.Slug},           // target merged away below
		{*chainB.Slug, "merge", *chainC.Slug},           // merged away and a target above
		{*sameA.Slug, "rename", "Same Name"},            // two renames to one name
		{*sameB.Slug, "rename", "same name"},            // two renames to one name
		{*keepRoom.Slug, "keep", "Keep Room"},           // keep with a target
		{*scout.Slug + "-x", "demolish", ""},            // unknown action
		{*cobalt.Slug, "rename", ""},                    // rename with no name
		{"", "keep", ""},                                // no slug
		{*scout.Slug + "-y", "keep", ""},                // keep of an unknown room
		{*tucsonTwin.Slug + "-unused", "rename", "!!!"}, // name with nothing in it
	}
	var b strings.Builder
	b.WriteString("room_slug,action,target\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, ",") + "\n")
	}
	b.WriteString(*scout.Slug + ",rename,Scout Bar\n") // the one good row, last

	before := s.count(`SELECT COUNT(*) FROM venues`)
	out, summary := s.run(b.String(), true)

	s.True(summary.Refused())
	s.Equal(len(rows), summary[statusRefused], out)
	s.Equal(1, summary[statusApplied], out)
	s.Equal("Scout Bar", s.venueByID(scout.ID).Name, "the good row after the refusals is applied")
	s.Equal(before, s.count(`SELECT COUNT(*) FROM venues`), "no refused merge deleted a room")
	s.Equal("The Cobalt Vancouver", s.venueByID(cobaltTwin.ID).Name)
	s.Equal("Twice Listed Denver", s.venueByID(dup.ID).Name)
	s.Equal("Same Name Reno", s.venueByID(sameA.ID).Name)

	for _, want := range []string{
		"no room has slug no-such-room",
		"no room has target slug no-such-target",
		"is the room itself",
		`different places: "Rialto Theatre" is in Phoenix, AZ and "Rialto Theatre" is in Tucson, AZ`,
		`different places: "State Theatre Portland" is in Portland, ME and "State Theatre" is in Portland, OR`,
		`already has "Cobalt"`,
		"appears on 2 rows",
		"is itself merged away on line 11",
		"is the merge target on line 10",
		"would give two rooms in Reno the same name",
		`keep takes no target, got "Keep Room"`,
		`unknown action "demolish"`,
		"rename needs a target",
		"room_slug is empty",
		"no room has slug " + *scout.Slug + "-y",
		`rename target "!!!" has no letters or digits`,
	} {
		s.Contains(out, want)
	}
}

// UpdateVenue's own check refuses a name another room of the same city NAME
// already has, in any state; the dry run reaches it by running the update and
// rolling it back.
func (s *WorklistSuite) TestDryRunReportsTheRefusalTheUpdateWouldGive() {
	s.venue("The Den", "Portland", "ME")
	room := s.venue("The Den Portland", "Portland", "OR")
	csv := "room_slug,action,target\n" + *room.Slug + ",rename,The Den\n"

	out, summary := s.run(csv, false)

	s.True(summary.Refused())
	s.Contains(out, "already exists in Portland")
	s.Equal("The Den Portland", s.venueByID(room.ID).Name)
}

// A second run over the same file changes nothing and refuses nothing.
func (s *WorklistSuite) TestSecondRunIsIdempotent() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	s.show("twin show", twin)
	renamed := s.venue("Scout Bar Houston", "Houston", "TX")
	keep := s.venue("House of Blues Chicago", "Chicago", "IL")
	csv := "room_slug,action,target\n" +
		*twin.Slug + ",merge," + *kept.Slug + "\n" +
		*renamed.Slug + ",rename,Scout Bar\n" +
		*keep.Slug + ",keep,\n"

	out, summary := s.run(csv, true)
	s.Require().False(summary.Refused(), out)
	s.Require().Equal(2, summary[statusApplied])
	s.waitForMergeAudit(kept.ID)
	venuesAfterFirst := s.count(`SELECT COUNT(*) FROM venues`)
	linksAfterFirst := s.count(`SELECT COUNT(*) FROM show_venues WHERE venue_id = ?`, kept.ID)

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(0, summary[statusApplied])
	s.Equal(2, summary[statusDone], out)
	s.Equal(1, summary[statusKept])
	s.Contains(out, "was merged into "+*kept.Slug)
	s.Equal(venuesAfterFirst, s.count(`SELECT COUNT(*) FROM venues`))
	s.Equal(linksAfterFirst, s.count(`SELECT COUNT(*) FROM show_venues WHERE venue_id = ?`, kept.ID))
}

// A missing room counts as already merged only when the merge record names
// both that slug and this target; otherwise it is an unknown slug.
func (s *WorklistSuite) TestMissingRoomWithoutAMergeRecordIsRefused() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	other := s.venue("Other Room", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	twinSlug := *twin.Slug

	_, summary := s.run("room_slug,action,target\n"+twinSlug+",merge,"+*other.Slug+"\n", true)
	s.Require().False(summary.Refused())
	s.waitForMergeAudit(other.ID)

	out, summary := s.run("room_slug,action,target\n"+twinSlug+",merge,"+*kept.Slug+"\n", false)

	s.True(summary.Refused(), "merged into a different room than this row names")
	s.Contains(out, "no room has slug "+twinSlug)
}

// Every write is recorded against the actor: the merge's audit row names it,
// and each rename leaves a revision naming it, with the old and new name. The
// dry run leaves neither.
func (s *WorklistSuite) TestWritesAreRecordedAgainstTheActor() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	room := s.venue("Scout Bar Houston", "Houston", "TX")
	csv := "room_slug,action,target\n" +
		*twin.Slug + ",merge," + *kept.Slug + "\n" +
		*room.Slug + ",rename,Scout Bar\n"

	_, summary := s.run(csv, false)
	s.Require().False(summary.Refused())
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM revisions`), "the dry run records nothing")

	_, summary = s.run(csv, true)
	s.Require().False(summary.Refused())
	s.waitForMergeAudit(kept.ID)

	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM audit_logs WHERE action = ? AND entity_id = ? AND actor_id = ?`,
		catalog.AuditActionMergeVenues, kept.ID, s.actor))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM revisions
		WHERE entity_type = 'venue' AND entity_id = ? AND user_id = ?
		  AND field_changes @> '[{"field": "name", "old_value": "Scout Bar Houston", "new_value": "Scout Bar"}]'`,
		room.ID, s.actor))
}

// Two renames to one name in two same-named cities of different states would
// pass the tool's own same-place rule, but the unique index on
// (lower(name), lower(city)) refuses the second. Both are refused up front, so
// the dry run and the confirmed run agree.
func (s *WorklistSuite) TestRenamesCollidingOnTheCityNameAcrossStatesAreRefused() {
	maine := s.venue("The Den Portland", "Portland", "ME")
	oregon := s.venue("Den Portland", "Portland", "OR")
	csv := "room_slug,action,target\n" + *maine.Slug + ",rename,The Den\n" + *oregon.Slug + ",rename,the den\n"

	out, summary := s.run(csv, true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "renames on lines 2, 3 would give two rooms in Portland the same name")
	s.Equal("The Den Portland", s.venueByID(maine.ID).Name)
	s.Equal("Den Portland", s.venueByID(oregon.ID).Name)
}

// Two renames whose names differ in more than case but normalize alike ("The
// Den" and "Den") are refused on the same-place key; the index would accept
// both, and the second would then fail the tool's own check under --confirm.
func (s *WorklistSuite) TestRenamesCollidingAfterNormalizationAreRefused() {
	x := s.venue("X Room Tucson", "Tucson", "AZ")
	y := s.venue("Y Room Tucson", "Tucson", "AZ")
	csv := "room_slug,action,target\n" + *x.Slug + ",rename,The Den\n" + *y.Slug + ",rename,Den\n"

	out, summary := s.run(csv, true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "renames on lines 2, 3 would give two rooms in Tucson the same name")
	s.Equal("X Room Tucson", s.venueByID(x.ID).Name)
	s.Equal("Y Room Tucson", s.venueByID(y.ID).Name)
}

// The rename's same-place check finds a room whose city is stored with stray
// whitespace, because the place comparison is trimmed.
func (s *WorklistSuite) TestRenameCollisionCheckIgnoresCityPadding() {
	padded := s.venue("Rialto Theatre", "Tucson ", "AZ")
	room := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	csv := "room_slug,action,target\n" + *room.Slug + ",rename,The Rialto Theater\n"

	out, summary := s.run(csv, true)

	s.True(summary.Refused(), out)
	s.Contains(out, `already has "Rialto Theatre" (slug `+*padded.Slug+`)`)
	s.Equal("Rialto Theatre Tucson", s.venueByID(room.ID).Name)
}

// Two merges into one room are refused: the second merge's preview cannot see
// the shows the first one moves.
func (s *WorklistSuite) TestTwoMergesIntoOneRoomAreRefused() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	a := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	b := s.venue("Rialto Theater Tucson AZ", "Tucson", "AZ")
	csv := "room_slug,action,target\n" + *a.Slug + ",merge," + *kept.Slug + "\n" + *b.Slug + ",merge," + *kept.Slug + "\n"

	out, summary := s.run(csv, true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "receives merges on lines 2, 3")
	s.NotNil(s.venueByID(a.ID))
	s.NotNil(s.venueByID(b.ID))

	// Applying one of them on its own, then running the whole file again, applies
	// the other: a merge already done moves nothing and entangles nothing.
	_, summary = s.run("room_slug,action,target\n"+*a.Slug+",merge,"+*kept.Slug+"\n", true)
	s.Require().False(summary.Refused())
	s.waitForMergeAudit(kept.ID)

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusDone], out)
	s.Equal(1, summary[statusApplied], out)
	s.Nil(s.venueByID(b.ID))
}

// A three-link chain is refused whole whatever order its rows come in: every
// link either merges away another row's target or receives another row's
// merge, and the verdicts are taken over the plan before any refusal.
func (s *WorklistSuite) TestThreeLinkChainIsRefusedInEitherOrder() {
	x := s.venue("Chain X Boise", "Boise", "ID")
	y := s.venue("Chain Y Boise", "Boise", "ID")
	z := s.venue("Chain Z Boise", "Boise", "ID")
	w := s.venue("Chain W", "Boise", "ID")
	a := *x.Slug + ",merge," + *y.Slug + "\n"
	b := *y.Slug + ",merge," + *z.Slug + "\n"
	c := *z.Slug + ",merge," + *w.Slug + "\n"

	for name, csv := range map[string]string{
		"A, B, C": "room_slug,action,target\n" + a + b + c,
		"C, B, A": "room_slug,action,target\n" + c + b + a,
	} {
		out, summary := s.run(csv, true)
		s.Equal(3, summary[statusRefused], "%s: %s", name, out)
		s.Equal(0, summary[statusApplied], "%s: %s", name, out)
		for _, v := range []*catalogm.Venue{x, y, z, w} {
			s.NotNil(s.venueByID(v.ID), "%s: no room is merged away", name)
		}
	}
}

// A merge chain stays refused after its first link was applied by an earlier
// run: merging the kept room away would leave the done row with no target, and
// the file would never run clean again.
func (s *WorklistSuite) TestChainIsRefusedAfterItsFirstLinkIsApplied() {
	x := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	y := s.venue("Rialto Theater", "Tucson", "AZ")
	z := s.venue("Rialto Theatre", "Tucson", "AZ")
	first := "room_slug,action,target\n" + *x.Slug + ",merge," + *y.Slug + "\n"
	_, summary := s.run(first, true)
	s.Require().False(summary.Refused())
	s.waitForMergeAudit(y.ID)

	out, summary := s.run(first+*y.Slug+",merge,"+*z.Slug+"\n", true)

	s.Equal(1, summary[statusDone], out)
	s.Equal(1, summary[statusRefused], out)
	s.Contains(out, "room "+*y.Slug+" already received the merge on line 2")
	s.NotNil(s.venueByID(y.ID), "the kept room is not merged away")
}

// The dry run shows each room's verification, and warns when a verified room
// would be merged into an unverified one, whose fields the merge keeps.
func (s *WorklistSuite) TestMergeIntoAnUnverifiedRoomIsFlagged() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", twin.ID).Update("verified", true).Error)

	out, summary := s.run("room_slug,action,target\n"+*twin.Slug+",merge,"+*kept.Slug+"\n", false)

	s.False(summary.Refused())
	s.Contains(out, "slug="+*twin.Slug+" shows=0 verified=true")
	s.Contains(out, "slug="+*kept.Slug+" shows=0 verified=false")
	s.Contains(out, "WARNING: the room kept is unverified")
}

func (s *WorklistSuite) TestRenameTargetShapedLikeASlugIsRefused() {
	room := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	csv := "room_slug,action,target\n" + *room.Slug + ",rename,rialto-theatre-tucson-az\n"

	out, summary := s.run(csv, true)

	s.True(summary.Refused())
	s.Contains(out, "looks like a slug")
	s.Equal("Rialto Theatre Tucson", s.venueByID(room.ID).Name)
}

func (s *WorklistSuite) TestTheActorMustBeAnActiveAdmin() {
	s.NoError(requireAdminActor(s.db, s.actor))
	s.ErrorContains(requireAdminActor(s.db, 0), "--actor is required")
	s.ErrorContains(requireAdminActor(s.db, s.actor+1000), "no such user")
	fan := s.user()
	s.ErrorContains(requireAdminActor(s.db, fan), "not an active admin")
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", s.actor).Update("is_active", false).Error)
	s.ErrorContains(requireAdminActor(s.db, s.actor), "not an active admin")
}

// A confirmed merge whose audit row is missing is reported, because without it
// a later run cannot recognise the merge.
func (s *WorklistSuite) TestMergeWithoutAnAuditRowIsReported() {
	kept := s.venue("Rialto Theatre", "Tucson", "AZ")
	twin := s.venue("Rialto Theatre Tucson", "Tucson", "AZ")
	_, report := s.runReport("room_slug,action,target\n"+*twin.Slug+",merge,"+*kept.Slug+"\n", true)
	s.Require().Len(report.Merged, 1)
	s.waitForMergeAudit(kept.ID)

	var out bytes.Buffer
	s.Equal(0, reportUnrecordedMerges(s.db, report, &out), out.String())

	s.Require().NoError(s.db.Exec(`DELETE FROM audit_logs`).Error)
	out.Reset()
	s.Equal(1, reportUnrecordedMerges(s.db, report, &out))
	s.Contains(out.String(), "WARNING: merge of "+*twin.Slug+" into "+*kept.Slug+" (line 2) is applied but has no audit row")
}
