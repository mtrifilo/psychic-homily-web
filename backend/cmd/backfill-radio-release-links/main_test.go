package main

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

func TestRedactDatabaseURL(t *testing.T) {
	const notShown = "set (not shown)"
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", "NOT SET"},
		{"userinfo and query dropped", "postgresql://user:secret@db.example.com:5432/railway?sslmode=require", "postgresql://***@db.example.com:5432/railway"},
		{"password with @", "postgres://user:se@cret@db.example.com:5432/db", "postgres://***@db.example.com:5432/db"},
		{"password with /", "postgres://user:1234/abc@db.example.com:5432/db", "postgres://***@db.example.com:5432/db"},
		{"no userinfo", "postgres://localhost:5432/db", "postgres://localhost:5432/db"},
		{"password in the query", "postgres://db.example.com/db?password=secret", "postgres://db.example.com/db"},
		{"fragment dropped", "postgres://u:p@db.example.com/db#secret", "postgres://***@db.example.com/db"},
		{"no scheme", "user:secret@db.example.com/db", notShown},
		{"? before the last @", "postgres://u:p?x@db.example.com/db", notShown},
		{"# before the last @", "postgres://u:p#x@db.example.com/db", notShown},
		{"space in the host part", "postgres://u:p@db example.com/db", notShown},
		{"empty host part", "postgres://u:p@", notShown},
		{"bad scheme character", "post gres://u:p@db.example.com/db", notShown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactDatabaseURL(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "secret")
		})
	}
}

func TestSampleByGroup(t *testing.T) {
	changes := func(firstID, n int) []catalog.RadioReleaseLinkChange {
		out := make([]catalog.RadioReleaseLinkChange, n)
		for i := range out {
			out[i] = catalog.RadioReleaseLinkChange{PlayID: uint(firstID + i)}
		}
		return out
	}
	tests := []struct {
		name  string
		sizes []int
		want  []int
	}{
		{"two large groups", []int{400, 5000}, []int{25, 25}},
		{"small first group cedes its share", []int{10, 5000}, []int{10, 40}},
		{"small second group cedes its share", []int{400, 7}, []int{43, 7}},
		{"three groups, one tiny", []int{400, 2, 5000}, []int{24, 2, 24}},
		{"fewer than n in total", []int{3, 4}, []int{3, 4}},
		{"only one group", []int{0, 80}, []int{0, 50}},
		{"none", []int{0, 0}, []int{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := make([][]catalog.RadioReleaseLinkChange, len(tt.sizes))
			for i, size := range tt.sizes {
				groups[i] = changes((i+1)*100000, size)
			}
			sample := sampleByGroup(groups, 50)
			got := make([]int, len(groups))
			seen := make(map[uint]bool)
			for _, c := range sample {
				got[int(c.PlayID)/100000-1]++
				assert.False(t, seen[c.PlayID], "play %d sampled twice", c.PlayID)
				seen[c.PlayID] = true
			}
			assert.Equal(t, tt.want, got)
			if len(sample) > 0 {
				assert.Equal(t, groups[firstNonEmpty(groups)][0].PlayID, sample[0].PlayID, "a group's first play is included")
			}
		})
	}
}

func firstNonEmpty(groups [][]catalog.RadioReleaseLinkChange) int {
	for i, g := range groups {
		if len(g) > 0 {
			return i
		}
	}
	return -1
}

func TestRun_DryRunThenConfirm(t *testing.T) {
	testDB := testutil.SetupTestPostgres(t)
	defer testDB.Cleanup()
	db := testDB.DB

	tySegall := &catalogm.Artist{Name: "Ty Segall"}
	niceNice := &catalogm.Artist{Name: "Nice Nice"}
	boris := &catalogm.Artist{Name: "Boris"}
	tmbg := &catalogm.Artist{Name: "They Might Be Giants"}
	for _, a := range []*catalogm.Artist{tySegall, niceNice, boris, tmbg} {
		require.NoError(t, db.Create(a).Error)
	}
	release := func(title string, artistID uint) uint {
		r := &catalogm.Release{Title: title}
		require.NoError(t, db.Create(r).Error)
		require.NoError(t, db.Create(&catalogm.ArtistRelease{ArtistID: artistID, ReleaseID: r.ID, Role: catalogm.ArtistReleaseRoleMain}).Error)
		return r.ID
	}
	chrome := release("Chrome", niceNice.ID)
	borisFlood := release("Flood", boris.ID)
	tmbgFlood := release("Flood", tmbg.ID)

	station := &catalogm.RadioStation{Name: "Test Station", Slug: "test-station", BroadcastType: catalogm.BroadcastTypeInternet}
	require.NoError(t, db.Create(station).Error)
	show := &catalogm.RadioShow{StationID: station.ID, Name: "Test Show", Slug: "test-show"}
	require.NoError(t, db.Create(show).Error)
	episode := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-05-28"}
	require.NoError(t, db.Create(episode).Error)
	play := func(position int, artist *catalogm.Artist, album string, releaseID uint) uint {
		p := &catalogm.RadioPlay{
			EpisodeID: episode.ID, Position: position, ArtistName: artist.Name, AlbumTitle: &album,
			ArtistID: &artist.ID, ReleaseID: &releaseID, MatchState: catalogm.RadioPlayMatchStateMatched,
		}
		require.NoError(t, db.Create(p).Error)
		return p.ID
	}
	removed := play(1, tySegall, "Chrome", chrome)
	relinked := play(2, tmbg, "Flood", borisFlood)
	releaseOf := func(playID uint) *uint {
		var p catalogm.RadioPlay
		require.NoError(t, db.First(&p, playID).Error)
		return p.ReleaseID
	}

	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.csv")
	recorder := testutil.NewSQLRecorder()
	var out bytes.Buffer
	require.NoError(t, run(db.Session(&gorm.Session{Logger: recorder}), runOptions{outPath: planPath}, &out))
	assert.Contains(t, out.String(), "DRY RUN: no DB writes. Planned changes are listed in "+planPath)
	assert.Contains(t, out.String(), "Sample of removals (1 of 1)")
	assert.Contains(t, out.String(), "Sample of relinks (1 of 1)")
	assert.Contains(t, out.String(), `artist "Ty Segall" (linked: "Ty Segall" #`)
	assert.Contains(t, out.String(), `album "Chrome": release `)
	assert.Contains(t, out.String(), "credited to Nice Nice")
	assert.Equal(t, &chrome, releaseOf(removed))
	assert.Equal(t, &borisFlood, releaseOf(relinked))
	require.NotEmpty(t, recorder.Statements())
	for _, stmt := range recorder.Statements() {
		assert.True(t, strings.HasPrefix(strings.ToUpper(stmt), "SELECT"), "dry run issued a non-SELECT statement: %s", stmt)
	}
	assert.Equal(t, mustReadChangeList(t, planPath), [][]string{
		{"play_id", "outcome", "artist_id", "from_release_id", "to_release_id", "admin_set"},
		{fmt.Sprint(removed), "removed_not_credited", fmt.Sprint(tySegall.ID), fmt.Sprint(chrome), "", "false"},
		{fmt.Sprint(relinked), "relinked_credited", fmt.Sprint(tmbg.ID), fmt.Sprint(borisFlood), fmt.Sprint(tmbgFlood), "false"},
	})

	out.Reset()
	err := run(db, runOptions{confirm: true, outPath: planPath}, &out)
	require.Error(t, err, "a live run never overwrites an existing change list")
	assert.Equal(t, &chrome, releaseOf(removed), "a refused change list leaves every play unchanged")
	assert.Equal(t, &borisFlood, releaseOf(relinked))

	stalePlanPath := filepath.Join(dir, "stale-plan.csv")
	require.NoError(t, writeChangeList(stalePlanPath, mustReadChangeList(t, planPath)[:2]))
	rejectedPath := filepath.Join(dir, "rejected.csv")
	err = run(db, runOptions{confirm: true, outPath: rejectedPath, planPath: stalePlanPath}, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differs from the reviewed plan")
	assert.Contains(t, err.Error(), "0 planned rows absent, 1 rows not in the plan")
	assert.NoFileExists(t, rejectedPath, "a plan mismatch aborts before the change list is written")
	assert.Equal(t, &chrome, releaseOf(removed))

	const failUpdates = "test:fail_radio_plays_update"
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(failUpdates, func(tx *gorm.DB) {
		if strings.HasPrefix(strings.TrimSpace(tx.Statement.SQL.String()), "UPDATE radio_plays") {
			_ = tx.AddError(errors.New("simulated update failure"))
		}
	}))
	failedPath := filepath.Join(dir, "failed.csv")
	err = run(db, runOptions{confirm: true, outPath: failedPath, planPath: planPath}, &out)
	require.NoError(t, db.Callback().Raw().Remove(failUpdates))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none of the changes listed in "+failedPath+".NOT-APPLIED were applied")
	assert.NoFileExists(t, failedPath)
	assert.FileExists(t, failedPath+".NOT-APPLIED")
	assert.Equal(t, &chrome, releaseOf(removed), "a failed live run rolls back")

	changesPath := filepath.Join(dir, "changes.csv")
	out.Reset()
	require.NoError(t, run(db, runOptions{confirm: true, verbose: true, outPath: changesPath, planPath: planPath}, &out))
	assert.Contains(t, out.String(), "LIVE: release links cleared 1, relinked 1. Every change is listed in "+changesPath)
	assert.Contains(t, out.String(), "Every removal and relink (2)")
	assert.Nil(t, releaseOf(removed))
	assert.Equal(t, &tmbgFlood, releaseOf(relinked))
	assert.Equal(t, mustReadChangeList(t, planPath), mustReadChangeList(t, changesPath), "the live run applies the reviewed plan")
}

func mustReadChangeList(t *testing.T, path string) [][]string {
	t.Helper()
	rows, err := readChangeList(path)
	require.NoError(t, err)
	return rows
}

func TestFormatReport_CountsAdminLinksAndEverySource(t *testing.T) {
	report := &catalog.RadioReleaseLinkReport{
		Scanned: 6,
		Counts:  map[catalog.RadioReleaseLinkOutcome]int{catalog.RadioReleaseLinkKeptAdmin: 2, catalog.RadioReleaseLinkKeptCredited: 4},
		SourceCounts: map[string]int{
			catalogm.RadioPlayReleaseLinkSourceMatcher:  3,
			catalogm.RadioPlayReleaseLinkSourceAdmin:    2,
			catalogm.RadioPlayReleaseLinkSourceBackfill: 0,
			"": 1,
		},
	}
	out := formatReport(report, runOptions{})
	for label, count := range map[string]int{
		"Kept: an admin chose the release (never judged):": 2,
		"matcher:":   3,
		"admin:":     2,
		"backfill:":  0,
		"no source:": 1,
	} {
		assert.Regexp(t, fmt.Sprintf(`(?m)^  %s +%d$`, regexp.QuoteMeta(label), count), out)
	}
}

func TestDiffChangeLists(t *testing.T) {
	header := changeListHeader
	a := []string{"1", "removed_not_credited", "7", "10", "", "false"}
	b := []string{"2", "relinked_credited", "7", "11", "12", "false"}
	assert.Empty(t, diffChangeLists([][]string{header, a, b}, [][]string{header, b, a}), "row order does not matter")
	assert.Equal(t, "1 planned rows absent, 0 rows not in the plan", diffChangeLists([][]string{header, a, b}, [][]string{header, a}))
	changed := []string{"2", "relinked_credited", "7", "11", "13", "false"}
	assert.Equal(t, "1 planned rows absent, 1 rows not in the plan", diffChangeLists([][]string{header, a, b}, [][]string{header, a, changed}))
}

func TestMarkNotAppliedNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changes.csv")
	require.NoError(t, writeChangeList(path, [][]string{changeListHeader}))
	runErr := errors.New("update failed")

	err := markNotApplied(path, runErr)
	require.ErrorIs(t, err, runErr)
	assert.FileExists(t, path+".NOT-APPLIED")
	assert.NoFileExists(t, path)

	require.NoError(t, writeChangeList(path, [][]string{changeListHeader}))
	err = markNotApplied(path, runErr)
	require.ErrorIs(t, err, runErr)
	assert.Contains(t, err.Error(), "not renamed")
	assert.FileExists(t, path, "an existing .NOT-APPLIED file is never replaced")
}

func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    runOptions
		wantErr string
	}{
		{"dry run", runOptions{}, ""},
		{"dry run with --out", runOptions{outPath: "plan.csv"}, ""},
		{"live run with --out", runOptions{confirm: true, outPath: "changes.csv"}, ""},
		{"live run with --out and --plan", runOptions{confirm: true, outPath: "changes.csv", planPath: "plan.csv"}, ""},
		{"live run without --out", runOptions{confirm: true}, "--confirm requires --out"},
		{"--plan on a dry run", runOptions{planPath: "plan.csv"}, "--plan applies only with --confirm"},
		{"negative --limit", runOptions{limit: -1}, "--limit must be 0 (all) or positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.opts)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestReadChangeListRejects(t *testing.T) {
	dir := t.TempDir()
	_, err := readChangeList(filepath.Join(dir, "missing.csv"))
	require.Error(t, err)

	other := filepath.Join(dir, "other.csv")
	require.NoError(t, writeChangeList(other, [][]string{{"id", "name"}, {"1", "x"}}))
	_, err = readChangeList(other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a change list written by this command")

	empty := filepath.Join(dir, "empty.csv")
	require.NoError(t, writeChangeList(empty, nil))
	_, err = readChangeList(empty)
	require.Error(t, err)
}
