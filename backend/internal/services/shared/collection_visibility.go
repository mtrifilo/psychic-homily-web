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
// serves a collection's identity or activity by collection id, and it is the
// exact counterpart of show_visibility.go — read that file first; the design
// notes there about aliases, parenthesisation, caller-controlled SQL and the
// no-enumeration-oracle obligation all apply here unchanged (PSY-1987).
//
// TWO WAYS THIS RULE DIFFERS FROM THE SHOW RULE, and both are load-bearing:
//
//   1. THERE IS NO ADMIN TIER. Every show spelling short-circuits to TRUE for an
//      admin. No collection spelling does, because no collection READ path in
//      this codebase has ever granted one: GetBySlug, GetByID, GetCollectionGraph,
//      CloneCollection, Subscribe, Like and Unlike all test `!IsPublic &&
//      CreatorID != viewerID` with no isAdmin term, and an admin who is not the
//      creator is refused a private collection today. A gate more permissive
//      than the route it mirrors is a second rule, and the second rule is the
//      leak. So `viewer.IsAdmin` is deliberately UNREAD by everything here;
//      TestCollectionVisibilitySpellingsAgree pins that with a real admin row.
//
//      This propagates. VisibleCommentEntitySQL cannot take the admin
//      short-circuit its show-only predecessor took, and the notification
//      inbox's predicate cannot answer `1 = 1` for an admin any more, because
//      both now decide collection-typed rows as well.
//
//   2. THE STATE IS ONE BOOLEAN, not a four-value status enum, and it is NOT
//      NULL with a default of true. `is_public = TRUE` is therefore written as a
//      SQL keyword rather than bound, so the only bound value in any spelling
//      here is the viewer's own id. Nothing derived from a request is ever
//      interpolated.
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

	// One condition carrying its own parentheses, for the reason ShowVisibleTo
	// gives: written out, the binding this pins is `id = X AND (is_public = TRUE
	// OR creator_id = Y)` — never `id = X AND is_public = TRUE OR creator_id =
	// Y`, which would answer yes for a private collection whenever the caller
	// created ANY collection at all.
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

// collectionExistsSQL wraps a collections-table condition in the correlated
// EXISTS every spelling uses. The subquery shape lives here once so the
// viewer-facing and recipient-facing forms cannot correlate on different
// columns.
func collectionExistsSQL(collectionIDExpr, collectionCond string) string {
	return "EXISTS (SELECT 1 FROM collections " + visibleCollectionsAlias +
		" WHERE " + visibleCollectionsAlias + ".id = " + collectionIDExpr +
		" AND " + collectionCond + ")"
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
	return "(" + entityTypeExpr + " <> '" + CommentEntityTypeCollection + "' OR " + visible + ")", args
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
	cond := "(" + visibleCollectionsAlias + ".is_public = TRUE OR " +
		visibleCollectionsAlias + ".creator_id = " + recipientIDExpr + ")"
	// collectionID is the only bind, and it binds first because
	// collectionExistsSQL puts the id comparison first and placeholders are
	// positional.
	return collectionExistsSQL("?", cond), []interface{}{collectionID}
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
