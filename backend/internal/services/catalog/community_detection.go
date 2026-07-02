package catalog

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Artist community detection (PSY-1262): the nightly step that runs the
// pure-Go Leiden partition (leiden.go) over the stored artist-similarity
// graph and persists the result.
//
// Input graph (maintainer decision, 2026-07-02): ALL relationship types with
// weight = score — except radio_cooccurrence, which is restricted to
// backbone-significant edges (backbone_significance < RadioBackboneAlpha())
// so hub noise can't glue everything into one blob. One GLOBAL partition,
// projected per scene; reusable by the ego graph and the PSY-1263 embedding
// spike.
//
// Persistence: artists.community_id + the artist_communities label table
// ("Around {artist}", anchored on each community's highest-strength member),
// swapped atomically in one transaction so readers never see a half-updated
// partition. The Leiden seed is FIXED so a re-run over unchanged data yields
// an identical partition (no color/grouping reshuffle between nights); when
// the graph itself changes, assignments legitimately move.

// leidenSeed pins the partition's RNG so recomputes are reproducible.
const leidenSeed int64 = 42

// CommunityComputeResult summarizes one persisted partition rebuild.
type CommunityComputeResult struct {
	Communities     int
	AssignedArtists int
	// ClearedArtists counts artists that LEFT the assigned set this rebuild
	// (diff-based swap: unchanged assignments are not rewritten).
	ClearedArtists int64
}

// LoadCommunityGraphEdges loads the exact filtered edge set the persisted
// partition is computed on. Exported so tuning tools (cmd/community-gamma-sweep)
// sweep the SAME graph the nightly compute uses — a duplicated loader would
// drift. Read-only.
func (s *RadioService) LoadCommunityGraphEdges() ([]WeightedEdge, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// ORDER BY closes a determinism hole: without it, Postgres scan order
	// varies run-to-run and float accumulation order (strengths, collapsed
	// weights) can differ at the ULP level, flipping exact-equality tie-breaks
	// — the seed alone does not pin the partition (adversarial finding).
	type relRow struct {
		SourceArtistID       uint     `gorm:"column:source_artist_id"`
		TargetArtistID       uint     `gorm:"column:target_artist_id"`
		RelationshipType     string   `gorm:"column:relationship_type"`
		Score                float64  `gorm:"column:score"`
		BackboneSignificance *float64 `gorm:"column:backbone_significance"`
	}
	var rows []relRow
	err := s.db.Model(&catalogm.ArtistRelationship{}).
		Select("source_artist_id, target_artist_id, relationship_type, score, backbone_significance").
		Where("score > 0").
		Order("source_artist_id, target_artist_id, relationship_type").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load similarity edges: %w", err)
	}

	// Filter in Go so the excluded radio edges are countable: a radio edge
	// joins the graph only when backbone-significant. NULL significance means
	// the backbone step hasn't stamped that pair; if EVERY radio edge is
	// unstamped, the backbone step evidently failed this cycle — computing on
	// that gutted graph would persist a wholesale reshuffle, so keep the
	// previous partition instead (adversarial finding: "non-fatal" must also
	// cover successfully-computed-on-degraded-input runs). Stamped-but-non-
	// significant is a legitimate state and never aborts.
	alpha := RadioBackboneAlpha()
	edges := make([]WeightedEdge, 0, len(rows))
	radioSeen, radioNull := 0, 0
	for _, r := range rows {
		if r.RelationshipType == catalogm.RelationshipTypeRadioCooccurrence {
			radioSeen++
			if r.BackboneSignificance == nil {
				radioNull++
				continue
			}
			if *r.BackboneSignificance >= alpha {
				continue
			}
		}
		edges = append(edges, WeightedEdge{A: r.SourceArtistID, B: r.TargetArtistID, Weight: r.Score})
	}
	if radioSeen > 0 && radioNull == radioSeen {
		return nil, fmt.Errorf(
			"all %d radio edges have NULL backbone significance — backbone step likely failed this cycle; keeping previous partition",
			radioSeen)
	}
	if radioNull > 0 {
		slog.Warn("artist communities: unstamped radio edges excluded from partition input",
			"null_significance", radioNull, "radio_total", radioSeen)
	}
	return edges, nil
}

// ComputeArtistCommunities rebuilds the persisted community partition.
// Callers treat errors as non-fatal (the previous partition simply stays
// live, mirroring ComputeBackboneSignificance's posture in the cycle).
func (s *RadioService) ComputeArtistCommunities() (CommunityComputeResult, error) {
	var result CommunityComputeResult
	edges, err := s.LoadCommunityGraphEdges()
	if err != nil {
		return result, err
	}

	assignment := LeidenCommunities(edges, LeidenResolution, leidenSeed)

	// Highest-strength member per community anchors the "Around {artist}"
	// label. Strength = sum of incident weights in the SAME filtered graph
	// the partition was computed on. Ties break to the lower artist ID so
	// labels are as stable as the partition.
	strength := make(map[uint]float64, len(assignment))
	for _, e := range edges {
		strength[e.A] += e.Weight
		strength[e.B] += e.Weight
	}
	type communityAgg struct {
		labelArtistID uint
		labelStrength float64
		size          int
	}
	aggs := map[int]*communityAgg{}
	for artistID, comm := range assignment {
		agg, ok := aggs[comm]
		if !ok {
			agg = &communityAgg{}
			aggs[comm] = agg
		}
		agg.size++
		st := strength[artistID]
		if st > agg.labelStrength || (st == agg.labelStrength && (agg.labelArtistID == 0 || artistID < agg.labelArtistID)) {
			agg.labelArtistID = artistID
			agg.labelStrength = st
		}
	}

	now := time.Now().UTC()
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Single-writer guard: overlapping computes (e.g. a future admin
		// trigger racing the ticker) would interleave two swaps. The advisory
		// lock is transaction-scoped and released automatically on commit or
		// rollback. Arbitrary app-unique key.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(7412620001)").Error; err != nil {
			return fmt.Errorf("failed to take community-compute lock: %w", err)
		}

		// DIFF-based swap (adversarial finding): assign only rows whose value
		// actually changes and clear only rows leaving the assigned set —
		// steady-state nightly writes are ~zero instead of two row versions
		// per assigned artist, and the row-lock footprint shrinks to real
		// churn. Raw SQL throughout so artists.updated_at is NOT bumped
		// (repo convention: bookkeeping writes never touch updated_at — see
		// enrich/artist_location.go, discography/importer.go).
		//
		// The swap is atomic for the WRITER; a two-statement reader can still
		// straddle the commit (scene.go therefore reads assignments + labels
		// in ONE statement).
		ids := make([]uint, 0, len(assignment))
		for id := range assignment {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		const batchSize = 500
		for start := 0; start < len(ids); start += batchSize {
			end := start + batchSize
			if end > len(ids) {
				end = len(ids)
			}
			values := ""
			args := make([]any, 0, (end-start)*2)
			for i, id := range ids[start:end] {
				if i > 0 {
					values += ","
				}
				values += "(?::bigint, ?::integer)"
				args = append(args, id, assignment[id])
			}
			q := "UPDATE artists AS a SET community_id = v.community_id " +
				"FROM (VALUES " + values + ") AS v(artist_id, community_id) " +
				"WHERE a.id = v.artist_id " +
				"AND a.community_id IS DISTINCT FROM v.community_id"
			if err := tx.Exec(q, args...).Error; err != nil {
				return fmt.Errorf("failed to assign communities: %w", err)
			}
		}

		// Clear only artists that LEFT the assigned set. Bind-parameter count
		// is fine at catalog scale (~thousands); revisit as a VALUES anti-join
		// if the assigned set ever approaches the 65535 bind limit.
		var clearQ *gorm.DB
		if len(ids) > 0 {
			clearQ = tx.Exec(
				"UPDATE artists SET community_id = NULL WHERE community_id IS NOT NULL AND id NOT IN ?",
				ids)
		} else {
			clearQ = tx.Exec("UPDATE artists SET community_id = NULL WHERE community_id IS NOT NULL")
		}
		if clearQ.Error != nil {
			return fmt.Errorf("failed to clear departed assignments: %w", clearQ.Error)
		}
		result.ClearedArtists = clearQ.RowsAffected

		if err := tx.Exec("DELETE FROM artist_communities").Error; err != nil {
			return fmt.Errorf("failed to clear community labels: %w", err)
		}
		commIDs := make([]int, 0, len(aggs))
		for c := range aggs {
			commIDs = append(commIDs, c)
		}
		sort.Ints(commIDs)
		for _, c := range commIDs {
			agg := aggs[c]
			row := catalogm.ArtistCommunity{
				ID:            uint(c),
				LabelArtistID: agg.labelArtistID,
				MemberCount:   agg.size,
				ComputedAt:    now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("failed to write community %d label: %w", c, err)
			}
		}
		return nil
	})
	if txErr != nil {
		return result, txErr
	}

	result.Communities = len(aggs)
	result.AssignedArtists = len(assignment)
	slog.Info("artist community partition rebuilt",
		"communities", result.Communities,
		"assigned", result.AssignedArtists,
		"previously_assigned", result.ClearedArtists,
		"edges", len(edges))
	return result, nil
}
