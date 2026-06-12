package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1080 — festival-scoped co-bill subgraph for GET /festivals/{id}/graph.
//
// Mirrors the scene graph (PSY-367, scene.go) end-to-end: same response
// shape, same is_isolate / is_cross_cluster server-side derivation, same
// type-filter semantics. The cluster signal at festival scope is the
// artist's billing tier on this festival's bill (festival_artists.billing_tier).

// festivalGraphMaxNodes is the hard ceiling on lineup nodes. Festivals render
// lineup-complete (no 30-node cap like the artist graph) per the PSY-1080
// decision, but a runaway lineup must not produce an unbounded payload.
// When the cap bites, the lineup ordering (tier rank, then position, then
// name) decides who stays — headliners are never the ones dropped.
const festivalGraphMaxNodes = 150

// festivalGraphClusterOtherID / Label are the fallback bucket for any lineup
// row whose billing_tier doesn't match a known tier (defensive — the column
// is NOT NULL with a default, but unknown values must not crash the graph).
const (
	festivalGraphClusterOtherID    = "other"
	festivalGraphClusterOtherLabel = "Other"
)

// allowedFestivalEdgeTypes whitelists relationship types the festival graph
// surfaces (per PSY-1080): the stored cross-signals between lineup members
// (shared_bills, shared_label, similar, radio_cooccurrence) plus the
// query-time festival_cobill signal. member_of / side_project are omitted —
// lineage edges don't carry lineup-structure signal the way co-activity does.
var allowedFestivalEdgeTypes = map[string]bool{
	catalogm.RelationshipTypeSharedBills:       true,
	catalogm.RelationshipTypeSharedLabel:       true,
	catalogm.RelationshipTypeSimilar:           true,
	catalogm.RelationshipTypeRadioCooccurrence: true,
	festivalCobillType:                         true,
}

// festivalGraphTierOrder fixes the cluster ordering (and Okabe-Ito
// color_index assignment) from top of the bill down. Mirrors the CASE
// ordering used by GetFestivalArtists.
var festivalGraphTierOrder = []catalogm.BillingTier{
	catalogm.BillingTierHeadliner,
	catalogm.BillingTierSubHeadliner,
	catalogm.BillingTierMidCard,
	catalogm.BillingTierUndercard,
	catalogm.BillingTierLocal,
	catalogm.BillingTierDJ,
	catalogm.BillingTierHost,
}

// festivalGraphTierLabels maps tiers to legend labels.
var festivalGraphTierLabels = map[catalogm.BillingTier]string{
	catalogm.BillingTierHeadliner:    "Headliner",
	catalogm.BillingTierSubHeadliner: "Sub-headliner",
	catalogm.BillingTierMidCard:      "Mid-card",
	catalogm.BillingTierUndercard:    "Undercard",
	catalogm.BillingTierLocal:        "Local",
	catalogm.BillingTierDJ:           "DJ",
	catalogm.BillingTierHost:         "Host",
}

// festivalLineupRow is one artist on the festival's bill, joined with the
// artist columns the graph node needs.
type festivalLineupRow struct {
	ArtistID    uint    `gorm:"column:artist_id"`
	Name        string  `gorm:"column:name"`
	Slug        *string `gorm:"column:slug"`
	City        *string `gorm:"column:city"`
	State       *string `gorm:"column:state"`
	BillingTier string  `gorm:"column:billing_tier"`
}

// GetFestivalGraph returns the co-bill subgraph of a festival's lineup:
// nodes are the artists on the bill (lineup-complete up to
// festivalGraphMaxNodes), clusters are billing tiers, and links are the
// query-time festival_cobill signal plus stored typed edges between lineup
// members.
//
// festival_cobill semantics at festival scope: every pair on this lineup
// trivially shares THIS festival, so counting it would yield a complete
// graph (no structure, O(n²) edges). The derivation therefore counts shared
// festivals EXCLUDING the festival being viewed — an edge means "these two
// artists have also shared other festival bills", which is the structural
// signal the Observatory wants. `detail.count` / `detail.festival_names`
// likewise cover only those other festivals.
//
// types filters to the subset of allowedFestivalEdgeTypes; empty/nil means
// "all allowed types" (festival_cobill included — unlike the artist graph's
// opt-in rule (PSY-954), the festival graph exists to show co-bill structure,
// and the derivation is bounded by the lineup set rather than unbounded).
func (s *FestivalService) GetFestivalGraph(festivalID uint, types []string) (*contracts.FestivalGraphResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var festival catalogm.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	// Whitelist + dedupe types. Mirrors resolveSceneEdgeTypes semantics: empty
	// input means "all allowed types"; a non-empty input that resolves to
	// nothing must short-circuit to zero edges, not fall back to "all types".
	resolvedTypes := resolveFestivalEdgeTypes(types)
	noEdgesByFilter := len(types) > 0 && len(resolvedTypes) == 0
	storedTypes, wantCobill := splitFestivalEdgeTypes(resolvedTypes)

	// 1. Lineup query — bill order (tier rank, position, name) so the
	//    festivalGraphMaxNodes cap drops from the bottom of the bill.
	rows, err := s.queryFestivalLineup(festivalID)
	if err != nil {
		return nil, fmt.Errorf("failed to query festival lineup: %w", err)
	}

	resp := &contracts.FestivalGraphResponse{
		Festival: contracts.FestivalGraphInfo{
			ID:          festival.ID,
			Slug:        festival.Slug,
			Name:        festival.Name,
			Year:        festival.EditionYear,
			ArtistCount: len(rows),
		},
		Clusters: []contracts.FestivalGraphCluster{},
		Nodes:    []contracts.FestivalGraphNode{},
		Links:    []contracts.FestivalGraphLink{},
	}

	if len(rows) == 0 {
		return resp, nil
	}

	// 2. Cluster pass — one cluster per billing tier present on the bill,
	//    ordered headliner-first. At most 7 tiers exist, so every tier fits
	//    inside the 8-color Okabe-Ito palette without a size threshold.
	clusters, clusterByArtist := buildFestivalTierClusters(rows)
	resp.Clusters = clusters

	artistIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		artistIDs = append(artistIDs, r.ArtistID)
	}

	// 3. Batch upcoming-show counts (shared with the scene graph).
	upcomingByArtist := batchArtistUpcomingShowCounts(s.db, artistIDs)

	// 4. Stored typed edges — both endpoints on the lineup.
	var storedLinks []sceneRelationshipRow
	if !noEdgesByFilter && len(storedTypes) > 0 {
		fetched, err := queryRelationshipsAmongArtists(s.db, artistIDs, storedTypes)
		if err != nil {
			return nil, fmt.Errorf("failed to query lineup relationships: %w", err)
		}
		storedLinks = fetched
	}

	// 5. Query-time festival_cobill edges among lineup pairs (excluding this
	//    festival — see method doc).
	var cobillLinks []contracts.FestivalGraphLink
	if !noEdgesByFilter && wantCobill {
		cobillLinks, err = s.queryFestivalGraphCobillEdges(festivalID, artistIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to derive festival co-bill edges: %w", err)
		}
	}

	// 6. Build the link payload + flag cross-cluster (cross-tier) ties.
	connected := make(map[uint]bool, len(rows))
	for _, l := range storedLinks {
		var detail any
		if len(l.Detail) > 0 {
			_ = json.Unmarshal(l.Detail, &detail)
		}
		resp.Links = append(resp.Links, contracts.FestivalGraphLink{
			SourceID:       l.SourceArtistID,
			TargetID:       l.TargetArtistID,
			Type:           l.RelationshipType,
			Score:          float64(l.Score),
			Detail:         detail,
			IsCrossCluster: clusterByArtist[l.SourceArtistID] != clusterByArtist[l.TargetArtistID],
		})
		connected[l.SourceArtistID] = true
		connected[l.TargetArtistID] = true
	}
	for _, l := range cobillLinks {
		l.IsCrossCluster = clusterByArtist[l.SourceID] != clusterByArtist[l.TargetID]
		resp.Links = append(resp.Links, l)
		connected[l.SourceID] = true
		connected[l.TargetID] = true
	}
	resp.Festival.EdgeCount = len(resp.Links)

	// 7. Nodes in bill order, is_isolate from the post-filter link set.
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		city := ""
		if r.City != nil {
			city = *r.City
		}
		state := ""
		if r.State != nil {
			state = *r.State
		}
		resp.Nodes = append(resp.Nodes, contracts.FestivalGraphNode{
			ID:                r.ArtistID,
			Name:              r.Name,
			Slug:              slug,
			City:              city,
			State:             state,
			UpcomingShowCount: upcomingByArtist[r.ArtistID],
			ClusterID:         clusterByArtist[r.ArtistID],
			IsIsolate:         !connected[r.ArtistID],
		})
	}

	return resp, nil
}

// resolveFestivalEdgeTypes filters the caller's requested types against the
// festival-graph allowlist and returns a deterministic slice. Empty input →
// all allowed types. Mirrors resolveSceneEdgeTypes.
func resolveFestivalEdgeTypes(requested []string) []string {
	if len(requested) == 0 {
		out := make([]string, 0, len(allowedFestivalEdgeTypes))
		for t := range allowedFestivalEdgeTypes {
			out = append(out, t)
		}
		sortStringsAsc(out)
		return out
	}
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, t := range requested {
		if !allowedFestivalEdgeTypes[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sortStringsAsc(out)
	return out
}

// splitFestivalEdgeTypes separates the resolved type list into stored
// relationship types (queried from artist_relationships) and the query-time
// festival_cobill flag.
func splitFestivalEdgeTypes(resolved []string) (stored []string, wantCobill bool) {
	stored = make([]string, 0, len(resolved))
	for _, t := range resolved {
		if t == festivalCobillType {
			wantCobill = true
			continue
		}
		stored = append(stored, t)
	}
	return stored, wantCobill
}

// queryFestivalLineup lists the festival's bill in lineup order (tier rank,
// then position, then name; artist id as the final determinism tiebreak),
// capped at festivalGraphMaxNodes. (festival_id, artist_id) is UNIQUE per
// migration 000039, so no dedup pass is needed.
func (s *FestivalService) queryFestivalLineup(festivalID uint) ([]festivalLineupRow, error) {
	const q = `
		SELECT
			a.id    AS artist_id,
			a.name  AS name,
			a.slug  AS slug,
			a.city  AS city,
			a.state AS state,
			fa.billing_tier AS billing_tier
		FROM festival_artists fa
		JOIN artists a ON a.id = fa.artist_id
		WHERE fa.festival_id = ?
		ORDER BY
			CASE fa.billing_tier
				WHEN 'headliner' THEN 1
				WHEN 'sub_headliner' THEN 2
				WHEN 'mid_card' THEN 3
				WHEN 'undercard' THEN 4
				WHEN 'local' THEN 5
				WHEN 'dj' THEN 6
				WHEN 'host' THEN 7
				ELSE 8
			END ASC, fa.position ASC, a.name ASC, a.id ASC
		LIMIT ?
	`
	var rows []festivalLineupRow
	if err := s.db.Raw(q, festivalID, festivalGraphMaxNodes).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// buildFestivalTierClusters converts lineup rows into tier clusters ordered
// headliner-first, with color_index assigned contiguously among the tiers
// actually present. Unknown tiers (defensive) roll into "other" (-1 / grey).
// Returns the cluster list and an artist_id → cluster_id lookup.
func buildFestivalTierClusters(rows []festivalLineupRow) ([]contracts.FestivalGraphCluster, map[uint]string) {
	sizeByTier := make(map[catalogm.BillingTier]int, len(festivalGraphTierOrder))
	clusterByArtist := make(map[uint]string, len(rows))
	otherSize := 0

	for _, r := range rows {
		tier := catalogm.BillingTier(r.BillingTier)
		if _, known := festivalGraphTierLabels[tier]; !known {
			clusterByArtist[r.ArtistID] = festivalGraphClusterOtherID
			otherSize++
			continue
		}
		sizeByTier[tier]++
		clusterByArtist[r.ArtistID] = festivalTierClusterID(tier)
	}

	clusters := make([]contracts.FestivalGraphCluster, 0, len(sizeByTier)+1)
	colorIndex := 0
	for _, tier := range festivalGraphTierOrder {
		size, present := sizeByTier[tier]
		if !present {
			continue
		}
		clusters = append(clusters, contracts.FestivalGraphCluster{
			ID:         festivalTierClusterID(tier),
			Label:      festivalGraphTierLabels[tier],
			Size:       size,
			ColorIndex: colorIndex,
		})
		colorIndex++
	}
	if otherSize > 0 {
		clusters = append(clusters, contracts.FestivalGraphCluster{
			ID:         festivalGraphClusterOtherID,
			Label:      festivalGraphClusterOtherLabel,
			Size:       otherSize,
			ColorIndex: -1,
		})
	}

	return clusters, clusterByArtist
}

// festivalTierClusterID builds the cluster id for a billing tier.
func festivalTierClusterID(tier catalogm.BillingTier) string {
	return "tier_" + string(tier)
}

// queryFestivalGraphCobillEdges derives festival_cobill edges between lineup
// pairs, counting shared festivals OTHER than `festivalID` (see
// GetFestivalGraph doc for why this festival is excluded). Pairs use the
// canonical artistA < artistB ordering, mirroring queryFestivalCobillRows'
// cross-edge mode. Scoring reuses festivalCobillScore (PSY-363) and the
// detail JSONB keeps the same shape ({festival_names, count,
// most_recent_year}) so the frontend edge tooltip renders unchanged.
//
// No edge cap: the pair space is bounded by festivalGraphMaxNodes and the
// requirement of a shared festival outside this one keeps real-world counts
// sparse.
func (s *FestivalService) queryFestivalGraphCobillEdges(festivalID uint, lineupIDs []uint) ([]contracts.FestivalGraphLink, error) {
	if len(lineupIDs) < 2 {
		return nil, nil
	}

	var rows []festivalCobillRow
	err := s.db.Raw(`
		SELECT
			fa1.artist_id AS artist_a,
			fa2.artist_id AS artist_b,
			COUNT(DISTINCT fa1.festival_id) AS shared_count,
			MAX(f.start_date) AS most_recent_start,
			MAX(EXTRACT(YEAR FROM f.start_date))::int AS most_recent_year
		FROM festival_artists fa1
		JOIN festival_artists fa2 ON fa1.festival_id = fa2.festival_id
			AND fa1.artist_id < fa2.artist_id
		JOIN festivals f ON f.id = fa1.festival_id
		WHERE fa1.festival_id <> ?
			AND fa1.artist_id IN ? AND fa2.artist_id IN ?
		GROUP BY fa1.artist_id, fa2.artist_id
		ORDER BY shared_count DESC, most_recent_start DESC, fa1.artist_id ASC, fa2.artist_id ASC
	`, festivalID, lineupIDs, lineupIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query festival co-lineup (festival graph): %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Tooltip names are best-effort (mirrors computeFestivalCobillCenterEdges):
	// a failure here degrades to count-only tooltips, never fails the graph.
	nameMap, err := s.queryFestivalGraphCobillNames(festivalID, rows)
	if err != nil {
		nameMap = nil
	}

	now := time.Now()
	links := make([]contracts.FestivalGraphLink, 0, len(rows))
	for _, r := range rows {
		var festivalNames string
		if nameMap != nil {
			festivalNames = strings.Join(nameMap[pairKey(r.ArtistA, r.ArtistB)], ", ")
		}
		detail := map[string]interface{}{
			"festival_names": festivalNames,
			"count":          r.SharedCount,
		}
		if r.MostRecentYear != nil {
			detail["most_recent_year"] = *r.MostRecentYear
		} else {
			detail["most_recent_year"] = nil
		}
		links = append(links, contracts.FestivalGraphLink{
			SourceID: r.ArtistA,
			TargetID: r.ArtistB,
			Type:     festivalCobillType,
			Score:    festivalCobillScore(r.SharedCount, r.MostRecentStart, now),
			Detail:   detail,
		})
	}
	return links, nil
}

// queryFestivalGraphCobillNames fetches up to festivalCobillTopFestivalNames
// shared-festival names (most recent first) per pair, excluding `festivalID`
// so the tooltip lists only the OTHER festivals the pair has shared —
// consistent with the edge's count. Pairs are canonical (artistA < artistB),
// matching queryFestivalGraphCobillEdges' projection, so no key mirroring is
// needed (unlike queryFestivalCobillNames' center mode).
func (s *FestivalService) queryFestivalGraphCobillNames(festivalID uint, pairs []festivalCobillRow) (map[string][]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	idSet := make(map[uint]struct{}, len(pairs)*2)
	for _, p := range pairs {
		idSet[p.ArtistA] = struct{}{}
		idSet[p.ArtistB] = struct{}{}
	}
	idList := make([]uint, 0, len(idSet))
	for id := range idSet {
		idList = append(idList, id)
	}

	type nameRow struct {
		ArtistA      uint
		ArtistB      uint
		FestivalName string
	}
	var results []nameRow
	err := s.db.Raw(`
		SELECT
			fa1.artist_id AS artist_a,
			fa2.artist_id AS artist_b,
			f.name AS festival_name
		FROM festival_artists fa1
		JOIN festival_artists fa2 ON fa1.festival_id = fa2.festival_id
			AND fa1.artist_id < fa2.artist_id
		JOIN festivals f ON f.id = fa1.festival_id
		WHERE fa1.festival_id <> ?
			AND fa1.artist_id IN ? AND fa2.artist_id IN ?
		ORDER BY f.start_date DESC, f.edition_year DESC
	`, festivalID, idList, idList).Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query festival names (festival graph): %w", err)
	}

	wanted := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		wanted[pairKey(p.ArtistA, p.ArtistB)] = struct{}{}
	}

	out := make(map[string][]string, len(pairs))
	for _, r := range results {
		key := pairKey(r.ArtistA, r.ArtistB)
		if _, ok := wanted[key]; !ok {
			continue
		}
		if len(out[key]) >= festivalCobillTopFestivalNames {
			continue
		}
		out[key] = append(out[key], r.FestivalName)
	}
	return out, nil
}
