package shared

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// SHOW VISIBILITY — ONE RULE, SEVERAL SPELLINGS
// =============================================================================
//
// GET /shows/{id} answers 404 for a show whose status is not approved unless
// the caller is an admin or the show's own submitter
// (handlers/catalog.GetShowHandler). That is the rule. This file is its single
// definition, and every route that serves a show's content, id, title or count
// by show id evaluates it from here rather than re-deriving it (PSY-1939).
//
// A rule spelled in N places is N rules. Anything that needs this one calls it
// from here; a `status = 'approved'` written by hand anywhere else is a bug
// waiting for the next status value, and the class of leak this file exists to
// close was exactly that — a copied predicate that several surfaces never got a
// copy of.
//
// The spellings differ because the callers are not the same shape:
//
//   - ShowVisibleTo answers for ONE already-identified show. Handlers use it.
//   - VisibleShowPredicateSQL is the same rule over a shows-table alias, for a
//     query already reading shows.
//   - VisibleShowExistsSQL wraps that in a correlated EXISTS, for a query
//     holding only a show id in some other table's column.
//   - VisibleShowRevisionsSQL adds the two revision-specific terms.
//   - VisibleShowCommentEntitySQL is the EXISTS form over a POLYMORPHIC
//     (entity_type, entity_id) pair, for the comment family's tables, where a
//     row may name a show or may name an artist.
//   - VisibleShowRecipientsSQL decides ONE show for MANY viewers at once,
//     reading the viewer id from a COLUMN, for a fan-out choosing recipients.
//   - PublicShowPredicateSQL and PublicShowRevisionsSQL are the public tier of
//     the two predicates with the status inlined, for statement builders that
//     return a bare string and have no argument list to bind into.
//
// They must all agree, and TestShowVisibilitySpellingsAgree in
// show_visibility_test.go enumerates the whole viewer x status matrix against
// every one of them, comparing each to a hand-written truth table rather than
// to the others. A shared bug cannot make them agree and be wrong together.
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
//
// Deliberately not `s`, which the queries this predicate is spliced into already
// use. An alias declared in this subquery SHADOWS an outer one of the same name,
// so a showIDExpr qualified with the colliding alias would self-correlate —
// `visible_show.id = visible_show.id` — and the EXISTS would be true whenever any
// approved show exists at all, opening the gate completely. Callers must never
// pass an expression qualified with this alias.
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
	return showExistsSQL(showIDExpr, inner), args
}

// showExistsSQL wraps a shows-table condition in the correlated EXISTS both
// tiers use. The subquery shape lives here once so the bound and inlined forms
// cannot correlate on different columns.
func showExistsSQL(showIDExpr, showCond string) string {
	return "EXISTS (SELECT 1 FROM shows " + visibleShowsAlias +
		" WHERE " + visibleShowsAlias + ".id = " + showIDExpr +
		" AND " + showCond + ")"
}

// CommentEntityTypeShow is the polymorphic entity_type value the comment family
// stores a show under — comments, comment subscriptions, last-read pointers.
//
// Read from the model rather than written out, because the rows this gate
// decides about are written from that constant and the comparison in Postgres is
// case-sensitive. It is spelled the same as RevisionEntityTypeShow today and
// deliberately kept separate: they are two vocabularies that happen to agree,
// and one changing is not the other changing.
const CommentEntityTypeShow = string(engagementm.CommentEntityShow)

// VisibleShowCommentEntitySQL returns a condition, true for the rows of a
// POLYMORPHIC (entity_type, entity_id) table that viewer may see, plus its bind
// arguments.
//
// For the comment family's tables — comments, comment_subscriptions,
// notification rows resolved through a comment — where one column decides
// whether the id beside it is a show id at all.
//
// A row naming any OTHER entity type passes untouched: show is the only comment
// parent with a read-time visibility rule. A row naming a show that no longer
// exists does NOT pass, because VisibleShowExistsSQL fails closed on a missing
// show, which is what lets a caller answer the same for a gated show and a
// deleted one.
//
// Both expressions are SQL the CALLER controls and must be literals in the
// calling code. Nothing derived from a request may reach them.
func VisibleShowCommentEntitySQL(entityTypeExpr, entityIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	visible, args := VisibleShowExistsSQL(entityIDExpr, viewer)
	return "(" + entityTypeExpr + " <> '" + CommentEntityTypeShow + "' OR " + visible + ")", args
}

// VisibleShowRecipientsSQL returns a condition, true for the rows whose
// recipientIDExpr names a user who may see show showID, plus its bind arguments.
//
// The rule INVERTED: every other spelling fixes the viewer and asks about many
// shows, this one fixes the show and asks about many viewers. A fan-out picking
// who to notify about one show needs exactly that, and doing it the other way
// round is one query per recipient.
//
// It has NO admin branch, and cannot have one: the branch would need each row's
// is_admin, which is a second join for a bypass whose only effect is to keep
// pushing mail about a show that has been taken private. A gated show's fan-out
// therefore reaches its submitter and nobody else, admin included.
//
// THIS GATE IS FINAL, and it is the one place in PSY-1983 that is. Every other
// gate the ticket adds suppresses at READ time, so republishing the show brings
// the withheld rows back. A fan-out that declines to write mints nothing, and
// nothing is what republication restores: activity during the gated window is
// never delivered, by any channel, to anyone the gate excluded. That asymmetry
// is deliberate — a push cannot be recalled, so the write side has to be decided
// once, at the moment of sending, and the recoverable direction is to withhold.
// The next comment after republication notifies normally.
//
// The admin consequence is worth stating plainly, because it is visible: an
// admin subscribed to a show that goes private keeps SEEING it (their watching
// list and the inbox's read gate both grant them the admin tier) while receiving
// no bell row and no mail for anything posted during the window.
//
// recipientIDExpr is SQL the CALLER controls and must be a literal in the
// calling code. Nothing derived from a request may reach it; showID is bound.
func VisibleShowRecipientsSQL(showID uint, recipientIDExpr string) (string, []interface{}) {
	cond := "(" + visibleShowsAlias + ".status = ? OR " +
		visibleShowsAlias + ".submitted_by = " + recipientIDExpr + ")"
	// showID binds ahead of the status because showExistsSQL puts the id
	// comparison first, and the placeholders are positional.
	return showExistsSQL("?", cond), []interface{}{showID, catalogm.ShowStatusApproved}
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
func VisibleShowRevisionsSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	visible, visibleArgs := VisibleShowExistsSQL(alias+".entity_id", viewer)
	return revisionVisibilitySQL(alias, visible), visibleArgs
}

// RevisionsTable is the alias to pass the revision spellings when the query
// leaves the revisions table unaliased.
const RevisionsTable = "revisions"

// revisionVisibilitySQL wraps a show-visibility condition in the two
// revision-specific terms both tiers share: a non-show row passes, and a
// merge-stamped row never does.
//
// alias is the revisions-table alias the enclosing query uses, and it is a
// parameter rather than a hardcoded "revisions" because a caller that writes
// FROM revisions r, or joins the table twice, would otherwise splice in a
// predicate naming a table not in its FROM clause: Postgres either errors or,
// where an outer scope has one, silently correlates against the wrong rows.
//
// The entity_type value is interpolated rather than bound because it is
// RevisionEntityTypeShow, a package constant, and because the inlined tier below
// has no bind slot to put it in. Keeping both tiers on one spelling of this
// skeleton matters more than keeping one of them on a placeholder.
func revisionVisibilitySQL(alias, showCond string) string {
	return "(" + alias + ".entity_type <> '" + RevisionEntityTypeShow + "' OR (" +
		alias + ".from_gated_show = FALSE AND " + showCond + "))"
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
//
// Shares the revision skeleton and the EXISTS shape with the bound form, so the
// only thing that differs between the two is which show condition goes inside.
func PublicShowRevisionsSQL(alias string) string {
	return revisionVisibilitySQL(
		alias,
		showExistsSQL(alias+".entity_id", PublicShowPredicateSQL(visibleShowsAlias)),
	)
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
