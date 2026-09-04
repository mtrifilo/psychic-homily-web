package revisiondiff

import (
	"fmt"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// RedactedValue replaces both sides of a masked diff.
//
// Both sides are always replaced, never just the new one. Consecutive revisions
// chain — revision N's old value is revision N-1's new value — so masking one
// side republishes the value from the neighbouring row. It is a fixed string
// rather than a derived one for the same reason a password field shows a fixed
// number of dots: a length- or format-preserving mask is an oracle.
//
// It is a display string, not a sentinel to branch on. Note that it also fixes
// the JSON type of a masked value to string, so adding a NUMERIC field to
// venuePrivateFields would flip that field's old_value/new_value type on masked
// rows; a consumer that types those values per field would have to account for
// it.
const RedactedValue = "(hidden)"

// venuePrivateFields names the revision fields that carry the unverified-venue
// address gate. It is the revision-history spelling of catalog.Venue's
// PublicAddress / PublicZipcode accessors, and the two must not drift: a field
// that is withheld from the live venue payload but published in that venue's
// edit history is not withheld at all.
//
// Capacity is deliberately absent, matching the live gate, which serves it for
// unverified venues because a room size does not locate anybody's house.
//
// # Where the two policies DIVERGE, on purpose (PSY-1717)
//
// The FIELD LIST is shared; the CALLER TIER is not. The live gate has no tier —
// PublicAddress returns nil for an unverified venue to everybody, admins
// included. Revision history serves an authenticated admin the unmasked view,
// because the History panel puts a Rollback button beside the diff and that
// button restores the real stored value: masking there hid what the moderation
// action used from the only person who could act on it.
//
// That asymmetry is a decision, not drift. THIS COMMENT IS THE ARGUMENT FOR IT;
// the two other places it could be "corrected" from —
// catalog.Venue.PublicAddress / PublicZipcode, and
// admin.RevisionService.applyPrivacyRedaction where the tier is applied — carry
// a pointer here and only the mechanics their own reader needs. Change the
// policy here first. Do not propagate the tier to the accessors, and do not
// delete it there to match them.
//
// Adding a field to this list therefore withholds it from anonymous and
// non-admin readers only; it is not a way to hide a value from an admin.
//
// The NAMES come from catalog.VenuePrivateFields, beside the accessors that do
// the withholding, so this file holds the read-time policy and not a second copy
// of the list it applies. A third gated column is one entry there, and
// validateVenuePrivateFields then refuses at startup if the name is not one a
// venue revision can carry.
var venuePrivateFields = func() map[string]struct{} {
	names := catalogm.VenuePrivateFields()
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}()

// RedactVenueChanges masks the address-family values in a diff recorded against
// an UNVERIFIED venue.
//
// Callers own the verification lookup; this function owns the field list. It
// returns a new slice and never mutates the input, because the input is
// unmarshalled from a stored row that other code paths (rollback) must still
// see raw.
func RedactVenueChanges(changes []adminm.FieldChange) []adminm.FieldChange {
	out := make([]adminm.FieldChange, len(changes))
	copy(out, changes)
	for i := range out {
		if _, private := venuePrivateFields[out[i].Field]; !private {
			continue
		}
		out[i].OldValue = RedactedValue
		out[i].NewValue = RedactedValue
	}
	return out
}

// validatePrivateFields fails startup if a masked field name is not one a venue
// revision can actually carry. A privacy gate that stops matching anything is
// indistinguishable from one that works, so the names are checked against BOTH
// vocabularies that can put a field into revisions.field_changes:
//
//   - VenueFields, used by Compare on the admin update path.
//   - catalog.VenueAllowedEditFields, forwarded verbatim by the pending-edit
//     approve path, which is the writer that actually records an unverified
//     venue's address. The admin path diffs two already-gated
//     VenueDetailResponse values, so for an unverified venue it emits
//     nil-to-nil and records nothing.
//
// Checking only the first would leave the leaking writer unguarded. The two
// lists are not identical — VenueAllowedEditFields carries names VenueFields
// does not — so a name must appear in both to be reliably maskable.
//
// Both vocabularies are parameters rather than package references so the
// asymmetry itself is testable: today every VenueFields name happens to be
// contributor-editable, which is exactly the condition the second check exists
// to notice stopping.
func validatePrivateFields(private map[string]struct{}, diffFields []Field, editable map[string]bool) error {
	diffed := make(map[string]struct{}, len(diffFields))
	for _, f := range diffFields {
		diffed[f.Name] = struct{}{}
	}
	for name := range private {
		if _, ok := diffed[name]; !ok {
			return fmt.Errorf("revisiondiff: venue private field %q is not in VenueFields", name)
		}
		if !editable[name] {
			return fmt.Errorf(
				"revisiondiff: venue private field %q is not in VenueAllowedEditFields, "+
					"the vocabulary the contributor edit path records", name)
		}
	}
	return nil
}

// validateVenuePrivateFields binds the check to the real vocabularies.
func validateVenuePrivateFields() error {
	return validatePrivateFields(venuePrivateFields, VenueFields, catalogm.VenueAllowedEditFields)
}
