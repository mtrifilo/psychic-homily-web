package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// Apply actions, one per report row Apply considered.
const (
	ApplyWritten      = "written"
	ApplyMissRecorded = "miss_recorded"
	ApplyRefused      = "refused" // a hit the reviewer did not accept, recorded as a miss
	ApplySkipped      = "skipped"
)

// VenueAddressApplyRow is what Apply did with one report row.
type VenueAddressApplyRow struct {
	VenueID uint   `json:"venue_id"`
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Address string `json:"address,omitempty"`
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
}

// VenueAddressApplyResult is the outcome of applying a reviewed report.
type VenueAddressApplyResult struct {
	Written        int                    `json:"written"`
	MissesRecorded int                    `json:"misses_recorded"`
	Refused        int                    `json:"refused"`
	Skipped        int                    `json:"skipped"`
	Rows           []VenueAddressApplyRow `json:"rows"`
	Errors         []string               `json:"errors,omitempty"`
}

// pagePrecisions are the precisions a page-phase hit may carry: those of the
// structured street geocode.
var pagePrecisions = map[string]bool{
	geo.PrecisionRooftop: true, geo.PrecisionInterpolated: true, geo.PrecisionCity: true,
}

// maxAddressLen is the venues.address column width.
const maxAddressLen = 500

// maxReportAge bounds how old a report Apply accepts: its misses are dated
// when the lookup ran and honoured for missMemoTTL from then, so an older
// report has nothing current to say.
const maxReportAge = missMemoTTL

// Apply writes the accepted hits of a reviewed report and records its misses.
// It makes no lookups: what it writes is what the report says, so the review
// is binding. The report is a file a person edited, so every row is checked
// before it is used.
//
// What a reviewer's edits mean:
//   - a hit with would_write true (and, for a REVIEW row, approve_review true)
//     is written;
//   - any other hit was refused, and is recorded as a miss for its phase, so
//     the next lookup moves past it (to the name search, for a refused page
//     hit) until missMemoTTL;
//   - a miss is recorded;
//   - a deleted row is left undecided: nothing is recorded, and the next
//     lookup tries that venue and phase again.
//
// A row is skipped, with the reason, when the venue now has an address, no
// longer exists, had its address cleared after it had one, has another name,
// city, or state than the row, or no longer has the inputs the row's lookup
// used; when an earlier row of the same report already wrote the venue; when
// its values are malformed; or when its outcome decided nothing. A report with
// two rows for one venue and phase, or older than maxReportAge, is refused
// whole. Every write is scoped to the venue row still holding an empty
// address.
func (b *VenueAddressBackfill) Apply(ctx context.Context, r *VenueAddressReport) (*VenueAddressApplyResult, error) {
	if b.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if r.GeneratedAt.IsZero() || time.Since(r.GeneratedAt) > maxReportAge {
		return nil, fmt.Errorf("the report is older than %s or undated: run a new lookup", maxReportAge)
	}
	seen := map[string]bool{}
	var ids []uint
	for _, row := range r.Rows {
		k := fmt.Sprintf("%d/%s", row.VenueID, row.Phase)
		if seen[k] {
			return nil, fmt.Errorf("the report has two rows for venue %d, phase %q: keep one", row.VenueID, row.Phase)
		}
		seen[k] = true
		ids = append(ids, row.VenueID)
	}
	ok, err := b.lookupsTableExists(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("venue_address_lookups does not exist: apply the database migrations first")
	}
	result := &VenueAddressApplyResult{}
	if len(ids) == 0 {
		return result, nil
	}
	cands, err := b.loadCandidates(ctx, VenueAddressBackfillOptions{}, time.Now(), ids)
	if err != nil {
		return nil, err
	}
	venues := map[uint]*addressCandidate{}
	for i := range cands {
		venues[cands[i].ID] = &cands[i]
	}
	memos, err := b.loadMemos(ctx, ids)
	if err != nil {
		return nil, err
	}
	ingestPages, err := b.loadIngestPages(ctx, ids)
	if err != nil {
		return nil, err
	}
	// The ticket pages the lookup saw: shows on or after its run, created by
	// then. Shows that passed or were added since do not change a row's key.
	ticketPages, err := b.loadTicketPages(ctx, ids, r.GeneratedAt, r.GeneratedAt)
	if err != nil {
		return nil, err
	}

	written := map[uint]bool{}
	for _, row := range r.Rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		out := VenueAddressApplyRow{VenueID: row.VenueID, Name: row.Name, Phase: row.Phase, Address: row.Address}
		skip := func(reason string) {
			out.Action, out.Reason = ApplySkipped, reason
			result.Skipped++
		}
		record := func(action, reason string) {
			if err := recordAddressLookup(b.DB, row.VenueID, row.Phase, row.LookupKey, catalogm.VenueAddressOutcomeMiss, "", "", r.GeneratedAt); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("venue %d record %s miss: %v", row.VenueID, row.Phase, err))
				skip("recording the miss failed")
				return
			}
			out.Action, out.Reason = action, reason
			if action == ApplyRefused {
				result.Refused++
			} else {
				result.MissesRecorded++
			}
		}
		c := venues[row.VenueID]
		switch {
		case row.Outcome != catalogm.VenueAddressOutcomeHit && row.Outcome != catalogm.VenueAddressOutcomeMiss:
			skip("the lookup did not decide anything (" + row.Outcome + ")")
		case written[row.VenueID]:
			skip("an earlier row of this report already wrote this venue")
		case c == nil:
			skip("the venue has an address now, or no longer exists")
		case c.ClearedByEditor || filledBefore(memos, row.VenueID):
			skip("the venue's address was cleared after it had one")
		case c.Name != row.Name || c.City != row.City || c.State != row.State:
			skip("the venue's name, city, or state changed since the report")
		case row.Phase == catalogm.VenueAddressPhasePage && len(pageSources(&c.Venue, ingestPages[c.ID], ticketPages[c.ID])) == 0:
			skip("the venue has no pages for a page-phase row")
		case currentLookupKey(c, row.Phase, ingestPages, ticketPages, r.AIEnabled) != row.LookupKey:
			skip("the venue's inputs changed since the report")
		case row.Outcome == catalogm.VenueAddressOutcomeMiss:
			record(ApplyMissRecorded, "")
		case !row.WouldWrite:
			record(ApplyRefused, "would_write is false")
		case (row.Review || !c.Verified) && !row.ApproveReview:
			record(ApplyRefused, "a REVIEW row (every row for an unverified venue is one) needs approve_review set to true")
		default:
			updates, why := applyColumns(&c.Venue, row)
			if why != "" {
				skip(why)
				break
			}
			ok, err := b.writeApproved(&c.Venue, row, updates)
			switch {
			case err != nil:
				result.Errors = append(result.Errors, fmt.Sprintf("venue %d write: %v", row.VenueID, err))
				skip("the write failed")
			case !ok:
				skip("the venue changed since it was read")
			default:
				written[row.VenueID] = true
				out.Action = ApplyWritten
				result.Written++
			}
		}
		result.Rows = append(result.Rows, out)
	}
	return result, nil
}

// currentLookupKey is the key the lookup run would use for this venue and
// phase from these inputs.
func currentLookupKey(c *addressCandidate, phase string, ingestPages map[uint]string, ticketPages map[uint][]string, aiEnabled bool) string {
	if phase == catalogm.VenueAddressPhasePage {
		return pageLookupKey(pageSources(&c.Venue, ingestPages[c.ID], ticketPages[c.ID]), aiEnabled)
	}
	return nameLookupKey(venuePlaceQuery(&c.Venue))
}

// applyColumns checks a hit row's values and returns the venue columns to
// write, or why the row is refused. The geocode fields belong to the address
// the lookup produced (LookedUpAddress): when a reviewer corrected the
// address, a page row is written without them, so the sweep geocodes the
// corrected address, and a name row is refused, since its point is the
// matched place's.
func applyColumns(v *catalogm.Venue, row VenueAddressRow) (map[string]interface{}, string) {
	address := strings.TrimSpace(row.Address)
	if address == "" || len(address) > maxAddressLen {
		return nil, "the address is empty or too long"
	}
	corrected := address != strings.TrimSpace(row.LookedUpAddress)
	if corrected && row.Phase == catalogm.VenueAddressPhaseName {
		return nil, "a name-phase address cannot be corrected: its point is the matched place's"
	}
	updated := *v
	updated.Address = &address
	key := streetGeocodeQuery(&updated).Key()
	updates := map[string]interface{}{}
	switch {
	case corrected:
		// The sweep geocodes the corrected address.
	case row.Geocode == GeocodeHit:
		precisionOK := pagePrecisions[row.Precision]
		if row.Phase == catalogm.VenueAddressPhaseName {
			precisionOK = row.Precision == geo.PrecisionNameSearch
		}
		if !precisionOK {
			return nil, fmt.Sprintf("precision %q is not valid for the %s phase", row.Precision, row.Phase)
		}
		if row.Latitude == nil || row.Longitude == nil ||
			*row.Latitude < -90 || *row.Latitude > 90 || *row.Longitude < -180 || *row.Longitude > 180 {
			return nil, "the coordinates are missing or out of range"
		}
		updates = streetGeocodeHitColumns(geo.AddressResult{
			Latitude: *row.Latitude, Longitude: *row.Longitude, Precision: row.Precision,
		}, key)
	case row.Geocode == GeocodeMiss:
		updates = streetGeocodeMissColumns(key)
	case row.Geocode == GeocodeNone || row.Geocode == "":
		if row.Phase == catalogm.VenueAddressPhaseName {
			return nil, "a name-phase row must carry its matched point"
		}
	default:
		return nil, fmt.Sprintf("geocode %q is not a known outcome", row.Geocode)
	}
	updates["address"] = address
	return updates, ""
}

// writeApproved stores the row's columns and records the phase hit in one
// transaction, only while the venue row still holds the empty address it was
// read with. written is false when that guard matched nothing.
func (b *VenueAddressBackfill) writeApproved(v *catalogm.Venue, row VenueAddressRow, updates map[string]interface{}) (written bool, err error) {
	err = b.DB.Transaction(func(tx *gorm.DB) error {
		res := streetGeocodeUpdateScope(tx, v).Updates(updates)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		written = true
		return recordAddressLookup(tx, v.ID, row.Phase, row.LookupKey, catalogm.VenueAddressOutcomeHit, row.Source, updates["address"].(string), time.Time{})
	})
	return written, err
}
