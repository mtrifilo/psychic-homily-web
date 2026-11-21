package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		"unknown column":   "show_slug,correct_room_slug,merge_into\nx,y,z\n",
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

	tally := run(&report{w: &out}, rows, false, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		r := row.Row
		sent = append(sent, r.ShowSlug)
		if r.MergeIntoSlug != "" {
			return catalog.ShowReattributionResult{ShowSlug: r.ShowSlug, Outcome: catalog.ReattributionFailed, Reason: "boom"}, nil
		}
		return catalog.ShowReattributionResult{ShowSlug: r.ShowSlug, Outcome: catalog.ReattributionMoved}, nil
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
	printResult(&report{w: &out}, worklistRow{Line: 5}, catalog.ShowReattributionResult{
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
	printResult(&report{w: &out}, worklistRow{Line: 9}, catalog.ShowReattributionResult{
		Outcome:  catalog.ReattributionMoved,
		ShowSlug: "cenit",
		SameNightAtTarget: []catalog.ReattributionNeighbour{
			{ID: 11, Slug: "cenit-mezz", Headliner: "Cenit / The Mezzanine", Status: "approved"},
			{ID: 12, Slug: "cenit-rejected", Headliner: "Cenit", Status: "rejected"},
		},
	}, true)

	text := out.String()
	assert.Contains(t, text, "line 9  MOVED  cenit")
	assert.Contains(t, text, "slug: kept as cenit (it names the old room)")
	assert.Contains(t, text,
		"WARNING: cenit-mezz (show 11, approved, Cenit / The Mezzanine) is also at the target room that night; "+
			"add it as merge_into_show_slug if it is the same event")
	assert.Contains(t, text,
		"WARNING: cenit-rejected (show 12, rejected, Cenit) is also at the target room that night; "+
			"it is rejected, so it cannot be merged with")
}

func TestPrintResult_AMergeRowsNeighbourIsNotOfferedAsAMergeTarget(t *testing.T) {
	var out bytes.Buffer
	printResult(&report{w: &out}, worklistRow{Line: 4}, catalog.ShowReattributionResult{
		Outcome:           catalog.ReattributionMerged,
		ShowSlug:          "a",
		MergeIntoID:       2,
		MergeIntoSlug:     "b",
		SameNightAtTarget: []catalog.ReattributionNeighbour{{ID: 3, Slug: "c", Headliner: "C", Status: "approved"}},
	}, true)

	assert.Contains(t, out.String(), "WARNING: c (show 3, approved, C) is also at the target room that night; it stays after this merge")
}

func TestPrintResult_ReplacesControlCharactersFromTheDatabase(t *testing.T) {
	var out bytes.Buffer
	printResult(&report{w: &out}, worklistRow{Line: 2, Note: "ok\x1b[2K"}, catalog.ShowReattributionResult{
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

func TestClean_ReplacesBidiOverridesAndLineSeparators(t *testing.T) {
	assert.Equal(t, "A?B?C?D", clean("A\u202eB\u2066C\u2028D"))
	assert.Equal(t, "Mot\u00f6rhead", clean("Mot\u00f6rhead"), "ordinary letters are kept")
}

func TestPrintMergeSummary_PrintsEveryCounterTheMergeFillsAndEveryTable(t *testing.T) {
	var out bytes.Buffer
	printMergeSummary(&report{w: &out}, &catalog.ShowDedupSummary{
		LosersMerged:      2,
		PendingEditsMoved: 3,
		EntityRefsMoved:   map[string]int64{"user_bookmarks": 4, "comments": 1},
		EntityRefsDropped: map[string]int64{"user_bookmarks": 1},
	})
	text := out.String()
	assert.Contains(t, text, "LosersMerged:          2")
	assert.Contains(t, text, "PendingEditsMoved:     3")
	assert.Contains(t, text, "EnrichmentMoved:       0", "a zero counter is printed too")
	assert.NotContains(t, text, "ClustersFound", "a counter this tool never fills is left out")
	assert.NotContains(t, text, "SlugsRewritten")
	assert.NotContains(t, text, "ShowVenuesSkipped")
	assert.Contains(t, text, "user_bookmarks:        moved=4 skipped=1")
	assert.Less(t, strings.Index(text, "comments:"), strings.Index(text, "user_bookmarks:"), "tables in name order")
}

// The left-out counters are named by string; each must still be a field, or a
// rename would quietly print it again.
func TestMergeSummaryFieldsNotSet_NameRealFields(t *testing.T) {
	typ := reflect.TypeOf(catalog.ShowDedupSummary{})
	for name := range mergeSummaryFieldsNotSet {
		_, ok := typ.FieldByName(name)
		assert.True(t, ok, "ShowDedupSummary has no field %s", name)
	}
}

// A merge row refused before its target was read still says it is a merge, and
// does not offer a neighbour as the merge target.
func TestPrintResult_ARefusedMergeRowStillNamesItsTarget(t *testing.T) {
	var out bytes.Buffer
	printResult(&report{w: &out}, worklistRow{Line: 6}, catalog.ShowReattributionResult{
		Outcome:           catalog.ReattributionRefused,
		ShowSlug:          "x",
		MergeIntoSlug:     "y",
		Reason:            "n (show 9, approved) at the target room bills one of this show's acts at the same time",
		SameNightAtTarget: []catalog.ReattributionNeighbour{{ID: 9, Slug: "n", Headliner: "N", Status: "approved", Collides: true}},
	}, false)

	text := out.String()
	assert.Contains(t, text, "    merge into: y\n")
	assert.Contains(t, text, "it stays after this merge")
	assert.NotContains(t, text, "add it as merge_into_show_slug")
}

func TestRedactDBHost_DropsCredentials(t *testing.T) {
	assert.Equal(t, "db.example:5432/app", redactDBHost("postgres://user:secret@db.example:5432/app?sslmode=require"))
	assert.Equal(t, "(unparseable)", redactDBHost("not a url"))
}

// A misspelt column refuses the whole file and is named, so a merge column typed
// wrong cannot turn every merge into a plain move.
func TestParseWorklist_RefusesAnUnknownColumnByName(t *testing.T) {
	_, err := parseWorklist(strings.NewReader("show_slug,correct_room_slug,merge_into\nx,y,z\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown column "merge_into"`)
}

// The evidence columns of the reviewed worklist format are read past.
func TestParseWorklist_AcceptsTheEvidenceColumns(t *testing.T) {
	header := "section,show_slug,correct_room_slug,merge_into_show_slug,note," + strings.Join(evidenceColumns[1:], ",")
	rows, err := parseWorklist(strings.NewReader(header + "\nA,x,y,,n,a,b,c,d,e,f,g\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "x", rows[0].Row.ShowSlug)
}

// A row's stop error ends the run after that row is reported; the rows after it
// are not run, and the report says exactly which rows were applied.
func TestRun_StopsAtTheFirstRowThatStopsTheRun(t *testing.T) {
	rows := []worklistRow{
		{Line: 2, Row: catalog.ShowReattribution{ShowSlug: "a", RoomSlug: "r"}},
		{Line: 3, Row: catalog.ShowReattribution{ShowSlug: "b", RoomSlug: "r"}},
		{Line: 4, Row: catalog.ShowReattribution{ShowSlug: "c", RoomSlug: "r"}},
	}
	var out bytes.Buffer
	var sent []string

	tally := run(&report{w: &out}, rows, true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		sent = append(sent, row.Row.ShowSlug)
		if row.Row.ShowSlug == "b" {
			return catalog.ShowReattributionResult{ShowSlug: "b", Outcome: catalog.ReattributionFailed, Reason: "lock timeout"},
				errors.New("the row failed and was rolled back: lock timeout")
		}
		return catalog.ShowReattributionResult{ShowSlug: row.Row.ShowSlug, Outcome: catalog.ReattributionMoved}, nil
	})

	assert.Equal(t, []string{"a", "b"}, sent, "the row after the stop is not run")
	require.Error(t, tally.stopped)
	text := out.String()
	assert.Contains(t, text, "STOPPED at line 3: the row failed and was rolled back: lock timeout")
	assert.Contains(t, text, "1 row(s) after it were not run.")
	assert.Contains(t, text, "Applied, and only these: line 2 a (moved).")
}

// failingWriter accepts n writes and fails every one after.
type failingWriter struct {
	out  bytes.Buffer
	left int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.left <= 0 {
		return 0, errors.New("broken pipe")
	}
	w.left--
	return w.out.Write(p)
}

// When the report cannot be written, no further row is applied, and the
// applied rows are known exactly.
func TestRun_StopsBeforeTheNextRowWhenTheReportCannotBeWritten(t *testing.T) {
	rows := []worklistRow{
		{Line: 2, Row: catalog.ShowReattribution{ShowSlug: "a", RoomSlug: "r"}},
		{Line: 3, Row: catalog.ShowReattribution{ShowSlug: "b", RoomSlug: "r"}},
	}
	w := &failingWriter{left: 3} // the header and part of row a's block
	var sent []string

	tally := run(&report{w: w}, rows, true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
		sent = append(sent, row.Row.ShowSlug)
		return catalog.ShowReattributionResult{ShowSlug: row.Row.ShowSlug, Outcome: catalog.ReattributionMoved}, nil
	})

	assert.Equal(t, []string{"a"}, sent, "row b is not applied once the report fails")
	require.Error(t, tally.stopped)
	assert.Contains(t, tally.stopped.Error(), "the report could not be written")
	assert.Equal(t, "Applied, and only these: line 2 a (moved).", tally.appliedSentence())
}

// --env is required on every run, dry run and confirmed run alike, and
// --record goes with --confirm only.
func TestValidateFlags(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		err := validateFlags(confirm, false, "", "run.jsonl")
		require.ErrorContains(t, err, "--env is required on every run, dry run included", "confirm=%v", confirm)
	}
	require.ErrorContains(t, validateFlags(true, false, "prod.env", ""), "--confirm requires --record")
	require.ErrorContains(t, validateFlags(false, false, "prod.env", "run.jsonl"), "--record is written only by --confirm")
	require.ErrorContains(t, validateFlags(true, true, "prod.env", "run.jsonl"), "cannot be combined")
	require.NoError(t, validateFlags(false, true, "prod.env", ""))
	require.NoError(t, validateFlags(true, false, "prod.env", "run.jsonl"))
}

func TestEnvIsRequiredOnEveryRun(t *testing.T) {
	for _, envFile := range []string{"", "   "} {
		require.ErrorContains(t, requireEnvFlag(envFile), "--env is required on every run, dry run included")
	}
	require.NoError(t, requireEnvFlag("prod.env"))
}

func TestLoadNamedEnvOverridesAnExportedDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://exported.example/prod")
	file := filepath.Join(t.TempDir(), "stage.env")
	require.NoError(t, os.WriteFile(file, []byte("DATABASE_URL=postgres://named.example/stage\n"), 0o600))

	require.NoError(t, loadNamedEnv(file))

	assert.Equal(t, "postgres://named.example/stage", os.Getenv("DATABASE_URL"))
}

// A named env file that does not set DATABASE_URL is refused, and the process
// environment is left as it was.
func TestLoadNamedEnvRefusesAFileWithoutADatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://exported.example/prod")
	t.Setenv("ENVIRONMENT", "development")
	file := filepath.Join(t.TempDir(), "partial.env")
	require.NoError(t, os.WriteFile(file, []byte("ENVIRONMENT=stage\n"), 0o600))

	err := loadNamedEnv(file)

	require.ErrorContains(t, err, "sets no DATABASE_URL")
	assert.Equal(t, "postgres://exported.example/prod", os.Getenv("DATABASE_URL"))
	assert.Equal(t, "development", os.Getenv("ENVIRONMENT"), "nothing from the refused file is loaded")
}

func TestLoadNamedEnvRefusesAMissingFile(t *testing.T) {
	require.ErrorContains(t, loadNamedEnv(filepath.Join(t.TempDir(), "absent.env")), "read env file")
}

// The record file is opened for append: an existing file keeps its lines, and
// a path that cannot be opened is refused.
func TestOpenRecord_AppendsAndNeverTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("earlier line\n"), 0o600))

	f, err := openRecord(path)
	require.NoError(t, err)
	_, err = f.WriteString("new line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "earlier line\nnew line\n", string(got))

	_, err = openRecord(filepath.Join(t.TempDir(), "missing-dir", "run.jsonl"))
	require.ErrorContains(t, err, "cannot be opened for append")
}

// dedup-shows --same-local-date writes this tool's worklist plus suggested_*
// columns, and its tests keep this golden file equal to what it writes. As
// written the file is refused, so an unreviewed worklist cannot run; with each
// suggested_ column renamed by dropping the prefix, it parses unchanged.
func TestParseWorklist_AcceptsTheSameLocalDateWorklistOnceRenamed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "dedup-shows", "testdata", "same_local_date_worklist.csv"))
	require.NoError(t, err)

	_, err = parseWorklist(bytes.NewReader(raw))
	require.ErrorContains(t, err, `unknown column "suggested_merge_into_show_slug"`)

	header, body, ok := bytes.Cut(raw, []byte("\n"))
	require.True(t, ok)
	renamed := append(bytes.ReplaceAll(header, []byte("suggested_"), nil), '\n')
	rows, err := parseWorklist(bytes.NewReader(append(renamed, body...)))
	require.NoError(t, err)
	require.Len(t, rows, 4)

	assert.Equal(t, catalog.ShowReattribution{
		ShowSlug:      "2026-12-09-maris-at-the-casbah",
		RoomSlug:      "the-casbah",
		MergeIntoSlug: "2026-12-09-maris-at-casbah",
	}, rows[0].Row)
	assert.Empty(t, rows[0].Refusal)
	// Review and keep-both rows name no room and no merge target, so they are
	// refused before the database.
	for i, slug := range []string{
		"2026-12-06-the-beat-farmers-at-casbah",
		"2026-12-06-the-beat-farmers-at-the-casbah",
		"2026-08-07-plus-minus-at-space-2",
	} {
		assert.Equal(t, catalog.ShowReattribution{ShowSlug: slug}, rows[1+i].Row)
		assert.Equal(t, "no target room given", rows[1+i].Refusal)
	}
	for _, row := range rows {
		assert.NotEmpty(t, row.Note)
	}
}
