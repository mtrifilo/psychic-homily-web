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
	assert.True(t, rows[2].Skip, "a checked row with no target room is skipped, not refused")
	assert.Empty(t, rows[2].Refusal)
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
	assert.Equal(t, "a merge target is named but no target room", rows[1].Refusal)
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

// Rows refused or skipped before the database is reached never touch it, so a
// nil handle is enough to pin how they are reported and counted.
func TestRun_ReportsRefusedAndSkippedRowsAndCountsThem(t *testing.T) {
	rows := []worklistRow{
		{Line: 2, Row: catalog.ShowReattribution{RoomSlug: "sunshine"}, Refusal: "no show slug"},
		{Line: 3, Row: catalog.ShowReattribution{ShowSlug: "locked-shut"}, Skip: true, Note: "right room"},
	}
	var out bytes.Buffer

	tally := run(&out, nil, rows, false)

	assert.Equal(t, 1, tally.refused)
	assert.Equal(t, 1, tally.skipped)
	text := out.String()
	assert.Contains(t, text, "DRY RUN")
	assert.Contains(t, text, "line 2  REFUSED")
	assert.Contains(t, text, "reason: no show slug")
	assert.Contains(t, text, "line 3  SKIPPED  locked-shut")
	assert.Contains(t, text, "note: right room")
	assert.Contains(t, text, "refused:   1")
}

func TestPrintResult_NamesWhatAMergeKeepsAndWhatItRemoves(t *testing.T) {
	var out bytes.Buffer
	printResult(&out, worklistRow{Line: 5}, catalog.ShowReattributionResult{
		Outcome:           catalog.ReattributionMerged,
		ShowID:            7,
		ShowSlug:          "aild-2",
		Headliner:         "As I Lay Dying",
		VenueLocalDate:    "2026-11-25",
		CurrentRoom:       &catalog.ReattributionRoom{Name: "Launchpad", Slug: "launchpad", City: "Albuquerque", State: "NM"},
		TargetRoom:        &catalog.ReattributionRoom{Name: "Sunshine Theater", Slug: "sunshine", City: "Albuquerque", State: "NM"},
		MergeIntoID:       3,
		MergeIntoSlug:     "aild",
		ActsAddedToWinner: []string{"Kill The Lights"},
	}, false)

	text := out.String()
	assert.Contains(t, text, "line 5  WOULD BE MERGED  aild-2 (show 7, As I Lay Dying, 2026-11-25)")
	assert.Contains(t, text, "room: Launchpad [launchpad] (Albuquerque, NM) -> Sunshine Theater [sunshine] (Albuquerque, NM)")
	assert.Contains(t, text, "merge into: aild (show 3), which keeps its slug; aild-2 stops answering")
	assert.Contains(t, text, "acts added to aild: Kill The Lights")
}

func TestPrintResult_WarnsAboutAShowAlreadyAtTheTargetThatNight(t *testing.T) {
	var out bytes.Buffer
	printResult(&out, worklistRow{Line: 9}, catalog.ShowReattributionResult{
		Outcome:  catalog.ReattributionMoved,
		ShowSlug: "cenit",
		SameNightAtTarget: []catalog.ReattributionNeighbour{
			{ID: 11, Slug: "cenit-mezz", Headliner: "Cenit / The Mezzanine"},
		},
	}, true)

	text := out.String()
	assert.Contains(t, text, "line 9  MOVED  cenit")
	assert.Contains(t, text, "slug: kept as cenit (it names the old room)")
	assert.Contains(t, text, "WARNING: cenit-mezz (show 11, Cenit / The Mezzanine) is also at the target room that night")
}
