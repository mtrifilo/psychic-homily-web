package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/catalog"
)

func TestParseWorklist_ReadsColumnsByNameAndIgnoresEvidence(t *testing.T) {
	in := "\ufeffsection,Show_Slug,correct_room_slug,note,page_read,merge_into_show_slug\n" +
		"A,aild-2,sunshine,\"merge, see page\",https://example.test/1,aild\n" +
		"A, amity ,sunshine,,https://example.test/2,\n" +
		"B,locked-shut,,no action,https://example.test/3,\n" +
		",,,,,\n"

	rows, err := parseWorklist(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, rows, 3, "a row with no slugs at all is not a row")

	assert.Equal(t, catalog.ShowReattribution{ShowSlug: "aild-2", RoomSlug: "sunshine", MergeIntoSlug: "aild"}, rows[0].Row)
	assert.Equal(t, "merge, see page", rows[0].Note)
	assert.Equal(t, 2, rows[0].Line)
	assert.Equal(t, "amity", rows[1].Row.ShowSlug, "cells are trimmed")
	assert.Empty(t, rows[1].Row.MergeIntoSlug)
	assert.Equal(t, "no target room given", rows[2].Refusal, "a row with no target room is refused")
}

func TestParseWorklist_LineIsTheFileLineAcrossQuotedNewlines(t *testing.T) {
	in := "show_slug,correct_room_slug,note\n" +
		"one,room,\"two\nline note\"\n" +
		"two,room,\n"

	rows, err := parseWorklist(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, 2, rows[0].Line)
	assert.Equal(t, 4, rows[1].Line)
}

func TestParseWorklist_RefusesRowsItCanJudgeWithoutTheDatabase(t *testing.T) {
	in := "show_slug,correct_room_slug,merge_into_show_slug\n" +
		",sunshine,\n" +
		"cenit,,cenit-mezz\n" +
		"bodysnatcher,sunshine,\n" +
		"bodysnatcher,el-rey,\n"

	rows, err := parseWorklist(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, "no show slug", rows[0].Refusal)
	assert.Equal(t, "no target room given", rows[1].Refusal)
	assert.Empty(t, rows[2].Refusal)
	assert.Equal(t, "show already named on line 4", rows[3].Refusal)
}

func TestParseWorklist_RejectsAnUnusableFile(t *testing.T) {
	for name, in := range map[string]string{
		"empty":            "",
		"no room column":   "show_slug,room\nx,y\n",
		"no show column":   "slug,correct_room_slug\nx,y\n",
		"duplicate header": "show_slug,correct_room_slug,show_slug\nx,y,z\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseWorklist(strings.NewReader(in))
			assert.Error(t, err)
		})
	}
}

// Rows refused while parsing never reach the database, and every other row is
// handed to reattribute in file order.
func TestRun_ReportsRefusedRowsAndSendsTheRestInOrder(t *testing.T) {
	rows := []worklistRow{
		{Line: 2, Row: catalog.ShowReattribution{RoomSlug: "sunshine"}, Refusal: "no show slug"},
		{Line: 3, Row: catalog.ShowReattribution{ShowSlug: "locked-shut"}, Refusal: "no target room given", Note: "right room"},
		{Line: 4, Row: catalog.ShowReattribution{ShowSlug: "cenit", RoomSlug: "el-rey"}},
		{Line: 5, Row: catalog.ShowReattribution{ShowSlug: "cenit-mezz", RoomSlug: "el-rey", MergeIntoSlug: "cenit"}},
	}
	var out bytes.Buffer
	var sent []string

	tally := run(&out, rows, false, func(r catalog.ShowReattribution) catalog.ShowReattributionResult {
		sent = append(sent, r.ShowSlug)
		if r.MergeIntoSlug != "" {
			return catalog.ShowReattributionResult{ShowSlug: r.ShowSlug, Outcome: catalog.ReattributionFailed, Reason: "boom"}
		}
		return catalog.ShowReattributionResult{ShowSlug: r.ShowSlug, Outcome: catalog.ReattributionMoved}
	})

	assert.Equal(t, []string{"cenit", "cenit-mezz"}, sent)
	assert.Equal(t, 2, tally.refused)
	assert.Equal(t, 1, tally.moved)
	assert.Equal(t, 1, tally.failed)
	assert.Contains(t, out.String(), "line 4  WOULD BE MOVED  cenit")
	assert.Contains(t, out.String(), "line 5  FAILED  cenit-mezz")
	text := out.String()
	assert.Contains(t, text, "DRY RUN")
	assert.Contains(t, text, "line 2  REFUSED")
	assert.Contains(t, text, "reason: no show slug")
	assert.Contains(t, text, "line 3  REFUSED  locked-shut")
	assert.Contains(t, text, "reason: no target room given")
	assert.Contains(t, text, "note: right room")
	assert.Contains(t, text, "refused:   2")
}

func TestPrintResult_NamesWhatAMergeKeepsAndWhatItRemoves(t *testing.T) {
	var out bytes.Buffer
	printResult(&out, worklistRow{Line: 5}, catalog.ShowReattributionResult{
		Outcome:            catalog.ReattributionMerged,
		ShowID:             7,
		ShowSlug:           "aild-2",
		ShowStatus:         "approved",
		Headliner:          "As I Lay Dying",
		VenueLocalDate:     "2026-11-25",
		CurrentRoom:        &catalog.ReattributionRoom{Name: "Launchpad", Slug: "launchpad", City: "Albuquerque", State: "NM"},
		TargetRoom:         &catalog.ReattributionRoom{Name: "Sunshine Theater", Slug: "sunshine", City: "Albuquerque", State: "NM"},
		MergeIntoID:        3,
		MergeIntoSlug:      "aild",
		MergeIntoStatus:    "approved",
		MergeIntoHeadliner: "As I Lay Dying",
		ActsAddedToWinner:  []catalog.ReattributionAct{{Name: "Kill The Lights", SetType: "performer", Position: 1}},
	}, false)

	text := out.String()
	assert.Contains(t, text, "line 5  WOULD BE MERGED  aild-2 (show 7, approved, As I Lay Dying, 2026-11-25)")
	assert.Contains(t, text, "room: Launchpad [launchpad] (Albuquerque, NM) -> Sunshine Theater [sunshine] (Albuquerque, NM)")
	assert.Contains(t, text, "merge into: aild (show 3, approved, As I Lay Dying)")
	assert.Contains(t, text, "aild keeps its slug, headliner and bill order; aild-2 stops answering")
	assert.Contains(t, text, "act added to its bill: Kill The Lights (performer, position 1)")
}

func TestPrintResult_WarnsAboutAShowAlreadyAtTheTargetThatNight(t *testing.T) {
	var out bytes.Buffer
	printResult(&out, worklistRow{Line: 9}, catalog.ShowReattributionResult{
		Outcome:  catalog.ReattributionMoved,
		ShowSlug: "cenit",
		SameNightAtTarget: []catalog.ReattributionNeighbour{
			{ID: 11, Slug: "cenit-mezz", Headliner: "Cenit / The Mezzanine", Status: "pending"},
		},
	}, true)

	text := out.String()
	assert.Contains(t, text, "line 9  MOVED  cenit")
	assert.Contains(t, text, "slug: kept as cenit (it names the old room)")
	assert.Contains(t, text,
		"WARNING: cenit-mezz (show 11, pending, Cenit / The Mezzanine) is also at the target room that night")
}

func TestPrintResult_ReplacesControlCharactersFromTheDatabase(t *testing.T) {
	var out bytes.Buffer
	printResult(&out, worklistRow{Line: 2, Note: "ok\x1b[2K"}, catalog.ShowReattributionResult{
		Outcome:   catalog.ReattributionRefused,
		ShowID:    1,
		ShowSlug:  "x",
		Headliner: "Act\x1b]0;owned\x07",
		Reason:    "show not found",
	}, true)

	text := out.String()
	assert.NotContains(t, text, "\x1b")
	assert.NotContains(t, text, "\x07")
	assert.Contains(t, text, "Act?]0;owned?")
}

func TestPrintMergeSummary_PrintsEveryCounterTheMergeFillsAndEveryTable(t *testing.T) {
	var out bytes.Buffer
	printMergeSummary(&out, &catalog.ShowDedupSummary{
		LosersMerged:      2,
		PendingEditsMoved: 3,
		EntityRefsMoved:   map[string]int64{"user_bookmarks": 4, "comments": 1},
		EntityRefsDropped: map[string]int64{"user_bookmarks": 1},
	})
	text := out.String()
	assert.Contains(t, text, "LosersMerged:          2")
	assert.Contains(t, text, "PendingEditsMoved:     3")
	assert.Contains(t, text, "ShowVenuesSkipped:     0", "a zero counter is printed too")
	assert.NotContains(t, text, "ClustersFound", "a counter this tool never fills is left out")
	assert.NotContains(t, text, "SlugsRewritten")
	assert.Contains(t, text, "user_bookmarks:        moved=4 skipped=1")
	assert.Less(t, strings.Index(text, "comments:"), strings.Index(text, "user_bookmarks:"), "tables in name order")
}
