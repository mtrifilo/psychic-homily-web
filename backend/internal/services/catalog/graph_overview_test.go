package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (Require Database)
// =============================================================================
//
// The layout subprocess is STUBBED here. The Node side has its own test suite
// (backend/layout/fa2-layout.test.mjs) covering determinism, the `fixed`
// attribute, and the stdin/stdout contract; what these tests own is everything
// the Go side is responsible for — scope order, the hub handoff, warm-start
// replay, Procrustes alignment, the append-only publish, and the failure
// posture. Stubbing the physics is what lets "unchanged data reproduces
// byte-identical positions" be asserted exactly rather than within a tolerance.

// stubLayoutRunner stands in for the Node subprocess.
//
// The default behaviour is the IDENTITY transform: it returns the seed
// positions it was handed. That makes the Go pipeline's own determinism
// observable — any position difference between two runs over unchanged data is
// then necessarily a Go-side ordering bug, not layout noise.
type stubLayoutRunner struct {
	// transform, when set, maps the seeds to the "laid out" positions.
	transform func([]layoutPoint) []layoutPoint
	// err, when set, fails the run.
	err error

	calls    int
	lastReq  layoutRequest
	lastSeed []layoutPoint
}

func (r *stubLayoutRunner) Run(_ context.Context, req layoutRequest) ([]layoutPoint, error) {
	r.calls++
	r.lastReq = req
	if r.err != nil {
		return nil, r.err
	}
	seeds := make([]layoutPoint, len(req.Nodes))
	for i, n := range req.Nodes {
		seeds[i] = layoutPoint(n)
	}
	r.lastSeed = seeds
	if r.transform != nil {
		return r.transform(seeds), nil
	}
	return seeds, nil
}

// rotateScaleTranslate is the drift ForceAtlas2 is free to introduce between two
// otherwise-equal runs: the layout has no preferred origin, orientation or
// scale. Procrustes alignment exists to remove exactly this.
func rotateScaleTranslate(points []layoutPoint) []layoutPoint {
	const angle = math.Pi / 3
	const scale = 1.7
	out := make([]layoutPoint, len(points))
	for i, p := range points {
		x, y := float64(p.X), float64(p.Y)
		out[i] = layoutPoint{
			X: float32(scale*(math.Cos(angle)*x-math.Sin(angle)*y) + 250),
			Y: float32(scale*(math.Sin(angle)*x+math.Cos(angle)*y) - 90),
		}
	}
	return out
}

type GraphOverviewSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RadioService
}

func TestGraphOverviewSuite(t *testing.T) {
	suite.Run(t, new(GraphOverviewSuite))
}

func (s *GraphOverviewSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &RadioService{db: s.db}
}

func (s *GraphOverviewSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *GraphOverviewSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, table := range []string{
		"graph_overview_snapshots",
		"artist_communities",
		"artist_relationships",
		"artist_labels",
		"show_artists",
		"shows",
		"labels",
		"artists",
	} {
		_, _ = sqlDB.Exec("DELETE FROM " + table)
	}
}

// --- fixtures ---

func (s *GraphOverviewSuite) createArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-go", name)
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

func (s *GraphOverviewSuite) relate(aID, bID uint, typ string, score float32) {
	lo, hi := min(aID, bID), max(aID, bID)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelationship{
		SourceArtistID:   lo,
		TargetArtistID:   hi,
		RelationshipType: typ,
		Score:            score,
		AutoDerived:      true,
	}).Error)
}

func (s *GraphOverviewSuite) createLabelWithRoster(name string, artistIDs ...uint) *catalogm.Label {
	slug := name + "-go"
	label := &catalogm.Label{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(label).Error)
	for _, id := range artistIDs {
		s.Require().NoError(s.db.Create(&catalogm.ArtistLabel{ArtistID: id, LabelID: label.ID}).Error)
	}
	return label
}

func (s *GraphOverviewSuite) createShow(title string, when time.Time, artistIDs ...uint) {
	slug := fmt.Sprintf("%s-go", title)
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: when,
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)
	for i, id := range artistIDs {
		s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
			ShowID:   show.ID,
			ArtistID: id,
			Position: i,
		}).Error)
	}
}

// seedScene builds a small but structurally complete catalog.
//
// THE SHAPE IS DICTATED BY THE DISPARITY FILTER, which is a far more aggressive
// sparsifier than it looks: at the default alpha an edge survives only when it
// is a large fraction of some endpoint's strength. A uniform-weight chain loses
// almost every edge (a degree-2 node's two equal edges each score 0.5, five
// times the 0.10 threshold), so a chain fixture would leave nothing to assert
// against. Two stars joined by a heavy bridge is the smallest shape where the
// whole graph survives honestly: every leaf is degree 1 (kept by the filter's
// niche-link convention) and the bridge is 10/13 of each centre's strength.
//
// On top of that: a three-artist label roster that qualifies as a hub, a 2019
// show so appearTime has an event clock to prefer over a row stamp, and two
// artists with no relationships at all so the isolate count has something to
// count. Returns the eight on-map artists, centres at index 0 and 4.
func (s *GraphOverviewSuite) seedScene() []*catalogm.Artist {
	artists := make([]*catalogm.Artist, 0, 8)
	for i := 1; i <= 8; i++ {
		artists = append(artists, s.createArtist(fmt.Sprintf("Band%d", i)))
	}
	for _, leaf := range []int{1, 2, 3} {
		s.relate(artists[0].ID, artists[leaf].ID, catalogm.RelationshipTypeSharedBills, 1)
	}
	for _, leaf := range []int{5, 6, 7} {
		s.relate(artists[4].ID, artists[leaf].ID, catalogm.RelationshipTypeSharedBills, 1)
	}
	// The bridge between the two stars. Heavy on purpose: at weight 1 it is a
	// quarter of each centre's strength and the filter cuts it, splitting the
	// map into two components.
	s.relate(artists[0].ID, artists[4].ID, catalogm.RelationshipTypeSharedBills, 10)

	// Three leaves of the first star share a label, so it qualifies as a hub.
	s.createLabelWithRoster("HubRecords", artists[1].ID, artists[2].ID, artists[3].ID)

	s.createShow("OldShow", time.Date(2019, 3, 14, 0, 0, 0, 0, time.UTC), artists[1].ID)

	// Isolates: in the catalog, off the map.
	s.createArtist("Loner1")
	s.createArtist("Loner2")

	_, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	return artists
}

func (s *GraphOverviewSuite) build(runner graphLayoutRunner, at time.Time) (GraphOverviewComputeResult, error) {
	return s.svc.computeGraphOverviewSnapshot(context.Background(), runner, func() time.Time { return at })
}

func (s *GraphOverviewSuite) newestSnapshot() catalogm.GraphOverviewSnapshot {
	var row catalogm.GraphOverviewSnapshot
	s.Require().NoError(s.db.Table("graph_overview_snapshots").Order("id DESC").Limit(1).Scan(&row).Error)
	return row
}

func (s *GraphOverviewSuite) newestPayload() contracts.GraphOverview {
	row := s.newestSnapshot()
	s.Require().NotEmpty(row.Payload, "expected a published snapshot")
	var payload contracts.GraphOverview
	s.Require().NoError(json.Unmarshal(row.Payload, &payload))
	return payload
}

func (s *GraphOverviewSuite) snapshotCount() int64 {
	var n int64
	s.Require().NoError(s.db.Table("graph_overview_snapshots").Count(&n).Error)
	return n
}

// --- acceptance: the published map has every part the ticket lists ---

func (s *GraphOverviewSuite) TestBuild_PublishesACompleteMap() {
	artists := s.seedScene()
	builtAt := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC)

	result, err := s.build(&stubLayoutRunner{}, builtAt)
	s.Require().NoError(err)

	payload := s.newestPayload()

	s.Assert().Equal(contracts.GraphOverviewVersion, payload.Version)
	s.Assert().Equal(builtAt, payload.LastMapped.UTC(), "last_mapped is the build clock")
	s.Assert().Equal(graphOverviewEpoch, payload.Epoch.UTC())

	// Positions: one per node, in the quantized coordinate space.
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.X))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Y))
	for i := range payload.Nodes.X {
		s.Require().LessOrEqual(int(payload.Nodes.X[i]), contracts.GraphOverviewCoordinateScale)
		s.Require().GreaterOrEqual(int(payload.Nodes.X[i]), -contracts.GraphOverviewCoordinateScale)
	}
	// Every columnar array is the same length — the payload's whole contract.
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.ID))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Kind))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Name))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Slug))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Community))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Degree))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Rank))
	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Appear))

	// Hubs and spokes: the three-artist roster collapsed into one hub node.
	hubIdx := -1
	for i, kind := range payload.Nodes.Kind {
		if kind == contracts.GraphOverviewNodeLabelHub {
			hubIdx = i
		}
	}
	s.Require().NotEqual(-1, hubIdx, "expected a label hub node")
	s.Assert().Equal("HubRecords", payload.Nodes.Name[hubIdx])
	s.Assert().Equal(int32(-1), payload.Nodes.Community[hubIdx], "a hub belongs to no community")
	spokes := 0
	for _, kind := range payload.Edges.Kind {
		if kind == contracts.GraphOverviewEdgeLabelSpoke {
			spokes++
		}
	}
	// Every CSR slot appears twice (both directions), so three spokes is six.
	s.Assert().Equal(6, spokes, "expected three membership spokes in both directions")

	// CSR: offsets are NodeCount+1 and the slot arrays agree.
	s.Require().Equal(payload.NodeCount+1, len(payload.Edges.Offsets))
	s.Require().Equal(2*payload.EdgeCount, len(payload.Edges.Targets))
	s.Require().Equal(2*payload.EdgeCount, len(payload.Edges.Kind))
	s.Require().Equal(2*payload.EdgeCount, len(payload.Edges.Appear))
	s.Assert().Equal(int32(0), payload.Edges.Offsets[0])
	s.Assert().Equal(int32(2*payload.EdgeCount), payload.Edges.Offsets[payload.NodeCount])
	// Degree must equal the CSR neighbourhood width, or hover is wrong.
	for i := 0; i < payload.NodeCount; i++ {
		width := payload.Edges.Offsets[i+1] - payload.Edges.Offsets[i]
		s.Require().Equal(payload.Nodes.Degree[i], width, "node %d degree disagrees with its CSR span", i)
	}

	// Regions: labelled, with a hull.
	s.Require().NotEmpty(payload.Regions, "expected at least one labelled region")
	for _, region := range payload.Regions {
		s.Assert().Contains(region.Label, "Around ")
		s.Assert().Positive(region.MemberCount)
	}

	// appearTime: the 2019 show, not the row's created_at.
	playedIn2019 := -1
	for i, id := range payload.Nodes.ID {
		if id == artists[1].ID && payload.Nodes.Kind[i] == contracts.GraphOverviewNodeArtist {
			playedIn2019 = i
		}
	}
	s.Require().NotEqual(-1, playedIn2019)
	wantAppear := appearSeconds(time.Date(2019, 3, 14, 0, 0, 0, 0, time.UTC))
	s.Assert().Equal(wantAppear, payload.Nodes.Appear[playedIn2019],
		"appearTime must come from the show date, not the artist row's created_at")
	// The hub inherits the earliest appearance on its roster, so it cannot be
	// younger than the artist that dragged the timeline back to 2019.
	s.Assert().Equal(wantAppear, payload.Nodes.Appear[hubIdx],
		"a hub cannot predate or postdate its earliest member")

	// Isolates: the two unconnected artists are counted, not drawn.
	s.Assert().Equal(2, payload.IsolateCount)
	for _, id := range payload.Nodes.ID {
		s.Require().NotEqual(uint(0), id)
	}

	s.Assert().Equal(payload.NodeCount, result.Nodes)
	s.Assert().Equal(payload.EdgeCount, result.Edges)
	s.Assert().Positive(result.PayloadBytes)

	row := s.newestSnapshot()
	s.Assert().NotEmpty(row.ContentHash)
	s.Assert().NotEmpty(row.Layout, "the warm-start layout must be persisted for the next epoch")
	s.Assert().Equal(payload.IsolateCount, row.IsolateCount)
}

// --- acceptance: the stability contract ---

func (s *GraphOverviewSuite) TestBuild_UnchangedStructureSkipsTheLayoutEntirely() {
	// THE HEADLINE CONTRACT. An unchanged graph must reproduce byte-identical
	// positions — and the only way that is true of a real ForceAtlas2 run is to
	// not make one. A layout that relaxes for another 50 iterations every night
	// drifts every dot every night, which is the reshuffle this whole design
	// exists to prevent. The assertion that the runner was never called is the
	// one that has teeth: it holds no matter what the physics would have done.
	s.seedScene()

	first := &stubLayoutRunner{}
	_, err := s.build(first, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Require().Equal(1, first.calls, "the first build has nothing to reuse")
	before := s.newestPayload()

	// A rebuild with a runner that would SCRAMBLE the map if it were consulted.
	scrambler := &stubLayoutRunner{transform: rotateScaleTranslate}
	_, err = s.build(scrambler, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	after := s.newestPayload()

	s.Assert().Equal(0, scrambler.calls, "an unchanged structure must not run the layout at all")
	s.Require().Equal(before.NodeCount, after.NodeCount)
	s.Assert().Equal(before.Nodes.ID, after.Nodes.ID, "node order must be reproducible")
	s.Assert().Equal(before.Nodes.X, after.Nodes.X, "positions must be byte-identical on unchanged data")
	s.Assert().Equal(before.Nodes.Y, after.Nodes.Y, "positions must be byte-identical on unchanged data")
	s.Assert().Equal(before.Nodes.Rank, after.Nodes.Rank)
	s.Assert().Equal(before.Edges.Targets, after.Edges.Targets)
	s.Assert().Equal(before.Regions, after.Regions)
}

func (s *GraphOverviewSuite) TestBuild_ChangedAttributesAloneStillSkipTheLayout() {
	// Renaming a band or moving a label changes the payload but must not move a
	// single dot: the structure key covers the node and edge sets only.
	artists := s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	before := s.newestPayload()

	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", artists[0].ID).
		Update("name", "Renamed Band").Error)
	s.Require().NoError(s.db.Model(&catalogm.Label{}).
		Where("name = ?", "HubRecords").
		Update("city", "Austin").Error)

	runner := &stubLayoutRunner{transform: rotateScaleTranslate}
	_, err = s.build(runner, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	after := s.newestPayload()

	s.Assert().Equal(0, runner.calls, "an attribute-only change must not re-run the layout")
	s.Assert().Equal(before.Nodes.X, after.Nodes.X)
	s.Assert().Equal(before.Nodes.Y, after.Nodes.Y)
	s.Assert().Contains(after.Nodes.Name, "Renamed Band", "the rename still reached the payload")
	s.Assert().Contains(after.Nodes.HubCity, "Austin", "the city edit still reached the payload")
}

func (s *GraphOverviewSuite) TestBuild_WarmStartResumesFromThePreviousSnapshot() {
	artists := s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	var stored map[string][2]float32
	s.Require().NoError(json.Unmarshal(s.newestSnapshot().Layout, &stored))
	before := s.newestPayload()

	// Change the STRUCTURE so the layout actually runs: a new leaf on the second
	// star. The returning nodes must still be handed their stored positions
	// rather than a fresh spiral, and the run must use the warm budget.
	newcomer := s.createArtist("WarmNewcomer")
	s.relate(artists[4].ID, newcomer.ID, catalogm.RelationshipTypeSharedBills, 1)
	_, err = s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)

	warm := &stubLayoutRunner{}
	_, err = s.build(warm, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	s.Require().Equal(1, warm.calls, "a structural change must run the layout")
	s.Assert().Equal(graphOverviewWarmIterations, warm.lastReq.Iterations,
		"a warm run must use the relaxation budget, not the cold one")

	after := s.newestPayload()
	seedByID := make(map[uint]layoutPoint, len(after.Nodes.ID))
	for i, id := range after.Nodes.ID {
		seedByID[id] = warm.lastSeed[i]
	}
	for _, id := range before.Nodes.ID {
		p, ok := stored[fmt.Sprintf("%d", id)]
		s.Require().True(ok, "node %d missing from the persisted layout", id)
		seed := seedByID[id]
		s.Require().Equal(p[0], seed.X, "node %d was not warm-started from its stored x", id)
		s.Require().Equal(p[1], seed.Y, "node %d was not warm-started from its stored y", id)
	}
}

func (s *GraphOverviewSuite) TestBuild_RefusesToWarmStartAcrossAPayloadVersion() {
	s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	// Pretend the live snapshot was written by a build with a different payload
	// schema. Node ids are the ONLY correspondence between epochs, so resuming
	// across a version that may have re-scoped them would silently place nodes
	// at another entity's coordinates.
	s.Require().NoError(s.db.Exec(
		"UPDATE graph_overview_snapshots SET payload = jsonb_set(payload, '{version}', '99'::jsonb)").Error)

	next := &stubLayoutRunner{}
	_, err = s.build(next, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	s.Assert().Equal(graphOverviewColdIterations, next.lastReq.Iterations,
		"a version mismatch must cold-start, not warm-start")
}

func (s *GraphOverviewSuite) TestBuild_ColdStartUsesTheFullIterationBudget() {
	s.seedScene()

	cold := &stubLayoutRunner{}
	_, err := s.build(cold, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	s.Assert().Equal(graphOverviewColdIterations, cold.lastReq.Iterations)
}

func (s *GraphOverviewSuite) TestBuild_ProcrustesUndoesRotationScaleAndTranslation() {
	s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	before := s.newestPayload()

	// The layout comes back rotated, rescaled and translated — the same map in a
	// different frame. Alignment must put every returning node back.
	_, err = s.build(&stubLayoutRunner{transform: rotateScaleTranslate}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	after := s.newestPayload()

	s.Require().Equal(before.NodeCount, after.NodeCount)
	for i := range before.Nodes.ID {
		s.Require().Equal(before.Nodes.ID[i], after.Nodes.ID[i])
		// One quantization step of slack: the alignment is solved in float64 and
		// applied to float32 coordinates.
		s.Assert().InDelta(before.Nodes.X[i], after.Nodes.X[i], 2,
			"node %d drifted on x after alignment", before.Nodes.ID[i])
		s.Assert().InDelta(before.Nodes.Y[i], after.Nodes.Y[i], 2,
			"node %d drifted on y after alignment", before.Nodes.ID[i])
	}
}

func (s *GraphOverviewSuite) TestBuild_GrowthKeepsExistingNodesInPlace() {
	artists := s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	before := s.newestPayload()
	beforeAt := make(map[uint][2]int16, before.NodeCount)
	for i, id := range before.Nodes.ID {
		beforeAt[id] = [2]int16{before.Nodes.X[i], before.Nodes.Y[i]}
	}

	// Growth: a new artist joins the second star as another leaf.
	newcomer := s.createArtist("Newcomer")
	s.relate(artists[4].ID, newcomer.ID, catalogm.RelationshipTypeSharedBills, 1)
	_, err = s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)

	_, err = s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	after := s.newestPayload()

	s.Assert().Equal(before.NodeCount+1, after.NodeCount, "the newcomer joined the map")

	// Every returning node keeps its coordinates. With the identity runner the
	// warm start replays them exactly, so the map does not reshuffle around the
	// new arrival — the property the whole stability contract exists for.
	returning := 0
	for i, id := range after.Nodes.ID {
		want, ok := beforeAt[id]
		if !ok {
			continue
		}
		returning++
		s.Assert().InDelta(want[0], after.Nodes.X[i], 2, "returning node %d moved on x", id)
		s.Assert().InDelta(want[1], after.Nodes.Y[i], 2, "returning node %d moved on y", id)
	}
	s.Assert().Equal(before.NodeCount, returning, "every previous node should have returned")

	// And the newcomer landed on its only neighbour rather than in a corner: a
	// single-neighbour barycenter IS that neighbour's position, plus the
	// deterministic sub-pixel offset that keeps two nodes off the same point.
	var newcomerPos, neighborPos [2]int16
	for i, id := range after.Nodes.ID {
		if after.Nodes.Kind[i] != contracts.GraphOverviewNodeArtist {
			continue
		}
		if id == newcomer.ID {
			newcomerPos = [2]int16{after.Nodes.X[i], after.Nodes.Y[i]}
		}
		if id == artists[4].ID {
			neighborPos = [2]int16{after.Nodes.X[i], after.Nodes.Y[i]}
		}
	}
	s.Assert().InDelta(neighborPos[0], newcomerPos[0], 2, "a new node seeds at its placed neighbour's barycenter")
	s.Assert().InDelta(neighborPos[1], newcomerPos[1], 2, "a new node seeds at its placed neighbour's barycenter")
}

func (s *GraphOverviewSuite) TestBuild_FlagsMarkPlayableAudioAndUpcomingShows() {
	artists := s.seedScene()

	// artists[1] already has the 2019 show (past). Give it something to play,
	// and give artists[5] a show in the future.
	embed := "https://bandcamp.com/EmbeddedPlayer/album=1"
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", artists[1].ID).
		Update("bandcamp_embed_url", embed).Error)
	s.createShow("FutureShow", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), artists[5].ID)

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	payload := s.newestPayload()

	s.Require().Equal(payload.NodeCount, len(payload.Nodes.Flags))
	flagOf := func(target uint) uint8 {
		for i, id := range payload.Nodes.ID {
			if id == target && payload.Nodes.Kind[i] == contracts.GraphOverviewNodeArtist {
				return payload.Nodes.Flags[i]
			}
		}
		s.Require().Fail("artist not on the map", "id %d", target)
		return 0
	}

	s.Assert().NotZero(flagOf(artists[1].ID)&contracts.GraphOverviewFlagPlayableAudio,
		"the artist with a bandcamp embed must be marked playable")
	s.Assert().Zero(flagOf(artists[1].ID)&contracts.GraphOverviewFlagUpcomingShow,
		"a 2019 show is not an upcoming show")
	s.Assert().NotZero(flagOf(artists[5].ID)&contracts.GraphOverviewFlagUpcomingShow,
		"the artist with a future show must be marked upcoming")
	s.Assert().Zero(flagOf(artists[3].ID),
		"an artist with neither signal carries no flags")

	// The hub unions its roster's flags — artists[1] is on HubRecords.
	for i, kind := range payload.Nodes.Kind {
		if kind == contracts.GraphOverviewNodeLabelHub {
			s.Assert().NotZero(payload.Nodes.Flags[i]&contracts.GraphOverviewFlagPlayableAudio,
				"a hub inherits playability from its roster")
		}
	}
}

// --- acceptance: scope order (the PSY-1722 contract) ---

func (s *GraphOverviewSuite) TestBuild_HubScopeIsTheDrawnNodeSet() {
	artists := s.seedScene()

	// An artist on the label roster but NOT on the map: it has no relationship
	// at all, so it never survives to the node set. The hub must be built on the
	// drawn set, so this artist gets no spoke — and the roster it is counted in
	// must not include it.
	offMap := s.createArtist("OffMap")
	s.Require().NoError(s.db.Create(&catalogm.ArtistLabel{
		ArtistID: offMap.ID,
		LabelID:  s.labelIDFor("HubRecords"),
	}).Error)

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	payload := s.newestPayload()

	for i, id := range payload.Nodes.ID {
		if payload.Nodes.Kind[i] == contracts.GraphOverviewNodeArtist {
			s.Require().NotEqual(offMap.ID, id, "an artist with no surviving edge must not be drawn")
		}
	}
	spokes := 0
	for _, kind := range payload.Edges.Kind {
		if kind == contracts.GraphOverviewEdgeLabelSpoke {
			spokes++
		}
	}
	s.Assert().Equal(6, spokes, "the off-map roster member must not get a spoke")

	// And the three on-map roster artists all did get one.
	hubIdx := -1
	for i, kind := range payload.Nodes.Kind {
		if kind == contracts.GraphOverviewNodeLabelHub {
			hubIdx = i
		}
	}
	s.Require().NotEqual(-1, hubIdx)
	neighbors := payload.Edges.Targets[payload.Edges.Offsets[hubIdx]:payload.Edges.Offsets[hubIdx+1]]
	onMap := map[uint]bool{artists[1].ID: false, artists[2].ID: false, artists[3].ID: false}
	for _, n := range neighbors {
		if _, ok := onMap[payload.Nodes.ID[n]]; ok {
			onMap[payload.Nodes.ID[n]] = true
		}
	}
	for id, connected := range onMap {
		s.Assert().True(connected, "roster artist %d has no spoke to its hub", id)
	}
}

func (s *GraphOverviewSuite) labelIDFor(name string) uint {
	var label catalogm.Label
	s.Require().NoError(s.db.Where("name = ?", name).First(&label).Error)
	return label.ID
}

func (s *GraphOverviewSuite) TestBuild_HubCarriesItsLabelsHomeLocation() {
	artists := s.seedScene()

	// ONE FULLY-LOCATED HUB AND ONE KNOWN ONLY BY COUNTRY, in the same payload.
	// "Only the parts that are set" is a claim about the difference between
	// them, and a fixture carrying only the positive case would pass against a
	// hardcoded string. The country-only hub is the PSY-1792 case specifically:
	// before the state/country columns shipped it captioned nothing on the map
	// while `/scenes` captioned it "England".
	s.Require().NoError(s.db.Model(&catalogm.Label{}).
		Where("name = ?", "HubRecords").
		Updates(map[string]any{"city": " Austin ", "state": " TX ", "country": " USA "}).Error)
	s.createLabelWithRoster("CountryOnlyRecords", artists[5].ID, artists[6].ID, artists[7].ID)
	s.Require().NoError(s.db.Model(&catalogm.Label{}).
		Where("name = ?", "CountryOnlyRecords").
		Update("country", "England").Error)

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	payload := s.newestPayload()

	for name, column := range map[string][]string{
		"hub_city":    payload.Nodes.HubCity,
		"hub_state":   payload.Nodes.HubState,
		"hub_country": payload.Nodes.HubCountry,
	} {
		s.Require().Len(column, payload.NodeCount,
			"%s must be a full-length column, like every other node column", name)
	}

	// hubLocation is the package-level type from graph_overview_unit_test.go.
	locationByHub := make(map[string]hubLocation)
	for i, kind := range payload.Nodes.Kind {
		if kind == contracts.GraphOverviewNodeLabelHub {
			locationByHub[payload.Nodes.Name[i]] = hubLocation{
				city:    payload.Nodes.HubCity[i],
				state:   payload.Nodes.HubState[i],
				country: payload.Nodes.HubCountry[i],
			}
			continue
		}
		s.Assert().Empty(payload.Nodes.HubCity[i],
			"artist %q must carry no hub city: the columns are the hub caption, not location columns",
			payload.Nodes.Name[i])
		s.Assert().Empty(payload.Nodes.HubState[i],
			"artist %q must carry no hub state", payload.Nodes.Name[i])
		s.Assert().Empty(payload.Nodes.HubCountry[i],
			"artist %q must carry no hub country", payload.Nodes.Name[i])
	}
	// Both hubs must EXIST before their locations mean anything. Without this
	// the negative case fails open: a missing "CountryOnlyRecords" hub reads
	// back as the zero value, which is exactly the "" the assertions want.
	s.Require().Contains(locationByHub, "HubRecords")
	s.Require().Contains(locationByHub, "CountryOnlyRecords")

	s.Assert().Equal(hubLocation{city: "Austin", state: "TX", country: "USA"},
		locationByHub["HubRecords"], "every located part reaches the payload, trimmed")
	s.Assert().Equal(hubLocation{country: "England"},
		locationByHub["CountryOnlyRecords"],
		"a label known only by country carries that country and nothing else")
}

// --- acceptance: failure posture ---

func (s *GraphOverviewSuite) TestBuild_PreviousSnapshotSurvivesAFailedRun() {
	artists := s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	good := s.newestSnapshot()
	s.Require().NotEmpty(good.Payload)

	// The structure has to change, or the next build reuses the stored layout
	// and never reaches the runner that is supposed to blow up.
	newcomer := s.createArtist("DoomedNewcomer")
	s.relate(artists[4].ID, newcomer.ID, catalogm.RelationshipTypeSharedBills, 1)

	_, err = s.build(&stubLayoutRunner{err: fmt.Errorf("node exploded")}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().Error(err)

	after := s.newestSnapshot()
	s.Assert().Equal(good.ID, after.ID, "the previous snapshot must still be the newest row")
	s.Assert().Equal(good.ContentHash, after.ContentHash)
	s.Assert().Equal(int64(1), s.snapshotCount(), "a failed run must not write a row")
}

func (s *GraphOverviewSuite) TestBuild_EmptyEdgeSetAbortsRatherThanPublishingAGuttedMap() {
	s.createArtist("Alone")

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "keeping previous snapshot")
	s.Assert().Equal(int64(0), s.snapshotCount())
}

func (s *GraphOverviewSuite) TestBuild_EveryDrawnNodeIsNamedAndLinkable() {
	// A node the client cannot label or open is chrome. artist_relationships
	// carries a foreign key to artists, so a dangling endpoint cannot be
	// inserted here — filterBackboneToArtists' defence against one is covered by
	// its unit test; this asserts the property that defence exists to protect.
	s.seedScene()

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	payload := s.newestPayload()
	for i, id := range payload.Nodes.ID {
		s.Require().NotZero(id, "node %d has no id", i)
		s.Require().NotEmpty(payload.Nodes.Name[i], "node %d has no name", id)
		s.Require().NotEmpty(payload.Nodes.Slug[i], "node %d has no slug", id)
	}
}

func (s *GraphOverviewSuite) TestBuild_SlugLessArtistIsNotDrawn() {
	// artists.slug is nullable. A node with no slug cannot be opened, and the
	// contract promises the client it never has to check — so it must be gone
	// before the payload, and before the hub builder sees the node set.
	artists := s.seedScene()
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", artists[3].ID).
		Update("slug", nil).Error)

	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	payload := s.newestPayload()
	for i, id := range payload.Nodes.ID {
		s.Require().NotEqual(artists[3].ID, id, "the slug-less artist must not be drawn")
		s.Require().NotEmpty(payload.Nodes.Slug[i], "node %d has no slug", id)
	}
	// Its label roster drops to two in-set artists, which is below the hub
	// threshold — so the hub goes with it rather than keeping a stale spoke.
	for _, kind := range payload.Nodes.Kind {
		s.Assert().NotEqual(contracts.GraphOverviewNodeLabelHub, kind,
			"a roster that fell under the threshold must not keep its hub")
	}
}

func (s *GraphOverviewSuite) TestBuild_RetainsOnlyTheConfiguredSnapshotHistory() {
	s.seedScene()

	for i := 0; i < graphOverviewRetainedSnapshots+2; i++ {
		_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, i, time.UTC))
		s.Require().NoError(err)
	}

	s.Assert().Equal(int64(graphOverviewRetainedSnapshots), s.snapshotCount())
}

// --- the read side ---

func (s *GraphOverviewSuite) TestRead_ColdDatabaseReportsNothingBuilt() {
	read := NewGraphOverviewService(s.db)

	payload, etag, err := read.GetGraphOverview()

	s.Require().NoError(err, "an unbuilt map is not an error")
	s.Assert().Nil(payload)
	s.Assert().Empty(etag)
}

func (s *GraphOverviewSuite) TestRead_ServesTheNewestSnapshotWithItsStoredHash() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	row := s.newestSnapshot()

	read := NewGraphOverviewService(s.db)
	payload, etag, err := read.GetGraphOverview()

	s.Require().NoError(err)
	s.Require().NotNil(payload)
	s.Assert().Equal(`W/"`+row.ContentHash+`"`, etag,
		"the ETag is the stored payload digest, marked WEAK because the response is served under "+
			"more than one content-coding (PSY-1734 gzip) so it is not byte-identical across them")
	s.Assert().Equal(row.NodeCount, payload.NodeCount)
	s.Assert().Equal(row.EdgeCount, payload.EdgeCount)
}

func (s *GraphOverviewSuite) TestRead_PicksUpANewSnapshotOnceTheCacheLapses() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	clock := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	read := NewGraphOverviewService(s.db)
	read.now = func() time.Time { return clock }

	_, firstETag, err := read.GetGraphOverview()
	s.Require().NoError(err)

	// A new snapshot lands. Within the TTL the reader keeps serving the old one
	// — that staleness is the deliberate trade for not querying per request.
	s.createArtist("LateArrival")
	_, err = s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	_, cachedETag, err := read.GetGraphOverview()
	s.Require().NoError(err)
	s.Assert().Equal(firstETag, cachedETag, "within the TTL the cached snapshot is served")

	clock = clock.Add(graphOverviewReadTTL + time.Second)
	_, freshETag, err := read.GetGraphOverview()
	s.Require().NoError(err)
	s.Assert().NotEqual(firstETag, freshETag, "past the TTL the newest snapshot must be picked up")
	s.Assert().Equal(`W/"`+s.newestSnapshot().ContentHash+`"`, freshETag)
}

// --- starting points (PSY-1749) ---

func (s *GraphOverviewSuite) startingPointNames() []string {
	read := NewGraphOverviewService(s.db)
	points, err := read.GetGraphStartingPoints()
	s.Require().NoError(err)
	names := make([]string, len(points))
	for i, p := range points {
		names[i] = p.ArtistName
	}
	return names
}

func (s *GraphOverviewSuite) TestStartingPoints_LeadWithTheMapsMostCentralArtists() {
	// seedScene is two stars joined by a heavy bridge: the two centres are the
	// only nodes any cross-star path runs through, so they take the top of the
	// betweenness ranking and their six leaves score exactly zero.
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	names := s.startingPointNames()

	s.Require().GreaterOrEqual(len(names), 2)
	s.Assert().ElementsMatch([]string{"Band1", "Band5"}, names[:2],
		"the two bridge centres must be suggested ahead of their leaves")
	s.Assert().NotContains(names, "Loner1", "an artist with no edge is not on the map at all")
}

func (s *GraphOverviewSuite) TestStartingPoints_ColdDatabaseSuggestsNothing() {
	read := NewGraphOverviewService(s.db)

	points, err := read.GetGraphStartingPoints()

	s.Require().NoError(err, "nothing to suggest is a state, not a failure")
	s.Assert().Empty(points)
	s.Assert().NotNil(points, "an empty result must marshal as [] rather than null")
}

// THE HEADLINE ACCEPTANCE CASE. The hero that shows these suggestions is the
// arm that renders when the map cannot be served, so the suggestions must
// outlive a payload the running build refuses — the deploy window between a
// schema change going live and the next nightly rebuild.
func (s *GraphOverviewSuite) TestStartingPoints_SurviveAPayloadTheMapRefuses() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Exec(
		"UPDATE graph_overview_snapshots SET payload = jsonb_set(payload, '{version}', ?::jsonb)",
		fmt.Sprintf("%d", contracts.GraphOverviewVersion+1)).Error)

	read := NewGraphOverviewService(s.db)
	overview, _, err := read.GetGraphOverview()
	s.Require().NoError(err)
	s.Require().Nil(overview, "precondition: the map must be refusing this payload")

	points, err := read.GetGraphStartingPoints()

	s.Require().NoError(err)
	s.Assert().NotEmpty(points, "the fallback hero's suggestions must not go down with the map")
}

// The "validate against the catalog before display" rule, which the nightly
// ranking cannot honor on its own: it is up to a night old, and an artist can
// be renamed, unslugged, or removed in between.
func (s *GraphOverviewSuite) TestStartingPoints_ResolveAgainstTheLiveCatalog() {
	artists := s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Require().Contains(s.startingPointNames(), "Band1", "precondition: Band1 is ranked")

	// One centre is renamed; the other loses the slug that made it linkable.
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", artists[0].ID).
		Update("name", "Band One, Actually").Error)
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", artists[4].ID).
		Update("slug", nil).Error)

	names := s.startingPointNames()

	s.Assert().Contains(names, "Band One, Actually", "the CURRENT name must be offered, not the one the build stored")
	s.Assert().NotContains(names, "Band1")
	s.Assert().NotContains(names, "Band5", "an artist that can no longer be linked must not be suggested")
}

// A build that predates the column, or one whose ranking step produced nothing,
// must not blank the suggestions while an older build still has a usable list.
func (s *GraphOverviewSuite) TestStartingPoints_FallBackToTheNewestBuildThatHasThem() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	_, err = s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Exec(
		"UPDATE graph_overview_snapshots SET starting_point_artist_ids = NULL WHERE id = ?",
		s.newestSnapshot().ID).Error)

	points, err := NewGraphOverviewService(s.db).GetGraphStartingPoints()

	s.Require().NoError(err)
	s.Assert().NotEmpty(points, "the previous build's ranking is a better answer than none")
}

// The column is a jsonb an operator can edit, and its contents go straight into
// an IN list. An over-long array must be truncated rather than turned into a
// bind message pgx refuses.
func (s *GraphOverviewSuite) TestStartingPoints_TruncateAnOverlongStoredRanking() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)

	// Every id repeated far past the cap. They resolve to real artists, so a
	// missing cap would show up as an over-long response rather than an error.
	var ids []uint
	for i := 0; i < 200; i++ {
		ids = append(ids, s.newestPayload().Nodes.ID...)
	}
	encoded, err := json.Marshal(ids)
	s.Require().NoError(err)
	s.Require().NoError(s.db.Exec(
		"UPDATE graph_overview_snapshots SET starting_point_artist_ids = ?::jsonb WHERE id = ?",
		string(encoded), s.newestSnapshot().ID).Error)

	points, err := NewGraphOverviewService(s.db).GetGraphStartingPoints()

	s.Require().NoError(err)
	s.Assert().LessOrEqual(len(points), graphOverviewStartingPoints)
}

// A stored ranking this build cannot decode must degrade to "nothing to
// suggest" — the client's random fallback — rather than failing the request.
func (s *GraphOverviewSuite) TestStartingPoints_UnreadableRankingSuggestsNothing() {
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Exec(
		`UPDATE graph_overview_snapshots SET starting_point_artist_ids = '{"not":"a list"}'::jsonb WHERE id = ?`,
		s.newestSnapshot().ID).Error)

	points, err := NewGraphOverviewService(s.db).GetGraphStartingPoints()

	s.Require().NoError(err, "an unreadable ranking is not worth failing the request over")
	s.Assert().Empty(points)
}

// The snapshot's stability contract is asserted against content_hash, which
// digests the payload. A list stored in its own column must therefore leave two
// runs over unchanged data byte-identical.
func (s *GraphOverviewSuite) TestStartingPoints_DoNotDisturbTheSnapshotIdentity() {
	// SAME BUILD CLOCK for both runs, so `last_mapped` — the one payload field
	// that legitimately moves every night — is held still and any hash
	// difference can only come from the new column.
	at := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	s.seedScene()
	_, err := s.build(&stubLayoutRunner{}, at)
	s.Require().NoError(err)
	first := s.newestSnapshot()
	s.Require().NotEmpty(first.StartingPointArtistIDs, "precondition: the build stores a ranking")

	_, err = s.build(&stubLayoutRunner{}, at)
	s.Require().NoError(err)
	second := s.newestSnapshot()

	s.Assert().Equal(first.ContentHash, second.ContentHash,
		"content_hash digests the payload; a ranking stored beside it must not enter the map's identity")
	s.Assert().JSONEq(string(first.StartingPointArtistIDs), string(second.StartingPointArtistIDs),
		"an unchanged graph must rank the same artists in the same order")
}
