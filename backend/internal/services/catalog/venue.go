package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
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

// VenueService handles venue-related business logic
type VenueService struct {
	db       *gorm.DB
	geocoder geo.Geocoder
	// addressGeocoder resolves street addresses to street-level coordinates
	// over the network (Nominatim). Unlike the offline geocoder it can fail or
	// be slow, so every use is log-and-continue — a venue write NEVER blocks or
	// fails on it. nil disables street geocoding (tests construct the service
	// bare; the backfill CLI covers anything skipped).
	addressGeocoder geo.AddressGeocoder
}

// NewVenueService creates a new venue service
func NewVenueService(database *gorm.DB) *VenueService {
	if database == nil {
		database = db.GetDB()
	}
	return &VenueService{
		db:              database,
		geocoder:        geo.Default(),
		addressGeocoder: geo.DefaultNominatim(),
	}
}

// WithAddressGeocoder overrides the network street-address geocoder; nil
// disables inline street geocoding entirely. For tests and tools that must
// never reach the public Nominatim API (the default constructor wires the
// live client).
func (s *VenueService) WithAddressGeocoder(ag geo.AddressGeocoder) *VenueService {
	s.addressGeocoder = ag
	return s
}

// applyGeocoding resolves and sets latitude/longitude/timezone AND the CBSA metro
// on a venue from its city/state/country via the offline geocoder (in-memory, no
// network, never errors). A miss leaves the fields nil so display falls back to
// the legacy state->timezone map — no regression. (PSY-985; metro PSY-1255 step B)
//
// Holds the PSY-1707 write-boundary invariant for the three paths that run
// through it — CreateVenue, UpdateVenue and FindOrCreateVenue: whatever lands in
// venues.timezone is a name Postgres itself recognizes. Readers (the venue-local
// show-list partition, the ICS feed, reminder rendering) resolve it with
// AT TIME ZONE, which does not degrade gracefully — an unknown name raises and
// takes the query down.
//
// It is NOT the only writer of that column, so a new write path does not inherit
// this for free — the others each hold the invariant themselves. The full census
// of writers lives on shared.NormalizedGeocodedTimezoneOrNull; don't re-list it
// here, because two copies of that list is how a writer went missing (PSY-1709).
//
// Since PSY-1747 this is a thin adapter over shared.DeriveVenueLocation rather
// than its own derivation: the admin apply-an-edit paths and the data-sync import
// seams resolve the same four columns through that same function, so a change to
// what a venue derives lands everywhere at once instead of in whichever copies
// the author found. Keep the behaviour here in the adapter and the policy down
// there.
//
// Deliberately NOT a GORM BeforeSave hook on the model, which looks like the
// obvious way to make this automatic: two of the writers update through
// db.Table("venues").Updates(map) with no model bound, so no hook fires for them
// and coverage would be inconsistent-by-construction — strictly worse than
// explicit calls. A hook also cannot see WHICH of city/state/country the caller
// meant to change (the gate every call site needs), has no access to the
// injected geocoder, and cannot return the classification backfillVenuePass
// reports.
//
// db is the handle the venue is about to be WRITTEN through, not necessarily
// s.db: FindOrCreateVenue runs inside a caller-supplied transaction, and
// validating on a second connection would put the guard outside the transaction
// that carries the write it guards (and take a second pooled connection while
// the first is held). Callers with no transaction of their own pass s.db.
func (s *VenueService) applyGeocoding(db *gorm.DB, v *catalogm.Venue) {
	shared.DeriveVenueLocation(db, s.geocoder, shared.VenueLocation(v),
		"venue_name", v.Name, "city", v.City, "state", v.State).
		ApplyTo(v)
}

// streetGeocodeTimeout caps ONE inline street-geocode for a venue, including
// time waiting on the shared 1 req/s Nominatim limiter and retries — and, since
// PSY-1609, covering up to TWO sequential lookups: the address as stored, then
// the abbreviation-expanded form if that missed.
//
// Deliberately NOT raised to accommodate the second lookup: this runs on the
// venue write path, so the budget is what a user waits, not what the geocoder
// would like. geocodeExpandingAbbreviations instead declines the retry when too
// little of the budget remains, which keeps a clean miss memoizable rather than
// letting it decay into an un-memoized timeout.
const streetGeocodeTimeout = 15 * time.Second

// streetGeocodeQuery builds the Nominatim query — and, via Key(), the
// canonical geocoded_address freshness key — for a venue's current location
// fields. Every producer AND consumer of venues.geocoded_address must go
// through this so the freshness comparison is exact.
//
// The Street is ABBREVIATION-EXPANDED here, which means the expansion also flows
// into Key(). That is deliberate and load-bearing: a clean miss is memoized
// against the key, so if the key were built from the raw address, widening the
// abbreviation table later would never re-query a venue already memoized as a
// miss — even though the new expansion would now resolve it. Keying on the
// expanded form makes a table change invalidate exactly the memos it could
// affect, and those venues get retried once.
//
// Consequence to expect on deploy: any venue whose address contains an
// expandable token has a new key, so it reads as un-attempted and is re-geocoded
// once by the sweep. Because streetGeocodeFresh recomputes the key at READ time,
// such a venue also stops serving street coordinates until that happens.
//
// MEASURED against production on 2026-07-29, not assumed: exactly one venue
// matches (128, Altar at the Masquerade), and it has NO street coordinates today
// -- it is the miss this ticket exists to fix. So nothing regresses and no
// one-shot backfill is needed. Re-measure before widening the table:
//
//	SELECT id, name, address, street_latitude IS NOT NULL AS has_coords
//	FROM venues WHERE address ~* 'mlk' OR address ~* 'm\.l\.k';
func streetGeocodeQuery(v *catalogm.Venue) geo.AddressQuery {
	return geo.AddressQuery{
		Street:  geo.ExpandStreetAbbreviations(derefString(v.Address)),
		City:    v.City,
		State:   v.State,
		Zipcode: derefString(v.Zipcode),
		Country: derefString(v.Country),
	}
}

// Two DISTINCT predicates govern the street-geocode columns — don't conflate
// them:
//
//   - streetGeocodeAttempted: "this exact address key was already looked up"
//     (hit OR recorded miss). Writers use it to skip redundant network calls;
//     a clean miss records the key with NULL coords (miss memo) so unresolvable
//     addresses aren't re-queried forever.
//   - streetGeocodeFresh: "there is a servable HIT for the current key" —
//     attempted AND coordinates present. The API read gate uses this; a miss
//     memo or a stale key never serves coordinates.

// streetGeocodeAttempted reports whether the venue's CURRENT address key has
// already been geocoded (successfully or as a recorded miss).
func streetGeocodeAttempted(v *catalogm.Venue) bool {
	return v.GeocodedAddress != nil && *v.GeocodedAddress == streetGeocodeQuery(v).Key()
}

// streetGeocodeFresh reports whether the venue's stored street coordinates
// were produced from its CURRENT address key. False means no servable geocode
// exists — never looked up, a recorded miss, or the address changed through a
// path that doesn't re-geocode inline (e.g. a contribution edit). Stale
// coordinates must never be served.
func streetGeocodeFresh(v *catalogm.Venue) bool {
	return streetGeocodeAttempted(v) &&
		v.StreetLatitude != nil && v.StreetLongitude != nil
}

// applyStreetGeocoding resolves and sets the venue's street-level coordinate
// fields from its address via the network AddressGeocoder (PSY-1536).
// Log-and-continue by contract: a failure or miss must NEVER block or fail the
// venue write — the coordinates are simply left NULL (never stale: they are
// cleared before the lookup). A transport/service error leaves the key NULL
// too, so a later run (or the manually-run geocode-venue-addresses backfill)
// retries; a CLEAN miss records the key as a miss memo so the same
// unresolvable address isn't re-queried on every touch. An unchanged address
// key (streetGeocodeAttempted) short-circuits without a network call.
func (s *VenueService) applyStreetGeocoding(v *catalogm.Venue) {
	q := streetGeocodeQuery(v)
	if strings.TrimSpace(q.Street) == "" {
		// No street address — nothing street-level can exist.
		v.StreetLatitude, v.StreetLongitude, v.GeocodePrecision, v.GeocodedAddress = nil, nil, nil, nil
		return
	}
	key := q.Key()
	if streetGeocodeAttempted(v) {
		return // this exact address was already looked up — keep the outcome
	}
	// Clear first so an error/miss below can never leave coordinates that
	// belong to a previous address.
	v.StreetLatitude, v.StreetLongitude, v.GeocodePrecision, v.GeocodedAddress = nil, nil, nil, nil
	if s.addressGeocoder == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), streetGeocodeTimeout)
	defer cancel()
	// Same stored-address-then-expanded path the sweep uses, and it must stay
	// that way: a venue created through this inline path would otherwise memoize
	// a miss, which the sweep then skips by design — so the abbreviation fix
	// would silently never reach it.
	res, ok, err := geocodeExpandingAbbreviations(ctx, s.addressGeocoder, q, derefString(v.Address))
	if err != nil {
		// Deliberately NOT logging the address — unverified submissions may be
		// private home addresses, and app logs have broader access/retention
		// than the DB. Name+city is enough to find the row.
		slog.Warn("street geocoding failed; continuing without street coords",
			"venue", v.Name, "city", v.City, "state", v.State, "error", err)
		return
	}
	if !ok {
		v.GeocodedAddress = &key // miss memo: don't re-query this exact key
		return
	}
	v.StreetLatitude = &res.Latitude
	v.StreetLongitude = &res.Longitude
	v.GeocodePrecision = &res.Precision
	v.GeocodedAddress = &key
}

// CreateVenue creates a new venue
// If isAdmin is true, the venue is automatically verified.
// If isAdmin is false, the venue requires admin approval (verified = false).
func (s *VenueService) CreateVenue(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists (same name in same city)
	var existingVenue catalogm.Venue
	err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", req.Name, req.City).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists in %s", req.Name, req.City)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Generate unique slug
	baseSlug := utils.GenerateVenueSlug(req.Name, req.City, req.State)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// A blank age policy means "no house default", which is SQL NULL: storing
	// '' would make a blank policy read as a present one. Trimmed first so a
	// whitespace-only value collapses too, and so "21+" and " 21+ " do not
	// become two buckets once a controlled vocabulary is derived from the
	// observed values. Note Description and ImageURL below are still written
	// through raw on create (they normalize only on update), so this is
	// deliberately NOT symmetric with its neighbours; changing theirs is a
	// behavior change for another PR.
	agePolicy := utils.NilIfBlankPtr(req.AgePolicy)

	// Create the venue - verified if created by admin, unverified otherwise
	venue := &catalogm.Venue{
		Name:        req.Name,
		Slug:        &slug,
		Address:     req.Address,
		City:        req.City,
		State:       req.State,
		Country:     req.Country,
		Zipcode:     req.Zipcode,
		Capacity:    req.Capacity,
		AgePolicy:   agePolicy,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		Verified:    isAdmin, // Admins create verified venues, non-admins require approval
		SubmittedBy: req.SubmittedBy,
		Social: catalogm.Social{
			Instagram:  req.Instagram,
			Facebook:   req.Facebook,
			Twitter:    req.Twitter,
			YouTube:    req.YouTube,
			Spotify:    req.Spotify,
			SoundCloud: req.SoundCloud,
			Bandcamp:   req.Bandcamp,
			Website:    req.Website,
		},
	}

	s.applyGeocoding(s.db, venue)
	s.applyStreetGeocoding(venue)

	if err := s.db.Create(venue).Error; err != nil {
		return nil, fmt.Errorf("failed to create venue: %w", err)
	}

	return s.buildVenueResponse(venue), nil
}

// GetVenue retrieves a venue by ID
func (s *VenueService) GetVenue(venueID uint) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// GetVenueBySlug retrieves a venue by slug
func (s *VenueService) GetVenueBySlug(slug string) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	err := s.db.Where("slug = ?", slug).First(&venue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(0)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// GetVenueDetail resolves a venue by numeric ID or slug and returns it WITH
// its provenance stamp — the venue-detail read, and the only read that pays
// for the stamp.
//
// Deliberately separate from GetVenue/GetVenueBySlug. Those are the cheap
// identity lookups the rest of the backend leans on: snapshotting a row before
// an admin edit, resolving a slug to an ID for an unrelated sub-resource.
// None of them render provenance, so attaching the aggregations there would
// make every one of them pay for a field it discards — and would hide that
// cost the day the stamp gains a more expensive signal.
func (s *VenueService) GetVenueDetail(idOrSlug string) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	query := s.db
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		query = query.Where("id = ?", uint(id))
	} else {
		query = query.Where("slug = ?", idOrSlug)
	}
	if err := query.First(&venue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(0)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	resp := s.buildVenueResponse(&venue)
	resp.Provenance = s.venueProvenanceFor(&venue)
	return resp, nil
}

// GetVenues retrieves venues with optional filtering
func (s *VenueService) GetVenues(filters map[string]interface{}) ([]*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db

	// Apply filters
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if name, ok := filters["name"].(string); ok && name != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", shared.LikePattern(name))
	}
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		query = ApplyTagFilter(query, s.db, catalogm.TagEntityVenue, "venues.id", tf)
	}

	// Default ordering by name
	query = query.Order("name ASC")

	var venues []catalogm.Venue
	err := query.Find(&venues).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*contracts.VenueDetailResponse, len(venues))
	for i, venue := range venues {
		responses[i] = s.buildVenueResponse(&venue)
	}

	return responses, nil
}

// UpdateVenue updates an existing venue
func (s *VenueService) UpdateVenue(venueID uint, req *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if name/city is being updated and if it conflicts with existing venue
	// Get current venue to check its city
	var currentVenue catalogm.Venue
	err := s.db.First(&currentVenue, venueID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Determine what values to check for the duplicate guard. A provided name
	// or city replaces the current value; an empty provided value falls back
	// to the current (mirrors the prior map-key check semantics).
	checkName := currentVenue.Name
	if req.Name != nil && *req.Name != "" {
		checkName = *req.Name
	}
	checkCity := currentVenue.City
	if req.City != nil && *req.City != "" {
		checkCity = *req.City
	}

	// Check for duplicate venue with same name and city (excluding current venue)
	var existingVenue catalogm.Venue
	err = s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?) AND id != ?", checkName, checkCity, venueID).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists in %s", checkName, checkCity)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Translate the typed request into a GORM update map. Only non-nil fields
	// are written so omitted fields stay unchanged. Name/City/State are NOT
	// NULL columns written verbatim; Description and ImageURL are nullable and
	// normalize empty input to SQL NULL. Address/Country/Zipcode and the social
	// fields are written through as-is to preserve prior behavior.
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.Zipcode != nil {
		updates["zipcode"] = *req.Zipcode
	}
	if req.Capacity != nil {
		updates["capacity"] = *req.Capacity
	}
	// Nullable free text: a blank string is the CLEAR gesture and must land as
	// SQL NULL, matching Description/ImageURL below. Trimmed, so whitespace-only
	// clears too and stored values do not carry incidental padding.
	if req.AgePolicy != nil {
		updates["age_policy"] = utils.NilIfBlank(*req.AgePolicy)
	}
	if req.Instagram != nil {
		updates["instagram"] = *req.Instagram
	}
	if req.Facebook != nil {
		updates["facebook"] = *req.Facebook
	}
	if req.Twitter != nil {
		updates["twitter"] = *req.Twitter
	}
	if req.YouTube != nil {
		updates["youtube"] = *req.YouTube
	}
	if req.Spotify != nil {
		updates["spotify"] = *req.Spotify
	}
	if req.SoundCloud != nil {
		updates["soundcloud"] = *req.SoundCloud
	}
	if req.Bandcamp != nil {
		updates["bandcamp"] = *req.Bandcamp
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.Description != nil {
		updates["description"] = utils.NilIfEmpty(*req.Description)
	}
	if req.ImageURL != nil {
		updates["image_url"] = utils.NilIfEmpty(*req.ImageURL)
	}

	// Re-geocode when any location field changes so the derived columns stay
	// consistent with the new city/state/country (PSY-985, metro PSY-1255 step B).
	//
	// ApplyToUpdates writes the whole derived set unconditionally, nils included:
	// on a geocode miss GORM's map Updates turns those into SQL NULL, so a
	// relocated-but-unresolvable venue falls back to the legacy state->tz map
	// instead of keeping the OLD location's stale timezone/coordinates and the OLD
	// metro's CBSA. This block used to name the four columns itself, which meant a
	// fifth derived column would have been silently dropped on the update path
	// while the create path picked it up — the PSY-1709/PSY-1744 shape. Since
	// PSY-1747 the column set is known only to shared.DerivedVenueLocation.
	if shared.LocationTouched(updates) {
		// The effective post-update location. City comes from checkCity rather than
		// updates["city"] because the duplicate-name check above already coalesced
		// it; that is the one component this path resolves differently from the
		// generic EffectiveLocation overlay, which is why it is built by hand.
		loc := shared.Location{
			City:    checkCity,
			State:   currentVenue.State,
			Country: shared.DerefOrEmpty(currentVenue.Country),
		}
		if req.State != nil {
			loc.State = *req.State
		}
		if req.Country != nil {
			loc.Country = *req.Country
		}
		// currentVenue.Name, not the bare struct this used to build, so a rejected
		// timezone logs the venue an operator can actually look up.
		shared.DeriveVenueLocation(s.db, s.geocoder, loc,
			"venue_name", currentVenue.Name, "city", loc.City, "state", loc.State).
			ApplyToUpdates(updates)
	}

	// Street-level geocode (PSY-1536): recompute when any component of the
	// address key (address, city, state, country, zipcode) changes. The
	// effective venue mirrors the VERBATIM column writes above (not the
	// coalesced duplicate-check values) so the stored geocoded_address key
	// always matches what streetGeocodeFresh recomputes at read time. An
	// update that doesn't actually change the key short-circuits inside
	// applyStreetGeocoding without a network call; a geocode failure writes
	// NULLs (never stale coords under a new address) and the venue update
	// itself always proceeds.
	if req.Address != nil || req.City != nil || req.State != nil || req.Country != nil || req.Zipcode != nil {
		effective := currentVenue
		if req.Address != nil {
			effective.Address = req.Address
		}
		if req.City != nil {
			effective.City = *req.City
		}
		if req.State != nil {
			effective.State = *req.State
		}
		if req.Country != nil {
			effective.Country = req.Country
		}
		if req.Zipcode != nil {
			effective.Zipcode = req.Zipcode
		}
		s.applyStreetGeocoding(&effective)
		updates["street_latitude"] = effective.StreetLatitude
		updates["street_longitude"] = effective.StreetLongitude
		updates["geocode_precision"] = effective.GeocodePrecision
		updates["geocoded_address"] = effective.GeocodedAddress
	}

	// Update the venue
	if len(updates) > 0 {
		err = s.db.Model(&catalogm.Venue{}).Where("id = ?", venueID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update venue: %w", err)
		}
	}

	return s.GetVenue(venueID)
}

// DeleteVenue deletes a venue
func (s *VenueService) DeleteVenue(venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if venue exists
	var venue catalogm.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrVenueNotFound(venueID)
		}
		return fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if venue is associated with any shows
	var count int64
	err = s.db.Model(&catalogm.ShowVenue{}).Where("venue_id = ?", venueID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check venue associations: %w", err)
	}

	if count > 0 {
		return apperrors.ErrVenueHasShows(venueID, count)
	}

	// Delete the venue
	err = s.db.Delete(&venue).Error
	if err != nil {
		return fmt.Errorf("failed to delete venue: %w", err)
	}

	return nil
}

func (venueService *VenueService) SearchVenues(query string) ([]*contracts.VenueDetailResponse, error) {
	if venueService.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if query == "" {
		return []*contracts.VenueDetailResponse{}, nil
	}

	var venues []catalogm.Venue
	var err error

	if len(query) <= 2 {
		err = venueService.db.
			Where("LOWER(name) LIKE LOWER(?)", shared.LikePrefixPattern(query)).
			Order("name ASC").
			Limit(10).
			Find(&venues).Error
	} else {
		// `name % ?` is the pg_trgm similarity operator (NOT a LIKE pattern),
		// so its argument stays raw. The ILIKE branch escapes via LikePattern.
		err = venueService.db.
			Select("venues.*, similarity(name, ?) as sim_score", query).
			Where("name ILIKE ? OR name % ?", shared.LikePattern(query), query).
			Order("sim_score DESC, name ASC").
			Limit(10).
			Find(&venues).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search venues: %w", err)
	}

	responses := make([]*contracts.VenueDetailResponse, len(venues))
	for i, venue := range venues {
		responses[i] = venueService.buildVenueResponse(&venue)
	}

	return responses, nil
}

// FindOrCreateVenue finds an existing venue by name and city or creates a new one.
// This method can be used within a transaction context by passing a *gorm.DB.
// If tx is nil, it uses the service's default database connection.
// City and state are required - this function will return an error if they're empty.
// If isAdmin is true, new venues are automatically verified.
// If isAdmin is false, new venues require admin approval (verified = false).
// Returns the venue and a boolean indicating if it was newly created.
func (s *VenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error) {
	// Use provided db or fall back to service's db
	query := db
	if query == nil {
		query = s.db
	}

	if query == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}

	// Validate required fields
	if name == "" {
		return nil, false, fmt.Errorf("venue name is required")
	}
	if city == "" {
		return nil, false, fmt.Errorf("venue city is required")
	}
	if state == "" {
		return nil, false, fmt.Errorf("venue state is required")
	}

	// Check if venue already exists by name and city
	var venue catalogm.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error

	if err == nil {
		// Venue exists — backfill slug if missing
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			venue.Slug = &slug
			query.Model(&venue).Update("slug", slug)
		}
		return &venue, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// No EXACT name match. Before creating, retry on a normalized name.
	//
	// The exact-match lookup above (and the matching unique index on
	// (lower(name), lower(city))) did not stop "7th St Entry" being created
	// alongside "7th Street Entry" in Minneapolis — the two lowercase strings
	// genuinely differ. That duplicate then silently duplicated ~90 shows,
	// because show dedup keys on (artist_id, venue_id, event_date) and a second
	// venue row means a second venue_id. Matching here RESOLVES to the existing
	// room instead of rejecting the write, which is what ingest needs.
	//
	// Scoped to the same city, so normalization can never join rooms in
	// different places.
	if normalized := utils.NormalizeVenueName(name); normalized != "" {
		var candidates []catalogm.Venue
		if err := query.Where("LOWER(city) = LOWER(?)", city).Find(&candidates).Error; err != nil {
			return nil, false, fmt.Errorf("failed to check existing venue by normalized name: %w", err)
		}
		for i := range candidates {
			if utils.NormalizeVenueName(candidates[i].Name) == normalized {
				return &candidates[i], false, nil
			}
		}
	}

	// Venue doesn't exist, create it - verified if created by admin, unverified otherwise
	// Generate unique slug
	baseSlug := utils.GenerateVenueSlug(name, city, state)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	venue = catalogm.Venue{
		Name:     name,
		Slug:     &slug,
		Address:  address,
		City:     city,
		State:    state,
		Zipcode:  zipcode,
		Verified: isAdmin,           // Admins create verified venues, non-admins require approval
		Social:   catalogm.Social{}, // Empty social fields
	}

	s.applyGeocoding(query, &venue)
	// Street-level geocoding (PSY-1536) is deliberately NOT done inline here:
	// this is the bulk ingest/show-import seam, and blocking each created venue
	// on Nominatim's 1 req/s budget would stall imports. The street fields
	// start NULL (nothing to expose or go stale); the geocode-venue-addresses
	// backfill CLI resolves them afterwards.

	if err := query.Create(&venue).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, true, nil // true = newly created
}

// VerifyVenue marks a venue as verified by an admin.
func (s *VenueService) VerifyVenue(venueID uint) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the venue
	var venue catalogm.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if already verified
	if venue.Verified {
		return s.buildVenueResponse(&venue), nil
	}

	// Generate slug if missing
	updates := map[string]interface{}{"verified": true}
	if venue.Slug == nil {
		baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}

	// Update verified status (and slug if generated)
	if err := s.db.Model(&venue).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to verify venue: %w", err)
	}

	// Reload to get updated data
	if err := s.db.First(&venue, venueID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// buildVenueResponse converts a Venue model to contracts.VenueDetailResponse
func (s *VenueService) buildVenueResponse(venue *catalogm.Venue) *contracts.VenueDetailResponse {
	slug := ""
	if venue.Slug != nil {
		slug = *venue.Slug
	}
	// Hide address and zipcode for unverified venues. Both go through the shared
	// gate so a later change to the rule cannot land on one and miss the other,
	// which would leave this response self-contradicting (a zipcode narrowing
	// down the house whose street address we just withheld).
	address := venue.PublicAddress()
	zipcode := venue.PublicZipcode()
	// Street-precise coordinates (PSY-1536) follow the same privacy gate as
	// address/zipcode: VERIFIED venues only, so DIY/house venues are never
	// street-mapped before human review. The freshness check additionally
	// drops a geocode whose address has since changed through a path that
	// doesn't re-geocode inline (contribution edit, data-sync) — stale street
	// coordinates are never served regardless of which writer touched the row.
	var streetLat, streetLng *float64
	var geocodePrecision *string
	if venue.Verified && streetGeocodeFresh(venue) {
		streetLat = venue.StreetLatitude
		streetLng = venue.StreetLongitude
		geocodePrecision = venue.GeocodePrecision
	}

	// On AgePolicy being served ungated below while Address is redacted: it
	// matches Description, the other contributor-writable free-text venue field,
	// which is already public here. That is a POSTURE, not a safety proof. A
	// trusted-tier contributor could type an address into either field on an
	// unverified venue, which is exactly what the Address gate exists to
	// prevent. The write boundary bounds and type-checks the value; only a
	// controlled vocabulary would close the channel properly.
	return &contracts.VenueDetailResponse{
		ID:               venue.ID,
		Slug:             slug,
		Name:             venue.Name,
		Address:          address,
		City:             venue.City,
		State:            venue.State,
		Country:          venue.Country,
		Latitude:         venue.Latitude,
		Longitude:        venue.Longitude,
		StreetLatitude:   streetLat,
		StreetLongitude:  streetLng,
		GeocodePrecision: geocodePrecision,
		Timezone:         venue.Timezone,
		Zipcode:          zipcode,
		Capacity:         venue.Capacity,  // not redacted: capacity is not sensitive
		AgePolicy:        venue.AgePolicy, // not redacted: see the note above
		Description:      venue.Description,
		ImageURL:         venue.ImageURL,
		Verified:         venue.Verified,
		SubmittedBy:      venue.SubmittedBy,
		Social: contracts.SocialResponse{
			Instagram:  venue.Social.Instagram,
			Facebook:   venue.Social.Facebook,
			Twitter:    venue.Social.Twitter,
			YouTube:    venue.Social.YouTube,
			Spotify:    venue.Social.Spotify,
			SoundCloud: venue.Social.SoundCloud,
			Bandcamp:   venue.Social.Bandcamp,
			Website:    venue.Social.Website,
		},
		CreatedAt: venue.CreatedAt,
		UpdatedAt: venue.UpdatedAt,
	}
}

// contracts.VenueWithShowCountResponse represents a venue with its upcoming show count

// VenueWithCount is used internally for querying venues with their show counts
type VenueWithCount struct {
	catalogm.Venue
	UpcomingShowCount int64 `gorm:"column:upcoming_show_count"`
}

// metroRollupPredicate returns the WHERE fragment that widens a City+State
// filter to the whole CBSA metro (PSY-1574), plus its bind args, plus whether
// it applies at all.
//
// The metro half comes from `metroScopeFor` — the SAME resolution the Atlas
// scene itself is built on — so "which venues belong to the Phoenix scene" has
// exactly one definition and the rail cannot drift from the scene page beside
// it. It yields `venues.metro = <CBSA>`, the denormalized rollup written by
// every venue write path (applyGeocoding) and indexed.
//
// It is a UNION with the literal city, not a replacement, and the ticket's own
// verb is why: the rail INCLUDES metro members, it does not trade the city for
// the metro. `venues.metro` is a derived column reconciled by a human-run
// backfill (cmd/backfill-entity-metro), so a venue sitting in a CBSA city with
// a NULL metro is a real state — and swapping the city predicate for the metro
// one would silently empty that city's rail of venues it lists today. The union
// can only ever be a superset of both readings: every row it adds is either in
// the metro or literally in the city.
//
// It deliberately does NOT extend the scope to member-city NAMES the way
// artistPredicate does. Artists needed that because `artists.metro` is often
// NULL for a hand-entered home town and a member city is the only other signal;
// here the principal city is already unioned in, and enumerating hundreds of
// member-city names (New York's CBSA has 889 places in the dataset) would put a
// vast OR-chain on a query that the indexed metro equality already answers.
//
// ok is false unless MetroRollup was asked for AND both City and State are
// present: a metro cannot be resolved from a bare city name without risking the
// wrong namesake (Pasadena CA vs TX), and silently widening to the wrong metro
// is worse than not widening.
func (s *VenueService) metroRollupPredicate(filters contracts.VenueListFilters) (string, []any, bool) {
	if !filters.MetroRollup || filters.City == "" || filters.State == "" {
		return "", nil, false
	}
	scope := metroScopeFor(s.geocoder, filters.City, filters.State)
	pred, args := scope.venuePredicate("venues")
	if !scope.isMetro() {
		// No CBSA: the scope IS the literal city already, so there is nothing
		// to union — and it matches case-insensitively, which the plain
		// `city = ?` filter it replaces did not.
		return pred, args, true
	}
	cityPred, cityArgs := sceneScope{city: filters.City, state: filters.State}.venuePredicate("venues")
	all := make([]any, 0, len(args)+len(cityArgs))
	all = append(all, args...)
	all = append(all, cityArgs...)
	return "(" + pred + " OR (" + cityPred + "))", all, true
}

// venueBrowseGate is what makes a venue public: it is verified.
//
// Shared by every query behind the /venues page — the list, its city facet, and
// the ItemList projection — so a future condition (soft delete, a hidden flag, a
// country scope) cannot be added to one and missed by the others. That would
// leave the page, its facet counts, and the schema block describing three
// different sets, which is the failure class this endpoint exists to remove.
//
// NOT shared by the admin-side unverified queries below: those are looking for
// the complement on purpose, and expressing them as "not the browse gate" would
// couple two rules that should be free to diverge.
//
// Qualified with the table name so it reads the same in a joined query as in a
// bare count; every caller selects FROM venues.
const venueBrowseGate = "venues.verified = ?"

// venueListingRow is what one statement has to carry to answer the listing
// without a second: the projected columns, plus the window count of the whole
// gated set beside every row. Slug is a pointer because this query deliberately
// selects rows the projection will drop.
type venueListingRow struct {
	Slug  *string
	Name  string
	Total int64
}

// GetVenueListing returns the slug+name projection behind GET /venues/listing,
// plus the size of the browse set it was projected from.
//
// It lists the same SET as an unfiltered GetVenuesWithShowCounts — which venues
// does /venues list — while reading two columns instead of hydrating a full
// response per row, and without a limit. Both halves matter: the width is what
// stops the response fitting a cache entry, and the missing limit is what stops
// the JSON-LD ItemList advertising a prefix of the catalogue. See
// contracts.VenueListingEntry for the measurements.
//
// THE GATE IS SHARED, THE ORDER DELIBERATELY IS NOT, and the asymmetry is the
// interesting part. venueBrowseGate is what makes the two sets equal, so it is
// stated once. The browse page's activity order is NOT reused here, unlike the
// artist twin, for two reasons that only apply to a complete list:
//
//   - The consumer stamps ItemList `position` from this order. Under an activity
//     sort, booking one show at one venue renumbers the whole list, so the
//     `/venues` HTML churns on every hourly revalidation with no change to the
//     set. Alphabetical is stable against everything except the venue list.
//   - Reproducing that order costs a LEFT JOIN against an aggregate over every
//     upcoming approved booking — the one input here that grows with SHOWS
//     rather than with venues — on a public unauthenticated endpoint, to sort a
//     list whose consumer renders all of it anyway.
//
// The browse page itself pages at 50, so an ItemList of every venue could not
// have matched its order past the first page in any case.
//
// TOTAL AND THE ENTRIES COME FROM ONE STATEMENT, which is what makes comparing
// them meaningful rather than racy. Total is the gated set BEFORE the slug
// filter — the same number `GET /venues` reports — and the window count is
// evaluated over the same snapshot as the rows, so the two cannot disagree
// because a venue was merged or verified between two round trips. A caller that
// renders one link per entry can therefore read a gap as exactly one thing:
// venues that cannot form a URL.
//
// Rows with a NULL or empty slug are dropped here rather than by the caller: a
// slug is what builds the URL, so an entry without one is unusable to any
// consumer. That is a data condition rather than a code defect — GenerateSlug
// can return "" for a name with no Latin characters — so it is logged here,
// where the row can actually be fixed.
func (s *VenueService) GetVenueListing() ([]contracts.VenueListingEntry, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	var rows []venueListingRow
	// COUNT(*) OVER () with no window ORDER BY counts the whole gated set, so it
	// is the pre-slug-filter total without a second statement.
	err := s.db.Table("venues").
		Select("venues.slug, venues.name, COUNT(*) OVER () as total").
		Where(venueBrowseGate, true).
		Order("venues.name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get venue listing: %w", err)
	}

	// Non-nil even when empty: the handler serialises this straight to JSON, and
	// a null collection is indistinguishable from a failed fetch to the caller.
	entries := make([]contracts.VenueListingEntry, 0, len(rows))
	var total int64
	for _, row := range rows {
		// Every row carries the same window count; an empty set has no row to
		// carry it, and a total of zero is the right answer there anyway.
		total = row.Total
		if row.Slug == nil || *row.Slug == "" {
			continue
		}
		entries = append(entries, contracts.VenueListingEntry{Slug: *row.Slug, Name: row.Name})
	}

	if gap := total - int64(len(entries)); gap > 0 {
		slog.Warn("venue_listing_unlinkable_venues",
			"unlinkable", gap,
			"total", total,
			"listed", len(entries),
			"detail", "verified venues with no slug cannot appear in the /venues ItemList; backfill their slugs",
		)
	}

	return entries, total, nil
}

// GetVenuesWithShowCounts retrieves verified venues with their upcoming show counts.
// Results are sorted by upcoming show count (descending), then by name (ascending),
// so venues with upcoming shows appear first.
func (s *VenueService) GetVenuesWithShowCounts(filters contracts.VenueListFilters, limit, offset int) ([]*contracts.VenueWithShowCountResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Build the base query with show count subquery
	// This allows us to sort by show count while also paginating correctly
	subquery := s.db.Table("show_venues").
		Select("show_venues.venue_id, COUNT(*) as show_count").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, catalogm.ShowStatusApproved).
		Group("show_venues.venue_id")

	// Start with verified venues only for public display
	query := s.db.Table("venues").
		Select("venues.*, COALESCE(sc.show_count, 0) as upcoming_show_count").
		Joins("LEFT JOIN (?) as sc ON venues.id = sc.venue_id", subquery).
		Where(venueBrowseGate, true)

	// Apply optional filters
	metroPred, metroArgs, metroRollup := s.metroRollupPredicate(filters)
	if len(filters.Cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range filters.Cities {
			if cs.City != "" && cs.State != "" {
				conditions = append(conditions, "(venues.city = ? AND venues.state = ?)")
				args = append(args, cs.City, cs.State)
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else if metroRollup {
		query = query.Where(metroPred, metroArgs...)
	} else {
		if filters.State != "" {
			query = query.Where("venues.state = ?", filters.State)
		}
		if filters.City != "" {
			query = query.Where("venues.city = ?", filters.City)
		}
	}
	tf := TagFilter{TagSlugs: filters.TagSlugs, MatchAny: filters.TagMatchAny}
	query = ApplyTagFilter(query, s.db, catalogm.TagEntityVenue, "venues.id", tf)

	// Get total count of matching venues
	var total int64
	countQuery := s.db.Table("venues").Where(venueBrowseGate, true)
	if len(filters.Cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range filters.Cities {
			if cs.City != "" && cs.State != "" {
				conditions = append(conditions, "(city = ? AND state = ?)")
				args = append(args, cs.City, cs.State)
			}
		}
		if len(conditions) > 0 {
			countQuery = countQuery.Where(strings.Join(conditions, " OR "), args...)
		}
	} else if metroRollup {
		// Same predicate object, not a re-derivation: the total under the list
		// must be counted over exactly the rows the list pages through, and
		// "venues." qualifies fine here — this query's table IS venues.
		countQuery = countQuery.Where(metroPred, metroArgs...)
	} else {
		if filters.State != "" {
			countQuery = countQuery.Where("state = ?", filters.State)
		}
		if filters.City != "" {
			countQuery = countQuery.Where("city = ?", filters.City)
		}
	}
	countQuery = ApplyTagFilter(countQuery, s.db, catalogm.TagEntityVenue, "venues.id", tf)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	}

	// Get venues with pagination, sorted by show count (desc) then name (asc)
	var venuesWithCount []VenueWithCount
	if err := query.Order("upcoming_show_count DESC, venues.name ASC").Limit(limit).Offset(offset).Find(&venuesWithCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*contracts.VenueWithShowCountResponse, len(venuesWithCount))
	dataSources := make(map[uint]*string, len(venuesWithCount))
	for i, vc := range venuesWithCount {
		responses[i] = &contracts.VenueWithShowCountResponse{
			VenueDetailResponse: *s.buildVenueResponse(&vc.Venue),
			UpcomingShowCount:   int(vc.UpcomingShowCount),
		}
		// data_source is not part of the serialized venue response (it is an
		// internal provenance column), so carry it alongside for the stamp.
		dataSources[vc.ID] = vc.DataSource
	}

	// Atlas venue-rail payload (next show, next-7-days slice, dominant genre) for
	// the venues on THIS page — three batched scans, no N+1, all best effort.
	// Opt-in: the venue browse page is this endpoint's other caller and renders
	// none of those fields, so it must not pay for them. See venue_rail.go.
	if filters.IncludeRailFields {
		s.enrichVenueRailFields(responses, now)
		// The freshness stamp rides the same opt-in for the same reason: two
		// more batched scans the browse page has no use for. See
		// venue_provenance.go.
		s.enrichVenueProvenance(responses, dataSources)
	}

	return responses, total, nil
}

// GetShowsForVenue retrieves a page of shows at a specific venue.
// query.TimeFilter can be: "upcoming", "past", or "all". Only returns approved
// shows.
//
// "today" is the show's OWN primary-venue calendar day (shared.VenueTZJoin)
// rather than this venue's. Identical for every single-venue show, and
// deliberate for the rest: one show gets one venue-local date, so it cannot read
// "upcoming" here and "past" on the artist page. The venue ICS feed
// (engagement/venue_calendar.go) still uses the QUERIED venue's zone and can
// therefore still disagree for a multi-venue show — see shared's header note.
// The page window and total both apply to the venue-local, year-filtered
// partition, so a caller can page to the end of `total` without a page
// disagreeing with the count above it.
//
// Deprecated parameter: timezone is accepted and ignored. It used to set the
// boundary from the CALLER's zone, which made the same show upcoming for one
// reader and past for another. Kept in the signature because it is the only
// thing TestGetShowsForVenue_SamePartitionForEveryCallerZone can vary to prove
// the boundary no longer moves with the reader; the frontend stopped sending it
// in PSY-1762. Do not add new callers that pass a meaningful value.
func (s *VenueService) GetShowsForVenue(venueID uint, timezone string, query contracts.VenueShowsQuery) ([]*contracts.VenueShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify venue exists
	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, 0, fmt.Errorf("failed to get venue: %w", err)
	}

	var orderDirection string
	if query.TimeFilter == "past" {
		orderDirection = "shows.event_date DESC" // Most recent past shows first
	} else {
		orderDirection = "shows.event_date ASC" // Soonest upcoming shows first
	}
	// Deterministic tiebreak on equal event_date, matching GetShowsForArtist
	// (PSY-1352): without it the Pluck below and the Find that re-orders the
	// same ids are each free to break a tie differently, so a venue with two
	// shows on one date could return them in one order and page them in another.
	// It is also what makes OFFSET paging safe: an unstable tiebreak lets one
	// show appear on two pages and another on none.
	orderDirection += ", shows.id ASC"

	baseQuery := s.venueShowsBaseQuery(venueID, query.TimeFilter, query.Year, venueZoneNotNeededBySelect)

	// Count total shows matching the filter
	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count shows: %w", err)
	}

	limit, offset := clampPageWindow(query.Limit, query.Offset)

	// Get the page of show IDs. Offset(0) is a no-op in GORM's clause builder,
	// so the unpaged first page plans exactly as it did before.
	var showIDs []uint
	if err := baseQuery().
		Select("show_venues.show_id").
		Order(orderDirection).
		Limit(limit).
		Offset(offset).
		Pluck("show_venues.show_id", &showIDs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get show IDs: %w", err)
	}

	// Fetch full show data
	var shows []catalogm.Show
	if len(showIDs) > 0 {
		if err := s.db.Where("id IN ?", showIDs).Order(orderDirection).Find(&shows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get shows: %w", err)
		}
	}

	// Batch-load all ShowArtist records and collect artist IDs
	allShowArtists := make(map[uint][]catalogm.ShowArtist)
	var allArtistIDs []uint
	if len(shows) > 0 {
		var showArtistRecords []catalogm.ShowArtist
		showIDsForArtists := make([]uint, len(shows))
		for i, show := range shows {
			showIDsForArtists[i] = show.ID
		}
		s.db.Where("show_id IN ?", showIDsForArtists).Order("position ASC").Find(&showArtistRecords)
		for _, sa := range showArtistRecords {
			allShowArtists[sa.ShowID] = append(allShowArtists[sa.ShowID], sa)
			allArtistIDs = append(allArtistIDs, sa.ArtistID)
		}
	}

	// Batch-fetch all artists in one query
	artistMap := make(map[uint]*catalogm.Artist)
	if len(allArtistIDs) > 0 {
		var allArtists []catalogm.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&allArtists)
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}
	}

	// Build responses using map lookup
	responses := make([]*contracts.VenueShowResponse, len(shows))
	for i, show := range shows {
		artists := make([]contracts.ArtistResponse, 0)
		for _, sa := range allShowArtists[show.ID] {
			artist, ok := artistMap[sa.ArtistID]
			if !ok {
				continue
			}
			isHeadliner := sa.SetType == "headliner"
			isNewArtist := false
			socials := contracts.ShowArtistSocials{
				Instagram:  artist.Social.Instagram,
				Facebook:   artist.Social.Facebook,
				Twitter:    artist.Social.Twitter,
				YouTube:    artist.Social.YouTube,
				Spotify:    artist.Social.Spotify,
				SoundCloud: artist.Social.SoundCloud,
				Bandcamp:   artist.Social.Bandcamp,
				Website:    artist.Social.Website,
			}
			var slug string
			if artist.Slug != nil {
				slug = *artist.Slug
			}
			artists = append(artists, contracts.ArtistResponse{
				ID:          artist.ID,
				Slug:        slug,
				Name:        artist.Name,
				State:       artist.State,
				City:        artist.City,
				Country:     artist.Country,
				IsHeadliner: &isHeadliner,
				SetType:     sa.SetType,
				Position:    sa.Position,
				IsNewArtist: &isNewArtist,
				Socials:     socials,
			})
		}

		var showSlug string
		if show.Slug != nil {
			showSlug = *show.Slug
		}

		responses[i] = &contracts.VenueShowResponse{
			ID:             show.ID,
			Slug:           showSlug,
			Title:          show.Title,
			EventDate:      show.EventDate,
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			IsCancelled:    show.IsCancelled,
			IsSoldOut:      show.IsSoldOut,
			Artists:        artists,
		}
	}

	return responses, total, nil
}

// venueShowsBaseQuery returns a builder FACTORY for one venue's approved shows
// under a time filter and optional venue-local year.
//
// A factory rather than a builder because GORM builders accumulate clauses: the
// COUNT and the id page must not share one, or the page's ORDER/LIMIT leak into
// the count. Every read of a venue's show list goes through here so the count,
// the page and the year histogram cannot drift apart about what they are
// counting.
func (s *VenueService) venueShowsBaseQuery(venueID uint, timeFilter string, year int, selectNeedsVenueZone bool) func() *gorm.DB {
	// Partition on each show's own venue-local calendar day. The fragment is
	// empty for "all".
	dateCondition := shared.VenueLocalDateCondition(timeFilter)
	yearCondition, yearArgs := shared.VenueLocalYearCondition(year)

	// Both exact conditions dereference venue_tz, so the lateral is needed if
	// EITHER is present: "all" plus a year still needs it. Joined at most once,
	// because a second copy would alias-collide. Left out entirely for an unfiltered
	// "all" list, which is the one shape that can read every row without a zone.
	needsVenueZone := selectNeedsVenueZone || dateCondition != "" || yearCondition != ""

	return func() *gorm.DB {
		q := s.db.Table("show_venues").
			Joins("JOIN shows ON show_venues.show_id = shows.id").
			Where("show_venues.venue_id = ? AND shows.status = ?", venueID, catalogm.ShowStatusApproved)
		if needsVenueZone {
			q = q.Joins(shared.VenueTZJoin)
		}
		if dateCondition != "" {
			q = q.Where(dateCondition)
		}
		if yearCondition != "" {
			q = q.Where(yearCondition, yearArgs...)
		}
		return q
	}
}

// GetVenueShowYears returns the venue's show counts bucketed by VENUE-LOCAL
// calendar year, newest year first, for the given time filter ("upcoming",
// "past" or "all"). Only approved shows are counted.
//
// Years with no shows are absent rather than zero: the histogram's consumer is a
// year picker, and an empty year is not a selectable option. That also means the
// result is naturally sparse for a venue with gaps in its history.
//
// It is a separate read from GetShowsForVenue rather than a field on that
// response because the two answer different questions: the histogram spans every
// year and must NOT narrow to the requested one, or selecting a year would erase
// every other option from the picker that produced it. It is also invariant
// across offset, so bundling it would recompute an unchanging aggregate on every
// page turn.
func (s *VenueService) GetVenueShowYears(venueID uint, timeFilter string) ([]contracts.VenueShowYearCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// year 0: the histogram is the thing that ENUMERATES years, so it can never
	// be narrowed to one.
	baseQuery := s.venueShowsBaseQuery(venueID, timeFilter, 0, venueZoneNeededBySelect)

	buckets, err := scanVenueLocalYearBuckets(baseQuery)
	if err != nil {
		return nil, err
	}

	// Non-nil even when empty: the histogram must serialize as [] rather than null.
	years := make([]contracts.VenueShowYearCount, len(buckets))
	for i, bucket := range buckets {
		years[i] = contracts.VenueShowYearCount{Year: bucket.Year, Count: bucket.Count}
	}
	return years, nil
}

// GetVenueShowMonths returns the venue's show counts bucketed by VENUE-LOCAL
// calendar month, newest month first, for the given time filter ("upcoming",
// "past" or "all"). Only approved shows are counted.
//
// It exists so the archive's pager can say what is BEHIND a page number without
// fetching that page (PSY-1769). A page's month span is a function of the row
// ordinals it covers, and cumulative counts answer that for every page at once —
// where deriving it from rows could only ever label the pages the reader had
// already visited, which is the whole defect.
//
// Months with no shows are absent rather than zero. Nothing downstream needs the
// gaps: the labels are computed by walking cumulative counts, and a month with no
// rows moves no page boundary.
//
// It spans EVERY year, like the year histogram and for a related reason: the
// archive's default view is every year, and its year-scoped views are slices of
// this same list. One read per venue therefore serves all of them, and switching
// years never leaves the pager briefly labelled from the previous year's counts.
func (s *VenueService) GetVenueShowMonths(venueID uint, timeFilter string) ([]contracts.VenueShowMonthCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// year 0: see GetVenueShowYears. A histogram narrowed to one year could not
	// label the all-years pager, which is the surface that needs it most.
	baseQuery := s.venueShowsBaseQuery(venueID, timeFilter, 0, venueZoneNeededBySelect)

	buckets, err := scanVenueLocalMonthBuckets(baseQuery)
	if err != nil {
		return nil, err
	}

	// Non-nil even when empty: the histogram must serialize as [] rather than null.
	months := make([]contracts.VenueShowMonthCount, len(buckets))
	for i, bucket := range buckets {
		months[i] = contracts.VenueShowMonthCount{
			Year:  bucket.Year,
			Month: bucket.Month,
			Count: bucket.Count,
		}
	}
	return months, nil
}
// HasPastShowsInYear reports whether the venue has at least one approved show on
// the ARCHIVE's side of the venue-local upcoming/past boundary, inside `year`.
//
// It is the existence question `/venues/{slug}/shows/{year}` asks before it will
// render, and nothing more: the answer is one bit, so this is the cheapest read
// that can settle it. Reached through the frontend proxy's venue-year branch —
// grep `shows/{year}/exists` there — because a page under cacheComponents cannot
// set its own 404 status.
//
// THREE SURFACES MUST AGREE on which years are documents: this probe, the
// histogram (GetVenueShowYears), and the `venue_years` sitemap family. They do,
// but NOT by construction, and the difference matters to anyone editing them.
// This and the histogram share venueShowsBaseQuery; the sitemap
// (catalog/sitemap.go venueYearEntries) is a separate hand-written query that
// merely composes the same shared.VenueLocalDateCondition("past") and
// VenueLocalYearSQL fragments. So adding a predicate HERE — say
// `AND shows.is_cancelled = false` — moves the page and the probe together and
// leaves the sitemap announcing years that now 404. What actually enforces the
// agreement is two chained tests: sitemap↔histogram in sitemap_integration_test,
// and probe↔histogram in TestHasPastShowsInYear_AgreesWithThePastHistogram.
// Change the predicate and run both.
//
// PAST is fixed rather than a parameter. A year archive is past-only by
// definition — the page 404s a year whose shows are all still upcoming — and a
// probe that could be asked a question its only caller never asks is surface
// that has to be kept correct for nobody. Widening it later is additive.
//
// Cheaper than the histogram, and the honest version of "cheaper" is narrower
// than it looks. What is certain: no GROUP BY over the venue's whole history, no
// venue zone in the SELECT (only the WHERE dereferences venue_tz), and LIMIT 1,
// so a POPULATED year stops at the first matching row. What is NOT verified is
// the plan: the query drives from show_venues on venue_id, and whether
// VenueLocalYearCondition's UTC bounds become an index range on
// idx_shows_event_date or a filter applied after the pk probe into shows has not
// been EXPLAINed against production-shaped data. For an EMPTY year — the common
// case when a crawler walks the year space — LIMIT 1 cannot stop early, so the
// cost scales with the venue's history rather than the year's.
//
// Do not harden that comment without measuring. This file has been wrong about
// its own plans before: see the loops=17228 note in
// services/shared/show_venue_local_sql.go, where a shape that looked equivalent
// re-scanned per row.
//
// It does NOT verify the venue exists, unlike its siblings, and the difference
// is invisible to every caller: an unknown venue has no shows, so both roads end
// at the same 404 at the handler. The handler's own slug resolution already
// separates the two cases for the shape a caller actually sends.
func (s *VenueService) HasPastShowsInYear(venueID uint, year int) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	baseQuery := s.venueShowsBaseQuery(venueID, "past", year, venueZoneNotNeededBySelect)

	// show_id is a primary key, so a zero here can only mean "no row matched" —
	// the same test EntityExistenceService makes against an id column.
	var showID uint
	if err := baseQuery().
		Select("show_venues.show_id").
		Limit(1).
		Scan(&showID).Error; err != nil {
		return false, fmt.Errorf("failed to probe venue shows for year: %w", err)
	}
	return showID != 0, nil
}

// contracts.VenueCityResponse represents a city with venue count for filtering

// GetVenueCities returns distinct cities that have verified venues, with venue counts.
// Results are sorted by venue count (descending) to show most active cities first.
func (s *VenueService) GetVenueCities() ([]*contracts.VenueCityResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type CityResult struct {
		City       string
		State      string
		VenueCount int64
	}

	var results []CityResult
	err := s.db.Table("venues").
		Select("city, state, COUNT(*) as venue_count").
		Where(venueBrowseGate, true).
		Group("city, state").
		Order("venue_count DESC, city ASC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venue cities: %w", err)
	}

	responses := make([]*contracts.VenueCityResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.VenueCityResponse{
			City:       r.City,
			State:      r.State,
			VenueCount: int(r.VenueCount),
		}
	}

	return responses, nil
}

// GetVenueGenreProfile returns genre tags derived from artists who have played approved
// shows at this venue. Returns the top 5 genres ranked by distinct artist count.
// Returns empty if the venue has fewer than 10 shows with tagged artists.
func (s *VenueService) GetVenueGenreProfile(venueID uint) ([]contracts.GenreCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// First check if venue has enough shows with tagged artists
	var showCount int64
	err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.show_id)
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE sv.venue_id = ? AND s.status = ?
	`, venueID, catalogm.ShowStatusApproved).Scan(&showCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count tagged shows for venue: %w", err)
	}

	if showCount < 10 {
		return []contracts.GenreCount{}, nil
	}

	type genreRow struct {
		TagID uint   `gorm:"column:tag_id"`
		Name  string `gorm:"column:name"`
		Slug  string `gorm:"column:slug"`
		Count int    `gorm:"column:count"`
	}

	var rows []genreRow
	err = s.db.Raw(`
		SELECT t.id AS tag_id, t.name, t.slug, COUNT(DISTINCT sa.artist_id) AS count
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE sv.venue_id = ? AND s.status = ?
		GROUP BY t.id, t.name, t.slug
		ORDER BY count DESC
		LIMIT 5
	`, venueID, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venue genre profile: %w", err)
	}

	result := make([]contracts.GenreCount, len(rows))
	for i, r := range rows {
		result[i] = contracts.GenreCount{
			TagID: r.TagID,
			Name:  r.Name,
			Slug:  r.Slug,
			Count: r.Count,
		}
	}

	return result, nil
}

// GetVenueModel retrieves a raw venue model (used by handlers to check ownership)
func (s *VenueService) GetVenueModel(venueID uint) (*catalogm.Venue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return &venue, nil
}

// contracts.UnverifiedVenueResponse represents an unverified venue for admin review

// GetUnverifiedVenues retrieves all unverified venues for admin review.
// Results are sorted by creation date (newest first).
func (s *VenueService) GetUnverifiedVenues(limit, offset int) ([]*contracts.UnverifiedVenueResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count of unverified venues
	var total int64
	if err := s.db.Model(&catalogm.Venue{}).Where("verified = ?", false).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count unverified venues: %w", err)
	}

	// Build query with show count subquery
	subquery := s.db.Table("show_venues").
		Select("venue_id, COUNT(*) as show_count").
		Group("venue_id")

	var results []struct {
		catalogm.Venue
		ShowCount int64 `gorm:"column:show_count"`
	}

	err := s.db.Table("venues").
		Select("venues.*, COALESCE(sc.show_count, 0) as show_count").
		Joins("LEFT JOIN (?) as sc ON venues.id = sc.venue_id", subquery).
		Where("venues.verified = ?", false).
		Order("venues.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get unverified venues: %w", err)
	}

	// Build responses
	responses := make([]*contracts.UnverifiedVenueResponse, len(results))
	for i, r := range results {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		responses[i] = &contracts.UnverifiedVenueResponse{
			ID:          r.ID,
			Slug:        slug,
			Name:        r.Name,
			Address:     r.Address,
			City:        r.City,
			State:       r.State,
			Zipcode:     r.Zipcode,
			SubmittedBy: r.SubmittedBy,
			CreatedAt:   r.CreatedAt,
			ShowCount:   int(r.ShowCount),
		}
	}

	return responses, total, nil
}
