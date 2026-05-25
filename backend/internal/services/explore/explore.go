// Package explore implements the read-side service for the /explore
// landing page. The page renders three slices: upcoming shows
// (chronological), the admin-curated featured bill + collection (from
// featured_slots — see PSY-835), and a random "shuffle target" artist
// for the surprise-me affordance.
//
// There is intentionally NO trending / ranking algorithm here. The
// product decision (locked at the open-questions stage) is to start
// chronological + editorial, and only add algorithmic ranking if a real
// recommendations engine ever becomes warranted. Resist the temptation
// to backfill heuristics — they erode trust when they're wrong.
package explore

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
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
// falls back to the package singleton so older test paths constructing
// the bare struct still resolve a connection. featuredSlots gives us
// the admin-curated picks; md renders curator notes + collection
// descriptions on read with the shared markdown stack.
type ExploreService struct {
	db            *gorm.DB
	featuredSlots contracts.FeaturedSlotServiceInterface
	md            *utils.MarkdownRenderer
	nowFn         func() time.Time
}

// NewExploreService constructs the /explore service with its
// dependencies. The featured-slot service is injected (rather than
// constructed inline) so handler integration tests can swap in a
// real instance from IntegrationDeps and unit tests can swap in a
// mock without standing up a Postgres container.
func NewExploreService(database *gorm.DB, featuredSlots contracts.FeaturedSlotServiceInterface) *ExploreService {
	if database == nil {
		database = db.GetDB()
	}
	return &ExploreService{
		db:            database,
		featuredSlots: featuredSlots,
		md:            utils.NewMarkdownRenderer(),
		nowFn:         time.Now,
	}
}

// SetNowFn overrides the clock used for "now" comparisons. Tests use
// this to make time-window queries deterministic — the alternative is
// inserting shows at hard-coded relative offsets and hoping no
// daylight-saving boundary shifts the assertion. Not for production use.
func (s *ExploreService) SetNowFn(fn func() time.Time) {
	if fn != nil {
		s.nowFn = fn
	}
}

func (s *ExploreService) now() time.Time {
	if s.nowFn == nil {
		return time.Now()
	}
	return s.nowFn()
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
func (s *ExploreService) GetUpcomingShows(limit, offset int) (*contracts.ExploreUpcomingShowsResponse, error) {
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

	now := s.now().UTC()

	// Count first so the response carries an authoritative total
	// without the page-size cap affecting it. Same WHERE clause as the
	// data query — no JOINs needed.
	var total int64
	if err := s.db.Model(&catalogm.Show{}).
		Where("event_date >= ? AND status = ?", now, catalogm.ShowStatusApproved).
		Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	var shows []catalogm.Show
	if err := s.db.
		Where("event_date >= ? AND status = ?", now, catalogm.ShowStatusApproved).
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
// Featured (admin-curated)
// ──────────────────────────────────────────────

// GetFeatured returns the currently active featured bill + collection.
//
// IMPORTANT: featured_slots.entity_id is polymorphic with NO database
// FK (the referent table varies by slot_type — shows or collections).
// The referent may have been deleted, made private, or otherwise
// removed from the public surface after the slot was set. We defensively
// LEFT-JOIN here so a stale slot returns nil for that field rather
// than 500-ing the entire response.
//
// Privacy rule for the collection slot: a collection that has been
// flipped to private (is_public=false) is excluded — we don't want to
// leak a private collection's title/description through the /explore
// landing just because someone privately featured it earlier. A new
// admin pick or a flip back to public restores visibility.
func (s *ExploreService) GetFeatured() (*contracts.ExploreFeaturedResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	out := &contracts.ExploreFeaturedResponse{}

	bill, err := s.resolveFeaturedBill()
	if err != nil {
		return nil, err
	}
	out.Bill = bill

	coll, err := s.resolveFeaturedCollection()
	if err != nil {
		return nil, err
	}
	out.Collection = coll

	return out, nil
}

// resolveFeaturedBill fetches the active bill slot and, if its
// referent still exists + is publicly visible (approved status), hydrates
// the response item. Returns (nil, nil) when there's no active slot or
// the referent is gone — distinguishes "nothing curated" / "stale
// referent" from real I/O failures.
func (s *ExploreService) resolveFeaturedBill() (*contracts.ExploreFeaturedBill, error) {
	slot, err := s.activeSlotOrNil(adminm.FeaturedSlotTypeBill)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, nil
	}

	var show catalogm.Show
	err = s.db.
		Where("id = ? AND status = ?", slot.EntityID, catalogm.ShowStatusApproved).
		First(&show).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Referent deleted or no longer approved — collapse the
			// slot gracefully rather than orphaning the response.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load featured bill referent: %w", err)
	}

	venues := s.firstVenueByShow([]uint{show.ID})
	headliners := s.headlinerNameByShow([]uint{show.ID})

	item := &contracts.ExploreFeaturedBill{
		ID:              show.ID,
		Title:           show.Title,
		EventDate:       show.EventDate,
		ImageURL:        show.ImageURL,
		CuratorNote:     slot.CuratorNote,
		CuratorNoteHTML: s.renderCuratorNote(slot.CuratorNote),
	}
	if show.Slug != nil {
		item.Slug = *show.Slug
	}
	if v, ok := venues[show.ID]; ok {
		item.VenueName = v.Name
		item.VenueCity = v.City
		item.VenueState = v.State
	}
	if name, ok := headliners[show.ID]; ok {
		item.HeadlinerName = name
	}
	return item, nil
}

// resolveFeaturedCollection mirrors resolveFeaturedBill for the
// Featured Collection slot. Privacy guard: a collection flipped to
// private after being featured collapses to nil here so the /explore
// landing doesn't leak its title/description. A subsequent flip back
// to public (or a new admin pick) restores the surface.
func (s *ExploreService) resolveFeaturedCollection() (*contracts.ExploreFeaturedCollection, error) {
	slot, err := s.activeSlotOrNil(adminm.FeaturedSlotTypeCollection)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, nil
	}

	var coll communitym.Collection
	err = s.db.
		Where("id = ? AND is_public = ?", slot.EntityID, true).
		First(&coll).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load featured collection referent: %w", err)
	}

	return &contracts.ExploreFeaturedCollection{
		ID:              coll.ID,
		Slug:            coll.Slug,
		Title:           coll.Title,
		Description:     coll.Description,
		DescriptionHTML: s.renderMarkdown(coll.Description),
		CoverImageURL:   coll.CoverImageURL,
		CuratorNote:     slot.CuratorNote,
		CuratorNoteHTML: s.renderCuratorNote(slot.CuratorNote),
	}, nil
}

// activeSlotOrNil wraps FeaturedSlotService.GetActiveSlot but translates
// the ErrFeaturedSlotNotFound sentinel into (nil, nil) so callers can
// branch on the not-found case without importing the admin-service
// sentinel themselves. Real I/O failures are propagated unchanged.
// Tolerates a nil featuredSlots dependency for legacy test paths that
// construct ExploreService without wiring the admin services.
func (s *ExploreService) activeSlotOrNil(slotType string) (*adminm.FeaturedSlot, error) {
	if s.featuredSlots == nil {
		return nil, nil
	}
	slot, err := s.featuredSlots.GetActiveSlot(slotType)
	if err != nil {
		if errors.Is(err, adminsvc.ErrFeaturedSlotNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load active featured slot (%s): %w", slotType, err)
	}
	return slot, nil
}

// renderCuratorNote delegates to FeaturedSlotService when available so
// both surfaces (admin + /explore) share one rendering boundary. Falls
// back to the local MarkdownRenderer when the dependency isn't wired
// (test paths).
func (s *ExploreService) renderCuratorNote(note *string) string {
	if note == nil || *note == "" {
		return ""
	}
	if s.featuredSlots != nil {
		return s.featuredSlots.RenderCuratorNote(note)
	}
	return s.renderMarkdown(*note)
}

func (s *ExploreService) renderMarkdown(body string) string {
	if body == "" {
		return ""
	}
	if s.md == nil {
		s.md = utils.NewMarkdownRenderer()
	}
	return s.md.Render(body)
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

	now := s.now().UTC()
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
