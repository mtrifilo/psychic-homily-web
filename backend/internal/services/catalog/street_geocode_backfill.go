package catalog

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// PSY-1536: reconcile the street-level geocode columns on venues
// (street_latitude/street_longitude/geocode_precision/geocoded_address) from
// venues.address via the network AddressGeocoder (Nominatim).
//
// The foreground write paths set these fields inline (applyStreetGeocoding in
// CreateVenue/UpdateVenue); this reconciler is the backstop for everything
// else — bulk creators (FindOrCreateVenue, data-sync import), contribution
// edits (which clear the fields on an address change), and venues that
// predate the feature. A venue whose stored geocoded_address already equals
// its current address key is skipped without a network call, so a clean
// second run makes zero requests.

// StreetGeocodeOptions configures a backfill run.
type StreetGeocodeOptions struct {
	// DryRun performs the SAME network lookups (so the printed per-venue
	// results are real) but writes nothing.
	DryRun bool
	// Limit caps the number of venues geocoded over the network this run
	// (0 = no limit). Skips and clears don't count against it.
	Limit int
	// Timeout caps a single venue's lookup, including time waiting on the
	// shared 1 req/s limiter and retries. Zero uses a sensible default.
	Timeout time.Duration
}

const defaultStreetGeocodeBackfillTimeout = 30 * time.Second

// Street-geocode backfill actions, one per scanned venue that needs anything.
const (
	StreetGeocodeSet       = "set"        // fields were NULL, now populated
	StreetGeocodeUpdated   = "updated"    // address changed since the last geocode; re-resolved
	StreetGeocodeMiss      = "miss"       // address didn't resolve; any stale fields cleared
	StreetGeocodeCleared   = "cleared"    // address removed/blank; stale fields cleared
	StreetGeocodeUnchanged = "unchanged"  // stored key matches — skipped, no network call
	StreetGeocodeNoAddress = "no-address" // no address and nothing stored — nothing to do
	StreetGeocodeError     = "error"      // lookup failed (transport/service); left as-is
)

// StreetGeocodeChange is one venue's outcome in the report.
type StreetGeocodeChange struct {
	VenueID   uint
	Name      string
	City      string
	State     string
	Action    string
	Key       string  // the address key that was (or would be) geocoded
	Latitude  float64 // set/updated only
	Longitude float64 // set/updated only
	Precision string  // set/updated only
	Err       string  // error only
}

// StreetGeocodeReport is the structured outcome of a backfill run.
type StreetGeocodeReport struct {
	Scanned         int
	Set             int
	Updated         int
	Unchanged       int
	Missed          int
	Cleared         int
	NoAddress       int
	LimitHit        bool
	PrecisionCounts map[string]int // precision label -> count among set/updated
	Changes         []StreetGeocodeChange
	Errors          []string
}

// BackfillVenueStreetGeocodes scans every venue and resolves street-level
// coordinates for those whose address key doesn't match their stored geocode.
// Idempotent: a second run over unchanged data performs no lookups and no
// writes.
func BackfillVenueStreetGeocodes(db *gorm.DB, ag geo.AddressGeocoder, opts StreetGeocodeOptions) (*StreetGeocodeReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if ag == nil {
		return nil, fmt.Errorf("address geocoder not initialized")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultStreetGeocodeBackfillTimeout
	}

	var venues []catalogm.Venue
	if err := db.Order("id").Find(&venues).Error; err != nil {
		return nil, fmt.Errorf("load venues: %w", err)
	}

	report := &StreetGeocodeReport{PrecisionCounts: make(map[string]int)}
	geocoded := 0
	for i := range venues {
		v := &venues[i]
		report.Scanned++
		q := streetGeocodeQuery(v)
		key := q.Key()

		hasStored := v.StreetLatitude != nil || v.StreetLongitude != nil ||
			v.GeocodePrecision != nil || v.GeocodedAddress != nil

		if q.Street == "" {
			if hasStored {
				// Address removed after a geocode was stored — clear.
				report.Cleared++
				report.Changes = append(report.Changes, change(v, StreetGeocodeCleared, key))
				if !opts.DryRun {
					if err := writeStreetGeocode(db, v.ID, nil); err != nil {
						report.Errors = append(report.Errors, fmt.Sprintf("venue %d clear: %v", v.ID, err))
					}
				}
			} else {
				report.NoAddress++
			}
			continue
		}

		if v.GeocodedAddress != nil && *v.GeocodedAddress == key &&
			v.StreetLatitude != nil && v.StreetLongitude != nil {
			report.Unchanged++
			continue
		}

		if opts.Limit > 0 && geocoded >= opts.Limit {
			report.LimitHit = true
			break
		}
		geocoded++

		res, ok, err := geocodeWithTimeout(ag, q, timeout)
		if err != nil {
			c := change(v, StreetGeocodeError, key)
			c.Err = err.Error()
			report.Changes = append(report.Changes, c)
			report.Errors = append(report.Errors, fmt.Sprintf("venue %d %q: %v", v.ID, v.Name, err))
			continue
		}
		if !ok {
			report.Missed++
			report.Changes = append(report.Changes, change(v, StreetGeocodeMiss, key))
			// A stored geocode necessarily belongs to a DIFFERENT address key
			// (the matching case was skipped above) — it must not survive a
			// miss on the current one.
			if hasStored && !opts.DryRun {
				if err := writeStreetGeocode(db, v.ID, nil); err != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("venue %d clear-on-miss: %v", v.ID, err))
				}
			}
			continue
		}

		action := StreetGeocodeSet
		if hasStored {
			action = StreetGeocodeUpdated
		}
		if action == StreetGeocodeSet {
			report.Set++
		} else {
			report.Updated++
		}
		report.PrecisionCounts[res.Precision]++
		c := change(v, action, key)
		c.Latitude, c.Longitude, c.Precision = res.Latitude, res.Longitude, res.Precision
		report.Changes = append(report.Changes, c)

		if !opts.DryRun {
			if err := writeStreetGeocode(db, v.ID, &streetGeocodeWrite{result: res, key: key}); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("venue %d write: %v", v.ID, err))
			}
		}
	}
	return report, nil
}

func geocodeWithTimeout(ag geo.AddressGeocoder, q geo.AddressQuery, timeout time.Duration) (geo.AddressResult, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ag.GeocodeAddress(ctx, q)
}

// streetGeocodeWrite bundles a hit with the address key it was produced from.
type streetGeocodeWrite struct {
	result geo.AddressResult
	key    string
}

// writeStreetGeocode persists a venue's street-geocode fields; a nil write
// clears all four to SQL NULL.
func writeStreetGeocode(db *gorm.DB, venueID uint, w *streetGeocodeWrite) error {
	updates := map[string]interface{}{
		"street_latitude":   (*float64)(nil),
		"street_longitude":  (*float64)(nil),
		"geocode_precision": (*string)(nil),
		"geocoded_address":  (*string)(nil),
	}
	if w != nil {
		updates["street_latitude"] = w.result.Latitude
		updates["street_longitude"] = w.result.Longitude
		updates["geocode_precision"] = w.result.Precision
		updates["geocoded_address"] = w.key
	}
	return db.Model(&catalogm.Venue{}).Where("id = ?", venueID).Updates(updates).Error
}

func change(v *catalogm.Venue, action, key string) StreetGeocodeChange {
	return StreetGeocodeChange{
		VenueID: v.ID,
		Name:    v.Name,
		City:    v.City,
		State:   v.State,
		Action:  action,
		Key:     key,
	}
}
