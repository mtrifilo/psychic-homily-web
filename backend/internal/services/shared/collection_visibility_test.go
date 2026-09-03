package shared_test

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The collection rule has six spellings and they are only useful if they agree,
// for the reason the show file's header gives: the SQL forms are strings
// assembled by concatenation, and the one that decides a security boundary is the
// one Postgres parses, not the one a reviewer reads.
//
// The matrix is smaller than the show one because the state is smaller: one
// boolean instead of a four-value enum. It is checked against every viewer TIER
// including a real admin, because the admin column is where this rule differs
// from its show twin and a difference nothing reads is a difference a later edit
// deletes for free.

// collectionCase is one row of the matrix: a collection in some state, and who
// created it.
type collectionCase struct {
	name      string
	isPublic  bool
	byCreator bool
}

var collectionCases = []collectionCase{
	{"public, created by the viewer", true, true},
	{"public, created by somebody else", true, false},
	{"private, created by the viewer", false, true},
	{"private, created by somebody else", false, false},
}

// wantCollectionVisible is the rule, written out once as a truth table rather
// than as a second implementation, so a shared bug in the spellings cannot make
// them agree with each other and be wrong together.
//
// NO ADMIN PARAMETER, and its absence is the assertion. GetBySlug, GetByID,
// GetCollectionGraph, CloneCollection, Subscribe, Like and Unlike all refuse an
// admin who is not the creator, so a gate that granted one would be more
// permissive than every route it mirrors. The caller passes admin viewers
// through this same function, which is what pins it.
func wantCollectionVisible(c collectionCase, viewerIsCreator bool) bool {
	if c.isPublic {
		return true
	}
	return c.byCreator && viewerIsCreator
}

func TestCollectionVisibilitySpellingsAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// Real user rows: collections.creator_id carries a foreign key, and a matrix
	// built on invented ids would fail to insert rather than exercise the rule.
	//
	// FOUR users. The stranger has to be somebody who created nothing — reusing
	// the "somebody else" creator as the stranger makes the stranger the owner on
	// half the matrix, and those rows then pass for the wrong reason. The admin
	// is a REAL row with is_admin set, because the recipient spelling reads the
	// users table and a viewer struct cannot stand in for that.
	viewerID := testhelpers.CreateTestUser(td.DB).ID
	otherID := testhelpers.CreateTestUser(td.DB).ID
	strangerID := testhelpers.CreateTestUser(td.DB).ID
	adminID := testhelpers.CreateAdminUser(td.DB).ID

	viewers := []struct {
		name   string
		viewer contracts.ShowViewer
		isSelf bool
	}{
		{"anonymous", contracts.ShowViewer{}, false},
		{"an authenticated stranger", contracts.ShowViewer{UserID: strangerID}, false},
		{"the creator", contracts.ShowViewer{UserID: viewerID}, true},
		// Carries IsAdmin AND a stranger's id, so this row fails if any spelling
		// grows an admin bypass — the answer must be the stranger's answer.
		{"an admin", contracts.ShowViewer{UserID: strangerID, IsAdmin: true}, false},
	}

	for i, c := range collectionCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			creator := otherID
			if c.byCreator {
				creator = viewerID
			}
			collection := testhelpers.CreateCollection(t, td.DB, creator,
				fmt.Sprintf("Matrix Collection %d", i),
				fmt.Sprintf("matrix-collection-%d", i), c.isPublic)

			for _, v := range viewers {
				want := wantCollectionVisible(c, v.isSelf)

				if got := shared.CollectionVisibleTo(td.DB, collection.ID, v.viewer); got != want {
					t.Errorf("CollectionVisibleTo for %s = %v, want %v", v.name, got, want)
				}

				predicateSQL, predicateArgs := shared.VisibleCollectionPredicateSQL("collections", v.viewer)
				if got := countCollectionsMatching(t, td.DB, collection.ID, predicateSQL, predicateArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionPredicateSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				existsSQL, existsArgs := shared.VisibleCollectionExistsSQL("e.entity_id", v.viewer)
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, existsSQL, existsArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionExistsSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				entitySQL, entityArgs := shared.VisibleCollectionCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, entitySQL, entityArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionCommentEntitySQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}
				// A row naming a SHOW with the same id must pass the collection
				// arm untouched. Without this the arm could be gating on the id
				// alone and the matrix would never notice.
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeShow, collection.ID, entitySQL, entityArgs); got != 1 {
					t.Errorf("VisibleCollectionCommentEntitySQL withheld a SHOW-typed row from %s", v.name)
				}

				// The composite. Its allowlist arm and its show arm are exercised
				// in entity_visibility_test.go; here it is checked to agree with
				// the collection arm it contains, for a collection-typed row.
				compositeSQL, compositeArgs := shared.VisibleCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, compositeSQL, compositeArgs); (got > 0) != want {
					t.Errorf("VisibleCommentEntitySQL for %s matched %d collection rows, want visible=%v", v.name, got, want)
				}
			}

			// The recipient form reads its viewer from a COLUMN, so it is checked
			// against real user rows. It takes no admin expression at all, which
			// is why the admin row's expectation is the stranger's expectation:
			// this gate is final, and an admin who was mailed a private
			// collection's comment could not have that message withdrawn.
			recipients := []struct {
				name   string
				userID uint
				isSelf bool
			}{
				{"the creator", viewerID, true},
				{"a stranger", strangerID, false},
				{"an admin", adminID, false},
			}
			recipientSQL, recipientArgs := shared.VisibleCollectionRecipientsSQL(collection.ID, "users.id")
			// The both-columns form, which the digest cycle splices into a query
			// that already joins the collection and the subscriber. It is the
			// predicate the EXISTS above wraps, so a disagreement here means the
			// wrapper and the mailer decide a collection differently.
			pairSQL := shared.VisibleCollectionRecipientPredicateSQL("collections", "users.id")
			for _, r := range recipients {
				want := wantCollectionVisible(c, r.isSelf)
				if got := countUsersMatching(t, td.DB, r.userID, recipientSQL, recipientArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionRecipientsSQL for %s matched %d rows, want visible=%v", r.name, got, want)
				}
				if got := countCollectionUserPairsMatching(t, td.DB, collection.ID, r.userID, pairSQL); (got > 0) != want {
					t.Errorf("VisibleCollectionRecipientPredicateSQL for %s matched %d rows, want visible=%v", r.name, got, want)
				}
			}

			// The ITEM spelling, which decides a collection_items id rather than a
			// collections id. Same truth table: an item is exactly as visible as
			// the collection holding it.
			itemID := createCollectionItem(t, td.DB, collection.ID, creator)
			for _, v := range viewers {
				want := wantCollectionVisible(c, v.isSelf)
				itemSQL, itemArgs := shared.VisibleCollectionItemExistsSQL("e.entity_id", v.viewer)
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, itemID, itemSQL, itemArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionItemExistsSQL for %s matched %d rows, want visible=%v", v.name, got, want)
				}

				// The TEXT-ID and SLUG spellings, which read their reference out
				// of an audit row's JSON metadata rather than from a typed
				// column. Same truth table, evaluated in the shape the
				// contributions timeline evaluates them in: a text expression
				// beside the enclosing query.
				textSQL, textArgs := shared.VisibleCollectionTextIDExistsSQL("meta.value", v.viewer)
				if got := countTextExprMatching(t, td.DB, fmt.Sprint(collection.ID), textSQL, textArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionTextIDExistsSQL for %s matched %d rows, want visible=%v",
						v.name, got, want)
				}
				slugSQL, slugArgs := shared.VisibleCollectionSlugExistsSQL("meta.value", v.viewer)
				if got := countTextExprMatching(t, td.DB, collection.Slug, slugSQL, slugArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionSlugExistsSQL for %s matched %d rows, want visible=%v",
						v.name, got, want)
				}
			}
		})
	}
}

// A collection id that matches no row must answer exactly like a private one, or
// the pair is an enumeration oracle over a dense id space. Collections are
// HARD-deleted, so this pair is reachable in production rather than theoretical.
func TestCollectionVisibilityFailsClosedOnAMissingCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	const absent = uint(99999999)
	viewers := []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: testhelpers.CreateTestUser(td.DB).ID}},
		{"an admin", contracts.ShowViewer{UserID: testhelpers.CreateAdminUser(td.DB).ID, IsAdmin: true}},
	}
	for _, v := range viewers {
		if shared.CollectionVisibleTo(td.DB, absent, v.viewer) {
			t.Errorf("a collection that does not exist answered visible to %s", v.name)
		}
		existsSQL, existsArgs := shared.VisibleCollectionExistsSQL("e.entity_id", v.viewer)
		if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, absent, existsSQL, existsArgs); got != 0 {
			t.Errorf("VisibleCollectionExistsSQL matched a missing collection for %s", v.name)
		}
	}
}

// The zero id is refused without a query. A caller that lost an id somewhere up
// the stack must not be handed a lookup that happens to match nothing today.
func TestCollectionVisibilityRefusesTheZeroID(t *testing.T) {
	if shared.CollectionVisibleTo(nil, 0, contracts.ShowViewer{UserID: 5}) {
		t.Error("collection id 0 answered visible")
	}
}

// A nil handle and a nil service both refuse. The nil receiver is reachable only
// from a construction bug, and the safe answer to a construction bug on a
// security boundary is "no".
func TestCollectionVisibilityFailsClosedWithoutADatabase(t *testing.T) {
	if shared.CollectionVisibleTo(nil, 1, contracts.ShowViewer{UserID: 5}) {
		t.Error("a nil database answered visible")
	}
	var svc *shared.ShowVisibilityService
	if svc.CollectionVisibleTo(1, contracts.ShowViewer{UserID: 5}) {
		t.Error("a nil ShowVisibilityService answered visible for a collection")
	}
}

// countCollectionUserPairsMatching runs a both-columns condition over one real
// collection row joined to one real user row, which is the shape the digest
// cycle evaluates it in.
func countCollectionUserPairsMatching(t *testing.T, db *gorm.DB, collectionID, userID uint, cond string) int64 {
	t.Helper()
	var count int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM collections, users WHERE collections.id = ? AND users.id = ? AND "+cond,
		collectionID, userID,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count collection/user pairs with %q: %v", cond, err)
	}
	return count
}

// createCollectionItem inserts one item into a collection and returns its id.
// The item's own id is what the audit writers store under entity_type
// "collection" for the item actions, so the item spelling has to be exercised
// against a real row rather than an invented number.
func createCollectionItem(t *testing.T, db *gorm.DB, collectionID, addedByUserID uint) uint {
	t.Helper()
	item := communitym.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    "artist",
		EntityID:      1,
		AddedByUserID: addedByUserID,
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create collection item: %v", err)
	}
	return item.ID
}

func countCollectionsMatching(t *testing.T, db *gorm.DB, collectionID uint, cond string, args []interface{}) int64 {
	t.Helper()
	var count int64
	q := db.Model(&communitym.Collection{}).Where("collections.id = ?", collectionID)
	if err := q.Where(cond, args...).Count(&count).Error; err != nil {
		t.Fatalf("count collections with %q: %v", cond, err)
	}
	return count
}

// countTextExprMatching evaluates a condition written against a TEXT expression,
// in the shape the contributions timeline evaluates it in: the value sits in a
// column beside the enclosing query rather than in a typed id column.
func countTextExprMatching(t *testing.T, db *gorm.DB, value string, cond string, args []interface{}) int64 {
	t.Helper()
	var count int64
	// The value binds FIRST, because placeholders are positional and the
	// subselect that produces the column is written before the condition.
	all := append([]interface{}{value}, args...)
	if err := db.Raw(
		"SELECT COUNT(*) FROM (SELECT ?::text AS value) AS meta WHERE "+cond, all...,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count text-expression matches with %q: %v", cond, err)
	}
	return count
}

// THE TEXT-ID SPELLING MUST NOT RAISE ON A VALUE THAT IS NOT A NUMBER. It reads
// a JSON field, so anything can be in it, and the statement it sits in decides an
// anonymous route: a cast that raised would take the whole timeline down rather
// than withhold one row.
func TestVisibleCollectionTextIDIsTotalOverItsInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	creator := testhelpers.CreateTestUser(td.DB)
	collection := testhelpers.CreateCollection(t, td.DB, creator.ID,
		"Text ID Total", "text-id-total", true)

	viewer := contracts.ShowViewer{UserID: creator.ID}
	cond, args := shared.VisibleCollectionTextIDExistsSQL("meta.value", viewer)

	for _, value := range []string{
		"",
		"not-a-number",
		"12x",
		" 1",
		"1 ",
		"-1",
		"1.0",
		// Longer than any bigint, which is what the digit cap in the guard is for:
		// an unbounded cast would overflow and raise here.
		"999999999999999999999999999999",
	} {
		if got := countTextExprMatching(t, td.DB, value, cond, args); got != 0 {
			t.Errorf("the text-id spelling matched %d rows for %q, want 0", got, value)
		}
	}

	// The control: the real id still matches, so the arm above is measuring the
	// guard rather than a condition that matches nothing at all.
	if got := countTextExprMatching(t, td.DB, fmt.Sprint(collection.ID), cond, args); got != 1 {
		t.Errorf("the text-id spelling matched %d rows for the real id, want 1", got)
	}
}
