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

func worklistGroups() []catalog.SameLocalDateGroup {
	return []catalog.SameLocalDateGroup{
		{
			Room:      catalog.SameLocalDateRoom{ID: 7, Slug: "the-casbah", Name: "The Casbah", City: "San Diego", State: "CA"},
			LocalDate: "2026-12-06", HeadlinerID: 3, HeadlinerName: "The Beat Farmers",
			Copies: []catalog.SameLocalDateCopy{
				{ShowID: 21, Slug: "2026-12-06-the-beat-farmers-at-casbah", EventDate: at("2026-12-07T00:00:00Z"),
					LocalTime: "16:00 PST", CreatedAt: at("2026-09-13T23:54:13Z"), Source: "user", RoomCount: 1},
				{ShowID: 11, Slug: "2026-12-06-the-beat-farmers-at-the-casbah", EventDate: at("2026-12-07T04:00:00Z"),
					LocalTime: "20:00 PST", DateOnlyAnchor: true, CreatedAt: at("2026-08-30T01:11:30Z"), Source: "user",
					TicketURL: "https://tickets.example/beat-farmers", Saves: 2, RoomCount: 1},
			},
		},
		{
			Room:      catalog.SameLocalDateRoom{ID: 9, Slug: "space", Name: "SPACE", City: "Evanston", State: "IL"},
			LocalDate: "2026-08-07", HeadlinerID: 4, HeadlinerName: "+/- (Plus Minus)",
			Copies: []catalog.SameLocalDateCopy{
				{ShowID: 31, Slug: "2026-08-07-plus-minus-at-space-2", EventDate: at("2026-08-07T20:00:00Z"),
					LocalTime: "15:00 CDT", CreatedAt: at("2026-07-30T02:45:34Z"), Source: "user", RoomCount: 1},
				{ShowID: 30, Slug: "2026-08-07-plus-minus-at-space", EventDate: at("2026-08-08T01:00:00Z"),
					LocalTime: "20:00 CDT", CreatedAt: at("2026-07-30T02:38:44Z"), Source: "user", RoomCount: 1},
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
	require.Len(t, records, 3)
	assert.Equal(t, sameLocalDateCSVHeader, records[0])

	// The copy with saves is kept although the other states a time.
	merge := records[1]
	assert.Equal(t, "2026-12-06-the-beat-farmers-at-casbah", merge[0])
	assert.Equal(t, "the-casbah", merge[1], "the room the copy is already in")
	assert.Equal(t, "2026-12-06-the-beat-farmers-at-the-casbah", merge[3])
	assert.True(t, strings.HasPrefix(merge[4], "merge (suggested): keep 2026-12-06-the-beat-farmers-at-the-casbah"))
	assert.Contains(t, merge[4], "time (16:00 PST) is not carried")
	assert.Contains(t, merge[2], "20:00 PST (20:00 date-only anchor, no music time stored)")

	// Both state a time five hours apart: no merge target.
	both := records[2]
	assert.Equal(t, "2026-08-07-plus-minus-at-space-2", both[0])
	assert.Empty(t, both[3])
	assert.True(t, strings.HasPrefix(both[4], "keep both (suggested): "))
	assert.True(t, strings.HasPrefix(both[2], "'+/- (Plus Minus) at SPACE"), "a note starting with + is not a formula")
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

func TestPrintSameLocalDateGroupsStatesTheRuleAndMarksTheKeptCopy(t *testing.T) {
	var buf bytes.Buffer
	printSameLocalDateGroups(&buf, worklistGroups())
	out := buf.String()
	assert.Contains(t, out, catalog.SameLocalDateRuleText)
	assert.Contains(t, out, "  * show 11  2026-12-06-the-beat-farmers-at-the-casbah")
	assert.Contains(t, out, "suggested for 2026-08-07-plus-minus-at-space-2: keep both")
}
