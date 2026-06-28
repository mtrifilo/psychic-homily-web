package enrich

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
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

		// The geocoder unambiguously confirms a US-dominant city's state — those are
		// correct by the same logic that fills them, so skip them WITHOUT a MusicBrainz
		// call. (A wrong such state would have to disagree with an unambiguous
		// US-dominant name, which the fill pass never writes.)
		if st, status := g.ResolveUSState(city); status == geo.USStateUnambiguous && strings.EqualFold(st, current) {
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

		switch {
		case confirmed == "":
			report.Unverified++ // couldn't confirm identity / no state → leave current
		case strings.EqualFold(confirmed, current):
			report.Confirmed++ // MusicBrainz agrees → already correct
		default:
			// Identity-confirmed a DIFFERENT state: the stored one was wrong.
			report.Corrected++
			report.Corrections = append(report.Corrections, StateCorrection{
				ArtistID: a.ID, Name: a.Name, City: city, OldState: current, NewState: confirmed,
			})
			if opts.DryRun {
				continue
			}
			if err := store.UpdateArtistLocation(a.ID, correctionUpdate(confirmed, now)); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("artist %d update: %v", a.ID, err))
			}
		}
	}
	return report, nil
}

// correctionUpdate overwrites a wrong state with the identity-confirmed
// MusicBrainz one and stamps coherent provenance: the state is now an
// MusicBrainz-verified origin, so data_source/source_confidence/last_verified_at
// describe that (unlike the fill pass's empty-only rule — here we are correcting a
// field whose new value IS authoritatively MusicBrainz's).
func correctionUpdate(state string, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"state":             state,
		"data_source":       DataSourceMusicBrainz,
		"source_confidence": confidenceMusicBrainz,
		"last_verified_at":  now,
	}
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
