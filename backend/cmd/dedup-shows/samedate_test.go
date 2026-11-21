package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/catalog"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/same_local_date_worklist.csv")

// goldenWorklist is the worklist reattribute-shows' own tests read, renamed, to
// prove that tool accepts it.
const goldenWorklist = "testdata/same_local_date_worklist.csv"

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// worklistGroups holds one group for each kind of row: a merge (an 8pm copy
// with a ticket link and saves, and an 8:30 copy stating its music time), a
// review (a stated 4pm and an untimed 8pm, a matinee gap apart), and a keep
// both (3pm and 8pm, both stated).
func worklistGroups() []catalog.SameLocalDateGroup {
	casbah := catalog.SameLocalDateRoom{ID: 7, Slug: "the-casbah", Name: "The Casbah", City: "San Diego", State: "CA"}
	untimed := func(local string) string {
		return local + " with no music time stored, so it may be the date-only anchor"
	}
	return []catalog.SameLocalDateGroup{
		{
			Room: casbah, LocalDate: "2026-12-09", HeadlinerID: 2, HeadlinerName: "MARIS",
			Copies: []catalog.SameLocalDateCopy{
				{ShowID: 12, Slug: "2026-12-09-maris-at-the-casbah", Title: "MARIS", EventDate: at("2026-12-10T04:00:00Z"),
					LocalTime: "20:00 PST", Clock: untimed("20:00 PST"), CreatedAt: at("2026-08-30T01:11:30Z"),
					Source: "user", TicketURL: "https://tickets.example/maris", DroppedFields: []string{"ticket link"},
					Saves: 2, RoomCount: 1},
				{ShowID: 22, Slug: "2026-12-09-maris-at-casbah", Title: "MARIS", EventDate: at("2026-12-10T04:30:00Z"),
					LocalTime: "20:30 PST", StartAt: at("2026-12-10T04:30:00Z"), Clock: "music 20:30 PST",
					CreatedAt: at("2026-09-13T23:54:13Z"), Source: "user", RoomCount: 1},
			},
		},
		{
			Room: casbah, LocalDate: "2026-12-06", HeadlinerID: 3, HeadlinerName: "The Beat Farmers",
			Copies: []catalog.SameLocalDateCopy{
				{ShowID: 21, Slug: "2026-12-06-the-beat-farmers-at-casbah", EventDate: at("2026-12-07T00:00:00Z"),
					LocalTime: "16:00 PST", StartAt: at("2026-12-07T00:00:00Z"), Clock: "start 16:00 PST",
					CreatedAt: at("2026-09-13T23:54:13Z"), Source: "user", RoomCount: 1},
				{ShowID: 11, Slug: "2026-12-06-the-beat-farmers-at-the-casbah", EventDate: at("2026-12-07T04:00:00Z"),
					LocalTime: "20:00 PST", Clock: untimed("20:00 PST"), CreatedAt: at("2026-08-30T01:11:30Z"),
					Source: "user", RoomCount: 1},
			},
		},
		{
			Room:      catalog.SameLocalDateRoom{ID: 9, Slug: "space", Name: "SPACE", City: "Evanston", State: "IL"},
			LocalDate: "2026-08-07", HeadlinerID: 4, HeadlinerName: "+/- (Plus Minus)",
			Copies: []catalog.SameLocalDateCopy{
				{ShowID: 31, Slug: "2026-08-07-plus-minus-at-space-2", EventDate: at("2026-08-07T20:00:00Z"),
					LocalTime: "15:00 CDT", StartAt: at("2026-08-07T20:00:00Z"), Clock: "music 15:00 CDT",
					CreatedAt: at("2026-07-30T02:45:34Z"), Source: "user", RoomCount: 1},
				{ShowID: 30, Slug: "2026-08-07-plus-minus-at-space", EventDate: at("2026-08-08T01:00:00Z"),
					LocalTime: "20:00 CDT", StartAt: at("2026-08-08T01:00:00Z"), Clock: "music 20:00 CDT",
					CreatedAt: at("2026-07-30T02:38:44Z"), Source: "user", RoomCount: 1},
			},
		},
	}
}

func TestSameLocalDateWorklistMatchesTheGoldenFile(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeSameLocalDateCSV(&buf, worklistGroups()))
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenWorklist), 0o755))
		require.NoError(t, os.WriteFile(goldenWorklist, buf.Bytes(), 0o644))
	}
	want, err := os.ReadFile(goldenWorklist)
	require.NoError(t, err)
	assert.Equal(t, string(want), buf.String())
}

func TestSameLocalDateWorklistRows(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeSameLocalDateCSV(&buf, worklistGroups()))
	records, err := csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 5)
	assert.Equal(t, sameLocalDateCSVHeader, records[0])

	// The copy stating a start is kept although the other has saves and a ticket
	// link: the saves move with the merge, and the ticket link is named for
	// copying over by hand.
	merge := records[1]
	assert.Equal(t, "2026-12-09-maris-at-the-casbah", merge[0])
	assert.Equal(t, "the-casbah", merge[1], "the room the copy is already in")
	assert.Equal(t, "2026-12-09-maris-at-casbah", merge[3])
	assert.True(t, strings.HasPrefix(merge[4], "merge (suggested): keep 2026-12-09-maris-at-casbah"))
	assert.Contains(t, merge[4], "this copy holds ticket link, which the kept copy lacks and the merge drops")
	assert.Contains(t, merge[4], "this copy's 2 saves and 0 visible comments move to the kept copy")
	assert.Contains(t, merge[2], `This copy: 2026-12-09-maris-at-the-casbah titled "MARIS" (20:00 PST with no music time stored`)
	assert.Contains(t, merge[2], "fills: ticket link, 2 saves")
	assert.Contains(t, merge[2], `Copy suggested to keep: 2026-12-09-maris-at-casbah titled "MARIS" (music 20:30 PST`)

	// A wide pair with one untimed copy is review: a row for each copy, with no
	// room and no merge target, so reattribute-shows refuses both.
	for i, slug := range []string{"2026-12-06-the-beat-farmers-at-casbah", "2026-12-06-the-beat-farmers-at-the-casbah"} {
		review := records[2+i]
		assert.Equal(t, slug, review[0])
		assert.Empty(t, review[1])
		assert.Empty(t, review[3])
		assert.True(t, strings.HasPrefix(review[4], "review (suggested): the copies are 4h apart"), review[4])
	}

	// Both state a time five hours apart: no room and no merge target.
	both := records[4]
	assert.Equal(t, "2026-08-07-plus-minus-at-space-2", both[0])
	assert.Empty(t, both[1])
	assert.Empty(t, both[3])
	assert.True(t, strings.HasPrefix(both[4], "keep both (suggested): "))
	assert.True(t, strings.HasPrefix(both[2], "'+/- (Plus Minus) at SPACE"), "a note starting with + is not a formula")
}

func TestWorklistInstructionsNameTheRealColumns(t *testing.T) {
	for _, col := range sameLocalDateCSVHeader[3:] {
		assert.Contains(t, worklistInstructions, col)
		assert.Contains(t, worklistInstructions, col+" to "+strings.TrimPrefix(col, suggestedPrefix))
	}
}

func TestCleanReplacesControlAndBidiCharacters(t *testing.T) {
	assert.Equal(t, "a?b?c", clean("a\u202eb\nc"))
}

func TestValidateSameLocalDateFlags(t *testing.T) {
	assert.Error(t, validateSameLocalDateFlags(true, "/tmp/env"), "--confirm is refused")
	assert.Error(t, validateSameLocalDateFlags(false, " "), "--env is required")
	assert.NoError(t, validateSameLocalDateFlags(false, "/tmp/env"))
}

func TestCreateWorklistFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worklist.csv")
	require.NoError(t, os.WriteFile(path, []byte("reviewed\n"), 0o600))
	_, err := createWorklistFile(path)
	require.Error(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "reviewed\n", string(got))
}

func TestPrintSameLocalDateGroupsStatesTheRuleAndMarksOnlyAKeptCopy(t *testing.T) {
	var buf bytes.Buffer
	printSameLocalDateGroups(&buf, worklistGroups())
	out := buf.String()
	assert.Contains(t, out, catalog.SameLocalDateRuleText)
	assert.Contains(t, out, "  * show 22  2026-12-09-maris-at-casbah")
	assert.NotContains(t, out, "  * show 21", "a review group stars no copy")
	assert.NotContains(t, out, "  * show 11", "a review group stars no copy")
	assert.NotContains(t, out, "  * show 30", "a keep-both pair stars no copy")
	assert.NotContains(t, out, "  * show 31", "a keep-both pair stars no copy")
	assert.Contains(t, out, "suggested for 2026-08-07-plus-minus-at-space-2: keep both")
}
