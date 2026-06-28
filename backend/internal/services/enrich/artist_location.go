// Package enrich harvests artist metadata from external sources we already
// fetch (the Bandcamp profile page and the MusicBrainz search response) and
// fills it onto artists fill-when-empty. PSY-1234: artist location enrichment.
//
// The orchestrator depends only on small interfaces (an artist store, a Bandcamp
// location resolver, a MusicBrainz candidate searcher), so its decision logic is
// unit-testable with fakes — no database or network. The production wiring
// (gormArtistStore + *catalog.BandcampProfileResolver + *pipeline.MusicBrainzClient)
// lives behind those interfaces.
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

// DataSource values stamped on artists.data_source when a location is enriched.
const (
	DataSourceBandcamp    = "bandcamp"
	DataSourceMusicBrainz = "musicbrainz"
)

// Confidence in an auto-derived location, by source, written to
// artists.source_confidence. MusicBrainz is the editor-curated ORIGIN and the
// preferred source — a stage dry-run showed it factually reliable (Bad Religion
// → Los Angeles, Social Distortion → Fullerton). Bandcamp is the band's own
// self-report and the fallback: useful, but sometimes a label/current base
// rather than origin (Tool's page says Seattle), so slightly lower.
const (
	confidenceMusicBrainz = 0.7
	confidenceBandcamp    = 0.6
)

// ResolvedLocation is a normalized artist location harvested from one source.
// Any field may be empty when the source didn't supply it (an international band
// has no US state). State is a two-letter US/DC abbreviation; Country is a
// country name or ISO code as the source provided it (Bandcamp gives a name like
// "Japan"; MusicBrainz gives an ISO-2 like "JP").
type ResolvedLocation struct {
	City    string
	State   string
	Country string
	// MBID is the MusicBrainz artist MBID of the exact-name match this location came
	// from (PSY-1249); empty for a Bandcamp-sourced location or when no MB match
	// resolved. Stamped onto artists.musicbrainz_artist_id fill-when-empty so later
	// passes browse MusicBrainz by ID instead of re-searching by name. Deliberately
	// NOT part of isZero — an MBID alone is not a usable LOCATION.
	MBID string
}

func (l ResolvedLocation) isZero() bool {
	return l.City == "" && l.State == "" && l.Country == ""
}

// mbErrorBreakerThreshold is how many CONSECUTIVE MusicBrainz errors trip the
// circuit breaker: after this many in a row, MusicBrainz is disabled for the
// rest of the run (remaining artists use Bandcamp only). Without it a sustained
// MB rate-limit / outage would make every remaining artist pay a doomed
// ~1s-throttled call, dragging the run on for minutes and prolonging the IP ban.
const mbErrorBreakerThreshold = 5

// Options configures a backfill run.
type Options struct {
	DryRun       bool
	Limit        int  // 0 = all artists needing location
	BandcampOnly bool // skip the MusicBrainz fallback
}

// Fill records one artist's enrichment outcome for the report.
type Fill struct {
	ArtistID uint
	Name     string
	Source   string
	Fields   []string // which of city/state/country were filled
	Location ResolvedLocation
}

// Conflict records an artist whose two sources both resolved a location but
// disagreed on COUNTRY — a likely homonym (MusicBrainz name-matched a different
// band). We skip it rather than guess and surface it for human review.
//
// Country is a deliberately coarse signal, with two known edges left for the
// reviewer: (1) a SAME-country homonym (two distinct US bands of one name) is NOT
// caught — comparing city/state instead would wrongly skip legitimate
// origin-vs-current-base differences (Tool LA vs Seattle), so the dry-run review
// remains the backstop there; (2) a genuinely RELOCATED band (MB origin country ≠
// its Bandcamp current country) also trips this and is skipped — surfaced so a
// human can fill it, rather than the tool guessing origin vs current.
type Conflict struct {
	ArtistID uint
	Name     string
	MB       ResolvedLocation
	Bandcamp ResolvedLocation
}

// Report is the structured outcome of a backfill run.
type Report struct {
	ArtistsScanned    int
	FilledBandcamp    int
	FilledMusicBrainz int
	Missed            int // no source yielded a usable location
	ResolvedNoFill    int // a location was found but every matching field was already set
	// StampedMBID counts artists that got a musicbrainz_artist_id written this run
	// (PSY-1249) — across BOTH location-fill rows and MBID-only rows. Surfaced in the
	// dry-run summary so the operator's mandatory review actually sees the MBID writes
	// (an MBID-only row otherwise lands silently in ResolvedNoFill).
	StampedMBID int
	Fills       []Fill
	Conflicts   []Conflict // sources disagreed on country — skipped for review
	Errors      []string
}

// BandcampLocationResolver fetches a band's self-reported location from a
// Bandcamp profile root. Implemented by *catalog.BandcampProfileResolver.
type BandcampLocationResolver interface {
	ResolveProfileLocation(ctx context.Context, profileURL string) (string, bool)
}

// MBCandidateSearcher returns MusicBrainz artist search candidates for a name.
// Implemented by *pipeline.MusicBrainzClient.
type MBCandidateSearcher interface {
	SearchArtistCandidates(ctx context.Context, name string) ([]pipeline.MBArtistResult, error)
}

// artistStore abstracts artist load/update so the orchestrator is unit-testable
// without a database. gormArtistStore is the production implementation.
type artistStore interface {
	ArtistsNeedingLocation(limit int) ([]catalogm.Artist, error)
	UpdateArtistLocation(id uint, fields map[string]interface{}) error
}

// BackfillArtistLocations enriches artists whose location is incomplete, trying
// MusicBrainz first then Bandcamp, filling only empty fields, dry-run by default.
// It is the production entry point; the store-agnostic core is testable via
// backfillArtistLocations.
func BackfillArtistLocations(db *gorm.DB, bandcamp BandcampLocationResolver, mb MBCandidateSearcher, opts Options) (*Report, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return backfillArtistLocations(context.Background(), &gormArtistStore{db: db}, bandcamp, mb, opts)
}

// backfillArtistLocations is the store-agnostic core. now is stamped on every
// fill as last_verified_at.
func backfillArtistLocations(
	ctx context.Context,
	store artistStore,
	bandcamp BandcampLocationResolver,
	mb MBCandidateSearcher,
	opts Options,
) (*Report, error) {
	artists, err := store.ArtistsNeedingLocation(opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("load artists: %w", err)
	}

	report := &Report{ArtistsScanned: len(artists)}
	now := time.Now()
	mbConsecutiveErrors := 0
	mbDisabled := false

	for i := range artists {
		a := &artists[i]

		useMB := !opts.BandcampOnly && !mbDisabled
		loc, source, conflict, mbErr := resolveLocation(ctx, a, bandcamp, mb, useMB)

		// Circuit breaker: after a sustained run of MusicBrainz errors, disable it
		// for the rest of the run so an outage doesn't make every remaining artist
		// pay a doomed ~1s-throttled call. A clean MB response (hit OR miss) resets
		// the streak.
		if useMB {
			if mbErr != nil {
				mbConsecutiveErrors++
				if mbConsecutiveErrors >= mbErrorBreakerThreshold && !mbDisabled {
					mbDisabled = true
					report.Errors = append(report.Errors, fmt.Sprintf(
						"musicbrainz disabled after %d consecutive errors; remaining artists use Bandcamp only (last: %v)",
						mbConsecutiveErrors, mbErr))
				}
			} else {
				mbConsecutiveErrors = 0
			}
		}

		// Sources disagreed on country (likely a homonym MB match) — skip rather
		// than write a probably-wrong location; surface it for review.
		if conflict != nil {
			report.Conflicts = append(report.Conflicts, *conflict)
			continue
		}

		if source == "" {
			// Resolved nothing. Surface a genuine MB error (an outage) but not a
			// clean miss. An artist recovered via Bandcamp despite an MB error is
			// NOT counted here — its mbErr only fed the breaker above.
			if mbErr != nil {
				report.Errors = append(report.Errors, mbErr.Error())
			}
			report.Missed++
			continue
		}

		confidence := confidenceMusicBrainz
		if source == DataSourceBandcamp {
			confidence = confidenceBandcamp
		}
		updates, filled := buildArtistLocationUpdate(a, loc, source, confidence, now)
		if updates == nil {
			// Found a location, but every field it could supply — and the MBID —
			// was already set.
			report.ResolvedNoFill++
			continue
		}

		if _, stamped := updates["musicbrainz_artist_id"]; stamped {
			report.StampedMBID++
		}

		if len(filled) > 0 {
			report.Fills = append(report.Fills, Fill{
				ArtistID: a.ID, Name: a.Name, Source: source, Fields: filled, Location: loc,
			})
			if source == DataSourceBandcamp {
				report.FilledBandcamp++
			} else {
				report.FilledMusicBrainz++
			}
		} else {
			// Only the resolved MBID was stamped — no new location field. Not a
			// location fill, but we still persist it (below) so the MBID isn't
			// re-searched next pass.
			report.ResolvedNoFill++
		}

		if opts.DryRun {
			continue
		}
		if err := store.UpdateArtistLocation(a.ID, updates); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("artist %d update: %v", a.ID, err))
		}
	}

	return report, nil
}

// resolveLocation gathers BOTH sources (when available) so it can detect a
// homonym: MusicBrainz (curated origin, the preferred fill) and Bandcamp (the
// band's identity-anchored self-report). useMB gates the MusicBrainz attempt
// (false under --bandcamp-only or once the run's circuit breaker has tripped).
//
// Decision:
//   - both resolve & their COUNTRIES disagree → return a *Conflict (skip): a
//     different-country match is the namesake red flag (e.g. our Phoenix
//     "Yellowcake" vs an Italian one). Same-country differences (origin vs base,
//     e.g. Tool LA vs its page's Seattle) are NOT a conflict — MusicBrainz wins.
//   - both resolve & agree, or MusicBrainz only → MusicBrainz.
//   - Bandcamp only → Bandcamp (also RECOVERS an artist whose MB lookup errored).
//
// Returns (loc, source, conflict, mbErr). mbErr is the MusicBrainz error (if
// any), returned independently of recovery so the caller's circuit breaker can
// observe it; the caller records it only when the artist resolved nothing.
func resolveLocation(
	ctx context.Context,
	a *catalogm.Artist,
	bandcamp BandcampLocationResolver,
	mb MBCandidateSearcher,
	useMB bool,
) (ResolvedLocation, string, *Conflict, error) {
	var mbErr error
	var mbLoc, bcLoc ResolvedLocation
	var mbOK, bcOK bool

	if useMB && mb != nil {
		candidates, err := mb.SearchArtistCandidates(ctx, a.Name)
		if err != nil {
			mbErr = fmt.Errorf("musicbrainz %q: %w", a.Name, err)
		} else {
			mbLoc, mbOK = matchMBLocation(candidates, a.Name)
		}
	}

	// Consult Bandcamp as the fallback (MB didn't resolve) OR for conflict
	// detection (MB resolved WITH a comparable country). If MB resolved without an
	// effective country, Bandcamp can't change the outcome — skip the HTTP fetch.
	// Only artists whose social.bandcamp is set; any bandcamp URL works (the
	// location is in the band header on band/album pages).
	needBandcamp := !mbOK
	if mbOK {
		if _, ok := effectiveCountryISO(mbLoc); ok {
			needBandcamp = true
		}
	}
	if needBandcamp && bandcamp != nil && a.Social.Bandcamp != nil {
		if raw, ok := bandcamp.ResolveProfileLocation(ctx, *a.Social.Bandcamp); ok {
			bcLoc, bcOK = parseBandcampLocation(raw)
		}
	}

	switch {
	case mbOK && bcOK:
		if countriesConflict(mbLoc, bcLoc) {
			return ResolvedLocation{}, "", &Conflict{
				ArtistID: a.ID, Name: a.Name, MB: mbLoc, Bandcamp: bcLoc,
			}, mbErr
		}
		return mbLoc, DataSourceMusicBrainz, nil, mbErr // agree → MusicBrainz wins
	case mbOK:
		return mbLoc, DataSourceMusicBrainz, nil, mbErr
	case bcOK:
		return bcLoc, DataSourceBandcamp, nil, mbErr
	default:
		return ResolvedLocation{}, "", nil, mbErr
	}
}

// countriesConflict reports whether two resolved locations name DIFFERENT
// countries. Each location's "effective country" is its explicit country if set,
// else "US" when it carries a US state (our State field only ever holds a US/DC
// abbreviation, so a state implies the US). A conflict is flagged only when BOTH
// effective countries are known and differ — an unknown one can't confirm a
// disagreement (conservative: don't skip a fillable artist on an ambiguous
// signal). This catches the homonym case (Phoenix "Yellowcake" carrying a US
// state vs an Italian MusicBrainz match) WITHOUT tripping on same-country
// origin-vs-base differences (Tool LA vs its own page's Seattle).
func countriesConflict(a, b ResolvedLocation) bool {
	isoA, okA := effectiveCountryISO(a)
	isoB, okB := effectiveCountryISO(b)
	if !okA || !okB {
		return false
	}
	return isoA != isoB
}

func effectiveCountryISO(loc ResolvedLocation) (string, bool) {
	if loc.Country != "" {
		return geo.CountryToISO(loc.Country)
	}
	if loc.State != "" {
		return "US", true // State holds only US/DC abbrevs (utils.StateNameToAbbrev)
	}
	return "", false
}

// matchMBLocation returns the location of the first candidate whose name matches
// the query under the SAME exact-name gate the discovery flow uses
// (pipeline.NormalizeArtistName, PSY-1197) — not a looser EqualFold — so the two
// identity checks can't drift (it folds punctuation/"&", catching e.g.
// "Godspeed You! Black Emperor"). A score/top-hit filter is deliberately avoided:
// it would pick a higher-scored famous namesake over the correct artist.
//
// NOTE: two genuinely different bands CAN share an exact name; this auto-writes
// the first match's location, so a homonym can be mis-attributed. The mandatory
// dry-run review is the backstop; PSY-1236 adds a source-disagreement guard.
func matchMBLocation(candidates []pipeline.MBArtistResult, name string) (ResolvedLocation, bool) {
	want := pipeline.NormalizeArtistName(name)
	if want == "" {
		return ResolvedLocation{}, false
	}
	for _, c := range candidates {
		if pipeline.NormalizeArtistName(c.Name) != want {
			continue
		}
		if loc, ok := locationFromMBResult(c); ok {
			// PSY-1249: carry the matched MB artist's MBID through to the write, but
			// only if it's a canonical UUID — never let a malformed id into the
			// identity column (the location still fills regardless).
			if pipeline.IsValidMBID(c.ID) {
				loc.MBID = c.ID
			}
			return loc, true
		}
	}
	return ResolvedLocation{}, false
}

// parseBandcampLocation splits a Bandcamp "location secondaryText" string into a
// ResolvedLocation. Bandcamp renders the band's home as "City, Region" where
// Region is a full US state name ("Phoenix, Arizona") or a country
// ("Tokyo, Japan"); occasionally "City, State, Country" or "City, County, State".
//
// Classification keys on the TRAILING (most-specific) token, then the one before
// it — never blindly on position (conservative; a wrong guess is harder to undo
// than an empty field):
//   - last token is a US state name/abbrev → State; else → Country.
//   - if last was a Country and a preceding token is a US state → also set State.
//   - single token: too ambiguous (city vs country) → skip; MusicBrainz covers it.
//
// KNOWN LIMITATION (PSY-1236): a region that is BOTH a US state name and a
// country — "Georgia" — is read as the US state. The MusicBrainz primary already
// covers most such artists correctly; the disambiguation (geocoder-validated)
// lands with the source-conflict guard.
func parseBandcampLocation(raw string) (ResolvedLocation, bool) {
	parts := strings.Split(raw, ",")
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) < 2 {
		return ResolvedLocation{}, false
	}

	loc := ResolvedLocation{City: cleaned[0]}
	last := cleaned[len(cleaned)-1]
	if abbr, ok := utils.StateNameToAbbrev(last); ok && !isCountryNotState(loc.City, last) {
		// Trailing token is a US state ("…, Arizona" / "…, County, New York").
		loc.State = abbr
	} else {
		// Trailing token is a country ("…, Japan" / "…, State, USA" / "…, Georgia"
		// the country).
		loc.Country = last
		if len(cleaned) >= 3 {
			if abbr, ok := utils.StateNameToAbbrev(cleaned[len(cleaned)-2]); ok {
				loc.State = abbr
			}
		}
	}
	return canonicalizeCountry(loc), true
}

// isCountryNotState resolves the "Georgia problem": a trailing token that maps to
// a US state abbreviation but is ALSO a country NAME. It returns true (→ treat the
// token as the country, not the state) ONLY when both hold:
//
//   - the token is a full NAME, not a 2-letter code. EVERY US state abbreviation
//     collides with some ISO country code (GA=Gabon, CA=Canada, AL=Albania,
//     LA=Laos…), so a bare 2-letter token is always the state in this catalog.
//   - the city POSITIVELY resolves inside that country. Positive evidence is
//     required because the offline cities dataset omits small US towns: keying on
//     "absent from US-GA" alone would exile a "Dahlonega, Georgia" band (a real
//     ~6k-pop US-GA town) to the Caucasus. "Tbilisi, Georgia" resolves in GE → the
//     country; "Dahlonega, Georgia" resolves in neither → stays the US state.
//
// The fast path (token isn't a country name) skips the geocoder entirely, so the
// common "City, Arizona" case costs nothing.
func isCountryNotState(city, token string) bool {
	if len(strings.TrimSpace(token)) <= 2 {
		return false
	}
	iso, ok := geo.CountryToISO(token)
	if !ok {
		return false
	}
	_, inCountry := geo.Default().Resolve(city, "", iso)
	return inCountry
}

// canonicalizeCountry rewrites a recognized country to its canonical display name
// (so MusicBrainz "US" and Bandcamp "USA" store identically as "United States");
// an unrecognized value is left untouched.
func canonicalizeCountry(loc ResolvedLocation) ResolvedLocation {
	if loc.Country != "" {
		if name, ok := geo.CanonicalCountryName(loc.Country); ok {
			loc.Country = name
		}
	}
	return loc
}

// locationFromMBResult extracts a ResolvedLocation from a MusicBrainz artist
// search result. MB tags areas by Type — "City" (origin city), "Subdivision"
// (US state, full name → utils.StateNameToAbbrev), "Country" — and carries a
// separate ISO-2 Country code. BeginArea is the specific origin (preferred for
// city); Area is the broader area. Returns ok=false when nothing usable is found.
func locationFromMBResult(r pipeline.MBArtistResult) (ResolvedLocation, bool) {
	var loc ResolvedLocation
	for _, a := range []*pipeline.MBArea{r.BeginArea, r.Area} {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		// Match Type case-insensitively, mirroring the discovery area helpers —
		// MusicBrainz's documented casing is "City"/"Subdivision"/"Country" but a
		// case-sensitive switch would silently drop a future casing change.
		switch strings.ToLower(a.Type) {
		case "city":
			if loc.City == "" {
				loc.City = name
			}
		case "subdivision":
			if loc.State == "" {
				if abbr, ok := utils.StateNameToAbbrev(name); ok {
					loc.State = abbr
				}
			}
		case "country":
			if loc.Country == "" {
				loc.Country = name
			}
		}
	}
	if loc.Country == "" {
		if c := strings.TrimSpace(r.Country); c != "" {
			loc.Country = c // ISO-2, e.g. "US" — canonicalized below
		}
	}
	loc = canonicalizeCountry(loc) // "US" / "United States" → one stable form
	if loc.isZero() {
		return ResolvedLocation{}, false
	}
	return loc, true
}

// buildArtistLocationUpdate computes the GORM update map to fill an artist's
// EMPTY location fields from a resolved location, plus provenance and (PSY-1249)
// the MusicBrainz MBID. Fill-when-empty: a field already set is never overwritten.
// Returns the update map and the filled LOCATION field names; nil updates means
// nothing to write. NOTE the asymmetry: `filled` lists only location fields, so it
// can be empty while updates is non-nil — when the only thing to write is an
// MBID-only stamp (a location resolved whose fields were all already set, or whose
// only new contribution is the identity). The caller keys "did we write?" on
// `updates != nil`, and "was it a location fill?" on `len(filled) > 0`.
//
// The provenance triple (data_source, source_confidence, last_verified_at) is
// written together or not at all, ONLY when a LOCATION field filled AND the artist
// has no prior data_source. That column is row-level and may already attribute a
// different enrichment (e.g. spotify images); bumping last_verified_at alone would
// make the triple describe a source it no longer matches. So we record coherent
// provenance for rows we "own" and leave another enrichment's provenance untouched
// (the location fields still fill regardless). An MBID-only stamp claims NO
// provenance — it doesn't change where the location came from.
func buildArtistLocationUpdate(
	a *catalogm.Artist,
	loc ResolvedLocation,
	source string,
	confidence float64,
	now time.Time,
) (map[string]interface{}, []string) {
	updates := map[string]interface{}{}
	var filled []string

	if isEmptyPtr(a.City) && loc.City != "" {
		updates["city"] = loc.City
		filled = append(filled, "city")
	}
	if isEmptyPtr(a.State) && loc.State != "" {
		updates["state"] = loc.State
		filled = append(filled, "state")
	}
	if isEmptyPtr(a.Country) && loc.Country != "" {
		updates["country"] = loc.Country
		filled = append(filled, "country")
	}

	// PSY-1249: stamp the resolved MusicBrainz MBID fill-when-empty, independent of
	// whether a location field filled — so a later pass needn't re-search even when
	// the location was already complete. loc.MBID is set only for an exact-name MB
	// match (matchMBLocation); it is empty for a Bandcamp-sourced location.
	mbidStamped := false
	if loc.MBID != "" && isEmptyPtr(a.MusicBrainzArtistID) {
		updates["musicbrainz_artist_id"] = loc.MBID
		mbidStamped = true
	}

	if len(filled) == 0 && !mbidStamped {
		return nil, nil
	}

	if len(filled) > 0 && isEmptyPtr(a.DataSource) {
		updates["data_source"] = source
		updates["source_confidence"] = confidence
		updates["last_verified_at"] = now
	}
	return updates, filled
}

// isEmptyPtr reports whether a string pointer is nil or points to a blank string.
func isEmptyPtr(s *string) bool {
	return s == nil || strings.TrimSpace(*s) == ""
}

// gormArtistStore is the production artistStore backed by GORM.
type gormArtistStore struct{ db *gorm.DB }

// ArtistsNeedingLocation loads artists missing a CITY — the universal location
// field both sources supply and the one the UI shows. Gating on city (not on
// "any of city/state/country empty") lets the run CONVERGE: an international band
// legitimately has no US state, and a US band filled "City, State" from Bandcamp
// has no country, so a multi-field gate would re-select — and re-fetch — those
// rows on every run forever. TRIM matches the in-memory isEmptyPtr check so the
// SQL gate and the fill logic agree on "empty". Ordered by id for a stable run;
// limit <= 0 means all. (State/country still fill opportunistically when we fill
// a city — they're just not what KEEPS an artist in the candidate set.)
func (s *gormArtistStore) ArtistsNeedingLocation(limit int) ([]catalogm.Artist, error) {
	var artists []catalogm.Artist
	q := s.db.
		Where("city IS NULL OR TRIM(city) = ''").
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return nil, err
	}
	return artists, nil
}

// UpdateArtistLocation applies a fields map to one artist row.
func (s *gormArtistStore) UpdateArtistLocation(id uint, fields map[string]interface{}) error {
	return s.db.Model(&catalogm.Artist{}).Where("id = ?", id).Updates(fields).Error
}
