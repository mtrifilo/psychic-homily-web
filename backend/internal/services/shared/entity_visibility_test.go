package shared_test

import (
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The composite's ALLOWLIST arm, run by Postgres rather than read.
//
// entity_visibility_internal_test.go proves the registry is complete and that
// its members reach the emitted string. This proves the string the database
// parses does what that string is supposed to do — which is the half that
// actually decides rows, and the half a concatenation bug breaks silently.
//
// It matters because the allowlist is the only part of the composite with no
// per-type spelling of its own to be checked against. The show arm and the
// collection arm each have their own matrix; "a row of an unregistered type is
// dropped" has only this.
func TestVisibleCommentEntitySQLDropsUnregisteredRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	viewers := []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: testhelpers.CreateTestUser(td.DB).ID}},
		// The admin case is the one a blanket bypass would break. If any spelling
		// grows `if viewer.IsAdmin { return "TRUE" }` on the composite, an
		// unregistered row starts passing and this fails.
		{"an admin", contracts.ShowViewer{UserID: testhelpers.CreateAdminUser(td.DB).ID, IsAdmin: true}},
	}

	// Ids are irrelevant here: the allowlist decides on entity_type alone, and
	// using a live id would let the row pass for a reason that is not the one
	// under test.
	const anyID = uint(1)
	unregistered := []string{"user", "scene", "tag", "radio_show", "festival_edition", ""}
	registeredAndPublic := []string{"artist", "venue", "release", "label", "festival"}

	for _, v := range viewers {
		sql, args := shared.VisibleCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)

		for _, entityType := range unregistered {
			if got := countEntityRowMatching(t, td.DB, entityType, anyID, sql, args); got != 0 {
				t.Errorf("a row of unregistered type %q was served to %s", entityType, v.name)
			}
		}
		// The control. A predicate that simply matched nothing could not pass
		// this, and without it the assertions above prove only that the SQL is
		// broken.
		for _, entityType := range registeredAndPublic {
			if got := countEntityRowMatching(t, td.DB, entityType, anyID, sql, args); got != 1 {
				t.Errorf("a row of public type %q was withheld from %s", entityType, v.name)
			}
		}
	}
}

// The fan-out spelling answers for every entity type, and the two gated arms
// disagree about admins ON PURPOSE.
//
// An admin is a recipient for a gated SHOW's comment because that gate is final
// and all three read gates grant them the show; an admin is NOT a recipient for a
// private COLLECTION's comment because no collection read grants them anything.
// Both halves are asserted here, against real user rows, because this asymmetry
// is the kind a later "make it consistent" edit removes in whichever direction
// looks tidier.
func TestCommentEntityRecipientsSQLSplitsTheAdminTierByEntityType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	adminID := testhelpers.CreateAdminUser(td.DB).ID
	creator := testhelpers.CreateTestUser(td.DB)

	// A public entity type needs no probe at all: it answers TRUE with no binds,
	// which is what lets a caller splice it in unconditionally.
	cond, args := shared.CommentEntityRecipientsSQL("artist", 1, "users.id", "users.is_admin")
	if got := countUsersMatching(t, td.DB, adminID, cond, args); got != 1 {
		t.Errorf("the fan-out withheld an artist's comment from an admin")
	}
	if cond != "TRUE" || args != nil {
		t.Errorf("CommentEntityRecipientsSQL for a public type = %q with %d args, want a bare TRUE", cond, len(args))
	}

	// THE SHOW HALF. A show nobody may read is still readable by an admin at
	// /shows/{id}, and a fan-out is final, so excluding an admin here would
	// permanently withhold a notification about something they are entitled to
	// see. A PENDING show is the gated state; the admin arm is what passes it.
	pending := testhelpers.CreatePendingShow(td.DB, creator.ID, "Fan-out Pending Show")
	showCond, showArgs := shared.CommentEntityRecipientsSQL("show", pending.ID, "users.id", "users.is_admin")
	if got := countUsersMatching(t, td.DB, adminID, showCond, showArgs); got != 1 {
		t.Error("the fan-out withheld a gated show's comment from an admin, who can read that " +
			"show at its detail route and cannot have the missing notification restored")
	}

	// THE COLLECTION HALF, which must answer the other way. No collection detail
	// or listing read grants an admin a private one, so notifying them here would
	// mail out what the detail route refuses.
	private := testhelpers.CreateCollection(t, td.DB, creator.ID, "Fan-out Private", "fan-out-private", false)
	collectionCond, collectionArgs := shared.CommentEntityRecipientsSQL(
		"collection", private.ID, "users.id", "users.is_admin")
	if got := countUsersMatching(t, td.DB, adminID, collectionCond, collectionArgs); got != 0 {
		t.Error("the fan-out named an admin as a recipient for a private collection's comment")
	}
	// The control on the same statement: the creator IS a recipient, so the
	// assertion above is about the admin tier rather than about a predicate that
	// matches nobody.
	if got := countUsersMatching(t, td.DB, creator.ID, collectionCond, collectionArgs); got != 1 {
		t.Error("the fan-out withheld a private collection's comment from its own creator")
	}
}

// The handler-boundary Go gate, the SQL row gate and the fan-out gate must agree
// about a private collection, or a route refuses what a listing still renders or
// an email still carries.
//
// Three spellings, one collection, one viewer tier that is neither the creator
// nor served by any of them.
func TestCollectionGatesAgreeAcrossSpellings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	creator := testhelpers.CreateTestUser(td.DB)
	stranger := testhelpers.CreateTestUser(td.DB)
	gate := shared.NewShowVisibilityService(td.DB)

	private := testhelpers.CreateCollection(t, td.DB, creator.ID, "Agreement Private", "agreement-private", false).ID
	public := testhelpers.CreateCollection(t, td.DB, creator.ID, "Agreement Public", "agreement-public", true).ID

	for _, c := range []struct {
		name         string
		collectionID uint
		want         bool
	}{
		{"a private collection", private, false},
		{"a public collection", public, true},
	} {
		strangerViewer := contracts.ShowViewer{UserID: stranger.ID}

		if got := shared.EntityVisibleTo(gate, "collection", c.collectionID, strangerViewer); got != c.want {
			t.Errorf("EntityVisibleTo on %s = %v, want %v", c.name, got, c.want)
		}

		rowSQL, rowArgs := shared.VisibleCommentEntitySQL("e.entity_type", "e.entity_id", strangerViewer)
		if got := countEntityRowMatching(t, td.DB, "collection", c.collectionID, rowSQL, rowArgs); (got > 0) != c.want {
			t.Errorf("the row gate on %s matched %d rows, want visible=%v", c.name, got, c.want)
		}

		fanSQL, fanArgs := shared.CommentEntityRecipientsSQL("collection", c.collectionID, "users.id", "users.is_admin")
		if got := countUsersMatching(t, td.DB, stranger.ID, fanSQL, fanArgs); (got > 0) != c.want {
			t.Errorf("the fan-out gate on %s matched %d recipients, want visible=%v", c.name, got, c.want)
		}
	}
}
