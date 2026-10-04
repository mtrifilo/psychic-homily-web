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

// Apply writes the hits of a reviewed report and records its misses. It makes
// no lookups: what it writes is what the report says, so the review is
// binding. The report is a file a person edited, so every row is checked
// before it is used. A row is skipped, with the reason, when:
//   - its outcome is an error (nothing was decided), or would_write is false;
//   - it is marked REVIEW and approve_review is not set;
//   - the venue now has an address, no longer exists, or had its address
//     cleared after it had one (by an earlier Apply or by an editor);
//   - the venue's inputs changed since the report (its lookup key differs),
//     so the row describes a lookup that would no longer be made;
//   - its values are malformed (precision outside the phase's vocabulary,
//     coordinates out of range, an empty or over-long address).
//
// Every write is scoped to the venue row still holding an empty address.
func (b *VenueAddressBackfill) Apply(ctx context.Context, r *VenueAddressReport) (*VenueAddressApplyResult, error) {
	if b.DB == nil {
		return nil, errors.New("database not initialized")
	}
	ok, err := b.lookupsTableExists(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("venue_address_lookups does not exist: apply the database migrations first")
	}
	result := &VenueAddressApplyResult{}
	var ids []uint
	for _, row := range r.Rows {
		ids = append(ids, row.VenueID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	now := time.Now()
	cands, err := b.loadCandidates(ctx, VenueAddressBackfillOptions{}, now, ids)
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
	ticketPages, err := b.loadTicketPages(ctx, ids, now)
	if err != nil {
		return nil, err
	}

	for _, row := range r.Rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		out := VenueAddressApplyRow{VenueID: row.VenueID, Name: row.Name, Phase: row.Phase, Address: row.Address}
		skip := func(reason string) {
			out.Action, out.Reason = ApplySkipped, reason
			result.Skipped++
		}
		c := venues[row.VenueID]
		switch {
		case row.Outcome != catalogm.VenueAddressOutcomeHit && row.Outcome != catalogm.VenueAddressOutcomeMiss:
			skip("the lookup did not decide anything (" + row.Outcome + ")")
		case c == nil:
			skip("the venue has an address now, or no longer exists")
		case c.ClearedByEditor || filledBefore(memos, row.VenueID):
			skip("the venue's address was cleared after it had one")
		case currentLookupKey(c, row.Phase, ingestPages, ticketPages, r.AIEnabled) != row.LookupKey:
			skip("the venue's inputs changed since the report")
		case row.Outcome == catalogm.VenueAddressOutcomeMiss:
			if err := recordAddressLookup(b.DB, row.VenueID, row.Phase, row.LookupKey, catalogm.VenueAddressOutcomeMiss, "", ""); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("venue %d record %s miss: %v", row.VenueID, row.Phase, err))
				skip("recording the miss failed")
				break
			}
			out.Action = ApplyMissRecorded
			result.MissesRecorded++
		case !row.WouldWrite:
			skip("would_write is false")
		case row.Review && !row.ApproveReview:
			skip("a REVIEW row needs approve_review set to true")
		default:
			updates, why := applyColumns(&c.Venue, row)
			if why != "" {
				skip(why)
				break
			}
			written, err := b.writeApproved(&c.Venue, row, updates)
			switch {
			case err != nil:
				result.Errors = append(result.Errors, fmt.Sprintf("venue %d write: %v", row.VenueID, err))
				skip("the write failed")
			case !written:
				skip("the venue changed since it was read")
			default:
				out.Action = ApplyWritten
				result.Written++
			}
		}
		result.Rows = append(result.Rows, out)
	}
	return result, nil
}

// currentLookupKey is the key the lookup run would use for this venue and
// phase today.
func currentLookupKey(c *addressCandidate, phase string, ingestPages map[uint]string, ticketPages map[uint][]string, aiEnabled bool) string {
	if phase == catalogm.VenueAddressPhasePage {
		return pageLookupKey(pageSources(&c.Venue, ingestPages[c.ID], ticketPages[c.ID]), aiEnabled)
	}
	return nameLookupKey(venuePlaceQuery(&c.Venue))
}

// applyColumns checks a hit row's values and returns the venue columns to
// write, or why the row is refused.
func applyColumns(v *catalogm.Venue, row VenueAddressRow) (map[string]interface{}, string) {
	address := strings.TrimSpace(row.Address)
	if address == "" || len(address) > maxAddressLen {
		return nil, "the address is empty or too long"
	}
	updated := *v
	updated.Address = &address
	key := streetGeocodeQuery(&updated).Key()
	updates := map[string]interface{}{}
	switch row.Geocode {
	case GeocodeHit:
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
	case GeocodeMiss:
		updates = streetGeocodeMissColumns(key)
	case GeocodeNone, "":
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
		return recordAddressLookup(tx, v.ID, row.Phase, row.LookupKey, catalogm.VenueAddressOutcomeHit, row.Source, updates["address"].(string))
	})
	return written, err
}
