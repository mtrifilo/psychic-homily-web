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

// ShowVisibilityInterface answers, for ONE already-identified entity, the
// question its own detail route answers: may this viewer see it at all.
//
// The narrow contract a HANDLER depends on. Handlers hold service interfaces
// rather than a database, so the gates reach them as these methods; the rules
// themselves live in services/shared/show_visibility.go and
// services/shared/collection_visibility.go, shared with the callers that
// express them as SQL instead.
//
// ONE METHOD PER ENTITY TYPE THAT HAS A RULE, rather than a single
// EntityVisibleTo(entityType, id, viewer). The polymorphic dispatch belongs in
// services/shared, where the type registry is (entity_visibility.go), and it
// must fail closed on a type nobody dispositioned — a single method taking a
// type string would put that decision behind an implementation this interface
// cannot see, and every mock would get a vote on it.
//
// THE NAME IS NARROWER THAN WHAT THE INTERFACE ANSWERS. CollectionVisibleTo
// belongs on it because the two gates are the same object at every call site
// (one *gorm.DB, one field, one construction). Renaming the interface and its
// generated mock touches every handler field, every construction and every mock
// in the test suite, which is a mechanical change worth doing on its own rather
// than inside a change to what the gates decide.
type ShowVisibilityInterface interface {
	// ShowVisibleTo mirrors GET /shows/{id}: approved, or the submitter's own,
	// or an admin. See services/shared.ShowVisibleTo.
	ShowVisibleTo(showID uint, viewer ShowViewer) bool

	// CollectionVisibleTo mirrors GET /collections/{slug}: public, or the
	// creator's own. NOT admins: no collection detail or listing read has an
	// admin tier, and a gate more permissive than the route it mirrors is the
	// leak, not the fix. The two admin surfaces that do serve a private
	// collection are named in services/shared/collection_visibility.go, which
	// says why they are moderation powers rather than a tier this gate should
	// carry. See services/shared.CollectionVisibleTo.
	CollectionVisibleTo(collectionID uint, viewer ShowViewer) bool
}
