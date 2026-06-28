package enrich

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/utils"
)

// PSY-1255 cleanup: re-check artists that ALREADY have a state and correct a
// wrong one. The blocked PSY-1244 pass wrote the highest-population US namesake's
// state for every city-only artist; for a multi-state name that is right for the
// dominant city (Austin→TX) but wrong for a smaller one (Pasadena→TX, not CA).
// Those two are indistinguishable offline — the only signal that separates them
// is the artist's OWN MusicBrainz record.
//
// So this pass re-derives the state through the same identity-confirmed path as
// the fill pass (mbState: a candidate is trusted only when its url-rels share the
// artist's Spotify/Bandcamp link) and OVERWRITES the stored state ONLY when
// MusicBrainz identity-confirms a DIFFERENT one. A correct guess (MusicBrainz
// agrees), an unconfirmable artist (no link / not in MusicBrainz), and a
// geocoder-unambiguous city are all left untouched — the pass never NULLs and
// never guesses, so it cannot destroy a correct state.

// VerifyOptions configures a verify-and-correct run.
type VerifyOptions struct {
	DryRun bool
	Limit  int // 0 = all state-set artists
}

// StateCorrection records one artist whose stored state disagreed with its
// identity-confirmed MusicBrainz origin.
type StateCorrection struct {
	ArtistID uint
	Name     string
	City     string
	OldState string
	NewState string
}

// VerifyReport is the structured outcome of a run.
type VerifyReport struct {
	ArtistsScanned int // artists with a city AND a state, considered
	DefiniteOK     int // geocoder unambiguously confirms the stored state — skipped, no MB call
	Confirmed      int // MusicBrainz identity-confirmed the SAME state
	Corrected      int // MusicBrainz identity-confirmed a DIFFERENT state → overwritten
	Unverified     int // couldn't confirm (no link / no match / non-US) → left as-is
	Corrections    []StateCorrection
	Errors         []string
}

// verifyArtistStore is the narrow store the verify pass needs.
type verifyArtistStore interface {
	ArtistsWithCityAndState(limit int) ([]catalogm.Artist, error)
	UpdateArtistLocation(id uint, fields map[string]interface{}) error
}

// VerifyArtistStates re-checks artists that already have a state and corrects a
// wrong one only when MusicBrainz identity-confirms a different state. Dry-run by
// default; the store-agnostic core is testable via verifyArtistStates with fakes.
func VerifyArtistStates(db *gorm.DB, g geo.Geocoder, mb MBStateResolver, opts VerifyOptions) (*VerifyReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if g == nil {
		return nil, fmt.Errorf("geocoder not initialized")
	}
	return verifyArtistStates(context.Background(), &gormArtistStore{db: db}, g, mb, opts)
}

func verifyArtistStates(
	ctx context.Context,
	store verifyArtistStore,
	g geo.Geocoder,
	mb MBStateResolver,
	opts VerifyOptions,
) (*VerifyReport, error) {
	artists, err := store.ArtistsWithCityAndState(opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("load artists: %w", err)
	}
	report := &VerifyReport{ArtistsScanned: len(artists)}
	now := time.Now()
	mbDisabled := mb == nil
	mbConsecutiveErrors := 0

	for i := range artists {
		a := &artists[i]
		city := trimPtr(a.City)
		current := trimPtr(a.State)
		if city == "" || current == "" {
			continue // store gate guarantees both; defensive
		}
		// Compare on a normalized 2-letter code: a stored value may be a full name
		// ("California", VARCHAR(10), written un-normalized from user input), so a
		// raw compare against MusicBrainz's "CA" would flag a correct state as a
		// "correction" and needlessly rewrite it. An unrecognized format normalizes
		// to itself (upper-cased) and so still compares unequal — handled as a real
		// disagreement, the conservative choice.
		currentNorm := normalizeUSState(current)

		// The geocoder unambiguously confirms a US-dominant city's state — those are
		// correct by the same logic that fills them, so skip them WITHOUT a MusicBrainz
		// call. (A wrong such state would have to disagree with an unambiguous
		// US-dominant name, which the fill pass never writes.)
		if st, status := g.ResolveUSState(city); status == geo.USStateUnambiguous && st == currentNorm {
			report.DefiniteOK++
			continue
		}

		// A non-US band's US state is a separate problem (this pass corrects US
		// states, never NULLs); MusicBrainz would yield no US state for it anyway.
		// Skip it without spending a call.
		country := trimPtr(a.Country)
		if iso, ok := geo.CountryToISO(country); ok && iso != "US" {
			report.Unverified++
			continue
		}

		if mbDisabled {
			report.Unverified++
			continue
		}

		confirmed, _, mbErr := mbState(ctx, mb, a, city)
		if mbErr != nil {
			mbConsecutiveErrors++
			report.Errors = append(report.Errors, fmt.Sprintf("musicbrainz %q: %v", a.Name, mbErr))
			if mbConsecutiveErrors >= mbErrorBreakerThreshold {
				mbDisabled = true
				report.Errors = append(report.Errors, fmt.Sprintf(
					"musicbrainz disabled after %d consecutive errors; remaining artists left unverified",
					mbConsecutiveErrors))
			}
			report.Unverified++
			continue
		}
		mbConsecutiveErrors = 0

		switch confirmed {
		case "":
			report.Unverified++ // couldn't confirm identity / no state → leave current
		case currentNorm:
			report.Confirmed++ // MusicBrainz agrees (modulo format) → already correct
		default:
			// Identity-confirmed a DIFFERENT state: the stored one was wrong.
			if opts.DryRun {
				report.recordCorrection(a, city, current, confirmed)
				continue
			}
			// Provenance: reuse the fill pass's empty-only rule (stateUpdate) — the
			// `state` value is corrected regardless, but data_source/confidence are
			// only stamped when the row has no prior source, so we don't clobber a
			// Bandcamp/user attribution (which also gates future MusicBrainz
			// enrichment, see pipeline/enrichment.go) for a single-field fix.
			if err := store.UpdateArtistLocation(a.ID, stateUpdate(a, confirmed, DataSourceMusicBrainz, now)); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("artist %d update: %v", a.ID, err))
				continue // a failed write is an error, NOT a correction — don't over-report
			}
			report.recordCorrection(a, city, current, confirmed)
		}
	}
	return report, nil
}

// recordCorrection counts and lists one correction; called only for a correction
// that was applied (or, in dry-run, would be applied) — never for a failed write.
func (r *VerifyReport) recordCorrection(a *catalogm.Artist, city, oldState, newState string) {
	r.Corrected++
	r.Corrections = append(r.Corrections, StateCorrection{
		ArtistID: a.ID, Name: a.Name, City: city, OldState: oldState, NewState: newState,
	})
}

// normalizeUSState reduces a stored state value to a 2-letter US code for
// comparison: a 2-letter input is upper-cased; a full state name is mapped via
// utils.StateNameToAbbrev; anything else is upper-cased as-is (so it compares
// unequal to a real code and is treated as a genuine disagreement).
func normalizeUSState(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) == 2 {
		return strings.ToUpper(s)
	}
	if abbr, ok := utils.StateNameToAbbrev(s); ok {
		return abbr
	}
	return strings.ToUpper(s)
}

// ArtistsWithCityAndState loads artists that have BOTH a non-blank city and a
// non-blank state — the candidates whose stored state can be re-verified.
// Ordered by id for a stable run; limit <= 0 means all.
func (s *gormArtistStore) ArtistsWithCityAndState(limit int) ([]catalogm.Artist, error) {
	var artists []catalogm.Artist
	q := s.db.
		Where("city IS NOT NULL AND TRIM(city) <> ''").
		Where("state IS NOT NULL AND TRIM(state) <> ''").
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return nil, err
	}
	return artists, nil
}
