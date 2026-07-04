package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// ──────────────────────────────────────────────
// GetRelationshipProvenance (PSY-1335)
// Methods attach to the existing integration suite in
// artist_relationship_service_test.go (same package, same runner).
// ──────────────────────────────────────────────

// createShowOn creates a show with an explicit event date, status, and
// optionally no slug (slug == "" → NULL slug, the unlinkable case).
func (suite *ArtistRelationshipServiceIntegrationTestSuite) createShowOn(title string, eventDate time.Time, status catalogm.ShowStatus, slug string) uint {
	show := &catalogm.Show{Title: title, EventDate: eventDate, Status: status}
	if slug != "" {
		show.Slug = &slug
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createStoredRel(a, b uint, relType string, score float32, detail map[string]any) {
	src, tgt := catalogm.CanonicalOrder(a, b)
	rel := &catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		Score:            score,
		AutoDerived:      true,
	}
	if detail != nil {
		raw, err := json.Marshal(detail)
		suite.Require().NoError(err)
		msg := json.RawMessage(raw)
		rel.Detail = &msg
	}
	suite.Require().NoError(suite.db.Create(rel).Error)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) assertArtistErrCode(err error, code string) {
	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.Require().ErrorAs(err, &artistErr)
	suite.Assert().Equal(code, artistErr.Code)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_ArtistNotFound() {
	a := suite.createArtist("Band A")

	_, err := suite.svc.GetRelationshipProvenance(a, 999999)
	suite.assertArtistErrCode(err, apperrors.CodeArtistNotFound)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_NoConnections() {
	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")

	_, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.assertArtistErrCode(err, apperrors.CodeArtistRelationshipNotFound)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_SelfPair() {
	a := suite.createArtist("Band A")

	_, err := suite.svc.GetRelationshipProvenance(a, a)
	suite.assertArtistErrCode(err, apperrors.CodeArtistRelationshipNotFound)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_SharedBills_NewestFirstCapAndStatus() {
	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")
	c := suite.createArtist("Band C")

	// 12 shared approved shows with distinct dates — 2 over the cap of 10.
	base := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		showID := suite.createShowOn(
			fmt.Sprintf("Show %02d", i),
			base.AddDate(0, 0, i),
			catalogm.ShowStatusApproved,
			fmt.Sprintf("prov-show-%02d-%d", i, time.Now().UnixNano()),
		)
		suite.addArtistToShow(showID, a)
		suite.addArtistToShow(showID, b)
	}
	// Pending show must not count; a show with only one of the pair must not count.
	pendingID := suite.createShowOn("Pending", base.AddDate(1, 0, 0), catalogm.ShowStatusPending, fmt.Sprintf("prov-pending-%d", time.Now().UnixNano()))
	suite.addArtistToShow(pendingID, a)
	suite.addArtistToShow(pendingID, b)
	soloID := suite.createShowOn("Solo", base.AddDate(1, 0, 1), catalogm.ShowStatusApproved, fmt.Sprintf("prov-solo-%d", time.Now().UnixNano()))
	suite.addArtistToShow(soloID, a)
	suite.addArtistToShow(soloID, c)

	suite.createStoredRel(a, b, catalogm.RelationshipTypeSharedBills, 0.8, map[string]any{"shared_count": 12})

	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)

	conn := prov.Connections[0]
	suite.Assert().Equal(catalogm.RelationshipTypeSharedBills, conn.Type)
	suite.Assert().InDelta(0.8, conn.Score, 0.0001)
	suite.Assert().NotNil(conn.Detail)
	suite.Assert().Equal(12, conn.EntityTotal)
	suite.Require().Len(conn.Entities, 10)

	// Newest first: the two oldest shows (00, 01) fall off the cap.
	suite.Assert().Equal("Show 11", conn.Entities[0].Name)
	suite.Assert().Equal("2026-01-12", conn.Entities[0].Date)
	suite.Assert().Equal("Show 02", conn.Entities[9].Name)
	for _, e := range conn.Entities {
		suite.Assert().Equal("show", e.Kind)
		suite.Assert().NotEmpty(e.Slug)
	}

	// Reversed argument order resolves the same pair.
	reversed, err := suite.svc.GetRelationshipProvenance(b, a)
	suite.Require().NoError(err)
	suite.Require().Len(reversed.Connections, 1)
	suite.Assert().Equal(conn.Entities, reversed.Connections[0].Entities)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_SharedBills_UntitledAndSluglessShows() {
	a := suite.createArtist("Alpha")
	b := suite.createArtist("Beta")

	// Untitled show → bill-name fallback (position order).
	untitledID := suite.createShowOn("", time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC), catalogm.ShowStatusApproved, fmt.Sprintf("prov-untitled-%d", time.Now().UnixNano()))
	suite.addArtistToShow(untitledID, a)
	suite.addArtistToShow(untitledID, b)

	// Slug-less show: unlinkable → excluded from BOTH the list and the total
	// (the "and N more" disclosure counts only what the list could show).
	sluglessID := suite.createShowOn("Slugless", time.Date(2026, 2, 1, 20, 0, 0, 0, time.UTC), catalogm.ShowStatusApproved, "")
	suite.addArtistToShow(sluglessID, a)
	suite.addArtistToShow(sluglessID, b)

	suite.createStoredRel(a, b, catalogm.RelationshipTypeSharedBills, 0.2, nil)

	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)

	conn := prov.Connections[0]
	suite.Assert().Equal(1, conn.EntityTotal)
	suite.Require().Len(conn.Entities, 1)
	suite.Assert().Equal("Alpha, Beta", conn.Entities[0].Name)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_SharedLabels() {
	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")

	shared1 := suite.createLabel("Fire Talk")
	shared2 := suite.createLabel("Castle Face")
	only := suite.createLabel("Solo Label")
	suite.addArtistToLabel(shared1, a)
	suite.addArtistToLabel(shared1, b)
	suite.addArtistToLabel(shared2, a)
	suite.addArtistToLabel(shared2, b)
	suite.addArtistToLabel(only, a)

	suite.createStoredRel(a, b, catalogm.RelationshipTypeSharedLabel, 0.5, map[string]any{"shared_count": 2})

	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)

	conn := prov.Connections[0]
	suite.Assert().Equal(2, conn.EntityTotal)
	suite.Require().Len(conn.Entities, 2)
	// Name order.
	suite.Assert().Equal("Castle Face", conn.Entities[0].Name)
	suite.Assert().Equal("Fire Talk", conn.Entities[1].Name)
	suite.Assert().Equal("label", conn.Entities[0].Kind)
	suite.Assert().Empty(conn.Entities[0].Date)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_FestivalCobill_NoStoredRow() {
	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")

	festID := suite.createFestival("Desert Daze", "2025-10-10", "2025-10-12", 2025)
	suite.addArtistToFestival(festID, a)
	suite.addArtistToFestival(festID, b)

	// NO stored relationship — festival co-lineup alone must resolve (not 404).
	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)

	conn := prov.Connections[0]
	suite.Assert().Equal("festival_cobill", conn.Type)
	suite.Assert().Greater(conn.Score, 0.0)
	suite.Assert().NotNil(conn.Detail)
	suite.Assert().Equal(1, conn.EntityTotal)
	suite.Require().Len(conn.Entities, 1)
	suite.Assert().Equal("festival", conn.Entities[0].Kind)
	suite.Assert().Equal("Desert Daze", conn.Entities[0].Name)
	suite.Assert().Equal("2025", conn.Entities[0].Date)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_RadioStations() {
	// SetupTest doesn't truncate radio tables — clean up ONLY this test's
	// chains (seed migrations provide real stations other suites may use).
	defer func() {
		sqlDB, _ := suite.db.DB()
		_, _ = sqlDB.Exec(`DELETE FROM radio_plays WHERE episode_id IN (
			SELECT e.id FROM radio_episodes e
			JOIN radio_shows rs ON rs.id = e.show_id
			WHERE rs.slug LIKE 'prov-station-%')`)
		_, _ = sqlDB.Exec(`DELETE FROM radio_episodes WHERE show_id IN (
			SELECT id FROM radio_shows WHERE slug LIKE 'prov-station-%')`)
		_, _ = sqlDB.Exec("DELETE FROM radio_shows WHERE slug LIKE 'prov-station-%'")
		_, _ = sqlDB.Exec("DELETE FROM radio_stations WHERE slug LIKE 'prov-station-%'")
	}()

	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")

	nano := time.Now().UnixNano()
	makeStationChain := func(name, slug string) uint {
		station := &catalogm.RadioStation{Name: name, Slug: slug}
		suite.Require().NoError(suite.db.Create(station).Error)
		show := &catalogm.RadioShow{StationID: station.ID, Name: name + " Show", Slug: slug + "-show"}
		suite.Require().NoError(suite.db.Create(show).Error)
		episode := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-05-01"}
		suite.Require().NoError(suite.db.Create(episode).Error)
		return episode.ID
	}
	addPlay := func(episodeID, artistID uint, name string) {
		play := &catalogm.RadioPlay{EpisodeID: episodeID, ArtistName: name, ArtistID: &artistID}
		suite.Require().NoError(suite.db.Create(play).Error)
	}

	// Both artists on the same episode at two stations; a third station plays
	// only artist A (must not appear). Names carry the nano suffix — seed
	// migrations already provide stations (KEXP/WFMU/NTS) and reusing a real
	// name trips a uniqueness constraint.
	sharedName1 := fmt.Sprintf("Prov Station A %d", nano)
	sharedName2 := fmt.Sprintf("Prov Station B %d", nano)
	soloName := fmt.Sprintf("Prov Station C %d", nano)
	ep1 := makeStationChain(sharedName1, fmt.Sprintf("prov-station-a-%d", nano))
	ep2 := makeStationChain(sharedName2, fmt.Sprintf("prov-station-b-%d", nano))
	ep3 := makeStationChain(soloName, fmt.Sprintf("prov-station-c-%d", nano))
	addPlay(ep1, a, "Band A")
	addPlay(ep1, b, "Band B")
	addPlay(ep2, a, "Band A")
	addPlay(ep2, b, "Band B")
	addPlay(ep3, a, "Band A")

	suite.createStoredRel(a, b, catalogm.RelationshipTypeRadioCooccurrence, 0.4,
		map[string]any{"co_occurrence_count": 2, "station_count": 2})

	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)

	conn := prov.Connections[0]
	suite.Assert().Equal(catalogm.RelationshipTypeRadioCooccurrence, conn.Type)
	suite.Assert().Equal(2, conn.EntityTotal)
	suite.Require().Len(conn.Entities, 2)
	suite.Assert().Equal(sharedName1, conn.Entities[0].Name)
	suite.Assert().Equal(sharedName2, conn.Entities[1].Name)
	suite.Assert().Equal("station", conn.Entities[0].Kind)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestProvenance_EntityLessTypes() {
	a := suite.createArtist("Band A")
	b := suite.createArtist("Band B")

	suite.createStoredRel(a, b, catalogm.RelationshipTypeSimilar, 0.9, nil)

	prov, err := suite.svc.GetRelationshipProvenance(a, b)
	suite.Require().NoError(err)
	suite.Require().Len(prov.Connections, 1)
	suite.Assert().Equal(catalogm.RelationshipTypeSimilar, prov.Connections[0].Type)
	suite.Assert().Empty(prov.Connections[0].Entities)
	suite.Assert().Zero(prov.Connections[0].EntityTotal)
}
