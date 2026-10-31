package shared

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// COLLECTION CREATOR STATE
// =============================================================================
//
// A collection is published under its creator, and two states of the creator's
// ACCOUNT withdraw it from readers. They compose with the is_public rule in
// collection_visibility.go into three tiers, each the one before it plus one
// term:
//
//   - VISIBLE (collection_visibility.go): public, or the viewer is the creator.
//     The is_public rule alone: CollectionVisibleTo, VisibleCollectionPredicateSQL
//     and VisibleCollectionRecipientPredicateSQL.
//   - SERVED: visible, and the creator is a live account (authm.LiveAccountSQL).
//     What every surface that answers a viewer about a collection decides on,
//     including the polymorphic registry's collection arm (entity_visibility.go)
//     and the mailing surfaces. CollectionServedTo, ServedCollectionPredicateSQL,
//     ServedCollectionRecipientPredicateSQL and the Served* spellings in
//     collection_visibility.go.
//   - LISTED: served, and the creator's profile is public or the viewer is the
//     creator. What a LISTING of many collections, and a count rendered beside
//     one, decides on. CollectionCreatorIsListedSQL beside the visible
//     predicate, and PublicListedCollectionSQL for the public-tier listings.
//
// No viewer-facing surface decides on VISIBLE alone. VISIBLE is the building
// block the other two are made of. A route that must still reach a closed
// creator's collection (an admin's moderation remedy) loads the row without
// the creator-state rules and says so at its definition.
//
// A CLOSED CREATOR (deactivated or soft-deleted) takes their collections with
// them: the routes answer the not-found error a never-used identifier gets, no
// listing carries them and no count includes them. The creator cannot sign in,
// so there is no owner to exempt, and no tier changes what a creator sees of
// their own collections. Nothing is scrubbed, so reopening the account serves
// every collection again.
//
// A PRIVATE PROFILE keeps the creator's collections off the listings for every
// viewer but the creator. Each public collection stays served at its own slug,
// which is the private profile's answer everywhere: the per-user list is
// hidden, the content is not.

// collectionCreatorAlias is the alias the profile condition binds the users
// table to. Distinct from every alias a calling query uses, so the correlation
// on alias.creator_id cannot be shadowed by the subquery.
const collectionCreatorAlias = "collection_creator"

// CollectionCreatorIsLiveSQL returns a correlated EXISTS condition, true for the
// rows of the collections table alias whose creator is a live account. It binds
// nothing, so it fits a raw SQL string and a GORM Where alike.
//
// alias is SQL the CALLER controls and must be a literal in the calling code.
func CollectionCreatorIsLiveSQL(alias string) string {
	return authm.LiveAccountExistsSQL(alias + ".creator_id")
}

// CollectionCreatorIsListedSQL returns a condition, true for the rows of the
// collections table alias that a listing may carry for viewer: the creator is a
// live account (CollectionCreatorIsLiveSQL), and their profile is public or
// viewer is the creator. Its only bind argument is the viewer's own id.
//
// It does not test is_public: the listing applies the visible predicate beside
// it.
//
// alias is SQL the CALLER controls and must be a literal in the calling code.
func CollectionCreatorIsListedSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	// One probe of the creator's users row decides both terms.
	creatorRow := "EXISTS (SELECT 1 FROM users " + collectionCreatorAlias +
		" WHERE " + collectionCreatorAlias + ".id = " + alias + ".creator_id" +
		" AND " + authm.LiveAccountSQL(collectionCreatorAlias)
	publicProfile := collectionCreatorAlias + ".profile_visibility <> 'private'"
	if viewer.UserID == 0 {
		return creatorRow + " AND " + publicProfile + ")", nil
	}
	return creatorRow + " AND (" + publicProfile + " OR " + alias + ".creator_id = ?))",
		[]interface{}{viewer.UserID}
}

// ServedCollectionPredicateSQL returns a condition, true for the rows of the
// collections table alias that viewer may be served: visible
// (VisibleCollectionPredicateSQL) with a live creator
// (CollectionCreatorIsLiveSQL). Its bind arguments are the visible predicate's.
//
// alias is SQL the CALLER controls and must be a literal in the calling code.
func ServedCollectionPredicateSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	visible, args := VisibleCollectionPredicateSQL(alias, viewer)
	return "(" + visible + " AND " + CollectionCreatorIsLiveSQL(alias) + ")", args
}

// PublicListedCollectionSQL returns the condition, true for the rows of the
// collections table alias that a PUBLIC-TIER listing may carry: public, with a
// live creator whose profile is public. A public-tier listing answers the same
// for every caller, so this takes no viewer, and it binds nothing, so it fits a
// raw SQL string and a GORM Where alike.
//
// alias is SQL the CALLER controls and must be a literal in the calling code.
func PublicListedCollectionSQL(alias string) string {
	// The anonymous forms of both conditions bind nothing: with no viewer id
	// neither has a creator branch. A form that ever bound an argument here
	// would leave a placeholder unfilled, so it lists nothing instead.
	visible, visibleArgs := VisibleCollectionPredicateSQL(alias, contracts.ShowViewer{})
	listed, listedArgs := CollectionCreatorIsListedSQL(alias, contracts.ShowViewer{})
	if len(visibleArgs) != 0 || len(listedArgs) != 0 {
		return "FALSE"
	}
	return "(" + visible + " AND " + listed + ")"
}

// PublicListedCollectionExistsSQL returns a correlated EXISTS condition, true
// when the collection named by collectionIDExpr is one a public-tier listing may
// carry (PublicListedCollectionSQL). For a listing that holds collection ids in
// another table's column, such as a tag's entity_tags rows. It binds nothing.
//
// collectionIDExpr is SQL the CALLER controls and must be a literal in the
// calling code, never qualified with visibleCollectionsAlias.
func PublicListedCollectionExistsSQL(collectionIDExpr string) string {
	return collectionExistsSQL(collectionIDExpr, PublicListedCollectionSQL(visibleCollectionsAlias))
}

// ServedCollectionRecipientPredicateSQL returns a condition, true when the
// collection at alias may be served to the user named by recipientIDExpr: the
// visible recipient predicate (VisibleCollectionRecipientPredicateSQL) with a
// live creator. The rule with both sides as columns, shared by the comment
// fan-out (ServedCollectionRecipientsSQL) and the digest cycle so the two
// mailing surfaces decide a collection on one rule. It binds nothing.
//
// alias and recipientIDExpr are SQL the CALLER controls and must be literals in
// the calling code.
func ServedCollectionRecipientPredicateSQL(alias, recipientIDExpr string) string {
	return "(" + VisibleCollectionRecipientPredicateSQL(alias, recipientIDExpr) +
		" AND " + CollectionCreatorIsLiveSQL(alias) + ")"
}

// CollectionServedTo reports whether viewer may be served collection
// collectionID: it is visible to them (CollectionVisibleTo's rule) and its
// creator is a live account.
//
// It fails closed on the terms CollectionVisibleTo states: a missing row, a nil
// db, a zero id and a failed lookup all answer false, so a closed creator's
// collection, a private one and a deleted one are one answer.
//
// viewer.IsAdmin is not read, for the reason collection_visibility.go's header
// gives.
func CollectionServedTo(db *gorm.DB, collectionID uint, viewer contracts.ShowViewer) bool {
	if db == nil || collectionID == 0 {
		return false
	}

	served, servedArgs := ServedCollectionPredicateSQL("collections", viewer)
	var count int64
	if err := db.Model(&communitym.Collection{}).
		Where("collections.id = ?", collectionID).
		Where(served, servedArgs...).
		Count(&count).Error; err != nil {
		logger.Default().Error("collection_served_lookup_failed",
			"collection_id", collectionID,
			"error", err.Error(),
		)
		return false
	}
	return count > 0
}
