package shared_test

import (
	"fmt"
	"testing"

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
// one Postgres parses, not the one a reviewer reads (PSY-1987).
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
			collection := &communitym.Collection{
				Title:     fmt.Sprintf("Matrix Collection %d", i),
				Slug:      fmt.Sprintf("matrix-collection-%d", i),
				CreatorID: creator,
				IsPublic:  c.isPublic,
			}
			// Written through a map on the boolean for the GORM gotcha this
			// codebase records: false is a zero value, GORM omits it on Create,
			// and the column's DEFAULT true would win. A fixture that silently
			// stayed public would make every private row below pass for the
			// wrong reason.
			if err := td.DB.Create(collection).Error; err != nil {
				t.Fatalf("create collection: %v", err)
			}
			if err := td.DB.Model(&communitym.Collection{}).Where("id = ?", collection.ID).
				Update("is_public", c.isPublic).Error; err != nil {
				t.Fatalf("set is_public: %v", err)
			}
			assertCollectionIsPublic(t, td.DB, collection.ID, c.isPublic)

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
			for _, r := range recipients {
				want := wantCollectionVisible(c, r.isSelf)
				if got := countUsersMatching(t, td.DB, r.userID, recipientSQL, recipientArgs); (got > 0) != want {
					t.Errorf("VisibleCollectionRecipientsSQL for %s matched %d rows, want visible=%v", r.name, got, want)
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

// createTestCollection writes one collection and returns its id, with is_public
// applied through a map update and read back.
//
// Both steps are load-bearing. GORM omits a false boolean on Create because
// false is the zero value, so `IsPublic: false` alone leaves the column at its
// DEFAULT true — a fixture that silently stayed public would make a privacy
// assertion pass for the wrong reason. The read-back is what turns that from a
// thing the writer believes into a thing the database confirms.
func createTestCollection(t *testing.T, db *gorm.DB, creatorID uint, slug string, isPublic bool) uint {
	t.Helper()
	collection := &communitym.Collection{
		Title:     "Collection " + slug,
		Slug:      slug,
		CreatorID: creatorID,
		IsPublic:  isPublic,
	}
	if err := db.Create(collection).Error; err != nil {
		t.Fatalf("create collection %q: %v", slug, err)
	}
	if err := db.Model(&communitym.Collection{}).Where("id = ?", collection.ID).
		Update("is_public", isPublic).Error; err != nil {
		t.Fatalf("set is_public on %q: %v", slug, err)
	}
	assertCollectionIsPublic(t, db, collection.ID, isPublic)
	return collection.ID
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

// assertCollectionIsPublic reads the flag back out of the database rather than
// trusting the write, for the GORM boolean reason the fixture above gives.
func assertCollectionIsPublic(t *testing.T, db *gorm.DB, collectionID uint, want bool) {
	t.Helper()
	var got bool
	if err := db.Model(&communitym.Collection{}).Where("id = ?", collectionID).
		Select("is_public").Scan(&got).Error; err != nil {
		t.Fatalf("read back is_public: %v", err)
	}
	if got != want {
		t.Fatalf("collection %d has is_public=%v, want %v — the fixture did not take", collectionID, got, want)
	}
}
