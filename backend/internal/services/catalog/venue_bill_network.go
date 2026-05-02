package catalog

// PSY-365 — Venue co-bill network.
//
// Mirrors the scene-scale graph (PSY-367) but rooted at a single venue.
// Edges are weighted by the number of shared shows BETWEEN two artists
// AT THIS VENUE within the active time window — the "unfair advantage"
// signal called out in docs/research/knowledge-graph-viz-prior-art.md §6.
//
// Clustering decision (v1): ship WITHOUT explicit clusters. The scene-graph
// cluster signal is "primary venue per artist", which collapses at venue
// scope (every artist's primary venue is, by definition, this venue).
// Headliner-anchored circles would require a stable concept of "anchor
// headliner" we don't have, and time-period clusters would conflict with
// the time-window filter. The orchestrator brief explicitly allows shipping
// without clusters — see PR description for the rationale. The shared
// ForceGraphView still understands clusters when present, so adding the
// signal later is a payload change, not a contract change.

import (
	"fmt"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Time-window enum values. The handler validates the user's input and the
// service treats anything unrecognized as "all" so a malformed query param
// degrades gracefully rather than 500ing.
const (
	venueWindowAll     = "all"
	venueWindow12M     = "12m"
	venueWindowYear    = "year"
	venueWindowDefault = venueWindowAll

	// Minimum shared shows for a co-bill edge to surface. Mirrors the
	// production threshold for `shared_bills` in `DeriveSharedBills`
	// (see docs/features/similar-artists.md §"shared_bills" / minShows=2).
	venueBillMinSharedShows = 2

	// Sparse-venue empty-state threshold. Frontend handles the actual UX,
	// but the service surfaces ShowCount and ArtistCount in the response so
	// callers can short-circuit before even looking at edges.
	venueBillSparseShowsThreshold = 10
)

// venueBillSourceShow is one approved show at the venue, scoped to the active
// time window. We keep show_id + event_date so we can pair-up artists per
// show below and pluck "last shared" timestamps for the edge detail blob.
type venueBillSourceShow struct {
	ShowID    uint
	EventDate time.Time
}

// venueBillSourceArtistRow joins each (show, artist) at the venue. Used
// twice: once to build the artist set + at-venue counts, and once to derive
// pairwise edge weights.
type venueBillSourceArtistRow struct {
	ShowID    uint      `gorm:"column:show_id"`
	EventDate time.Time `gorm:"column:event_date"`
	ArtistID  uint      `gorm:"column:artist_id"`
}

// GetVenueBillNetwork returns the co-bill graph rooted at a venue. See the
// file-level comment for the clustering decision and edge-weight semantics.
func (s *VenueService) GetVenueBillNetwork(venueID uint, window string, year *int) (*contracts.VenueBillNetworkResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate venue exists. Reuse the existing 404 path so the handler can
	// surface it consistently with /venues/{id}/genres.
	venue, err := s.GetVenueModel(venueID)
	if err != nil {
		return nil, err
	}

	resolvedWindow := normalizeVenueWindow(window)
	startDate, endDate := resolveVenueWindowBounds(resolvedWindow, year)

	resp := &contracts.VenueBillNetworkResponse{
		Venue: contracts.VenueBillNetworkInfo{
			ID:    venue.ID,
			Slug:  derefString(venue.Slug),
			Name:  venue.Name,
			City:  venue.City,
			State: venue.State,
		},
		Clusters: []contracts.VenueBillNetworkCluster{},
		Nodes:    []contracts.VenueBillNetworkNode{},
		Links:    []contracts.VenueBillNetworkLink{},
	}

	switch resolvedWindow {
	case venueWindow12M:
		resp.Venue.Window = "last_12m"
	case venueWindowYear:
		resp.Venue.Window = "year"
		if year != nil {
			y := *year
			resp.Venue.Year = &y
		}
	default:
		resp.Venue.Window = "all_time"
	}

	// 1. Pull every (show, artist) pair at the venue within the window. One
	//    query — server-side filter by date, status, venue.
	rows, err := s.queryVenueBillSourceRows(venueID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query venue bill source rows: %w", err)
	}

	// 2. Aggregate: distinct shows + per-artist counts + per-show artist sets.
	//    `showsByID` is the de-duplicated set of approved shows; `artistsByID`
	//    is the de-duplicated artist set with at-venue counts and last-played
	//    timestamps; `byShow` is `show_id` → unique sorted artist list (used
	//    to enumerate co-bill pairs in step 4 without the pair-set blowing up
	//    on a show that listed an artist twice).
	showsByID := make(map[uint]venueBillSourceShow)
	type artistAggregate struct {
		ID               uint
		AtVenueShowCount int
		LastPlayedAt     time.Time
	}
	artistsByID := make(map[uint]*artistAggregate)
	byShow := make(map[uint]map[uint]struct{})
	for _, r := range rows {
		showsByID[r.ShowID] = venueBillSourceShow{ShowID: r.ShowID, EventDate: r.EventDate}
		agg, ok := artistsByID[r.ArtistID]
		if !ok {
			agg = &artistAggregate{ID: r.ArtistID}
			artistsByID[r.ArtistID] = agg
		}
		artists := byShow[r.ShowID]
		if artists == nil {
			artists = make(map[uint]struct{})
			byShow[r.ShowID] = artists
		}
		if _, dup := artists[r.ArtistID]; !dup {
			artists[r.ArtistID] = struct{}{}
			agg.AtVenueShowCount++
			if r.EventDate.After(agg.LastPlayedAt) {
				agg.LastPlayedAt = r.EventDate
			}
		}
	}

	resp.Venue.ShowCount = len(showsByID)
	resp.Venue.ArtistCount = len(artistsByID)

	if len(artistsByID) == 0 {
		return resp, nil
	}

	// 3. Hydrate artist names + slugs in one batch (the source rows don't
	//    carry display fields — `show_artists` is just the join table).
	artistIDs := make([]uint, 0, len(artistsByID))
	for id := range artistsByID {
		artistIDs = append(artistIDs, id)
	}
	artistDetails, err := s.batchArtistDetails(artistIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch artist details: %w", err)
	}
	upcomingByArtist := s.batchVenueBillUpcomingShowCount(artistIDs)

	// 4. Pairwise edge build. For each show with k artists, enumerate the
	//    k(k-1)/2 unique pairs and accumulate the count. Tracking
	//    LastSharedAt per pair lets the tooltip surface the same
	//    `(shared_count, last_shared)` shape that PSY-362 standardized.
	type pairKey struct {
		A uint
		B uint
	}
	type pairAggregate struct {
		Count        int
		LastSharedAt time.Time
	}
	pairs := make(map[pairKey]*pairAggregate)
	for showID, artistSet := range byShow {
		if len(artistSet) < 2 {
			continue
		}
		ids := make([]uint, 0, len(artistSet))
		for id := range artistSet {
			ids = append(ids, id)
		}
		// Sort ascending so canonical (min,max) pair key works. Insertion
		// sort — same shape as the existing helpers in this package.
		for i := 1; i < len(ids); i++ {
			for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
				ids[j], ids[j-1] = ids[j-1], ids[j]
			}
		}
		eventDate := showsByID[showID].EventDate
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				key := pairKey{A: ids[i], B: ids[j]}
				agg, ok := pairs[key]
				if !ok {
					agg = &pairAggregate{}
					pairs[key] = agg
				}
				agg.Count++
				if eventDate.After(agg.LastSharedAt) {
					agg.LastSharedAt = eventDate
				}
			}
		}
	}

	// 5. Build edges from pairs above threshold. Score = count / 10 capped at
	//    1.0 — same scaling as `DeriveSharedBills` so the frontend doesn't
	//    have to special-case venue edges. Mark connectivity for the isolate
	//    derivation at the end.
	connected := make(map[uint]bool, len(artistsByID))
	for key, agg := range pairs {
		if agg.Count < venueBillMinSharedShows {
			continue
		}
		score := float64(agg.Count) / 10.0
		if score > 1.0 {
			score = 1.0
		}
		// Inline the detail blob shape used by `DeriveSharedBills` so the
		// frontend buildLinkLabel formatter renders identically. Date in
		// YYYY-MM-DD per the existing radio + festival linkers.
		detail := map[string]any{
			"shared_count": agg.Count,
		}
		if !agg.LastSharedAt.IsZero() {
			detail["last_shared"] = agg.LastSharedAt.UTC().Format("2006-01-02")
		}
		resp.Links = append(resp.Links, contracts.VenueBillNetworkLink{
			SourceID:       key.A,
			TargetID:       key.B,
			Type:           catalogm.RelationshipTypeSharedBills,
			Score:          score,
			Detail:         detail,
			IsCrossCluster: false, // v1 has no clusters; ForceGraphView styles the edge anyway
		})
		connected[key.A] = true
		connected[key.B] = true
	}
	resp.Venue.EdgeCount = len(resp.Links)

	// 6. Build node list. ClusterID="other" because v1 ships without explicit
	//    clusters; the field is reserved for a future iteration that picks a
	//    venue-appropriate signal (see file-level comment).
	for _, id := range artistIDs {
		details, ok := artistDetails[id]
		if !ok {
			continue
		}
		agg := artistsByID[id]
		resp.Nodes = append(resp.Nodes, contracts.VenueBillNetworkNode{
			ID:                id,
			Name:              details.Name,
			Slug:              details.Slug,
			City:              details.City,
			State:             details.State,
			UpcomingShowCount: upcomingByArtist[id],
			ClusterID:         sceneClusterOtherID, // shared sentinel; ForceGraphView treats this as "ungrouped"
			IsIsolate:         !connected[id],
			AtVenueShowCount:  agg.AtVenueShowCount,
		})
	}

	return resp, nil
}

// normalizeVenueWindow coerces the caller's `window` string to a known value.
// Empty/unknown input falls back to "all" so a malformed query param degrades
// gracefully (same posture as the scene graph's `types` allowlist).
func normalizeVenueWindow(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case venueWindow12M:
		return venueWindow12M
	case venueWindowYear:
		return venueWindowYear
	default:
		return venueWindowDefault
	}
}

// resolveVenueWindowBounds translates the window to a [start, end) date range
// usable in the SQL filter. Returns zero values for the all-time case so the
// query helper can skip the date predicate.
//
// 12m: rolling — events from 12 months ago up to now.
// year: calendar year YYYY-01-01 inclusive to YYYY+1-01-01 exclusive. If
//
//	Year is nil, falls back to all-time (defensive — handler validates).
//
// all: no bounds.
func resolveVenueWindowBounds(window string, year *int) (time.Time, time.Time) {
	switch window {
	case venueWindow12M:
		now := time.Now().UTC()
		return now.AddDate(-1, 0, 0), now
	case venueWindowYear:
		if year == nil {
			return time.Time{}, time.Time{}
		}
		start := time.Date(*year, time.January, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(1, 0, 0)
		return start, end
	default:
		return time.Time{}, time.Time{}
	}
}

// queryVenueBillSourceRows fetches every (show_id, event_date, artist_id) at
// the venue, scoped to the active time window. The window is applied
// server-side so we don't haul down the venue's entire history when a filter
// is active.
func (s *VenueService) queryVenueBillSourceRows(venueID uint, startDate, endDate time.Time) ([]venueBillSourceArtistRow, error) {
	q := s.db.Table("show_artists sa").
		Select("sa.show_id AS show_id, s.event_date AS event_date, sa.artist_id AS artist_id").
		Joins("JOIN shows s ON s.id = sa.show_id").
		Joins("JOIN show_venues sv ON sv.show_id = sa.show_id").
		Where("sv.venue_id = ? AND s.status = ?", venueID, catalogm.ShowStatusApproved)
	if !startDate.IsZero() {
		q = q.Where("s.event_date >= ?", startDate)
	}
	if !endDate.IsZero() {
		q = q.Where("s.event_date < ?", endDate)
	}
	var rows []venueBillSourceArtistRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// venueBillArtistDetails carries the display fields for a graph node. Slim
// projection so we don't pull bandcamp/spotify/etc. just to render a label.
type venueBillArtistDetails struct {
	ID    uint
	Name  string
	Slug  string
	City  string
	State string
}

// batchArtistDetails returns a {artist_id → details} map for the given IDs.
// One query, indexed by primary key.
func (s *VenueService) batchArtistDetails(artistIDs []uint) (map[uint]venueBillArtistDetails, error) {
	out := make(map[uint]venueBillArtistDetails, len(artistIDs))
	if len(artistIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID    uint    `gorm:"column:id"`
		Name  string  `gorm:"column:name"`
		Slug  *string `gorm:"column:slug"`
		City  *string `gorm:"column:city"`
		State *string `gorm:"column:state"`
	}
	var rows []row
	if err := s.db.Table("artists").
		Select("id, name, slug, city, state").
		Where("id IN ?", artistIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = venueBillArtistDetails{
			ID:    r.ID,
			Name:  r.Name,
			Slug:  derefString(r.Slug),
			City:  derefString(r.City),
			State: derefString(r.State),
		}
	}
	return out, nil
}

// batchVenueBillUpcomingShowCount mirrors the scene-graph helper: per-artist
// upcoming approved show count globally (not just at this venue), so the
// graph node green-dot indicator stays consistent with the rest of the app.
func (s *VenueService) batchVenueBillUpcomingShowCount(artistIDs []uint) map[uint]int {
	out := make(map[uint]int, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID  uint
		ShowCount int64
	}
	var rows []row
	s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) AS show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.ArtistID] = int(r.ShowCount)
	}
	return out
}

// derefString returns the pointed-to string or "" when nil. Inline to avoid
// pulling in `lo` for one call site.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
