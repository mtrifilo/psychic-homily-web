package shared

import (
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// This file is the ONE answer to "what does the system derive from an entity's
// (city, state, country), and how?" — the lower layer PSY-1747 was filed to
// create.
//
// Before it, the derivation existed as a copy per write path, each with its own
// deref-and-overlay helpers. Eleven write-path derivations now route through
// here — five that produce a venue's full column set
// (catalog.VenueService.applyGeocoding, catalog.VenueService.UpdateVenue's
// re-geocode branch, admin.applyDerivedVenueLocation, and the two data-sync
// import seams) and six that produce only a metro (catalog UpdateArtist /
// UpdateFestival / CreateFestival / FindOrCreateArtistTx,
// admin.applyDerivedEntityMetro, and catalog.metroDecision for the reconciler).
//
// Three more consumers take pieces without deriving: catalog.backfillVenuePass
// uses VenueLocation, admin.ApprovePendingEdit uses LocationTouched to gate its
// street-geocode clearing, and testhelpers.MetroFor uses DeriveMetro so fixtures
// resolve the way production does.
//
// The copies drifted independently, and the drift is not hypothetical: a venue's
// timezone shipped stale on the rollback path (PSY-1709) and an artist's metro
// shipped stale on the same path (PSY-1744), each because one copy learned
// something the others did not. Two consecutive tickets for one duplication is
// what bought this file.
//
// The split of responsibilities, which is what keeps the copies from coming back:
//
//   - Location and EffectiveLocation answer "what location is this write leaving
//     the entity in?" — the question every caller was answering with its own
//     deref-and-overlay helper.
//   - DeriveVenueLocation and DeriveMetro answer "what falls out of that
//     location?" — the geocoder calls plus the PSY-1707 timezone write-boundary.
//   - ApplyTo / ApplyToUpdates answer "how do I get it onto the row?", once for
//     model writes and once for untyped GORM update maps.
//
// Adding a fifth derived column means editing DerivedVenueLocation and its two
// Apply methods, all in this file. No call site changes.

// Location is the (city, state, country) tuple every derived location column is
// computed from.
//
// Blank, not nil, is how "not set" reaches the geocoder — geo.Resolve and
// geo.ResolveMetro take plain strings — so the constructors below do the
// unwrapping once rather than leaving each caller its own deref helper (the tree
// had four).
type Location struct {
	City    string
	State   string
	Country string
}

// NullableLocation builds a Location from nullable columns — the shape artists
// and festivals carry, where all three parts may be SQL NULL.
func NullableLocation(city, state, country *string) Location {
	return Location{
		City:    DerefOrEmpty(city),
		State:   DerefOrEmpty(state),
		Country: DerefOrEmpty(country),
	}
}

// VenueLocation reads the tuple off a venue model. Venue city/state are NOT NULL
// columns and country is nullable, which is why venues get their own constructor
// rather than reusing NullableLocation.
//
// It reads v.Country, and every venue write path now goes through it. That
// settles a question the copies used to disagree on: the two data-sync import
// seams passed a hardcoded "" because their ExportedVenue DTO carries no country
// field at all, so the constant was an artifact of the payload shape rather than
// a deliberate posture. Reading the column is behaviour-identical for a venue the
// importer builds (Country is never set, so it derefs to "") and is the correct
// answer the day the payload grows one. Pinned by TestVenueLocationReadsCountry.
func VenueLocation(v *catalogm.Venue) Location {
	return Location{
		City:    v.City,
		State:   v.State,
		Country: DerefOrEmpty(v.Country),
	}
}

// LocationTouched reports whether an untyped GORM updates map writes any
// component of the location the derived columns are computed from. One
// definition, so adding a fourth component means editing one line rather than
// finding every copy of the three-key loop.
func LocationTouched(updates map[string]interface{}) bool {
	for _, key := range []string{"city", "state", "country"} {
		if _, ok := updates[key]; ok {
			return true
		}
	}
	return false
}

// EffectiveLocation returns the location the entity will be in AFTER the update
// map is applied: the write's own value for each component it carries, the
// entity's current value for each it does not.
//
// Deriving from anything else is the exact bug this file exists to prevent — an
// entity left in its new city carrying what was resolved for the city it moved
// away from.
//
// A key present with an explicit nil is a write that CLEARS the column, so it
// resolves to blank, NOT to the current value. That case is real on the rollback
// path: artists and festivals have nullable location columns, so undoing
// "someone added a city" restores SQL NULL, and falling back there would derive
// from the very city the same write erases.
func EffectiveLocation(updates map[string]interface{}, current Location) Location {
	return Location{
		City:    effectiveComponent(updates, "city", current.City),
		State:   effectiveComponent(updates, "state", current.State),
		Country: effectiveComponent(updates, "country", current.Country),
	}
}

// effectiveComponent resolves one component of EffectiveLocation.
//
// It accepts every shape the callers stage location columns in, because they
// genuinely differ and unifying the call sites' staging would be a separate,
// riskier change: the admin paths build their maps from JSONB and stage a
// `string` or an untyped nil, catalog.UpdateArtist stages utils.NilIfEmpty's
// `*string` (possibly nil), and catalog.UpdateFestival stages a plain `string`.
// Understanding all of them is precisely what the per-package copies did not do.
//
// Anything else is a shape this function cannot interpret; deriving from the
// entity's current value beats deriving from a guess. That resolves a genuine
// disagreement between the two helpers this replaced in favour of the admin one:
// catalog's returned blank for an uninterpretable value. The branch is
// unreachable from every current caller, so nothing changes today.
func effectiveComponent(updates map[string]interface{}, key, fallback string) string {
	value, ok := updates[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case *string:
		return DerefOrEmpty(typed)
	default:
		return fallback
	}
}

// DerivedVenueLocation is the COMPLETE set of columns a venue derives from its
// Location. Complete is the point: a caller holding one of these has every
// derived column or none, so it cannot refresh three and leave the fourth stale.
type DerivedVenueLocation struct {
	Latitude  *float64
	Longitude *float64
	Timezone  *string
	Metro     *string
}

// DeriveVenueLocation resolves the four columns a venue derives from loc.
//
// A geocode MISS yields nil across all four rather than the previous values.
// That is deliberate and uniform: a venue that moved to a city the geocoder
// cannot place must not keep the coordinates and TIMEZONE of the city it left.
// The timezone is the severe one — every venue-local surface (the show-list
// partition, the ICS feed, reminder rendering) reads it as the venue's real zone
// and silently mis-dates shows instead of failing visibly. NULL is a shape every
// reader already handles; it falls back to the legacy state->zone map.
//
// db is the handle the venue is about to be WRITTEN through, not necessarily the
// service's own: it is what the PSY-1707 write-boundary invariant validates the
// resolved zone against, and validating on a second connection would put the
// guard outside the transaction carrying the write it guards (and hold a second
// pooled connection while the first is busy). Callers inside a transaction pass
// that tx; callers with none pass their service handle. A nil db skips
// validation entirely, which is what unit tests that use this as a pure function
// want.
//
// logCtx is appended to the timezone-rejection log line so an operator can find
// the affected row.
func DeriveVenueLocation(db *gorm.DB, g geo.Geocoder, loc Location, logCtx ...any) DerivedVenueLocation {
	lat, lng, tz := geo.LookupPointers(g, loc.City, loc.State, loc.Country)
	return DerivedVenueLocation{
		Latitude:  lat,
		Longitude: lng,
		// The PSY-1707 write-boundary invariant, held here for every venue write
		// path at once. Policy (degrade to NULL rather than fail the write) lives
		// on NormalizedGeocodedTimezoneOrNull.
		Timezone: NormalizedGeocodedTimezoneOrNull(db, tz, logCtx...),
		Metro:    DeriveMetro(g, loc),
	}
}

// ApplyTo sets the derived columns on a venue model — the shape the create and
// bulk-import paths write through.
func (d DerivedVenueLocation) ApplyTo(v *catalogm.Venue) {
	v.Latitude = d.Latitude
	v.Longitude = d.Longitude
	v.Timezone = d.Timezone
	v.Metro = d.Metro
}

// ApplyToUpdates writes the derived columns into an untyped GORM updates map —
// the shape the admin apply-an-edit paths write through, where no model is bound
// and therefore no GORM hook fires.
//
// All four keys are written unconditionally, including the nils: an omitted key
// leaves the stale value in place, which is the failure mode, and an explicit nil
// is how a miss reaches SQL NULL.
func (d DerivedVenueLocation) ApplyToUpdates(updates map[string]interface{}) {
	updates["latitude"] = d.Latitude
	updates["longitude"] = d.Longitude
	updates["timezone"] = d.Timezone
	updates["metro"] = d.Metro
}

// DeriveMetro resolves the US Census CBSA code a Location rolls up to (PSY-1255
// step B), nil on a miss or a non-US location. Metro is what keys an entity into
// an Atlas scene, so a stale one keeps the entity showing up in — and counting
// toward — a scene it has left.
//
// Venues get theirs through DeriveVenueLocation, alongside the geocoding it is a
// sibling of. Artists and festivals derive a metro and nothing else, so they call
// this directly.
//
// The column is on no editor's field list, so recomputing it can never clobber a
// value a human chose, and a resolution MISS writes NULL rather than leaving the
// old code — a stale metro is worse than an absent one.
//
// catalog.Reconcile{Artist,Venue,Festival}Metros — run MANUALLY via
// cmd/backfill-entity-metro, which is dry-run unless passed --confirm, not on a
// schedule — is the backstop for the enrichment passes that change a location
// without going through a write path. Those three functions are also the other
// place the set of metro-carrying entities is written down, so a fourth entity
// type has to be added there as well as here.
func DeriveMetro(g geo.Geocoder, loc Location) *string {
	return geo.MetroPointer(g, loc.City, loc.State, loc.Country)
}
