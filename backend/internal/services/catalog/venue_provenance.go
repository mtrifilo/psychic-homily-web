package catalog

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Community freshness for venues (PSY-1542).
//
// A crowdsourced venue map dies of staleness nobody can see. The cheap
// maintenance loop that keeps one alive is two-sided: a one-tap "this is still
// accurate" that costs a contributor nothing and edits nothing, and a
// provenance stamp that makes the age of a listing visible so a stale one is
// obvious rather than silently wrong.
//
// This file owns both halves. Neither denormalises a counter onto venues —
// every number is aggregated at read time, batched over a page of venue IDs
// the same way the rail's other aggregations are (see venue_rail.go). The
// aggregations are BEST EFFORT for the same reason: a missing stamp degrades a
// row, a failed list degrades the page.

// ConfirmVenue records that userID vouches for this venue's info being current.
//
// Idempotent by construction: the composite PK (user_id, venue_id) makes the
// row unique and ON CONFLICT DO NOTHING turns a repeat tap into a no-op that
// returns the same aggregate. This is the point of the mechanic — a
// contributor should be able to tap without wondering whether they already
// did, and without an error being the answer.
//
// Any authenticated user at any trust tier may confirm. Gating the cheapest
// possible contribution behind a trust tier defeats its purpose: it is the
// on-ramp, not the reward.
func (s *VenueService) ConfirmVenue(venueID uint, userID uint) (*contracts.VenueConfirmationResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if userID == 0 {
		return nil, fmt.Errorf("confirm requires an authenticated user")
	}

	// Resolve the venue first so confirming a nonexistent venue is a clean 404
	// rather than an FK violation surfaced as a 500.
	var venue catalogm.Venue
	if err := s.db.Select("id").First(&venue, venueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to load venue for confirmation: %w", err)
	}

	if err := s.db.Exec(
		"INSERT INTO venue_confirmations (user_id, venue_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		userID, venue.ID,
	).Error; err != nil {
		return nil, fmt.Errorf("failed to confirm venue: %w", err)
	}

	counts, err := s.venueConfirmationCounts([]uint{venue.ID})
	if err != nil {
		return nil, fmt.Errorf("failed to read confirmation aggregate: %w", err)
	}
	agg := counts[venue.ID]

	return &contracts.VenueConfirmationResponse{
		ConfirmationCount: agg.Count,
		LastConfirmedAt:   agg.LastConfirmedAt,
		// True on the first confirm and on every idempotent repeat: the row
		// exists either way, which is exactly what the client renders.
		ViewerHasConfirmed: true,
	}, nil
}

// venueConfirmationAggregate is the per-venue confirmation rollup.
type venueConfirmationAggregate struct {
	Count           int
	LastConfirmedAt *time.Time
}

// venueConfirmationCounts returns, per venue ID, how many distinct users have
// confirmed it and when the most recent confirmation landed. One indexed scan
// for the whole page (idx_venue_confirmations_venue_id), never one per venue.
func (s *VenueService) venueConfirmationCounts(venueIDs []uint) (map[uint]venueConfirmationAggregate, error) {
	out := make(map[uint]venueConfirmationAggregate, len(venueIDs))
	if len(venueIDs) == 0 {
		return out, nil
	}

	type row struct {
		VenueID         uint       `gorm:"column:venue_id"`
		Count           int        `gorm:"column:count"`
		LastConfirmedAt *time.Time `gorm:"column:last_confirmed_at"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT venue_id, COUNT(*) AS count, MAX(created_at) AS last_confirmed_at
		FROM venue_confirmations
		WHERE venue_id IN ?
		GROUP BY venue_id
	`, venueIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate venue confirmations: %w", err)
	}

	for _, r := range rows {
		out[r.VenueID] = venueConfirmationAggregate{Count: r.Count, LastConfirmedAt: r.LastConfirmedAt}
	}
	return out, nil
}

// venueEditAggregate is the per-venue community-edit rollup.
type venueEditAggregate struct {
	EditCount        int
	ContributorCount int
}

// venueEditCounts returns, per venue ID, how many APPROVED community edits the
// venue has and how many distinct people made them.
//
// Source is pending_entity_edits, not entity_edit_audit_logs. Both tables
// exist, but the venue edit path (PUT /venues/{id}/suggest-edit, including the
// trusted-user auto-approve branch) deliberately writes ONLY the
// pending_entity_edits row — PSY-618 removed the parallel audit emit because
// it double-rendered in the contributor feed. Counting the audit table here
// would therefore report zero edits for every venue.
//
// Pending and rejected rows are excluded: a proposal that was never applied did
// not change what the reader is looking at, and counting it would inflate the
// stamp. Indexed by idx_pending_entity_edits_entity, so this is one scan for a
// whole page of venues.
func (s *VenueService) venueEditCounts(venueIDs []uint) (map[uint]venueEditAggregate, error) {
	out := make(map[uint]venueEditAggregate, len(venueIDs))
	if len(venueIDs) == 0 {
		return out, nil
	}

	type row struct {
		EntityID         uint `gorm:"column:entity_id"`
		EditCount        int  `gorm:"column:edit_count"`
		ContributorCount int  `gorm:"column:contributor_count"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT entity_id,
		       COUNT(*) AS edit_count,
		       COUNT(DISTINCT submitted_by) AS contributor_count
		FROM pending_entity_edits
		WHERE entity_type = ? AND status = ? AND entity_id IN ?
		GROUP BY entity_id
	`, adminm.PendingEditEntityVenue, adminm.PendingEditStatusApproved, venueIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate venue edits: %w", err)
	}

	for _, r := range rows {
		out[r.EntityID] = venueEditAggregate{EditCount: r.EditCount, ContributorCount: r.ContributorCount}
	}
	return out, nil
}

// buildVenueProvenance assembles one venue's stamp from the batched rollups.
//
// dataSource is venues.data_source verbatim — the only stored evidence that a
// row came from an ingest/enrichment writer. It is NOT inferred from a missing
// submitted_by or from anything else: an unpopulated column yields no "ingest"
// source rather than a plausible guess.
func buildVenueProvenance(updatedAt time.Time, dataSource *string, edits venueEditAggregate, confirms venueConfirmationAggregate) *contracts.VenueProvenance {
	sources := make([]string, 0, 2)
	if dataSource != nil && *dataSource != "" {
		sources = append(sources, contracts.VenueProvenanceSourceIngest)
	}
	if edits.EditCount > 0 || confirms.Count > 0 {
		sources = append(sources, contracts.VenueProvenanceSourceCommunity)
	}

	return &contracts.VenueProvenance{
		UpdatedAt:         updatedAt,
		EditCount:         edits.EditCount,
		ContributorCount:  edits.ContributorCount,
		ConfirmationCount: confirms.Count,
		LastConfirmedAt:   confirms.LastConfirmedAt,
		Sources:           sources,
	}
}

// loadProvenanceAggregates runs both rollups for a set of venue IDs.
//
// BEST EFFORT by contract: an aggregation that fails is logged and returned as
// a nil map, so its counts read as zero. The stamp still carries the updated-at
// timestamp, which is the half readers depend on most, and a failed count must
// never blank a venue list or a venue page.
func (s *VenueService) loadProvenanceAggregates(venueIDs []uint) (map[uint]venueEditAggregate, map[uint]venueConfirmationAggregate) {
	edits, err := s.venueEditCounts(venueIDs)
	if err != nil {
		slog.Default().Error("venue edit-count aggregation failed; provenance renders without edit counts",
			"venue_ids", venueIDs, "error", err)
		edits = nil
	}
	confirms, err := s.venueConfirmationCounts(venueIDs)
	if err != nil {
		slog.Default().Error("venue confirmation aggregation failed; provenance renders without confirmations",
			"venue_ids", venueIDs, "error", err)
		confirms = nil
	}
	return edits, confirms
}

// enrichVenueProvenance fills the provenance stamp on an already-built page of
// venue responses, in place.
func (s *VenueService) enrichVenueProvenance(responses []*contracts.VenueWithShowCountResponse, dataSources map[uint]*string) {
	if len(responses) == 0 {
		return
	}
	venueIDs := make([]uint, 0, len(responses))
	for _, r := range responses {
		venueIDs = append(venueIDs, r.ID)
	}

	edits, confirms := s.loadProvenanceAggregates(venueIDs)
	for _, r := range responses {
		r.Provenance = buildVenueProvenance(r.UpdatedAt, dataSources[r.ID], edits[r.ID], confirms[r.ID])
	}
}

// venueProvenanceFor builds the stamp for a single venue (the venue detail
// read). Same aggregations as the page path, batched over a page of one.
func (s *VenueService) venueProvenanceFor(venue *catalogm.Venue) *contracts.VenueProvenance {
	edits, confirms := s.loadProvenanceAggregates([]uint{venue.ID})
	return buildVenueProvenance(venue.UpdatedAt, venue.DataSource, edits[venue.ID], confirms[venue.ID])
}
