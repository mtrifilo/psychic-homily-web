package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
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
func (s *ArtistRelationshipService) CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*models.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if sourceID == targetID {
		return nil, fmt.Errorf("cannot create relationship between an artist and itself")
	}

	src, tgt := models.CanonicalOrder(sourceID, targetID)

	// Check for existing
	var existing models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("relationship already exists between artists %d and %d (type: %s)", src, tgt, relType)
	}

	rel := &models.ArtistRelationship{
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
func (s *ArtistRelationshipService) GetRelationship(artistA, artistB uint, relType string) (*models.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	var rel models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
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
	query := s.db.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? OR target_artist_id = ?", artistID, artistID)

	if relType != "" {
		query = query.Where("relationship_type = ?", relType)
	}

	query = query.Order("score DESC").Limit(limit)

	var rels []models.ArtistRelationship
	if err := query.Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("failed to get related artists: %w", err)
	}

	responses := make([]contracts.RelatedArtistResponse, 0, len(rels))
	for _, rel := range rels {
		// Determine the "other" artist
		otherID := rel.TargetArtistID
		if otherID == artistID {
			otherID = rel.SourceArtistID
		}

		// Fetch artist info
		var artist models.Artist
		if err := s.db.First(&artist, otherID).Error; err != nil {
			continue
		}

		slug := ""
		if artist.Slug != nil {
			slug = *artist.Slug
		}

		// Get vote counts
		upvotes, downvotes := s.getVoteCounts(rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType)

		resp := contracts.RelatedArtistResponse{
			ArtistID:         otherID,
			Name:             artist.Name,
			Slug:             slug,
			RelationshipType: rel.RelationshipType,
			Score:            rel.Score,
			Upvotes:          upvotes,
			Downvotes:        downvotes,
			WilsonScore:      rel.WilsonScore(upvotes, downvotes),
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

	src, tgt := models.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete votes first
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&models.ArtistRelationshipVote{})

		result := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&models.ArtistRelationship{})
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

	src, tgt := models.CanonicalOrder(artistA, artistB)

	// Verify relationship exists
	var rel models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
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
		var existing models.ArtistRelationshipVote
		err := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			vote := models.ArtistRelationshipVote{
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

	src, tgt := models.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).Delete(&models.ArtistRelationshipVote{})

		return s.recalculateScore(tx, src, tgt, relType)
	})
}

// GetUserVote returns a user's vote on a relationship, or nil if not voted.
func (s *ArtistRelationshipService) GetUserVote(artistA, artistB uint, relType string, userID uint) (*models.ArtistRelationshipVote, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	var vote models.ArtistRelationshipVote
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
		src, tgt, relType, userID).First(&vote).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
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

// festivalCobillRow is the result of the festival co-occurrence query.
// We capture both the canonical pair (artistA<artistB), the count, and
// the start date of the most recent shared festival (for recency-weighted
// scoring + tooltip).
type festivalCobillRow struct {
	ArtistA          uint
	ArtistB          uint
	SharedCount      int
	MostRecentStart  *time.Time
	MostRecentYear   *int
}

// festivalNameRow is one festival name for a given artist pair, used to
// fetch the top-N festivals by recency for the tooltip detail.
type festivalNameRow struct {
	ArtistA      uint
	ArtistB      uint
	FestivalName string
	StartDate    time.Time
	EditionYear  int
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
	var centerArtist models.Artist
	if err := s.db.First(&centerArtist, artistID).Error; err != nil {
		return nil, fmt.Errorf("artist not found: %w", err)
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

	var rels []models.ArtistRelationship
	// If types was non-empty AND every requested type was a query-time
	// derived type, storedTypes will be empty here — skip the stored-rels
	// query entirely. Otherwise (empty input = "all types", or any stored
	// type requested) run the normal query.
	if len(types) == 0 || len(storedTypes) > 0 {
		query := s.db.Model(&models.ArtistRelationship{}).
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
	var relatedArtists []models.Artist
	if err := s.db.Where("id IN ?", relatedIDs).Find(&relatedArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to get related artist details: %w", err)
	}

	artistMap := make(map[uint]models.Artist)
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

	// 6. Build links from center relationships
	for _, rel := range rels {
		upvotes, downvotes := s.getVoteCounts(rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType)

		var detail interface{}
		if rel.Detail != nil {
			_ = json.Unmarshal(*rel.Detail, &detail)
		}

		graph.Links = append(graph.Links, contracts.ArtistGraphLink{
			SourceID:  rel.SourceArtistID,
			TargetID:  rel.TargetArtistID,
			Type:      rel.RelationshipType,
			Score:     float64(rel.Score),
			VotesUp:   upvotes,
			VotesDown: downvotes,
			Detail:    detail,
		})
	}

	// 6b. Append the query-time festival_cobill center edges (PSY-363).
	graph.Links = append(graph.Links, festivalCobillLinks...)

	// 7. Get cross-connections between related artists
	if len(relatedIDs) > 1 {
		var crossRels []models.ArtistRelationship
		crossQuery := s.db.Model(&models.ArtistRelationship{}).
			Where("source_artist_id IN ? AND target_artist_id IN ?", relatedIDs, relatedIDs)

		if len(storedTypes) > 0 {
			crossQuery = crossQuery.Where("relationship_type IN ?", storedTypes)
		}

		if err := crossQuery.Find(&crossRels).Error; err == nil {
			for _, rel := range crossRels {
				upvotes, downvotes := s.getVoteCounts(rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType)

				var detail interface{}
				if rel.Detail != nil {
					_ = json.Unmarshal(*rel.Detail, &detail)
				}

				graph.Links = append(graph.Links, contracts.ArtistGraphLink{
					SourceID:  rel.SourceArtistID,
					TargetID:  rel.TargetArtistID,
					Type:      rel.RelationshipType,
					Score:     float64(rel.Score),
					VotesUp:   upvotes,
					VotesDown: downvotes,
					Detail:    detail,
				})
			}
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
		var existing models.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			row.ArtistA, row.ArtistB, models.RelationshipTypeSharedBills).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			rel := &models.ArtistRelationship{
				SourceArtistID:   row.ArtistA,
				TargetArtistID:   row.ArtistB,
				RelationshipType: models.RelationshipTypeSharedBills,
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
	ArtistA      uint
	ArtistB      uint
	SharedCount  int
	LabelNames   string
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
		var existing models.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			row.ArtistA, row.ArtistB, models.RelationshipTypeSharedLabel).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			rel := &models.ArtistRelationship{
				SourceArtistID:   row.ArtistA,
				TargetArtistID:   row.ArtistB,
				RelationshipType: models.RelationshipTypeSharedLabel,
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

// isQueryTimeTypeRequested returns true when `target` is a query-time
// edge type AND either (a) the filter is empty (=all types) or (b) the
// filter explicitly contains `target`.
func isQueryTimeTypeRequested(types []string, target string) bool {
	if _, ok := queryTimeRelationshipTypes[target]; !ok {
		return false
	}
	if len(types) == 0 {
		return true
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
	for _, r := range rows {
		pairs = append(pairs, r)
	}
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

// getVoteCounts returns upvote and downvote counts for a relationship.
func (s *ArtistRelationshipService) getVoteCounts(sourceID, targetID uint, relType string) (int, int) {
	var upvotes, downvotes int64
	s.db.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = 1",
			sourceID, targetID, relType).Count(&upvotes)
	s.db.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = -1",
			sourceID, targetID, relType).Count(&downvotes)
	return int(upvotes), int(downvotes)
}

// recalculateScore recalculates the score for a relationship from vote counts.
func (s *ArtistRelationshipService) recalculateScore(tx *gorm.DB, sourceID, targetID uint, relType string) error {
	var upvotes, downvotes int64
	tx.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = 1",
			sourceID, targetID, relType).Count(&upvotes)
	tx.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = -1",
			sourceID, targetID, relType).Count(&downvotes)

	var rel models.ArtistRelationship
	score := float32(rel.WilsonScore(int(upvotes), int(downvotes)))

	return tx.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			sourceID, targetID, relType).
		Update("score", score).Error
}
