package shared

import (
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// COLLECTION CREATOR STATE
// =============================================================================
//
// A collection is published under its creator, and two states of the creator's
// ACCOUNT withdraw it from readers. Both sit beside the is_public rule in
// collection_visibility.go rather than inside it: that rule is shared with
// comments, tags, the inbox and the fan-outs, and these two apply only to the
// surfaces named here.
//
//  1. A CLOSED CREATOR (deactivated or soft-deleted, authm.LiveAccountSQL) takes
//     their collections with them: the slug and id readers answer the not-found
//     error a never-used identifier gets, and no listing carries them. The
//     creator cannot sign in, so there is no owner to exempt. Nothing is
//     scrubbed, so reopening the account serves every collection again.
//
//  2. A PRIVATE PROFILE keeps the creator's collections off the browse list and
//     the entity's collections list for every viewer but the creator. Each
//     public collection stays readable at its own slug, which is the private
//     profile's answer everywhere: the per-user list is hidden, the content is
//     not.

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
// alias is SQL the CALLER controls and must be a literal in the calling code.
func CollectionCreatorIsListedSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	live := CollectionCreatorIsLiveSQL(alias)
	publicProfile := "EXISTS (SELECT 1 FROM users " + collectionCreatorAlias +
		" WHERE " + collectionCreatorAlias + ".id = " + alias + ".creator_id" +
		" AND " + collectionCreatorAlias + ".profile_visibility <> 'private')"
	if viewer.UserID == 0 {
		return "(" + live + " AND " + publicProfile + ")", nil
	}
	return "(" + live + " AND (" + alias + ".creator_id = ? OR " + publicProfile + "))",
		[]interface{}{viewer.UserID}
}
