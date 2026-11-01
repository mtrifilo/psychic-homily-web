package shared_test

import (
	"fmt"
	"testing"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// creatorState is one state of a collection creator's account.
type creatorState struct {
	name          string
	live          bool
	publicProfile bool
	apply         func(t *testing.T, db *gorm.DB, userID uint)
}

var creatorStates = []creatorState{
	{name: "live, public profile", live: true, publicProfile: true,
		apply: func(*testing.T, *gorm.DB, uint) {}},
	{name: "live, private profile", live: true, publicProfile: false,
		apply: func(t *testing.T, db *gorm.DB, userID uint) {
			testutil.SetProfileVisibility(t, db, userID, "private")
		}},
	{name: "deactivated", live: false, publicProfile: true, apply: testutil.DeactivateAccount},
	{name: "soft-deleted", live: false, publicProfile: true, apply: testutil.SoftDeleteAccount},
}

// Every SERVED spelling answers the VISIBLE truth table AND a live creator, so a
// closed creator's collection (deactivated or soft-deleted) is served to nobody,
// public or not, while the VISIBLE spellings are unmoved by the creator's
// state. A private profile changes no served answer and removes the collection
// from the public-tier listing rule.
func TestCollectionServedSpellingsRefuseAClosedCreator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	strangerID := testhelpers.CreateTestUser(td.DB).ID
	adminID := testhelpers.CreateAdminUser(td.DB).ID
	gate := shared.NewShowVisibilityService(td.DB)

	n := 0
	for _, state := range creatorStates {
		for _, isPublic := range []bool{true, false} {
			n++
			t.Run(fmt.Sprintf("%s, public=%v", state.name, isPublic), func(t *testing.T) {
				creatorID := testhelpers.CreateTestUser(td.DB).ID
				collection := testhelpers.CreateCollection(t, td.DB, creatorID,
					fmt.Sprintf("Served Matrix %d", n), fmt.Sprintf("served-matrix-%d", n), isPublic)
				itemID := createCollectionItem(t, td.DB, collection.ID, creatorID)
				state.apply(t, td.DB, creatorID)

				viewers := []struct {
					name   string
					viewer contracts.ShowViewer
					isSelf bool
				}{
					{"anonymous", contracts.ShowViewer{}, false},
					{"a stranger", contracts.ShowViewer{UserID: strangerID}, false},
					{"the creator", contracts.ShowViewer{UserID: creatorID}, true},
					{"an admin", contracts.ShowViewer{UserID: strangerID, IsAdmin: true}, false},
				}
				for _, v := range viewers {
					visible := isPublic || v.isSelf
					served := visible && state.live

					if got := shared.CollectionVisibleTo(td.DB, collection.ID, v.viewer); got != visible {
						t.Errorf("CollectionVisibleTo for %s = %v, want %v (the visible tier ignores the creator)", v.name, got, visible)
					}
					visibleSQL, visibleArgs := shared.VisibleCollectionPredicateSQL("collections", v.viewer)
					if got := countCollectionsMatching(t, td.DB, collection.ID, visibleSQL, visibleArgs) > 0; got != visible {
						t.Errorf("VisibleCollectionPredicateSQL for %s = %v, want %v", v.name, got, visible)
					}

					checks := map[string]bool{}
					checks["CollectionServedTo"] = shared.CollectionServedTo(td.DB, collection.ID, v.viewer)
					checks["ShowVisibilityService.CollectionServedTo"] = gate.CollectionServedTo(collection.ID, v.viewer)
					checks["EntityVisibleTo"] = shared.EntityVisibleTo(gate, shared.CommentEntityTypeCollection, collection.ID, v.viewer)

					predSQL, predArgs := shared.ServedCollectionPredicateSQL("collections", v.viewer)
					checks["ServedCollectionPredicateSQL"] = countCollectionsMatching(t, td.DB, collection.ID, predSQL, predArgs) > 0
					fenceSQL, fenceArgs := shared.EntityIdentityFenceSQL(shared.CommentEntityTypeCollection, "collections", v.viewer)
					checks["EntityIdentityFenceSQL"] = countCollectionsMatching(t, td.DB, collection.ID, fenceSQL, fenceArgs) > 0

					existsSQL, existsArgs := shared.ServedCollectionExistsSQL("e.entity_id", v.viewer)
					checks["ServedCollectionExistsSQL"] = countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, existsSQL, existsArgs) > 0
					armSQL, armArgs := shared.ServedCollectionCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
					checks["ServedCollectionCommentEntitySQL"] = countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, armSQL, armArgs) > 0
					compositeSQL, compositeArgs := shared.VisibleCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
					checks["VisibleCommentEntitySQL"] = countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, compositeSQL, compositeArgs) > 0
					entitySQL, entityArgs := shared.VisibleEntityExistsSQL(shared.CommentEntityTypeCollection, "e.entity_id", v.viewer)
					checks["VisibleEntityExistsSQL"] = countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, entitySQL, entityArgs) > 0

					itemSQL, itemArgs := shared.ServedCollectionItemExistsSQL("e.entity_id", v.viewer)
					checks["ServedCollectionItemExistsSQL"] = countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, itemID, itemSQL, itemArgs) > 0
					textSQL, textArgs := shared.ServedCollectionTextIDExistsSQL("meta.value", v.viewer)
					checks["ServedCollectionTextIDExistsSQL"] = countTextExprMatching(t, td.DB, fmt.Sprint(collection.ID), textSQL, textArgs) > 0
					slugSQL, slugArgs := shared.ServedCollectionSlugExistsSQL("meta.value", v.viewer)
					checks["ServedCollectionSlugExistsSQL"] = countTextExprMatching(t, td.DB, collection.Slug, slugSQL, slugArgs) > 0

					for name, got := range checks {
						if got != served {
							t.Errorf("%s for %s = %v, want %v", name, v.name, got, served)
						}
					}
				}

				// The anonymous public-tier arms bind nothing and must agree with
				// the anonymous served answer.
				publicArms := shared.PublicEntityTypeArmsSQL("e.entity_type", "e.entity_id")
				wantPublicServed := isPublic && state.live
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID, publicArms, nil) > 0; got != wantPublicServed {
					t.Errorf("PublicEntityTypeArmsSQL = %v, want %v", got, wantPublicServed)
				}

				// The fan-out: nobody receives a closed creator's collection.
				recipientSQL, recipientArgs := shared.ServedCollectionRecipientsSQL(collection.ID, "users.id")
				for _, r := range []struct {
					name   string
					userID uint
					isSelf bool
				}{{"the creator", creatorID, true}, {"a stranger", strangerID, false}, {"an admin", adminID, false}} {
					want := (isPublic || r.isSelf) && state.live
					if got := countUsersMatching(t, td.DB, r.userID, recipientSQL, recipientArgs) > 0; got != want {
						t.Errorf("ServedCollectionRecipientsSQL for %s = %v, want %v", r.name, got, want)
					}
				}

				// The public-tier listing rule: served to the public AND a public
				// profile.
				wantListed := isPublic && state.live && state.publicProfile
				if got := countCollectionsMatching(t, td.DB, collection.ID, shared.PublicListedCollectionSQL("collections"), nil) > 0; got != wantListed {
					t.Errorf("PublicListedCollectionSQL = %v, want %v", got, wantListed)
				}
				if got := countEntityRowMatching(t, td.DB, shared.CommentEntityTypeCollection, collection.ID,
					shared.PublicListedCollectionExistsSQL("e.entity_id"), nil) > 0; got != wantListed {
					t.Errorf("PublicListedCollectionExistsSQL = %v, want %v", got, wantListed)
				}

				// Reopening serves it again: nothing was scrubbed.
				if !state.live {
					testutil.ReopenAccount(t, td.DB, creatorID)
					if got := shared.CollectionServedTo(td.DB, collection.ID, contracts.ShowViewer{}); got != isPublic {
						t.Errorf("CollectionServedTo after reopening = %v, want %v", got, isPublic)
					}
				}
			})
		}
	}
}

// CollectionServedTo fails closed without a database, on a zero id and on a
// collection that does not exist, as CollectionVisibleTo does.
func TestCollectionServedToFailsClosed(t *testing.T) {
	if shared.CollectionServedTo(nil, 1, contracts.ShowViewer{UserID: 5}) {
		t.Error("a nil database answered served")
	}
	if testing.Short() {
		return
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	if shared.CollectionServedTo(td.DB, 0, contracts.ShowViewer{UserID: 5}) {
		t.Error("collection id 0 answered served")
	}
	if shared.CollectionServedTo(td.DB, 99999999, contracts.ShowViewer{}) {
		t.Error("a collection that does not exist answered served")
	}
}
