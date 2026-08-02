package contracts

import (
	"slices"
	"strings"
)

// ──────────────────────────────────────────────
// Bill roles (show_artists.set_type)
// ──────────────────────────────────────────────

// The curated vocabulary for show_artists.set_type.
//
// set_type answers "what slot did this act play on this bill", which is a
// CURATED fact: somebody has to know it. It is therefore never inferred from
// list order. Only SetTypeHeadliner has ever been inferable (from position or
// the legacy is_headliner flag), and it is the only inference this package
// still sanctions.
//
// SetTypePerformer is the neutral default: "on the bill, slot unknown". It
// must stay semantically empty, because it is what every uncurated row holds.
// Writing a specific role (opener, direct support, ...) as a default would
// publish a guess as a fact, which is exactly the defect this vocabulary
// replaces.
const (
	SetTypeHeadliner     = "headliner"
	SetTypeDirectSupport = "direct_support"
	SetTypeOpener        = "opener"
	SetTypeDJ            = "dj"
	SetTypeSpecialGuest  = "special_guest"
	SetTypePerformer     = "performer"
)

// SetTypeDefault is the value written when nothing curated is known about an
// act's slot.
//
// Every writer in this codebase supplies it rather than leaving the column to
// chance, and the PSY-1673 migration normalizes the rows that predate that
// rule, so readers may treat set_type as always-present. Note the SCHEMA does
// not enforce it: the column is `VARCHAR(50) DEFAULT 'performer'` with no NOT
// NULL and no CHECK, so this is a convention the code keeps, not a guarantee
// the database makes. Adding those constraints would make it one.
const SetTypeDefault = SetTypePerformer

// setTypeVocabulary is ordered top-of-bill first, then by descending
// specificity, so the API docs, the error message, and the form selector all
// present the same order.
var setTypeVocabulary = []string{
	SetTypeHeadliner,
	SetTypeDirectSupport,
	SetTypeOpener,
	SetTypeSpecialGuest,
	SetTypeDJ,
	SetTypePerformer,
}

// SetTypeVocabulary returns the accepted set_type values in presentation
// order. Returns a copy so callers cannot mutate the vocabulary.
func SetTypeVocabulary() []string {
	return slices.Clone(setTypeVocabulary)
}

// SetTypeVocabularyCSV renders the vocabulary as a comma-separated list, for
// OpenAPI enum tags and validation messages.
func SetTypeVocabularyCSV() string {
	return strings.Join(setTypeVocabulary, ",")
}

// IsValidSetType reports whether value is exactly one of the curated set_type
// values. STRICT on purpose: this is the API contract check, so a client that
// sends "Headliner" or "support" gets a 422 naming the field rather than a
// silently coerced role. Lenient coercion of third-party vocabulary is
// NormalizeSetType's job, and only ingest may use it.
func IsValidSetType(value string) bool {
	return slices.Contains(setTypeVocabulary, value)
}

// NormalizeSetType maps a set_type as stated by an outside source (a venue
// calendar, an AI extraction of a flyer) onto the curated vocabulary. It is
// the INGEST-side counterpart to IsValidSetType and must not be used to
// validate API input.
//
// Returns "" for anything it cannot map with confidence, which callers must
// read as "the source said nothing about this slot" and answer with
// SetTypeDefault. Guessing here would reintroduce the very defect this
// vocabulary exists to remove: a role label nobody actually asserted.
//
// Mapping table, and why each row is honest:
//
//	headliner       -> headliner       identical term
//	direct_support  -> direct_support  identical term
//	support         -> direct_support  the extraction prompt emits "support"
//	                                   ONLY for an act the source billed under
//	                                   "w/" or "with", which is the direct
//	                                   support slot by definition
//	opener          -> opener          the source said "opener"; a stated
//	                                   opening slot is a real curated fact
//	special_guest   -> special_guest   identical term
//	dj              -> dj              now a first-class slot, no longer
//	                                   flattened into performer
//	performer       -> performer       identical term
//	host / mc       -> ""              hosting is a real role the vocabulary
//	                                   does not model; "performer" would be a
//	                                   lossy guess, so the caller defaults
//	anything else   -> ""              unknown means unknown
func NormalizeSetType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SetTypeHeadliner:
		return SetTypeHeadliner
	case SetTypeDirectSupport, "support":
		return SetTypeDirectSupport
	case SetTypeOpener:
		return SetTypeOpener
	case SetTypeSpecialGuest:
		return SetTypeSpecialGuest
	case SetTypeDJ:
		return SetTypeDJ
	case SetTypePerformer:
		return SetTypePerformer
	default:
		return ""
	}
}
