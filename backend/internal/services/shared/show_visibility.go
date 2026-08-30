package shared

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// SHOW VISIBILITY — ONE RULE, THREE SPELLINGS
// =============================================================================
//
// GET /shows/{id} answers 404 for a show whose status is not approved unless
// the caller is an admin or the show's own submitter
// (handlers/catalog.GetShowHandler). That is the rule. This file is its single
// definition, and every route that serves a show's content, id, title or count
// by show id evaluates it from here rather than re-deriving it (PSY-1939).
//
// Before this file the rule was copied. PSY-1715 mirrored it inside the
// revision service while field notes, comments, tags, collections, the
// contributions timeline and the public contribution counts kept serving a
// gated show's content anonymously — the detail route's 404 cost a reader one
// extra request. Copies drift; the fix for a class of leak is one definition
// with N callers.
//
// THREE spellings, because the callers are not the same shape:
//
//   - ShowVisibleTo answers for ONE already-identified show. Handlers use it.
//   - VisibleShowPredicateSQL is the same rule over a shows-table alias, for a
//     query already reading shows.
//   - VisibleShowExistsSQL wraps that in a correlated EXISTS, for a query
//     holding only a show id in some other table's column.
//
// The three must agree, and TestShowVisibilityGoAndSQLAgree in
// show_visibility_test.go enumerates the viewer x status matrix against all
// three rather than trusting that they read alike.
//
// EVERY spelling fails closed. A missing show row, a nil db, a zero id and a
// failed lookup all resolve to "not visible" for a non-admin. Withholding a
// publishable show's field notes is recoverable; publishing an unpublished
// show's address is not.
//
// Nothing here scrubs anything: the rows keep their values, and re-approving a
// show restores every sub-resource to everyone, because the gate is evaluated
// at read time against the show's current status.
//
// WHICH VIEWER a caller passes is decided by what the surface is addressed BY,
// and the split is deliberate:
//
//   - A show's OWN sub-resource, addressed by show id — its field notes, its
//     comments, its tags, the collections it sits in. These are the show page,
//     reached by the same id the detail route refuses, so they take the REAL
//     viewer and match the detail route exactly: a submitter reading their own
//     unpublished show's page sees its notes.
//   - A listing that merely CONTAINS shows — tag members, collection contents,
//     an author's field notes, the leaderboard. These take ShowViewer{}, the
//     public tier, for every caller including admins. They are shared and
//     cacheable, so their contents must not vary by credential, and every
//     sibling listing in this codebase already reads approved-only.
//
// The second tier is why ShowViewer{} is spelled as a value rather than hidden
// behind a PublicViewer() constructor: a caller passing it is making a choice,
// and the choice should be visible at the call site.
//
// NO ENUMERATION ORACLE is the second requirement, and it is the caller's to
// keep, not this file's. A gated show must answer exactly like a show that does
// not exist: a route that 404s an unknown id 404s a gated one with the same
// body, and a route that answers an unknown id with an empty list answers a
// gated one with the same empty list — never a 403, and never an empty list
// beside a non-zero total.

// visibleShowsAlias is the alias VisibleShowExistsSQL binds the shows table to.
// Deliberately not `s`: the queries this predicate is spliced into already use
// short aliases, and a collision would silently re-point the correlation at the
// outer query's row.
const visibleShowsAlias = "visible_show"

// ShowVisibleTo reports whether viewer may see show showID at all.
//
// The Go spelling of the detail route's predicate. Returns false when the show
// does not exist, which is the same answer GET /shows/{id} gives by 404ing, and
// false on any lookup failure.
func ShowVisibleTo(db *gorm.DB, showID uint, viewer contracts.ShowViewer) bool {
	if viewer.IsAdmin {
		return true
	}
	if db == nil || showID == 0 {
		return false
	}

	// One condition carrying its own parentheses. GORM does parenthesize a raw
	// condition containing OR today, so this is a deliberate refusal to let a
	// security boundary rest on framework behaviour, not a workaround for a bug.
	// Written out, the binding this pins is `id = X AND (status = 'approved' OR
	// submitted_by = Y)` — never `id = X AND status = 'approved' OR
	// submitted_by = Y`, which would answer yes for a gated show whenever the
	// caller submitted ANY show at all.
	q := db.Model(&catalogm.Show{})
	if viewer.UserID != 0 {
		q = q.Where("id = ? AND (status = ? OR submitted_by = ?)",
			showID, catalogm.ShowStatusApproved, viewer.UserID)
	} else {
		q = q.Where("id = ? AND status = ?", showID, catalogm.ShowStatusApproved)
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		logger.Default().Error("show_visibility_lookup_failed",
			"show_id", showID,
			"error", err.Error(),
		)
		return false
	}
	return count > 0
}

// VisibleShowPredicateSQL returns a SQL condition, true for the rows of a shows
// table alias that viewer may see, plus its bind arguments.
//
// For a query already selecting FROM shows. An admin gets the constant TRUE
// rather than an omitted clause so a caller can splice the result into an AND
// chain unconditionally and cannot forget the admin branch.
//
// alias is a SQL identifier the CALLER controls and must be a literal in the
// calling code. Nothing derived from a request may reach it: it is concatenated
// into the statement, while every value is bound.
func VisibleShowPredicateSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	// Built by concatenation rather than written whole because the submitter
	// branch must disappear for an anonymous caller instead of comparing against
	// user id 0. The outer parentheses are written here, not left to the query
	// builder, for the reason ShowVisibleTo gives.
	cond := "(" + alias + ".status = ?"
	args := []interface{}{catalogm.ShowStatusApproved}
	if viewer.UserID != 0 {
		cond += " OR " + alias + ".submitted_by = ?"
		args = append(args, viewer.UserID)
	}
	cond += ")"
	return cond, args
}

// VisibleShowExistsSQL returns a correlated EXISTS condition, true when the show
// named by showIDExpr is one viewer may see, plus its bind arguments.
//
// For a query that holds a show id in some other table's column — a revision's
// entity_id, a contribution row's entity_id. One index probe on the shows
// primary key per row considered, not a join that multiplies rows.
//
// A row whose showIDExpr matches no show is NOT visible, which is what makes
// this usable as the gate: it answers the same for a gated show and a deleted
// one.
//
// showIDExpr is SQL the CALLER controls and must be a literal in the calling
// code. Nothing derived from a request may reach it.
func VisibleShowExistsSQL(showIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	inner, args := VisibleShowPredicateSQL(visibleShowsAlias, viewer)
	cond := "EXISTS (SELECT 1 FROM shows " + visibleShowsAlias +
		" WHERE " + visibleShowsAlias + ".id = " + showIDExpr +
		" AND " + inner + ")"
	return cond, args
}

// RevisionEntityTypeShow is the polymorphic entity_type value show revisions
// are stored under.
//
// Here rather than in the revision service because two packages now filter
// revisions by show visibility, and a constant spelled twice is the drift this
// file exists to remove. Case matters: revisions are written with this exact
// value and the entity_type comparison in Postgres is case-sensitive.
const RevisionEntityTypeShow = "show"

// VisibleShowRevisionsSQL returns a condition, true for the rows of the
// revisions table viewer may read, plus its bind arguments.
//
// THE definition of which revisions are visible. The revision service filters
// its listings with it and the contributor profile counts with it, and those two
// numbers must agree: both are public, so a difference between the total
// GET /users/{id}/revisions reports and the profile's revisions_made is a count
// of the author's edits on hidden shows, published as arithmetic.
//
// Three terms, and all three are load-bearing:
//   - a non-show revision passes, show being the only entity type with a
//     read-time visibility rule;
//   - a merge-stamped row never passes, because the show it was stamped for was
//     deleted by that merge and a status lookup would answer for the survivor
//     (see adminm.Revision.FromGatedShow);
//   - otherwise the show's own visibility decides.
//
// The outer parentheses are written here, not left to the query builder, for the
// reason ShowVisibleTo gives.
func VisibleShowRevisionsSQL(viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	visible, visibleArgs := VisibleShowExistsSQL("revisions.entity_id", viewer)
	cond := "(revisions.entity_type <> ? OR (revisions.from_gated_show = FALSE AND " + visible + "))"
	args := append([]interface{}{RevisionEntityTypeShow}, visibleArgs...)
	return cond, args
}

// =============================================================================
// THE PUBLIC TIER, FOR STATEMENT BUILDERS THAT HAVE NO BIND SLOT
// =============================================================================
//
// Some queries are assembled as one SQL string by a builder that returns no
// argument list to append to (the leaderboard's ranking subqueries). Threading
// binds through those builders means changing their signatures for a value that
// is a compile-time constant.
//
// So these two spell the PUBLIC tier with the status inlined. Two properties
// make that safe and keep it safe:
//
//   - The only interpolated value is catalogm.ShowStatusApproved, a package
//     constant. Nothing derived from a request can reach these, and there is no
//     parameter through which it could: neither takes a viewer.
//   - They express the public tier ONLY. A submitter branch would need a user
//     id, which is request data, and must never be built this way.
//
// Use the bound forms above wherever the caller can carry arguments.

// publicShowStatusLiteral is the approved status as a SQL literal. Derived from
// the model constant so the two cannot drift.
const publicShowStatusLiteral = "'" + string(catalogm.ShowStatusApproved) + "'"

// PublicShowPredicateSQL is VisibleShowPredicateSQL's public tier, inlined.
func PublicShowPredicateSQL(alias string) string {
	return "(" + alias + ".status = " + publicShowStatusLiteral + ")"
}

// PublicShowRevisionsSQL is VisibleShowRevisionsSQL's public tier, inlined.
func PublicShowRevisionsSQL() string {
	return "(revisions.entity_type <> '" + RevisionEntityTypeShow + "' OR (" +
		"revisions.from_gated_show = FALSE AND EXISTS (" +
		"SELECT 1 FROM shows " + visibleShowsAlias +
		" WHERE " + visibleShowsAlias + ".id = revisions.entity_id" +
		" AND " + PublicShowPredicateSQL(visibleShowsAlias) + ")))"
}

// ShowVisibilityService is the injectable form of ShowVisibleTo, for handlers,
// which hold service interfaces rather than a database.
//
// It satisfies contracts.ShowVisibilityInterface and holds nothing but the
// handle: there is no state to get stale, and the gate is re-evaluated per
// request against the show's current status.
type ShowVisibilityService struct {
	db *gorm.DB
}

// NewShowVisibilityService creates a ShowVisibilityService.
func NewShowVisibilityService(db *gorm.DB) *ShowVisibilityService {
	return &ShowVisibilityService{db: db}
}

// ShowVisibleTo reports whether viewer may see show showID at all.
func (s *ShowVisibilityService) ShowVisibleTo(showID uint, viewer contracts.ShowViewer) bool {
	if s == nil {
		// A handler wired without a gate refuses rather than serves. The nil
		// receiver is reachable only from a construction bug, and the safe
		// answer to a construction bug on a security boundary is "no".
		return false
	}
	return ShowVisibleTo(s.db, showID, viewer)
}
