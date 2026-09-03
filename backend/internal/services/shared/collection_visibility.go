package shared

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// COLLECTION VISIBILITY — ONE RULE, SEVERAL SPELLINGS
// =============================================================================
//
// GET /collections/{slug} refuses a collection whose is_public is false unless
// the caller is the creator (services/community.CollectionService.GetBySlug).
// That is the rule. This file is its single definition for every surface that
// decides a collection AGAINST A SPECIFIC VIEWER, and it is the counterpart of
// show_visibility.go: read that file first, because the design notes there about
// aliases, parenthesisation, caller-controlled SQL and the no-enumeration-oracle
// obligation all apply here unchanged.
//
// WHAT IT IS NOT THE SINGLE DEFINITION OF: the PUBLIC-TIER sites, which decide a
// collection the same way for everybody and spell `is_public = true` by hand.
// catalog/tag_service.go's enrichCollections, catalog/scene_collections.go,
// catalog/tag_intersection.go and catalog/charts_featured_collection.go each
// carry their own literal, and so do community/collection.go's ListCollections
// (under its PublicOnly filter) and GetUserPublicCollections, whose whole
// contract is the public tier. The `!IsPublic && CreatorID != userID` tests
// inside services/community/collection.go are the Go spelling of the same rule
// at the route boundary, where there is no query to splice a predicate into.
// Migrating the literals onto the spellings here is a separate change: it is a
// refactor of routes this file does not otherwise touch, and doing it under a
// privacy fix would put untested churn beside the gates.
//
// TWO WAYS THIS RULE DIFFERS FROM THE SHOW RULE, and both are load-bearing:
//
//  1. THERE IS NO ADMIN TIER. Every show spelling short-circuits to TRUE for an
//     admin. No collection spelling does, because no collection DETAIL OR
//     LISTING read grants one: GetBySlug, GetByID, GetCollectionGraph,
//     CloneCollection, Subscribe, Like and Unlike all test `!IsPublic &&
//     CreatorID != viewerID` with no isAdmin term, so an admin who is not the
//     creator is refused a private collection. A gate more permissive than the
//     route it mirrors is a second rule, and the second rule is the leak. So
//     `viewer.IsAdmin` is deliberately UNREAD by everything here;
//     TestCollectionVisibilitySpellingsAgree pins that with a real admin row.
//
//     THE SURFACES THAT DO SERVE AN ADMIN A PRIVATE COLLECTION are exceptions
//     with a reason rather than counter-examples, and each names its own reason
//     at its own definition:
//     GET /admin/comments/pending serves the moderation queue every pending
//     comment's body and (entity_type, entity_id) regardless of the parent's
//     visibility; GET /admin/entity-reports and its detail route resolve a
//     reported collection's title and slug the same way; and
//     PUT /collections/{slug} and DELETE /collections/{slug} take `isAdmin`, so
//     an admin can flip a reported collection public or remove it. Those two are
//     the remedies the report queue exists to reach: a queue that showed an
//     admin the report while every remedy refused them is a queue they cannot
//     act on. They are moderation powers on moderated routes.
//     WHAT PUT DOES NOT DO IS LET AN ADMIN READ ONE. It applies the update and
//     then reads back with GetBySlug against the CALLER, so an admin who edits a
//     private collection without publishing it gets the write applied and 204:
//     committed, and nothing they may see. Only the is_public flip returns a
//     body, because the flip is what makes the collection readable. That is the
//     remedy working and the read staying refused, not a half-finished exception.
//     PUT /collections/{slug}/feature reaches one too, and it is listed here
//     because the code does not test visibility rather than because it needs to:
//     it publishes nothing (empty response, and both featured-collection chart
//     surfaces filter `is_public = true`), so what it leaves is a 200-vs-404 slug
//     oracle bounded to the admin group. Whether an admin may pre-feature an
//     unpublished collection is a product question, and the suite encodes today
//     that they may.
//     The rule this file states is about the tiers a NON-admin surface answers
//     for, and adding an admin tier here would extend those powers to the
//     watching list, the inbox, the item writes and the comment fan-out, which is
//     not what moderation trust was granted for. Every collection write outside
//     the routes named above refuses an admin a private collection.
//
//     This propagates. VisibleCommentEntitySQL cannot take the admin
//     short-circuit its show-only predecessor took, and the notification
//     inbox's predicate cannot answer `1 = 1` for an admin, because both decide
//     collection-typed rows as well.
//
//  2. THE STATE IS ONE BOOLEAN, not a four-value status enum, and it is NOT
//     NULL with a default of true. `is_public = TRUE` is therefore written as a
//     SQL keyword rather than bound, so the only bound value in any spelling
//     here is the viewer's own id. Nothing derived from a request is ever
//     interpolated.
//
// CREATOR-ONLY, NOT CONTRIBUTOR-ONLY. `collaborative` opens a collection's item
// and tag writes to any authenticated caller, and it is the model default, so
// every write path tests VISIBILITY before it considers `collaborative`: a
// caller who may not read the collection may not write to it either. That is the
// detail route's answer rather than a narrowing invented here, and it is what
// stops a bulk add from reporting a per-row duplicate for each entity already in
// a collection the caller cannot see.
//
// The consequence is that a contributor to a PRIVATE collaborative collection
// can do nothing with it at all, which is consistent (GetBySlug already refuses
// them) but is probably not what "private plus collaborative" is meant to mean.
// Widening the gate to contributors would make these surfaces more permissive
// than the route they mirror, so that is a product question rather than a code
// one.
//
// EVERY spelling fails closed, on the same terms show_visibility.go states: a
// missing collection row, a nil db, a zero id and a failed lookup all resolve to
// "not visible". A collection that was deleted and one that is private must
// answer alike, or the pair is an enumeration oracle over a dense id space.
//
// Nothing here scrubs anything. Flipping is_public back to true restores every
// gated sub-resource to everyone, because the gate is evaluated at read time
// against the collection's current flag. The single exception is the fan-out,
// for the reason VisibleCollectionRecipientsSQL gives.

// visibleCollectionsAlias is the alias VisibleCollectionExistsSQL binds the
// collections table to.
//
// Deliberately not `c` or `collections`, for the reason visibleShowsAlias gives
// at length: an alias declared in this subquery SHADOWS an outer one of the same
// name, so a collectionIDExpr qualified with a colliding alias would
// self-correlate and the EXISTS would be true whenever any public collection
// exists at all. Callers must never pass an expression qualified with this
// alias.
const visibleCollectionsAlias = "visible_collection"

// CommentEntityTypeCollection is the polymorphic entity_type value the comment
// family stores a collection under — comments, comment subscriptions, last-read
// pointers.
//
// Read from the model rather than written out, because the rows this gate
// decides about are written from that constant and the comparison in Postgres is
// case-sensitive.
const CommentEntityTypeCollection = string(engagementm.CommentEntityCollection)

// CollectionVisibleTo reports whether viewer may see collection collectionID at
// all.
//
// The Go spelling of the detail route's predicate. Returns false when the
// collection does not exist, which is the answer GetBySlug gives by returning a
// not-found error, and false on any lookup failure.
//
// viewer.IsAdmin is not read. See this file's header.
func CollectionVisibleTo(db *gorm.DB, collectionID uint, viewer contracts.ShowViewer) bool {
	if db == nil || collectionID == 0 {
		return false
	}

	// One condition carrying its own parentheses, written out here rather than
	// left to the query builder: the binding this pins is `id = X AND (is_public
	// = TRUE OR creator_id = Y)`, never `id = X AND is_public = TRUE OR
	// creator_id = Y`, which would answer yes for a private collection whenever
	// the caller created ANY collection at all. A deliberate refusal to let a
	// security boundary rest on framework behaviour, not a workaround for a bug.
	// The show rule closed the same hazard by removing the OR entirely; this one
	// still has it.
	q := db.Model(&communitym.Collection{})
	if viewer.UserID != 0 {
		q = q.Where("id = ? AND (is_public = TRUE OR creator_id = ?)", collectionID, viewer.UserID)
	} else {
		q = q.Where("id = ? AND is_public = TRUE", collectionID)
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		logger.Default().Error("collection_visibility_lookup_failed",
			"collection_id", collectionID,
			"error", err.Error(),
		)
		return false
	}
	return count > 0
}

// VisibleCollectionPredicateSQL returns a SQL condition, true for the rows of a
// collections table alias that viewer may see, plus its bind arguments.
//
// For a query already selecting FROM collections.
//
// Unlike its show twin there is no admin branch and so no constant-TRUE tier:
// every caller gets the real predicate. alias is a SQL identifier the CALLER
// controls and must be a literal in the calling code.
func VisibleCollectionPredicateSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	// Built by concatenation rather than written whole because the creator
	// branch must disappear for an anonymous caller instead of comparing against
	// user id 0. The outer parentheses are written here, not left to the query
	// builder.
	cond := "(" + alias + ".is_public = TRUE"
	var args []interface{}
	if viewer.UserID != 0 {
		cond += " OR " + alias + ".creator_id = ?"
		args = append(args, viewer.UserID)
	}
	cond += ")"
	return cond, args
}

// VisibleCollectionExistsSQL returns a correlated EXISTS condition, true when
// the collection named by collectionIDExpr is one viewer may see, plus its bind
// arguments.
//
// For a query that holds a collection id in some other table's column — a
// comment's entity_id, a subscription's entity_id. One index probe on the
// collections primary key per row considered, not a join that multiplies rows.
//
// A row whose collectionIDExpr matches no collection is NOT visible, which is
// what makes this usable as the gate: it answers the same for a private
// collection and a deleted one. Collections are hard-deleted, so that pair is
// reachable in practice.
//
// collectionIDExpr is SQL the CALLER controls and must be a literal in the
// calling code. Nothing derived from a request may reach it.
func VisibleCollectionExistsSQL(collectionIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	inner, args := VisibleCollectionPredicateSQL(visibleCollectionsAlias, viewer)
	return collectionExistsSQL(collectionIDExpr, inner), args
}

// visibleCollectionItemsAlias is the alias VisibleCollectionItemExistsSQL binds
// the collection_items table to. Distinct from every alias an enclosing query is
// likely to use, for the reason visibleCollectionsAlias gives.
const visibleCollectionItemsAlias = "visible_collection_item"

// VisibleCollectionItemExistsSQL returns a correlated EXISTS condition, true
// when the collection ITEM named by itemIDExpr belongs to a collection viewer
// may see, plus its bind arguments.
//
// For a query holding a collection_items id rather than a collections id. The
// audit writers store both under entity_type "collection", so a caller deciding
// audit rows needs one spelling per kind of id it can be holding, and picking
// the wrong one judges a row against an unrelated record that happens to share
// the number.
//
// An item that no longer exists is NOT visible, which is the same fail-closed
// answer every spelling here gives. collection_items are hard-deleted, so a row
// naming a removed item resolves to nothing and is withheld.
//
// itemIDExpr is SQL the CALLER controls and must be a literal in the calling
// code.
func VisibleCollectionItemExistsSQL(itemIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	inner, args := VisibleCollectionPredicateSQL(visibleCollectionsAlias, viewer)
	return "EXISTS (SELECT 1 FROM collection_items " + visibleCollectionItemsAlias +
		" JOIN collections " + visibleCollectionsAlias +
		" ON " + visibleCollectionsAlias + ".id = " + visibleCollectionItemsAlias + ".collection_id" +
		" WHERE " + visibleCollectionItemsAlias + ".id = " + itemIDExpr +
		" AND " + inner + ")", args
}

// visibleCollectionTextIDAlias is the alias VisibleCollectionTextIDExistsSQL
// binds the collections table to. Distinct from every other alias here, for the
// reason visibleCollectionsAlias gives.
const visibleCollectionTextIDAlias = "visible_collection_text_id"

// VisibleCollectionTextIDExistsSQL returns a correlated EXISTS condition, true
// when the collection whose id is written as TEXT in idExpr is one viewer may
// see, plus its bind arguments.
//
// For a caller holding a collection id inside a JSON document rather than in a
// typed column: an audit row's metadata records the parent a polymorphic
// entity_id cannot name.
//
// THE CAST IS ON THE JSON SIDE, guarded by a digits-only regex, so the
// comparison is `collections.id = <bigint>` and probes the primary key. Casting
// the COLUMN instead (`id::text = idExpr`) is not sargable: it evaluates per row
// of collections, inside a CASE, in both the count and the page query of an
// anonymous route. The guard is what makes the cast total — a value that is not
// a run of digits yields NULL, the comparison is NULL, and the EXISTS is false
// rather than the statement raising. The guard caps the run at 18 digits, which
// is inside bigint's range on every input, so no value can overflow the cast
// either.
//
// A SLUG WOULD NOT DO. Renaming a collection regenerates its slug and deleting
// one frees the string, so a later collection can take that name and a slug
// match would republish the original's rows. Ids come from a sequence and are
// never reissued.
//
// A row whose idExpr matches no collection is NOT visible, so a deleted parent
// answers the same as a private one.
//
// idExpr is SQL the CALLER controls and must be a literal in the calling code.
func VisibleCollectionTextIDExistsSQL(idExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	inner, args := VisibleCollectionPredicateSQL(visibleCollectionTextIDAlias, viewer)
	return "EXISTS (SELECT 1 FROM collections " + visibleCollectionTextIDAlias +
		" WHERE " + visibleCollectionTextIDAlias + ".id = " + textIDAsBigintSQL(idExpr) +
		" AND " + inner + ")", args
}

// textIDAsBigintSQL renders a TEXT expression as a bigint, or as NULL when it is
// not a bounded run of digits.
//
// The digit guard is what makes the cast total: `NULL` compares unequal to every
// id, so a non-numeric value answers "no such collection" instead of raising and
// taking down the statement it sits in. 18 digits is inside bigint's range for
// every input, so no value can overflow the cast either.
func textIDAsBigintSQL(idExpr string) string {
	return "(CASE WHEN " + idExpr + " ~ '^[0-9]{1,18}$' THEN (" + idExpr + ")::bigint END)"
}

// visibleCollectionSlugAlias is the alias VisibleCollectionSlugExistsSQL binds
// the collections table to. Distinct from every other alias here, for the reason
// visibleCollectionsAlias gives.
const visibleCollectionSlugAlias = "visible_collection_slug"

// VisibleCollectionSlugExistsSQL returns a correlated EXISTS condition, true
// when the collection NAMED BY SLUG in slugExpr is one viewer may see, plus its
// bind arguments.
//
// FOR FROZEN ROWS THAT RECORD NOTHING ELSE, and for nothing else. A slug is a
// weaker reference than an id: renaming a collection regenerates its slug and
// deleting one frees the string, so a later collection can take that name and
// this condition would then decide the row against a collection that has no
// relation to it. Every spelling that has an id available must use the id, and
// the id forms above are what the writers feed today.
//
// The cases that have no id are audit rows written before the writers recorded
// one, and rows that recorded the 0 sentinel, which names no collection. Such a
// row carries the slug alone, its entity_id names a hard-deleted
// collection_items row, and judging it by id therefore withholds it from
// everyone including its own author on a public collection. A slug match is the
// only reference it still has.
//
// THE SET IS CLOSED. Every item writer now takes the parent id from the service
// that authorised the write, which loaded the parent row to authorise it, so a
// successful write cannot omit the key or stamp a zero. What this condition
// decides is the rows that already exist, and it gains no new ones.
//
// A row whose slugExpr matches no collection is NOT visible, which is the same
// fail-closed answer every spelling here gives.
//
// slugExpr is SQL the CALLER controls and must be a literal in the calling code.
func VisibleCollectionSlugExistsSQL(slugExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	inner, args := VisibleCollectionPredicateSQL(visibleCollectionSlugAlias, viewer)
	return "EXISTS (SELECT 1 FROM collections " + visibleCollectionSlugAlias +
		" WHERE " + visibleCollectionSlugAlias + ".slug = " + slugExpr +
		" AND " + inner + ")", args
}

// collectionExistsSQL wraps a collections-table condition in the correlated
// EXISTS every spelling uses. Named here so the viewer-facing and
// recipient-facing forms cannot correlate on different columns, and delegating
// to entityExistsSQL so this rule and the shows rule cannot correlate on
// different SHAPES.
func collectionExistsSQL(collectionIDExpr, collectionCond string) string {
	return entityExistsSQL("collections", visibleCollectionsAlias, collectionIDExpr, collectionCond)
}

// VisibleCollectionCommentEntitySQL returns a condition, true for the rows of a
// POLYMORPHIC (entity_type, entity_id) table whose COLLECTION reference viewer
// may see, plus its bind arguments.
//
// The collection arm of VisibleCommentEntitySQL, and the exact counterpart of
// VisibleShowCommentEntitySQL: a row naming any other entity type passes this
// arm untouched, because this arm judges collections and nothing else. It is
// NOT a gate on its own — the composite in entity_visibility.go is, and the
// allowlist arm there is what turns "not a collection" into a decision rather
// than a shrug.
//
// Both expressions are SQL the CALLER controls and must be literals in the
// calling code.
func VisibleCollectionCommentEntitySQL(entityTypeExpr, entityIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	visible, args := VisibleCollectionExistsSQL(entityIDExpr, viewer)
	return commentEntityArmSQL(entityTypeExpr, CommentEntityTypeCollection, visible), args
}

// VisibleCollectionRecipientsSQL returns a condition, true for the rows whose
// recipientIDExpr names a user who may see collection collectionID, plus its
// bind arguments.
//
// The rule INVERTED, for a fan-out choosing who to notify: every other spelling
// fixes the viewer and asks about many collections, this one fixes the
// collection and asks about many viewers.
//
// It takes no admin expression, and that is not an omission for cost. Its show
// twin keeps an admin branch precisely BECAUSE the three read gates grant an
// admin a gated show, and a fan-out is final — a row never written is never
// restored, so excluding an admin there would permanently contradict what they
// are entitled to read. Here the read gates grant an admin nothing, so an admin
// branch would contradict them in the other direction: it would mail a private
// collection's comment to somebody the detail route refuses.
//
// recipientIDExpr is SQL the CALLER controls and must be a literal in the
// calling code; collectionID is bound.
func VisibleCollectionRecipientsSQL(collectionID uint, recipientIDExpr string) (string, []interface{}) {
	cond := VisibleCollectionRecipientPredicateSQL(visibleCollectionsAlias, recipientIDExpr)
	// collectionID is the only bind, and it binds first because
	// collectionExistsSQL puts the id comparison first and placeholders are
	// positional.
	return collectionExistsSQL("?", cond), []interface{}{collectionID}
}

// VisibleCollectionRecipientPredicateSQL returns a condition, true when the
// collection at alias is one the user named by recipientIDExpr may see.
//
// The rule with BOTH sides as columns, for a query that already joins the
// collection and the candidate recipient: a fan-out choosing who to notify, a
// digest cycle choosing who to mail. It binds nothing, because both operands are
// columns of the enclosing query.
//
// It is the predicate VisibleCollectionRecipientsSQL wraps in its EXISTS, shared
// rather than copied so a mailing surface cannot decide a collection on terms the
// fan-out does not use. TestCollectionVisibilitySpellingsAgree checks it against
// the same truth table as every other spelling.
//
// alias and recipientIDExpr are SQL the CALLER controls and must be literals in
// the calling code.
func VisibleCollectionRecipientPredicateSQL(alias, recipientIDExpr string) string {
	return "(" + alias + ".is_public = TRUE OR " + alias + ".creator_id = " + recipientIDExpr + ")"
}

// CollectionVisibleTo reports whether viewer may see collection collectionID at
// all.
//
// The injectable form, on the same service as ShowVisibleTo: the two gates share
// one database handle, one handler field and one construction, so splitting them
// across two services would only mean two ways for a handler to be wired with
// half a gate.
func (s *ShowVisibilityService) CollectionVisibleTo(collectionID uint, viewer contracts.ShowViewer) bool {
	if s == nil {
		// A handler wired without a gate refuses rather than serves, for the
		// reason the show twin gives.
		return false
	}
	return CollectionVisibleTo(s.db, collectionID, viewer)
}
