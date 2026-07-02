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
	ClearedArtists  int64
}

// ComputeArtistCommunities rebuilds the persisted community partition.
// Callers treat errors as non-fatal (the previous partition simply stays
// live, mirroring ComputeBackboneSignificance's posture in the cycle).
func (s *RadioService) ComputeArtistCommunities() (CommunityComputeResult, error) {
	var result CommunityComputeResult
	if s.db == nil {
		return result, fmt.Errorf("database not initialized")
	}

	type relRow struct {
		SourceArtistID uint    `gorm:"column:source_artist_id"`
		TargetArtistID uint    `gorm:"column:target_artist_id"`
		Score          float64 `gorm:"column:score"`
	}
	var rows []relRow
	err := s.db.Model(&catalogm.ArtistRelationship{}).
		Select("source_artist_id, target_artist_id, score").
		Where("relationship_type <> ? OR backbone_significance < ?",
			catalogm.RelationshipTypeRadioCooccurrence, RadioBackboneAlpha()).
		Where("score > 0").
		Scan(&rows).Error
	if err != nil {
		return result, fmt.Errorf("failed to load similarity edges: %w", err)
	}

	edges := make([]WeightedEdge, 0, len(rows))
	for _, r := range rows {
		edges = append(edges, WeightedEdge{A: r.SourceArtistID, B: r.TargetArtistID, Weight: r.Score})
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
		// Clear, then batch-assign. The whole swap is one transaction, so a
		// reader either sees the old partition or the new one.
		res := tx.Model(&catalogm.Artist{}).
			Where("community_id IS NOT NULL").
			Update("community_id", nil)
		if res.Error != nil {
			return fmt.Errorf("failed to clear previous partition: %w", res.Error)
		}
		result.ClearedArtists = res.RowsAffected

		// Batch UPDATE ... FROM (VALUES ...) — the same shape the backbone
		// writer uses (radio_unmatched.go), batched to keep statements sane.
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
				"WHERE a.id = v.artist_id"
			if err := tx.Exec(q, args...).Error; err != nil {
				return fmt.Errorf("failed to assign communities: %w", err)
			}
		}

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
