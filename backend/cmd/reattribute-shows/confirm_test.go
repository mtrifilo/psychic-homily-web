package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

// ConfirmedRowSuite drives applyConfirmedRow, the --confirm path, against a
// real database: each row's own transaction, its timeouts, and its record line.
type ConfirmedRowSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func TestConfirmedRowSuite(t *testing.T) {
	suite.Run(t, new(ConfirmedRowSuite))
}

func (s *ConfirmedRowSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ConfirmedRowSuite) TearDownSuite() {
	if s.testDB != nil {
		s.testDB.Cleanup()
	}
}

func (s *ConfirmedRowSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{"show_artists", "show_venues", "shows", "artists", "venues"} {
		_, _ = sqlDB.Exec(fmt.Sprintf("DELETE FROM %s", t))
	}
}

var evening = time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC) // 8pm Mountain, Nov 25

func (s *ConfirmedRowSuite) room(slug string) uint {
	tz := "America/Denver"
	v := &catalogm.Venue{Name: slug, Slug: &slug, City: "Albuquerque", State: "NM", Verified: true, Timezone: &tz}
	s.Require().NoError(s.db.Create(v).Error)
	return v.ID
}

func (s *ConfirmedRowSuite) show(slug string, roomID uint, act string) uint {
	a := &catalogm.Artist{Name: act, Slug: &act}
	s.Require().NoError(s.db.Create(a).Error)
	var id uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO shows (title, slug, event_date, state, status, source, created_at, updated_at)
		VALUES (?, ?, ?, 'NM', 'approved', 'user', NOW(), NOW()) RETURNING id`, slug, slug, evening).Row().Scan(&id))
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, roomID).Error)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')`, id, a.ID).Error)
	return id
}

func (s *ConfirmedRowSuite) roomOf(showID uint) uint {
	var ids []uint
	s.Require().NoError(s.db.Model(&catalogm.ShowVenue{}).Where("show_id = ?", showID).Pluck("venue_id", &ids).Error)
	s.Require().Len(ids, 1)
	return ids[0]
}

func (s *ConfirmedRowSuite) recordTo(path string) (*recorder, func()) {
	f, err := openRecord(path)
	s.Require().NoError(err)
	return &recorder{w: f, sync: f.Sync, now: time.Now}, func() { _ = f.Close() }
}

func (s *ConfirmedRowSuite) readRecord(path string) []recordLine {
	b, err := os.ReadFile(path)
	s.Require().NoError(err)
	var out []recordLine
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var l recordLine
		s.Require().NoError(json.Unmarshal([]byte(line), &l), line)
		out = append(out, l)
	}
	return out
}

func confirmRows(rows ...catalog.ShowReattribution) []worklistRow {
	out := make([]worklistRow, len(rows))
	for i, r := range rows {
		out[i] = worklistRow{Line: i + 2, Row: r}
	}
	return out
}

func (s *ConfirmedRowSuite) TestEachAppliedRowIsRecordedAndARefusedOneIsNot() {
	launchpad := s.room("launchpad")
	sunshine := s.room("sunshine")
	amity := s.show("amity", launchpad, "Amity")
	winner := s.show("aild", sunshine, "AILD")
	loser := s.show("aild-2", launchpad, "AILD Twin")
	path := filepath.Join(s.T().TempDir(), "run.jsonl")
	rec, done := s.recordTo(path)
	defer done()
	var out bytes.Buffer

	tally := run(&report{w: &out}, confirmRows(
		catalog.ShowReattribution{ShowSlug: "amity", RoomSlug: "sunshine"},
		catalog.ShowReattribution{ShowSlug: "missing", RoomSlug: "sunshine"},
		catalog.ShowReattribution{ShowSlug: "aild-2", RoomSlug: "sunshine", MergeIntoSlug: "aild"},
	), true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		return applyConfirmedRow(s.db, rec, row, transactionTimeouts)
	})

	s.Require().NoError(tally.stopped)
	s.Equal(1, tally.moved)
	s.Equal(1, tally.merged)
	s.Equal(1, tally.refused)
	lines := s.readRecord(path)
	s.Require().Len(lines, 2, "one line per applied row, none for the refused row")

	moved := lines[0]
	s.Equal(recordMoved, moved.Outcome)
	s.Equal(2, moved.Line)
	s.Equal(amity, moved.ShowID)
	s.Equal("amity", moved.ShowSlug)
	s.Equal(launchpad, moved.FromRoomID)
	s.Equal("launchpad", moved.FromRoomSlug)
	s.Equal(sunshine, moved.ToRoomID)
	s.Equal("sunshine", moved.ToRoomSlug)
	_, err := time.Parse(time.RFC3339Nano, moved.At)
	s.NoError(err, "each line carries its time")

	merged := lines[1]
	s.Equal(recordMerged, merged.Outcome)
	s.Equal(4, merged.Line)
	s.Equal(winner, merged.KeptShowID)
	s.Equal("aild", merged.KeptShowSlug)
	s.Equal(loser, merged.DeletedShowID)
	s.Equal(launchpad, merged.FromRoomID)
	s.Equal(sunshine, merged.ToRoomID)

	s.Equal(sunshine, s.roomOf(amity))
}

// A confirmed row that hits its lock timeout fails, is rolled back, writes no
// record line, and stops the run before the next row.
func (s *ConfirmedRowSuite) TestATimeoutRollsTheRowBackAndStopsTheRun() {
	launchpad := s.room("launchpad")
	s.room("sunshine")
	locked := s.show("locked", launchpad, "Locked Act")
	next := s.show("next", launchpad, "Next Act")
	path := filepath.Join(s.T().TempDir(), "run.jsonl")
	rec, done := s.recordTo(path)
	defer done()

	holder := s.db.Begin()
	s.Require().NoError(holder.Error)
	defer func() { _ = holder.Rollback() }()
	s.Require().NoError(holder.Exec(`SELECT id FROM shows WHERE id = ? FOR UPDATE`, locked).Error)
	short := []string{"SET LOCAL lock_timeout = '200ms'", "SET LOCAL statement_timeout = '5s'"}
	var out bytes.Buffer

	tally := run(&report{w: &out}, confirmRows(
		catalog.ShowReattribution{ShowSlug: "locked", RoomSlug: "sunshine"},
		catalog.ShowReattribution{ShowSlug: "next", RoomSlug: "sunshine"},
	), true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		return applyConfirmedRow(s.db, rec, row, short)
	})
	s.Require().NoError(holder.Rollback().Error)

	s.Require().Error(tally.stopped)
	s.Equal(1, tally.failed)
	s.Equal(0, tally.moved, "the row after the timeout is not run")
	s.Contains(out.String(), "STOPPED at line 2")
	s.Contains(out.String(), "lock")
	s.Empty(s.readRecord(path))
	s.Equal(launchpad, s.roomOf(locked))
	s.Equal(launchpad, s.roomOf(next))
}

// brokenRecord accepts no write.
type brokenRecord struct{}

func (brokenRecord) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// A record line that cannot be written rolls its row back and stops the run.
func (s *ConfirmedRowSuite) TestARecordThatCannotBeWrittenRollsTheRowBackAndStops() {
	launchpad := s.room("launchpad")
	s.room("sunshine")
	first := s.show("first", launchpad, "First Act")
	second := s.show("second", launchpad, "Second Act")
	rec := &recorder{w: brokenRecord{}, sync: func() error { return nil }, now: time.Now}
	var out bytes.Buffer

	tally := run(&report{w: &out}, confirmRows(
		catalog.ShowReattribution{ShowSlug: "first", RoomSlug: "sunshine"},
		catalog.ShowReattribution{ShowSlug: "second", RoomSlug: "sunshine"},
	), true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		return applyConfirmedRow(s.db, rec, row, transactionTimeouts)
	})

	s.Require().Error(tally.stopped)
	s.Contains(tally.stopped.Error(), "the record file could not be written")
	s.Equal(1, tally.failed)
	s.Equal(0, tally.moved)
	s.Contains(out.String(), "the record line could not be written, so the row was rolled back: disk full")
	s.Contains(out.String(), "No row was applied.")
	s.Equal(launchpad, s.roomOf(first), "the row is rolled back")
	s.Equal(launchpad, s.roomOf(second), "the row after it is not run")
}

// A sync that fails is a record that cannot be written.
func (s *ConfirmedRowSuite) TestARecordThatCannotBeSyncedRollsTheRowBack() {
	launchpad := s.room("launchpad")
	s.room("sunshine")
	id := s.show("only", launchpad, "Only Act")
	var buf bytes.Buffer
	rec := &recorder{w: &buf, sync: func() error { return errors.New("fsync failed") }, now: time.Now}

	res, err := applyConfirmedRow(s.db, rec, worklistRow{Line: 2,
		Row: catalog.ShowReattribution{ShowSlug: "only", RoomSlug: "sunshine"}}, transactionTimeouts)

	s.Require().Error(err)
	s.Equal(catalog.ReattributionFailed, res.Outcome)
	s.Equal(launchpad, s.roomOf(id))
}
