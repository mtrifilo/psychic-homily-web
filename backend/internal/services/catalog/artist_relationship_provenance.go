package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// Edge provenance (PSY-1335) — the entities behind each typed connection
// between an artist pair, resolved by LIVE queries against the underlying
// fact tables (show_artists, artist_labels, festival_artists, radio_plays)
// rather than by hydrating the derive-time `detail` snapshot, so provenance
// can never drift from current data.
//
// Per-type entity sources:
//   - shared_bills        → shared approved shows, newest first
//   - shared_label        → shared labels
//   - festival_cobill     → shared festivals (query-time type: no stored row.
//     Unlike GetArtistGraph, where the festival union is strictly OPT-IN via
//     the types filter (PSY-954: graph-wide cost + not a similarity signal),
//     provenance ALWAYS unions it — the request is pair-scoped so the cost
//     concern doesn't apply, and the panel's contract is every connection)
//   - radio_cooccurrence  → shared stations ONLY (episode-level rows are too
//     numerous; radio_artist_affinity stores only a station COUNT, so the
//     stations are resolved from the raw radio_plays facts)
//   - similar / member_of / side_project → no entities (score/votes suffice)

// provenanceEntityKind* are the wire values for RelationshipProvenanceEntity.Kind,
// matching the frontend ConnectionEntity contract (ConnectionPanel.tsx).
const (
	provenanceEntityKindShow     = "show"
	provenanceEntityKindLabel    = "label"
	provenanceEntityKindFestival = "festival"
	provenanceEntityKindStation  = "station"
)

// provenanceUntitledShow mirrors the frontend showDisplayTitle convention
// (title → bill names → "Untitled Show", PSY-1328) for shows with neither.
const provenanceUntitledShow = "Untitled Show"

// provenanceBillNameCap bounds the bill-name fallback for untitled shows,
// matching the narrow-row cap used by scene preview rows (PSY-1325/1328).
const provenanceBillNameCap = 3

// GetRelationshipProvenance returns every typed connection between the pair —
// stored artist_relationships rows plus the query-time festival_cobill signal —
// each with the resolvable entities behind the claim. Returns
// ErrArtistNotFound when either artist doesn't exist and
// ErrArtistRelationshipNotFound when no connection of any type exists.
func (s *ArtistRelationshipService) GetRelationshipProvenance(artistA, artistB uint) (*contracts.RelationshipProvenance, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// A pair needs two distinct artists; the canonical CHECK constraint means
	// a self-pair can never have a stored row, and the entity queries below
	// assume distinct endpoints.
	if artistA == artistB {
		return nil, apperrors.ErrArtistRelationshipNotFound()
	}

	for _, id := range []uint{artistA, artistB} {
		var artist catalogm.Artist
		if err := s.db.Select("id").First(&artist, id).Error; err != nil {
			// Only a genuine miss is a 404 — a transient DB failure coerced to
			// not-found would be cached client-side as "no provenance" and
			// never retried (4xx is non-retryable on the FE).
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.ErrArtistNotFound(id)
			}
			return nil, fmt.Errorf("failed to verify artist %d: %w", id, err)
		}
	}

	src, tgt := catalogm.CanonicalOrder(artistA, artistB)

	var rels []catalogm.ArtistRelationship
	if err := s.db.
		Where("source_artist_id = ? AND target_artist_id = ?", src, tgt).
		Order("score DESC").
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	connections := make([]contracts.RelationshipProvenanceConnection, 0, len(rels)+1)

	for _, rel := range rels {
		var detail any
		if rel.Detail != nil {
			// A corrupt blob degrades this row to generic copy (never fails
			// the request), but must be discoverable.
			if err := json.Unmarshal(*rel.Detail, &detail); err != nil {
				slog.Warn("relationship provenance: corrupt detail blob",
					"source", rel.SourceArtistID, "target", rel.TargetArtistID,
					"type", rel.RelationshipType, "error", err)
			}
		}

		conn := contracts.RelationshipProvenanceConnection{
			Type:   rel.RelationshipType,
			Score:  float64(rel.Score),
			Detail: detail,
		}

		// Entity-query errors fail the request (the entities ARE the point of
		// this endpoint; the client keeps its phase-1 text rows on error) —
		// deliberately stricter than GetArtistGraph's degrade-and-log edges.
		var err error
		switch rel.RelationshipType {
		case catalogm.RelationshipTypeSharedBills:
			conn.Entities, conn.EntityTotal, err = s.sharedShowEntities(src, tgt)
		case catalogm.RelationshipTypeSharedLabel:
			conn.Entities, conn.EntityTotal, err = s.sharedLabelEntities(src, tgt)
		case catalogm.RelationshipTypeRadioCooccurrence:
			conn.Entities, conn.EntityTotal, err = s.sharedStationEntities(src, tgt)
		}
		if err != nil {
			return nil, err
		}

		connections = append(connections, conn)
	}

	// festival_cobill has no stored row (query-time type, PSY-363) — always
	// union it from festival_artists so festival-only pairs still resolve.
	// (Graph loads keep it opt-in per PSY-954; that's a graph-wide cost and
	// similarity-semantics concern, neither of which applies pair-scoped.)
	festivalConn, err := s.festivalCobillConnection(src, tgt)
	if err != nil {
		return nil, err
	}
	if festivalConn != nil {
		connections = append(connections, *festivalConn)
	}

	if len(connections) == 0 {
		return nil, apperrors.ErrArtistRelationshipNotFound()
	}

	return &contracts.RelationshipProvenance{Connections: connections}, nil
}

// sharedShowEntities resolves the approved shows both artists played, newest
// first, capped at RelationshipProvenanceEntityCap with the uncapped total.
func (s *ArtistRelationshipService) sharedShowEntities(a, b uint) ([]contracts.RelationshipProvenanceEntity, int, error) {
	type row struct {
		ID        uint
		Slug      *string
		Title     string
		EventDate time.Time
		Total     int
	}
	var rows []row
	// COUNT(*) OVER () is computed before LIMIT, so every returned row carries
	// the uncapped total — one round-trip instead of a separate COUNT.
	// Slug-less shows are excluded in SQL (not post-filtered): they can't be
	// linked, and letting them consume LIMIT slots would push linkable shows
	// out of the window while the total kept counting them — the list and the
	// "and N more" disclosure must agree on what they're counting.
	if err := s.db.Raw(`
		SELECT s.id, s.slug, s.title, s.event_date, COUNT(*) OVER () AS total
		FROM show_artists sa1
		JOIN show_artists sa2 ON sa2.show_id = sa1.show_id AND sa2.artist_id = ?
		JOIN shows s ON s.id = sa1.show_id
		WHERE sa1.artist_id = ? AND s.status = ?
			AND s.slug IS NOT NULL AND s.slug <> ''
		ORDER BY s.event_date DESC, s.id DESC
		LIMIT ?
	`, b, a, catalogm.ShowStatusApproved, contracts.RelationshipProvenanceEntityCap).
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query shared shows: %w", err)
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	// Untitled shows fall back to their bill names (the PSY-1328 display-title
	// convention) — resolved in one batch for the capped set only.
	var untitledIDs []uint
	for _, r := range rows {
		if strings.TrimSpace(r.Title) == "" {
			untitledIDs = append(untitledIDs, r.ID)
		}
	}
	var namesByShow map[uint][]string
	if len(untitledIDs) > 0 {
		var err error
		namesByShow, err = shared.BatchResolveShowArtistNames(s.db, untitledIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to resolve show bill names: %w", err)
		}
	}

	entities := make([]contracts.RelationshipProvenanceEntity, 0, len(rows))
	for _, r := range rows {
		entities = append(entities, contracts.RelationshipProvenanceEntity{
			Kind: provenanceEntityKindShow,
			ID:   r.ID,
			Slug: *r.Slug, // non-null by the SQL filter
			Name: showEntityName(r.Title, namesByShow[r.ID]),
			Date: r.EventDate.UTC().Format("2006-01-02"),
		})
	}
	return entities, rows[0].Total, nil
}

// showEntityName applies the title → capped bill → "Untitled Show" convention.
func showEntityName(title string, billNames []string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	names := make([]string, 0, len(billNames))
	for _, n := range billNames {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		return provenanceUntitledShow
	}
	if len(names) > provenanceBillNameCap {
		return fmt.Sprintf("%s +%d more",
			strings.Join(names[:provenanceBillNameCap], ", "),
			len(names)-provenanceBillNameCap)
	}
	return strings.Join(names, ", ")
}

// sharedLabelEntities resolves the labels both artists appear on.
func (s *ArtistRelationshipService) sharedLabelEntities(a, b uint) ([]contracts.RelationshipProvenanceEntity, int, error) {
	type row struct {
		ID    uint
		Slug  *string
		Name  string
		Total int
	}
	var rows []row
	// Slug-less labels excluded in SQL for the same list/total coherence
	// reason as sharedShowEntities.
	if err := s.db.Raw(`
		SELECT l.id, l.slug, l.name, COUNT(*) OVER () AS total
		FROM artist_labels al1
		JOIN artist_labels al2 ON al2.label_id = al1.label_id AND al2.artist_id = ?
		JOIN labels l ON l.id = al1.label_id
		WHERE al1.artist_id = ?
			AND l.slug IS NOT NULL AND l.slug <> ''
		ORDER BY l.name ASC, l.id ASC
		LIMIT ?
	`, b, a, contracts.RelationshipProvenanceEntityCap).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query shared labels: %w", err)
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	entities := make([]contracts.RelationshipProvenanceEntity, 0, len(rows))
	for _, r := range rows {
		entities = append(entities, contracts.RelationshipProvenanceEntity{
			Kind: provenanceEntityKindLabel,
			ID:   r.ID,
			Slug: *r.Slug, // non-null by the SQL filter
			Name: r.Name,
		})
	}
	return entities, rows[0].Total, nil
}

// sharedStationEntities resolves the stations on which both artists were
// played together (same episode). Station-level only: radio_artist_affinity
// stores just a station COUNT (radio_station_graph.go doc), so this walks the
// raw play facts — radio_plays → radio_episodes → radio_shows → radio_stations.
func (s *ArtistRelationshipService) sharedStationEntities(a, b uint) ([]contracts.RelationshipProvenanceEntity, int, error) {
	type row struct {
		ID    uint
		Slug  string
		Name  string
		Total int
	}
	var rows []row
	if err := s.db.Raw(`
		SELECT id, slug, name, COUNT(*) OVER () AS total FROM (
			SELECT DISTINCT rs.id, rs.slug, rs.name
			FROM radio_plays rp1
			JOIN radio_plays rp2 ON rp2.episode_id = rp1.episode_id AND rp2.artist_id = ?
			JOIN radio_episodes re ON re.id = rp1.episode_id
			JOIN radio_shows rsh ON rsh.id = re.show_id
			JOIN radio_stations rs ON rs.id = rsh.station_id
			WHERE rp1.artist_id = ?
		) stations
		ORDER BY name ASC, id ASC
		LIMIT ?
	`, b, a, contracts.RelationshipProvenanceEntityCap).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query shared stations: %w", err)
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	entities := make([]contracts.RelationshipProvenanceEntity, 0, len(rows))
	for _, r := range rows {
		entities = append(entities, contracts.RelationshipProvenanceEntity{
			Kind: provenanceEntityKindStation,
			ID:   r.ID,
			Slug: r.Slug,
			Name: r.Name,
		})
	}
	return entities, rows[0].Total, nil
}

// festivalCobillConnection computes the query-time festival_cobill connection
// for the pair, or nil when they share no festival. Score + detail reuse the
// exact cross-edge path GetArtistGraph uses (computeFestivalCobillCrossEdges),
// so panel copy can't drift from the graph tooltip.
func (s *ArtistRelationshipService) festivalCobillConnection(src, tgt uint) (*contracts.RelationshipProvenanceConnection, error) {
	// centerID=0: no center exclusion applies — the ID pair scopes the query.
	links, err := s.computeFestivalCobillCrossEdges(0, []uint{src, tgt})
	if err != nil {
		return nil, fmt.Errorf("failed to compute festival co-lineup: %w", err)
	}
	if len(links) == 0 {
		return nil, nil
	}
	link := links[0]

	entities, total, err := s.sharedFestivalEntities(src, tgt)
	if err != nil {
		return nil, err
	}

	return &contracts.RelationshipProvenanceConnection{
		Type:        festivalCobillType,
		Score:       link.Score,
		Detail:      link.Detail,
		Entities:    entities,
		EntityTotal: total,
	}, nil
}

// sharedFestivalEntities resolves the festivals both artists appeared on,
// newest edition first.
func (s *ArtistRelationshipService) sharedFestivalEntities(a, b uint) ([]contracts.RelationshipProvenanceEntity, int, error) {
	type row struct {
		ID          uint
		Slug        string
		Name        string
		EditionYear int
		Total       int
	}
	var rows []row
	if err := s.db.Raw(`
		SELECT f.id, f.slug, f.name, f.edition_year, COUNT(*) OVER () AS total
		FROM festival_artists fa1
		JOIN festival_artists fa2 ON fa2.festival_id = fa1.festival_id AND fa2.artist_id = ?
		JOIN festivals f ON f.id = fa1.festival_id
		WHERE fa1.artist_id = ?
		ORDER BY f.start_date DESC, f.id DESC
		LIMIT ?
	`, b, a, contracts.RelationshipProvenanceEntityCap).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query shared festivals: %w", err)
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	entities := make([]contracts.RelationshipProvenanceEntity, 0, len(rows))
	for _, r := range rows {
		entity := contracts.RelationshipProvenanceEntity{
			Kind: provenanceEntityKindFestival,
			ID:   r.ID,
			Slug: r.Slug,
			Name: r.Name,
		}
		if r.EditionYear > 0 {
			entity.Date = strconv.Itoa(r.EditionYear)
		}
		entities = append(entities, entity)
	}
	return entities, rows[0].Total, nil
}
