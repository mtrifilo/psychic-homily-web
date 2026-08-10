package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Nightly graph-overview snapshot ("Map of the Scene")
// ──────────────────────────────────────────────
//
// The overview map ships POSITIONS, NOT PHYSICS. A client-side force warmup at
// this node count costs seconds of blocked main thread, so the layout runs once
// a night here and every visitor renders the same precomputed asset.
//
// The pipeline, in the order the steps MUST run:
//
//  1. LoadCommunityGraphEdges — verbatim, so the map is drawn on exactly the
//     edge set the Leiden partition colours it with. A second loader would
//     drift and the regions would stop matching the graph under them.
//  2. Disparity backbone — the same statistical sparsification the scene graph
//     uses, whose doc comment nominates scene-scale-and-above as its consumer.
//  3. NODE SET, built from what survived steps 2 and the edge cap.
//  4. Label hubs, passed EXACTLY that node set. Order matters and is not
//     interchangeable: the hub builder evaluates every rule in-set, so an
//     under-complete artist set silently deflates roster counts and drops
//     spokes with no error anywhere. Hub first, prune second would do exactly
//     that.
//  5. Layout, alignment, quantization (graph_overview_layout.go).
//  6. Metrics and regions (graph_overview_metrics.go).
//  7. One INSERT into the append-only snapshot table.
//
// FAILURE POSTURE mirrors ComputeArtistCommunities: the caller treats an error
// as non-fatal and the previous snapshot stays live. Because the table is
// append-only, that is automatic — nothing is deleted until a new row has
// committed. Degraded INPUT aborts rather than persisting: an empty edge set or
// an empty backbone means an upstream step failed this cycle, and publishing
// the gutted map that results would replace a good map with a wrong one.

const (
	// graphOverviewMaxEdges caps the drawn edge set. The disparity backbone is
	// the real sparsifier; this is the backstop that keeps the payload bounded
	// if catalog growth or an alpha change ever lets the backbone through
	// wholesale. When it binds, the strongest edges are kept.
	graphOverviewMaxEdges = 60000

	// graphOverviewRetainedSnapshots is how many builds stay in the table. One
	// is live, one is the warm-start source it was built from, and the third is
	// an operator's ability to see what changed without a backup restore.
	graphOverviewRetainedSnapshots = 3

	// graphOverviewHullPadding pads each region hull outward as a fraction of
	// its radius, so boundary artists sit inside their region instead of on its
	// edge.
	graphOverviewHullPadding = 0.12

	// graphOverviewArtistChunk bounds an IN-list read. pgx refuses a bind
	// message over 65535 parameters; the catalog held ~4,700 artists when this
	// was written, so today every read is one statement — but the node set is
	// derived from catalog size and this is the ceiling it would grow into.
	// Chunked reads are re-sorted in Go to the same key the single-statement
	// ORDER BY would have produced, so chunking never changes a result.
	graphOverviewArtistChunk = 8000

	// graphOverviewAdvisoryLock serializes snapshot builds against each other
	// (an operator-triggered rebuild racing the nightly ticker). Arbitrary
	// app-unique key, same discipline as the community compute's.
	graphOverviewAdvisoryLock = 7412620002
)

// graphOverviewEpoch is the origin for every appearTime in the payload. It
// predates the catalog's oldest row by years, so no entity has a negative
// appearance and the value fits an int32 for centuries.
var graphOverviewEpoch = time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)

// GraphOverviewComputeResult summarizes one snapshot build.
type GraphOverviewComputeResult struct {
	Nodes        int
	Edges        int
	Regions      int
	Isolates     int
	PayloadBytes int
	Duration     time.Duration
}

// ComputeGraphOverviewSnapshot builds and publishes the nightly overview map.
// Callers treat errors as non-fatal — the previous snapshot stays live.
func (s *RadioService) ComputeGraphOverviewSnapshot(ctx context.Context) (GraphOverviewComputeResult, error) {
	return s.computeGraphOverviewSnapshot(ctx, graphOverviewLayoutRunner(), time.Now)
}

// computeGraphOverviewSnapshot is the testable core: the layout runner and the
// clock are injected so the pipeline can be exercised without Node installed
// and without a wall-clock dependency in assertions.
func (s *RadioService) computeGraphOverviewSnapshot(
	ctx context.Context,
	runner graphLayoutRunner,
	now func() time.Time,
) (GraphOverviewComputeResult, error) {
	start := time.Now()
	var result GraphOverviewComputeResult
	if s.db == nil {
		return result, fmt.Errorf("database not initialized")
	}

	// 1. The Leiden edge set, verbatim.
	edges, err := s.LoadCommunityGraphEdges()
	if err != nil {
		return result, err
	}
	if len(edges) == 0 {
		return result, fmt.Errorf("overview snapshot: no relationship edges; keeping previous snapshot")
	}

	// 2. Disparity backbone + cap.
	backbone := graphOverviewBackbone(edges)
	if len(backbone) == 0 {
		return result, fmt.Errorf(
			"overview snapshot: backbone is empty at alpha %g over %d edges; keeping previous snapshot",
			RadioBackboneAlpha(), len(edges))
	}

	// 3. The node set — built from what survived, never before.
	artistIDs := backboneArtistIDs(backbone)

	artistMeta, err := queryGraphOverviewArtists(s.db, artistIDs)
	if err != nil {
		return result, err
	}
	// Drop anything that cannot be drawn as a real, clickable node:
	//
	//   - no artists row at all — a dangling relationship, which would otherwise
	//     become a nameless dot;
	//   - no slug — the map's whole payoff is clicking through to the artist, and
	//     a node promising a click it cannot honor is worse than an absent one.
	//     This is the same rule the hub builder already applies to slug-less
	//     labels, and the contract's "a slug is never empty" claim depends on it.
	//
	// BOTH must happen BEFORE the hub builder sees the set, since that set is the
	// hub scope definition and an under-complete one is silently wrong.
	drawable := make(map[uint]overviewArtist, len(artistMeta))
	for id, meta := range artistMeta {
		if meta.Slug == nil || *meta.Slug == "" {
			continue
		}
		drawable[id] = meta
	}
	if len(drawable) != len(artistIDs) {
		filtered := artistIDs[:0]
		for _, id := range artistIDs {
			if _, ok := drawable[id]; ok {
				filtered = append(filtered, id)
			}
		}
		artistIDs = filtered
		backbone = filterBackboneToArtists(backbone, drawable)
	}
	artistMeta = drawable
	if len(artistIDs) == 0 || len(backbone) == 0 {
		return result, fmt.Errorf("overview snapshot: no drawable artists after backbone; keeping previous snapshot")
	}

	// 4. Label hubs over EXACTLY that node set.
	hubs := s.buildOverviewLabelHubs(artistIDs)

	// Pairwise shared-label edges the hubs now carry are dropped. A pair is
	// only eligible when EVERY relationship it has is a shared_label one —
	// otherwise the pair also encodes a co-bill or a member overlap that no
	// spoke represents, and dropping it would delete that fact.
	if len(hubs.Spokes) > 0 {
		labelOnly, err := queryLabelOnlyPairs(s.db)
		if err != nil {
			// Non-fatal: without the pair set nothing is dropped, so the map
			// carries the cliques the hubs duplicate. Uglier and larger, but
			// correct — and strictly better than failing the whole build.
			slog.Error("overview snapshot: shared-label-only pair query failed; keeping pairwise label edges", "error", err)
		} else {
			backbone = dropHubbedLabelEdges(backbone, labelOnly, hubs)
		}
	}

	build := newOverviewBuild(artistIDs, artistMeta, hubs, backbone)

	// 5. Layout.
	previous, previousStructure, err := s.previousOverviewLayout()
	if err != nil {
		return result, err
	}
	structure := build.structureKey()

	var points []layoutPoint
	iterations := 0
	reused := structure != "" && structure == previousStructure && layoutCovers(previous, build.nodeIDs)
	if reused {
		// NOTHING MOVED, SO NOTHING MOVES. The node and edge sets are identical
		// to the snapshot these positions came from, so running the layout again
		// could only relax an already-relaxed map a little further — drifting
		// every dot for no new information, every night, forever. Replaying the
		// stored positions is what makes "unchanged data reproduces byte-
		// identical positions" literally true rather than approximately true.
		points = make([]layoutPoint, len(build.nodeIDs))
		for i, id := range build.nodeIDs {
			points[i] = previous[id]
		}
	} else {
		iterations = graphOverviewColdIterations
		if len(previous) > 0 {
			iterations = graphOverviewWarmIterations
		}
		seeds := seedLayoutPositions(build.nodeIDs, build.neighbors, previous)
		req := layoutRequest{
			Iterations: iterations,
			Nodes:      make([]layoutNode, len(seeds)),
			Edges:      make([]layoutEdge, len(build.edgeSrc)),
		}
		for i, p := range seeds {
			req.Nodes[i] = layoutNode(p)
		}
		for i := range build.edgeSrc {
			// UNIFORM LAYOUT WEIGHT. The stored scores mix relationship types on
			// incomparable scales (a co-billing count against a normalized label
			// overlap), so feeding them in as attraction strengths would let one
			// derivation's units decide the map's shape. The backbone has already
			// decided WHICH edges are real; the layout draws their topology.
			req.Edges[i] = layoutEdge{Source: int(build.edgeSrc[i]), Target: int(build.edgeDst[i]), Weight: 1}
		}

		points, err = runner.Run(ctx, req)
		if err != nil {
			return result, fmt.Errorf("overview snapshot: layout failed: %w", err)
		}
		points = alignToPrevious(build.nodeIDs, points, previous)
	}
	if err := assertFinitePositions(points); err != nil {
		return result, fmt.Errorf("overview snapshot: %w", err)
	}
	xs, ys, extent := quantizePositions(points)

	// 6. Metrics and regions.
	centrality, rankMetric := betweennessCentrality(build.neighbors)
	ranks := rankByScore(centrality)

	firstShow, err := queryArtistFirstShowDates(s.db)
	if err != nil {
		return result, err
	}
	appear := build.appearTimes(artistMeta, firstShow)

	upcoming, err := queryArtistsWithUpcomingShows(s.db, now().UTC().Truncate(24*time.Hour))
	if err != nil {
		return result, err
	}
	flags := build.nodeFlags(artistMeta, upcoming)

	communityLabels, err := queryCommunityLabels(s.db)
	if err != nil {
		return result, err
	}
	regions := build.regions(points, communityLabels, extent)

	totalArtists, err := countCatalogArtists(s.db)
	if err != nil {
		return result, err
	}
	isolates := totalArtists - len(artistIDs)
	if isolates < 0 {
		isolates = 0
	}

	// 7. Assemble and publish.
	builtAt := now().UTC()
	payload := build.payload(builtAt, extent, xs, ys, ranks, rankMetric, appear, flags, regions, isolates)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("overview snapshot: failed to encode payload: %w", err)
	}
	layoutJSON, err := json.Marshal(build.layoutState(points))
	if err != nil {
		return result, fmt.Errorf("overview snapshot: failed to encode layout state: %w", err)
	}
	sum := sha256.Sum256(payloadJSON)

	// The /graph hero's starting suggestions (PSY-1749). Stored in their own
	// column, NOT in the payload: content_hash digests payloadJSON and the
	// stability contract is asserted against it, so a list derived from the same
	// centrality must not be able to change the map's identity stamp.
	//
	// A failure here is NON-FATAL and degrades to "no suggestions", which the
	// client already answers with a random catalog artist. Losing a sentence in
	// a zero state is not worth keeping last night's whole map live over.
	var startingPointsJSON json.RawMessage
	startingPoints := pickStartingPointArtists(build, centrality)
	if len(startingPoints) > 0 {
		encoded, err := json.Marshal(startingPoints)
		if err != nil {
			slog.Error("overview snapshot: failed to encode starting points; publishing without them", "error", err)
		} else {
			startingPointsJSON = encoded
		}
	}

	snapshot := catalogm.GraphOverviewSnapshot{
		Payload:                payloadJSON,
		Layout:                 layoutJSON,
		NodeCount:              payload.NodeCount,
		EdgeCount:              payload.EdgeCount,
		IsolateCount:           payload.IsolateCount,
		StartingPointArtistIDs: startingPointsJSON,
		ContentHash:            hex.EncodeToString(sum[:]),
		StructureKey:           structure,
		ComputedAt:             builtAt,
	}
	if err := s.publishOverviewSnapshot(&snapshot); err != nil {
		return result, err
	}

	result = GraphOverviewComputeResult{
		Nodes:        payload.NodeCount,
		Edges:        payload.EdgeCount,
		Regions:      len(payload.Regions),
		Isolates:     payload.IsolateCount,
		PayloadBytes: len(payloadJSON),
		Duration:     time.Since(start),
	}
	slog.Info("graph overview snapshot published",
		"nodes", result.Nodes,
		"edges", result.Edges,
		"regions", result.Regions,
		"isolates", result.Isolates,
		"starting_points", len(startingPoints),
		"payload_bytes", result.PayloadBytes,
		"layout_reused", reused,
		"layout_iterations", iterations,
		"warm_start_nodes", len(previous),
		"duration", result.Duration)
	return result, nil
}

// ──────────────────────────────────────────────
// Backbone
// ──────────────────────────────────────────────

// overviewEdge is one undirected backbone edge in canonical (A < B) order.
//
// Weight is only populated when the edge cap binds, which is the sole consumer
// — the layout deliberately runs on uniform weights and the payload does not
// carry them. Treat it as zero everywhere else.
type overviewEdge struct {
	A      uint
	B      uint
	Weight float64
}

// graphOverviewBackbone applies the disparity filter to the loaded edge set and
// returns the surviving edges in a deterministic order.
//
// The cap is applied on WEIGHT, strongest first, with the canonical endpoint
// pair as the tiebreaker so two runs over the same data cut at the same place.
func graphOverviewBackbone(edges []WeightedEdge) []overviewEdge {
	significance := DisparitySignificance(edges)
	alpha := RadioBackboneAlpha()

	kept := make([]overviewEdge, 0, len(significance))
	for key, sig := range significance {
		if sig >= alpha {
			continue
		}
		kept = append(kept, overviewEdge{A: key[0], B: key[1]})
	}
	sortOverviewEdges(kept)

	if len(kept) > graphOverviewMaxEdges {
		// Weights are only needed to RANK the cut, so they are collapsed here
		// rather than for every build. DisparitySignificance already did this
		// collapse internally; re-deriving it the same way (drop self-loops and
		// non-positive weights, sum parallels by canonical pair) is what makes
		// the cap rank on the same number the filter tested.
		weightByPair := make(map[EdgeKey]float64, len(significance))
		for _, e := range edges {
			if e.A == e.B || e.Weight <= 0 {
				continue
			}
			weightByPair[canonicalKey(e.A, e.B)] += e.Weight
		}
		byStrength := make([]overviewEdge, len(kept))
		copy(byStrength, kept)
		for i := range byStrength {
			byStrength[i].Weight = weightByPair[canonicalKey(byStrength[i].A, byStrength[i].B)]
		}
		sort.SliceStable(byStrength, func(i, j int) bool {
			return byStrength[i].Weight > byStrength[j].Weight
		})
		kept = byStrength[:graphOverviewMaxEdges]
		sortOverviewEdges(kept)
		slog.Warn("overview snapshot: backbone exceeded the edge cap; kept the strongest",
			"cap", graphOverviewMaxEdges, "backbone", len(kept), "candidate_pairs", len(significance))
	}
	return kept
}

// sortOverviewEdges puts edges in canonical (A, B) order. Map iteration is
// randomized in Go, so this is what makes every downstream index assignment
// reproducible.
func sortOverviewEdges(edges []overviewEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].A != edges[j].A {
			return edges[i].A < edges[j].A
		}
		return edges[i].B < edges[j].B
	})
}

// backboneArtistIDs returns the ascending, deduplicated endpoint set.
func backboneArtistIDs(edges []overviewEdge) []uint {
	seen := make(map[uint]struct{}, len(edges)*2)
	ids := make([]uint, 0, len(edges)*2)
	for _, e := range edges {
		for _, id := range [2]uint{e.A, e.B} {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// filterBackboneToArtists drops edges with an endpoint outside the known set.
func filterBackboneToArtists(edges []overviewEdge, known map[uint]overviewArtist) []overviewEdge {
	out := edges[:0]
	for _, e := range edges {
		if _, ok := known[e.A]; !ok {
			continue
		}
		if _, ok := known[e.B]; !ok {
			continue
		}
		out = append(out, e)
	}
	return out
}

// dropHubbedLabelEdges removes pairwise edges whose only content is a shared
// label that became a hub.
func dropHubbedLabelEdges(edges []overviewEdge, labelOnly map[EdgeKey]struct{}, hubs labelHubs) []overviewEdge {
	if len(labelOnly) == 0 {
		return edges
	}
	out := edges[:0]
	for _, e := range edges {
		if _, only := labelOnly[canonicalKey(e.A, e.B)]; only && hubs.replacesSharedLabelEdge(e.A, e.B) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// buildOverviewLabelHubs runs the shared hub builder over the map's node set.
//
// Hub failure is NON-FATAL and degrades to the pairwise cliques the hubs would
// have replaced — a larger, uglier, still-correct map. The alternative,
// failing the build, would keep last night's map live over a decorative layer.
func (s *RadioService) buildOverviewLabelHubs(artistIDs []uint) labelHubs {
	rosterRows, err := queryLabelRostersChunked(s.db, artistIDs)
	if err != nil {
		slog.Error("overview snapshot: label roster query failed; falling back to pairwise label edges",
			"artist_count", len(artistIDs), "error", err)
		return labelHubs{}
	}
	built, err := buildLabelHubs(rosterRows, artistIDs)
	if err != nil {
		slog.Error("overview snapshot: label hub build failed; falling back to pairwise label edges",
			"artist_count", len(artistIDs), "roster_rows", len(rosterRows), "error", err)
		return labelHubs{}
	}
	return built
}

// queryLabelRostersChunked is queryLabelRosters for a catalog-wide artist set.
//
// pgx caps a bind message at 65535 parameters and the artist set lands in a
// single IN list, so a catalog large enough to exceed the cap would fail the
// read outright. Chunking splits the set and re-sorts the union on the SAME key
// the single-statement ORDER BY uses — label name, then label id (labels.name
// carries no unique constraint, so the id is what keeps two same-named labels
// in a fixed order), then artist id. Reproducing that order is the whole
// contract: buildLabelHubs emits hubs in first-seen label order, and an
// unstable order re-lays out the canvas under the reader.
func queryLabelRostersChunked(db *gorm.DB, artistIDs []uint) ([]labelRosterRow, error) {
	if len(artistIDs) <= graphOverviewArtistChunk {
		return queryLabelRosters(db, artistIDs)
	}
	var all []labelRosterRow
	for _, batch := range chunkIDs(artistIDs, graphOverviewArtistChunk) {
		rows, err := queryLabelRosters(db, batch)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		if all[i].LabelID != all[j].LabelID {
			return all[i].LabelID < all[j].LabelID
		}
		return all[i].ArtistID < all[j].ArtistID
	})
	return all, nil
}

// ──────────────────────────────────────────────
// Reads
// ──────────────────────────────────────────────

// overviewArtist is the per-artist metadata the payload needs.
type overviewArtist struct {
	ID          uint       `gorm:"column:id"`
	Name        string     `gorm:"column:name"`
	Slug        *string    `gorm:"column:slug"`
	CommunityID *int       `gorm:"column:community_id"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	FirstShow   *time.Time `gorm:"-"`
	// BandcampEmbedURL is read only to answer "does this dot play something",
	// which is one of the two affordances that decide whether a dot is worth
	// clicking. The URL itself is not shipped — the overview links to the
	// artist page, which already knows how to play it.
	BandcampEmbedURL *string `gorm:"column:bandcamp_embed_url"`
}

// queryGraphOverviewArtists loads node metadata for the artist set, chunked
// against the bind-parameter ceiling (see graphOverviewArtistChunk).
func queryGraphOverviewArtists(db *gorm.DB, artistIDs []uint) (map[uint]overviewArtist, error) {
	out := make(map[uint]overviewArtist, len(artistIDs))
	for _, batch := range chunkIDs(artistIDs, graphOverviewArtistChunk) {
		var rows []overviewArtist
		err := db.Table("artists").
			Select("id, name, slug, community_id, created_at, bandcamp_embed_url").
			Where("id IN ?", batch).
			Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to load overview artist metadata: %w", err)
		}
		for _, r := range rows {
			out[r.ID] = r
		}
	}
	return out, nil
}

// queryArtistFirstShowDates returns each artist's earliest approved show date.
//
// THE CLOCK IS THE SHOW DATE, NOT A ROW STAMP. `artist_relationships.created_at`
// records when a derive run last wrote the row, so an appearance timeline built
// on it would show the whole graph springing into existence on the night of the
// last backfill. `shows.event_date` and the entity's own `created_at` are the
// only two clocks in this schema that describe when something happened.
//
// No bind parameters: the aggregate covers the whole table, and the map's
// artists are a subset of it. That is cheaper than chunking a large IN list and
// removes a way for the read to disagree with the node set.
func queryArtistFirstShowDates(db *gorm.DB) (map[uint]time.Time, error) {
	type row struct {
		ArtistID  uint      `gorm:"column:artist_id"`
		FirstShow time.Time `gorm:"column:first_show"`
	}
	var rows []row
	err := db.Table("show_artists AS sa").
		Select("sa.artist_id AS artist_id, MIN(s.event_date) AS first_show").
		Joins("JOIN shows s ON s.id = sa.show_id").
		Where("s.status = ?", catalogm.ShowStatusApproved).
		Group("sa.artist_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load first show dates: %w", err)
	}
	out := make(map[uint]time.Time, len(rows))
	for _, r := range rows {
		out[r.ArtistID] = r.FirstShow
	}
	return out, nil
}

// queryLabelOnlyPairs returns the artist pairs whose EVERY stored relationship
// is a shared_label one — the only pairs a label hub can fully replace.
//
// A pair that also carries a co-bill or a member overlap encodes a fact no
// spoke represents, so it keeps its edge even when both artists sit on a
// hubbed label. Keyed canonically to match the backbone's edge keys.
func queryLabelOnlyPairs(db *gorm.DB) (map[EdgeKey]struct{}, error) {
	type row struct {
		SourceArtistID uint `gorm:"column:source_artist_id"`
		TargetArtistID uint `gorm:"column:target_artist_id"`
	}
	var rows []row
	err := db.Table("artist_relationships").
		Select("source_artist_id, target_artist_id").
		Where("score > 0").
		Group("source_artist_id, target_artist_id").
		Having("BOOL_AND(relationship_type = ?)", catalogm.RelationshipTypeSharedLabel).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load shared-label-only pairs: %w", err)
	}
	out := make(map[EdgeKey]struct{}, len(rows))
	for _, r := range rows {
		out[canonicalKey(r.SourceArtistID, r.TargetArtistID)] = struct{}{}
	}
	return out, nil
}

// queryArtistsWithUpcomingShows returns the artists with an approved show on or
// after `from`.
//
// Same shape and the same reasoning as queryArtistFirstShowDates: no bind
// parameters beyond the date, because the map's artists are a subset of the
// whole table and an aggregate is cheaper than chunking a catalog-sized IN list.
func queryArtistsWithUpcomingShows(db *gorm.DB, from time.Time) (map[uint]struct{}, error) {
	var ids []uint
	err := db.Table("show_artists AS sa").
		Select("DISTINCT sa.artist_id").
		Joins("JOIN shows s ON s.id = sa.show_id").
		Where("s.status = ?", catalogm.ShowStatusApproved).
		Where("s.event_date >= ?", from).
		Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load artists with upcoming shows: %w", err)
	}
	out := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// overviewCommunityLabel is one community's display metadata.
type overviewCommunityLabel struct {
	CommunityID int    `gorm:"column:community_id"`
	ArtistName  string `gorm:"column:artist_name"`
}

// queryCommunityLabels loads the "Around {artist}" anchors written by the
// Leiden compute, in one statement so a label can never be read from a
// different partition than the assignments it belongs to.
func queryCommunityLabels(db *gorm.DB) (map[int]string, error) {
	var rows []overviewCommunityLabel
	err := db.Table("artist_communities AS ac").
		Select("ac.id AS community_id, a.name AS artist_name").
		Joins("JOIN artists a ON a.id = ac.label_artist_id").
		Order("ac.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load community labels: %w", err)
	}
	out := make(map[int]string, len(rows))
	for _, r := range rows {
		out[r.CommunityID] = r.ArtistName
	}
	return out, nil
}

// countCatalogArtists counts every artist, so the payload can report how many
// are NOT on the map.
func countCatalogArtists(db *gorm.DB) (int, error) {
	var n int64
	if err := db.Table("artists").Count(&n).Error; err != nil {
		return 0, fmt.Errorf("failed to count artists: %w", err)
	}
	return int(n), nil
}

// previousOverviewLayout loads the newest snapshot's warm-start positions.
// A missing snapshot is not an error — it is the cold start.
func (s *RadioService) previousOverviewLayout() (map[uint]layoutPoint, string, error) {
	var row struct {
		Layout       json.RawMessage `gorm:"column:layout"`
		StructureKey string          `gorm:"column:structure_key"`
		Version      *int            `gorm:"column:version"`
	}
	// The version comes out of the stored payload rather than a column so the
	// two can never disagree: it is by construction the version that wrote this
	// layout.
	err := s.db.Table("graph_overview_snapshots").
		Select("layout, structure_key, (payload->>'version')::int AS version").
		Order("id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, "", fmt.Errorf("failed to load previous overview layout: %w", err)
	}
	if len(row.Layout) == 0 {
		return nil, "", nil
	}
	// A WARM START ACROSS A SCHEMA VERSION IS NOT SAFE. Node ids are the only
	// correspondence between two epochs, so a version that re-scoped them (a
	// different hub-id offset, say) would resume nodes at coordinates belonging
	// to different entities — silently, because every id would still be found.
	// Cold-starting instead is the "explicit versioned product event" the
	// stability contract reserves for exactly this.
	if row.Version == nil || *row.Version != contracts.GraphOverviewVersion {
		slog.Warn("overview snapshot: previous layout is a different payload version; cold-starting",
			"previous_version", row.Version, "current_version", contracts.GraphOverviewVersion)
		return nil, "", nil
	}
	var stored map[string][2]float32
	if err := json.Unmarshal(row.Layout, &stored); err != nil {
		// A layout blob this build cannot read is a cold start, not a failure:
		// refusing to build would keep an unreadable snapshot live forever.
		slog.Warn("overview snapshot: previous layout unreadable; cold-starting", "error", err)
		return nil, "", nil
	}
	out := make(map[uint]layoutPoint, len(stored))
	unparsable := 0
	for key, p := range stored {
		id, err := parseNodeID(key)
		if err != nil {
			// Count rather than fail: a key this build cannot read is a node it
			// will cold-seed, which is survivable. Silence is not — a changed
			// id scheme would drop every warm start and reshuffle the whole map
			// while the build still reported success.
			unparsable++
			continue
		}
		out[id] = layoutPoint{X: p[0], Y: p[1]}
	}
	if unparsable > 0 {
		slog.Warn("overview snapshot: previous layout had unreadable node keys; those nodes cold-seed",
			"unparsable", unparsable, "readable", len(out))
	}
	return out, row.StructureKey, nil
}

// publishOverviewSnapshot inserts the new snapshot and prunes old ones.
//
// INSERT-then-prune inside one transaction under an advisory lock: a reader
// taking the newest row sees the previous snapshot until this commits and the
// new one immediately after, never neither. The lock is transaction-scoped and
// releases on commit or rollback.
func (s *RadioService) publishOverviewSnapshot(snapshot *catalogm.GraphOverviewSnapshot) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", graphOverviewAdvisoryLock).Error; err != nil {
			return fmt.Errorf("failed to take overview snapshot lock: %w", err)
		}
		if err := tx.Create(snapshot).Error; err != nil {
			return fmt.Errorf("failed to insert overview snapshot: %w", err)
		}
		prune := tx.Exec(
			"DELETE FROM graph_overview_snapshots WHERE id NOT IN (SELECT id FROM graph_overview_snapshots ORDER BY id DESC LIMIT ?)",
			graphOverviewRetainedSnapshots)
		if prune.Error != nil {
			return fmt.Errorf("failed to prune old overview snapshots: %w", prune.Error)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("overview snapshot: %w", err)
	}
	return nil
}

// parseNodeID converts a stored layout key back to a node id. It is the exact
// inverse of the strconv.FormatUint the layout state is written with — Sscanf
// would accept trailing garbage the writer can never produce.
func parseNodeID(key string) (uint, error) {
	id, err := strconv.ParseUint(key, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

// chunkIDs splits an id set into bind-parameter-sized batches.
//
// pgx refuses a bind message over 65535 parameters, so any catalog-wide IN list
// has a ceiling. Three reads in this package now need the same split; doing it
// here keeps the off-by-one in one place instead of once per query.
func chunkIDs(ids []uint, size int) [][]uint {
	if size <= 0 || len(ids) <= size {
		return [][]uint{ids}
	}
	out := make([][]uint, 0, (len(ids)+size-1)/size)
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[start:end])
	}
	return out
}

// appearSeconds converts a timestamp to the payload's epoch-relative seconds,
// clamped at zero for anything predating the epoch.
func appearSeconds(t time.Time) int32 {
	secs := t.UTC().Sub(graphOverviewEpoch).Seconds()
	if secs < 0 {
		return 0
	}
	if secs > float64(int32(^uint32(0)>>1)) {
		return int32(^uint32(0) >> 1)
	}
	return int32(secs)
}
