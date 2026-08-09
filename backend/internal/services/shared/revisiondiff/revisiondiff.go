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
//   - *float64          → deref or 0, emit float64
//   - int               → compare with ==, emit int
//   - *int              → deref or 0, emit int (both-nil counts as equal)
//   - time.Time         → compare with Equal, emit RFC3339 string
//   - *time.Time        → compare with Equal, emit RFC3339 string or nil
//
// The unset sentinel differs by type on purpose. The pre-existing pointer
// kinds emit a zero value ("" / 0) because that is what their hand-written
// predecessors did and their columns accept it. *time.Time emits nil instead:
// Rollback feeds these values straight back into a GORM update map, and ""
// is not a valid TIMESTAMPTZ, so a zero-value sentinel would make any
// revision touching a nullable timestamp permanently unrollbackable.
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
// Revision history is read through ANONYMOUS endpoints
// (GET /revisions/{entity_type}/{entity_id}, GET /revisions/{revision_id},
// GET /users/{user_id}/revisions), so anything Compare records about an entity
// is world-readable for the life of the row. A field that the live entity
// payload withholds must therefore be withheld here too, or the gate on the
// live payload is decorative: edit the field once and the value is published in
// the history instead.
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
// the same anonymous routes, so the natural summary for an address correction
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
// ONE KNOWN GAP remains that is not closed by a field list, and it should not be
// read as covered by the paragraphs above.
//
// Shows are gated at the ENTITY level rather than the field level.
// GET /shows/{id} 404s an anonymous caller for a show whose status is pending,
// rejected or private, while GET /revisions/show/{id} still publishes every
// recorded field, and its summary. Unpublishing a show hides the show but not
// its history.
//
// Closing it has to carry all three of the mechanisms above, because a show
// equivalent that copies only one of them reopens a hole that is already shut:
//
//   - The field masking, which is what privacy.go and RedactVenueChanges do.
//   - The prose withholding, which lives in redactVenueRevision. That function
//     is venue-named, so a show path has to withhold Summary itself.
//   - A provenance stamp. A read-time lookup on shows.status reproduces the
//     venue merge problem verbatim and on a worse path:
//     catalog.MergeDuplicateShow re-points revisions off a losing show and
//     deletes it, and it runs from the dedup CLI with no admin in the loop.
//     Whatever closes this has to stamp there the way the venue merge stamps
//     from_unverified_venue.
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
// *string (deref or ""), *float64 (deref or 0), *int (deref or 0). A nil
// pointer is treated as the zero value, matching ptrToStr / shared.Deref /
// intPtrVal so a nil↔value transition is a change and nil↔nil is not.
//
// *time.Time is the exception to the nil-as-zero rule: a set value emits the
// same RFC3339 string the non-pointer time.Time case emits, but unset emits
// nil, not "". Rollback writes these values back into the column verbatim, and
// "" is not a valid TIMESTAMPTZ; nil restores SQL NULL, which is the state the
// field actually had.
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
		b := derefFloat64(before)
		a := derefFloat64(after)
		return b, a, b != a

	case reflect.Int:
		b := derefInt(before)
		a := derefInt(after)
		return b, a, b != a

	default:
		panic(fmt.Sprintf("revisiondiff: unsupported pointer element kind %s", elem.Kind()))
	}
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

func derefFloat64(p reflect.Value) float64 {
	if p.IsNil() {
		return 0
	}
	return p.Elem().Float()
}

func derefInt(p reflect.Value) int {
	if p.IsNil() {
		return 0
	}
	return int(p.Elem().Int())
}
