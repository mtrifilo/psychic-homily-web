package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ArtistRelationshipService handles artist relationship business logic.
type ArtistRelationshipService struct {
	db *gorm.DB
}

// NewArtistRelationshipService creates a new artist relationship service.
func NewArtistRelationshipService(database *gorm.DB) *ArtistRelationshipService {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistRelationshipService{db: database}
}

// ──────────────────────────────────────────────
// CRUD
// ──────────────────────────────────────────────

// CreateRelationship creates a new artist relationship with canonical ordering.
func (s *ArtistRelationshipService) CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*catalogm.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if sourceID == targetID {
		return nil, fmt.Errorf("cannot create relationship between an artist and itself")
	}

	src, tgt := catalogm.CanonicalOrder(sourceID, targetID)

	// Check for existing
	var existing catalogm.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("relationship already exists between artists %d and %d (type: %s)", src, tgt, relType)
	}

	rel := &catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		AutoDerived:      autoDerived,
	}

	if err := s.db.Create(rel).Error; err != nil {
		return nil, fmt.Errorf("failed to create relationship: %w", err)
	}

	return rel, nil
}

// GetRelationship retrieves a relationship between two artists (order-independent).
func (s *ArtistRelationshipService) GetRelationship(artistA, artistB uint, relType string) (*catalogm.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	var rel catalogm.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get relationship: %w", err)
	}

	return &rel, nil
}

// GetRelatedArtists returns artists related to the given artist with vote counts.
// Pass relType="" to get all types. Results sorted by score descending.
func (s *ArtistRelationshipService) GetRelatedArtists(artistID uint, relType string, limit int) ([]contracts.RelatedArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 30
	}

	// Query both directions
	query := s.db.Model(&catalogm.ArtistRelationship{}).
		Where("source_artist_id = ? OR target_artist_id = ?", artistID, artistID)

	if relType != "" {
		query = query.Where("relationship_type = ?", relType)
	}

	query = query.Order("score DESC").Limit(limit)

	var rels []catalogm.ArtistRelationship
	if err := query.Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("failed to get related artists: %w", err)
	}

	// Batched vote counts (PSY-1301) — every rel here has artistID as one
	// endpoint, so the node set is the center plus each rel's other side.
	voteIDs := make([]uint, 0, len(rels)+1)
	voteIDs = append(voteIDs, artistID)
	for _, rel := range rels {
		if rel.SourceArtistID == artistID {
			voteIDs = append(voteIDs, rel.TargetArtistID)
		} else {
			voteIDs = append(voteIDs, rel.SourceArtistID)
		}
	}
	voteCounts := s.getVoteCountsAmong(voteIDs)

	responses := make([]contracts.RelatedArtistResponse, 0, len(rels))
	for _, rel := range rels {
		// Determine the "other" artist
		otherID := rel.TargetArtistID
		if otherID == artistID {
			otherID = rel.SourceArtistID
		}

		// Fetch artist info
		var artist catalogm.Artist
		if err := s.db.First(&artist, otherID).Error; err != nil {
			continue
		}

		slug := ""
		if artist.Slug != nil {
			slug = *artist.Slug
		}

		votes := voteCounts[voteCountKey{rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType}]

		resp := contracts.RelatedArtistResponse{
			ArtistID:         otherID,
			Name:             artist.Name,
			Slug:             slug,
			RelationshipType: rel.RelationshipType,
			Score:            rel.Score,
			Upvotes:          votes.up,
			Downvotes:        votes.down,
			WilsonScore:      rel.WilsonScore(votes.up, votes.down),
			AutoDerived:      rel.AutoDerived,
		}

		responses = append(responses, resp)
	}

	return responses, nil
}

// DeleteRelationship deletes a relationship and its votes.
func (s *ArtistRelationshipService) DeleteRelationship(artistA, artistB uint, relType string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete votes first
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&catalogm.ArtistRelationshipVote{})

		result := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&catalogm.ArtistRelationship{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete relationship: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("relationship not found")
		}
		return nil
	})
}

// ──────────────────────────────────────────────
// Voting
// ──────────────────────────────────────────────

// Vote adds or updates a vote on an artist relationship.
func (s *ArtistRelationshipService) Vote(artistA, artistB uint, relType string, userID uint, isUpvote bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	// Verify relationship exists
	var rel catalogm.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("relationship not found between artists %d and %d (type: %s)", src, tgt, relType)
		}
		return fmt.Errorf("failed to verify relationship: %w", err)
	}

	direction := int16(-1)
	if isUpvote {
		direction = 1
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert vote
		var existing catalogm.ArtistRelationshipVote
		err := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).First(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			vote := catalogm.ArtistRelationshipVote{
				SourceArtistID:   src,
				TargetArtistID:   tgt,
				RelationshipType: relType,
				UserID:           userID,
				Direction:        direction,
			}
			if err := tx.Create(&vote).Error; err != nil {
				return fmt.Errorf("failed to create vote: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check existing vote: %w", err)
		} else {
			if err := tx.Model(&existing).Update("direction", direction).Error; err != nil {
				return fmt.Errorf("failed to update vote: %w", err)
			}
		}

		// Recalculate score
		return s.recalculateScore(tx, src, tgt, relType)
	})
}

// RemoveVote removes a user's vote on an artist relationship.
func (s *ArtistRelationshipService) RemoveVote(artistA, artistB uint, relType string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).Delete(&catalogm.ArtistRelationshipVote{})

		return s.recalculateScore(tx, src, tgt, relType)
	})
}

// GetUserVote returns a user's vote on a relationship, or nil if not voted.
func (s *ArtistRelationshipService) GetUserVote(artistA, artistB uint, relType string, userID uint) (*catalogm.ArtistRelationshipVote, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	var vote catalogm.ArtistRelationshipVote
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
		src, tgt, relType, userID).First(&vote).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}

	return &vote, nil
}

// ──────────────────────────────────────────────
// Graph
// ──────────────────────────────────────────────

// festivalCobillType is the edge type identifier for the query-time
// festival co-lineup signal (PSY-363). It is *not* a stored relationship
// row in artist_relationships — the edges are derived at query time via
// JOIN on festival_artists. Defined here (rather than in the models
// package) on purpose, so it cannot be accidentally written to the table.
const festivalCobillType = "festival_cobill"

// festivalCobillTopN is the normalisation cap for the festival co-bill
// score. With min(count/N, 1.0), three shared festivals = max signal.
// Festivals are sparser than shared shows (most artists have 1-3 festival
// appearances), so this is intentionally lower than the shared_bills cap
// of 10 set in similar-artists.md.
const festivalCobillTopN = 3.0

// festivalCobillRecencyBoostYears: if MAX(festivals.start_date) is within
// this many years of "now", apply the same 1.2x recency boost shared_bills
// uses (see DeriveSharedBills). Festivals are annual, so the equivalent of
// shared_bills' "<3 months" window is "<2 years" here.
const festivalCobillRecencyBoostYears = 2

// festivalCobillRecencyBoost is the multiplier applied to the score when
// the most recent shared festival is within the recency window.
const festivalCobillRecencyBoost = 1.2

// festivalCobillCenterLimit caps the number of festival_cobill edges
// derived from the center artist. Matches the 30-edge cap used elsewhere
// in GetArtistGraph so the default 30-node budget is respected.
const festivalCobillCenterLimit = 30

// festivalCobillTopFestivalNames is the number of representative festival
// names to surface in the edge's `detail` JSONB for tooltip rendering.
const festivalCobillTopFestivalNames = 3

// egoBackboneUnionLimit caps how many disparity-filter backbone radio edges (PSY-1293) are
// UNIONed onto the center artist's top-k score-ranked edges. The top-k cap (Limit(30)) can drop
// niche-but-significant radio co-occurrence links; the backbone surfaces them so a mid-degree ego
// is never reduced to only its loudest neighbors. Capped (and taken strongest-first) so a hub can
// never re-introduce the hairball the PSY-1258 cap exists to prevent; matches the 30-edge budget.
const egoBackboneUnionLimit = 30

// egoCrossRadioPerNodeCap bounds step-7 RADIO cross edges to the top-K per node, kept when the
// edge is top-K for EITHER endpoint (PSY-1301). Radio co-occurrence is the only cross-edge type
// dense enough to need a server bound (a 30-60-node related set can store hundreds of pairs);
// other types stay unbounded — they are sparse by nature AND the client renders them uncapped
// (edgeCap.ts caps only radio_cooccurrence), so any server truncation of them would be visible.
// The either-endpoint rule mirrors the client's PSY-1258 cap semantics (a niche node's few
// strong links survive a hub neighbor), and K=10 is a superset of the client's k=5 — up to
// tie-ordering: the server breaks score ties by partner ID while the client breaks them by
// payload order, so within a tie group straddling the boundary a same-score sibling may render
// instead of the exact edge the uncapped payload would have picked. Deliberately NOT a backbone
// alpha filter: the PSY-1261 tuning showed the scene-scale alpha empties mid-degree ego link
// sets (Cola: 2/28 at alpha=0.10) — and those emptied links ARE these cross edges — plus a
// NULL-significance deploy window would star-ify every ego. See disparity_filter.go.
const egoCrossRadioPerNodeCap = 10

// festivalCobillRow is the result of the festival co-occurrence query.
// We capture both the canonical pair (artistA<artistB), the count, and
// the start date of the most recent shared festival (for recency-weighted
// scoring + tooltip).
type festivalCobillRow struct {
	ArtistA         uint
	ArtistB         uint
	SharedCount     int
	MostRecentStart *time.Time
	MostRecentYear  *int
}

// GetArtistGraph returns the relationship graph for an artist (depth 1).
// types filters by relationship type; empty slice means all types.
// userID > 0 includes user vote data; 0 means no user context.
// Returns max 30 nodes sorted by combined score.
func (s *ArtistRelationshipService) GetArtistGraph(artistID uint, types []string, userID uint) (*contracts.ArtistGraph, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// 1. Get center artist details
	var centerArtist catalogm.Artist
	if err := s.db.First(&centerArtist, artistID).Error; err != nil {
		return nil, apperrors.ErrArtistNotFound(artistID)
	}

	centerSlug := ""
	if centerArtist.Slug != nil {
		centerSlug = *centerArtist.Slug
	}

	// Count upcoming shows for center
	var centerShowCount int64
	s.db.Table("show_artists").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id = ? AND shows.status = 'approved' AND shows.event_date > NOW()", artistID).
		Count(&centerShowCount)

	centerCity := ""
	if centerArtist.City != nil {
		centerCity = *centerArtist.City
	}
	centerState := ""
	if centerArtist.State != nil {
		centerState = *centerArtist.State
	}

	graph := &contracts.ArtistGraph{
		Center: contracts.ArtistGraphNode{
			ID:                centerArtist.ID,
			Name:              centerArtist.Name,
			Slug:              centerSlug,
			City:              centerCity,
			State:             centerState,
			UpcomingShowCount: int(centerShowCount),
		},
		Nodes: []contracts.ArtistGraphNode{},
		Links: []contracts.ArtistGraphLink{},
	}

	// 2. Get all stored relationships for this artist (depth 1)
	// Note: festival_cobill is NOT a stored type — it is filtered out of
	// this query and computed separately from festival_artists below.
	storedTypes := filterOutQueryTimeTypes(types)
	wantFestivalCobill := isQueryTimeTypeRequested(types, festivalCobillType)

	var rels []catalogm.ArtistRelationship
	// If types was non-empty AND every requested type was a query-time
	// derived type, storedTypes will be empty here — skip the stored-rels
	// query entirely. Otherwise (empty input = "all STORED types", or any
	// stored type requested) run the normal query. Empty input never pulls
	// in query-time types (festival_cobill) — those are opt-in (PSY-954).
	if len(types) == 0 || len(storedTypes) > 0 {
		query := s.db.Model(&catalogm.ArtistRelationship{}).
			Where("source_artist_id = ? OR target_artist_id = ?", artistID, artistID)

		if len(storedTypes) > 0 {
			query = query.Where("relationship_type IN ?", storedTypes)
		}

		query = query.Order("score DESC").Limit(30)

		if err := query.Find(&rels).Error; err != nil {
			return nil, fmt.Errorf("failed to get relationships: %w", err)
		}
	}

	// 2b. Compute query-time festival_cobill center edges (PSY-363).
	// These are derived from festival_artists at request time; no stored
	// row is read or written. Skipped when the type filter excludes it.
	var festivalCobillLinks []contracts.ArtistGraphLink
	if wantFestivalCobill {
		links, err := s.computeFestivalCobillCenterEdges(artistID, festivalCobillCenterLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to compute festival co-lineup edges: %w", err)
		}
		festivalCobillLinks = links
	}

	// 2c. UNION the center artist's disparity-filter backbone radio edges (PSY-1293) onto the
	// score-ranked top-k from step 2. The top-k cap can drop niche-but-significant radio
	// co-occurrence links; the backbone (Serrano 2009, computed on the global co-occurrence graph)
	// surfaces them so a mid-degree ego is never emptied of its meaningful neighbors. Only runs when
	// radio co-occurrence is in scope (empty filter = all stored types, or an explicit request).
	// Deduped against the top-k set by (source,target,type); bounded by egoBackboneUnionLimit and
	// taken strongest-first so a hub can't re-introduce the hairball. Non-fatal: on error the ego
	// degrades to just the top-k floor (never worse than pre-PSY-1293).
	wantRadioCooccurrence := len(types) == 0
	for _, t := range storedTypes {
		if t == catalogm.RelationshipTypeRadioCooccurrence {
			wantRadioCooccurrence = true
			break
		}
	}
	if wantRadioCooccurrence {
		alpha := RadioBackboneAlpha()
		var backboneRels []catalogm.ArtistRelationship
		bbErr := s.db.Model(&catalogm.ArtistRelationship{}).
			Where("(source_artist_id = ? OR target_artist_id = ?)", artistID, artistID).
			Where("relationship_type = ?", catalogm.RelationshipTypeRadioCooccurrence).
			Where("backbone_significance IS NOT NULL AND backbone_significance < ?", alpha).
			Order("backbone_significance ASC").
			Limit(egoBackboneUnionLimit).
			Find(&backboneRels).Error
		if bbErr != nil {
			slog.Warn("artist graph: backbone union query failed; falling back to top-k floor",
				"artist_id", artistID, "error", bbErr)
		} else {
			type relKey struct {
				src, tgt uint
				typ      string
			}
			seen := make(map[relKey]bool, len(rels))
			for _, r := range rels {
				seen[relKey{r.SourceArtistID, r.TargetArtistID, r.RelationshipType}] = true
			}
			for _, r := range backboneRels {
				k := relKey{r.SourceArtistID, r.TargetArtistID, r.RelationshipType}
				if !seen[k] {
					rels = append(rels, r)
					seen[k] = true
				}
			}
		}
	}

	if len(rels) == 0 && len(festivalCobillLinks) == 0 {
		return graph, nil
	}

	// Collect related artist IDs from BOTH stored relationships and the
	// query-time festival_cobill edges.
	relatedIDSet := make(map[uint]bool)
	for _, rel := range rels {
		otherID := rel.TargetArtistID
		if otherID == artistID {
			otherID = rel.SourceArtistID
		}
		relatedIDSet[otherID] = true
	}
	for _, link := range festivalCobillLinks {
		otherID := link.TargetID
		if otherID == artistID {
			otherID = link.SourceID
		}
		relatedIDSet[otherID] = true
	}

	relatedIDs := make([]uint, 0, len(relatedIDSet))
	for id := range relatedIDSet {
		relatedIDs = append(relatedIDs, id)
	}

	// 3. Fetch artist details for all related artists
	var relatedArtists []catalogm.Artist
	if err := s.db.Where("id IN ?", relatedIDs).Find(&relatedArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to get related artist details: %w", err)
	}

	artistMap := make(map[uint]catalogm.Artist)
	for _, a := range relatedArtists {
		artistMap[a.ID] = a
	}

	// 4. Count upcoming shows per related artist (batch query)
	type showCountRow struct {
		ArtistID  uint
		ShowCount int64
	}
	var showCounts []showCountRow
	s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) as show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = 'approved' AND shows.event_date > NOW()", relatedIDs).
		Group("show_artists.artist_id").
		Scan(&showCounts)

	showCountMap := make(map[uint]int)
	for _, sc := range showCounts {
		showCountMap[sc.ArtistID] = int(sc.ShowCount)
	}

	// 5. Build nodes
	for _, id := range relatedIDs {
		a, ok := artistMap[id]
		if !ok {
			continue
		}

		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		city := ""
		if a.City != nil {
			city = *a.City
		}
		state := ""
		if a.State != nil {
			state = *a.State
		}

		graph.Nodes = append(graph.Nodes, contracts.ArtistGraphNode{
			ID:                a.ID,
			Name:              a.Name,
			Slug:              slug,
			City:              city,
			State:             state,
			UpcomingShowCount: showCountMap[a.ID],
		})
	}

	// 5b. Batch the vote counts for every edge steps 6 + 7 can emit — one grouped
	// query over the node set instead of two COUNTs per edge (PSY-1301). A dense
	// ego (30-60 nodes after the PSY-1293 union) previously issued hundreds of
	// sequential queries here.
	voteIDs := append([]uint{artistID}, relatedIDs...)
	voteCounts := s.getVoteCountsAmong(voteIDs)

	// 6. Build links from center relationships
	for _, rel := range rels {
		votes := voteCounts[voteCountKey{rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType}]

		var detail interface{}
		if rel.Detail != nil {
			_ = json.Unmarshal(*rel.Detail, &detail)
		}

		graph.Links = append(graph.Links, contracts.ArtistGraphLink{
			SourceID:  rel.SourceArtistID,
			TargetID:  rel.TargetArtistID,
			Type:      rel.RelationshipType,
			Score:     float64(rel.Score),
			VotesUp:   votes.up,
			VotesDown: votes.down,
			Detail:    detail,
		})
	}

	// 6b. Append the query-time festival_cobill center edges (PSY-363).
	graph.Links = append(graph.Links, festivalCobillLinks...)

	// 7. Get cross-connections between related artists. Two queries by design
	// (PSY-1301): non-radio types are fetched unbounded exactly as before (sparse
	// by nature; the client renders them uncapped, so any server truncation of
	// them would be visible), while radio co-occurrence — the only type dense
	// enough to blow up the payload — is bounded to the per-node top-K with
	// either-endpoint semantics. No backbone alpha here: see the
	// egoCrossRadioPerNodeCap doc-comment.
	//
	// Type-filter note: when only query-time types (festival_cobill) are
	// requested, storedTypes is empty and this query returns every stored
	// NON-radio cross type — a pre-existing leak (the client filters by active
	// types anyway). Radio is the deliberate exception since PSY-1301: it only
	// joins the cross set when wantRadioCooccurrence, narrowing that leak.
	if len(relatedIDs) > 1 {
		var crossRels []catalogm.ArtistRelationship

		// Skip the non-radio query when radio is the ONLY requested stored
		// type — `type <> radio AND type IN (radio)` can never match.
		radioOnly := len(storedTypes) == 1 && storedTypes[0] == catalogm.RelationshipTypeRadioCooccurrence
		if !radioOnly {
			crossQuery := s.db.Model(&catalogm.ArtistRelationship{}).
				Where("source_artist_id IN ? AND target_artist_id IN ?", relatedIDs, relatedIDs).
				Where("relationship_type <> ?", catalogm.RelationshipTypeRadioCooccurrence)

			if len(storedTypes) > 0 {
				crossQuery = crossQuery.Where("relationship_type IN ?", storedTypes)
			}

			// Errors degrade to fewer cross edges (pre-existing behavior),
			// but are logged so a half-empty graph is diagnosable.
			if err := crossQuery.Find(&crossRels).Error; err != nil {
				slog.Warn("artist graph: cross-edge query failed; rendering without non-radio cross edges",
					"artist_id", artistID, "error", err)
			}
		}

		if wantRadioCooccurrence {
			// Each edge is ranked once per ENDPOINT (the UNION ALL explode) —
			// partitioning by the stored source/target columns alone would rank
			// per canonical ROLE and miss half a node's edges. Ties broken by
			// the partner ID so the kept set is deterministic across requests
			// (score ties are common in the bucketed co-occurrence scores).
			// total_count rides along on every row so cap truncation is
			// observable without a second query (AC: flag or slog).
			type radioCrossRow struct {
				catalogm.ArtistRelationship
				TotalCount int `gorm:"column:total_count"`
			}
			var radioRows []radioCrossRow
			radioErr := s.db.Raw(`
				WITH radio AS (
					SELECT * FROM artist_relationships
					WHERE relationship_type = ?
						AND source_artist_id IN ?
						AND target_artist_id IN ?
				),
				endpoint_ranks AS (
					SELECT source_artist_id, target_artist_id,
						ROW_NUMBER() OVER (PARTITION BY node ORDER BY score DESC, source_artist_id, target_artist_id) AS rn
					FROM (
						SELECT source_artist_id, target_artist_id, score, source_artist_id AS node FROM radio
						UNION ALL
						SELECT source_artist_id, target_artist_id, score, target_artist_id AS node FROM radio
					) exploded
				)
				SELECT r.*, (SELECT COUNT(*) FROM radio) AS total_count FROM radio r
				WHERE EXISTS (
					SELECT 1 FROM endpoint_ranks er
					WHERE er.source_artist_id = r.source_artist_id
						AND er.target_artist_id = r.target_artist_id
						AND er.rn <= ?
				)`,
				catalogm.RelationshipTypeRadioCooccurrence, relatedIDs, relatedIDs,
				egoCrossRadioPerNodeCap).
				Scan(&radioRows).Error
			if radioErr != nil {
				slog.Warn("artist graph: radio cross-edge query failed; rendering without radio cross edges",
					"artist_id", artistID, "error", radioErr)
			}
			if len(radioRows) > 0 && radioRows[0].TotalCount > len(radioRows) {
				slog.Info("artist graph: radio cross edges capped to per-node top-K",
					"artist_id", artistID, "stored", radioRows[0].TotalCount,
					"kept", len(radioRows), "per_node_cap", egoCrossRadioPerNodeCap)
			}
			for _, r := range radioRows {
				crossRels = append(crossRels, r.ArtistRelationship)
			}
		}

		for _, rel := range crossRels {
			votes := voteCounts[voteCountKey{rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType}]

			var detail interface{}
			if rel.Detail != nil {
				_ = json.Unmarshal(*rel.Detail, &detail)
			}

			graph.Links = append(graph.Links, contracts.ArtistGraphLink{
				SourceID:  rel.SourceArtistID,
				TargetID:  rel.TargetArtistID,
				Type:      rel.RelationshipType,
				Score:     float64(rel.Score),
				VotesUp:   votes.up,
				VotesDown: votes.down,
				Detail:    detail,
			})
		}

		// 7b. Festival co-lineup cross-edges between related artists (PSY-363).
		// Same query-time JOIN, scoped to the related-artist pool.
		if wantFestivalCobill {
			crossLinks, err := s.computeFestivalCobillCrossEdges(artistID, relatedIDs)
			if err == nil {
				graph.Links = append(graph.Links, crossLinks...)
			}
		}
	}

	// 8. Include user votes if authenticated
	if userID > 0 {
		graph.UserVotes = make(map[string]string)
		for _, link := range graph.Links {
			vote, err := s.GetUserVote(link.SourceID, link.TargetID, link.Type, userID)
			if err == nil && vote != nil {
				key := fmt.Sprintf("%d-%d-%s", link.SourceID, link.TargetID, link.Type)
				if vote.Direction == 1 {
					graph.UserVotes[key] = "up"
				} else {
					graph.UserVotes[key] = "down"
				}
			}
		}
	}

	return graph, nil
}

// ──────────────────────────────────────────────
// Bill composition (PSY-364)
// ──────────────────────────────────────────────

const (
	billCompositionMinShows  = 3  // hide section entirely below this many shows
	billCompositionTopRows   = 10 // top N for opens-with / closes-with tables
	billCompositionMaxNodes  = 30 // mini-graph node cap (matches GetArtistGraph)
	billCompositionMaxMonths = 24 // bound on time filter
)

// billStatsRow is the shape of the stats query.
type billStatsRow struct {
	TotalShows     int
	HeadlinerCount int
	OpenerCount    int
}

// billCoBillRow is the shape of the co-bill aggregation query, one row per
// (co-artist, role pair).
type billCoBillRow struct {
	CoArtistID  uint
	ARole       string // 'headliner' | 'opener'
	CoRole      string
	SharedCount int
	LastShared  time.Time
}

// GetArtistBillComposition returns an artist's bill-slot stats and top co-bill artists,
// optionally filtered to the last `months` months (months=0 means all-time).
//
// The endpoint mirrors the shape of GetArtistGraph but is derived live from show_artists
// rather than the stored ArtistRelationship rows — bill-slot data isn't a stored relationship
// type. is_headliner is derived (position = 0 OR set_type = 'headliner') consistent with
// the rest of the codebase (see discovery.go logic).
func (s *ArtistRelationshipService) GetArtistBillComposition(artistID uint, months int) (*contracts.ArtistBillComposition, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if months < 0 {
		months = 0
	}
	if months > billCompositionMaxMonths {
		months = billCompositionMaxMonths
	}

	// 1. Center artist
	var centerArtist catalogm.Artist
	if err := s.db.First(&centerArtist, artistID).Error; err != nil {
		return nil, apperrors.ErrArtistNotFound(artistID)
	}

	centerNode := buildArtistGraphNode(centerArtist, 0)

	result := &contracts.ArtistBillComposition{
		Artist:           centerNode,
		Stats:            contracts.BillStats{},
		OpensWith:        []contracts.BillCoArtist{},
		ClosesWith:       []contracts.BillCoArtist{},
		Graph:            contracts.ArtistGraph{Center: centerNode, Nodes: []contracts.ArtistGraphNode{}, Links: []contracts.ArtistGraphLink{}},
		TimeFilterMonths: months,
	}

	// 2. Stats query — counts headliner vs opener slots over the time window.
	statsSQL := `
		SELECT
			COUNT(DISTINCT sa.show_id) AS total_shows,
			COALESCE(SUM(CASE WHEN (sa.position = 0 OR sa.set_type = 'headliner') THEN 1 ELSE 0 END), 0) AS headliner_count,
			COALESCE(SUM(CASE WHEN NOT (sa.position = 0 OR sa.set_type = 'headliner') THEN 1 ELSE 0 END), 0) AS opener_count
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		WHERE sa.artist_id = ? AND s.status = 'approved'
		  AND (? = 0 OR s.event_date >= NOW() - make_interval(months => ?))
	`

	var statsRow billStatsRow
	if err := s.db.Raw(statsSQL, artistID, months, months).Scan(&statsRow).Error; err != nil {
		return nil, fmt.Errorf("failed to query bill stats: %w", err)
	}

	result.Stats = contracts.BillStats{
		TotalShows:     statsRow.TotalShows,
		HeadlinerCount: statsRow.HeadlinerCount,
		OpenerCount:    statsRow.OpenerCount,
	}

	// Below-threshold short-circuit: stats are populated, everything else stays empty.
	if statsRow.TotalShows < billCompositionMinShows {
		result.BelowThreshold = true
		return result, nil
	}

	// 3. Co-bill aggregation — one row per (co-artist, A's role, co's role).
	coBillSQL := `
		SELECT
			sa2.artist_id AS co_artist_id,
			CASE WHEN (sa1.position = 0 OR sa1.set_type = 'headliner') THEN 'headliner' ELSE 'opener' END AS a_role,
			CASE WHEN (sa2.position = 0 OR sa2.set_type = 'headliner') THEN 'headliner' ELSE 'opener' END AS co_role,
			COUNT(DISTINCT sa1.show_id) AS shared_count,
			MAX(s.event_date) AS last_shared
		FROM show_artists sa1
		JOIN show_artists sa2 ON sa2.show_id = sa1.show_id AND sa2.artist_id != sa1.artist_id
		JOIN shows s ON s.id = sa1.show_id
		WHERE sa1.artist_id = ? AND s.status = 'approved'
		  AND (? = 0 OR s.event_date >= NOW() - make_interval(months => ?))
		GROUP BY sa2.artist_id, a_role, co_role
		ORDER BY shared_count DESC
	`

	var coBillRows []billCoBillRow
	if err := s.db.Raw(coBillSQL, artistID, months, months).Scan(&coBillRows).Error; err != nil {
		return nil, fmt.Errorf("failed to query co-bills: %w", err)
	}

	if len(coBillRows) == 0 {
		return result, nil
	}

	// 4. Bucket rows + collect unique co-artist IDs (ordered by best shared_count first).
	opensWithBest := make(map[uint]*opensClosesEntry)  // co_role='opener', a_role='headliner'
	closesWithBest := make(map[uint]*opensClosesEntry) // co_role='headliner', a_role='opener'

	graphLinkAgg := make(map[uint]struct {
		count int
		last  time.Time
	}) // per co-artist, total shared count for the mini-graph
	graphOrder := make([]uint, 0, len(coBillRows))
	graphSeen := make(map[uint]bool)

	for _, r := range coBillRows {
		// Tables: only asymmetric pairs.
		if r.ARole == "headliner" && r.CoRole == "opener" {
			if cur, ok := opensWithBest[r.CoArtistID]; !ok || r.SharedCount > cur.sharedCount {
				opensWithBest[r.CoArtistID] = &opensClosesEntry{coID: r.CoArtistID, sharedCount: r.SharedCount, lastShared: r.LastShared}
			}
		} else if r.ARole == "opener" && r.CoRole == "headliner" {
			if cur, ok := closesWithBest[r.CoArtistID]; !ok || r.SharedCount > cur.sharedCount {
				closesWithBest[r.CoArtistID] = &opensClosesEntry{coID: r.CoArtistID, sharedCount: r.SharedCount, lastShared: r.LastShared}
			}
		}

		// Graph: include every role-pair, summed.
		agg := graphLinkAgg[r.CoArtistID]
		agg.count += r.SharedCount
		if r.LastShared.After(agg.last) {
			agg.last = r.LastShared
		}
		graphLinkAgg[r.CoArtistID] = agg

		if !graphSeen[r.CoArtistID] {
			graphSeen[r.CoArtistID] = true
			graphOrder = append(graphOrder, r.CoArtistID)
		}
	}

	// 5. Cap graph to top N by total shared_count.
	var graphIDs []uint
	{
		sorted := make([]uint, len(graphOrder))
		copy(sorted, graphOrder)
		// Sort by count desc; ties keep insertion order (already by SharedCount desc from SQL).
		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && graphLinkAgg[sorted[j]].count > graphLinkAgg[sorted[j-1]].count; j-- {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}
		if len(sorted) > billCompositionMaxNodes {
			sorted = sorted[:billCompositionMaxNodes]
		}
		graphIDs = sorted
	}

	// 6. Fetch co-artist details + upcoming-show counts for the union of all referenced IDs.
	relevantIDs := make(map[uint]bool, len(graphIDs)+len(opensWithBest)+len(closesWithBest))
	for _, id := range graphIDs {
		relevantIDs[id] = true
	}
	for id := range opensWithBest {
		relevantIDs[id] = true
	}
	for id := range closesWithBest {
		relevantIDs[id] = true
	}

	idList := make([]uint, 0, len(relevantIDs))
	for id := range relevantIDs {
		idList = append(idList, id)
	}

	var coArtists []catalogm.Artist
	if err := s.db.Where("id IN ?", idList).Find(&coArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch co-artist details: %w", err)
	}

	artistByID := make(map[uint]catalogm.Artist, len(coArtists))
	for _, a := range coArtists {
		artistByID[a.ID] = a
	}

	// Upcoming-show counts batched (mirrors GetArtistGraph:372–388).
	type showCountRow struct {
		ArtistID  uint
		ShowCount int64
	}
	var showCounts []showCountRow
	s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) as show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = 'approved' AND shows.event_date > NOW()", idList).
		Group("show_artists.artist_id").
		Scan(&showCounts)
	upcomingByID := make(map[uint]int, len(showCounts))
	for _, sc := range showCounts {
		upcomingByID[sc.ArtistID] = int(sc.ShowCount)
	}

	// 7. Build OpensWith and ClosesWith lists, top N each.
	result.OpensWith = sortAndCapCoArtists(opensWithBest, artistByID, upcomingByID, billCompositionTopRows)
	result.ClosesWith = sortAndCapCoArtists(closesWithBest, artistByID, upcomingByID, billCompositionTopRows)

	// 8. Build mini-graph nodes + links (center → co-artist as shared_bills).
	for _, id := range graphIDs {
		a, ok := artistByID[id]
		if !ok {
			continue
		}
		result.Graph.Nodes = append(result.Graph.Nodes, buildArtistGraphNode(a, upcomingByID[id]))

		agg := graphLinkAgg[id]
		score := math.Min(float64(agg.count)/10.0, 1.0)
		detail := map[string]interface{}{
			"shared_count": agg.count,
			"last_shared":  agg.last.Format("2006-01-02"),
		}
		result.Graph.Links = append(result.Graph.Links, contracts.ArtistGraphLink{
			SourceID: artistID,
			TargetID: id,
			Type:     catalogm.RelationshipTypeSharedBills,
			Score:    score,
			Detail:   detail,
		})
	}

	// 9. Cross-connections: shared_bills among the top graph artists, derived live
	//    from show_artists (parallels DeriveSharedBills but scoped to a fixed ID set).
	if len(graphIDs) > 1 {
		crossSQL := `
			SELECT
				sa1.artist_id AS artist_a,
				sa2.artist_id AS artist_b,
				COUNT(DISTINCT sa1.show_id) AS shared_count,
				MAX(s.event_date) AS last_shared
			FROM show_artists sa1
			JOIN show_artists sa2 ON sa1.show_id = sa2.show_id AND sa1.artist_id < sa2.artist_id
			JOIN shows s ON s.id = sa1.show_id
			WHERE sa1.artist_id IN ? AND sa2.artist_id IN ? AND s.status = 'approved'
			  AND (? = 0 OR s.event_date >= NOW() - make_interval(months => ?))
			GROUP BY sa1.artist_id, sa2.artist_id
			HAVING COUNT(DISTINCT sa1.show_id) >= 1
		`
		var crossRows []sharedBillRow
		if err := s.db.Raw(crossSQL, graphIDs, graphIDs, months, months).Scan(&crossRows).Error; err == nil {
			for _, r := range crossRows {
				score := math.Min(float64(r.SharedCount)/10.0, 1.0)
				detail := map[string]interface{}{
					"shared_count": r.SharedCount,
					"last_shared":  r.LastShared.Format("2006-01-02"),
				}
				result.Graph.Links = append(result.Graph.Links, contracts.ArtistGraphLink{
					SourceID: r.ArtistA,
					TargetID: r.ArtistB,
					Type:     catalogm.RelationshipTypeSharedBills,
					Score:    score,
					Detail:   detail,
				})
			}
		}
	}

	return result, nil
}

// buildArtistGraphNode constructs the standard ArtistGraphNode from a model + a precomputed
// upcoming-show count (callers are responsible for the count query so they can batch it).
func buildArtistGraphNode(a catalogm.Artist, upcomingShowCount int) contracts.ArtistGraphNode {
	slug := ""
	if a.Slug != nil {
		slug = *a.Slug
	}
	city := ""
	if a.City != nil {
		city = *a.City
	}
	state := ""
	if a.State != nil {
		state = *a.State
	}
	return contracts.ArtistGraphNode{
		ID:                a.ID,
		Name:              a.Name,
		Slug:              slug,
		City:              city,
		State:             state,
		UpcomingShowCount: upcomingShowCount,
	}
}

// sortAndCapCoArtists turns a map of best-per-co-artist entries into a sorted top-N list
// of BillCoArtist rows. Sort key: shared_count desc, then last_shared desc.
func sortAndCapCoArtists(
	best map[uint]*opensClosesEntry,
	artistByID map[uint]catalogm.Artist,
	upcomingByID map[uint]int,
	cap int,
) []contracts.BillCoArtist {
	if len(best) == 0 {
		return []contracts.BillCoArtist{}
	}

	entries := make([]*opensClosesEntry, 0, len(best))
	for _, e := range best {
		entries = append(entries, e)
	}

	// Insertion sort — small N (<=30 distinct co-artists); avoids a sort.Slice import.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j], entries[j-1]
			better := a.sharedCount > b.sharedCount || (a.sharedCount == b.sharedCount && a.lastShared.After(b.lastShared))
			if !better {
				break
			}
			entries[j], entries[j-1] = b, a
		}
	}

	if len(entries) > cap {
		entries = entries[:cap]
	}

	out := make([]contracts.BillCoArtist, 0, len(entries))
	for _, e := range entries {
		a, ok := artistByID[e.coID]
		if !ok {
			continue
		}
		out = append(out, contracts.BillCoArtist{
			Artist:      buildArtistGraphNode(a, upcomingByID[e.coID]),
			SharedCount: e.sharedCount,
			LastShared:  e.lastShared.Format("2006-01-02"),
		})
	}
	return out
}

// opensClosesEntry is one (co-artist, best shared_count, last_shared) row used
// internally by GetArtistBillComposition + sortAndCapCoArtists.
type opensClosesEntry struct {
	coID        uint
	sharedCount int
	lastShared  time.Time
}

// ──────────────────────────────────────────────
// Auto-derivation
// ──────────────────────────────────────────────

// sharedBillRow represents the result of the co-occurrence query.
type sharedBillRow struct {
	ArtistA     uint
	ArtistB     uint
	SharedCount int
	LastShared  time.Time
}

// DeriveSharedBills computes shared_bills relationships from show_artists co-occurrences.
// Creates or updates relationships where artists share minShows or more approved shows.
func (s *ArtistRelationshipService) DeriveSharedBills(minShows int) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	if minShows <= 0 {
		minShows = 2
	}

	var rows []sharedBillRow
	err := s.db.Raw(`
		SELECT
			sa1.artist_id AS artist_a,
			sa2.artist_id AS artist_b,
			COUNT(DISTINCT sa1.show_id) AS shared_count,
			MAX(s.event_date) AS last_shared
		FROM show_artists sa1
		JOIN show_artists sa2 ON sa1.show_id = sa2.show_id
			AND sa1.artist_id < sa2.artist_id
		JOIN shows s ON s.id = sa1.show_id
		WHERE s.status = 'approved'
		GROUP BY sa1.artist_id, sa2.artist_id
		HAVING COUNT(DISTINCT sa1.show_id) >= ?
	`, minShows).Scan(&rows).Error

	if err != nil {
		return 0, fmt.Errorf("failed to query shared bills: %w", err)
	}

	var upserted int64
	now := time.Now()

	for _, row := range rows {
		// Compute recency-weighted score
		monthsSince := now.Sub(row.LastShared).Hours() / (24 * 30)
		score := float32(math.Min(float64(row.SharedCount)/10.0, 1.0))
		// Boost for recency
		if monthsSince < 3 {
			score = float32(math.Min(float64(score)*1.2, 1.0))
		}

		detail, _ := json.Marshal(map[string]interface{}{
			"shared_count": row.SharedCount,
			"last_shared":  row.LastShared.Format("2006-01-02"),
		})
		detailRaw := json.RawMessage(detail)

		// Upsert
		var existing catalogm.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			row.ArtistA, row.ArtistB, catalogm.RelationshipTypeSharedBills).First(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			rel := &catalogm.ArtistRelationship{
				SourceArtistID:   row.ArtistA,
				TargetArtistID:   row.ArtistB,
				RelationshipType: catalogm.RelationshipTypeSharedBills,
				Score:            score,
				AutoDerived:      true,
				Detail:           &detailRaw,
			}
			if err := s.db.Create(rel).Error; err == nil {
				upserted++
			}
		} else if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"score":  score,
				"detail": &detailRaw,
			})
			upserted++
		}
	}

	return upserted, nil
}

// sharedLabelRow represents the result of the shared-labels co-occurrence query.
type sharedLabelRow struct {
	ArtistA     uint
	ArtistB     uint
	SharedCount int
	LabelNames  string
}

// DeriveSharedLabels computes shared_label relationships from the artist_labels join table.
// Creates or updates relationships where artists share minLabels or more labels.
func (s *ArtistRelationshipService) DeriveSharedLabels(minLabels int) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	if minLabels <= 0 {
		minLabels = 1
	}

	var rows []sharedLabelRow
	err := s.db.Raw(`
		SELECT
			al1.artist_id AS artist_a,
			al2.artist_id AS artist_b,
			COUNT(DISTINCT al1.label_id) AS shared_count,
			STRING_AGG(DISTINCT l.name, ', ' ORDER BY l.name) AS label_names
		FROM artist_labels al1
		JOIN artist_labels al2 ON al1.label_id = al2.label_id
			AND al1.artist_id < al2.artist_id
		JOIN labels l ON l.id = al1.label_id
		GROUP BY al1.artist_id, al2.artist_id
		HAVING COUNT(DISTINCT al1.label_id) >= ?
	`, minLabels).Scan(&rows).Error

	if err != nil {
		return 0, fmt.Errorf("failed to query shared labels: %w", err)
	}

	var upserted int64

	for _, row := range rows {
		// Score: proportion of shared labels (cap at 1.0)
		// More shared labels = stronger relationship
		score := float32(math.Min(float64(row.SharedCount)/5.0, 1.0))

		detail, _ := json.Marshal(map[string]interface{}{
			"shared_count": row.SharedCount,
			"label_names":  row.LabelNames,
		})
		detailRaw := json.RawMessage(detail)

		// Upsert
		var existing catalogm.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			row.ArtistA, row.ArtistB, catalogm.RelationshipTypeSharedLabel).First(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			rel := &catalogm.ArtistRelationship{
				SourceArtistID:   row.ArtistA,
				TargetArtistID:   row.ArtistB,
				RelationshipType: catalogm.RelationshipTypeSharedLabel,
				Score:            score,
				AutoDerived:      true,
				Detail:           &detailRaw,
			}
			if err := s.db.Create(rel).Error; err == nil {
				upserted++
			}
		} else if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"score":  score,
				"detail": &detailRaw,
			})
			upserted++
		}
	}

	return upserted, nil
}

// ──────────────────────────────────────────────
// Festival co-lineup (PSY-363) — query-time derivation
// ──────────────────────────────────────────────

// queryTimeRelationshipTypes is the set of edge types that are computed
// at query time (no stored row in artist_relationships). Centralised so
// future query-time signals (PSY-365 venue co-bill, etc.) can extend it.
var queryTimeRelationshipTypes = map[string]struct{}{
	festivalCobillType: {},
}

// filterOutQueryTimeTypes returns a copy of `types` with any query-time
// edge types removed. Used when building the stored-relationship query
// so we never search for a non-stored type in the artist_relationships
// table. If `types` is empty (meaning "all types"), returns an empty
// slice — the caller should treat that as "no filter on stored types".
func filterOutQueryTimeTypes(types []string) []string {
	if len(types) == 0 {
		return nil
	}
	out := make([]string, 0, len(types))
	for _, t := range types {
		if _, ok := queryTimeRelationshipTypes[t]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

// isQueryTimeTypeRequested returns true ONLY when `target` is a query-time
// edge type AND the filter explicitly contains `target`.
//
// Query-time types are strictly OPT-IN (PSY-954). An empty filter means
// "all STORED types only" — it must NOT auto-include query-time signals.
// Two reasons: (1) performance — query-time types (festival_cobill) run an
// expensive festival_artists JOIN on every request, so we never pay that cost
// on a default load; (2) product — festival co-lineup is not a default
// similarity signal (sharing one festival lineup says nothing about musical
// similarity), so it must never seed the "Similar artists" sidebar or the
// default graph view. Callers that want it pass it explicitly in `types`.
func isQueryTimeTypeRequested(types []string, target string) bool {
	if _, ok := queryTimeRelationshipTypes[target]; !ok {
		return false
	}
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}

// computeFestivalCobillCenterEdges derives festival co-lineup edges
// between the center artist and every other artist who has shared at
// least one festival appearance with them. Returns up to `limit` edges
// ordered by (shared_count DESC, most_recent_start DESC).
//
// Score formula:
//
//	base    = min(shared_count / festivalCobillTopN, 1.0)
//	final   = min(base * festivalCobillRecencyBoost, 1.0)   if most-recent shared festival is within
//	                                                         festivalCobillRecencyBoostYears years of now
//	final   = base                                           otherwise
//
// `detail` JSONB shape:
//
//	{
//	  "festival_names":   "ACL, Coachella, Lollapalooza",   // top N by recency
//	  "count":            3,                                // total shared festivals
//	  "most_recent_year": 2025                              // EXTRACT(YEAR FROM MAX(start_date))
//	}
//
// `most_recent_year` may be null when the festival start_date is missing
// (defensive — start_date is NOT NULL per the schema, but we don't crash
// the request if a row violates that for any reason).
func (s *ArtistRelationshipService) computeFestivalCobillCenterEdges(centerID uint, limit int) ([]contracts.ArtistGraphLink, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = festivalCobillCenterLimit
	}

	rows, err := s.queryFestivalCobillRows(centerID, nil, limit)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Collect (artistA, artistB) pairs to fetch their representative
	// festival names in a single batch query.
	pairs := make([]festivalCobillRow, 0, len(rows))
	pairs = append(pairs, rows...)
	nameMap, err := s.queryFestivalCobillNames(pairs)
	if err != nil {
		// Tooltip names are best-effort — log nothing, return whatever we have.
		nameMap = nil
	}

	now := time.Now()
	links := make([]contracts.ArtistGraphLink, 0, len(rows))
	for _, r := range rows {
		score := festivalCobillScore(r.SharedCount, r.MostRecentStart, now)

		// Build representative names: prefer "top-N by recency" if we got
		// it, otherwise leave empty so the FE falls back to count-only text.
		var festivalNames string
		if nameMap != nil {
			key := pairKey(r.ArtistA, r.ArtistB)
			festivalNames = strings.Join(nameMap[key], ", ")
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

		links = append(links, contracts.ArtistGraphLink{
			SourceID: r.ArtistA,
			TargetID: r.ArtistB,
			Type:     festivalCobillType,
			Score:    score,
			Detail:   detail,
		})
	}

	return links, nil
}

// computeFestivalCobillCrossEdges derives festival co-lineup edges
// between pairs of related artists (excluding the center). Used to fill
// in cross-connections in the graph so the layout has structure.
//
// `centerID` is excluded from both ends of returned edges to avoid
// duplicating center→related edges already produced by
// computeFestivalCobillCenterEdges.
func (s *ArtistRelationshipService) computeFestivalCobillCrossEdges(centerID uint, relatedIDs []uint) ([]contracts.ArtistGraphLink, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if len(relatedIDs) < 2 {
		return nil, nil
	}

	// No top-N cap on cross-edges — they are already bounded by the
	// (relatedIDs × relatedIDs) Cartesian space. The 30-node ceiling on
	// relatedIDs keeps the absolute count <= ~435 in the worst case.
	rows, err := s.queryFestivalCobillRows(0, relatedIDs, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Filter out any pair that touches the center artist. The center
	// edges are produced by the dedicated path and we don't want them
	// duplicated here.
	filtered := rows[:0]
	for _, r := range rows {
		if r.ArtistA == centerID || r.ArtistB == centerID {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	nameMap, err := s.queryFestivalCobillNames(filtered)
	if err != nil {
		nameMap = nil
	}

	now := time.Now()
	links := make([]contracts.ArtistGraphLink, 0, len(filtered))
	for _, r := range filtered {
		score := festivalCobillScore(r.SharedCount, r.MostRecentStart, now)

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

		links = append(links, contracts.ArtistGraphLink{
			SourceID: r.ArtistA,
			TargetID: r.ArtistB,
			Type:     festivalCobillType,
			Score:    score,
			Detail:   detail,
		})
	}

	return links, nil
}

// queryFestivalCobillRows runs the JOIN that aggregates festival
// co-occurrences into (artistA, artistB, count, most_recent_start) rows.
// One of (centerID, relatedIDs) selects the scope:
//   - centerID > 0, relatedIDs nil/empty: center edges. Pairs are
//     (centerID, otherID) with no canonical-order requirement; rows are
//     normalised so ArtistA = centerID, ArtistB = the other artist.
//   - centerID == 0, relatedIDs non-empty: cross edges. Pairs use the
//     fa1.artist_id < fa2.artist_id canonical ordering and rows are
//     restricted to pairs where both ends are in relatedIDs.
//
// `limit > 0` applies a LIMIT clause; `limit == 0` means no limit.
func (s *ArtistRelationshipService) queryFestivalCobillRows(
	centerID uint,
	relatedIDs []uint,
	limit int,
) ([]festivalCobillRow, error) {
	if centerID == 0 && len(relatedIDs) == 0 {
		return nil, nil
	}

	var rows []festivalCobillRow

	if centerID > 0 {
		// Center mode: every artist who has shared at least one festival
		// with `centerID`. fa1 is anchored to the center; fa2 is the
		// other artist. We project fa1.artist_id as ArtistA so the
		// normalised pair always has the center on the SourceID side.
		err := s.db.Raw(`
			SELECT
				CAST(? AS BIGINT) AS artist_a,
				fa2.artist_id AS artist_b,
				COUNT(DISTINCT fa1.festival_id) AS shared_count,
				MAX(f.start_date) AS most_recent_start,
				MAX(EXTRACT(YEAR FROM f.start_date))::int AS most_recent_year
			FROM festival_artists fa1
			JOIN festival_artists fa2 ON fa1.festival_id = fa2.festival_id
				AND fa2.artist_id <> fa1.artist_id
			JOIN festivals f ON f.id = fa1.festival_id
			WHERE fa1.artist_id = ?
			GROUP BY fa2.artist_id
			HAVING COUNT(DISTINCT fa1.festival_id) >= 1
			ORDER BY shared_count DESC, most_recent_start DESC
			LIMIT ?
		`, centerID, centerID, limit).Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query festival co-lineup (center): %w", err)
		}
		return rows, nil
	}

	// Cross-edge mode. Canonical ordering enforced via fa1.artist_id <
	// fa2.artist_id.
	query := s.db.Raw(`
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
		WHERE fa1.artist_id IN ? AND fa2.artist_id IN ?
		GROUP BY fa1.artist_id, fa2.artist_id
		HAVING COUNT(DISTINCT fa1.festival_id) >= 1
		ORDER BY shared_count DESC, most_recent_start DESC
	`, relatedIDs, relatedIDs)

	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query festival co-lineup (cross): %w", err)
	}
	return rows, nil
}

// queryFestivalCobillNames fetches the top-N festival names by recency
// (start_date DESC) for each (artistA, artistB) pair in `pairs`. Used to
// populate the `festival_names` field in the edge's `detail` JSONB.
//
// Returns a map keyed by pairKey(artistA, artistB) to a slice of festival
// names sorted most-recent-first. The slice has at most
// festivalCobillTopFestivalNames entries.
func (s *ArtistRelationshipService) queryFestivalCobillNames(pairs []festivalCobillRow) (map[string][]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	// Build a UNION ALL of (artistA, artistB) literal pairs so a single
	// query can return names for all pairs. Postgres tolerates the OR-of-
	// pair-pairs form, which is simpler in GORM than building a temp
	// table.
	allArtistIDs := make(map[uint]struct{})
	for _, p := range pairs {
		allArtistIDs[p.ArtistA] = struct{}{}
		allArtistIDs[p.ArtistB] = struct{}{}
	}
	idList := make([]uint, 0, len(allArtistIDs))
	for id := range allArtistIDs {
		idList = append(idList, id)
	}

	// Fetch every shared festival between any two artists in idList,
	// then filter in Go to the exact pairs we care about. Cheaper than
	// emitting `pairs` parameters in SQL when len(pairs) is large.
	type nameRow struct {
		ArtistA      uint
		ArtistB      uint
		FestivalName string
		StartDate    time.Time
		EditionYear  int
	}
	var results []nameRow
	err := s.db.Raw(`
		SELECT
			fa1.artist_id AS artist_a,
			fa2.artist_id AS artist_b,
			f.name AS festival_name,
			f.start_date AS start_date,
			f.edition_year AS edition_year
		FROM festival_artists fa1
		JOIN festival_artists fa2 ON fa1.festival_id = fa2.festival_id
			AND fa1.artist_id < fa2.artist_id
		JOIN festivals f ON f.id = fa1.festival_id
		WHERE fa1.artist_id IN ? AND fa2.artist_id IN ?
		ORDER BY f.start_date DESC, f.edition_year DESC
	`, idList, idList).Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query festival names: %w", err)
	}

	// Build the requested set of pair keys, both orientations, so we can
	// match the canonical (a < b) projection coming back from the query.
	wanted := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		a, b := p.ArtistA, p.ArtistB
		if a > b {
			a, b = b, a
		}
		wanted[pairKey(a, b)] = struct{}{}
	}

	out := make(map[string][]string, len(pairs))
	for _, r := range results {
		canonKey := pairKey(r.ArtistA, r.ArtistB)
		if _, ok := wanted[canonKey]; !ok {
			continue
		}
		// Map back to BOTH orientations so callers can use whichever
		// convention they project on (center mode = (centerID, other);
		// cross mode = (a < b)).
		canonOut := canonKey
		if len(out[canonOut]) >= festivalCobillTopFestivalNames {
			continue
		}
		out[canonOut] = append(out[canonOut], r.FestivalName)
	}

	// Mirror the canonical-ordered keys to the alternative orientation
	// so center-mode lookups (where ArtistA = centerID, which may be the
	// larger id) still hit a key.
	mirrored := make(map[string][]string, len(out)*2)
	for k, v := range out {
		mirrored[k] = v
		// keys are "a-b" with a<b; also store "b-a"
		var a, b uint
		// Keys were just built via pairKey() (fmt.Sprintf("%d-%d", ...)) so
		// the Sscanf round-trip is guaranteed to parse. Discarding count+err
		// is intentional: any failure here would mean we corrupted our own
		// key format, not malformed input.
		//nolint:errcheck // round-trip of locally-constructed key; no failure mode in practice
		fmt.Sscanf(k, "%d-%d", &a, &b)
		mirrored[fmt.Sprintf("%d-%d", b, a)] = v
	}
	return mirrored, nil
}

// festivalCobillScore computes the [0,1] score for a festival co-lineup
// edge from the shared count and most-recent shared-festival date.
func festivalCobillScore(sharedCount int, mostRecentStart *time.Time, now time.Time) float64 {
	if sharedCount <= 0 {
		return 0
	}
	base := math.Min(float64(sharedCount)/festivalCobillTopN, 1.0)
	if mostRecentStart != nil {
		yearsSince := now.Sub(*mostRecentStart).Hours() / (24 * 365.25)
		if yearsSince < float64(festivalCobillRecencyBoostYears) {
			base = math.Min(base*festivalCobillRecencyBoost, 1.0)
		}
	}
	return base
}

// pairKey builds the map key used to associate a (artistA, artistB) pair
// with its representative festival names. Format: "artistA-artistB".
func pairKey(a, b uint) string {
	return fmt.Sprintf("%d-%d", a, b)
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

// voteCountKey identifies one relationship in a batched vote-count lookup.
type voteCountKey struct {
	sourceID uint
	targetID uint
	relType  string
}

// voteTally is the up/down pair for one relationship.
type voteTally struct {
	up   int
	down int
}

// getVoteCountsAmong returns vote tallies for EVERY relationship whose two
// endpoints are both in artistIDs, in a single grouped query (PSY-1301) —
// the batch replacement for calling getVoteCounts per edge. Slightly
// over-fetches (votes for relationships the caller won't emit), which is
// harmless: lookups miss to a zero tally. Non-fatal like getVoteCounts:
// votes are decorative on the graph, so an error degrades to zero counts.
func (s *ArtistRelationshipService) getVoteCountsAmong(artistIDs []uint) map[voteCountKey]voteTally {
	counts := make(map[voteCountKey]voteTally)
	if len(artistIDs) == 0 {
		return counts
	}

	// COUNT(*) FILTER — the repo idiom for tallying two directions in one row
	// (see scene.go / admin analytics), one row per relationship.
	type voteRow struct {
		SourceArtistID   uint   `gorm:"column:source_artist_id"`
		TargetArtistID   uint   `gorm:"column:target_artist_id"`
		RelationshipType string `gorm:"column:relationship_type"`
		Up               int    `gorm:"column:up"`
		Down             int    `gorm:"column:down"`
	}
	var rows []voteRow
	err := s.db.Model(&catalogm.ArtistRelationshipVote{}).
		Select("source_artist_id, target_artist_id, relationship_type, " +
			"COUNT(*) FILTER (WHERE direction = 1) AS up, " +
			"COUNT(*) FILTER (WHERE direction = -1) AS down").
		Where("source_artist_id IN ? AND target_artist_id IN ?", artistIDs, artistIDs).
		Group("source_artist_id, target_artist_id, relationship_type").
		Scan(&rows).Error
	if err != nil {
		slog.Warn("artist graph: batched vote-count query failed; rendering zero counts", "error", err)
		return counts
	}

	for _, r := range rows {
		counts[voteCountKey{r.SourceArtistID, r.TargetArtistID, r.RelationshipType}] = voteTally{up: r.Up, down: r.Down}
	}
	return counts
}

// recalculateScore recalculates the score for a relationship from vote counts.
func (s *ArtistRelationshipService) recalculateScore(tx *gorm.DB, sourceID, targetID uint, relType string) error {
	var upvotes, downvotes int64
	tx.Model(&catalogm.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = 1",
			sourceID, targetID, relType).Count(&upvotes)
	tx.Model(&catalogm.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = -1",
			sourceID, targetID, relType).Count(&downvotes)

	var rel catalogm.ArtistRelationship
	score := float32(rel.WilsonScore(int(upvotes), int(downvotes)))

	return tx.Model(&catalogm.ArtistRelationship{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			sourceID, targetID, relType).
		Update("score", score).Error
}
