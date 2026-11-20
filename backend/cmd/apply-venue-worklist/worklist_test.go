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
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	var out bytes.Buffer
	summary := runWorklist(s.db, rows, confirm, &out)
	return out.String(), summary
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

	rows := [][]string{
		{"no-such-room", "merge", *tucson.Slug},         // unknown room
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
		"different places (Phoenix, AZ and Tucson, AZ)",
		"different places (Portland, ME and Portland, OR)",
		`already has "Cobalt"`,
		"appears on 2 rows",
		"is itself merged away on line 11",
		"is the merge target on line 10",
		"would give two rooms in Reno, NV the same name",
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
