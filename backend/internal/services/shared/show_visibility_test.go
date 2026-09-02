package shared_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The rule has eight spellings: one in Go and seven in SQL. They are only useful
// if they agree, and reading alike is not agreeing — the SQL forms are strings
// assembled by concatenation, and the one that decides a security boundary is
// the one Postgres parses, not the one a reviewer reads.
//
// So these tests enumerate the whole viewer x status matrix and run every
// spelling against a real database. The five viewer-taking spellings are checked
// against every viewer; the two inlined public-tier forms take no viewer and are
// checked against the anonymous row only, which is exactly what they claim to
// be; the recipient form takes its viewer from a row and is checked against the
// no-admin half of the table, which is what it claims to be. A change to one
// that the others do not follow fails here rather than in whichever route
// happens to use it.

// showCase is one row of the matrix: a show in some state, and who submitted it.
type showCase struct {
	name        string
	status      catalogm.ShowStatus
	bySubmitter bool
}

var showCases = []showCase{
	{"approved, submitted by the viewer", catalogm.ShowStatusApproved, true},
	{"approved, submitted by somebody else", catalogm.ShowStatusApproved, false},
	{"private, submitted by the viewer", catalogm.ShowStatusPrivate, true},
	{"private, submitted by somebody else", catalogm.ShowStatusPrivate, false},
	{"pending, submitted by the viewer", catalogm.ShowStatusPending, true},
	{"pending, submitted by somebody else", catalogm.ShowStatusPending, false},
	{"rejected, submitted by the viewer", catalogm.ShowStatusRejected, true},
	{"rejected, submitted by somebody else", catalogm.ShowStatusRejected, false},
}

// wantVisible is the rule, written out once as a truth table rather than as a
// second implementation. Every spelling is compared against THIS, so a shared
// bug in the implementations cannot make them agree with each other and be
// wrong together.
func wantVisible(c showCase, viewerIsSubmitter bool, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	if c.status == catalogm.ShowStatusApproved {
		return true
	}
	return c.bySubmitter && viewerIsSubmitter
}

// emptyFieldChanges satisfies the revisions.field_changes NOT NULL column. The
// matrix decides visibility from the show, never from the revision's contents.
var emptyFieldChanges = json.RawMessage(`[]`)

func TestShowVisibilitySpellingsAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// Real user rows: shows.submitted_by carries a foreign key, and a matrix
	// built on invented ids would fail to insert rather than exercise the rule.
	//
	// THREE users, not two. The stranger has to be somebody who submitted
	// nothing: reusing the "somebody else" submitter as the stranger makes the
	// stranger the owner on half the matrix, and those rows then pass for the
	// wrong reason.
	viewerID := testhelpers.CreateTestUser(td.DB).ID
	otherID := testhelpers.CreateTestUser(td.DB).ID
	strangerID := testhelpers.CreateTestUser(td.DB).ID
	// A REAL admin row. VisibleShowRecipientsSQL reads the admin tier from the
	// users table rather than from a ShowViewer, so a viewer struct claiming
	// IsAdmin cannot exercise that branch — only a row with is_admin set can.
	adminID := testhelpers.CreateAdminUser(td.DB).ID

	viewers := []struct {
		name   string
		viewer contracts.ShowViewer
		isSelf bool
	}{
		{"anonymous", contracts.ShowViewer{}, false},
		{"an authenticated stranger", contracts.ShowViewer{UserID: strangerID}, false},
		{"the submitter", contracts.ShowViewer{UserID: viewerID}, true},
		{"an admin", contracts.ShowViewer{UserID: strangerID, IsAdmin: true}, false},
	}

	for i, c := range showCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			submitter := otherID
			if c.bySubmitter {
				submitter = viewerID
			}
			show := &catalogm.Show{
				Title:       fmt.Sprintf("Matrix Show %d", i),
				EventDate:   time.Now().UTC().AddDate(0, 0, 7),
				Status:      c.status,
				SubmittedBy: &submitter,
			}
			if err := td.DB.Create(show).Error; err != nil {
				t.Fatalf("create show: %v", err)
			}

			// One revision on this show, so the revisions spellings have a row to
			// decide about. from_gated_show stays false: the merge-stamp case is
			// separate and is covered in the revision service's own tests.
			revision := &adminm.Revision{
				EntityType:   shared.RevisionEntityTypeShow,
				EntityID:     show.ID,
				UserID:       submitter,
				FieldChanges: &emptyFieldChanges,
			}
			if err := td.DB.Create(revision).Error; err != nil {
				t.Fatalf("create revision: %v", err)
			}

			for _, v := range viewers {
				want := wantVisible(c, v.isSelf, v.viewer.IsAdmin)

				if got := shared.ShowVisibleTo(td.DB, show.ID, v.viewer); got != want {
					t.Errorf("ShowVisibleTo for %s = %v, want %v", v.name, got, want)
				}

				predSQL, predArgs := shared.VisibleShowPredicateSQL("shows", v.viewer)
				if got := countShowsMatching(t, td.DB, show.ID, predSQL, predArgs); (got > 0) != want {
					t.Errorf("VisibleShowPredicateSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				existsSQL, existsArgs := shared.VisibleShowExistsSQL("revisions.entity_id", v.viewer)
				if got := countRevisionsMatching(t, td.DB, revision.ID, existsSQL, existsArgs); (got > 0) != want {
					t.Errorf("VisibleShowExistsSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				revSQL, revArgs := shared.VisibleShowRevisionsSQL(shared.RevisionsTable, v.viewer)
				if got := countRevisionsMatching(t, td.DB, revision.ID, revSQL, revArgs); (got > 0) != want {
					t.Errorf("VisibleShowRevisionsSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				entitySQL, entityArgs := shared.VisibleShowCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeShow, show.ID, entitySQL, entityArgs); (got > 0) != want {
					t.Errorf("VisibleShowCommentEntitySQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}
				// The other half of the polymorphic form: a row naming a
				// non-show entity passes for everyone, whatever the show with
				// that id happens to be. A gate that read the id without the
				// type would hide an artist's comments because a private show
				// shares its number.
				if got := countEntityRowMatching(t, td.DB, "artist", show.ID, entitySQL, entityArgs); got != 1 {
					t.Errorf("VisibleShowCommentEntitySQL withheld an ARTIST row from %s", v.name)
				}

				// The recipient form fixes the show and varies the viewer, and it
				// reads BOTH viewer facts out of the row rather than out of a
				// ShowViewer. So the expected answer here is driven by what the
				// USERS TABLE says, not by what v.viewer claims: all three users
				// in this matrix are ordinary rows, so even the viewer that
				// carries IsAdmin gets the non-admin answer. The real admin row
				// is checked separately below.
				//
				// The anonymous viewer is skipped rather than expected to fail:
				// this form reads a user id out of a row, and there is no row
				// for nobody. A fan-out has recipients or it does not run.
				if v.viewer.UserID != 0 {
					recipientSQL, recipientArgs := shared.VisibleShowRecipientsSQL(show.ID, "users.id", "users.is_admin")
					wantRecipient := wantVisible(c, v.isSelf, false)
					if got := countUsersMatching(t, td.DB, v.viewer.UserID, recipientSQL, recipientArgs); (got > 0) != wantRecipient {
						t.Errorf("VisibleShowRecipientsSQL for %s matched %d rows, want visible=%v",
							v.name, got, wantRecipient)
					}
				}
			}

			// The admin ROW: is_admin comes off the users table, so this is the
			// only way to exercise that branch. An admin is a recipient for every
			// show state, which is what keeps a fan-out from permanently
			// disagreeing with the read gates that do grant them the show.
			recipientSQL, recipientArgs := shared.VisibleShowRecipientsSQL(show.ID, "users.id", "users.is_admin")
			if got := countUsersMatching(t, td.DB, adminID, recipientSQL, recipientArgs); got != 1 {
				t.Errorf("VisibleShowRecipientsSQL withheld a %s show from an ADMIN recipient", c.name)
			}
			// ...and with no admin expression the branch disappears entirely,
			// which is what a call site with no users table in scope gets.
			noAdminSQL, noAdminArgs := shared.VisibleShowRecipientsSQL(show.ID, "users.id", "")
			if got := countUsersMatching(t, td.DB, adminID, noAdminSQL, noAdminArgs); (got > 0) != wantVisible(c, false, false) {
				t.Errorf("VisibleShowRecipientsSQL with no admin expression matched %d rows for %s", got, c.name)
			}

			// The two inlined public-tier forms take no viewer, so they are
			// compared against the anonymous answer only. That IS what they claim
			// to be: the public tier, for every caller.
			publicWant := wantVisible(c, false, false)
			if got := countShowsMatching(t, td.DB, show.ID,
				shared.PublicShowPredicateSQL("shows"), nil); (got > 0) != publicWant {
				t.Errorf("PublicShowPredicateSQL matched %d rows, want visible=%v", got, publicWant)
			}
			if got := countRevisionsMatching(t, td.DB, revision.ID,
				shared.PublicShowRevisionsSQL(shared.RevisionsTable), nil); (got > 0) != publicWant {
				t.Errorf("PublicShowRevisionsSQL matched %d rows, want visible=%v", got, publicWant)
			}
		})
	}
}

// A show id nobody has used must be invisible to every non-admin, because
// "hidden" and "absent" have to answer the same on every route that uses this.
func TestShowVisibilityFailsClosedOnAMissingShow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	const absent = uint(99999999)
	for _, v := range []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: 7}},
	} {
		if shared.ShowVisibleTo(td.DB, absent, v.viewer) {
			t.Errorf("a show id that does not exist is visible to %s", v.name)
		}
		predSQL, predArgs := shared.VisibleShowPredicateSQL("shows", v.viewer)
		if got := countShowsMatching(t, td.DB, absent, predSQL, predArgs); got != 0 {
			t.Errorf("VisibleShowPredicateSQL matched %d rows for a missing show as %s", got, v.name)
		}
	}
}

// Show id 0 is the zero value a caller reaches this with when an id failed to
// parse or was never set. It is never a real row, and answering "visible"
// for it would open every route that gates on this to an unset parameter.
func TestShowVisibilityRefusesTheZeroID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	if shared.ShowVisibleTo(td.DB, 0, contracts.ShowViewer{UserID: 5}) {
		t.Error("show id 0 is visible")
	}
}

// A nil handle is a construction bug, and the safe answer to a construction bug
// on a security boundary is "no".
func TestShowVisibilityFailsClosedWithoutADatabase(t *testing.T) {
	if shared.ShowVisibleTo(nil, 1, contracts.ShowViewer{UserID: 5}) {
		t.Error("a nil database answered visible")
	}
	var svc *shared.ShowVisibilityService
	if svc.ShowVisibleTo(1, contracts.ShowViewer{UserID: 5}) {
		t.Error("a nil ShowVisibilityService answered visible")
	}
}

func countShowsMatching(t *testing.T, db *gorm.DB, showID uint, cond string, args []interface{}) int64 {
	t.Helper()
	var count int64
	q := db.Model(&catalogm.Show{}).Where("id = ?", showID)
	if err := q.Where(cond, args...).Count(&count).Error; err != nil {
		t.Fatalf("count shows with %q: %v", cond, err)
	}
	return count
}

// countEntityRowMatching runs a polymorphic (entity_type, entity_id) condition
// against a one-row synthetic table rather than against comments or
// comment_subscriptions.
//
// The condition under test reads exactly two columns and nothing else, so a real
// table would add foreign keys, a moderation status and an author to a fixture
// whose only job is to carry a type beside an id. The alias is `e`, which is
// what the caller above qualifies the expressions with.
func countEntityRowMatching(t *testing.T, db *gorm.DB, entityType string, entityID uint, cond string, args []interface{}) int64 {
	t.Helper()
	sqlArgs := append([]interface{}{entityType, entityID}, args...)
	var count int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM (SELECT ?::text AS entity_type, ?::bigint AS entity_id) e WHERE "+cond,
		sqlArgs...,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count entity rows with %q: %v", cond, err)
	}
	return count
}

// countUsersMatching runs a recipient condition against one real user row, which
// is what the fan-out queries it over.
func countUsersMatching(t *testing.T, db *gorm.DB, userID uint, cond string, args []interface{}) int64 {
	t.Helper()
	var count int64
	q := db.Table("users").Where("users.id = ?", userID)
	if err := q.Where(cond, args...).Count(&count).Error; err != nil {
		t.Fatalf("count users with %q: %v", cond, err)
	}
	return count
}

func countRevisionsMatching(t *testing.T, db *gorm.DB, revisionID uint, cond string, args []interface{}) int64 {
	t.Helper()
	var count int64
	q := db.Model(&adminm.Revision{}).Where("revisions.id = ?", revisionID)
	if err := q.Where(cond, args...).Count(&count).Error; err != nil {
		t.Fatalf("count revisions with %q: %v", cond, err)
	}
	return count
}
