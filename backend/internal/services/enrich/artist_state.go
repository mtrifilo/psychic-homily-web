package enrich

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/utils"
)

// PSY-1255 (absorbs PSY-1244): derive a US state for artists that have a city
// but no state, so a MusicBrainz city-only artist (Tool -> "Los Angeles", "")
// becomes matchable by the scenes local-artist filter (PSY-1233), which keys on
// city AND state.
//
// The state comes from the SOURCE, never a population guess. The blocked first
// attempt resolved every bare city to its highest-population namesake, which
// silently wrote the wrong state for any multi-state name (Pasadena -> TX, not
// CA). This pass is layered so a bare name never decides a state on its own:
//
//  1. Offline, free: geo.ResolveUSState fills the state ONLY when the city name
//     maps to exactly one US state in the dataset (Chicago -> IL). A multi-state
//     namesake is reported ambiguous, not guessed.
//  2. MusicBrainz, for the ambiguous residual: re-search the artist, and trust a
//     name-matched candidate ONLY when it independently names the SAME city; then
//     the state is that city's parent Subdivision — taken from the search result
//     if MusicBrainz tagged one, else fetched with one area-rels lookup. The city
//     cross-check is what stops a same-named band in another state from writing
//     the wrong state.
//
// Anything the two layers can't confirm is left NULL (a review bucket) rather
// than guessed — a wrong state is harder to undo than an empty one, and it would
// plant a phantom "local" in the wrong scene.

// Confidence written to artists.source_confidence for a derived state. The
// geocoder value is an unambiguous dataset inference (safe, but not a source
// statement) so it sits just below Bandcamp's self-report; MusicBrainz matches
// the location enrichment's curated-origin confidence.
const (
	confidenceGeoState = 0.5
	// DataSourceGeoNames attributes a state derived from the offline geocoder.
	DataSourceGeoNames = "geonames"
)

// StateOptions configures a state-derivation run.
type StateOptions struct {
	DryRun       bool
	Limit        int  // 0 = all candidates
	GeocoderOnly bool // skip the MusicBrainz pass (ambiguous cities stay unresolved)
}

// StateFill records one artist's derived state for the report.
type StateFill struct {
	ArtistID uint
	Name     string
	City     string
	State    string // the derived 2-letter US state
	Source   string // DataSourceGeoNames or DataSourceMusicBrainz
}

// StateReport is the structured outcome of a run.
type StateReport struct {
	ArtistsScanned      int
	FilledGeo           int // unambiguous city -> state, offline
	FilledMusicBrainz   int // ambiguous city disambiguated via MusicBrainz
	AmbiguousUnresolved int // multi-state name MusicBrainz could not confirm
	Skipped             int // non-US country, blank city, or no US state derivable
	Fills               []StateFill
	Errors              []string
}

// MBStateResolver is the MusicBrainz capability the ambiguous-city path needs:
// search candidates by name, and walk an area to its parent relations. Both are
// satisfied by *pipeline.MusicBrainzClient. Kept narrow (interface segregation)
// and separate from artist_location.go's searcher so each test fakes only what
// it uses; a nil resolver disables the MusicBrainz pass.
type MBStateResolver interface {
	SearchArtistCandidates(ctx context.Context, name string) ([]pipeline.MBArtistResult, error)
	LookupAreaRelations(ctx context.Context, areaID string) ([]pipeline.MBAreaRelation, error)
}

// stateArtistStore is the narrow store the state pass needs. It is kept separate
// from artist_location.go's artistStore (interface segregation) so each test
// fakes only the methods it uses.
type stateArtistStore interface {
	ArtistsWithCityMissingState(limit int) ([]catalogm.Artist, error)
	UpdateArtistLocation(id uint, fields map[string]interface{}) error
}

// BackfillArtistStates fills the missing state of artists that already have a
// city, deriving it from the source (unambiguous geocoder, then MusicBrainz for
// ambiguous names), dry-run by default. It is the production entry point; the
// store-agnostic core is testable via backfillArtistStates with fakes.
func BackfillArtistStates(db *gorm.DB, g geo.Geocoder, mb MBStateResolver, opts StateOptions) (*StateReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if g == nil {
		return nil, fmt.Errorf("geocoder not initialized")
	}
	return backfillArtistStates(context.Background(), &gormArtistStore{db: db}, g, mb, opts)
}

func backfillArtistStates(
	ctx context.Context,
	store stateArtistStore,
	g geo.Geocoder,
	mb MBStateResolver,
	opts StateOptions,
) (*StateReport, error) {
	artists, err := store.ArtistsWithCityMissingState(opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("load artists: %w", err)
	}
	report := &StateReport{ArtistsScanned: len(artists)}
	now := time.Now()
	useMB := mb != nil && !opts.GeocoderOnly
	mbConsecutiveErrors := 0

	for i := range artists {
		a := &artists[i]
		city := trimPtr(a.City)
		country := trimPtr(a.Country)
		if city == "" {
			// The store gate excludes blank cities; a row that slips through has
			// nothing to resolve. Counted as non-US/unknown rather than dropped.
			report.Skipped++
			continue
		}

		// A known non-US country settles it: never write a US state for a band
		// whose own country is, say, Japan. An empty/unknown country falls through
		// to the geocoder, which only yields US states anyway.
		if iso, ok := geo.CountryToISO(country); ok && iso != "US" {
			report.Skipped++
			continue
		}

		state, source := geoState(g, city)
		if source == "" && useMB {
			// Ambiguous (or geocoder-unknown but US-plausible) name: ask the source.
			var mbErr error
			state, source, mbErr = mbState(ctx, mb, a.Name, city)
			if mbErr != nil {
				// A sustained MusicBrainz outage disables the pass for the rest of the
				// run so an outage doesn't make every remaining ambiguous artist pay a
				// doomed ~1s-throttled call (mirrors the location backfill's breaker).
				mbConsecutiveErrors++
				report.Errors = append(report.Errors, fmt.Sprintf("musicbrainz %q: %v", a.Name, mbErr))
				if mbConsecutiveErrors >= mbErrorBreakerThreshold {
					useMB = false
					report.Errors = append(report.Errors, fmt.Sprintf(
						"musicbrainz disabled after %d consecutive errors; remaining ambiguous artists left unresolved",
						mbConsecutiveErrors))
				}
				report.AmbiguousUnresolved++
				continue
			}
			mbConsecutiveErrors = 0
		}

		switch source {
		case DataSourceGeoNames:
			report.FilledGeo++
		case DataSourceMusicBrainz:
			report.FilledMusicBrainz++
		default:
			// No layer resolved a state. Distinguish an ambiguous name we tried (or
			// would have tried) on MusicBrainz from a city that simply isn't a US
			// place, so the report shows what a later pass could still recover.
			if _, status := g.ResolveUSState(city); status == geo.USStateAmbiguous {
				report.AmbiguousUnresolved++
			} else {
				report.Skipped++
			}
			continue
		}

		report.Fills = append(report.Fills, StateFill{
			ArtistID: a.ID, Name: a.Name, City: city, State: state, Source: source,
		})
		if opts.DryRun {
			continue
		}
		if err := store.UpdateArtistLocation(a.ID, stateUpdate(a, state, source, now)); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("artist %d update: %v", a.ID, err))
		}
	}
	return report, nil
}

// geoState returns the unambiguous US state for a city from the offline geocoder,
// with DataSourceGeoNames, or ("", "") when the name is ambiguous or not a known
// US place — leaving the decision to the MusicBrainz pass.
func geoState(g geo.Geocoder, city string) (state, source string) {
	if st, status := g.ResolveUSState(city); status == geo.USStateUnambiguous {
		return st, DataSourceGeoNames
	}
	return "", ""
}

// mbState disambiguates a multi-state city name via MusicBrainz. It trusts a
// name-matched candidate ONLY when that candidate independently names the SAME
// city; then the state is the city's parent Subdivision — already on the search
// result if MusicBrainz tagged one, otherwise fetched with a single area-rels
// lookup. Returns ("", "", nil) when nothing trustworthy is found, and a non-nil
// error only for a MusicBrainz transport failure (so the caller's breaker can see
// it). The city cross-check is the homonym guard: a same-named band in another
// state does not get to write its state onto this artist.
func mbState(ctx context.Context, mb MBStateResolver, artistName, storedCity string) (state, source string, err error) {
	candidates, err := mb.SearchArtistCandidates(ctx, artistName)
	if err != nil {
		return "", "", err
	}
	want := pipeline.NormalizeArtistName(artistName)
	if want == "" {
		return "", "", nil
	}
	for i := range candidates {
		c := candidates[i]
		if pipeline.NormalizeArtistName(c.Name) != want {
			continue
		}
		cityArea := mbCityArea(c)
		if cityArea == nil || !geo.SamePlaceName(cityArea.Name, storedCity) {
			continue // not the same city → can't trust this candidate's state
		}
		// The search result may already carry the parent Subdivision (MusicBrainz
		// tagged both city and state); use it without a second call.
		if loc, ok := locationFromMBResult(c); ok && loc.State != "" {
			return loc.State, DataSourceMusicBrainz, nil
		}
		// Otherwise walk the confirmed city up to its parent Subdivision.
		if cityArea.ID == "" {
			return "", "", nil
		}
		rels, lookupErr := mb.LookupAreaRelations(ctx, cityArea.ID)
		if lookupErr != nil {
			return "", "", lookupErr
		}
		if name, ok := parentSubdivisionName(rels); ok {
			if abbr, ok := utils.StateNameToAbbrev(name); ok {
				return abbr, DataSourceMusicBrainz, nil
			}
		}
		return "", "", nil // matched the city but no usable US parent state
	}
	return "", "", nil
}

// mbCityArea returns the first City-typed area on a MusicBrainz candidate
// (begin-area preferred — the origin city — then area), or nil. BeginArea is the
// specific origin, so it is checked first.
func mbCityArea(r pipeline.MBArtistResult) *pipeline.MBArea {
	for _, a := range []*pipeline.MBArea{r.BeginArea, r.Area} {
		if a != nil && strings.EqualFold(a.Type, "City") && strings.TrimSpace(a.Name) != "" {
			return a
		}
	}
	return nil
}

// parentSubdivisionName returns the name of the parent Subdivision (US state)
// among an area's relations. A city is linked to its state by a "part of"
// relation whose Area.Type is "Subdivision"; the Direction label is not relied on
// (it varies by how the edit was entered). A County intermediate is ignored — we
// want the Subdivision specifically.
func parentSubdivisionName(rels []pipeline.MBAreaRelation) (string, bool) {
	for _, r := range rels {
		if r.Area == nil {
			continue
		}
		if strings.EqualFold(r.Area.Type, "Subdivision") {
			if name := strings.TrimSpace(r.Area.Name); name != "" {
				return name, true
			}
		}
	}
	return "", false
}

// stateUpdate builds the GORM update map to write a derived state. It mirrors
// buildArtistLocationUpdate's provenance rule: the (data_source, source_
// confidence, last_verified_at) triple is written together and ONLY when the
// row has no prior data_source — that column is row-level and may already
// attribute another enrichment (the city fill, an image), so we record coherent
// provenance for rows we "own" and leave another source's untouched. The state
// itself is written regardless.
func stateUpdate(a *catalogm.Artist, state, source string, now time.Time) map[string]interface{} {
	updates := map[string]interface{}{"state": state}
	if isEmptyPtr(a.DataSource) {
		confidence := confidenceMusicBrainz
		if source == DataSourceGeoNames {
			confidence = confidenceGeoState
		}
		updates["data_source"] = source
		updates["source_confidence"] = confidence
		updates["last_verified_at"] = now
	}
	return updates
}

func trimPtr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// ArtistsWithCityMissingState loads artists that have a non-blank city but a
// NULL/blank state — the candidates whose state can be derived. TRIM mirrors the
// in-memory blank check so the SQL gate and the fill logic agree on "empty".
// Ordered by id for a stable run; limit <= 0 means all.
func (s *gormArtistStore) ArtistsWithCityMissingState(limit int) ([]catalogm.Artist, error) {
	var artists []catalogm.Artist
	q := s.db.
		Where("city IS NOT NULL AND TRIM(city) <> ''").
		Where("state IS NULL OR TRIM(state) = ''").
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return nil, err
	}
	return artists, nil
}
