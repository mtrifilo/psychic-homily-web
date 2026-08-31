// Package revisiondiff computes field-level diffs between two structs for
// revision history (PSY-563/759). It replaces the six near-identical
// compute*Changes functions that lived in handlers/catalog with a single
// reflection-driven comparator.
//
// Each entity declares an ordered list of Fields (output name + struct path).
// Compare walks that list, reads the named field from the before/after structs
// via reflection, and emits an adminm.FieldChange whenever the value differs.
// The emitted value type and comparison semantics are inferred from the
// field's static Go type so output stays byte-identical with the hand-written
// predecessors:
//
//   - string            → compare with ==, emit string
//   - *string           → deref or "" (matches the old ptrToStr helper)
//   - *float64          → emit float64, or nil when unset
//   - int               → compare with ==, emit int
//   - *int              → emit int, or nil when unset
//   - time.Time         → compare with Equal, emit RFC3339 string
//   - *time.Time        → compare with Equal, emit RFC3339 string or nil
//
// Every nullable kind except *string emits nil for unset, because Rollback
// feeds these values straight back into a GORM update map: the sentinel IS what
// the column is restored to. *string is the exception on its own merits, argued
// at diffPtr along with the rest of the rule.
//
// This was NOT always true, and the history is the argument for the rule.
// *time.Time got nil first because "" is not a valid TIMESTAMPTZ, so the zero
// sentinel made any revision touching doors_at unrollbackable — a loud failure.
// *float64 and *int carried the same defect silently until PSY-1960, since 0 is
// perfectly writable to those columns: the rollback succeeded and restored a
// number nobody had ever entered.
//
// NO BACKFILL of the rows written before that fix (decision recorded PSY-1960).
// A stored old_value of 0 is byte-identical whether it means "was unset" or
// "was genuinely free", so rewriting them would be guessing, and guessing wrong
// turns a recoverable wrong number into a destroyed record of a real $0 show.
// Rolling back a PRE-fix revision therefore still restores 0.
//
// What bounds the exposure is that a bad 0 is now CORRECTABLE -- but only on
// the two prices, and that limit is worth stating rather than implying. The
// show edit form can clear price/door_price back to NULL (PSY-1961), which it
// could not when this defect was found. venues.capacity, labels.founded_year
// and releases.release_year have no clear gesture on any surface: their update
// requests are still plain *int, so a pre-fix 0 on one of them stays until
// somebody writes SQL. Rarer (each needs an edit that first populated a NULL
// column, then a rollback of that edit) and less harmful (a wrong capacity is
// not a false price claim), which is why it is recorded here rather than
// blocking, but it is not zero.
//
// The per-entity field lists — not the contributor allowlist — are the source
// of truth for which fields appear in revision history. They intentionally
// diverge from models/admin allowlists (e.g. festival tracks admin-only
// start_date/status; show has no allowlist entry at all), so driving Compare
// off the allowlist would change the recorded diffs. Where a list happens to
// match an allowlist that is coincidence, not coupling.
//
// Field paths are validated against their struct once at package init via
// ValidateAll, so a renamed or mistyped field fails loudly at startup (and in
// tests) rather than silently dropping from every future revision row.
//
// # Privacy: what revision history may publish
//
// Revision history is read through PUBLIC endpoints
// (GET /revisions/{entity_type}/{entity_id}, GET /revisions/{revision_id},
// GET /users/{user_id}/revisions), so anything Compare records about an entity
// is world-readable for the life of the row unless a read-time gate says
// otherwise. A field that the live entity payload withholds must therefore be
// withheld here too, or the gate on the live payload is decorative: edit the
// field once and the value is published in the history instead. The two gates
// that exist are described below — field masking for unverified venues, and
// whole-entity suppression for non-approved shows.
//
// Those endpoints are optionally authenticated rather than strictly anonymous:
// they never require a credential, but they read one when it is offered, so an
// admin is served the unmasked view (PSY-1717). Caller tier is where this policy
// diverges from the live payload gate, which has none; privacy.go states the
// divergence in full. Everything below describes the PUBLIC tier — the view an
// anonymous or non-admin caller gets, which is unchanged by it.
//
// One such field family exists today. catalog.Venue.PublicAddress /
// PublicZipcode withhold an unverified venue's street address and zipcode,
// because an unverified venue is routinely a DIY show at somebody's home. The
// same rule, spelled in revision-field names, lives in privacy.go and is
// applied at READ time by admin.RevisionService — the stored row keeps the real
// values, so admin rollback still restores what was actually there.
//
// Read time, not write time, is deliberate: the gate depends on venues.verified,
// which changes after the revision is written. Masking at write time would
// permanently withhold the history of a venue that later gets verified, and
// would leave every already-stored row leaking.
//
// Every other entity's field list was audited against its live detail response
// at the commit that added this: artist, show, release, label and festival
// builders serve every field they record unconditionally, so no other field
// family needs masking. That sentence is a point-in-time reading, not a check —
// what keeps it true is the rule, which is that adding a FIELD-level gate to
// any live entity response means adding its revision-field name to privacy.go
// in the same change.
//
// Venue merge is the one case where the read-time gate cannot answer on its
// own. It re-points a losing venue's revisions onto the canonical row and then
// DELETES the loser, so a gate reading venues.verified for the current
// entity_id sees the canonical venue's state and republishes an unverified
// room's masked history. The merge therefore stamps the loser's rows with
// revisions.from_unverified_venue before re-pointing them, and
// applyPrivacyRedaction masks a stamped row regardless of the venue it now
// points at. This is a provenance marker, not a scrub: the stored
// diff still holds the real values, so rollback is unaffected, and the marker
// is set-only so a chain of merges cannot launder an address. Merges predating
// the column are not backfilled, because the evidence needed to do so was
// deleted with the losing venue rows.
//
// Every merge re-points its revisions through catalog.repointRevisions, which
// takes the provenance decision as a required argument. That is the choke
// point to change when a new entity type gains a read-time gate: the artist
// and show merges pass "nothing to carry" today, and it is a compile error to
// add a merge path that never answers the question.
//
// # Privacy: the revision summary
//
// revisions.summary is contributor-authored free text served beside the diff on
// the same public routes, so the natural summary for an address correction
// contains the address the diff beside it masks. On the trusted-tier auto-apply
// path no reviewer ever reads it before it is published. No field-name rule can
// reach prose.
//
// The policy is to withhold the summary WHOLE, at read time, on every revision
// whose subject is a gated venue, riding the SAME verdict as the field masking
// above rather than computing its own: both of applyPrivacyRedaction's masking
// conditions therefore reach prose too, so a row stamped
// from_unverified_venue by a merge loses its summary exactly as it loses its
// address. Two consequences are accepted deliberately:
//
//   - It costs real transparency. The summary is the only prose explaining WHY
//     an edit happened, and it disappears for exactly the venues most likely to
//     need community review. The alternative is publishing addresses, and an
//     unverified venue is routinely somebody's house.
//   - It is coarse. A summary that says nothing sensitive is withheld along
//     with one that does. Prose cannot be inspected for the value it might
//     paraphrase, so the gate keys off the SUBJECT rather than the content.
//
// Verifying a venue restores its summaries, the same way it restores its
// addresses, because nothing is scrubbed on write. Stored rows keep the real
// text: rollback needs it, and a future policy that can tell safe prose from
// unsafe would need it too.
//
// The complementary half is contributor-facing and lives in the frontend, on
// the edit drawer's summary field: whatever that copy says, it must not imply
// the audience is moderators. Withholding summaries on gated venues does not
// remove that obligation, because the field is still world-readable on every
// entity that is not a gated venue, which is nearly all of them.
//
// # Privacy: entities gated whole
//
// Everything above masks VALUES inside a revision that is still served. One
// entity is gated the other way, and reading the paragraphs above as covering it
// would be a mistake: shows are gated at the ENTITY level, so their revisions
// are not served at all rather than served with fields blanked.
//
// GET /shows/{id} answers 404 for a show whose status is pending, rejected or
// private unless the caller is an admin or the show's own submitter. Revision
// history mirrors that rule rather than approximating it (PSY-1715): the entity
// history route 404s, the single-revision route answers "not found", and the
// contributions listing omits the rows from the page AND from its total. The
// mechanism is admin/revision_visibility.go, which states the policy in full and
// is the file to read before changing it; the routes can see the caller at all
// because they sit on an optional-auth group (PSY-1717, routes/revisions.go).
//
// There is no masked variant because there is nothing coherent to serve: what
// the 404 withholds is the show's title, date, location, price and ticket url,
// which is most of what a revision records. Masking the values and publishing
// the row would still publish the FACT that an unpublished show exists and is
// being edited.
//
// This gate does not use privacy.go, RedactVenueChanges or redactVenueRevision.
// Those are the field-level mechanism and they stay venue-scoped. It DOES need
// the third mechanism, the provenance stamp, for the same reason the venue merge
// does and on a worse path: catalog.MergeDuplicateShow re-points a losing show's
// revisions and then deletes the show a read-time status lookup would have
// consulted, and it runs from the dedup CLI with no admin in the loop over a
// candidate set that includes private shows. The merge therefore stamps
// revisions.from_gated_show (catalog.reassignShowRevisions), and the read gate
// suppresses a stamped row whatever the show it now points at says.
//
// The stamp is coarser than the live gate it preserves: a stamped row is served
// to admins only, because the submitted_by that would grant the author access
// was deleted with the losing show.
//
// The rule this leaves for the next entity type: a live payload gate that hides
// FIELDS means adding the field names to privacy.go; a gate that hides the whole
// ENTITY means adding a case to admin/revision_visibility.go and a provenance
// value to catalog.repointRevisions. Both are additions in the same change as
// the gate, not follow-ups.
package revisiondiff

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
)

// Field maps an output field name (the snake_case name stored in
// revisions.field_changes) to a dot-separated path into the struct being
// diffed. Nested structs are addressed with dots, e.g. "Social.Instagram".
type Field struct {
	// Name is the value written to FieldChange.Field — the column-style name
	// the frontend renders and the allowlist keys on.
	Name string
	// Path is the dot-separated Go field path resolved by reflection,
	// e.g. "EventDate" or "Social.Instagram".
	Path string
}

var timeType = reflect.TypeOf(time.Time{})

// Compare returns the field-level diffs between before and after for the given
// ordered field list. before and after must be the same struct type (or
// pointer to it). Fields are emitted in list order, only when they differ.
//
// Compare panics if a path does not resolve to a supported field type — that
// is a programming error caught by ValidateAll at init and in tests, never a
// runtime data condition.
func Compare(before, after interface{}, fields []Field) []adminm.FieldChange {
	bv := derefStruct(before)
	av := derefStruct(after)

	var changes []adminm.FieldChange
	for _, f := range fields {
		bf := resolvePath(bv, f.Path)
		af := resolvePath(av, f.Path)

		oldVal, newVal, changed := diffValue(bf, af)
		if changed {
			changes = append(changes, adminm.FieldChange{
				Field:    f.Name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}
	return changes
}

// derefStruct returns the struct reflect.Value behind an interface that may be
// a struct or a pointer to one.
func derefStruct(v interface{}) reflect.Value {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}

// resolvePath walks a dot-separated field path from a struct value, descending
// through nested struct values (not pointers — the response structs nest
// SocialResponse by value).
func resolvePath(structVal reflect.Value, path string) reflect.Value {
	cur := structVal
	for _, part := range strings.Split(path, ".") {
		cur = cur.FieldByName(part)
	}
	return cur
}

// diffValue compares two reflect.Values of the same supported type and returns
// the old/new emit values plus whether they differ. The emit types match the
// original compute*Changes helpers exactly so JSONB output is byte-identical.
func diffValue(before, after reflect.Value) (oldVal, newVal interface{}, changed bool) {
	t := before.Type()

	// time.Time: compare with Equal, emit RFC3339 string (matches the old
	// `EventDate.Format(time.RFC3339)` calls).
	if t == timeType {
		bt := before.Interface().(time.Time)
		at := after.Interface().(time.Time)
		return bt.Format(time.RFC3339), at.Format(time.RFC3339), !bt.Equal(at)
	}

	switch t.Kind() {
	case reflect.String:
		b := before.String()
		a := after.String()
		return b, a, b != a

	case reflect.Int:
		b := int(before.Int())
		a := int(after.Int())
		return b, a, b != a

	case reflect.Ptr:
		return diffPtr(before, after, t.Elem())

	default:
		panic(fmt.Sprintf("revisiondiff: unsupported field kind %s", t.Kind()))
	}
}

// diffPtr handles the pointer field types the compute*Changes helpers used:
// *string (deref or ""), *time.Time, *float64 and *int.
//
// Every kind except *string distinguishes unset from its zero value and emits
// nil for unset. Rollback writes these values back into the column verbatim, so
// the emitted sentinel IS the value the column is restored to: a zero sentinel
// restores a number nobody chose, and the pointer kinds are pointers precisely
// because their columns are nullable.
//
// *time.Time was the first to need it — "" is not a valid TIMESTAMPTZ, so the
// zero sentinel made any revision that first populated doors_at permanently
// unrollbackable, a loud failure. *float64 and *int had the same defect quietly
// (PSY-1960): 0 IS writable to those columns, so the rollback succeeded and
// restored a fabricated zero. For a price that zero is a false PUBLIC claim, not
// merely a wrong number, because the ticket line renders 0 as "Free".
//
// Emitting nil is what makes an unset↔zero transition visible too. Flattened,
// both sides read 0 and the diff recorded NOTHING, so setting a show to free —
// or retracting that — left no history at all.
//
// *string keeps the nil-as-"" rule deliberately, and the exception is a claim
// about THE FIELDS IN fields.go, not about *string in the abstract. Every text
// field diffed today (image_url, ticket_url, description, address, zipcode,
// age_policy, the socials) has a column that takes the empty string and no
// reader who can tell NULL from "" — every render guard on them is a falsy
// check, so the two states are indistinguishable downstream. Changing it would
// rewrite the shape of every historical text diff to buy nothing.
//
// Note the rule is "indistinguishable", NOT "they already normalize to NULL":
// only image_url does that (utils.NilIfEmpty in showUpdatesToMap). The others
// really do store "".
//
// So the thing to check when ADDING a text field to fields.go is whether that
// list still holds for it. A column where NULL and "" differ — one under a
// unique index, or read by an `== nil` gate rather than a falsy one — would be
// flattened here silently, because a rollback writing "" over NULL fails
// nothing and looks like it worked.
func diffPtr(before, after reflect.Value, elem reflect.Type) (oldVal, newVal interface{}, changed bool) {
	if elem == timeType {
		b, bok := derefTime(before)
		a, aok := derefTime(after)
		return optionalTimeValue(b, bok), optionalTimeValue(a, aok), bok != aok || (bok && !b.Equal(a))
	}

	switch elem.Kind() {
	case reflect.String:
		b := derefString(before)
		a := derefString(after)
		return b, a, b != a

	case reflect.Float64:
		b, bok := derefFloat64(before)
		a, aok := derefFloat64(after)
		return optionalValue(b, bok), optionalValue(a, aok), bok != aok || (bok && b != a)

	case reflect.Int:
		b, bok := derefInt(before)
		a, aok := derefInt(after)
		return optionalValue(b, bok), optionalValue(a, aok), bok != aok || (bok && b != a)

	default:
		panic(fmt.Sprintf("revisiondiff: unsupported pointer element kind %s", elem.Kind()))
	}
}

// optionalValue returns nil for unset so Rollback restores SQL NULL rather than
// the type's zero value. The numeric counterpart of optionalTimeValue, and
// generic only because *float64 and *int need the identical treatment; the time
// case stays separate because it formats rather than passing the value through.
//
// The interface{} return type is load-bearing, exactly as it is there: a typed
// zero here is what publishes "DOOR Free" for a price nobody recorded.
func optionalValue[T float64 | int](v T, set bool) interface{} {
	if !set {
		return nil
	}
	return v
}

// derefTime reports the pointed-to instant and whether the pointer was set.
// The bool is what lets a set-to-unset transition register as a change without
// conflating "unset" with any particular instant.
func derefTime(p reflect.Value) (time.Time, bool) {
	if p.IsNil() {
		return time.Time{}, false
	}
	return p.Elem().Interface().(time.Time), true
}

// optionalTimeValue returns nil for unset so Rollback restores SQL NULL rather
// than trying to write a string into a TIMESTAMPTZ column. The interface{}
// return type is load-bearing: returning "" here is what made a doors_at
// revision unrollbackable.
func optionalTimeValue(t time.Time, set bool) interface{} {
	if !set {
		return nil
	}
	return t.Format(time.RFC3339)
}

func derefString(p reflect.Value) string {
	if p.IsNil() {
		return ""
	}
	return p.Elem().String()
}

// derefFloat64 and derefInt report the pointed-to number and whether the
// pointer was set, mirroring derefTime. The bool is what separates "unset" from
// a genuine zero, which for a price is the difference between silence and the
// word "Free".
func derefFloat64(p reflect.Value) (float64, bool) {
	if p.IsNil() {
		return 0, false
	}
	return p.Elem().Float(), true
}

func derefInt(p reflect.Value) (int, bool) {
	if p.IsNil() {
		return 0, false
	}
	return int(p.Elem().Int()), true
}
