package shared

import (
	"errors"

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
//   - LoadedShowVisibleTo is the Go spelling, over a show's status and
//     submitter, for a caller that already holds the row. The detail route is
//     one, through handlers/shared.ShowRowVisible.
//   - ShowVisibleTo is that same predicate with a LOOKUP in front, for a caller
//     holding only an id. It is not a second spelling of the rule.
//   - VisibleShowPredicateSQL is the same rule over a shows-table alias, for a
//     query already reading shows.
//   - VisibleShowExistsSQL wraps that in a correlated EXISTS, for a query
//     holding only a show id in some other table's column.
//   - VisibleShowRevisionsSQL adds the two revision-specific terms.
//   - VisibleShowCommentEntitySQL is the SHOW ARM of a polymorphic
//     (entity_type, entity_id) predicate. It is not a gate on its own: it lets
//     every non-show row through, and shared.VisibleCommentEntitySQL is what
//     composes it with the other arms and the allowlist.
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
// GET /shows/{id} is INSIDE that matrix rather than beside it, because the
// handler evaluates one of the forms above instead of restating the rule.
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

// LoadedShowVisibleTo reports whether viewer may see a show whose status and
// submitter are ALREADY IN HAND.
//
// The rule as a pure function of the two facts it decides on, for a caller
// holding the row: the detail route, which has just loaded the show it is about
// to serve, and ShowVisibleTo, which loads those two columns and asks this.
//
// submittedBy is a pointer because the column is nullable, and a NULL submitter
// matches nobody: an anonymous submission is not the caller's own, whoever the
// caller is. The zero viewer is the anonymous tier and matches no submitter
// either, since user ids start at 1.
//
// status is catalogm.ShowStatus rather than a bare string so a caller holding a
// response DTO has to convert deliberately. A security predicate that accepted
// any string would decide on display text the day one of those fields is
// repurposed.
func LoadedShowVisibleTo(status catalogm.ShowStatus, submittedBy *uint, viewer contracts.ShowViewer) bool {
	if viewer.IsAdmin {
		return true
	}
	if status == catalogm.ShowStatusApproved {
		return true
	}
	return viewer.UserID != 0 && submittedBy != nil && *submittedBy == viewer.UserID
}

// ShowVisibleTo reports whether viewer may see show showID at all.
//
// LoadedShowVisibleTo with a lookup in front, for a caller holding an id rather
// than a row. It reads the two columns and asks the predicate rather than asking
// the database to decide, which also removes the OR-precedence hazard a compound
// WHERE carries: there is no OR left in this statement to parenthesise.
//
// For a NON-ADMIN it answers false when the show does not exist, which is the
// same answer GET /shows/{id} gives by 404ing, and false on any lookup failure.
// An ADMIN is answered true without a lookup, so this is the one input on which
// it and LoadedShowVisibleTo differ: an admin asking about an id that carries no
// row gets true here and false from the predicate, which has a row to look at.
// Neither answer is a disclosure, because an admin may see every show that
// exists; the callers that must not serve a deleted show say so themselves
// (VisibleShowRecipientsSQL puts its admin term INSIDE the shows EXISTS).
func ShowVisibleTo(db *gorm.DB, showID uint, viewer contracts.ShowViewer) bool {
	// A LOOKUP SHORT-CIRCUIT, and the one input on which this and the predicate
	// disagree: an admin is answered true without a read, so an id that carries
	// no row answers true here and false from the predicate below.
	if viewer.IsAdmin {
		return true
	}
	if db == nil || showID == 0 {
		return false
	}

	var row struct {
		Status      catalogm.ShowStatus
		SubmittedBy *uint
	}
	err := db.Model(&catalogm.Show{}).
		Select("status, submitted_by").
		Where("id = ?", showID).
		Take(&row).Error
	if err != nil {
		// A show that is not there is not a failure, and it is the answer this
		// gate is built to give: absent and gated are one response. Only a real
		// lookup failure is logged, so the log stays a signal.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Default().Error("show_visibility_lookup_failed",
				"show_id", showID,
				"error", err.Error(),
			)
		}
		return false
	}
	return LoadedShowVisibleTo(row.Status, row.SubmittedBy, viewer)
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
// tiers use. Named here so the bound and inlined forms cannot correlate on
// different columns, and delegating to entityExistsSQL so the shows rule and the
// collections rule cannot correlate on different SHAPES.
func showExistsSQL(showIDExpr, showCond string) string {
	return entityExistsSQL("shows", visibleShowsAlias, showIDExpr, showCond)
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
// THIS IS ONE ARM, NOT A GATE. It judges rows whose entity_type is `show` and
// lets every other row through untouched, because it knows nothing about the
// other entity types. Splicing it into a polymorphic query on its own serves
// every private collection's rows. The gate is
// VisibleCommentEntitySQL in entity_visibility.go: it composes this arm with
// one arm per other gated type and with the allowlist that refuses a type
// nobody has dispositioned.
//
// A row naming a show that no longer exists does NOT pass, because
// VisibleShowExistsSQL fails closed on a missing show, which is what lets a
// caller answer the same for a gated show and a deleted one.
//
// Both expressions are SQL the CALLER controls and must be literals in the
// calling code. Nothing derived from a request may reach them.
func VisibleShowCommentEntitySQL(entityTypeExpr, entityIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return "TRUE", nil
	}
	visible, args := VisibleShowExistsSQL(entityIDExpr, viewer)
	return commentEntityArmSQL(entityTypeExpr, CommentEntityTypeShow, visible), args
}

// VisibleShowRecipientsSQL returns a condition, true for the rows whose
// recipientIDExpr names a user who may see show showID, plus its bind arguments.
//
// The rule INVERTED: every other spelling fixes the viewer and asks about many
// shows, this one fixes the show and asks about many viewers. A fan-out picking
// who to notify about one show needs exactly that, and doing it the other way
// round is one query per recipient.
//
// recipientIsAdminExpr carries the admin tier the other spellings get from
// ShowViewer.IsAdmin. It is a COLUMN here for the same reason the viewer id is:
// the answer differs per row. Both call sites already have the users table in
// scope, so it costs no extra join — pass "" only where no such column exists,
// which drops the branch and answers approved-or-submitter.
//
// Keeping the admin branch matters because this gate is FINAL, and it is the one
// place in PSY-1983 that is. Every other gate the ticket adds suppresses at READ
// time, so republishing the show brings the withheld rows back. A fan-out that
// declines to write mints nothing, and nothing is what republication restores:
// activity during the gated window is never delivered to anyone the gate
// excluded, ever. A push cannot be recalled, so the write side is decided once,
// at the moment of sending, and the recoverable direction is to withhold.
//
// That finality is exactly why the admin branch is not optional. Without it an
// admin's inbox would permanently disagree with what all three READ gates say
// they are entitled to — on the moderation path, where a pending show's
// discussion is the thing they are most likely to be watching.
//
// recipientIDExpr is SQL the CALLER controls and must be a literal in the
// calling code. Nothing derived from a request may reach it; showID is bound.
func VisibleShowRecipientsSQL(showID uint, recipientIDExpr, recipientIsAdminExpr string) (string, []interface{}) {
	cond := "(" + visibleShowsAlias + ".status = ? OR " +
		visibleShowsAlias + ".submitted_by = " + recipientIDExpr
	if recipientIsAdminExpr != "" {
		cond += " OR " + recipientIsAdminExpr
	}
	cond += ")"
	// showID binds ahead of the status because showExistsSQL puts the id
	// comparison first, and the placeholders are positional.
	//
	// The admin term sits INSIDE the shows EXISTS rather than beside it, so an
	// admin passes only when the show actually exists. A deleted show reaches
	// nobody, which is what keeps "gated" and "gone" answering alike here too.
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
