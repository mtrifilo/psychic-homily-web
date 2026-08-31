package contracts

// ShowViewer identifies the caller a show-scoped read is answered for.
//
// It is the whole of what a visibility gate is allowed to know about who is
// asking, and every gate that reads it fails closed on the zero value: an
// anonymous caller is ShowViewer{}, which is neither an admin nor anybody's
// submitter.
//
// UserID is 0 for an anonymous caller, and the gates do not compare that
// against anything: they OMIT the submitter branch entirely when it is 0 rather
// than relying on "no row has id 0" or on NULL = 0 evaluating non-true. A
// refactor that folds the branch back in unconditionally reintroduces both
// assumptions at once.
//
// It is a struct rather than two parameters because the two facts are read
// together by every call site and a bare (uint, bool) pair is the shape a later
// edit transposes. The next viewer fact — a trusted tier, a moderator role —
// belongs here rather than as a fourth positional argument.
type ShowViewer struct {
	// UserID is the authenticated caller's id, or 0 when there is none.
	UserID uint
	// IsAdmin is true only for an authenticated admin. It is resolved from the
	// user row loaded during token validation, never from a claim.
	IsAdmin bool
}

// RevisionViewer is ShowViewer under the name the revision service introduced
// it with (PSY-1715).
//
// An ALIAS, not a second struct. The revision gate and every sibling show gate
// read the same two facts, so they are the same type: a copy is the shape that
// lets one grow a third fact the other does not have, and the two rules then
// drift apart without a compile error.
type RevisionViewer = ShowViewer

// ShowVisibilityInterface answers the show detail route's question for one
// show: may this viewer see it at all.
//
// The narrow contract a HANDLER depends on. Handlers hold service interfaces
// rather than a database, so the gate reaches them as this one method; the rule
// itself lives in services/shared/show_visibility.go and is shared with the
// callers that express it as SQL instead.
type ShowVisibilityInterface interface {
	ShowVisibleTo(showID uint, viewer ShowViewer) bool
}
