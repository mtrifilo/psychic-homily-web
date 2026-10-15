package main

import (
	"bytes"
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

func TestSampleRemovals(t *testing.T) {
	changes := func(outcome catalog.RadioReleaseLinkOutcome, firstID, n int) []catalog.RadioReleaseLinkChange {
		out := make([]catalog.RadioReleaseLinkChange, n)
		for i := range out {
			out[i] = catalog.RadioReleaseLinkChange{PlayID: uint(firstID + i), Outcome: outcome}
		}
		return out
	}
	count := func(sample []catalog.RadioReleaseLinkChange, outcome catalog.RadioReleaseLinkOutcome) int {
		n := 0
		for _, c := range sample {
			if c.Outcome == outcome {
				n++
			}
		}
		return n
	}
	notCredited := catalog.ReleaseLinkRemovedNotCredited
	unlinked := catalog.ReleaseLinkRemovedUnlinkedArtist

	tests := []struct {
		name                    string
		notCredited, unlinked   int
		wantNotCred, wantUnlink int
	}{
		{"both groups large", 400, 5000, 25, 25},
		{"small first group cedes its share", 10, 5000, 10, 40},
		{"small second group cedes its share", 400, 7, 43, 7},
		{"fewer than n in total", 3, 4, 3, 4},
		{"only one group", 0, 80, 0, 50},
		{"none", 0, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removals := append(changes(notCredited, 1, tt.notCredited), changes(unlinked, 100000, tt.unlinked)...)
			sample := sampleRemovals(removals, 50)
			assert.Equal(t, tt.wantNotCred, count(sample, notCredited))
			assert.Equal(t, tt.wantUnlink, count(sample, unlinked))
			seen := make(map[uint]bool)
			for _, c := range sample {
				assert.False(t, seen[c.PlayID], "play %d sampled twice", c.PlayID)
				seen[c.PlayID] = true
			}
			if tt.notCredited > 0 {
				assert.Equal(t, uint(1), sample[0].PlayID, "the first play of a group is included")
			}
		})
	}
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

	recorder := testutil.NewSQLRecorder()
	var out bytes.Buffer
	require.NoError(t, run(db.Session(&gorm.Session{Logger: recorder}), false, 0, false, &out))
	assert.Contains(t, out.String(), "DRY RUN: no DB writes.")
	assert.Contains(t, out.String(), "Sample of removals (1 of 1)")
	assert.Contains(t, out.String(), `album "Chrome": release `)
	assert.Contains(t, out.String(), "credited to Nice Nice")
	assert.Equal(t, &chrome, releaseOf(removed))
	assert.Equal(t, &borisFlood, releaseOf(relinked))
	require.NotEmpty(t, recorder.Statements())
	for _, stmt := range recorder.Statements() {
		assert.True(t, strings.HasPrefix(strings.ToUpper(stmt), "SELECT"), "dry run issued a non-SELECT statement: %s", stmt)
	}

	out.Reset()
	require.NoError(t, run(db, true, 0, true, &out))
	assert.Contains(t, out.String(), "LIVE: release links cleared 1, relinked 1.")
	assert.Contains(t, out.String(), "Every removal and relink (2)")
	assert.Nil(t, releaseOf(removed))
	assert.Equal(t, &tmbgFlood, releaseOf(relinked))
}
