package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// defaultReMatchNamePageSize is how many distinct unmatched artist names the
// scheduled rematch ticker processes per DB page — keeps memory bounded vs
// MatchAllUnmatched() loading every unmatched play at once (PSY-1363).
const defaultReMatchNamePageSize = 500

// ReMatchChunkedResult aggregates per-name rematch stats across a paginated sweep.
type ReMatchChunkedResult struct {
	contracts.MatchResult
	NamesProcessed int
}

// UnmatchedArtistNameFilter optionally scopes distinct-name enumeration for chunked rematch.
type UnmatchedArtistNameFilter struct {
	StationID *uint
	ShowID    *uint
}

// GetUnmatchedPlays returns unmatched plays grouped by artist_name,
// optionally filtered by station_id, with pagination.
func (s *RadioService) GetUnmatchedPlays(stationID uint, limit, offset int) ([]*contracts.UnmatchedPlayGroup, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	// Count total distinct artist names
	var total int64
	countQuery := s.db.Table("radio_plays rp").
		Select("COUNT(DISTINCT rp.artist_name)").
		Where("rp.artist_id IS NULL")
	if stationID > 0 {
		countQuery = countQuery.
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}
	countQuery.Scan(&total)

	// Get grouped results
	type groupResult struct {
		ArtistName string `gorm:"column:artist_name"`
		PlayCount  int    `gorm:"column:play_count"`
	}

	groupQuery := s.db.Table("radio_plays rp").
		Select("rp.artist_name, COUNT(*) as play_count").
		Where("rp.artist_id IS NULL")

	if stationID > 0 {
		groupQuery = groupQuery.
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}

	var groups []groupResult
	err := groupQuery.
		Group("rp.artist_name").
		Order("play_count DESC").
		Limit(limit).
		Offset(offset).
		Find(&groups).Error
	if err != nil {
		return nil, 0, fmt.Errorf("querying unmatched plays: %w", err)
	}

	// For each group, get station names and suggested matches
	results := make([]*contracts.UnmatchedPlayGroup, len(groups))
	for i, g := range groups {
		group := &contracts.UnmatchedPlayGroup{
			ArtistName: g.ArtistName,
			PlayCount:  g.PlayCount,
		}

		// Get station names for this artist_name
		type stationResult struct {
			StationName string `gorm:"column:station_name"`
		}
		var stations []stationResult
		s.db.Table("radio_plays rp").
			Select("DISTINCT rs.name as station_name").
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Joins("JOIN radio_stations rs ON rs.id = rsh.station_id").
			Where("rp.artist_name = ? AND rp.artist_id IS NULL", g.ArtistName).
			Find(&stations)

		stationNames := make([]string, len(stations))
		for j, st := range stations {
			stationNames[j] = st.StationName
		}
		group.StationNames = stationNames

		// Get suggested matches (top 3 artists by trigram/name similarity)
		group.SuggestedMatches = s.suggestArtistMatches(g.ArtistName, 3)

		results[i] = group
	}

	return results, total, nil
}

// suggestArtistMatches returns top N artists that match the given name.
// Uses case-insensitive exact match first, then prefix match, then LIKE match.
func (s *RadioService) suggestArtistMatches(artistName string, limit int) []contracts.SuggestedMatch {
	var matches []contracts.SuggestedMatch
	normalizedName := strings.TrimSpace(strings.ToLower(artistName))

	// 1. Exact match (case-insensitive)
	var exactMatches []catalogm.Artist
	s.db.Where("LOWER(name) = ?", normalizedName).Limit(limit).Find(&exactMatches)
	for _, a := range exactMatches {
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   a.ID,
			ArtistName: a.Name,
			ArtistSlug: slug,
		})
	}
	if len(matches) >= limit {
		return matches[:limit]
	}

	// 2. Alias match (case-insensitive)
	remaining := limit - len(matches)
	existingIDs := make(map[uint]bool)
	for _, m := range matches {
		existingIDs[m.ArtistID] = true
	}

	var aliasMatches []struct {
		ArtistID uint   `gorm:"column:artist_id"`
		Name     string `gorm:"column:name"`
		Slug     string `gorm:"column:slug"`
	}
	s.db.Table("artist_aliases aa").
		Select("aa.artist_id, a.name, COALESCE(a.slug, '') as slug").
		Joins("JOIN artists a ON a.id = aa.artist_id").
		Where("LOWER(aa.alias) = ?", normalizedName).
		Limit(remaining).
		Find(&aliasMatches)

	for _, am := range aliasMatches {
		if existingIDs[am.ArtistID] {
			continue
		}
		existingIDs[am.ArtistID] = true
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   am.ArtistID,
			ArtistName: am.Name,
			ArtistSlug: am.Slug,
		})
	}
	if len(matches) >= limit {
		return matches[:limit]
	}

	// 3. LIKE match (prefix)
	remaining = limit - len(matches)
	var likeMatches []catalogm.Artist
	s.db.Where("LOWER(name) LIKE ?", shared.LikePrefixPattern(normalizedName)).
		Limit(remaining).
		Find(&likeMatches)

	for _, a := range likeMatches {
		if existingIDs[a.ID] {
			continue
		}
		existingIDs[a.ID] = true
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   a.ID,
			ArtistName: a.Name,
			ArtistSlug: slug,
		})
	}

	return matches
}

// LinkPlay links a single radio play to an artist/release/label.
func (s *RadioService) LinkPlay(playID uint, req *contracts.LinkPlayRequest) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var play catalogm.RadioPlay
	if err := s.db.First(&play, playID).Error; err != nil {
		return fmt.Errorf("play not found: %w", err)
	}

	updates := make(map[string]interface{})
	if req.ArtistID != nil {
		updates["artist_id"] = *req.ArtistID
	}
	if req.ReleaseID != nil {
		updates["release_id"] = *req.ReleaseID
	}
	if req.LabelID != nil {
		updates["label_id"] = *req.LabelID
	}

	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	return s.db.Model(&play).Updates(updates).Error
}

// BulkLinkPlays links all unmatched plays with a given artist_name to an artist.
func (s *RadioService) BulkLinkPlays(req *contracts.BulkLinkRequest) (*contracts.BulkLinkResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if req.ArtistName == "" {
		return nil, fmt.Errorf("artist_name is required")
	}
	if req.ArtistID == 0 {
		return nil, fmt.Errorf("artist_id is required")
	}

	result := s.db.Model(&catalogm.RadioPlay{}).
		Where("artist_name = ? AND artist_id IS NULL", req.ArtistName).
		Update("artist_id", req.ArtistID)

	if result.Error != nil {
		return nil, fmt.Errorf("bulk linking plays: %w", result.Error)
	}

	return &contracts.BulkLinkResult{
		Updated: int(result.RowsAffected),
	}, nil
}

// ComputeAffinity recomputes the radio_artist_affinity table from scratch.
// It truncates the existing data, then aggregates co-occurrence counts from episodes
// where both plays have matched artist_id values.
func (s *RadioService) ComputeAffinity() error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Truncate existing affinity data
	if err := s.db.Exec("DELETE FROM radio_artist_affinity").Error; err != nil {
		return fmt.Errorf("clearing affinity table: %w", err)
	}

	// Compute co-occurrences: for each episode, find all pairs of matched artists
	// that co-occur, then aggregate across episodes.
	// Uses a self-join on radio_plays within the same episode, with canonical ordering
	// (artist_a_id < artist_b_id) to avoid duplicates.
	query := `
		INSERT INTO radio_artist_affinity (artist_a_id, artist_b_id, co_occurrence_count, show_count, station_count, last_co_occurrence, updated_at)
		SELECT
			LEAST(rp1.artist_id, rp2.artist_id) AS artist_a_id,
			GREATEST(rp1.artist_id, rp2.artist_id) AS artist_b_id,
			COUNT(*) AS co_occurrence_count,
			COUNT(DISTINCT re.show_id) AS show_count,
			COUNT(DISTINCT rsh.station_id) AS station_count,
			MAX(re.air_date) AS last_co_occurrence,
			NOW() AS updated_at
		FROM radio_plays rp1
		JOIN radio_plays rp2 ON rp1.episode_id = rp2.episode_id
			AND rp1.artist_id < rp2.artist_id
		JOIN radio_episodes re ON re.id = rp1.episode_id
		JOIN radio_shows rsh ON rsh.id = re.show_id
		WHERE rp1.artist_id IS NOT NULL
			AND rp2.artist_id IS NOT NULL
		GROUP BY LEAST(rp1.artist_id, rp2.artist_id), GREATEST(rp1.artist_id, rp2.artist_id)
		HAVING COUNT(*) >= 2
	`

	if err := s.db.Exec(query).Error; err != nil {
		return fmt.Errorf("computing affinity: %w", err)
	}

	return nil
}

// backboneUpdateBatch is how many edges go into one `UPDATE ... FROM (VALUES ...)` statement, so the
// backbone pass is a handful of queries instead of one round-trip per edge.
const backboneUpdateBatch = 500

// ComputeBackboneSignificance computes the disparity-filter significance (PSY-1261) for every radio
// co-occurrence edge — over the FULL graph, which the per-node null model requires — and stores it
// in radio_artist_affinity.backbone_significance. It runs in the nightly affinity cycle right AFTER
// ComputeAffinity has repopulated the table. The stored value (the smaller of the two endpoints'
// p-values; lower = stronger) is alpha-INDEPENDENT, so the backbone threshold stays tunable at
// query time without a recompute. Weight = co_occurrence_count (the raw tie strength; the filter
// normalizes per-node, so absolute scale is irrelevant). See catalog.DisparitySignificance.
//
// The column is currently WRITE-ONLY: its consumer — ego/scene graph rendering at a tunable alpha
// (recommended scene-scale alpha 0.10) — is PSY-1293. Until that lands nothing reads it; this
// precompute is cheap + non-fatal, so it's safe to run ahead of the consumer.
func (s *RadioService) ComputeBackboneSignificance() error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	type affinityEdge struct {
		ArtistAID         uint
		ArtistBID         uint
		CoOccurrenceCount int
	}
	var rows []affinityEdge
	if err := s.db.Table("radio_artist_affinity").
		Select("artist_a_id, artist_b_id, co_occurrence_count").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("loading affinity edges: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	edges := make([]WeightedEdge, 0, len(rows))
	for _, r := range rows {
		edges = append(edges, WeightedEdge{A: r.ArtistAID, B: r.ArtistBID, Weight: float64(r.CoOccurrenceCount)})
	}
	sigByEdge := DisparitySignificance(edges)

	keys := make([]EdgeKey, 0, len(sigByEdge))
	for k := range sigByEdge {
		keys = append(keys, k)
	}

	// Batched `UPDATE ... FROM (VALUES ...)`. radio_artist_affinity is canonically ordered
	// (artist_a_id < artist_b_id), matching EdgeKey's (min,max), so a,b map straight to the columns.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < len(keys); i += backboneUpdateBatch {
			end := i + backboneUpdateBatch
			if end > len(keys) {
				end = len(keys)
			}
			chunk := keys[i:end]

			var sb strings.Builder
			sb.WriteString("UPDATE radio_artist_affinity AS r SET backbone_significance = v.sig FROM (VALUES ")
			args := make([]interface{}, 0, len(chunk)*3)
			for j, k := range chunk {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("(?::bigint,?::bigint,?::real)")
				args = append(args, k[0], k[1], sigByEdge[k])
			}
			sb.WriteString(") AS v(a, b, sig) WHERE r.artist_a_id = v.a AND r.artist_b_id = v.b")

			if err := tx.Exec(sb.String(), args...).Error; err != nil {
				return fmt.Errorf("updating backbone significance (batch at %d): %w", i, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	slog.Info("radio backbone significance computed",
		"edges", len(edges),
		"nodes", countNodes(edges))
	return nil
}

// countNodes returns the number of distinct artists touched by the edge set (for logging).
func countNodes(edges []WeightedEdge) int {
	seen := make(map[uint]struct{}, len(edges))
	for _, e := range edges {
		seen[e.A] = struct{}{}
		seen[e.B] = struct{}{}
	}
	return len(seen)
}

// SyncAffinityToRelationships syncs radio_artist_affinity rows into artist_relationships
// as radio_cooccurrence relationships. Creates new, updates existing, and deletes stale relationships.
func (s *RadioService) SyncAffinityToRelationships() (*contracts.SyncAffinityResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.SyncAffinityResult{}

	// 1. Query all affinity pairs with co_occurrence_count >= 2
	//    (the ComputeAffinity query already filters >= 2, but be safe)
	var affinities []catalogm.RadioArtistAffinity
	if err := s.db.Where("co_occurrence_count >= 2").Find(&affinities).Error; err != nil {
		return nil, fmt.Errorf("querying affinity data: %w", err)
	}

	// Build a set of affinity pairs for stale detection
	affinityPairs := make(map[[2]uint]bool, len(affinities))

	for _, aff := range affinities {
		// artist_a_id < artist_b_id is enforced by the affinity table CHECK constraint
		pair := [2]uint{aff.ArtistAID, aff.ArtistBID}
		affinityPairs[pair] = true

		// Compute normalized score: min(1.0, co_occurrence_count / 50.0)
		score := float32(aff.CoOccurrenceCount) / 50.0
		if score > 1.0 {
			score = 1.0
		}

		// Cross-station multiplier: if station_count > 1, multiply by 1.5 (cap at 1.0)
		if aff.StationCount > 1 {
			score *= 1.5
			if score > 1.0 {
				score = 1.0
			}
		}

		// Build JSONB detail
		detail, _ := json.Marshal(map[string]interface{}{
			"co_occurrence_count": aff.CoOccurrenceCount,
			"station_count":       aff.StationCount,
			"show_count":          aff.ShowCount,
		})
		detailRaw := json.RawMessage(detail)

		// Upsert into artist_relationships
		var existing catalogm.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			aff.ArtistAID, aff.ArtistBID, catalogm.RelationshipTypeRadioCooccurrence).
			First(&existing).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			rel := &catalogm.ArtistRelationship{
				SourceArtistID:   aff.ArtistAID,
				TargetArtistID:   aff.ArtistBID,
				RelationshipType: catalogm.RelationshipTypeRadioCooccurrence,
				Score:            score,
				AutoDerived:      true,
				Detail:           &detailRaw,
				// Denormalize the disparity-filter significance (PSY-1293) so the scene + ego graph
				// endpoints can filter on it at query time. nil (significance not yet computed for
				// this edge) persists as NULL = not in the scene backbone.
				BackboneSignificance: aff.BackboneSignificance,
			}
			if createErr := s.db.Create(rel).Error; createErr != nil {
				slog.Error("radio affinity sync: failed to create relationship",
					"source_artist_id", aff.ArtistAID,
					"target_artist_id", aff.ArtistBID,
					"error", createErr)
				result.Failed++
			} else {
				result.Created++
			}
		case err == nil:
			if updateErr := s.db.Model(&existing).Updates(map[string]interface{}{
				"score":  score,
				"detail": &detailRaw,
				// Re-sync the disparity-filter significance (PSY-1293); a nil pointer (significance
				// not computed this cycle) clears the column back to NULL via the map update.
				"backbone_significance": aff.BackboneSignificance,
			}).Error; updateErr != nil {
				slog.Error("radio affinity sync: failed to update relationship",
					"source_artist_id", aff.ArtistAID,
					"target_artist_id", aff.ArtistBID,
					"error", updateErr)
				result.Failed++
			} else {
				result.Updated++
			}
		default:
			// A real lookup error (not ErrRecordNotFound) was previously
			// swallowed, silently skipping the pair.
			slog.Error("radio affinity sync: failed to look up existing relationship",
				"source_artist_id", aff.ArtistAID,
				"target_artist_id", aff.ArtistBID,
				"error", err)
			result.Failed++
		}
	}

	// 2. Delete stale radio_cooccurrence relationships that no longer exist in affinity table
	var staleRels []catalogm.ArtistRelationship
	// A failed query aborts the sync (vs. the per-row failures above, which are
	// counted in result.Failed and skipped): without the stale set, cleanup can't run.
	if err := s.db.Where("relationship_type = ? AND auto_derived = TRUE", catalogm.RelationshipTypeRadioCooccurrence).
		Find(&staleRels).Error; err != nil {
		slog.Error("radio affinity sync: failed to query stale relationships", "error", err)
		return result, fmt.Errorf("querying stale relationships: %w", err)
	}

	for _, rel := range staleRels {
		pair := [2]uint{rel.SourceArtistID, rel.TargetArtistID}
		if !affinityPairs[pair] {
			if err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
				rel.SourceArtistID, rel.TargetArtistID, catalogm.RelationshipTypeRadioCooccurrence).
				Delete(&catalogm.ArtistRelationship{}).Error; err != nil {
				slog.Error("radio affinity sync: failed to delete stale relationship",
					"source_artist_id", rel.SourceArtistID,
					"target_artist_id", rel.TargetArtistID,
					"error", err)
				result.Failed++
			} else {
				result.Deleted++
			}
		}
	}

	return result, nil
}

// ReMatchUnmatched re-runs the matching engine on all plays where artist_id IS NULL.
// This catches newly added artists since the last match attempt.
func (s *RadioService) ReMatchUnmatched() (*contracts.MatchResult, error) {
	return s.ReMatchUnmatchedWithFilter(nil)
}

// listUnmatchedArtistNamesPage returns the next page of distinct unmatched
// artist_name values in ascending order. afterName is the cursor (empty for the
// first page); uses keyset pagination so large archives don't pay OFFSET cost.
func (s *RadioService) listUnmatchedArtistNamesPage(afterName string, limit int, filter UnmatchedArtistNameFilter) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = defaultReMatchNamePageSize
	}

	type nameRow struct {
		ArtistName string `gorm:"column:artist_name"`
	}
	var rows []nameRow
	q := s.db.Table("radio_plays rp").
		Select("DISTINCT rp.artist_name").
		Where("rp.artist_id IS NULL")
	if filter.ShowID != nil {
		q = q.Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Where("re.show_id = ?", *filter.ShowID)
	} else if filter.StationID != nil {
		q = q.Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", *filter.StationID)
	}
	if afterName != "" {
		q = q.Where("rp.artist_name > ?", afterName)
	}
	err := q.Order("rp.artist_name ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing unmatched artist names: %w", err)
	}

	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.ArtistName
	}
	return names, nil
}

// ReMatchUnmatchedChunked rematches unmatched plays by sweeping distinct
// artist_name values in bounded pages. ctx is checked between pages so a
// shutdown can abandon a long sweep without waiting for the full archive.
// When runID is non-zero, progress is written to the radio_sync_runs row and
// isSyncRunCancelled is polled between names (PSY-1364).
func (s *RadioService) ReMatchUnmatchedChunked(ctx context.Context, pageSize int, filter UnmatchedArtistNameFilter, runID uint) (*ReMatchChunkedResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if pageSize <= 0 {
		pageSize = defaultReMatchNamePageSize
	}

	agg := &ReMatchChunkedResult{}
	var afterName string
	var sinceLastProgress int

	for {
		if err := ctx.Err(); err != nil {
			return agg, err
		}
		if runID != 0 && s.isSyncRunCancelled(runID) {
			return agg, context.Canceled
		}

		names, err := s.listUnmatchedArtistNamesPage(afterName, pageSize, filter)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			break
		}

		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return agg, err
			}
			if runID != 0 && s.isSyncRunCancelled(runID) {
				return agg, context.Canceled
			}
			result, err := s.ReMatchUnmatchedWithFilter(&contracts.ReMatchRequest{ArtistName: name})
			if err != nil {
				return nil, err
			}
			agg.Total += result.Total
			agg.Matched += result.Matched
			agg.Unmatched += result.Unmatched
			agg.PersistErrors += result.PersistErrors
			agg.NamesProcessed++

			if runID != 0 {
				sinceLastProgress++
				if sinceLastProgress >= 10 {
					sinceLastProgress = 0
					s.writeRematchRunProgress(runID, agg)
				}
			}
		}

		afterName = names[len(names)-1]
		if len(names) < pageSize {
			break
		}
	}

	return agg, nil
}

// ReMatchUnmatchedWithFilter rematches unmatched plays, optionally scoped to a
// single artist or label name. A nil or empty request rematches everything.
func (s *RadioService) ReMatchUnmatchedWithFilter(req *contracts.ReMatchRequest) (*contracts.MatchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	matcher := NewRadioMatchingEngine(s.db)
	if req == nil {
		return matcher.MatchAllUnmatched()
	}
	if req.ArtistName != "" {
		return matcher.MatchUnmatchedPlaysForArtistName(req.ArtistName)
	}
	if req.LabelName != "" {
		return matcher.MatchUnmatchedPlaysForLabelName(req.LabelName)
	}
	return matcher.MatchAllUnmatched()
}

// ScheduleRematchForArtistName asynchronously rematches unmatched plays for one
// artist name. Safe to call from entity-create hooks after commit.
func (s *RadioService) ScheduleRematchForArtistName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	n := name
	shared.GoSafe(context.Background(), "radio_rematch_artist", func() {
		result, err := s.ReMatchUnmatchedWithFilter(&contracts.ReMatchRequest{ArtistName: n})
		if err != nil {
			slog.Error("radio rematch for artist name failed", "artist_name", n, "error", err)
			return
		}
		if result.Matched > 0 {
			slog.Info("radio rematch for artist name linked plays",
				"artist_name", n, "matched", result.Matched, "total", result.Total)
		}
	})
}

// ScheduleRematchForLabelName asynchronously rematches unmatched plays for one
// label name. Safe to call from entity-create hooks after commit.
func (s *RadioService) ScheduleRematchForLabelName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	n := name
	shared.GoSafe(context.Background(), "radio_rematch_label", func() {
		result, err := s.ReMatchUnmatchedWithFilter(&contracts.ReMatchRequest{LabelName: n})
		if err != nil {
			slog.Error("radio rematch for label name failed", "label_name", n, "error", err)
			return
		}
		if result.Matched > 0 {
			slog.Info("radio rematch for label name linked plays",
				"label_name", n, "matched", result.Matched, "total", result.Total)
		}
	})
}

// GetActiveStationsWithPlaylistSource returns all active stations that have an
// AUTOMATED playlist provider (an actual scraper/API source). 'manual'
// (hand-curated, no provider) and empty/NULL (link-only) are excluded so the
// scheduled discover/fetch cycle never dispatches a station getProvider can't
// serve — which would otherwise log a permanent failure and trip the circuit
// breaker every cycle for a deliberately-configured manual station. (PSY-927)
func (s *RadioService) GetActiveStationsWithPlaylistSource() ([]catalogm.RadioStation, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var stations []catalogm.RadioStation
	err := s.db.
		Where("is_active = TRUE AND playlist_source IS NOT NULL AND playlist_source != '' AND playlist_source != ?", catalogm.PlaylistSourceManual).
		Find(&stations).Error
	if err != nil {
		return nil, fmt.Errorf("querying active stations: %w", err)
	}
	return stations, nil
}

// Note (PSY-1269): the legacy RecordFetchSuccess/RecordFetchFailure helpers were
// removed here — they were dead (no callers, not on the contracts interface) and
// RecordFetchSuccess advanced last_playlist_fetch_at ungated, the exact watermark
// write site the unify-the-gate change set out to close. The live path advances the
// watermark only via advanceLastFetch (radio_import.go).
