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
// artists.source_confidence. Bandcamp is the band's own self-report (higher);
// the MusicBrainz begin-area is the editor-curated ORIGIN, which can differ from
// where the band is now (slightly lower).
const (
	confidenceBandcamp    = 0.7
	confidenceMusicBrainz = 0.6
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

// Options configures a backfill run.
type Options struct {
	DryRun       bool
	Verbose      bool
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
// Bandcamp first then MusicBrainz, filling only empty fields, dry-run by default.
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

	for i := range artists {
		a := &artists[i]

		loc, source, err := resolveLocation(ctx, a, bandcamp, mb, opts)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
			report.Missed++
			continue
		}
		if source == "" {
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

// resolveLocation tries Bandcamp first (the band's self-report), then falls back
// to MusicBrainz (unless BandcampOnly). Returns (loc, source, nil) on a hit,
// (zero, "", nil) on a clean miss, and (zero, "", err) on a hard source failure
// (e.g. a MusicBrainz rate-limit) so a persistent outage surfaces in the report
// instead of masquerading as "no data".
func resolveLocation(
	ctx context.Context,
	a *catalogm.Artist,
	bandcamp BandcampLocationResolver,
	mb MBCandidateSearcher,
	opts Options,
) (ResolvedLocation, string, error) {
	// Bandcamp primary — only artists whose social.bandcamp is set. Any bandcamp
	// URL works: the location element is in the band header on band/album pages.
	if bandcamp != nil && a.Social.Bandcamp != nil {
		if raw, ok := bandcamp.ResolveProfileLocation(ctx, *a.Social.Bandcamp); ok {
			if loc, ok := parseBandcampLocation(raw); ok {
				return loc, DataSourceBandcamp, nil
			}
		}
	}

	// MusicBrainz fallback — structured, matched by exact (case-insensitive) name.
	if !opts.BandcampOnly && mb != nil {
		candidates, err := mb.SearchArtistCandidates(ctx, a.Name)
		if err != nil {
			return ResolvedLocation{}, "", fmt.Errorf("musicbrainz %q: %w", a.Name, err)
		}
		if loc, ok := matchMBLocation(candidates, a.Name); ok {
			return loc, DataSourceMusicBrainz, nil
		}
	}

	return ResolvedLocation{}, "", nil
}

// matchMBLocation returns the location of the first candidate whose name matches
// the query exactly (case-insensitive). The exact-name gate mirrors the
// discovery flow (PSY-1191): a score/top-hit filter would pick a higher-scored
// famous namesake over the correct artist.
func matchMBLocation(candidates []pipeline.MBArtistResult, name string) (ResolvedLocation, bool) {
	want := strings.TrimSpace(name)
	for _, c := range candidates {
		if !strings.EqualFold(strings.TrimSpace(c.Name), want) {
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
// ("Tokyo, Japan"); some bands set only a single token.
//
// Rules (conservative — a wrong guess is harder to undo than an empty field):
//   - "City, Region": Region via utils.StateNameToAbbrev → State (US); else Country.
//   - "City, State, Country": middle as State (if it maps), last as Country.
//   - single token: too ambiguous to classify as city vs country → skip (ok=false);
//     the structured MusicBrainz fallback covers these.
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
	if len(cleaned) >= 3 {
		if abbr, ok := utils.StateNameToAbbrev(cleaned[len(cleaned)-2]); ok {
			loc.State = abbr
		}
		loc.Country = cleaned[len(cleaned)-1]
		return loc, true
	}
	// Exactly two parts: the region is a US state OR a country.
	region := cleaned[1]
	if abbr, ok := utils.StateNameToAbbrev(region); ok {
		loc.State = abbr
	} else {
		loc.Country = region
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
		switch a.Type {
		case "City":
			if loc.City == "" {
				loc.City = name
			}
		case "Subdivision":
			if loc.State == "" {
				if abbr, ok := utils.StateNameToAbbrev(name); ok {
					loc.State = abbr
				}
			}
		case "Country":
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
// data_source/source_confidence are set ONLY when the artist has no prior
// data_source — that column is row-level and may already attribute a different
// enrichment (e.g. spotify images), so we record provenance without clobbering
// it. last_verified_at is always bumped when we fill something.
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

	updates["last_verified_at"] = now
	if isEmptyPtr(a.DataSource) {
		updates["data_source"] = source
		updates["source_confidence"] = confidence
	}
	return updates, filled
}

// isEmptyPtr reports whether a string pointer is nil or points to a blank string.
func isEmptyPtr(s *string) bool {
	return s == nil || strings.TrimSpace(*s) == ""
}

// gormArtistStore is the production artistStore backed by GORM.
type gormArtistStore struct{ db *gorm.DB }

// ArtistsNeedingLocation loads artists with at least one empty location field,
// ordered by id for a stable run. limit <= 0 means all.
func (s *gormArtistStore) ArtistsNeedingLocation(limit int) ([]catalogm.Artist, error) {
	var artists []catalogm.Artist
	q := s.db.
		Where("city IS NULL OR city = '' OR state IS NULL OR state = '' OR country IS NULL OR country = ''").
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
