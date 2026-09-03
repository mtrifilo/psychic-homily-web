package notification

import (
	"sort"
	"strings"
	"testing"

	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// Every notification_log entity_type must have a decided position on the show
// visibility rule (PSY-1983).
//
// inboxRowsVisibleTo gates rows by entity_type, and its two type lists are
// hand-maintained. The comment arm at least derives its list from the map the
// writers use; the direct arm does not, so a new family whose entity_id is a
// show id gets no gate, no failing test, and publishes a show that
// GET /shows/{id} refuses the same caller. That is exactly how this ticket's
// leak was introduced in the first place — a rule that did not travel to a
// surface added later.
//
// This is the notification-table analogue of TestEveryShowAddressableRouteHasADisposition
// in api/routes: it does not check behaviour, it checks that a DECISION EXISTS.
// The cost of adding an entity type is one line here plus the judgement it
// forces, which is the point.
//
// KNOWN LIMIT, so this is not read as more than it is: the inventory is a
// hand-written list of the constants, because Go cannot enumerate the constants
// of a package at runtime. A new entity type declared and never added here is
// invisible to this test as well — what the test catches is a type added to the
// inventory without a disposition, and a disposition that no longer matches the
// predicate. The second half is the part that actually bites, and it is checked
// against the live SQL below.

type inboxEntityDisposition int

const (
	// gatedByShowID: entity_id IS a show id, so the direct arm gates the row.
	gatedByShowID inboxEntityDisposition = iota
	// gatedByCommentParent: entity_id is a COMMENT id whose parent may be a
	// gated entity, so the comment arm gates the row through the comments
	// table. Which parents are gated is shared.VisibleCommentEntitySQL's
	// decision — a show, a collection, or a type nobody dispositioned — and this
	// arm carries no copy of that list.
	gatedByCommentParent
	// fencedElsewhere: the row reaches a show, but not through entity_id, so
	// gating the ROW would be wrong. Its show data is fenced at its own
	// enrichment site instead, and the entry names where.
	fencedElsewhere
	// reachesNoShow: entity_id names something that is not a show and cannot
	// resolve to one.
	reachesNoShow
)

var inboxEntityTypeDispositions = map[string]inboxEntityDisposition{
	// entity_id = show id. Written by the show-filter matcher and the
	// scene-follow fan-out.
	notificationm.NotificationEntityShow: gatedByShowID,
	// entity_id = show id, subject_entity_id = artist id (PSY-1896).
	notificationm.NotificationEntityArtistShowAlert: gatedByShowID,

	// entity_id = comment id; the comment names its own parent.
	engagement.NotificationEntityCommentReply:   gatedByCommentParent,
	engagement.NotificationEntityCommentMention: gatedByCommentParent,

	// entity_id = VENUE id, and the row is COALESCED over every show announced
	// at that venue on one day. Gating the row would hide the shows that are
	// still public, so the fence is inside the batch-membership query in
	// enrichVenueShowAlertNotifications instead, at the PUBLIC tier for every
	// caller (a listing that merely contains shows, per services/shared's rule).
	notificationm.NotificationEntityVenueShowAlert: fencedElsewhere,

	// entity_id = request id.
	notificationm.NotificationEntityRequestFulfillmentProposed: reachesNoShow,

	// NO ENTITY TYPE HERE CARRIES A COLLECTION ID, which is why the direct arm
	// stays show-only after PSY-1987. Collection activity reaches this table
	// only as comment_reply / comment_mention, through the comment arm. A future
	// notification family whose entity_id IS a collection id — a collection
	// follow, a collaborator invite — would need a THIRD arm and a disposition
	// of its own, and would be caught by this map's completeness check rather
	// than by an audit.
}

func TestEveryInboxEntityTypeHasADisposition(t *testing.T) {
	// Built for a non-admin viewer because that is the tier whose arms this map
	// describes. Every tier now produces real SQL: TestInboxPredicateHasNoBlanketAdminBypass
	// below fails if an admin ever short-circuits to a constant again.
	sql, _ := inboxRowsVisibleTo("nl", contracts.ShowViewer{UserID: 7})

	for entityType, disposition := range inboxEntityTypeDispositions {
		quoted := "'" + entityType + "'"
		inDirectArm := strings.Contains(showIDBearingEntityTypeList, quoted)
		inCommentArm := strings.Contains(commentNotificationEntityTypeList, quoted)

		switch disposition {
		case gatedByShowID:
			if !inDirectArm {
				t.Errorf("%q is recorded as gated by its show id but is missing from "+
					"showIDBearingEntityTypeList, so the inbox publishes it for a show the "+
					"caller cannot see", entityType)
			}
			if inCommentArm {
				t.Errorf("%q is in BOTH arms: its entity_id would be read as a comment id "+
					"and as a show id at once", entityType)
			}
		case gatedByCommentParent:
			if !inCommentArm {
				t.Errorf("%q is recorded as gated through its comment parent but is missing "+
					"from commentNotificationEntityTypeList", entityType)
			}
			if inDirectArm {
				t.Errorf("%q is in BOTH arms: its entity_id is a comment id, so the direct "+
					"arm would gate it on whether a SHOW with that comment's id is visible",
					entityType)
			}
		case fencedElsewhere, reachesNoShow:
			if inDirectArm || inCommentArm {
				t.Errorf("%q is recorded as not gated by entity_type, but appears in one of "+
					"the predicate's lists — update the disposition or the list, and say "+
					"which is true", entityType)
			}
		}

		// Whatever the disposition, the type must not have been quietly dropped
		// from the statement the database actually runs.
		if (inDirectArm || inCommentArm) && !strings.Contains(sql, quoted) {
			t.Errorf("%q is in a type list but not in the emitted predicate", entityType)
		}
	}
}

// The inventory is only a guard while it describes types that exist. An entry
// naming a constant nothing writes is a claim about nothing, and it hides the
// removal of a family the predicate still thinks it covers.
func TestInboxEntityTypeInventoryHasNoStaleEntries(t *testing.T) {
	known := map[string]bool{
		notificationm.NotificationEntityShow:                       true,
		notificationm.NotificationEntityArtistShowAlert:            true,
		notificationm.NotificationEntityVenueShowAlert:             true,
		notificationm.NotificationEntityRequestFulfillmentProposed: true,
		engagement.NotificationEntityCommentReply:                  true,
		engagement.NotificationEntityCommentMention:                true,
	}

	var stale []string
	for entityType := range inboxEntityTypeDispositions {
		if !known[entityType] {
			stale = append(stale, entityType)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("inbox disposition recorded for entity types that no writer produces: %v", stale)
	}

	var undecided []string
	for entityType := range known {
		if _, ok := inboxEntityTypeDispositions[entityType]; !ok {
			undecided = append(undecided, entityType)
		}
	}
	if len(undecided) > 0 {
		sort.Strings(undecided)
		t.Errorf("%d notification entity type(s) have no recorded position on the show "+
			"visibility rule:\n  %v\n\nEvery type whose row can lead to a show has to decide "+
			"whether a caller who cannot see that show may read it. Add each to "+
			"inboxEntityTypeDispositions with the disposition that is TRUE of it.",
			len(undecided), undecided)
	}
}

// entityTypeArm drops its gate when the type list is empty, and it MUST drop the
// gate's bind arguments with it.
//
// Returning the SQL without the args would leave the statement short of
// placeholders while the caller still supplied them, and Postgres rejects the
// whole query rather than ignoring the extras — taking out the inbox, the badge
// and both mark-read writes together. That failure needs no database to catch.
func TestEntityTypeArmDropsArgsWithItsGate(t *testing.T) {
	gateArgs := []interface{}{"approved", uint(7)}

	sql, args := entityTypeArm("nl", "", "some_gate = ?", gateArgs)
	if sql != "1 = 1" {
		t.Errorf("an empty type list produced %q, want the no-op", sql)
	}
	if len(args) != 0 {
		t.Errorf("the no-op arm kept %d bind argument(s); the statement has no placeholder "+
			"left for them and Postgres will reject the whole query", len(args))
	}

	sql, args = entityTypeArm("nl", "'show'", "some_gate = ?", gateArgs)
	if !strings.Contains(sql, "some_gate = ?") {
		t.Errorf("a populated type list dropped its gate: %q", sql)
	}
	if len(args) != len(gateArgs) {
		t.Errorf("the gated arm carried %d args, want %d", len(args), len(gateArgs))
	}
}

// An admin gets a REAL predicate, not a blanket bypass.
//
// A blanket bypass is right only while every arm judges shows, which an admin
// sees all of. The comment arm also judges COLLECTIONS, and no collection detail
// or listing read grants an admin a private one, so `1 = 1` would extend the two
// deliberate admin exceptions (the pending-comment moderation queue and the
// admin write path on PUT /collections/{slug}) to a passive feed that nobody
// decided to grant. Pinned here because the bypass is the tidier-looking code
// and a later edit will want it back.
func TestInboxPredicateHasNoBlanketAdminBypass(t *testing.T) {
	sql, _ := inboxRowsVisibleTo("nl", contracts.ShowViewer{UserID: 7, IsAdmin: true})
	if sql == "1 = 1" || sql == "TRUE" {
		t.Fatalf("the inbox predicate answers %q for an admin — a private collection's "+
			"comment activity is published to every admin inbox", sql)
	}
	if !strings.Contains(sql, "collections") {
		t.Errorf("the admin predicate does not reach the collections table: %s", sql)
	}
}

// The two lists must agree with the emitted SQL's placeholder count, per viewer
// tier. Every tier gets one bind per gate per arm; the admin tier's show arms
// fold to constants while its collection arm does not, so its count differs from
// the others and must still match its own statement.
func TestInboxPredicatePlaceholdersMatchArgs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: 7}},
		{"an admin", contracts.ShowViewer{UserID: 7, IsAdmin: true}},
	} {
		sql, args := inboxRowsVisibleTo("nl", tc.viewer)
		if got := strings.Count(sql, "?"); got != len(args) {
			t.Errorf("for %s the predicate has %d placeholder(s) and %d argument(s): %s",
				tc.name, got, len(args), sql)
		}
	}
}
