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

// Report is the structured outcome of a backfill run.
type Report struct {
	ArtistsScanned    int
	FilledBandcamp    int
	FilledMusicBrainz int
	Missed            int // no source yielded a usable location
	ResolvedNoFill    int // a location was found but every matching field was already set
	Fills             []Fill
	Errors            []string
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
		loc, source, mbErr := resolveLocation(ctx, a, bandcamp, mb, useMB)

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
		if len(filled) == 0 {
			// Found a location, but every field it could supply was already set.
			report.ResolvedNoFill++
			continue
		}

		report.Fills = append(report.Fills, Fill{
			ArtistID: a.ID, Name: a.Name, Source: source, Fields: filled, Location: loc,
		})
		if source == DataSourceBandcamp {
			report.FilledBandcamp++
		} else {
			report.FilledMusicBrainz++
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

// resolveLocation tries MusicBrainz first (the curated origin — preferred for
// factual accuracy after the stage dry-run), then falls back to Bandcamp's
// self-report. useMB gates the MusicBrainz attempt (false under --bandcamp-only
// or once the run's circuit breaker has tripped).
//
// Returns (loc, source, mbErr): source != "" means resolved — possibly via
// Bandcamp EVEN WHEN MusicBrainz errored, because a transient MB failure must not
// suppress the fallback. mbErr is the MusicBrainz error (if any), returned
// independently of recovery so the caller's circuit breaker can observe it; the
// caller only records it as a run error when the artist resolved nothing.
func resolveLocation(
	ctx context.Context,
	a *catalogm.Artist,
	bandcamp BandcampLocationResolver,
	mb MBCandidateSearcher,
	useMB bool,
) (ResolvedLocation, string, error) {
	var mbErr error

	// MusicBrainz primary — structured, matched by the discovery exact-name gate.
	if useMB && mb != nil {
		candidates, err := mb.SearchArtistCandidates(ctx, a.Name)
		if err != nil {
			mbErr = fmt.Errorf("musicbrainz %q: %w", a.Name, err)
		} else if loc, ok := matchMBLocation(candidates, a.Name); ok {
			return loc, DataSourceMusicBrainz, nil
		}
	}

	// Bandcamp fallback — only artists whose social.bandcamp is set. Any bandcamp
	// URL works: the location element is in the band header on band/album pages.
	// This also RECOVERS an artist whose MusicBrainz lookup errored above.
	if bandcamp != nil && a.Social.Bandcamp != nil {
		if raw, ok := bandcamp.ResolveProfileLocation(ctx, *a.Social.Bandcamp); ok {
			if loc, ok := parseBandcampLocation(raw); ok {
				return loc, DataSourceBandcamp, mbErr
			}
		}
	}

	return ResolvedLocation{}, "", mbErr
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
	if abbr, ok := utils.StateNameToAbbrev(last); ok {
		// Trailing token is a US state ("…, Arizona" / "…, County, New York").
		loc.State = abbr
	} else {
		// Trailing token is a country ("…, Japan" / "…, State, USA").
		loc.Country = last
		if len(cleaned) >= 3 {
			if abbr, ok := utils.StateNameToAbbrev(cleaned[len(cleaned)-2]); ok {
				loc.State = abbr
			}
		}
	}
	return loc, true
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
			loc.Country = c // ISO-2, e.g. "US"
		}
	}
	if loc.isZero() {
		return ResolvedLocation{}, false
	}
	return loc, true
}

// buildArtistLocationUpdate computes the GORM update map to fill an artist's
// EMPTY location fields from a resolved location, plus provenance. Fill-when-
// empty: a field already set is never overwritten. Returns the update map and
// the filled field names; an empty filled list means nothing to write.
//
// The provenance triple (data_source, source_confidence, last_verified_at) is
// written together or not at all, ONLY when the artist has no prior data_source.
// That column is row-level and may already attribute a different enrichment (e.g.
// spotify images); bumping last_verified_at alone would make the triple describe
// a source it no longer matches. So we record coherent provenance for rows we
// "own" and leave another enrichment's provenance untouched (the location fields
// still fill regardless).
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
	if len(filled) == 0 {
		return nil, nil
	}

	if isEmptyPtr(a.DataSource) {
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
