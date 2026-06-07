package catalog

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/utils"
)

// Backfill of venue geocoding + re-anchoring of mis-zoned show instants
// (PSY-987, the final PR of the venue-timezone epic PSY-984).
//
// Two passes, dry-run by default:
//
//  1. Geocode every venue offline and populate latitude/longitude/timezone.
//     Idempotent — re-running yields the same resolved values, so a second run
//     reports zero changes.
//
//  2. Re-anchor show event_date instants that were stored under a WRONG assumed
//     timezone. The CLI ingest (cli/src/commands/submit-show.ts normalizeDate)
//     and the web ShowForm stamp a date-only show at 20:00 in a guessed zone
//     (the state map, defaulting empty/non-US states to America/Phoenix), so
//     e.g. a Berlin show landed at 20:00 Phoenix → 03:00Z and now renders at
//     05:00 in Berlin instead of 20:00. Re-anchoring recovers the intended
//     20:00 wall-time and re-stamps it in the venue's real geocoded zone.
//
// The re-anchor pass is deliberately CONSERVATIVE (see reanchorEventDate): it
// only touches shows it can confidently recognize as mis-zoned 20:00 date-only
// shows, and leaves anything ambiguous untouched for manual review. event_date
// is a destructive rewrite of shared data, so the safe default is to do nothing
// when unsure.
//
// NOT every date-only show is a 20:00 show: the AI-discovery pipeline
// (services/pipeline/discovery.go parseEventDate) stamps date-only events at
// 00:00 UTC, which this pass deliberately leaves ambiguous (no 20:00 marker) —
// it does not attempt to recover them. And the safety argument is airtight only
// for US venues (see reanchorEventDate); for empty/non-US-state venues the
// assumed zone falls back to Phoenix, so a dry-run review before --confirm is
// the backstop, not just a courtesy.

// defaultEveningHour is the local hour a date-only show defaults to when it is
// created (mirrors the CLI's normalizeDate, which stamps 20:00 venue-local).
// It is the signal the re-anchor pass keys on to recognize a date-only show.
const defaultEveningHour = 20

// coordPrecision is the number of decimal places venue latitude/longitude are
// stored at (DB column numeric(9,6)). Rounding the geocoder's full-precision
// result to the same scale before comparing keeps the backfill idempotent —
// otherwise raw-float vs DB-rounded-float would always look "changed".
const coordPrecision = 1e6

// BackfillOptions configures a backfill run.
type BackfillOptions struct {
	DryRun  bool
	Verbose bool
}

// VenueGeoChange records the geocoding outcome for a single venue.
type VenueGeoChange struct {
	VenueID uint
	Name    string
	City    string
	State   string
	OldTz   *string
	NewTz   *string
	Action  string // "set", "updated", "unchanged", "miss"
}

// ShowReanchorChange records the re-anchor outcome for a single show.
type ShowReanchorChange struct {
	ShowID     uint
	Title      string
	VenueID    uint
	AssumedTz  string
	GeocodedTz string
	OldInstant time.Time
	NewInstant time.Time
	// Action is one of "reanchored", "ambiguous", "no-venue-tz". (Already-correct
	// shows are counted in the report but not recorded as a change row.)
	Action string
}

// BackfillReport is the structured outcome of a backfill run.
type BackfillReport struct {
	// Venue pass
	VenuesScanned    int
	VenuesSet        int // tz set where there was none
	VenuesUpdated    int // tz changed from an existing value
	VenuesCoordsOnly int // tz unchanged, only latitude/longitude changed
	VenuesUnchanged  int
	VenuesMissed     int // no geocode match
	VenueChanges     []VenueGeoChange

	// Show pass
	ShowsScanned    int
	ShowsReanchored int
	ShowsAlreadyOK  int
	ShowsAmbiguous  int
	ShowsNoVenueTz  int
	ShowChanges     []ShowReanchorChange

	Errors []string
}

// BackfillVenueTimezones geocodes every venue and re-anchors mis-zoned show
// instants. With opts.DryRun the report describes exactly what a live run would
// change without writing anything; the resolved-timezone map is computed the
// same way in both modes so the dry-run is faithful.
func BackfillVenueTimezones(database *gorm.DB, g geo.Geocoder, opts BackfillOptions) (*BackfillReport, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if g == nil {
		return nil, fmt.Errorf("geocoder not initialized")
	}

	report := &BackfillReport{}

	// effectiveTz holds the post-backfill IANA zone per venue id: the freshly
	// geocoded zone on a hit, otherwise the venue's existing stored zone. The
	// show pass reads this map (NOT the DB) so a dry-run re-anchors against the
	// zones a live run WOULD have written.
	effectiveTz := make(map[uint]*string)

	if err := backfillVenuePass(database, g, opts, report, effectiveTz); err != nil {
		return report, err
	}
	if err := reanchorShowPass(database, opts, report, effectiveTz); err != nil {
		return report, err
	}

	return report, nil
}

// backfillVenuePass geocodes each venue and (on a live run) writes the resolved
// latitude/longitude/timezone.
func backfillVenuePass(
	database *gorm.DB,
	g geo.Geocoder,
	opts BackfillOptions,
	report *BackfillReport,
	effectiveTz map[uint]*string,
) error {
	var venues []catalogm.Venue
	if err := database.Find(&venues).Error; err != nil {
		return fmt.Errorf("load venues: %w", err)
	}
	report.VenuesScanned = len(venues)

	for i := range venues {
		v := &venues[i]
		country := ""
		if v.Country != nil {
			country = *v.Country
		}

		res, ok := g.Resolve(v.City, v.State, country)
		if !ok {
			// Leave existing values untouched — the backfill only adds data.
			effectiveTz[v.ID] = v.Timezone
			report.VenuesMissed++
			if opts.Verbose {
				report.VenueChanges = append(report.VenueChanges, VenueGeoChange{
					VenueID: v.ID, Name: v.Name, City: v.City, State: v.State,
					OldTz: v.Timezone, NewTz: v.Timezone, Action: "miss",
				})
			}
			continue
		}

		newTz := res.Timezone
		newLat := roundCoord(res.Latitude)
		newLng := roundCoord(res.Longitude)
		effectiveTz[v.ID] = &newTz

		tzChanged := derefString(v.Timezone) != newTz
		latChanged := !floatPtrEq(v.Latitude, &newLat)
		lngChanged := !floatPtrEq(v.Longitude, &newLng)

		switch {
		case !tzChanged && !latChanged && !lngChanged:
			report.VenuesUnchanged++
			if opts.Verbose {
				report.VenueChanges = append(report.VenueChanges, VenueGeoChange{
					VenueID: v.ID, Name: v.Name, City: v.City, State: v.State,
					OldTz: v.Timezone, NewTz: &newTz, Action: "unchanged",
				})
			}
			continue
		case v.Timezone == nil:
			report.VenuesSet++
			report.VenueChanges = append(report.VenueChanges, VenueGeoChange{
				VenueID: v.ID, Name: v.Name, City: v.City, State: v.State,
				OldTz: v.Timezone, NewTz: &newTz, Action: "set",
			})
		case tzChanged:
			report.VenuesUpdated++
			report.VenueChanges = append(report.VenueChanges, VenueGeoChange{
				VenueID: v.ID, Name: v.Name, City: v.City, State: v.State,
				OldTz: v.Timezone, NewTz: &newTz, Action: "updated",
			})
		default:
			// Timezone unchanged; only the coordinates moved (e.g. a GeoNames
			// refresh shifted the city centroid). Don't print a misleading
			// "tz -> tz" line.
			report.VenuesCoordsOnly++
			report.VenueChanges = append(report.VenueChanges, VenueGeoChange{
				VenueID: v.ID, Name: v.Name, City: v.City, State: v.State,
				OldTz: v.Timezone, NewTz: &newTz, Action: "coords",
			})
		}

		if opts.DryRun {
			continue
		}
		// Plain values, not pointers: a geocode hit always yields all three, and
		// GORM's map-Updates rejects pointer values with "invalid field".
		if err := database.Model(&catalogm.Venue{}).
			Where("id = ?", v.ID).
			Updates(map[string]interface{}{
				"latitude":  newLat,
				"longitude": newLng,
				"timezone":  newTz,
			}).Error; err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("venue %d update: %v", v.ID, err))
		}
	}

	return nil
}

// reanchorShowPass re-anchors show instants that were stored under a wrong
// assumed timezone, using the freshly resolved venue zones in effectiveTz.
func reanchorShowPass(
	database *gorm.DB,
	opts BackfillOptions,
	report *BackfillReport,
	effectiveTz map[uint]*string,
) error {
	var shows []catalogm.Show
	if err := database.Preload("Venues").Find(&shows).Error; err != nil {
		return fmt.Errorf("load shows: %w", err)
	}
	report.ShowsScanned = len(shows)

	for i := range shows {
		show := &shows[i]

		primary, ok := primaryVenue(show.Venues)
		if !ok {
			report.ShowsNoVenueTz++
			continue
		}
		venueID := primary.ID
		tzPtr := effectiveTz[venueID]
		if tzPtr == nil || *tzPtr == "" {
			report.ShowsNoVenueTz++
			if opts.Verbose {
				report.ShowChanges = append(report.ShowChanges, ShowReanchorChange{
					ShowID: show.ID, Title: show.Title, VenueID: venueID, Action: "no-venue-tz",
				})
			}
			continue
		}

		geocoded, err := time.LoadLocation(*tzPtr)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("show %d: bad geocoded tz %q: %v", show.ID, *tzPtr, err))
			continue
		}
		// Key the assumed (legacy-fallback) zone on the VENUE's state — the field
		// the writers keyed on — not the show's denormalized State (which can be
		// NULL or drift). utils.StateTimezones is the full 50-state + DC map, kept
		// in sync with the CLI writer's map (cli/src/lib/timezone.ts); for US
		// states this REQUIRED-for-safety match makes a correctly-stored US show
		// resolve assumed == geocoded so sameZone() short-circuits the recover
		// branch (a short map defaulting to Phoenix would corrupt a real 11pm
		// Eastern show — see reanchorEventDate). This is a best-effort
		// reconstruction, not an exact replay: the web ShowForm stamps 20:00 in
		// the SUBMITTER'S BROWSER zone when the venue has no state, and non-US
		// states fall back to Phoenix here. Those cases either land on
		// outcomeAmbiguous (safe) or are recovered correctly; the one residual
		// false-positive (a non-US explicit show at exactly 03:00Z) is covered by
		// the dry-run review (see reanchorEventDate's doc).
		assumedName := utils.GetTimezoneForState(primary.State)
		assumed, err := time.LoadLocation(assumedName)
		if err != nil {
			assumed = time.UTC
		}

		newInstant, outcome := reanchorEventDate(show.EventDate, geocoded, assumed)
		switch outcome {
		case outcomeAlreadyCorrect:
			report.ShowsAlreadyOK++
			continue
		case outcomeAmbiguous:
			report.ShowsAmbiguous++
			report.ShowChanges = append(report.ShowChanges, ShowReanchorChange{
				ShowID: show.ID, Title: show.Title, VenueID: venueID,
				AssumedTz: assumedName, GeocodedTz: *tzPtr,
				OldInstant: show.EventDate.UTC(), Action: "ambiguous",
			})
			continue
		}

		// outcomeReanchored
		report.ShowsReanchored++
		report.ShowChanges = append(report.ShowChanges, ShowReanchorChange{
			ShowID: show.ID, Title: show.Title, VenueID: venueID,
			AssumedTz: assumedName, GeocodedTz: *tzPtr,
			OldInstant: show.EventDate.UTC(), NewInstant: newInstant.UTC(), Action: "reanchored",
		})

		if opts.DryRun {
			continue
		}
		// Rewrite shows.event_date AND cascade onto the denormalized
		// show_artists.event_date in one transaction so the partial unique
		// index (artist_id, venue_id, event_date) stays consistent. A
		// per-show transaction keeps a collision on one show from rolling
		// back the rest (mirrors cmd/dedup-shows).
		err = database.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&catalogm.Show{}).
				Where("id = ?", show.ID).
				Update("event_date", newInstant.UTC()).Error; err != nil {
				return fmt.Errorf("update event_date: %w", err)
			}
			if err := syncShowArtistDedupColumns(tx, show.ID); err != nil {
				return fmt.Errorf("sync show_artists: %w", err)
			}
			return nil
		})
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("show %d reanchor: %v", show.ID, err))
		}
	}

	return nil
}

// reanchorOutcome is the single classification of a show's re-anchor decision,
// so the caller never has to re-derive the "already correct" condition (which
// would let the report drift from the actual change logic).
type reanchorOutcome int

const (
	// outcomeAlreadyCorrect: the instant already reads 20:00 in the geocoded
	// zone — stored correctly, nothing to do.
	outcomeAlreadyCorrect reanchorOutcome = iota
	// outcomeReanchored: recovered a mis-zoned date-only show; the returned
	// instant is the corrected value.
	outcomeReanchored
	// outcomeAmbiguous: cannot confidently classify (explicit non-20:00 time,
	// or neither zone yields the 20:00 marker) — left untouched.
	outcomeAmbiguous
)

// reanchorEventDate decides whether a stored event instant was mis-zoned and,
// if so, returns the corrected instant. It is intentionally conservative:
//
//   - outcomeAlreadyCorrect: the instant already lands on the default evening
//     wall-time (20:00) in the venue's real (geocoded) zone — stored correctly.
//   - outcomeReanchored: the instant lands on 20:00 in the zone the writer
//     ACTUALLY assumed (the legacy state-fallback zone, distinct from the
//     geocoded one) — a recoverable mis-zoned date-only show, re-stamped to
//     20:00 local in the geocoded zone.
//   - outcomeAmbiguous: an explicit non-20:00 time, or a case where neither
//     zone yields 20:00 — left untouched for manual review.
//
// Safety relies on `assumed` matching the zone the data was actually written
// under. For US shows that holds exactly: StateTimezones is the full 50-state +
// DC writer map, so a correctly-stored show resolves assumed == geocoded (its
// real zone) and sameZone() short-circuits before the re-stamp can fire — a real
// 11pm-Eastern show (= 20:00 Phoenix wall-clock) is therefore left untouched,
// not corrupted.
//
// For empty/non-US-state venues `assumed` falls back to Phoenix (the writers'
// own default for them), which is what lets the genuinely mis-zoned non-US
// date-only shows recover. The residual risk: a correctly-stored NON-US
// explicit-time show whose UTC instant happens to be exactly 03:00:00Z reads as
// 20:00 in Phoenix and would be wrongly re-anchored. This is an inherent
// ambiguity (a 20:00-Phoenix date-only show and a foreign 03:00Z explicit show
// are indistinguishable from the instant alone) — not closable without dropping
// non-US recovery entirely. The backstop is the mandatory dry-run review before
// --confirm: every re-anchor is listed old→new for the operator to eyeball.
func reanchorEventDate(stored time.Time, geocoded, assumed *time.Location) (time.Time, reanchorOutcome) {
	// Already correct in the venue's real zone.
	if isDefaultEveningWall(stored.In(geocoded)) {
		return stored, outcomeAlreadyCorrect
	}
	// A different assumed zone that recovers the 20:00 default → re-anchor.
	if !sameZone(geocoded, assumed) {
		wall := stored.In(assumed)
		if isDefaultEveningWall(wall) {
			corrected := time.Date(
				wall.Year(), wall.Month(), wall.Day(),
				defaultEveningHour, 0, 0, 0,
				geocoded,
			)
			return corrected, outcomeReanchored
		}
	}
	// Ambiguous — leave it for manual review.
	return stored, outcomeAmbiguous
}

// isDefaultEveningWall reports whether t's wall-clock time is exactly the
// date-only default (20:00:00.000). A date-only show created via the CLI is
// stamped at precisely this time in its assumed zone, so it's a reliable marker
// that the instant carries no meaningful time-of-day.
func isDefaultEveningWall(t time.Time) bool {
	return t.Hour() == defaultEveningHour &&
		t.Minute() == 0 &&
		t.Second() == 0 &&
		t.Nanosecond() == 0
}

// sameZone compares two locations by IANA name.
func sameZone(a, b *time.Location) bool {
	return a.String() == b.String()
}

// primaryVenue returns the venue with the smallest id among a show's venues.
// This deterministic lowest-venue_id tiebreaker matches syncShowArtistDedupColumns
// and the partial unique index (artist_id, venue_id, event_date), so re-anchoring
// the instant against THIS venue's zone keeps the stored event_date consistent
// with the dedup column the same pass rewrites. ok is false when the show has no
// venues.
//
// Note: render surfaces (ShowCard/ShowHeader, the iCal feed, Discord) display
// using show.Venues[0], not the lowest id. For the overwhelmingly common
// single-venue show the two coincide. A multi-venue show whose venues span
// different zones could therefore read slightly differently across surfaces —
// an extremely rare, pre-existing ambiguity not introduced by this pass.
func primaryVenue(venues []catalogm.Venue) (*catalogm.Venue, bool) {
	if len(venues) == 0 {
		return nil, false
	}
	primary := &venues[0]
	for i := 1; i < len(venues); i++ {
		if venues[i].ID < primary.ID {
			primary = &venues[i]
		}
	}
	return primary, true
}

func roundCoord(f float64) float64 {
	return math.Round(f*coordPrecision) / coordPrecision
}

func floatPtrEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
