package pipeline

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// StreamingWorklistService owns the admin streaming-discovery worklist
// surface. The list query joins artists with their soonest future show
// via a LATERAL subquery; the mutation enforces the transition matrix
// declared in `allowedTransitions` and writes the new status + optional
// reason in one UPDATE.
type StreamingWorklistService struct {
	db *gorm.DB
}

// NewStreamingWorklistService creates the service. A nil database is
// resolved against the process default so callers can construct the
// service without threading the global DB pointer.
func NewStreamingWorklistService(database *gorm.DB) *StreamingWorklistService {
	if database == nil {
		database = db.GetDB()
	}
	return &StreamingWorklistService{db: database}
}

// allowedTransitions enumerates legal {from → to} streaming-discovery
// status transitions. The default contract is intentionally restrictive
// so it can be loosened later without breaking callers:
//
//   - Non-terminal states (`unreviewed`, `candidates_pending`) can move
//     to any of the three terminal admin decisions (`linked`,
//     `no_links_found`, `skipped`).
//   - Terminal states can only move back to `unreviewed` (manual
//     re-open).
//   - `candidates_pending` is never a legal TARGET via this admin
//     endpoint. That state is engine-set (provider lookups populated
//     candidates awaiting review); an admin flipping a row into it has
//     no clear semantics. Easy to allow later if a workflow surfaces.
//   - Same-state transitions are rejected (no-op via this endpoint).
//
// Keys are the FROM status; values are the set of permitted TO statuses.
var allowedTransitions = map[catalogm.StreamingDiscoveryStatus]map[catalogm.StreamingDiscoveryStatus]struct{}{
	catalogm.StreamingDiscoveryStatusUnreviewed: {
		catalogm.StreamingDiscoveryStatusLinked:        {},
		catalogm.StreamingDiscoveryStatusNoLinksFound:  {},
		catalogm.StreamingDiscoveryStatusSkipped:       {},
	},
	catalogm.StreamingDiscoveryStatusCandidatesPending: {
		catalogm.StreamingDiscoveryStatusLinked:        {},
		catalogm.StreamingDiscoveryStatusNoLinksFound:  {},
		catalogm.StreamingDiscoveryStatusSkipped:       {},
	},
	catalogm.StreamingDiscoveryStatusLinked: {
		catalogm.StreamingDiscoveryStatusUnreviewed: {},
	},
	catalogm.StreamingDiscoveryStatusNoLinksFound: {
		catalogm.StreamingDiscoveryStatusUnreviewed: {},
	},
	catalogm.StreamingDiscoveryStatusSkipped: {
		catalogm.StreamingDiscoveryStatusUnreviewed: {},
	},
}

// validStreamingStatuses is the set of the five legal status values,
// used to validate the request body before consulting the transition
// matrix.
var validStreamingStatuses = map[catalogm.StreamingDiscoveryStatus]struct{}{
	catalogm.StreamingDiscoveryStatusUnreviewed:        {},
	catalogm.StreamingDiscoveryStatusCandidatesPending: {},
	catalogm.StreamingDiscoveryStatusLinked:            {},
	catalogm.StreamingDiscoveryStatusNoLinksFound:      {},
	catalogm.StreamingDiscoveryStatusSkipped:           {},
}

// nonTerminalWorklistStatuses are the statuses surfaced by the worklist
// list query. Terminal states (linked/no_links_found/skipped) are
// excluded — those rows have been triaged.
//
// Slice form is what the SQL `IN ?` placeholder expects; the set form
// is used for O(1) membership checks when validating the status filter.
var nonTerminalWorklistStatuses = []string{
	string(catalogm.StreamingDiscoveryStatusUnreviewed),
	string(catalogm.StreamingDiscoveryStatusCandidatesPending),
}

var nonTerminalWorklistStatusSet = map[string]struct{}{
	string(catalogm.StreamingDiscoveryStatusUnreviewed):        {},
	string(catalogm.StreamingDiscoveryStatusCandidatesPending): {},
}

// ListStreamingWorklist returns artists whose streaming_discovery_status
// is non-terminal AND who have at least one future show, ordered by
// soonest future event_date ASC, then artists.name ASC, then artist.id
// ASC for deterministic pagination.
//
// status, when non-empty, narrows the query to that single
// non-terminal status. Empty status means "all non-terminal".
//
// limit is clamped to [1, 200]; offset is clamped to >= 0.
func (s *StreamingWorklistService) ListStreamingWorklist(status string, limit, offset int) (*contracts.StreamingWorklistResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	// Allowed status filter values — only the two non-terminal statuses.
	// An explicit empty string means "no filter; both non-terminal
	// statuses". Unknown values reject with the same error class as an
	// invalid transition so callers handle a single failure mode.
	statusFilter := nonTerminalWorklistStatuses
	if status != "" {
		if _, ok := nonTerminalWorklistStatusSet[status]; !ok {
			return nil, fmt.Errorf("%w: status %q is not a non-terminal worklist status", contracts.ErrInvalidStreamingStatusTransition, status)
		}
		statusFilter = []string{status}
	}

	// LATERAL subquery selects the soonest future show for each artist
	// along with that show's venue (lowest venue_id breaks ties when a
	// show has multiple venues, mirroring the convention in
	// syncShowArtistDedupColumns). The outer aggregate counts ALL
	// upcoming shows for the artist so the UI can hint at queue
	// pressure ("3 upcoming shows") without a follow-up request.
	//
	// The query intentionally re-derives the upcoming-show count in
	// a correlated subquery rather than a separate JOIN+GROUP BY —
	// the row count is small (the worklist is bounded by limit=200)
	// and the correlated form keeps the projection flat for GORM
	// scanning.
	query := `
		WITH worklist AS (
			SELECT
				a.id              AS artist_id,
				a.name            AS artist_name,
				a.slug            AS artist_slug,
				a.streaming_discovery_status AS streaming_discovery_status,
				ns.event_date     AS soonest_event_date,
				ns.venue_name     AS venue_name,
				ns.venue_city     AS venue_city,
				(
					SELECT COUNT(*)
					FROM show_artists sa2
					JOIN shows s2 ON s2.id = sa2.show_id
					WHERE sa2.artist_id = a.id
					  AND s2.event_date >= NOW()
				) AS upcoming_show_count
			FROM artists a
			JOIN LATERAL (
				SELECT
					s.event_date,
					v.name AS venue_name,
					v.city AS venue_city
				FROM show_artists sa
				JOIN shows s         ON s.id = sa.show_id
				LEFT JOIN show_venues sv ON sv.show_id = s.id
				LEFT JOIN venues v       ON v.id = sv.venue_id
				WHERE sa.artist_id = a.id
				  AND s.event_date >= NOW()
				ORDER BY s.event_date ASC, v.id ASC
				LIMIT 1
			) ns ON TRUE
			WHERE a.streaming_discovery_status IN ?
		)
		SELECT
			artist_id,
			artist_name,
			artist_slug,
			streaming_discovery_status,
			soonest_event_date,
			venue_name,
			venue_city,
			upcoming_show_count
		FROM worklist
		ORDER BY soonest_event_date ASC, artist_name ASC, artist_id ASC
		LIMIT ? OFFSET ?
	`

	entries := make([]contracts.StreamingWorklistEntry, 0)
	if err := s.db.Raw(query, statusFilter, limit, offset).Scan(&entries).Error; err != nil {
		return nil, fmt.Errorf("list streaming worklist: %w", err)
	}

	// Total count uses the same status + future-show filter as the page
	// query (without the LATERAL projection of soonest-show fields).
	// EXISTS rather than JOIN — we only need "has at least one future
	// show".
	var total int64
	countQuery := `
		SELECT COUNT(*)
		FROM artists a
		WHERE a.streaming_discovery_status IN ?
		  AND EXISTS (
			SELECT 1
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE sa.artist_id = a.id
			  AND s.event_date >= NOW()
		  )
	`
	if err := s.db.Raw(countQuery, statusFilter).Scan(&total).Error; err != nil {
		return nil, fmt.Errorf("count streaming worklist: %w", err)
	}

	return &contracts.StreamingWorklistResult{
		Entries: entries,
		Total:   total,
	}, nil
}

// UpdateStreamingDiscoveryStatus validates the requested transition
// against allowedTransitions, then writes status + reason in a single
// UPDATE. Returns the updated row.
//
// Validation errors return ErrInvalidStreamingStatusTransition wrapped
// with a human-readable message; the handler unwraps and maps to a
// 400. ErrStreamingArtistNotFound is returned when the row doesn't
// exist (handler → 404).
func (s *StreamingWorklistService) UpdateStreamingDiscoveryStatus(input contracts.UpdateStreamingDiscoveryStatusInput) (*contracts.StreamingDiscoveryArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if input.ArtistID == 0 {
		return nil, fmt.Errorf("artist_id is required")
	}

	requested := catalogm.StreamingDiscoveryStatus(input.Status)
	if _, ok := validStreamingStatuses[requested]; !ok {
		return nil, fmt.Errorf("%w: %q is not a recognized streaming-discovery status", contracts.ErrInvalidStreamingStatusTransition, input.Status)
	}

	// Reject candidates_pending as a target — engine-set state, never
	// admin-set. Documented in allowedTransitions doc comment.
	if requested == catalogm.StreamingDiscoveryStatusCandidatesPending {
		return nil, fmt.Errorf("%w: candidates_pending is engine-set and cannot be assigned by an admin", contracts.ErrInvalidStreamingStatusTransition)
	}

	// Load the current row to validate the transition + return the
	// post-update payload. Two trips (SELECT then UPDATE) is acceptable
	// here — the endpoint is admin-only and low-traffic, and a single
	// UPDATE … RETURNING would require both validating the transition
	// in SQL (awkward) and a second round-trip to reject invalid
	// transitions with a clear error.
	var artist catalogm.Artist
	if err := s.db.First(&artist, input.ArtistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrStreamingArtistNotFound
		}
		return nil, fmt.Errorf("load artist for status update: %w", err)
	}

	current := artist.StreamingDiscoveryStatus
	if current == requested {
		return nil, fmt.Errorf("%w: status is already %q", contracts.ErrInvalidStreamingStatusTransition, input.Status)
	}

	legalTargets, ok := allowedTransitions[current]
	if !ok {
		// Defensive: a value passed the column CHECK at write time but
		// isn't in the transition map. Treat as invalid — the matrix
		// authoritative.
		return nil, fmt.Errorf("%w: current status %q has no legal transitions", contracts.ErrInvalidStreamingStatusTransition, current)
	}
	if _, ok := legalTargets[requested]; !ok {
		return nil, fmt.Errorf("%w: %q → %q is not allowed", contracts.ErrInvalidStreamingStatusTransition, current, requested)
	}

	// Reason normalization: empty string → NULL. Non-nil non-empty is
	// passed through. Re-opens (terminal → unreviewed) clear any prior
	// reason since the previous decision no longer applies.
	var nextReason *string
	if requested != catalogm.StreamingDiscoveryStatusUnreviewed && input.Reason != nil && *input.Reason != "" {
		nextReason = input.Reason
	}

	updates := map[string]interface{}{
		"streaming_discovery_status": string(requested),
		"streaming_discovery_reason": nextReason,
	}

	if err := s.db.Model(&catalogm.Artist{}).
		Where("id = ?", input.ArtistID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update artist streaming-discovery status: %w", err)
	}

	// Re-read so we return the canonical updated_at value the DB chose.
	if err := s.db.First(&artist, input.ArtistID).Error; err != nil {
		return nil, fmt.Errorf("reload artist after status update: %w", err)
	}

	return &contracts.StreamingDiscoveryArtistResponse{
		ID:                       artist.ID,
		Name:                     artist.Name,
		Slug:                     artist.Slug,
		StreamingDiscoveryStatus: string(artist.StreamingDiscoveryStatus),
		StreamingDiscoveryReason: artist.StreamingDiscoveryReason,
		UpdatedAt:                artist.UpdatedAt,
	}, nil
}
