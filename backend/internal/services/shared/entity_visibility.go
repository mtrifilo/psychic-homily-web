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
// that read them cannot ask "may this viewer see this show". They have to ask
// "may this viewer see whatever this row points at". This file answers that
// question once, for every spelling of it.
//
// The shape is a REGISTRY and a closed switch, not a special case:
//
//   - Every entity type that can reach these gates has an explicit entry below,
//     and an entity type with no entry is NOT VISIBLE. Refusing is recoverable;
//     publishing is not.
//   - `ruleAlwaysVisible` is a decision, spelled out per type with the reason,
//     rather than the absence of one.
//   - TestEveryCommentEntityTypeHasAVisibilityRule fails for a type declared in
//     the models and missing here, so an entity type cannot ship
//     un-dispositioned. The runtime fail-closed is the belt behind that brace,
//     for a type that reaches these gates without going through the model
//     constants at all: three of the four call-site families pass unvalidated
//     path text straight in.
//
// THE ADMIN TIER IS NOT UNIFORM ACROSS THE ARMS, and nothing here may add one.
// An admin sees every show and no private collection, because that is what the
// two detail routes do (see collection_visibility.go). A gate that unified the
// two would be more permissive than the route it mirrors on one arm or less on
// the other, and either way it would be a second rule.

// entityVisibilityRule names WHICH rule decides one entity type.
//
// Go does not check a switch over this for exhaustiveness and this repo enables
// no linter that would (.golangci.yml), so adding a member is NOT caught by the
// compiler. Every switch below therefore ends in a refusal rather than a
// fall-through, and VisibleCommentEntitySQL derives its arms from the registry
// rather than naming them, so a rule added without its spellings withholds rows
// instead of publishing them. The loud half is
// TestOnlyShowsAndCollectionsAreGated, which fails until the new member is
// dispositioned by hand.
type entityVisibilityRule int

const (
	// ruleUnregistered is the ZERO VALUE and is never stored in the registry. A
	// bare map index on a type nobody registered yields this, and every switch
	// below refuses it, so the cheapest possible mistake stays the closed one.
	// The permissive rule must never be the value a missing key produces.
	ruleUnregistered entityVisibilityRule = iota
	// ruleAlwaysVisible: this entity type has no read-time visibility rule
	// anywhere in the codebase, so every row naming it is answerable to every
	// caller. A CLAIM, checked against the model in the disposition test.
	ruleAlwaysVisible
	// ruleShow: decided by services/shared/show_visibility.go.
	ruleShow
	// ruleCollection: decided by services/shared/collection_visibility.go.
	ruleCollection
)

// String names a rule so a failing disposition test reads as a rule name rather
// than as an integer whose meaning depends on the declaration order above.
func (r entityVisibilityRule) String() string {
	switch r {
	case ruleUnregistered:
		return "ruleUnregistered"
	case ruleAlwaysVisible:
		return "ruleAlwaysVisible"
	case ruleShow:
		return "ruleShow"
	case ruleCollection:
		return "ruleCollection"
	}
	return "unknown entityVisibilityRule"
}

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

// entityVisibilityRuleFor resolves an {entity_type} segment to its rule.
//
// EXACT MATCH against the canonical spelling, with no case folding and no plural
// fallback, because this gate has to agree with the service vocabulary that runs
// behind it. engagement.validateCommentEntityType and
// catalogm.IsValidTagEntityType both compare exactly, so a gate that accepted
// more spellings than they do would answer two different refusals for the same
// id depending on how the caller spelled the type, and the pair of answers is
// itself a disclosure. One canonical vocabulary, one answer.
//
// It is also what the SQL spellings do: they compare against STORED values, and
// the writers store the canonical spelling. A Go side that normalised and a SQL
// side that did not would be two rules.
//
// ok is false for anything not registered, and every caller treats that as "not
// visible".
func entityVisibilityRuleFor(entityType string) (entityVisibilityRule, bool) {
	rule, ok := entityVisibilityRules[entityType]
	return rule, ok
}

// EntityVisibleTo reports whether viewer may see the entity a polymorphic row
// names, for ONE already-identified pair.
//
// The Go spelling, used by the handler boundary. An entity type with no
// registered rule answers FALSE.
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
// Two parts, and BOTH are derived from the registry rather than written out:
//
//   - an ALLOWLIST: a row whose entity_type is not registered is not visible.
//     This is the fail-closed default in its SQL form, and it is what stops an
//     eighth entity type from being served by rows written before anybody
//     decided whether it should be.
//   - one ARM PER GATED TYPE, walked in sorted order off the registry. Naming
//     the arms here instead would reopen the defect one level up: adding
//     `"radio_show": ruleRadioShow` puts that value into the allowlist for free
//     while no arm judges it, so the SQL would serve rows the Go gate refuses —
//     the two spellings disagreeing, in the leaking direction. Derived, a rule
//     with no arm yet REFUSES every row of its type instead.
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
	// SQLQuotedList's contract: empty means empty, and the caller must not emit a
	// placeholder for it. `IN ()` is a syntax error, so an empty registry answers
	// the refusal the allowlist would have meant rather than a broken statement.
	// Unreachable while the registry has entries; spelled out because the failure
	// it prevents is a 500 on every surface at once.
	if registeredEntityTypeList == "" {
		return "1 = 0", nil
	}
	conds := []string{entityTypeExpr + " IN (" + registeredEntityTypeList + ")"}
	var args []interface{}
	// SORTED, so the emitted statement is byte-identical across processes and
	// builds — and so the bind order is a property of the registry rather than of
	// Go's map iteration, which is randomised. Placeholders in a raw statement
	// bind by POSITION, so an order that varied per call would bind a viewer id
	// into the wrong arm.
	for _, entityType := range gatedEntityTypes() {
		arm, armArgs := commentEntityArmFor(entityType, entityTypeExpr, entityIDExpr, viewer)
		conds = append(conds, arm)
		args = append(args, armArgs...)
	}
	return "(" + strings.Join(conds, " AND ") + ")", args
}

// commentEntityArmFor returns the polymorphic arm that judges ONE gated entity
// type, plus its bind arguments.
//
// The per-type spellings are the SAME functions the spellings-agree tests check
// one by one, spliced rather than re-written: a composite that wrote its own
// `<> 'show' OR EXISTS (…)` would be a third copy of the show rule, which is the
// drift these files exist to remove.
//
// A gated rule with no arm here EXCLUDES every row of its type. That is the
// fail-closed answer to a half-finished rule: withholding is recoverable and a
// disposition test says so loudly, while passing the rows through would restore
// the default-open for whoever adds the next type.
func commentEntityArmFor(entityType, entityTypeExpr, entityIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	switch entityVisibilityRules[entityType] {
	case ruleShow:
		return VisibleShowCommentEntitySQL(entityTypeExpr, entityIDExpr, viewer)
	case ruleCollection:
		return VisibleCollectionCommentEntitySQL(entityTypeExpr, entityIDExpr, viewer)
	}
	return "(" + entityTypeExpr + " <> '" + entityType + "')", nil
}

// PublicEntityTypeArmsSQL returns VisibleCommentEntitySQL's condition for an
// ANONYMOUS reader, inlined so it binds nothing.
//
// For a query that cannot carry bind arguments to this position: a subquery
// assembled by string concatenation whose placeholders are counted elsewhere,
// where one more `?` would shift every argument after it. It is the same
// allowlist and the same per-type arms, with each gated type's PUBLIC spelling
// spliced in place of its viewer-bound one.
//
// PUBLIC TIER, and that is the whole contract: it answers for a caller with no
// credential, so a surface that used it for an authenticated reader would
// withhold that reader's own rows. Every caller here wants exactly that — a
// shared number that must not vary by credential.
//
// Derived from the registry, like its bound twin, so a type added there reaches
// this statement without a second edit, and a gated type with no public arm
// EXCLUDES its rows rather than passing them.
//
// Both expressions are SQL the CALLER controls and must be literals in the
// calling code.
func PublicEntityTypeArmsSQL(entityTypeExpr, entityIDExpr string) string {
	if registeredEntityTypeList == "" {
		return "1 = 0"
	}
	conds := []string{entityTypeExpr + " IN (" + registeredEntityTypeList + ")"}
	for _, entityType := range gatedEntityTypes() {
		conds = append(conds, publicEntityArmFor(entityType, entityTypeExpr, entityIDExpr))
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}

// publicEntityArmFor returns the ANONYMOUS-reader arm that judges ONE gated
// entity type, binding nothing.
//
// commentEntityArmFor's public twin, and it fails closed on the same terms: a
// gated rule with no arm here EXCLUDES every row of its type rather than
// passing it, so a half-added rule withholds.
func publicEntityArmFor(entityType, entityTypeExpr, entityIDExpr string) string {
	switch entityVisibilityRules[entityType] {
	case ruleShow:
		return commentEntityArmSQL(entityTypeExpr, CommentEntityTypeShow,
			showExistsSQL(entityIDExpr, PublicShowPredicateSQL(visibleShowsAlias)))
	case ruleCollection:
		// The anonymous collection predicate binds nothing already: with no
		// viewer id there is no creator branch, so it collapses to
		// `is_public = TRUE`. Read from the same function the bound form uses
		// rather than written out, so the two cannot drift.
		inner, args := VisibleCollectionPredicateSQL(visibleCollectionsAlias, contracts.ShowViewer{})
		if len(args) != 0 {
			// Unreachable while the anonymous branch stays bind-free. Spelled
			// out because the failure it prevents is a statement whose
			// placeholders no longer match its arguments, which is a 500 on
			// every surface using it at once.
			return "(" + entityTypeExpr + " <> '" + CommentEntityTypeCollection + "')"
		}
		return commentEntityArmSQL(entityTypeExpr, CommentEntityTypeCollection,
			collectionExistsSQL(entityIDExpr, inner))
	}
	return "(" + entityTypeExpr + " <> '" + entityType + "')"
}

// gatedEntityTypes is the sorted set of registered types that are NOT
// alwaysVisible — the ones VisibleCommentEntitySQL needs an arm for.
//
// Computed per call rather than at init because it is a scan of a handful of
// entries behind a database round trip, and a package var would be a second thing
// to keep in step with the registry.
func gatedEntityTypes() []string {
	gated := make([]string, 0, len(entityVisibilityRules))
	for entityType, rule := range entityVisibilityRules {
		if rule != ruleAlwaysVisible {
			gated = append(gated, entityType)
		}
	}
	sort.Strings(gated)
	return gated
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
// its bind arguments.
//
// The fence is not an alternative to the row gates the listings apply, and the
// two are not redundant. The row gates are what remove the SIGNAL: a
// de-identified row is still a row, and its position in a list is the disclosure
// restated. This is what stops the NEXT caller — a digest, an export, an admin
// view — from resolving a private entity's identity with nothing in the path to
// stop it.
//
// ALWAYS RETURNS A CONDITION, so the caller splices it in unconditionally and
// cannot forget it: `TRUE` where no rule applies, `FALSE` for an unregistered
// type. That is VisibleShowPredicateSQL's convention for its admin tier and the
// reasoning is the same one — an omitted clause is a branch at every call site,
// and a branch on a security boundary is a branch somebody drops.
//
// alias is the table alias the enclosing query uses and is a literal in the
// calling code.
func EntityIdentityFenceSQL(entityType, alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	rule, ok := entityVisibilityRuleFor(entityType)
	if !ok {
		return "FALSE", nil
	}
	switch rule {
	case ruleAlwaysVisible:
		return "TRUE", nil
	case ruleShow:
		return VisibleShowPredicateSQL(alias, viewer)
	case ruleCollection:
		return VisibleCollectionPredicateSQL(alias, viewer)
	}
	return "FALSE", nil
}

// =============================================================================
// THE TWO SQL SKELETONS EVERY PER-TYPE RULE IS BUILT FROM
// =============================================================================
//
// Both gated rules assemble the same two shapes, and they live here once because
// the invariants that make them safe are shared. Two hand-maintained copies is
// two places each invariant has to keep holding.

// entityExistsSQL wraps a table condition in a correlated EXISTS.
//
// The shape every "this row names an id in some other table" gate uses: one
// index probe on that table's primary key per row considered, not a join that
// multiplies rows. It fails closed on a missing row by construction, which is
// what lets a caller answer the same for a gated entity and a deleted one.
//
// alias MUST be a name no enclosing query uses. An alias declared in this
// subquery SHADOWS an outer one of the same name, so an idExpr qualified with a
// colliding alias would self-correlate — `x.id = x.id` — and the EXISTS would be
// true whenever any visible row exists at all, opening the gate completely. Each
// rule owns its own alias constant for that reason, and callers never pass an
// expression qualified with it.
//
// table, alias, idExpr and cond are all SQL the CALLER controls and must be
// literals in the calling code.
func entityExistsSQL(table, alias, idExpr, cond string) string {
	return "EXISTS (SELECT 1 FROM " + table + " " + alias +
		" WHERE " + alias + ".id = " + idExpr +
		" AND " + cond + ")"
}

// commentEntityArmSQL builds "this row is not of the type I judge, OR it passes
// visible".
//
// The polymorphic arm shape. It reads entity_type before it trusts entity_id,
// which is the whole point: entity_id means a DIFFERENT THING per entity_type,
// so an arm that skipped the type test would decide a row by whatever unrelated
// record happened to share that number.
//
// entityType is a package constant, never request data, which is what lets it be
// interpolated rather than bound.
func commentEntityArmSQL(entityTypeExpr, entityType, visible string) string {
	return "(" + entityTypeExpr + " <> '" + entityType + "' OR " + visible + ")"
}

// registeredEntityTypeList is the SQL IN-list of every entity_type with a
// recorded disposition, built once at init.
//
// Derived from the registry rather than written out, so a type added there
// reaches the allowlist without a second edit — the drift that produced this
// ticket.
var registeredEntityTypeList = SQLQuotedList(registeredEntityTypes())

func registeredEntityTypes() []string {
	types := make([]string, 0, len(entityVisibilityRules))
	for entityType := range entityVisibilityRules {
		types = append(types, entityType)
	}
	return types
}

// SQLQuotedList renders entity-type constants as a sorted, quoted SQL IN-list.
//
// Shared by every gate predicate that decides rows by entity_type — this
// package's allowlist and the notification inbox's two arms — because they are
// one encoder and a second copy is one place a change to quoting or to the empty
// case would not reach.
//
// SORTED so the emitted statement is byte-identical across processes and builds:
// a query logged on one instance can be matched against another, and a change to
// the underlying set produces a reviewable diff. Within a single process the
// string is already stable, since every caller computes it once at init.
//
// EMPTY MEANS EMPTY. Callers must handle that themselves and must NOT emit a
// placeholder for it: `IN ()` is a syntax error and `NOT IN (NULL)` is never
// true, which does not disable a type test — it applies the gate beside it to
// every row regardless of type. See notification.entityTypeArm, which turns the
// empty string into a no-op arm and drops that arm's bind arguments with it.
//
// The values are package constants, never request data, which is what lets them
// be interpolated at all.
func SQLQuotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, "'"+v+"'")
	}
	sort.Strings(quoted)
	return strings.Join(quoted, ", ")
}
