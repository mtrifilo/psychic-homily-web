package catalog

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS — normalizeName (no DB)
// =============================================================================

// TestNormalizeName covers the Go-side normalization pipeline used by the
// radio matching engine. The DB side mirrors the diacritic/lowercase parts
// via the `unaccent` expression indexes (PSY-886).
func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Empty / trivial.
		{"empty string", "", ""},
		{"whitespace only", "   \t\n", ""},
		{"already canonical", "the who", "the who"},

		// Diacritic folding (NFKD + Mn strip).
		{"acute accent", "José", "jose"},
		{"two diacritics", "José González", "jose gonzalez"},
		{"umlaut", "Mötley Crüe", "motley crue"},
		{"tilde", "Señor Coconut", "senor coconut"},
		{"cedilla", "Françoise Hardy", "francoise hardy"},
		{"circumflex", "Beyoncé", "beyonce"},

		// Compatibility decomposition via NFKD (full-width → ASCII).
		{"fullwidth latin", "ＡＢＣ", "abc"},

		// Lowercase.
		{"uppercase ascii", "THE WHO", "the who"},
		{"mixed case", "PiNk FlOyD", "pink floyd"},

		// Boundary trim (non-letter/digit at start/end, interior kept).
		{"trailing exclamation", "The Who!", "the who"},
		{"leading hash", "#trending", "trending"},
		{"surrounding parens", "(The Beatles)", "the beatles"},
		{"trailing period", "Inc.", "inc"},
		{"surrounding quotes", `"Joy Division"`, "joy division"},

		// Whitespace collapsing (interior runs flatten to one ASCII space).
		{"double internal space", "the  who", "the who"},
		{"triple internal space", "the   who", "the who"},
		{"internal tab", "the\twho", "the who"},
		{"internal newline", "the\nwho", "the who"},
		{"mixed whitespace runs", "the \t \n who", "the who"},
		{"NBSP collapses to space", "the who", "the who"},

		// Interior punctuation PRESERVED (false-positive guards).
		{"AC/DC slash kept", "AC/DC", "ac/dc"},
		{"hyphenated name", "Twenty One Pilots", "twenty one pilots"},
		{"interior ampersand", "Earth, Wind & Fire", "earth, wind & fire"},
		{"interior period", "P.I.L.", "p.i.l"}, // trailing period trimmed, interiors kept

		// Numbers preserved.
		{"digits preserved", "Blink 182", "blink 182"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeName(tt.in)
			if got != tt.want {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeName_FalsePositiveGuards verifies that distinct names with
// only interior-punctuation or whitespace differences do NOT collide after
// normalization. These are the cases that motivated keeping interior
// punctuation rather than stripping all non-alphanumerics.
func TestNormalizeName_FalsePositiveGuards(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{"AC/DC vs ACDC", "AC/DC", "ACDC"},
		{"The The vs The", "The The", "The"},
		{"Earth Wind Fire vs EarthWindFire", "Earth Wind & Fire", "EarthWindFire"},
		{"P.I.L. vs PIL", "P.I.L.", "PIL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			na := normalizeName(tt.a)
			nb := normalizeName(tt.b)
			if na == nb {
				t.Errorf("normalizeName(%q) == normalizeName(%q) == %q — would collide", tt.a, tt.b, na)
			}
		})
	}
}

// TestNormalizeName_EmptyAfterNormalize covers the contract that all-punctuation
// inputs (e.g. "!!!" the band) collapse to "" — and the matcher guards against
// that by short-circuiting on empty so it never issues a `LIKE ''` lookup that
// would match every empty-string column row.
func TestNormalizeName_EmptyAfterNormalize(t *testing.T) {
	cases := []string{"!!!", "...", "---", "   ", "@@@"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if got := normalizeName(in); got != "" {
				t.Errorf("normalizeName(%q) = %q, want empty", in, got)
			}
		})
	}
}

// TestNormalizeName_PositiveMatches verifies that diacritic / case /
// punctuation / whitespace variants of the same name collapse to one form.
func TestNormalizeName_PositiveMatches(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{"diacritic vs ascii", "José González", "Jose Gonzalez"},
		{"case difference", "THE WHO", "the who"},
		{"trailing punctuation", "The Who!", "The Who"},
		{"leading punctuation", "#Trending", "Trending"},
		{"whitespace noise", "The   Who", "The Who"},
		{"tab vs space", "Pink\tFloyd", "Pink Floyd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			na := normalizeName(tt.a)
			nb := normalizeName(tt.b)
			if na != nb || na == "" {
				t.Errorf("normalizeName(%q)=%q != normalizeName(%q)=%q — should collide", tt.a, na, tt.b, nb)
			}
		})
	}
}

// TestNormalizeName_ConcurrentSafe drives normalizeName from many goroutines
// at once. transform.Chain has mutable per-instance state, so an earlier
// implementation that shared one chain instance across goroutines was a
// data race (run with `go test -race` to verify the regression guard).
// Each goroutine asserts its own deterministic input/output mapping; a race
// would either trip the race detector or produce a mismatched output.
func TestNormalizeName_ConcurrentSafe(t *testing.T) {
	cases := []struct{ in, want string }{
		{"José González", "jose gonzalez"},
		{"Mötley Crüe", "motley crue"},
		{"The Who!", "the who"},
		{"AC/DC", "ac/dc"},
		{"Beyoncé", "beyonce"},
		{"  Stereolab  ", "stereolab"},
	}

	const goroutines = 50
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				tc := cases[i%len(cases)]
				if got := normalizeName(tc.in); got != tc.want {
					t.Errorf("normalizeName(%q) = %q, want %q", tc.in, got, tc.want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// Note: the nil-DB error path is covered by TestRadioMatchingEngine_NilDB in
// radio_provider_test.go; no duplicate here.

// =============================================================================
// INTEGRATION TESTS (real Postgres via testcontainers)
// =============================================================================

type RadioMatchingIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	engine *RadioMatchingEngine
}

func (suite *RadioMatchingIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.engine = NewRadioMatchingEngine(suite.db)
}

func (suite *RadioMatchingIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *RadioMatchingIntegrationTestSuite) SetupTest() {
	suite.cleanup()
}

func (suite *RadioMatchingIntegrationTestSuite) TearDownTest() {
	suite.cleanup()
}

func (suite *RadioMatchingIntegrationTestSuite) cleanup() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// FK-safe order. radio_plays references episodes/shows/stations.
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM radio_networks")
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
}

func TestRadioMatchingIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(RadioMatchingIntegrationTestSuite))
}

func (suite *RadioMatchingIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	slug := utils.GenerateArtistSlug(name)
	a := &catalogm.Artist{Name: name, Slug: &slug}
	suite.Require().NoError(suite.db.Create(a).Error)
	return a
}

func (suite *RadioMatchingIntegrationTestSuite) createAlias(artistID uint, alias string) {
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistAlias{
		ArtistID: artistID, Alias: alias,
	}).Error)
}

func (suite *RadioMatchingIntegrationTestSuite) createRelease(title string) *catalogm.Release {
	r := &catalogm.Release{Title: title}
	suite.Require().NoError(suite.db.Create(r).Error)
	return r
}

func (suite *RadioMatchingIntegrationTestSuite) createLabel(name string) *catalogm.Label {
	l := &catalogm.Label{Name: name}
	suite.Require().NoError(suite.db.Create(l).Error)
	return l
}

// createStationShowEpisode wires up the minimal radio fixture chain so we can
// insert plays. Each test reuses one fixture chain.
func (suite *RadioMatchingIntegrationTestSuite) createStationShowEpisode() uint {
	station := &catalogm.RadioStation{
		Name:          "Test Station",
		Slug:          "test-station",
		BroadcastType: catalogm.BroadcastTypeInternet,
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	show := &catalogm.RadioShow{
		StationID: station.ID,
		Name:      "Test Show",
		Slug:      "test-show",
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	episode := &catalogm.RadioEpisode{
		ShowID:  show.ID,
		AirDate: "2026-05-28",
	}
	suite.Require().NoError(suite.db.Create(episode).Error)
	return episode.ID
}

// createPlay inserts a play and returns its ID.
func (suite *RadioMatchingIntegrationTestSuite) createPlay(episodeID uint, position int, artistName string, albumTitle, labelName *string) *catalogm.RadioPlay {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
		AlbumTitle: albumTitle,
		LabelName:  labelName,
	}
	suite.Require().NoError(suite.db.Create(play).Error)
	return play
}

// TestMatchArtist_DiacriticInsensitive verifies the headline use case from
// PSY-886: a radio play arriving as "José González" matches an artist stored
// as either "José González" or "Jose Gonzalez", and vice versa.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_DiacriticInsensitive() {
	jose := suite.createArtist("Jose Gonzalez") // stored without diacritics

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "José González", nil, nil)

	matched := suite.engine.matchPlay(play)
	suite.True(matched, "diacritic input should match diacritic-free stored name")

	// Reload to verify the persisted artist_id.
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(jose.ID, *reloaded.ArtistID)
}

// TestMatchArtist_StoredWithDiacritic — reverse direction: dirty index, clean
// input. Validates the expression index applies unaccent() to the column too.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_StoredWithDiacritic() {
	beyonce := suite.createArtist("Beyoncé") // stored WITH diacritic

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "Beyonce", nil, nil)

	matched := suite.engine.matchPlay(play)
	suite.True(matched, "ascii input should match diacritic-stored name via unaccent index")

	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(beyonce.ID, *reloaded.ArtistID)
}

// TestMatchArtist_CaseInsensitive — sanity that case-only differences still
// match (regression guard for the old LOWER(...) behavior).
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_CaseInsensitive() {
	a := suite.createArtist("The Who")

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "THE WHO", nil, nil)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(a.ID, *reloaded.ArtistID)
}

// TestMatchArtist_PunctuationTrim — radio feed appends an "!", DB has the
// clean name. Normalizer trims the trailing punctuation so they match.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_PunctuationTrim() {
	a := suite.createArtist("The Who")

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "The Who!", nil, nil)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(a.ID, *reloaded.ArtistID)
}

// TestMatchArtist_AliasFolding — same diacritic-insensitive fold but resolved
// via an alias row instead of the canonical name.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_AliasFolding() {
	a := suite.createArtist("Sigur Ros")
	suite.createAlias(a.ID, "Sigur Rós") // alias stored with diacritic

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "Sigur Ros", nil, nil)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(a.ID, *reloaded.ArtistID)
}

// TestMatchArtist_FalsePositiveGuards — interior-punctuation differences
// must NOT collide.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchArtist_FalsePositiveGuards() {
	suite.createArtist("AC/DC")
	suite.createArtist("The The")

	episodeID := suite.createStationShowEpisode()
	playACDC := suite.createPlay(episodeID, 1, "ACDC", nil, nil)
	playThe := suite.createPlay(episodeID, 2, "The", nil, nil)

	suite.False(suite.engine.matchPlay(playACDC), `"ACDC" must NOT match "AC/DC"`)
	suite.False(suite.engine.matchPlay(playThe), `"The" must NOT match "The The"`)

	// Both plays should remain unmatched in the DB.
	var reloadedACDC catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloadedACDC, playACDC.ID).Error)
	suite.Nil(reloadedACDC.ArtistID)

	var reloadedThe catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloadedThe, playThe.ID).Error)
	suite.Nil(reloadedThe.ArtistID)
}

// TestMatchRelease_DiacriticInsensitive — release-side parity.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchRelease_DiacriticInsensitive() {
	a := suite.createArtist("Jose Gonzalez")
	r := suite.createRelease("Veneer") // boring ascii title for the join side

	episodeID := suite.createStationShowEpisode()
	title := "Veneer"
	play := suite.createPlay(episodeID, 1, "José González", &title, nil)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(a.ID, *reloaded.ArtistID)
	suite.Require().NotNil(reloaded.ReleaseID)
	suite.Equal(r.ID, *reloaded.ReleaseID)
}

// TestMatchRelease_WithDiacritic — release stored with diacritic, input
// without; the unaccent expression index resolves the match.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchRelease_WithDiacritic() {
	// Artist seeded so matchPlay reports artist-matched true; we assert on
	// the release-side resolution here.
	suite.createArtist("Stereolab")
	r := suite.createRelease("Cobra et Phasès") // diacritic stored

	episodeID := suite.createStationShowEpisode()
	title := "Cobra et Phases"
	play := suite.createPlay(episodeID, 1, "Stereolab", &title, nil)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ReleaseID)
	suite.Equal(r.ID, *reloaded.ReleaseID)
}

// TestMatchLabel_DiacriticInsensitive — label-side parity.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchLabel_DiacriticInsensitive() {
	a := suite.createArtist("Test Artist")
	l := suite.createLabel("Cafe Tacuba Records") // stored ascii

	episodeID := suite.createStationShowEpisode()
	label := "Café Tacuba Records" // input with diacritic
	play := suite.createPlay(episodeID, 1, "Test Artist", nil, &label)

	suite.True(suite.engine.matchPlay(play))
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(a.ID, *reloaded.ArtistID)
	suite.Require().NotNil(reloaded.LabelID)
	suite.Equal(l.ID, *reloaded.LabelID)
}

// TestMatchAllUnmatched_BeforeAfterCounts is the AC-required curated
// international-name integration probe. It seeds a corpus that exercises the
// diacritic / case / punctuation cases the old LOWER-only matcher missed,
// runs the matcher, and asserts the post-change match rate against the
// hand-counted expectation. The before/after delta is the manual repro
// artifact referenced in the PR body.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchAllUnmatched_BeforeAfterCounts() {
	// Stored entities (no diacritics in DB to simulate clean curated data).
	suite.createArtist("Jose Gonzalez")
	suite.createArtist("Beyonce")
	suite.createArtist("Motley Crue")
	suite.createArtist("Sigur Ros")
	suite.createArtist("The Who")
	suite.createArtist("Stereolab")
	suite.createArtist("Cafe Tacuba")
	suite.createArtist("Bjork")
	// False-positive guards present in the corpus.
	suite.createArtist("AC/DC")
	suite.createArtist("The The")

	episodeID := suite.createStationShowEpisode()

	// 8 plays that previously failed under LOWER-only matching.
	previouslyFailing := []string{
		"José González",
		"Beyoncé",
		"Mötley Crüe",
		"Sigur Rós",
		"The Who!",        // punctuation trim
		"  Stereolab  ",   // whitespace trim
		"Café Tacuba",     // diacritic
		"björk",           // case + diacritic
	}
	// 2 plays that should remain unmatched (false-positive guards).
	mustNotMatch := []string{
		"ACDC",  // distinct from AC/DC
		"The",   // distinct from The The
	}

	pos := 1
	for _, name := range previouslyFailing {
		suite.createPlay(episodeID, pos, name, nil, nil)
		pos++
	}
	for _, name := range mustNotMatch {
		suite.createPlay(episodeID, pos, name, nil, nil)
		pos++
	}

	// Before PSY-886, LOWER(name)=LOWER(?) would have matched 0 of the 8
	// previouslyFailing entries (all carry a diacritic, trailing
	// punctuation, or excess whitespace that survives a plain LOWER fold).
	// The mustNotMatch entries would also have been 0 — they are distinct
	// from the stored names regardless of pipeline. So the pre-change
	// match count for this corpus was 0/10.
	const beforeMatchCount = 0
	const totalPlays = 10
	const expectedAfterMatchCount = 8 // previouslyFailing all resolve; mustNotMatch stay unmatched

	result, err := suite.engine.MatchAllUnmatched()
	suite.Require().NoError(err)

	suite.Equalf(totalPlays, result.Total, "Total plays loaded by matcher")
	suite.Equalf(
		expectedAfterMatchCount, result.Matched,
		"PSY-886 match rate on curated international corpus: before=%d/%d, after=%d/%d",
		beforeMatchCount, totalPlays, result.Matched, totalPlays,
	)
	suite.Equalf(totalPlays-expectedAfterMatchCount, result.Unmatched, "Unmatched count")
}

// TestMatchPlaysForEpisode_PersistFailureSurfaced is the PSY-1119 match-persist
// regression. PSY-814 already stopped over-counting a play whose match update
// failed as "matched" (it's reported unmatched instead), but that failure lived
// only in logs — MatchResult could not tell a genuine no-match from a
// computed-match-that-couldn't-be-saved. This test injects an Update failure on
// radio_plays via a GORM callback (the same idiom as the PSY-814 affinity-sync
// failure test) and asserts the failure now appears in MatchResult.PersistErrors
// while the play is NOT counted as matched and stays unmatched on disk.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchPlaysForEpisode_PersistFailureSurfaced() {
	// Seed an artist that the play will match by exact name — so the matcher
	// computes a match and reaches the persist step.
	artist := suite.createArtist("Radiohead")
	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "Radiohead", nil, nil)

	// Inject a failure scoped to radio_plays UPDATEs only, so the matcher's
	// SELECTs (artist lookup, unmatched-plays load) still succeed and the
	// failure lands exactly on the match-persist write.
	const cbName = "test:fail_radio_plays_update"
	suite.Require().NoError(suite.db.Callback().Update().Before("gorm:update").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "radio_plays" {
			_ = tx.AddError(fmt.Errorf("simulated radio_plays update failure"))
		}
	}))
	defer func() {
		suite.Require().NoError(suite.db.Callback().Update().Remove(cbName))
	}()

	result, err := suite.engine.MatchPlaysForEpisode(episodeID)
	suite.Require().NoError(err, "a per-play persist failure must not abort the whole episode match")
	suite.Equal(1, result.Total)
	suite.Equal(0, result.Matched, "an unpersisted match must not be counted as matched")
	suite.Equal(1, result.Unmatched)
	suite.Equal(1, result.PersistErrors, "the persist failure must surface in the result, not just logs")

	// The play must remain unmatched on disk — the artist_id never persisted.
	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.First(&reloaded, play.ID).Error)
	suite.Nil(reloaded.ArtistID, "artist_id must be unset since the update failed")
	_ = artist
}

func (suite *RadioMatchingIntegrationTestSuite) TestMatchUnmatchedPlaysForArtistName_TargetsNameOnly() {
	episodeID := suite.createStationShowEpisode()
	suite.createPlay(episodeID, 1, "Metric", nil, nil)
	suite.createPlay(episodeID, 2, "Pixel Grip", nil, nil)
	suite.createArtist("Metric")

	result, err := suite.engine.MatchUnmatchedPlaysForArtistName("Metric")
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(1, result.Matched)

	var metricPlay catalogm.RadioPlay
	suite.Require().NoError(suite.db.Where("artist_name = ?", "Metric").First(&metricPlay).Error)
	suite.Require().NotNil(metricPlay.ArtistID)

	var pixelPlay catalogm.RadioPlay
	suite.Require().NoError(suite.db.Where("artist_name = ?", "Pixel Grip").First(&pixelPlay).Error)
	suite.Nil(pixelPlay.ArtistID)
}

func (suite *RadioMatchingIntegrationTestSuite) TestMatchUnmatchedPlaysForArtistName_ResolvesAlias() {
	artist := suite.createArtist("World's Worst")
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistAlias{
		ArtistID: artist.ID,
		Alias:    "Worlds Worst",
	}).Error)

	episodeID := suite.createStationShowEpisode()
	suite.createPlay(episodeID, 1, "Worlds Worst", nil, nil)

	result, err := suite.engine.MatchUnmatchedPlaysForArtistName("Worlds Worst")
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(1, result.Matched)

	var reloaded catalogm.RadioPlay
	suite.Require().NoError(suite.db.Where("artist_name = ?", "Worlds Worst").First(&reloaded).Error)
	suite.Require().NotNil(reloaded.ArtistID)
	suite.Equal(artist.ID, *reloaded.ArtistID)
}
