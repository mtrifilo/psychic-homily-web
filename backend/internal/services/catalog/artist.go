package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// ArtistService handles artist-related business logic
type ArtistService struct {
	db *gorm.DB
	// bandcampResolver resolves a *.bandcamp.com profile root → a featured
	// /album|/track embed URL (PSY-1190). Injectable so tests can point it at an
	// httptest server (SetBandcampResolver); nil disables profile resolution
	// (the fill is a no-op), so a service built without one simply never resolves.
	bandcampResolver *BandcampProfileResolver
	// dispatchAsync runs fire-and-forget post-write work (the Bandcamp profile
	// fetch) OFF the request goroutine, so a CreateArtist/UpdateArtist HTTP request
	// returns immediately instead of blocking up to bandcampFetchTimeout on a slow
	// Bandcamp page. Production uses shared.GoSafe (a panic-recovering goroutine);
	// tests override it (SetSyncDispatch) to run the work inline so they can assert
	// the embed was filled without racing the goroutine.
	dispatchAsync func(name string, work func())
}

// NewArtistService creates a new artist service
func NewArtistService(database *gorm.DB) *ArtistService {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistService{
		db:               database,
		bandcampResolver: NewBandcampProfileResolver(),
		dispatchAsync: func(name string, work func()) {
			shared.GoSafe(context.Background(), name, work)
		},
	}
}

// SetBandcampResolver overrides the profile→album resolver (tests inject one
// pointed at an httptest server). Passing nil disables profile resolution.
func (s *ArtistService) SetBandcampResolver(r *BandcampProfileResolver) {
	s.bandcampResolver = r
}

// SetSyncDispatch makes post-write resolution run inline on the calling
// goroutine instead of in a fire-and-forget goroutine. Tests use this so they can
// assert the resolved embed landed without waiting on a goroutine.
func (s *ArtistService) SetSyncDispatch() {
	s.dispatchAsync = func(_ string, work func()) { work() }
}

// FillProfileResolvedEmbedFromBandcamp resolves a Bandcamp PROFILE root → a
// featured embed and fills artistID's bandcamp_embed_url (profile_resolved,
// fill-when-empty) — the SAME dispatch the artist Create/Update paths use. It is
// the exported entry point for OTHER write paths that set social.bandcamp WITHOUT
// going through ArtistService.UpdateArtist — specifically the pending-edit
// approval flow (community + trusted-tier inline edits apply a `bandcamp` change
// via a direct UPDATE), which would otherwise leave the embed unresolved on a
// common "set a Bandcamp profile" path. A non-profile/empty value is a no-op.
func (s *ArtistService) FillProfileResolvedEmbedFromBandcamp(artistID uint, profileURL string) {
	s.runProfileResolve(artistID, profileURL)
}

// runProfileResolve dispatches the profile→embed fill for artistID through
// dispatchAsync (a GoSafe goroutine in prod; inline when a test opted in via
// SetSyncDispatch). A nil dispatcher — a service built via &ArtistService{}
// without the constructor — defaults to the PRODUCTION goroutine behavior, NOT
// inline: that way a test that wires a resolver (SetBandcampResolver) but forgets
// SetSyncDispatch gets the same async path production does (and must opt into sync
// to assert the fill), rather than a silent inline path that diverges from prod.
// The goroutine is panic-recovered (GoSafe) and no-ops when the resolver is nil
// (the legacy direct-construction suites) before touching the DB, so a forgotten
// dispatcher never spawns a DB-touching goroutine.
func (s *ArtistService) runProfileResolve(artistID uint, profileURL string) {
	// Gate on the cheap, pure profile-root classifier BEFORE spawning a goroutine,
	// so a non-profile value (an /album|/track link, a cleared/empty value, a
	// non-Bandcamp URL — the common case on a whole-form artist update) never
	// dispatches one. Also skip when no resolver is configured (legacy suites).
	if s.bandcampResolver == nil || !isBandcampProfileRoot(profileURL) {
		return
	}
	work := func() {
		s.resolveProfileEmbedForArtist(context.Background(), artistID, profileURL)
	}
	dispatch := s.dispatchAsync
	if dispatch == nil {
		dispatch = func(name string, w func()) { shared.GoSafe(context.Background(), name, w) }
	}
	dispatch("bandcamp_profile_resolve", work)
}

// CreateArtist creates a new artist
func (s *ArtistService) CreateArtist(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Single artist write path (PSY-1254): dedup by name, unique slug, insert (and
	// PSY-1247 will add image-enrichment enqueue here). The admin create is explicit,
	// so a found artist is an error, not a silent reuse — note the funnel may backfill
	// that found artist's missing slug (a committed write) before we return the error.
	artist, created, err := FindOrCreateArtistTx(s.db, req.Name, func(a *catalogm.Artist) {
		a.State = req.State
		a.City = req.City
		a.Country = req.Country
		// (metro is derived from this location by the create funnel — PSY-1255 step B.)
		a.Description = req.Description
		a.ImageURL = req.ImageURL
		a.BandcampEmbedURL = req.BandcampEmbedURL
		// Stamp provenance whenever this create sets an embed (a human/admin/AI
		// value): "manual" so the PSY-1189 keep-fresh hook never auto-refreshes a
		// curated embed. nil when no embed is supplied so the source isn't falsely
		// claimed (PSY-1188).
		a.BandcampEmbedSource = manualEmbedSourceIfSet(req.BandcampEmbedURL)
		a.Social = catalogm.Social{
			Instagram:  req.Instagram,
			Facebook:   req.Facebook,
			Twitter:    req.Twitter,
			YouTube:    req.YouTube,
			Spotify:    req.Spotify,
			SoundCloud: req.SoundCloud,
			Bandcamp:   req.Bandcamp,
			Website:    req.Website,
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create artist: %w", err)
	}
	if !created {
		return nil, apperrors.ErrArtistExists(req.Name)
	}

	// PSY-1190: when this create set a Bandcamp PROFILE root (not an embeddable
	// album/track URL) and supplied no embed, resolve the profile → a featured
	// album URL and fill bandcamp_embed_url (profile_resolved, fill-when-empty).
	// Dispatched off the request goroutine (a network fetch); a no-op when no
	// profile was set or the embed was supplied above (the resolver's IS NULL
	// guard skips the manually-stamped row anyway).
	if req.Bandcamp != nil {
		s.runProfileResolve(artist.ID, *req.Bandcamp)
	}

	return s.buildArtistResponse(artist), nil
}

// GetArtist retrieves an artist by ID
func (s *ArtistService) GetArtist(artistID uint) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist catalogm.Artist
	err := s.db.First(&artist, artistID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	resp := s.buildArtistResponse(&artist)
	resp.Stats = s.buildArtistStats(artist.ID)
	return resp, nil
}

// GetArtistByName retrieves an artist by name (case-insensitive)
func (s *ArtistService) GetArtistByName(name string) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist catalogm.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&artist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(0)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtistBySlug retrieves an artist by slug
func (s *ArtistService) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist catalogm.Artist
	err := s.db.Where("slug = ?", slug).First(&artist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(0)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	resp := s.buildArtistResponse(&artist)
	resp.Stats = s.buildArtistStats(artist.ID)
	return resp, nil
}

// GetArtists retrieves artists with optional filtering
func (s *ArtistService) GetArtists(filters map[string]interface{}) ([]*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db

	// Apply filters
	if cities, ok := filters["cities"].([]map[string]string); ok && len(cities) > 0 {
		// Multi-city filter: (city = ? AND state = ?) OR ...
		var conditions []string
		var args []interface{}
		for _, cs := range cities {
			if cs["city"] != "" && cs["state"] != "" {
				conditions = append(conditions, "(city = ? AND state = ?)")
				args = append(args, cs["city"], cs["state"])
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if state, ok := filters["state"].(string); ok && state != "" {
			query = query.Where("state = ?", state)
		}
		if city, ok := filters["city"].(string); ok && city != "" {
			query = query.Where("city = ?", city)
		}
	}
	if name, ok := filters["name"].(string); ok && name != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", shared.LikePattern(name))
	}
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		query = ApplyTagFilter(query, s.db, catalogm.TagEntityArtist, "artists.id", tf)
	}

	// Default ordering by name
	query = query.Order("name ASC")

	var artists []catalogm.Artist
	err := query.Find(&artists).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get artists: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ArtistDetailResponse, len(artists))
	for i, artist := range artists {
		responses[i] = s.buildArtistResponse(&artist)
	}

	return responses, nil
}

// locationFieldChanged reports whether a UpdateArtist updates map touches any
// location column, i.e. whether the artist's metro may need recomputing.
func locationFieldChanged(updates map[string]interface{}) bool {
	for _, k := range []string{"city", "state", "country"} {
		if _, ok := updates[k]; ok {
			return true
		}
	}
	return false
}

// effectiveLocField returns the post-update value of a location field: the value
// just staged in the updates map (a *string from NilIfEmpty, possibly nil) when
// the request changed it, else the current stored value.
func effectiveLocField(updates map[string]interface{}, key string, current *string) string {
	if v, ok := updates[key]; ok {
		if p, ok := v.(*string); ok {
			return derefString(p)
		}
		return "" // unreachable: location keys are always staged as *string (NilIfEmpty)
	}
	return derefString(current)
}

// UpdateArtist updates an existing artist
func (s *ArtistService) UpdateArtist(artistID uint, req *contracts.UpdateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Translate the typed request into a GORM update map. Only non-nil fields
	// are written so omitted fields stay unchanged. Every column is nullable
	// except name, so empty input normalizes to SQL NULL via utils.NilIfEmpty.
	updates := map[string]interface{}{}

	// Name maps to a NOT NULL column and additionally drives the uniqueness
	// guard and slug regeneration.
	if req.Name != nil {
		name := *req.Name
		var existingArtist catalogm.Artist
		err := s.db.Where("LOWER(name) = LOWER(?) AND id != ?", name, artistID).First(&existingArtist).Error
		if err == nil {
			return nil, fmt.Errorf("artist with name '%s' already exists", name)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to check existing artist: %w", err)
		}
		updates["name"] = name

		// Regenerate slug when name changes
		baseSlug := utils.GenerateArtistSlug(name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.Artist{}).Where("slug = ? AND id != ?", candidate, artistID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}
	if req.State != nil {
		updates["state"] = utils.NilIfEmpty(*req.State)
	}
	if req.City != nil {
		updates["city"] = utils.NilIfEmpty(*req.City)
	}
	if req.Country != nil {
		updates["country"] = utils.NilIfEmpty(*req.Country)
	}
	// metro is derived from (city, state, country) — recompute it when this admin
	// edit relocates the artist, so it stays a sibling of the location (like the
	// venue write paths, PSY-1255 step B). cmd/backfill-entity-metro is the
	// backstop for the background enrichment / state-correction passes that change
	// location without going through here.
	if locationFieldChanged(updates) {
		var cur catalogm.Artist
		if err := s.db.Select("city", "state", "country").First(&cur, artistID).Error; err == nil {
			updates["metro"] = geo.MetroPointer(geo.Default(),
				effectiveLocField(updates, "city", cur.City),
				effectiveLocField(updates, "state", cur.State),
				effectiveLocField(updates, "country", cur.Country))
		}
	}
	if req.Description != nil {
		updates["description"] = utils.NilIfEmpty(*req.Description)
	}
	if req.BandcampEmbedURL != nil {
		embed := utils.NilIfEmpty(*req.BandcampEmbedURL)
		updates["bandcamp_embed_url"] = embed
		// Keep the provenance column in lockstep with the URL on every write
		// through this typed path (admin endpoint, AI fulfiller): a non-empty
		// value is a human/admin/AI edit → "manual"; clearing the embed also
		// clears the source (PSY-1188). Both are explicit map entries so GORM
		// writes the NULL on a clear rather than skipping a zero value.
		if embed != nil {
			updates["bandcamp_embed_source"] = catalogm.BandcampEmbedSourceManual
		} else {
			updates["bandcamp_embed_source"] = nil
		}
	}
	if req.Instagram != nil {
		updates["instagram"] = utils.NilIfEmpty(*req.Instagram)
	}
	if req.Facebook != nil {
		updates["facebook"] = utils.NilIfEmpty(*req.Facebook)
	}
	if req.Twitter != nil {
		updates["twitter"] = utils.NilIfEmpty(*req.Twitter)
	}
	if req.YouTube != nil {
		updates["youtube"] = utils.NilIfEmpty(*req.YouTube)
	}
	if req.Spotify != nil {
		updates["spotify"] = utils.NilIfEmpty(*req.Spotify)
	}
	if req.SoundCloud != nil {
		updates["soundcloud"] = utils.NilIfEmpty(*req.SoundCloud)
	}
	if req.Bandcamp != nil {
		updates["bandcamp"] = utils.NilIfEmpty(*req.Bandcamp)
	}
	if req.Website != nil {
		updates["website"] = utils.NilIfEmpty(*req.Website)
	}

	if len(updates) > 0 {
		err := s.db.Model(&catalogm.Artist{}).Where("id = ?", artistID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update artist: %w", err)
		}
	}

	// PSY-1190: when this update set the Bandcamp social link to a PROFILE root
	// (band.bandcamp.com, not an /album|/track URL) and the artist's embed is
	// still NULL, resolve the profile → a featured album URL and fill
	// bandcamp_embed_url (profile_resolved, fill-when-empty; a manual value is
	// left untouched by the resolver's IS NULL guard). Dispatched off the request
	// goroutine (a network fetch), after the write commits.
	if req.Bandcamp != nil {
		s.runProfileResolve(artistID, *req.Bandcamp)
	}

	return s.GetArtist(artistID)
}

// DeleteArtist deletes an artist
func (s *ArtistService) DeleteArtist(artistID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if artist exists
	var artist catalogm.Artist
	err := s.db.First(&artist, artistID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrArtistNotFound(artistID)
		}
		return fmt.Errorf("failed to get artist: %w", err)
	}

	// Check if artist is associated with any shows
	var count int64
	err = s.db.Model(&catalogm.ShowArtist{}).Where("artist_id = ?", artistID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check artist associations: %w", err)
	}

	if count > 0 {
		return apperrors.ErrArtistHasShows(artistID, count)
	}

	// Delete the artist
	err = s.db.Delete(&artist).Error
	if err != nil {
		return fmt.Errorf("failed to delete artist: %w", err)
	}

	return nil
}

// SearchArtists performs autocomplete search on artist names and aliases.
// Uses pg_trgm extension for performant fuzzy matching with intelligent query strategy:
// - Short queries (1-2 chars): Fast case-insensitive prefix search
// - Longer queries (3+ chars): Similarity-based fuzzy matching with ranking
// Alias matches return the canonical artist (deduplicated).
func (s *ArtistService) SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Return empty results for empty query
	if query == "" {
		return []*contracts.ArtistDetailResponse{}, nil
	}

	var artists []catalogm.Artist
	var err error

	// Strategy depends on query length for optimal performance
	if len(query) <= 2 {
		// For short queries: use fast case-insensitive prefix search on name + aliases
		prefixPattern := shared.LikePrefixPattern(query)
		err = s.db.
			Where("id IN (?)",
				s.db.Table("artists").Select("id").Where("LOWER(name) LIKE LOWER(?)", prefixPattern),
			).
			Or("id IN (?)",
				s.db.Table("artist_aliases").Select("artist_id").Where("LOWER(alias) LIKE LOWER(?)", prefixPattern),
			).
			Order("name ASC").
			Limit(10).
			Find(&artists).Error
	} else {
		// For longer queries: search names and aliases with similarity scoring.
		// Uses UNION to find matching artists by name or alias, then ranks by best similarity.
		// `name % ?` / `alias % ?` are the pg_trgm similarity operator and take the
		// raw term; the ILIKE branches escape via LikePattern.
		substringPattern := shared.LikePattern(query)
		err = s.db.Raw(`
			SELECT a.* FROM artists a
			WHERE a.id IN (
				SELECT id FROM artists WHERE name ILIKE ? OR name % ?
				UNION
				SELECT artist_id FROM artist_aliases WHERE alias ILIKE ? OR alias % ?
			)
			ORDER BY GREATEST(
				similarity(a.name, ?),
				COALESCE((SELECT MAX(similarity(aa.alias, ?)) FROM artist_aliases aa WHERE aa.artist_id = a.id), 0)
			) DESC, a.name ASC
			LIMIT 10
		`, substringPattern, query, substringPattern, query, query, query).
			Scan(&artists).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search artists: %w", err)
	}

	// Build response
	responses := make([]*contracts.ArtistDetailResponse, len(artists))
	for i, artist := range artists {
		responses[i] = s.buildArtistResponse(&artist)
	}

	return responses, nil
}

// ArtistWithCount is used internally for querying artists with their show counts
type ArtistWithCount struct {
	catalogm.Artist
	UpcomingShowCount int64      `gorm:"column:upcoming_show_count"`
	LastShowDate      *time.Time `gorm:"column:last_show_date"`
}

// contracts.ArtistWithShowCountResponse represents an artist with its upcoming show count

// GetArtistsWithShowCounts retrieves artists with their upcoming show counts.
//
// By default the result is gated on "has at least one upcoming approved show"
// (the /artists landing/browse flow). When `skip_active_filter` is set to true
// in the filters map (PSY-495: tag-filter engaged → Bandcamp-style evergreen
// discovery), the activity gate is dropped and every matching artist is
// returned, including those with zero upcoming shows. In that mode we also
// surface `last_show_date` (most recent past approved show) so cards can
// render a "no upcoming shows · last show <Mon Year>" affordance instead of
// looking broken.
//
// Sort order: upcoming_show_count DESC, name ASC. Active artists surface
// first, then alphabetical. Preserved across both gated and evergreen modes.
func (s *ArtistService) GetArtistsWithShowCounts(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	skipActiveFilter, _ := filters["skip_active_filter"].(bool)

	now := time.Now().UTC()

	// Subquery: count upcoming approved shows per artist
	upcomingSubquery := s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(*) as show_count").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id")

	var query *gorm.DB
	if skipActiveFilter {
		// Evergreen mode: include artists without upcoming shows.
		// LEFT JOIN on upcoming counts (zero when none), and compute
		// last past-show date so cards can show "last show <Mon Year>".
		pastSubquery := s.db.Table("show_artists").
			Select("show_artists.artist_id, MAX(shows.event_date) as last_show_date").
			Joins("JOIN shows ON show_artists.show_id = shows.id").
			Where("shows.event_date < ? AND shows.status = ?", now, catalogm.ShowStatusApproved).
			Group("show_artists.artist_id")

		query = s.db.Table("artists").
			Select("artists.*, COALESCE(sc.show_count, 0) as upcoming_show_count, ps.last_show_date as last_show_date").
			Joins("LEFT JOIN (?) as sc ON artists.id = sc.artist_id", upcomingSubquery).
			Joins("LEFT JOIN (?) as ps ON artists.id = ps.artist_id", pastSubquery)
	} else {
		// Gated mode (default for unfiltered /artists): INNER JOIN so only
		// artists with upcoming shows are returned. last_show_date is NULL
		// in this path — the card renders an upcoming count anyway.
		query = s.db.Table("artists").
			Select("artists.*, COALESCE(sc.show_count, 0) as upcoming_show_count, NULL as last_show_date").
			Joins("JOIN (?) as sc ON artists.id = sc.artist_id", upcomingSubquery)
	}

	// Apply filters
	if cities, ok := filters["cities"].([]map[string]string); ok && len(cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range cities {
			if cs["city"] != "" && cs["state"] != "" {
				conditions = append(conditions, "(artists.city = ? AND artists.state = ?)")
				args = append(args, cs["city"], cs["state"])
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if state, ok := filters["state"].(string); ok && state != "" {
			query = query.Where("artists.state = ?", state)
		}
		if city, ok := filters["city"].(string); ok && city != "" {
			query = query.Where("artists.city = ?", city)
		}
	}
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		query = ApplyTagFilter(query, s.db, catalogm.TagEntityArtist, "artists.id", tf)
	}

	var artistsWithCount []ArtistWithCount
	if err := query.Order("upcoming_show_count DESC, artists.name ASC").Find(&artistsWithCount).Error; err != nil {
		return nil, fmt.Errorf("failed to get artists with show counts: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ArtistWithShowCountResponse, len(artistsWithCount))
	for i, ac := range artistsWithCount {
		resp := &contracts.ArtistWithShowCountResponse{
			ArtistDetailResponse: *s.buildArtistResponse(&ac.Artist),
			UpcomingShowCount:    int(ac.UpcomingShowCount),
		}
		if ac.LastShowDate != nil {
			ts := ac.LastShowDate.UTC()
			resp.LastShowDate = &ts
		}
		responses[i] = resp
	}

	return responses, nil
}

// GetArtistCities returns distinct cities for artists that have upcoming approved shows.
// Only artists with both city and state set are included.
// Results are sorted by artist count (descending) to show most active cities first.
func (s *ArtistService) GetArtistCities() ([]*contracts.ArtistCityResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type CityResult struct {
		City        string
		State       string
		ArtistCount int64
	}

	// Subquery: artist IDs that have upcoming approved shows
	artistsWithShows := s.db.Table("show_artists").
		Select("DISTINCT show_artists.artist_id").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, catalogm.ShowStatusApproved)

	var results []CityResult
	err := s.db.Table("artists").
		Select("city, state, COUNT(*) as artist_count").
		Where("city IS NOT NULL AND city != '' AND state IS NOT NULL AND state != ''").
		Where("id IN (?)", artistsWithShows).
		Group("city, state").
		Order("artist_count DESC, city ASC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get artist cities: %w", err)
	}

	responses := make([]*contracts.ArtistCityResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.ArtistCityResponse{
			City:        r.City,
			State:       r.State,
			ArtistCount: int(r.ArtistCount),
		}
	}

	return responses, nil
}

// buildArtistStats populates the at-a-glance stats block on the artist
// detail page (PSY-639) in a single round-trip — five scalar subqueries,
// each hitting an indexed artist_id FK. Stats are sidebar polish: on query
// failure we log and return zeroed counts rather than failing the whole
// artist response.
//
// Subquery notes:
//   - releases: DISTINCT release_id — the (artist_id, release_id, role) PK
//     lets one artist have multiple rows per release (composer + performer).
//   - shows_tracked: plain COUNT — (show_id, artist_id) PK is one row per
//     show; includes past + future.
//   - similar_artists: artist_relationships has a CHECK source_artist_id <
//     target_artist_id, so this artist is the source in some rows and the
//     target in others; DISTINCT on the *other* side collapses multiple
//     relationship_type rows between the same pair.
//   - festival_appearances: plain COUNT — festival_artists has a UNIQUE
//     (festival_id, artist_id) constraint, so COUNT(*) == COUNT(DISTINCT).
func (s *ArtistService) buildArtistStats(artistID uint) *contracts.ArtistStatsResponse {
	stats := &contracts.ArtistStatsResponse{}
	if s.db == nil {
		return stats
	}

	err := s.db.Raw(`
		SELECT
			(SELECT COUNT(DISTINCT release_id) FROM artist_releases WHERE artist_id = @id) AS releases,
			(SELECT COUNT(*) FROM artist_labels WHERE artist_id = @id) AS labels,
			(SELECT COUNT(*) FROM show_artists WHERE artist_id = @id) AS shows_tracked,
			(SELECT COUNT(DISTINCT CASE
				WHEN source_artist_id = @id THEN target_artist_id
				ELSE source_artist_id
			END) FROM artist_relationships
			WHERE source_artist_id = @id OR target_artist_id = @id) AS similar_artists,
			(SELECT COUNT(*) FROM festival_artists WHERE artist_id = @id) AS festival_appearances
	`, map[string]interface{}{"id": artistID}).Scan(stats).Error
	if err != nil {
		log.Printf("WARN buildArtistStats: failed to count stats for artist_id=%d: %v", artistID, err)
	}

	return stats
}

// buildArtistResponse converts an Artist model to contracts.ArtistDetailResponse
func (s *ArtistService) buildArtistResponse(artist *catalogm.Artist) *contracts.ArtistDetailResponse {
	slug := ""
	if artist.Slug != nil {
		slug = *artist.Slug
	}
	return &contracts.ArtistDetailResponse{
		ID:               artist.ID,
		Slug:             slug,
		Name:             artist.Name,
		State:            artist.State,
		City:             artist.City,
		Country:          artist.Country,
		BandcampEmbedURL: artist.BandcampEmbedURL,
		Description:      artist.Description,
		ImageURL:         artist.ImageURL,
		ImageSource:      artist.ImageSource,
		ImageSourceURL:   artist.ImageSourceURL,
		ImageLicense:     artist.ImageLicense,
		ImageAuthor:      artist.ImageAuthor,
		Social: contracts.SocialResponse{
			Instagram:  artist.Social.Instagram,
			Facebook:   artist.Social.Facebook,
			Twitter:    artist.Social.Twitter,
			YouTube:    artist.Social.YouTube,
			Spotify:    artist.Social.Spotify,
			SoundCloud: artist.Social.SoundCloud,
			Bandcamp:   artist.Social.Bandcamp,
			Website:    artist.Social.Website,
		},
		CreatedAt: artist.CreatedAt,
		UpdatedAt: artist.UpdatedAt,
	}
}

// contracts.ArtistLabelResponse represents a label the artist is on

// GetLabelsForArtist retrieves all labels associated with an artist
func (s *ArtistService) GetLabelsForArtist(artistID uint) ([]*contracts.ArtistLabelResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist catalogm.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	// Get label IDs from junction table
	var artistLabels []catalogm.ArtistLabel
	s.db.Where("artist_id = ?", artistID).Find(&artistLabels)

	if len(artistLabels) == 0 {
		return []*contracts.ArtistLabelResponse{}, nil
	}

	labelIDs := make([]uint, len(artistLabels))
	for i, al := range artistLabels {
		labelIDs[i] = al.LabelID
	}

	var labels []catalogm.Label
	s.db.Where("id IN ?", labelIDs).Order("name ASC").Find(&labels)

	responses := make([]*contracts.ArtistLabelResponse, len(labels))
	for i, label := range labels {
		slug := ""
		if label.Slug != nil {
			slug = *label.Slug
		}
		responses[i] = &contracts.ArtistLabelResponse{
			ID:    label.ID,
			Name:  label.Name,
			Slug:  slug,
			City:  label.City,
			State: label.State,
		}
	}

	return responses, nil
}

// GetShowsForArtist retrieves shows for a specific artist with time filtering.
// timeFilter can be: "upcoming" (event_date >= today), "past" (event_date < today), or "all"
// Only returns approved shows.
func (s *ArtistService) GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*contracts.ArtistShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist catalogm.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, 0, fmt.Errorf("failed to get artist: %w", err)
	}

	// Load timezone, default to UTC
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	// Get start of today in the user's timezone, then convert to UTC for query
	now := time.Now().In(loc)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	startOfTodayUTC := startOfToday.UTC()

	// Apply time filter and determine ordering
	var dateCondition string
	var orderDirection string
	switch timeFilter {
	case "past":
		dateCondition = "shows.event_date < ?"
		orderDirection = "shows.event_date DESC" // Most recent past shows first
	case "all":
		dateCondition = "" // No date filter
		orderDirection = "shows.event_date ASC"
	default: // "upcoming"
		dateCondition = "shows.event_date >= ?"
		orderDirection = "shows.event_date ASC" // Soonest upcoming shows first
	}

	// Count total shows matching the filter
	var total int64
	countQuery := s.db.Table("show_artists").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, catalogm.ShowStatusApproved)
	if dateCondition != "" {
		countQuery = countQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get show IDs with limit
	var showIDs []uint
	showQuery := s.db.Table("show_artists").
		Select("show_artists.show_id").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, catalogm.ShowStatusApproved)
	if dateCondition != "" {
		showQuery = showQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := showQuery.Order(orderDirection).Limit(limit).Pluck("show_artists.show_id", &showIDs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get show IDs: %w", err)
	}

	// Fetch full show data with artists
	var shows []catalogm.Show
	if len(showIDs) > 0 {
		if err := s.db.Preload("Artists").Where("id IN ?", showIDs).Order(orderDirection).Find(&shows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get shows: %w", err)
		}
	}

	// Batch-load all ShowVenue records
	showIDsList := make([]uint, len(shows))
	for i, show := range shows {
		showIDsList[i] = show.ID
	}

	var allShowVenues []catalogm.ShowVenue
	s.db.Where("show_id IN ?", showIDsList).Find(&allShowVenues)

	// Batch-fetch all venue models
	venueIDs := make([]uint, 0, len(allShowVenues))
	showVenueMap := make(map[uint]uint) // showID -> venueID
	for _, sv := range allShowVenues {
		showVenueMap[sv.ShowID] = sv.VenueID
		venueIDs = append(venueIDs, sv.VenueID)
	}
	venueMap := make(map[uint]*catalogm.Venue)
	if len(venueIDs) > 0 {
		var venues []catalogm.Venue
		s.db.Where("id IN ?", venueIDs).Find(&venues)
		for i := range venues {
			venueMap[venues[i].ID] = &venues[i]
		}
	}

	// Batch-load all ShowArtist records
	var allShowArtists []catalogm.ShowArtist
	s.db.Where("show_id IN ?", showIDsList).Order("position ASC").Find(&allShowArtists)
	showArtistsMap := make(map[uint][]catalogm.ShowArtist)
	var allArtistIDs []uint
	for _, sa := range allShowArtists {
		showArtistsMap[sa.ShowID] = append(showArtistsMap[sa.ShowID], sa)
		allArtistIDs = append(allArtistIDs, sa.ArtistID)
	}
	artistMap := make(map[uint]*catalogm.Artist)
	if len(allArtistIDs) > 0 {
		var artists []catalogm.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&artists)
		for i := range artists {
			artistMap[artists[i].ID] = &artists[i]
		}
	}

	// Build responses
	responses := make([]*contracts.ArtistShowResponse, len(shows))
	for i, show := range shows {
		// Look up venue for this show
		var venue *contracts.ArtistShowVenueResponse
		if venueID, ok := showVenueMap[show.ID]; ok {
			if venueModel, ok := venueMap[venueID]; ok {
				var venueSlug string
				if venueModel.Slug != nil {
					venueSlug = *venueModel.Slug
				}
				venue = &contracts.ArtistShowVenueResponse{
					ID:       venueModel.ID,
					Slug:     venueSlug,
					Name:     venueModel.Name,
					City:     venueModel.City,
					State:    venueModel.State,
					Timezone: venueModel.Timezone,
				}
			}
		}

		// Look up ordered artists for this show
		artists := make([]contracts.ArtistShowArtist, 0)
		for _, sa := range showArtistsMap[show.ID] {
			if artistModel, ok := artistMap[sa.ArtistID]; ok {
				var artistSlug string
				if artistModel.Slug != nil {
					artistSlug = *artistModel.Slug
				}
				artists = append(artists, contracts.ArtistShowArtist{
					ID:   artistModel.ID,
					Slug: artistSlug,
					Name: artistModel.Name,
				})
			}
		}

		responses[i] = &contracts.ArtistShowResponse{
			ID:             show.ID,
			Title:          show.Title,
			EventDate:      show.EventDate,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Venue:          venue,
			Artists:        artists,
		}
	}

	return responses, total, nil
}

// ──────────────────────────────────────────────
// Alias CRUD
// ──────────────────────────────────────────────

// AddArtistAlias adds an alias for an artist. Validates uniqueness against
// other aliases and artist names (case-insensitive).
func (s *ArtistService) AddArtistAlias(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, fmt.Errorf("alias cannot be empty")
	}

	// Verify artist exists
	var artist catalogm.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	// Check for duplicate alias (case-insensitive)
	var existing catalogm.ArtistAlias
	if err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&existing).Error; err == nil {
		return nil, apperrors.ErrArtistAliasExists(fmt.Sprintf("alias '%s' already exists", alias))
	}

	// Check if alias matches an existing artist name
	var existingArtist catalogm.Artist
	if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&existingArtist).Error; err == nil {
		return nil, apperrors.ErrArtistAliasExists(fmt.Sprintf("alias '%s' conflicts with existing artist name", alias))
	}

	artistAlias := &catalogm.ArtistAlias{
		ArtistID: artistID,
		Alias:    alias,
	}

	if err := s.db.Create(artistAlias).Error; err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	return &contracts.ArtistAliasResponse{
		ID:        artistAlias.ID,
		ArtistID:  artistAlias.ArtistID,
		Alias:     artistAlias.Alias,
		CreatedAt: artistAlias.CreatedAt.Format(time.RFC3339),
	}, nil
}

// RemoveArtistAlias deletes an alias by ID.
func (s *ArtistService) RemoveArtistAlias(aliasID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&catalogm.ArtistAlias{}, aliasID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete alias: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrArtistAliasNotFound()
	}

	return nil
}

// GetArtistAliases returns all aliases for an artist.
func (s *ArtistService) GetArtistAliases(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist catalogm.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	var aliases []catalogm.ArtistAlias
	if err := s.db.Where("artist_id = ?", artistID).Order("alias ASC").Find(&aliases).Error; err != nil {
		return nil, fmt.Errorf("failed to list aliases: %w", err)
	}

	responses := make([]*contracts.ArtistAliasResponse, len(aliases))
	for i, a := range aliases {
		responses[i] = &contracts.ArtistAliasResponse{
			ID:        a.ID,
			ArtistID:  a.ArtistID,
			Alias:     a.Alias,
			CreatedAt: a.CreatedAt.Format(time.RFC3339),
		}
	}

	return responses, nil
}

// ──────────────────────────────────────────────
// Artist Merge
// ──────────────────────────────────────────────

// MergeArtists merges the "mergeFrom" artist into the "canonical" artist.
// All relationships (shows, releases, labels, festivals, etc.) are transferred
// to the canonical artist. Conflicts (duplicate rows) are deleted before transfer.
// The merged artist's name is added as an alias, then the merged artist is deleted.
func (s *ArtistService) MergeArtists(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if canonicalID == mergeFromID {
		return nil, apperrors.ErrArtistMergeSelf()
	}

	// Verify both artists exist
	var canonical catalogm.Artist
	if err := s.db.First(&canonical, canonicalID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(canonicalID)
		}
		return nil, fmt.Errorf("failed to get canonical artist: %w", err)
	}

	var mergeFrom catalogm.Artist
	if err := s.db.First(&mergeFrom, mergeFromID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrArtistNotFound(mergeFromID)
		}
		return nil, fmt.Errorf("failed to get merge-from artist: %w", err)
	}

	result := &contracts.MergeArtistResult{
		CanonicalArtistID: canonicalID,
		MergedArtistID:    mergeFromID,
		MergedArtistName:  mergeFrom.Name,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. show_artists: delete conflicts, then update remaining
		tx.Where("artist_id = ? AND show_id IN (?)", mergeFromID,
			tx.Table("show_artists").Select("show_id").Where("artist_id = ?", canonicalID),
		).Delete(&catalogm.ShowArtist{})
		r := tx.Model(&catalogm.ShowArtist{}).Where("artist_id = ?", mergeFromID).Update("artist_id", canonicalID)
		result.ShowsMoved = r.RowsAffected

		// 2. artist_releases: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_releases WHERE artist_id = ? AND (release_id, role) IN (SELECT release_id, role FROM artist_releases WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE artist_releases SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.ReleasesMoved = r.RowsAffected

		// 3. artist_labels: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_labels WHERE artist_id = ? AND label_id IN (SELECT label_id FROM artist_labels WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE artist_labels SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.LabelsMoved = r.RowsAffected

		// 4. festival_artists: delete conflicts, then update remaining
		tx.Exec("DELETE FROM festival_artists WHERE artist_id = ? AND festival_id IN (SELECT festival_id FROM festival_artists WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE festival_artists SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.FestivalsMoved = r.RowsAffected

		// 5. artist_relationships: re-canonicalize with source < target, delete self-referential and conflicts
		// First delete any that would become self-referential
		tx.Exec("DELETE FROM artist_relationship_votes WHERE (source_artist_id = ? OR target_artist_id = ?) AND (source_artist_id = ? OR target_artist_id = ?)",
			mergeFromID, mergeFromID, canonicalID, canonicalID)
		tx.Exec("DELETE FROM artist_relationships WHERE (source_artist_id = ? AND target_artist_id = ?) OR (source_artist_id = ? AND target_artist_id = ?)",
			mergeFromID, canonicalID, canonicalID, mergeFromID)
		// Delete votes for relationships that will be deleted as self-referential after merge
		tx.Exec("DELETE FROM artist_relationship_votes WHERE source_artist_id = ? OR target_artist_id = ?", mergeFromID, mergeFromID)
		// Delete remaining relationships involving mergeFrom (after conflicts removed)
		r = tx.Exec("DELETE FROM artist_relationships WHERE source_artist_id = ? OR target_artist_id = ?", mergeFromID, mergeFromID)
		result.RelationshipsMoved = r.RowsAffected

		// 6. entity_tags: delete conflicts, then update remaining
		tx.Exec("DELETE FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ? AND tag_id IN (SELECT tag_id FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ?)", mergeFromID, canonicalID)
		tx.Exec("UPDATE entity_tags SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 7. user_bookmarks: delete conflicts, then update remaining
		tx.Exec("DELETE FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ? AND (user_id, action) IN (SELECT user_id, action FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE user_bookmarks SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)
		result.BookmarksMoved = r.RowsAffected

		// 8. artist_reports: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_reports WHERE artist_id = ? AND reported_by IN (SELECT reported_by FROM artist_reports WHERE artist_id = ?)", mergeFromID, canonicalID)
		tx.Exec("UPDATE artist_reports SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)

		// 9. revisions: just update entity_id (no conflict key)
		tx.Exec("UPDATE revisions SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 10. tag_votes for entity tags: delete conflicts, then update remaining
		tx.Exec(`DELETE FROM tag_votes WHERE entity_type = 'artist' AND entity_id = ?
			AND (tag_id, user_id) IN (SELECT tag_id, user_id FROM tag_votes WHERE entity_type = 'artist' AND entity_id = ?)`, mergeFromID, canonicalID)
		tx.Exec("UPDATE tag_votes SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 11. collection_items: delete conflicts (same collection + same entity), then update remaining
		tx.Exec("DELETE FROM collection_items WHERE entity_type = 'artist' AND entity_id = ? AND collection_id IN (SELECT collection_id FROM collection_items WHERE entity_type = 'artist' AND entity_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE collection_items SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)
		result.CollectionItemsMoved = r.RowsAffected

		// 12. notification_filters: replace mergeFromID in artist_ids arrays
		r = tx.Exec(`UPDATE notification_filters
			SET artist_ids = array_replace(artist_ids, ?, ?),
			    updated_at = NOW()
			WHERE artist_ids @> ARRAY[?]::bigint[]
			AND NOT artist_ids @> ARRAY[?]::bigint[]`,
			mergeFromID, canonicalID, mergeFromID, canonicalID)
		result.FiltersUpdated = r.RowsAffected
		// Remove duplicates where filter already had canonical (just remove the old ID)
		tx.Exec(`UPDATE notification_filters
			SET artist_ids = array_remove(artist_ids, ?),
			    updated_at = NOW()
			WHERE artist_ids @> ARRAY[?]::bigint[]`,
			mergeFromID, mergeFromID)

		// 13. notification_log: update entity references (informational, no unique constraint on entity_id)
		tx.Exec("UPDATE notification_log SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 14. requests: update requested_entity_id references
		tx.Exec("UPDATE requests SET requested_entity_id = ? WHERE entity_type = 'artist' AND requested_entity_id = ?", canonicalID, mergeFromID)

		// 15. Transfer aliases from merged artist to canonical
		tx.Exec("UPDATE artist_aliases SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)

		// 16. Create alias from merged artist's name (if not conflicting)
		var aliasCount int64
		tx.Model(&catalogm.ArtistAlias{}).Where("LOWER(alias) = LOWER(?)", mergeFrom.Name).Count(&aliasCount)
		var nameCount int64
		tx.Model(&catalogm.Artist{}).Where("LOWER(name) = LOWER(?)", mergeFrom.Name).Where("id != ?", mergeFromID).Count(&nameCount)
		if aliasCount == 0 && nameCount == 0 {
			newAlias := catalogm.ArtistAlias{
				ArtistID: canonicalID,
				Alias:    mergeFrom.Name,
			}
			if err := tx.Create(&newAlias).Error; err != nil {
				return fmt.Errorf("failed to create alias from merged artist name: %w", err)
			}
			result.AliasCreated = true
		}

		// 17. Delete the merged artist
		if err := tx.Delete(&mergeFrom).Error; err != nil {
			return fmt.Errorf("failed to delete merged artist: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("merge failed: %w", err)
	}

	return result, nil
}

// ──────────────────────────────────────────────
// Bandcamp embed derivation (PSY-1188)
// ──────────────────────────────────────────────

// manualEmbedSourceIfSet returns a pointer to the "manual" provenance value
// when embed is a non-nil, non-empty embed URL (a human/admin/AI write actually
// set it), and nil otherwise. Centralizes the "stamp manual only when an embed
// is supplied" rule so CreateArtist doesn't claim a source for an absent embed.
func manualEmbedSourceIfSet(embed *string) *string {
	if embed == nil || strings.TrimSpace(*embed) == "" {
		return nil
	}
	src := catalogm.BandcampEmbedSourceManual
	return &src
}

// ──────────────────────────────────────────────
// Profile→album embed resolution (PSY-1190)
//
// Discovered Bandcamp links and many existing social.bandcamp values are PROFILE
// roots (band.bandcamp.com), not the /album|/track URLs the artist-page player
// needs in bandcamp_embed_url. When such a profile is set/accepted for an artist
// whose embed is still NULL, fetch the root and resolve its featured/latest album
// URL, stamping profile_resolved.
//
// Mirrors the PSY-1188/1189 invariants exactly:
//   - FILL-WHEN-EMPTY: only writes when bandcamp_embed_url IS NULL (the UPDATE
//     re-asserts the guard so a concurrent manual write is never clobbered).
//   - PROVENANCE: stamps profile_resolved (an auto-derived source). A manual value
//     is immutable — the IS NULL gate skips any already-set row.
//
// The network fetch runs OUTSIDE any DB transaction (it is a slow external call;
// coupling it to a GORM write would tie a DB write to an 8s fetch). A failed
// resolve is a no-op, never an error on the triggering write.
// ──────────────────────────────────────────────

// isBandcampProfileRoot reports whether rawURL is a *.bandcamp.com profile root
// (an artist subdomain with no /album or /track path) — the only shape the
// resolver acts on. An album/track URL is already embeddable (handled by the
// manual write path) and the bare apex (bandcamp.com) is not an artist profile,
// so both are excluded here.
func isBandcampProfileRoot(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	// Must be an artist subdomain (the shared host-anchor also excludes the bare
	// apex, which is the storefront, not a profile).
	if !utils.IsBandcampArtistHost(u.Hostname()) {
		return false
	}
	// Already an embeddable release URL → not a profile root to resolve.
	if strings.HasPrefix(u.Path, "/album/") || strings.HasPrefix(u.Path, "/track/") {
		return false
	}
	return true
}

// resolveProfileEmbedForArtist resolves a Bandcamp PROFILE root → a featured
// /album|/track URL and, if one is found, writes it stamped profile_resolved —
// but only when the artist's embed is still NULL. It is invoked AFTER a write that
// set the artist's social.bandcamp to a profile root; the resolver does a network
// fetch, so this runs outside the triggering write's transaction (s.db, not tx).
//
// Skips silently (no error) when: no resolver is configured, the value isn't a
// profile root, the row already has an embed (manual or auto-derived), or the
// resolve yields nothing/an unembeddable URL. The fetch is host-anchored and
// redirect-re-anchored to *.bandcamp.com inside the resolver (SSRF), and the
// resolved URL is re-validated by utils.IsValidBandcampEmbedURL before it is
// stored, so a malformed extraction never lands in the column.
func (s *ArtistService) resolveProfileEmbedForArtist(ctx context.Context, artistID uint, profileURL string) {
	if s.bandcampResolver == nil || !isBandcampProfileRoot(profileURL) {
		return
	}

	// Cheap pre-check: skip the network fetch entirely when the embed is already
	// set. The UPDATE below re-asserts IS NULL, so this is an optimization, not the
	// correctness guard.
	var current catalogm.Artist
	if err := s.db.Select("id", "bandcamp_embed_url").First(&current, artistID).Error; err != nil {
		return // artist gone or unreadable — nothing to fill.
	}
	if current.BandcampEmbedURL != nil {
		return // manual or previously-derived value present — never overwrite.
	}

	embed, ok := s.bandcampResolver.ResolveProfileEmbed(ctx, profileURL)
	if !ok {
		return // unfetchable profile or no featured release — leave the column NULL.
	}
	// Defense in depth: the resolved URL must pass the SAME strict gate every
	// other write path enforces before it reaches the iframe-rendered column.
	if !utils.IsValidBandcampEmbedURL(embed) {
		log.Printf("WARN resolveProfileEmbedForArtist: resolver returned non-embeddable URL %q for artist %d", embed, artistID)
		return
	}

	if err := s.db.Model(&catalogm.Artist{}).
		Where("id = ? AND bandcamp_embed_url IS NULL", artistID).
		Updates(map[string]interface{}{
			"bandcamp_embed_url":    embed,
			"bandcamp_embed_source": catalogm.BandcampEmbedSourceProfileResolved,
		}).Error; err != nil {
		log.Printf("WARN resolveProfileEmbedForArtist: failed to fill profile-resolved embed for artist %d: %v", artistID, err)
	}
}

// embedCandidate is one embeddable Bandcamp link found on a release, carrying
// the fields the selection rule sorts on.
type embedCandidate struct {
	url     string
	year    *int // release year; nil sorts last (oldest/unknown)
	isAlbum bool // /album/ (richer embed) vs /track/
	relID   uint
	linkID  uint
}

// selectBandcampEmbedFromReleases picks a representative Bandcamp album/track
// embed URL from an artist's releases, or returns nil when none of the releases
// carry an embeddable Bandcamp link.
//
// Selection rule (deterministic — the cmd must pick the same URL on re-run):
//   - Consider only release external links whose URL passes the strict
//     utils.IsValidBandcampEmbedURL gate (host-anchored *.bandcamp.com +
//     /album|/track path). The link's `platform` label is intentionally NOT
//     trusted — the URL itself is the source of truth, so a mislabeled or
//     unlabeled link is still matched and a non-Bandcamp link labeled
//     "bandcamp" is still rejected.
//   - Prefer the most RECENT release by ReleaseYear. A nil year sorts LAST
//     (treated as oldest/unknown) so dated releases win over undated ones.
//   - Tie-break, in order: /album over /track (an album page is the richer
//     embed), then the lowest release ID (stable, insertion-order-ish).
//   - Within a single release, prefer an /album link over a /track link, then
//     the lowest external-link ID, for the same determinism.
//
// releases MUST have ExternalLinks preloaded. The returned pointer is to a
// freshly allocated string (safe to store directly).
func selectBandcampEmbedFromReleases(releases []catalogm.Release) *string {
	var best *embedCandidate
	for ri := range releases {
		rel := &releases[ri]
		// Pick the best embeddable link WITHIN this release first.
		var relBest *embedCandidate
		for li := range rel.ExternalLinks {
			link := &rel.ExternalLinks[li]
			if !utils.IsValidBandcampEmbedURL(link.URL) {
				continue
			}
			c := embedCandidate{
				url:  strings.TrimSpace(link.URL),
				year: rel.ReleaseYear,
				// Anchor on the parsed PATH, not a substring of the whole URL:
				// a /track/ link with "/album/" in its query string must not be
				// mis-classified as an album (the URL already passed the strict
				// validator, so the path is /album/… or /track/…).
				isAlbum: utils.IsBandcampAlbumURL(link.URL),
				relID:   rel.ID,
				linkID:  link.ID,
			}
			if relBest == nil || betterLinkWithinRelease(&c, relBest) {
				cc := c
				relBest = &cc
			}
		}
		if relBest == nil {
			continue
		}
		if best == nil || betterCandidate(relBest, best) {
			best = relBest
		}
	}

	if best == nil {
		return nil
	}
	out := best.url
	return &out
}

// betterLinkWithinRelease reports whether a is the better embed link than b for
// the SAME release: album over track, then lowest external-link ID.
func betterLinkWithinRelease(a, b *embedCandidate) bool {
	if a.isAlbum != b.isAlbum {
		return a.isAlbum // album wins
	}
	return a.linkID < b.linkID
}

// betterCandidate reports whether a should beat the current best b ACROSS
// releases: most recent year first (nil year sorts last), then album over
// track, then lowest release ID.
func betterCandidate(a, b *embedCandidate) bool {
	// Year: a non-nil higher year wins; nil is treated as oldest.
	switch {
	case a.year != nil && b.year != nil:
		if *a.year != *b.year {
			return *a.year > *b.year
		}
	case a.year != nil && b.year == nil:
		return true // a is dated, b is not → a wins
	case a.year == nil && b.year != nil:
		return false // b is dated → b wins
	}
	// Equal (or both-nil) year → album over track.
	if a.isAlbum != b.isAlbum {
		return a.isAlbum
	}
	// Final tie-break: lowest release ID for determinism.
	return a.relID < b.relID
}

// DeriveBandcampEmbedForArtist loads the artist's releases (with external links)
// and returns the representative Bandcamp embed URL per the selection rule, or
// nil when none of the releases carry an embeddable Bandcamp link. It does NOT
// write anything — the backfill decides whether to persist it.
func (s *ArtistService) DeriveBandcampEmbedForArtist(artistID uint) (*string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return deriveBandcampEmbedForArtist(s.db, artistID)
}

// deriveBandcampEmbedForArtist is the transaction-aware core of
// DeriveBandcampEmbedForArtist: it runs the same release-load + selection rule
// against an explicit *gorm.DB so the PSY-1189 keep-fresh hooks can derive an
// embed inside the SAME transaction as the triggering release write (a failed
// embed update then rolls back together with the release change). The exported
// method delegates here with s.db; the hooks pass tx.
func deriveBandcampEmbedForArtist(db *gorm.DB, artistID uint) (*string, error) {
	// Release IDs the artist is credited on (any role).
	var releaseIDs []uint
	if err := db.Table("artist_releases").
		Where("artist_id = ?", artistID).
		Distinct().
		Pluck("release_id", &releaseIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to get artist release ids: %w", err)
	}
	if len(releaseIDs) == 0 {
		return nil, nil
	}

	var releases []catalogm.Release
	if err := db.Preload("ExternalLinks").
		Where("id IN ?", releaseIDs).
		Find(&releases).Error; err != nil {
		return nil, fmt.Errorf("failed to load releases for embed derivation: %w", err)
	}

	return selectBandcampEmbedFromReleases(releases), nil
}

// ──────────────────────────────────────────────
// Keep-fresh hooks (PSY-1189)
//
// PSY-1188's one-time backfill filled artists.bandcamp_embed_url for the catalog
// as it stood then, but a release ingested LATER (CLI batch, entity-request
// fulfillment, admin) doesn't populate the artist embed, and a deleted/unlinked
// featured release leaves a stale auto-derived value. These two hooks keep the
// auto-derived embed fresh on every release write path, while NEVER touching a
// human-curated ("manual") value.
//
// They mirror the backfill's two load-bearing invariants:
//   - FILL-WHEN-EMPTY: only populate bandcamp_embed_url when it IS NULL; never
//     overwrite a non-null value.
//   - PROVENANCE GATING: only act on rows whose bandcamp_embed_source is
//     release_derived (or NULL when filling an empty row). A "manual" value is
//     immutable here — never overwritten, recomputed, or nulled.
//
// Both run against an explicit *gorm.DB (the caller's tx) so the embed mutation
// participates in the same transaction as the triggering release write.
// ──────────────────────────────────────────────

// fillReleaseDerivedEmbedsForRelease populates bandcamp_embed_url for every
// artist credited on releaseID whose embed is currently NULL, deriving the value
// from that artist's releases via the shared selection rule and stamping
// release_derived. There is no "primary artist" column on the schema — a release
// is credited to one-or-more artists via artist_releases — so the fill applies to
// every credited artist whose embed is empty (each artist's own catalogue drives
// its own derivation). Already-set embeds (manual OR previously release-derived)
// are skipped by the IS NULL gate, so this never overwrites a value.
//
// db MUST be the transaction handle of the triggering release write so a failure
// here rolls the whole write back. A no-op (no Bandcamp link, or all credited
// artists already have an embed) is not an error.
func fillReleaseDerivedEmbedsForRelease(db *gorm.DB, releaseID uint) error {
	// Artists credited on this release whose embed is still empty — the only
	// fill candidates. Distinct because an artist can hold multiple roles on one
	// release (composer + performer).
	var artistIDs []uint
	if err := db.Table("artist_releases").
		Joins("JOIN artists ON artists.id = artist_releases.artist_id").
		Where("artist_releases.release_id = ? AND artists.bandcamp_embed_url IS NULL", releaseID).
		Distinct().
		Pluck("artist_releases.artist_id", &artistIDs).Error; err != nil {
		return fmt.Errorf("failed to list fill-candidate artists for release %d: %w", releaseID, err)
	}

	for _, artistID := range artistIDs {
		if err := fillReleaseDerivedEmbedForArtist(db, artistID); err != nil {
			return err
		}
	}
	return nil
}

// fillReleaseDerivedEmbedForArtist derives a Bandcamp embed for artistID and, if
// one is found, writes it stamped release_derived — but only when the embed is
// still NULL (the WHERE re-asserts the IS NULL guard so a concurrent manual write
// can't be clobbered). A nil derivation (no embeddable Bandcamp link) is a no-op.
func fillReleaseDerivedEmbedForArtist(db *gorm.DB, artistID uint) error {
	embed, err := deriveBandcampEmbedForArtist(db, artistID)
	if err != nil {
		return fmt.Errorf("failed to derive embed for artist %d: %w", artistID, err)
	}
	if embed == nil {
		return nil // no embeddable Bandcamp link — leave the column NULL.
	}

	if err := db.Model(&catalogm.Artist{}).
		Where("id = ? AND bandcamp_embed_url IS NULL", artistID).
		Updates(map[string]interface{}{
			"bandcamp_embed_url":    *embed,
			"bandcamp_embed_source": catalogm.BandcampEmbedSourceReleaseDerived,
		}).Error; err != nil {
		return fmt.Errorf("failed to fill release-derived embed for artist %d: %w", artistID, err)
	}
	return nil
}

// releaseDerivedArtistIDsForRelease returns the IDs of artists credited on
// releaseID whose CURRENT embed is release_derived — the only artists a removal
// of this release (or one of its links) can affect. A manual or empty embed is
// excluded so the recompute caller never even loads it. Callers MUST invoke this
// BEFORE the delete, since a release delete cascades the artist_releases rows.
func releaseDerivedArtistIDsForRelease(db *gorm.DB, releaseID uint) ([]uint, error) {
	var artistIDs []uint
	if err := db.Table("artist_releases").
		Joins("JOIN artists ON artists.id = artist_releases.artist_id").
		Where("artist_releases.release_id = ? AND artists.bandcamp_embed_source = ?",
			releaseID, catalogm.BandcampEmbedSourceReleaseDerived).
		Distinct().
		Pluck("artist_releases.artist_id", &artistIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list release-derived artists for release %d: %w", releaseID, err)
	}
	return artistIDs, nil
}

// recomputeReleaseDerivedEmbeds re-derives the auto-derived embed for each of
// artistIDs after a release (or one of its Bandcamp links) is removed, so a
// deleted/unlinked release no longer leaves a stale embed pointing at a release
// the artist no longer has. ONLY rows whose bandcamp_embed_source is
// release_derived are touched — a manual embed is immutable here (the WHERE
// re-asserts the source so a manual value is never recomputed or nulled, even
// under concurrency).
//
// For each release_derived artist: re-run the selection rule over the artist's
// REMAINING releases and write the new value only if it differs from the stored
// one (no churn when the removed release wasn't the one the embed came from). If
// no embeddable Bandcamp link remains, both the URL and the source are nulled.
//
// db MUST be the transaction handle of the triggering removal so a failure here
// rolls the removal back too.
func recomputeReleaseDerivedEmbeds(db *gorm.DB, artistIDs []uint) error {
	for _, artistID := range artistIDs {
		if err := recomputeReleaseDerivedEmbedForArtist(db, artistID); err != nil {
			return err
		}
	}
	return nil
}

// recomputeReleaseDerivedEmbedForArtist re-derives a single artist's
// release_derived embed (see recomputeReleaseDerivedEmbeds). It loads the
// artist's current embed + source first and returns early — touching nothing —
// unless the source is exactly release_derived, so a manual or legacy (NULL)
// embed is left alone.
func recomputeReleaseDerivedEmbedForArtist(db *gorm.DB, artistID uint) error {
	var current catalogm.Artist
	if err := db.Select("id", "bandcamp_embed_url", "bandcamp_embed_source").
		First(&current, artistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // artist gone (e.g. cascaded) — nothing to recompute.
		}
		return fmt.Errorf("failed to load artist %d for embed recompute: %w", artistID, err)
	}

	// Provenance gate: only auto-derived embeds are eligible. A manual value is
	// immutable here; a NULL source (legacy/unknown, or no embed) is left alone.
	if current.BandcampEmbedSource == nil ||
		*current.BandcampEmbedSource != catalogm.BandcampEmbedSourceReleaseDerived {
		return nil
	}

	derived, err := deriveBandcampEmbedForArtist(db, artistID)
	if err != nil {
		return fmt.Errorf("failed to re-derive embed for artist %d: %w", artistID, err)
	}

	if derived == nil {
		// No embeddable Bandcamp link remains → clear the auto-derived value and
		// its source. Re-assert the release_derived guard so a manual write that
		// landed concurrently is not clobbered.
		if err := db.Model(&catalogm.Artist{}).
			Where("id = ? AND bandcamp_embed_source = ?", artistID, catalogm.BandcampEmbedSourceReleaseDerived).
			Updates(map[string]interface{}{
				"bandcamp_embed_url":    nil,
				"bandcamp_embed_source": nil,
			}).Error; err != nil {
			return fmt.Errorf("failed to clear release-derived embed for artist %d: %w", artistID, err)
		}
		return nil
	}

	// A value remains. Only write when it actually changed (the removed release
	// wasn't necessarily the one the embed came from — avoid needless churn).
	if current.BandcampEmbedURL != nil && *current.BandcampEmbedURL == *derived {
		return nil
	}
	if err := db.Model(&catalogm.Artist{}).
		Where("id = ? AND bandcamp_embed_source = ?", artistID, catalogm.BandcampEmbedSourceReleaseDerived).
		Updates(map[string]interface{}{
			"bandcamp_embed_url":    *derived,
			"bandcamp_embed_source": catalogm.BandcampEmbedSourceReleaseDerived,
		}).Error; err != nil {
		return fmt.Errorf("failed to update release-derived embed for artist %d: %w", artistID, err)
	}
	return nil
}
