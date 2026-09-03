package routes

// THE COMMENT-ID ROUTE FAMILY, recorded once for both visibility inventories.
//
// A comment hangs off a show as readily as off a private collection, so every
// route addressed by a COMMENT id can name either without the path saying so.
// The show inventory and the collection inventory both have to disposition the
// whole family, and typing it into both maps means the next comment route is two
// edits in two files with two vocabularies. Enumerated here instead, so a route
// added to this slice reaches both inventories and cannot be dispositioned in
// one and forgotten in the other.
//
// Comment ids are DENSE and sequential, which is what makes the family worth
// enumerating at all: a route that answered differently for a gated parent would
// be walkable over the whole table.

// commentIDGatedRoutes are gated on the parent entity the handler resolves out
// of the comment, and refuse it with the answer a comment id nobody has used
// already gets.
//
// The first eight are gated in Go, by separate shared.EntitySubResourceVisible
// calls in handlers/engagement. The report route is gated in the SERVICE
// (services/admin/entity_report.go resolves the parent and asks the registry),
// which is why it is listed rather than assumed: reading its handler alone would
// say it is open. Its `/shows/{show_id}/report` sibling is deliberately NOT
// gated, and carries its own disposition in the show inventory.
var commentIDGatedRoutes = []string{
	"GET /comments/{comment_id}",
	"GET /comments/{comment_id}/thread",
	"POST /comments/{comment_id}/replies",
	"PUT /comments/{comment_id}",
	"DELETE /comments/{comment_id}",
	"PUT /comments/{comment_id}/reply-permission",
	"POST /comments/{comment_id}/vote",
	"DELETE /comments/{comment_id}/vote",
	"POST /comments/{entity_id}/report",
}

// commentIDAdminRoutes are the moderation queue's own remedies, registered on
// the admin group.
//
// They reach a comment on a private collection deliberately: that is the
// documented moderation exception in services/shared/collection_visibility.go,
// and a queue whose remedies refused what the queue shows would be one an admin
// could not act on.
var commentIDAdminRoutes = []string{
	"GET /admin/comments/{comment_id}/edits",
	"POST /admin/comments/{comment_id}/hide",
	"POST /admin/comments/{comment_id}/restore",
	"POST /admin/comments/{comment_id}/approve",
	"POST /admin/comments/{comment_id}/reject",
}

// addRoutes stamps one disposition onto every route in keys.
//
// Generic over the two inventories' disposition types, which are separate
// vocabularies on purpose: `gated` and `collectionGated` are claims about
// different entities and must not become interchangeable.
func addRoutes[D comparable](m map[string]D, keys []string, disposition D) {
	for _, key := range keys {
		m[key] = disposition
	}
}
