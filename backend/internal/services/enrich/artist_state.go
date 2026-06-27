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
	ArtistsScanned    int
	FilledGeo         int // unambiguous city -> state, offline
	FilledMusicBrainz int // ambiguous/unknown city confirmed via MusicBrainz
	Unresolved        int // attempted (geocoder declined; MusicBrainz couldn't confirm)
	Skipped           int // non-US country or blank city — never attempted
	Fills             []StateFill
	Errors            []string
}

// MBStateResolver is the MusicBrainz capability the ambiguous-city path needs:
// search candidates by name, walk an area to its parent Subdivision, and read a
// candidate's URL relations to confirm identity. All three are satisfied by
// *pipeline.MusicBrainzClient. Kept narrow (interface segregation) and separate
// from artist_location.go's searcher so each test fakes only what it uses; a nil
// resolver disables the MusicBrainz pass.
type MBStateResolver interface {
	SearchArtistCandidates(ctx context.Context, name string) ([]pipeline.MBArtistResult, error)
	LookupAreaRelations(ctx context.Context, areaID string) ([]pipeline.MBAreaRelation, error)
	LookupArtistURLRelations(ctx context.Context, mbid string) ([]pipeline.MBURLRelation, error)
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

		// Layer 1: the offline geocoder, which fills only an unambiguous, US-
		// dominant city. Ambiguous and internationally-dominant names fall through
		// to MusicBrainz; a known non-US place is left for the Skipped bucket.
		state, status := g.ResolveUSState(city)
		source := ""
		if status == geo.USStateUnambiguous {
			source = DataSourceGeoNames
		}

		// Layer 2: MusicBrainz, for any name the geocoder wouldn't fill (ambiguous
		// or unknown-but-US-plausible). It returns a state only when it can confirm
		// the band's identity, so an unconfirmable name stays NULL.
		if source == "" && useMB {
			var mbErr error
			state, source, mbErr = mbState(ctx, mb, a, city)
			if mbErr != nil {
				// A sustained MusicBrainz outage disables the pass for the rest of the
				// run so an outage doesn't make every remaining artist pay a doomed
				// ~1s-throttled call (mirrors the location backfill's breaker).
				mbConsecutiveErrors++
				report.Errors = append(report.Errors, fmt.Sprintf("musicbrainz %q: %v", a.Name, mbErr))
				if mbConsecutiveErrors >= mbErrorBreakerThreshold {
					useMB = false
					report.Errors = append(report.Errors, fmt.Sprintf(
						"musicbrainz disabled after %d consecutive errors; remaining artists left unresolved",
						mbConsecutiveErrors))
				}
				report.Unresolved++
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
			// Neither layer produced a state. It was attempted (the geocoder ran, and
			// MusicBrainz too unless --geocoder-only or the breaker tripped) — count
			// it Unresolved, not Skipped, so the report doesn't read it as non-US.
			report.Unresolved++
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

// mbState resolves a US state for a city the geocoder wouldn't fill, via
// MusicBrainz, WITHOUT ever guessing. It re-searches by name, keeps only
// candidates that independently name the SAME city, and resolves each to a state
// (the parent Subdivision — on the search result if MusicBrainz tagged one, else
// via one area-rels lookup). Then it decides:
//
//   - candidates disagree on the state, or none resolved → "" (genuinely
//     ambiguous; leave NULL).
//   - they unanimously agree AND there are ≥2 of them → that state. Two distinct
//     MusicBrainz records naming the same city in the same state make the STATE
//     correct regardless of which one is "our" band.
//   - exactly one candidate → a name + same-city coincidence is NOT proof it is
//     our band (a same-named band in a same-named city of another state looks
//     identical — the homonym that a bare name+city check misses), so require an
//     IDENTITY match: the candidate's url-rels must share one of the artist's own
//     Spotify/Bandcamp links. Confirmed → that state; otherwise "" (leave NULL).
//
// Returns a non-nil error only for a MusicBrainz transport failure, so the
// caller's circuit breaker can observe it.
func mbState(ctx context.Context, mb MBStateResolver, a *catalogm.Artist, storedCity string) (state, source string, err error) {
	want := pipeline.NormalizeArtistName(a.Name)
	if want == "" {
		return "", "", nil
	}
	candidates, err := mb.SearchArtistCandidates(ctx, a.Name)
	if err != nil {
		return "", "", err
	}

	type cityMatch struct {
		cand  pipeline.MBArtistResult
		state string
	}
	var matches []cityMatch
	for i := range candidates {
		c := candidates[i]
		if pipeline.NormalizeArtistName(c.Name) != want {
			continue
		}
		cityArea := mbCityArea(c)
		if cityArea == nil || !geo.SamePlaceName(cityArea.Name, storedCity) {
			continue // not the same city → can't trust this candidate's state
		}
		st, lookupErr := candidateState(ctx, mb, c, cityArea)
		if lookupErr != nil {
			return "", "", lookupErr
		}
		if st != "" {
			matches = append(matches, cityMatch{cand: c, state: st})
		}
	}
	if len(matches) == 0 {
		return "", "", nil
	}

	// Every surviving candidate must agree on one state, or we can't disambiguate.
	state = matches[0].state
	for _, m := range matches[1:] {
		if m.state != state {
			return "", "", nil // candidates disagree → leave NULL
		}
	}
	if len(matches) >= 2 {
		return state, DataSourceMusicBrainz, nil // consensus: state is right either way
	}

	// Exactly one candidate: confirm it is actually our artist before trusting it.
	confirmed, idErr := candidateIsArtist(ctx, mb, matches[0].cand, a)
	if idErr != nil {
		return "", "", idErr
	}
	if confirmed {
		return state, DataSourceMusicBrainz, nil
	}
	return "", "", nil
}

// candidateState resolves a name + city-matched candidate to its US state: the
// parent Subdivision from the search result if MusicBrainz tagged one, else via a
// single area-rels lookup on the city. Returns "" (no error) when no usable US
// state is found; an error only for a MusicBrainz transport failure.
func candidateState(ctx context.Context, mb MBStateResolver, c pipeline.MBArtistResult, cityArea *pipeline.MBArea) (string, error) {
	if loc, ok := locationFromMBResult(c); ok && loc.State != "" {
		return loc.State, nil
	}
	if cityArea.ID == "" {
		return "", nil
	}
	rels, err := mb.LookupAreaRelations(ctx, cityArea.ID)
	if err != nil {
		return "", err
	}
	if name, ok := parentSubdivisionName(rels); ok {
		if abbr, ok := utils.StateNameToAbbrev(name); ok {
			return abbr, nil
		}
	}
	return "", nil
}

// candidateIsArtist confirms a MusicBrainz candidate IS our artist via a shared
// streaming link: the candidate's url-rels include one of the artist's own
// Spotify/Bandcamp URLs (compared in canonical form). An artist with no such link
// to anchor on cannot be confirmed → false, so the caller leaves the state NULL
// rather than trust a bare name + city coincidence. Returns an error only for a
// MusicBrainz transport failure.
func candidateIsArtist(ctx context.Context, mb MBStateResolver, c pipeline.MBArtistResult, a *catalogm.Artist) (bool, error) {
	links := artistPlatformLinks(a)
	if len(links) == 0 || c.ID == "" {
		return false, nil
	}
	rels, err := mb.LookupArtistURLRelations(ctx, c.ID)
	if err != nil {
		return false, err
	}
	for _, rel := range rels {
		for _, link := range links {
			if pipeline.SamePlatformArtistURL(rel.URL.Resource, link) {
				return true, nil
			}
		}
	}
	return false, nil
}

// artistPlatformLinks returns the artist's Spotify and Bandcamp URLs — the links
// SamePlatformArtistURL can anchor identity on — skipping blanks.
func artistPlatformLinks(a *catalogm.Artist) []string {
	var links []string
	for _, p := range []*string{a.Social.Spotify, a.Social.Bandcamp} {
		if s := trimPtr(p); s != "" {
			links = append(links, s)
		}
	}
	return links
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
