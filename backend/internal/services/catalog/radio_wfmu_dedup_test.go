package catalog

// Integration tests for the WFMU family show dedup (PSY-1073). Seeds the
// duplicated-catalog state the broken discovery produced (the same show as a
// distinct row under every family station, each with its own episode/play
// copies), runs the cleanup, and asserts per-station scoping, play-data
// preservation, FK integrity (import jobs), dry-run no-op, and idempotency.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

type WFMUDedupIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB

	// Family stations as seeded by migration 20260502023012, looked up once.
	stationsBySlug map[string]catalogm.RadioStation
}

func (s *WFMUDedupIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	// The WFMU seed migration creates the four family stations — use them
	// directly so the test validates the real slugs the command relies on.
	var stations []catalogm.RadioStation
	s.Require().NoError(s.db.Where("slug IN ?", WFMUFamilySlugs).Find(&stations).Error)
	s.Require().Len(stations, len(WFMUFamilySlugs), "WFMU seed migration should create all 4 family stations")
	s.stationsBySlug = make(map[string]catalogm.RadioStation, len(stations))
	for _, st := range stations {
		s.stationsBySlug[st.Slug] = st
	}
}

func (s *WFMUDedupIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// SetupTest wipes show-level data (keeping the migration-seeded stations) so
// every test starts from a known state. Migrations also seed a handful of
// flagship shows — those are wiped too.
func (s *WFMUDedupIntegrationTestSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// FK-safe order: jobs reference shows without cascade.
	_, _ = sqlDB.Exec("DELETE FROM radio_import_jobs")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
}

func TestWFMUDedupIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(WFMUDedupIntegrationTestSuite))
}

// =============================================================================
// Seed helpers
// =============================================================================

func (s *WFMUDedupIntegrationTestSuite) stationID(slug string) uint {
	return s.stationsBySlug[slug].ID
}

func (s *WFMUDedupIntegrationTestSuite) createShow(stationSlug, name, slug, externalID string, hostName *string) catalogm.RadioShow {
	show := catalogm.RadioShow{
		StationID:  s.stationID(stationSlug),
		Name:       name,
		Slug:       slug,
		ExternalID: &externalID,
		HostName:   hostName,
		IsActive:   true,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

func (s *WFMUDedupIntegrationTestSuite) createEpisodeWithPlays(showID uint, airDate, externalID string, playCount int) catalogm.RadioEpisode {
	ep := catalogm.RadioEpisode{
		ShowID:     showID,
		AirDate:    airDate,
		ExternalID: &externalID,
		PlayCount:  playCount,
	}
	s.Require().NoError(s.db.Create(&ep).Error)
	for i := 0; i < playCount; i++ {
		play := catalogm.RadioPlay{
			EpisodeID:  ep.ID,
			Position:   i,
			ArtistName: "Test Artist",
		}
		s.Require().NoError(s.db.Create(&play).Error)
	}
	return ep
}

// seedDuplicatedFamily recreates the PSY-1073 blast radius in miniature:
//
//	MG "Midnight in the Guest Room" — airs on Drummer; duplicated on all 4
//	  stations with overlapping + disjoint episode copies of varying richness.
//	RQ "Rock'n'Soul Radio" — the channel-stream artifact, duplicated on all 4.
//	WA "Wake" — flagship show (no ownership entry → flagship default),
//	  duplicated on all 4.
//
// Returns the per-station MG rows for FK assertions.
func (s *WFMUDedupIntegrationTestSuite) seedDuplicatedFamily() map[string]catalogm.RadioShow {
	host := "Curated Host"
	mg := map[string]catalogm.RadioShow{
		// Flagship copy is the oldest (created first) and carries curated
		// metadata the winner lacks.
		"wfmu":                s.createShow("wfmu", "Midnight in the Guest Room", "mg-wfmu", "MG", &host),
		"wfmu-drummer":        s.createShow("wfmu-drummer", "Midnight in the Guest Room", "mg-drummer", "MG", nil),
		"wfmu-rocknsoulradio": s.createShow("wfmu-rocknsoulradio", "Midnight in the Guest Room", "mg-rns", "MG", nil),
		"wfmu-sheena":         s.createShow("wfmu-sheena", "Midnight in the Guest Room", "mg-sheena", "MG", nil),
	}

	// Episode "100" exists on three stations with different richness:
	// flagship 3 plays, drummer (the future winner) 5, rocknsoul 7 (richest —
	// must survive even though its show row is a loser).
	s.createEpisodeWithPlays(mg["wfmu"].ID, "2026-01-05", "100", 3)
	s.createEpisodeWithPlays(mg["wfmu"].ID, "2026-01-12", "101", 2) // flagship-only → moves
	s.createEpisodeWithPlays(mg["wfmu-drummer"].ID, "2026-01-05", "100", 5)
	s.createEpisodeWithPlays(mg["wfmu-drummer"].ID, "2026-01-19", "102", 1)
	s.createEpisodeWithPlays(mg["wfmu-rocknsoulradio"].ID, "2026-01-05", "100", 7)
	// sheena copy has no episodes.

	// Import job pinned to a loser row (flagship copy) — must be re-pointed,
	// not orphaned (radio_import_jobs.show_id has no ON DELETE clause).
	job := catalogm.RadioImportJob{
		ShowID:    mg["wfmu"].ID,
		StationID: s.stationID("wfmu"),
		Since:     "2026-01-01",
		Until:     "2026-02-01",
		Status:    catalogm.RadioImportJobStatusCompleted,
	}
	s.Require().NoError(s.db.Create(&job).Error)

	// The channel-stream artifact, duplicated everywhere, each with its own
	// whole-stream episode (disjoint dates → all histories merge intact).
	for i, slug := range WFMUFamilySlugs {
		show := s.createShow(slug, "Rock'n'Soul Radio", "rq-"+slug, "RQ", nil)
		s.createEpisodeWithPlays(show.ID, "2026-02-0"+string(rune('1'+i)), "rq-ep-"+slug, 2)
	}

	// Flagship-default show (no ownership entry).
	for _, slug := range WFMUFamilySlugs {
		s.createShow(slug, "Wake", "wa-"+slug, "WA", nil)
	}

	return mg
}

// testOwnership mirrors what WFMUProvider.FetchShowOwnership would return
// for the seeded fixture: MG airs on Drummer, RQ is the Rock'n'Soul
// artifact, WA is absent (defaults to flagship).
func testOwnership() map[string]string {
	return map[string]string{
		"MG": "wfmu-drummer",
		"RQ": "wfmu-rocknsoulradio",
	}
}

func (s *WFMUDedupIntegrationTestSuite) showCodesByStation(slug string) []string {
	var shows []catalogm.RadioShow
	s.Require().NoError(s.db.Where("station_id = ?", s.stationID(slug)).Find(&shows).Error)
	codes := make([]string, 0, len(shows))
	for _, show := range shows {
		s.Require().NotNil(show.ExternalID)
		codes = append(codes, *show.ExternalID)
	}
	return codes
}

func (s *WFMUDedupIntegrationTestSuite) countRows(table string) int64 {
	var n int64
	s.Require().NoError(s.db.Table(table).Count(&n).Error)
	return n
}

// =============================================================================
// Tests
// =============================================================================

func (s *WFMUDedupIntegrationTestSuite) TestDryRun_ReportsPlanWithoutMutating() {
	s.seedDuplicatedFamily()

	showsBefore := s.countRows("radio_shows")
	episodesBefore := s.countRows("radio_episodes")
	playsBefore := s.countRows("radio_plays")

	result, err := DedupWFMUFamilyShows(s.db, testOwnership(), true)
	s.Require().NoError(err)

	s.True(result.DryRun)
	s.Equal(3, result.GroupsTotal)          // MG, RQ, WA
	s.Equal(3, result.GroupsWithDuplicates) // every group has 4 copies
	// The plan ran: counts reflect the full merge (3 groups x 3 losers)...
	totalDeleted := 0
	for _, c := range result.PerStation {
		totalDeleted += c.ShowsDeleted
	}
	s.Equal(9, totalDeleted)
	s.Positive(result.PerStation["wfmu-drummer"].EpisodesMovedIn)
	// ...but nothing was committed.
	s.Equal(showsBefore, s.countRows("radio_shows"))
	s.Equal(episodesBefore, s.countRows("radio_episodes"))
	s.Equal(playsBefore, s.countRows("radio_plays"))
}

func (s *WFMUDedupIntegrationTestSuite) TestConfirm_ScopesShowsToOwnerStations() {
	mg := s.seedDuplicatedFamily()

	result, err := DedupWFMUFamilyShows(s.db, testOwnership(), false)
	s.Require().NoError(err)
	s.False(result.DryRun)
	s.Equal(3, result.GroupsTotal)
	s.Equal(3, result.GroupsWithDuplicates)

	// Per-station scoping: each show exists exactly once, on its owner.
	s.ElementsMatch([]string{"WA"}, s.showCodesByStation("wfmu"))
	s.ElementsMatch([]string{"MG"}, s.showCodesByStation("wfmu-drummer"))
	s.ElementsMatch([]string{"RQ"}, s.showCodesByStation("wfmu-rocknsoulradio"))
	s.ElementsMatch([]string{}, s.showCodesByStation("wfmu-sheena"))

	// MG winner is the pre-existing drummer row (owner station).
	var winner catalogm.RadioShow
	s.Require().NoError(s.db.First(&winner, mg["wfmu-drummer"].ID).Error)
	s.Equal(s.stationID("wfmu-drummer"), winner.StationID)

	// Slug recanonicalised: the disambiguated "mg-drummer" reverts to the
	// base slug for the show name once the merge frees it.
	s.Equal("midnight-in-the-guest-room", winner.Slug)
	s.Positive(result.SlugsRecanonicalised)

	// Curated metadata adopted from the flagship loser.
	s.Require().NotNil(winner.HostName)
	s.Equal("Curated Host", *winner.HostName)

	// Episode history merged: ep 100 survives as the RICHEST copy (7 plays,
	// from the rocknsoul loser), 101 moved from flagship, 102 was already
	// the winner's.
	var episodes []catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("show_id = ?", winner.ID).Order("air_date").Find(&episodes).Error)
	s.Require().Len(episodes, 3)
	epByExt := map[string]catalogm.RadioEpisode{}
	for _, ep := range episodes {
		s.Require().NotNil(ep.ExternalID)
		epByExt[*ep.ExternalID] = ep
	}
	s.Equal(7, epByExt["100"].PlayCount, "richest duplicate copy must win")
	s.Equal(2, epByExt["101"].PlayCount)
	s.Equal(1, epByExt["102"].PlayCount)

	// Play rows follow their episodes: 7 + 2 + 1 for MG, plus 4 artifact
	// episodes x 2 plays for RQ = 18 total. The poorer ep-100 copies
	// (3 and 5 plays) cascaded away with their duplicate episodes.
	s.Equal(int64(18), s.countRows("radio_plays"))
	var ep100Plays int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", epByExt["100"].ID).Count(&ep100Plays).Error)
	s.Equal(int64(7), ep100Plays)

	// RQ artifact kept all four whole-stream episode histories.
	var rq catalogm.RadioShow
	s.Require().NoError(s.db.Where("station_id = ? AND external_id = ?", s.stationID("wfmu-rocknsoulradio"), "RQ").First(&rq).Error)
	var rqEpisodes int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", rq.ID).Count(&rqEpisodes).Error)
	s.Equal(int64(4), rqEpisodes)

	// Import job re-pointed from the deleted flagship row to the winner.
	var job catalogm.RadioImportJob
	s.Require().NoError(s.db.First(&job).Error)
	s.Equal(winner.ID, job.ShowID)
	s.Equal(s.stationID("wfmu-drummer"), job.StationID)
	s.Equal(1, result.PerStation["wfmu-drummer"].JobsReassigned)
}

func (s *WFMUDedupIntegrationTestSuite) TestConfirm_IsIdempotent() {
	s.seedDuplicatedFamily()

	_, err := DedupWFMUFamilyShows(s.db, testOwnership(), false)
	s.Require().NoError(err)

	showsAfterFirst := s.countRows("radio_shows")
	episodesAfterFirst := s.countRows("radio_episodes")
	playsAfterFirst := s.countRows("radio_plays")

	second, err := DedupWFMUFamilyShows(s.db, testOwnership(), false)
	s.Require().NoError(err)

	s.Equal(0, second.GroupsWithDuplicates, "second run must find nothing to do")
	for slug, c := range second.PerStation {
		s.Zero(c.ShowsDeleted, "station %s", slug)
		s.Zero(c.ShowsReassignedIn, "station %s", slug)
		s.Zero(c.EpisodesMovedIn, "station %s", slug)
		s.Zero(c.EpisodesDeleted, "station %s", slug)
		s.Zero(c.JobsReassigned, "station %s", slug)
	}
	s.Equal(showsAfterFirst, s.countRows("radio_shows"))
	s.Equal(episodesAfterFirst, s.countRows("radio_episodes"))
	s.Equal(playsAfterFirst, s.countRows("radio_plays"))
}

func (s *WFMUDedupIntegrationTestSuite) TestRicherEpisodeCopy_DecidedByActualPlays_NotStaleCounter() {
	// The merge keeps whichever duplicate episode copy has more REAL
	// radio_plays rows. A stale/inflated play_count column must not decide
	// which play history survives.
	winnerShow := s.createShow("wfmu-drummer", "Counter Test", "ct-drummer", "CT", nil)
	loserShow := s.createShow("wfmu", "Counter Test", "ct-wfmu", "CT", nil)

	// Winner's copy: 2 real plays, but a lying play_count of 9.
	winnerEp := s.createEpisodeWithPlays(winnerShow.ID, "2026-04-01", "300", 2)
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("id = ?", winnerEp.ID).Update("play_count", 9).Error)
	// Loser's copy: 5 real plays, but a stale play_count of 0.
	loserEp := s.createEpisodeWithPlays(loserShow.ID, "2026-04-01", "300", 5)
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("id = ?", loserEp.ID).Update("play_count", 0).Error)

	_, err := DedupWFMUFamilyShows(s.db, map[string]string{"CT": "wfmu-drummer"}, false)
	s.Require().NoError(err)

	// The loser's 5-play copy survived; the winner's 2-play copy is gone.
	var surviving []catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("show_id = ?", winnerShow.ID).Find(&surviving).Error)
	s.Require().Len(surviving, 1)
	s.Equal(loserEp.ID, surviving[0].ID)
	var plays int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", surviving[0].ID).Count(&plays).Error)
	s.Equal(int64(5), plays)
}

func (s *WFMUDedupIntegrationTestSuite) TestSingleMisplacedShow_IsReassignedNotDeleted() {
	// A show that exists ONLY on the flagship but is owned by a channel
	// (e.g. Bodega Pop airing on Give the Drummer Radio) moves station —
	// keeping its row, episodes, and slug intact.
	show := s.createShow("wfmu", "Solo Channel Show", "solo-show", "SOLO", nil)
	ep := s.createEpisodeWithPlays(show.ID, "2026-03-01", "solo-ep", 4)
	job := catalogm.RadioImportJob{
		ShowID:    show.ID,
		StationID: s.stationID("wfmu"),
		Since:     "2026-03-01",
		Until:     "2026-03-31",
		Status:    catalogm.RadioImportJobStatusCompleted,
	}
	s.Require().NoError(s.db.Create(&job).Error)

	result, err := DedupWFMUFamilyShows(s.db, map[string]string{"SOLO": "wfmu-sheena"}, false)
	s.Require().NoError(err)

	s.Equal(1, result.GroupsWithDuplicates)
	s.Equal(1, result.PerStation["wfmu-sheena"].ShowsReassignedIn)

	var reloaded catalogm.RadioShow
	s.Require().NoError(s.db.First(&reloaded, show.ID).Error)
	s.Equal(s.stationID("wfmu-sheena"), reloaded.StationID)

	// The reassigned show's own import jobs follow it to the new station.
	var reloadedJob catalogm.RadioImportJob
	s.Require().NoError(s.db.First(&reloadedJob, job.ID).Error)
	s.Equal(s.stationID("wfmu-sheena"), reloadedJob.StationID)
	s.Equal(show.ID, reloadedJob.ShowID)

	var playCount int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&playCount).Error)
	s.Equal(int64(4), playCount)
}

func (s *WFMUDedupIntegrationTestSuite) TestShowsWithoutExternalID_AreSkipped() {
	show := catalogm.RadioShow{
		StationID: s.stationID("wfmu"),
		Name:      "Hand-curated show",
		Slug:      "hand-curated",
		IsActive:  true,
	}
	s.Require().NoError(s.db.Create(&show).Error)

	result, err := DedupWFMUFamilyShows(s.db, testOwnership(), false)
	s.Require().NoError(err)

	s.Equal(1, result.ShowsWithNoExternalID)
	s.Equal(0, result.GroupsTotal)
	var still catalogm.RadioShow
	s.NoError(s.db.First(&still, show.ID).Error)
}

func (s *WFMUDedupIntegrationTestSuite) TestUnknownOwnershipSlug_KeepsExistingHome() {
	// An ownership entry pointing outside the family (corrupt map, future
	// channel not yet seeded) must not error. Pre-PSY-1349 this force-moved the
	// row to the flagship; now an unresolvable claim is treated like a
	// map-absent code — the show's unique existing substream home stands (the
	// established row is the strong claim, the resolution artifact the weak one).
	s.createShow("wfmu-sheena", "Mystery Show", "mystery-show", "MYST", nil)

	result, err := DedupWFMUFamilyShows(s.db, map[string]string{"MYST": "wfmu-not-a-station"}, false)
	s.Require().NoError(err)

	s.Equal(0, result.PerStation["wfmu"].ShowsReassignedIn, "no forced flagship reassignment")
	s.ElementsMatch([]string{"MYST"}, s.showCodesByStation("wfmu-sheena"))
}

// =============================================================================
// PSY-1349 — ownership-flap hardening
// =============================================================================

// A code ABSENT from the ownership map (dropped off every roster page) must keep
// its existing non-flagship home — forcing the flagship default onto such groups
// is the ownership flap that minted twins (the M7 / Mellow Tambourine shape). A
// map-absent code living ONLY on the flagship still defaults there (rule 4).
func (s *WFMUDedupIntegrationTestSuite) TestMapAbsentCode_KeepsExistingSubstreamHome() {
	// Oldest row on Sheena's, newer twin on the flagship, code on no roster page.
	sheena := s.createShow("wfmu-sheena", "The Mellow Tambourine", "mellow-tambourine", "M7", nil)
	flagTwin := s.createShow("wfmu", "The Mellow Tambourine", "mellow-tambourine-2", "M7", nil)
	s.createEpisodeWithPlays(sheena.ID, "2026-06-20", "m7-e1", 3)
	s.createEpisodeWithPlays(flagTwin.ID, "2026-06-27", "m7-e2", 2)

	// Flagship-only map-absent code: stays on the flagship.
	s.createShow("wfmu", "Nat Roe", "nat-roe", "NR", nil)

	result, err := DedupWFMUFamilyShows(s.db, map[string]string{}, false)
	s.Require().NoError(err)
	s.Equal(2, result.GroupsTotal)

	var survivors []catalogm.RadioShow
	s.Require().NoError(s.db.Where("external_id = ?", "M7").Find(&survivors).Error)
	s.Require().Len(survivors, 1, "twins collapse to one row")
	s.Equal(s.stationID("wfmu-sheena"), survivors[0].StationID, "map-absent code keeps its substream home, not the flagship default")
	s.Equal(sheena.ID, survivors[0].ID, "the original substream row is the winner")

	var epCount int64
	s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", sheena.ID).Count(&epCount)
	s.Equal(int64(2), epCount, "the flagship twin's episodes merged onto the home row")

	var natRoe catalogm.RadioShow
	s.Require().NoError(s.db.Where("external_id = ?", "NR").First(&natRoe).Error)
	s.Equal(s.stationID("wfmu"), natRoe.StationID, "flagship-only map-absent code stays on the flagship")
}

// PSY-1349 create-path stickiness: when a WFMU-family discover run tries to create a
// show whose code already lives under a SIBLING family station, upsertRadioShow reuses
// that row instead of minting a twin; a genuinely new code still creates normally.
func (s *WFMUDedupIntegrationTestSuite) TestUpsertRadioShow_FamilyStickiness() {
	svc := &RadioService{db: s.db}

	existing := s.createShow("wfmu-drummer", "Explorers Room", "explorers-room", "EX", nil)

	// Ownership flapped to flagship this run: the flagship discover tries to create EX.
	id, created, err := svc.upsertRadioShow(s.stationID("wfmu"), RadioShowImport{Name: "Explorers Room", ExternalID: "EX"})
	s.Require().NoError(err)
	s.False(created, "no twin minted")
	s.Equal(existing.ID, id, "episodes route to the established sibling-station row")

	var count int64
	s.db.Model(&catalogm.RadioShow{}).Where("external_id = ?", "EX").Count(&count)
	s.Equal(int64(1), count)

	// Genuinely new code: creates under the requesting station (rule 4 preserved).
	newID, created, err := svc.upsertRadioShow(s.stationID("wfmu"), RadioShowImport{Name: "Brand New Show", ExternalID: "ZZ"})
	s.Require().NoError(err)
	s.True(created)
	var fresh catalogm.RadioShow
	s.Require().NoError(s.db.First(&fresh, newID).Error)
	s.Equal(s.stationID("wfmu"), fresh.StationID)
}

// PSY-1349: a map-absent code with rows on MULTIPLE substreams is contradictory
// evidence (the legacy full-catalog-pollution shape) — it defaults to the flagship,
// not an arbitrary substream.
func (s *WFMUDedupIntegrationTestSuite) TestMapAbsentCode_MultiSubstream_DefaultsToFlagship() {
	s.createShow("wfmu-drummer", "Ambiguous Show", "ambiguous-drummer", "AM", nil)
	s.createShow("wfmu-sheena", "Ambiguous Show", "ambiguous-sheena", "AM", nil)
	s.createShow("wfmu", "Ambiguous Show", "ambiguous-wfmu", "AM", nil)

	_, err := DedupWFMUFamilyShows(s.db, map[string]string{}, false)
	s.Require().NoError(err)

	var survivors []catalogm.RadioShow
	s.Require().NoError(s.db.Where("external_id = ?", "AM").Find(&survivors).Error)
	s.Require().Len(survivors, 1)
	s.Equal(s.stationID("wfmu"), survivors[0].StationID, "contradictory substream evidence defaults to the flagship")
}

// PSY-1349 AC1, exercised through the REAL discover-run funnel: with a family twin
// present, importRosterShowEpisodes on the other station imports the episode onto the
// existing sibling row and creates nothing.
func (s *WFMUDedupIntegrationTestSuite) TestImportRosterShow_FamilyTwin_EpisodesLandOnSibling() {
	svc := &RadioService{db: s.db}
	existing := s.createShow("wfmu-drummer", "Bodega Pop", "bodega-pop", "PG", nil)

	provider := &mockPlaylistProvider{
		fetchNewEpisodesFn: func(showExternalID string, since, until time.Time) ([]RadioEpisodeImport, error) {
			return []RadioEpisodeImport{{ExternalID: "pg-ep-1", AirDate: time.Now().UTC().Format("2006-01-02")}}, nil
		},
	}

	result := &contracts.RadioImportResult{}
	created := svc.importRosterShowEpisodes(
		s.stationID("wfmu"), // ownership flapped: the FLAGSHIP run sees the show
		contracts.RadioRosterShow{Name: "Bodega Pop", ExternalID: "PG"},
		time.Now().UTC().AddDate(0, 0, -7), time.Now().UTC().AddDate(0, 0, 1),
		provider, result)

	s.False(created, "no twin row created")
	var count int64
	s.db.Model(&catalogm.RadioShow{}).Where("external_id = ?", "PG").Count(&count)
	s.Equal(int64(1), count)

	var ep catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("external_id = ?", "pg-ep-1").First(&ep).Error)
	s.Equal(existing.ID, ep.ShowID, "episode lands on the established sibling-station row")
	s.Equal(1, result.EpisodesImported, "import result attributes the episode honestly")
}
