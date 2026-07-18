// Package explore implements the read-side service for the /explore
// landing endpoints. Surviving slices after PSY-1480: upcoming shows
// (chronological) and a random "shuffle target" artist for the
// surprise-me affordance. Featured Bill/Collection editorial slots
// were retired with the /explore → /graph cutover (PSY-1457/1480).
//
// There is intentionally NO trending / ranking algorithm here. The
// product decision (locked at the open-questions stage) is to start
// chronological + editorial, and only add algorithmic ranking if a real
// recommendations engine ever becomes warranted. Resist the temptation
// to backfill heuristics — they erode trust when they're wrong.
package explore

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Upper bound on the upcoming-shows page. Mirrors the bounds we use on
// other public list endpoints — keeps a single request from pulling
// thousands of rows + venue/artist joins.
const (
	maxUpcomingShowsLimit     = 50
	defaultUpcomingShowsLimit = 20
)

// Shuffle pool window — artists with a show inside this window are
// eligible to surface from the surprise-me affordance. Symmetric
// past/future to bias the pool toward currently-relevant artists
// (recent shows + upcoming shows both count). The 90-day choice is a
// product call, not a derived constant; tighten / widen here if the
// pool turns out to be too narrow or too broad in practice.
const shuffleWindowDays = 90

// ExploreService backs the /explore landing endpoints. The DB handle
// falls back to the package singleton so the bare-struct construction
// path used by older test fixtures still resolves a connection.
type ExploreService struct {
	db *gorm.DB
}

// NewExploreService constructs the /explore service.
func NewExploreService(database *gorm.DB) *ExploreService {
	if database == nil {
		database = db.GetDB()
	}
	return &ExploreService{db: database}
}

// ──────────────────────────────────────────────
// Upcoming Shows
// ──────────────────────────────────────────────

// GetUpcomingShows returns approved shows with event_date >= NOW(),
// ordered by event_date ASC then id ASC. The (event_date, id) tuple is
// strictly monotonic across the pagination window, so offset-based
// paging stays deterministic even when two shows share the same
// timestamp (festival lineups, parallel matinee/evening pairings).
//
// We deliberately limit the response to a compact projection
// (ExploreUpcomingShowItem) rather than the full ShowResponse — the
// /explore tile only needs headliner + venue + date. Skipping the
// per-show artist preload keeps the query at three round-trips total
// (shows page, batch venue lookup, batch headliner lookup) instead of
// the N+1 explosion the full ShowResponse builder would produce.
func (s *ExploreService) GetUpcomingShows(limit, offset int, cities []contracts.CityStateFilter) (*contracts.ExploreUpcomingShowsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = defaultUpcomingShowsLimit
	}
	if limit > maxUpcomingShowsLimit {
		limit = maxUpcomingShowsLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Timezone caveat (PSY-987): this list is deliberately UTC-bounded
	// (event_date >= now UTC) rather than "start of today in the viewer's
	// timezone" the way services/catalog.GetUpcomingShows is. /explore is
	// SSR-prefetched with no viewer context, and keeping the query
	// timezone-free is what lets that prefetch stay cacheable/seedable. The
	// only observable difference is a show that already started earlier *today*
	// in the viewer's zone but is still before UTC "now": it drops off this
	// list slightly early. The picker-count vs row-count consequence of that is
	// documented on applyUpcomingCityFilter below. Show *times* still render in
	// venue-local zones everywhere via the venue timezone (PSY-985/986); only
	// this list's day boundary is UTC.
	now := time.Now().UTC()

	// Count first so the response carries an authoritative total
	// without the page-size cap affecting it. Same WHERE clause as the
	// data query — no JOINs needed.
	var total int64
	countQuery := s.applyUpcomingCityFilter(
		s.db.Model(&catalogm.Show{}).
			Where("event_date >= ? AND status = ?", now, catalogm.ShowStatusApproved),
		cities,
	)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	var shows []catalogm.Show
	dataQuery := s.applyUpcomingCityFilter(
		s.db.Where("event_date >= ? AND status = ?", now, catalogm.ShowStatusApproved),
		cities,
	)
	if err := dataQuery.
		Order("event_date ASC, id ASC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("failed to list upcoming shows: %w", err)
	}

	items := make([]contracts.ExploreUpcomingShowItem, 0, len(shows))
	if len(shows) == 0 {
		return &contracts.ExploreUpcomingShowsResponse{
			Shows:  items,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	showIDs := make([]uint, len(shows))
	for i, sh := range shows {
		showIDs[i] = sh.ID
	}

	// Batch-fetch first venue per show. Most shows have exactly one;
	// when there are several we deterministically take the lowest
	// venue ID per show so the response is stable across requests.
	venueByShow := s.firstVenueByShow(showIDs)

	// Batch-fetch headliner artist per show. Prefer the row with
	// set_type='headliner'; fall back to position ASC if none.
	headlinerByShow := s.headlinerNameByShow(showIDs)

	for _, sh := range shows {
		item := contracts.ExploreUpcomingShowItem{
			ID:        sh.ID,
			Title:     sh.Title,
			EventDate: sh.EventDate,
			City:      sh.City,
			State:     sh.State,
		}
		if sh.Slug != nil {
			item.Slug = *sh.Slug
		}
		if v, ok := venueByShow[sh.ID]; ok {
			item.VenueName = v.Name
			item.VenueCity = v.City
			item.VenueState = v.State
		}
		if name, ok := headlinerByShow[sh.ID]; ok {
			item.HeadlinerName = name
		}
		items = append(items, item)
	}

	return &contracts.ExploreUpcomingShowsResponse{
		Shows:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// applyUpcomingCityFilter ANDs a multi-city (city, state) predicate onto
// q when cities is non-empty, mirroring services/catalog/show.go's
// GetUpcomingShows: it matches on shows.city / shows.state — the same
// columns the /explore response surfaces and GetShowCities groups by, so
// the picker offers exactly the cities this filter keys on (PSY-840).
//
// Count caveat: GetShowCities (the picker) counts with
// event_date >= start-of-today in the viewer's timezone, while this list
// uses event_date >= NOW() UTC — explore stays timezone-free so the
// page's SSR prefetch stays seedable. A city's picker count can therefore
// slightly exceed its filtered row count for a show that already started
// earlier today; the empty-state "Show all cities" affordance recovers
// the edge case where a selected city's only show has already started.
//
// Empty slice ⇒ q unchanged (all cities). The grouped OR-conditions are
// built on a fresh s.db session per the GORM group-condition idiom.
func (s *ExploreService) applyUpcomingCityFilter(q *gorm.DB, cities []contracts.CityStateFilter) *gorm.DB {
	if len(cities) == 0 {
		return q
	}
	conditions := s.db
	for i, cs := range cities {
		if i == 0 {
			conditions = conditions.Where("(city = ? AND state = ?)", cs.City, cs.State)
		} else {
			conditions = conditions.Or("(city = ? AND state = ?)", cs.City, cs.State)
		}
	}
	return q.Where(conditions)
}

// firstVenueByShow returns the lowest-ID venue per show ID. We index
// the per-show join with ORDER BY venue_id ASC so a show with two
// venue rows (rare) deterministically surfaces the same one across
// pagination pages.
func (s *ExploreService) firstVenueByShow(showIDs []uint) map[uint]catalogm.Venue {
	out := make(map[uint]catalogm.Venue, len(showIDs))
	if len(showIDs) == 0 {
		return out
	}

	type row struct {
		ShowID  uint
		VenueID uint
	}
	var pairs []row
	if err := s.db.Table("show_venues").
		Select("show_id, venue_id").
		Where("show_id IN ?", showIDs).
		Order("show_id ASC, venue_id ASC").
		Find(&pairs).Error; err != nil {
		// Best-effort: empty map → empty venue fields on the response.
		return out
	}

	venueIDs := make([]uint, 0, len(pairs))
	firstVenueIDByShow := make(map[uint]uint, len(showIDs))
	for _, p := range pairs {
		if _, claimed := firstVenueIDByShow[p.ShowID]; claimed {
			continue
		}
		firstVenueIDByShow[p.ShowID] = p.VenueID
		venueIDs = append(venueIDs, p.VenueID)
	}
	if len(venueIDs) == 0 {
		return out
	}

	var venues []catalogm.Venue
	if err := s.db.Where("id IN ?", venueIDs).Find(&venues).Error; err != nil {
		return out
	}
	venueByID := make(map[uint]catalogm.Venue, len(venues))
	for _, v := range venues {
		venueByID[v.ID] = v
	}
	for showID, venueID := range firstVenueIDByShow {
		if v, ok := venueByID[venueID]; ok {
			out[showID] = v
		}
	}
	return out
}

// headlinerNameByShow returns the headliner artist name per show ID.
// Selection rule: prefer set_type='headliner'; if no row is so flagged
// for a show, fall back to position ASC (the bill's first listed
// artist). Matches the existing convention in
// services/engagement/saved_show.go where set_type='headliner' is the
// canonical flag.
func (s *ExploreService) headlinerNameByShow(showIDs []uint) map[uint]string {
	out := make(map[uint]string, len(showIDs))
	if len(showIDs) == 0 {
		return out
	}

	var rows []catalogm.ShowArtist
	if err := s.db.
		Where("show_id IN ?", showIDs).
		Order("show_id ASC, (set_type = 'headliner') DESC, position ASC, artist_id ASC").
		Find(&rows).Error; err != nil {
		return out
	}

	chosen := make(map[uint]uint, len(showIDs)) // show_id -> artist_id
	artistIDs := make([]uint, 0, len(showIDs))
	for _, r := range rows {
		if _, taken := chosen[r.ShowID]; taken {
			continue
		}
		chosen[r.ShowID] = r.ArtistID
		artistIDs = append(artistIDs, r.ArtistID)
	}
	if len(artistIDs) == 0 {
		return out
	}

	var artists []catalogm.Artist
	if err := s.db.Where("id IN ?", artistIDs).Find(&artists).Error; err != nil {
		return out
	}
	nameByID := make(map[uint]string, len(artists))
	for _, a := range artists {
		nameByID[a.ID] = a.Name
	}
	for showID, artistID := range chosen {
		if name, ok := nameByID[artistID]; ok {
			out[showID] = name
		}
	}
	return out
}

// ──────────────────────────────────────────────
// Shuffle Target
// ──────────────────────────────────────────────

// GetShuffleTarget returns one random artist drawn from the pool of
// artists who have a show within ±90 days of NOW(). The pool is the
// distinct set of show_artists.artist_id where the parent show's
// event_date falls in the window. We use `ORDER BY random() LIMIT 1`
// directly — the qualifying pool is small enough (low thousands at
// peak) that the postgres sort scan is cheap, and we don't need
// repeatable selection across requests.
//
// When the pool is empty (very early production, test environments,
// etc.) all response fields are nil and the frontend can render a
// graceful "nothing to shuffle to" affordance.
func (s *ExploreService) GetShuffleTarget() (*contracts.ExploreShuffleTargetResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	windowStart := now.AddDate(0, 0, -shuffleWindowDays)
	windowEnd := now.AddDate(0, 0, shuffleWindowDays)

	// Postgres rejects `ORDER BY random()` directly on a SELECT
	// DISTINCT projection (the ORDER BY expression must appear in the
	// select list). Two-step: dedupe the artist IDs in a subquery,
	// then ORDER BY random() over the deduped pool. The optimizer
	// folds this into a single index scan + sort.
	type row struct {
		ID   uint
		Slug *string
		Name string
	}

	subquery := s.db.Table("show_artists AS sa").
		Select("DISTINCT sa.artist_id").
		Joins("JOIN shows ON shows.id = sa.show_id").
		Where("shows.event_date >= ? AND shows.event_date <= ? AND shows.status = ?",
			windowStart, windowEnd, catalogm.ShowStatusApproved)

	var picked row
	err := s.db.Table("artists").
		Select("artists.id, artists.slug, artists.name").
		Where("artists.id IN (?)", subquery).
		Order("random()").
		Limit(1).
		Scan(&picked).Error
	if err != nil {
		return nil, fmt.Errorf("failed to pick shuffle target: %w", err)
	}

	out := &contracts.ExploreShuffleTargetResponse{}
	if picked.ID == 0 {
		return out, nil
	}
	id := picked.ID
	name := picked.Name
	out.ArtistID = &id
	out.ArtistSlug = picked.Slug
	out.ArtistName = &name
	return out, nil
}
