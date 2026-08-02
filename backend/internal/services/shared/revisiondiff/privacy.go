package revisiondiff

import (
	"fmt"

	adminm "psychic-homily-backend/internal/models/admin"
)

// RedactedValue replaces both sides of a masked diff. It is a display string,
// not a sentinel to branch on: the History UI renders old/new values verbatim,
// so a masked entry reads as "this field changed, the value is withheld" while
// the surrounding revision (author, timestamp, which field, the other fields)
// stays intact.
//
// Both sides are always replaced, never just the new one. Consecutive revisions
// chain — revision N's old value is revision N-1's new value — so masking one
// side republishes the value from the neighbouring row.
const RedactedValue = "(hidden)"

// venuePrivateFields names the revision fields that carry the unverified-venue
// address gate. It is the revision-history spelling of catalog.Venue's
// PublicAddress / PublicZipcode accessors, and the two must not drift: a field
// that is withheld from the live venue payload but published in that venue's
// edit history is not withheld at all.
//
// Capacity is deliberately absent, matching the live gate, which serves it for
// unverified venues because a room size does not locate anybody's house.
var venuePrivateFields = map[string]struct{}{
	"address": {},
	"zipcode": {},
}

// RedactVenueChanges masks the address-family values in a diff recorded against
// an UNVERIFIED venue, and reports whether anything was masked.
//
// Callers own the verification lookup; this function owns the field list. It
// returns a new slice and never mutates the input, because the input is
// unmarshalled from a stored row that other code paths (rollback) must still
// see raw.
//
// The `redacted` return exists so callers can leave an untouched revision's
// stored JSON bytes alone rather than round-tripping every row through
// marshal/unmarshal for no change.
func RedactVenueChanges(changes []adminm.FieldChange) (out []adminm.FieldChange, redacted bool) {
	out = make([]adminm.FieldChange, len(changes))
	copy(out, changes)
	for i := range out {
		if _, private := venuePrivateFields[out[i].Field]; !private {
			continue
		}
		out[i].OldValue = RedactedValue
		out[i].NewValue = RedactedValue
		redacted = true
	}
	return out, redacted
}

// validateVenuePrivateFields fails startup if a private field name is not one
// the venue diff can actually emit. Without it, renaming "address" in
// VenueFields would silently turn the redaction into a no-op — a privacy gate
// that stops matching anything is indistinguishable from one that works.
func validateVenuePrivateFields() error {
	present := make(map[string]struct{}, len(VenueFields))
	for _, f := range VenueFields {
		present[f.Name] = struct{}{}
	}
	for name := range venuePrivateFields {
		if _, ok := present[name]; !ok {
			return fmt.Errorf("revisiondiff: venue private field %q is not in VenueFields", name)
		}
	}
	return nil
}
