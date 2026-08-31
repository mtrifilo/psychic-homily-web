package shared

import (
	"sort"
	"strings"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// THE POLYMORPHIC GATE — ONE REGISTRY, AND IT FAILS CLOSED
// =============================================================================
//
// A comment, a comment subscription, a last-read pointer and an entity-tag row
// all name their parent as an (entity_type, entity_id) pair, so the surfaces
// that read them cannot ask "may this viewer see this show" — they have to ask
// "may this viewer see whatever this row points at". This file is where that
// question is answered, once, for every spelling of it (PSY-1987).
//
// THE ROOT DEFECT THIS FILE EXISTS TO REMOVE was a default-open. The gate
// PSY-1939 introduced recognised `show` and waved everything else through,
// including `collection` — which has a real read-time rule of its own — so a
// logged-in stranger could subscribe to a guessed private collection id and
// watch it from the watching list, the notification inbox and the comment
// fan-out. The bug was not that the collection rule was written wrong. It is
// that nothing forced anybody to write it.
//
// So the shape here is a REGISTRY and a closed switch, not a special case:
//
//   - Every entity type that can reach these gates has an explicit entry below,
//     and an entity type with no entry is NOT VISIBLE. Refusing is recoverable;
//     publishing is not.
//   - `alwaysVisible` is a decision, spelled out per type with the reason. It is
//     not the absence of one, which is exactly what it replaces.
//   - TestEveryCommentEntityTypeHasAVisibilityRule fails for a type declared in
//     the models and missing here, so the eighth entity type cannot ship
//     un-dispositioned. The runtime fail-closed is the belt behind that brace,
//     for a type that reaches these gates without going through the model
//     constants at all — three of the four call-site families pass unvalidated
//     path text straight in.
//
// THE ADMIN TIER IS NOT UNIFORM ACROSS THE ARMS, and nothing here may add one.
// An admin sees every show and no private collection, because that is what the
// two detail routes do (see collection_visibility.go). A gate that unified the
// two would be more permissive than the route it mirrors on one arm or less on
// the other, and either way it would be a second rule.

// entityVisibilityRule names WHICH rule decides one entity type.
//
// A closed set, deliberately: adding a member forces every switch over it to be
// revisited, which is the compile-time half of what this file is for.
type entityVisibilityRule int

const (
	// ruleAlwaysVisible: this entity type has no read-time visibility rule
	// anywhere in the codebase, so every row naming it is answerable to every
	// caller. A CLAIM, checked against the model in the disposition test.
	ruleAlwaysVisible entityVisibilityRule = iota
	// ruleShow: decided by services/shared/show_visibility.go.
	ruleShow
	// ruleCollection: decided by services/shared/collection_visibility.go.
	ruleCollection
)

// entityVisibilityRules is the disposition of every polymorphic entity_type,
// keyed by the CANONICAL value the writers store.
//
// Keys come from the model constants rather than string literals because the
// comparison in Postgres is case-sensitive and these are the values the rows
// actually carry. The SQL allowlist below is derived from this map's keys, so a
// type added here reaches the statement without a second edit.
//
// The `alwaysVisible` entries each carry the reason they are safe, because
// "there is no rule for it" is the sentence that has to stop being true silently:
//
//   - artist, release: the model has no visibility, status or soft-delete
//     column at all. Every row is public.
//   - venue: `verified` gates the street ADDRESS at field level
//     (Venue.PublicAddress), not the row. The venue's name and existence are
//     public, and the fields this gate protects are name, slug and activity.
//   - label: `status` is active/inactive/defunct — whether the label still
//     operates, not who may read it. Every listing serves all three.
//   - festival: `status` is announced/confirmed/cancelled/completed — the
//     event's lifecycle, same reasoning.
var entityVisibilityRules = map[string]entityVisibilityRule{
	string(engagementm.CommentEntityArtist):   ruleAlwaysVisible,
	string(engagementm.CommentEntityVenue):    ruleAlwaysVisible,
	string(engagementm.CommentEntityRelease):  ruleAlwaysVisible,
	string(engagementm.CommentEntityLabel):    ruleAlwaysVisible,
	string(engagementm.CommentEntityFestival): ruleAlwaysVisible,

	CommentEntityTypeShow:       ruleShow,
	CommentEntityTypeCollection: ruleCollection,
}

// entityTypeAliases maps the plural spelling of each registered type to the
// canonical singular one, for the GO side only.
//
// Built from the registry rather than hand-listed so a new type gets its alias
// for free. The reason the plurals are recognised at all is the one PSY-1939
// gave for `shows`: an {entity_type} path segment is caller-supplied text, the
// codebase uses plurals for the same concept elsewhere
// (catalog.EntityExistenceService), and a gate that a spelling slips past is not
// a gate. Under a fail-closed default the cost of NOT recognising them changed
// sign — an unrecognised plural now refuses a legitimate read rather than waving
// a gated one through — which is a second, independent reason to keep them.
//
// The SQL side has no equivalent and must not grow one: it compares against
// STORED values, and the writers store the canonical spelling.
var entityTypeAliases = buildEntityTypeAliases()

func buildEntityTypeAliases() map[string]string {
	aliases := make(map[string]string, len(entityVisibilityRules))
	for entityType := range entityVisibilityRules {
		aliases[entityType+"s"] = entityType
	}
	return aliases
}

// entityVisibilityRuleFor resolves an {entity_type} segment to its rule.
//
// Case is folded and surrounding space trimmed for the reason PSY-1939 gave: a
// gate that `show` passes but `Show` slips through is not a gate. ok is false
// for anything not registered, and every caller treats that as "not visible".
func entityVisibilityRuleFor(entityType string) (entityVisibilityRule, bool) {
	normalized := strings.ToLower(strings.TrimSpace(entityType))
	if canonical, isAlias := entityTypeAliases[normalized]; isAlias {
		normalized = canonical
	}
	rule, ok := entityVisibilityRules[normalized]
	return rule, ok
}

// EntityVisibleTo reports whether viewer may see the entity a polymorphic row
// names, for ONE already-identified pair.
//
// The Go spelling, used by the handler boundary. An entity type with no
// registered rule answers FALSE, which is the change PSY-1987 exists to make.
//
// A nil checker answers false for the types that need one, and true for the
// types that do not: there is nothing for a missing gate to have decided about
// an artist. That keeps a construction bug failing closed exactly where it
// matters and stops it from taking six public surfaces down with it.
func EntityVisibleTo(checker contracts.ShowVisibilityInterface, entityType string, entityID uint, viewer contracts.ShowViewer) bool {
	rule, ok := entityVisibilityRuleFor(entityType)
	if !ok {
		return false
	}
	switch rule {
	case ruleAlwaysVisible:
		return true
	case ruleShow:
		return checker != nil && checker.ShowVisibleTo(entityID, viewer)
	case ruleCollection:
		return checker != nil && checker.CollectionVisibleTo(entityID, viewer)
	}
	// Unreachable while the switch covers the closed set above. Spelled out
	// anyway so that ADDING a member without extending the switch refuses rather
	// than publishes.
	return false
}

// VisibleCommentEntitySQL returns a condition, true for the rows of a
// POLYMORPHIC (entity_type, entity_id) table that viewer may see, plus its bind
// arguments.
//
// The SQL spelling of EntityVisibleTo, for the comment family's tables — the
// comments table, comment_subscriptions, and the notification rows resolved
// through a comment — where one column decides what kind of id sits beside it.
//
// Three parts, and the FIRST is the one PSY-1987 adds:
//
//   - an allowlist: a row whose entity_type is not registered above is not
//     visible. This is the fail-closed default in its SQL form, and it is what
//     stops an eighth entity type from being served by rows written before
//     anybody decided whether it should be.
//   - the show arm, unchanged.
//   - the collection arm.
//
// A row naming a show or a collection that no longer exists does NOT pass,
// because both EXISTS forms fail closed on a missing row — which is what lets a
// caller answer the same for a gated entity and a deleted one.
//
// THERE IS NO ADMIN SHORT-CIRCUIT, unlike the show-only predicate this replaces.
// An admin does not get to see private collections (collection_visibility.go),
// so returning a constant TRUE for them would open the collection arm. The show
// arm still short-circuits internally, so an admin pays for the collection probe
// and nothing else.
//
// Both expressions are SQL the CALLER controls and must be literals in the
// calling code. Nothing derived from a request may reach them.
func VisibleCommentEntitySQL(entityTypeExpr, entityIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	// The two per-type arms are the SAME functions the spellings-agree test
	// checks one by one, spliced rather than re-spelled. A composite that wrote
	// its own `<> 'show' OR EXISTS (…)` would be a third copy of the show rule,
	// which is the drift these files exist to remove.
	showArm, showArgs := VisibleShowCommentEntitySQL(entityTypeExpr, entityIDExpr, viewer)
	collectionArm, collectionArgs := VisibleCollectionCommentEntitySQL(entityTypeExpr, entityIDExpr, viewer)

	// Argument order follows the statement text: placeholders in a raw statement
	// bind by POSITION, and the show arm is written before the collection arm.
	args := make([]interface{}, 0, len(showArgs)+len(collectionArgs))
	args = append(args, showArgs...)
	args = append(args, collectionArgs...)

	return "(" + entityTypeExpr + " IN (" + registeredEntityTypeList + ")" +
		" AND " + showArm + " AND " + collectionArm + ")", args
}

// CommentEntityRecipientsSQL returns a condition, true for the rows whose
// recipientIDExpr names a user who may see the entity a comment hangs off, plus
// its bind arguments.
//
// The fan-out's spelling: the entity is fixed and known in Go, the viewers are a
// column. Its two arms delegate to the recipient forms in the show and
// collection files, which differ on the admin tier for the reasons each states.
//
// An UNREGISTERED entity type answers `1 = 0` — nobody is notified. That is the
// harshest fail-closed in this file and it is deliberate: this gate is the only
// one that is FINAL. A read-time gate that refuses wrongly is corrected by
// publishing the collection; a fan-out that declines to write mints nothing, and
// republication has nothing to restore. Withholding is the recoverable
// direction, so an undecided type withholds.
//
// recipientIDExpr and recipientIsAdminExpr are SQL the CALLER controls and must
// be literals in the calling code. recipientIsAdminExpr is used by the show arm
// only; pass "" where the query has no such column.
func CommentEntityRecipientsSQL(entityType string, entityID uint, recipientIDExpr, recipientIsAdminExpr string) (string, []interface{}) {
	rule, ok := entityVisibilityRuleFor(entityType)
	if !ok {
		return "1 = 0", nil
	}
	switch rule {
	case ruleAlwaysVisible:
		return "TRUE", nil
	case ruleShow:
		return VisibleShowRecipientsSQL(entityID, recipientIDExpr, recipientIsAdminExpr)
	case ruleCollection:
		return VisibleCollectionRecipientsSQL(entityID, recipientIDExpr)
	}
	return "1 = 0", nil
}

// EntityIdentityFenceSQL returns the predicate an ENRICHMENT pass must add when
// it resolves one entity type's name and slug out of that type's own table, plus
// its bind arguments, plus whether a predicate applies at all.
//
// The fence is not an alternative to the row gates the listings apply, and the
// two are not redundant. The row gates are what remove the SIGNAL: a
// de-identified row is still a row, and its position in a list is the disclosure
// restated. This is what stops the NEXT caller — a digest, an export, an admin
// view — from resolving a private entity's identity with nothing in the path to
// stop it.
//
// alias is the table alias the enclosing query uses and is a literal in the
// calling code. An unregistered entity type fences to FALSE rather than passing.
func EntityIdentityFenceSQL(entityType, alias string, viewer contracts.ShowViewer) (string, []interface{}, bool) {
	rule, ok := entityVisibilityRuleFor(entityType)
	if !ok {
		return "FALSE", nil, true
	}
	switch rule {
	case ruleAlwaysVisible:
		return "", nil, false
	case ruleShow:
		cond, args := VisibleShowPredicateSQL(alias, viewer)
		return cond, args, true
	case ruleCollection:
		cond, args := VisibleCollectionPredicateSQL(alias, viewer)
		return cond, args, true
	}
	return "FALSE", nil, true
}

// registeredEntityTypeList is the SQL IN-list of every entity_type with a
// recorded disposition, built once at init.
//
// Derived from the registry rather than written out, so a type added there
// reaches the allowlist without a second edit — the drift that produced this
// ticket. Sorted so the emitted statement is byte-identical across processes and
// builds: a query logged on one instance can be matched against another, and a
// change to the registry produces a reviewable diff.
//
// The values are package constants, never request data, which is what lets them
// be interpolated at all.
var registeredEntityTypeList = registeredEntityTypesSQL()

func registeredEntityTypesSQL() string {
	quoted := make([]string, 0, len(entityVisibilityRules))
	for entityType := range entityVisibilityRules {
		quoted = append(quoted, "'"+entityType+"'")
	}
	sort.Strings(quoted)
	return strings.Join(quoted, ", ")
}
