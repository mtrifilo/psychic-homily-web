package routes

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

const (
	closedCreatorCollectionTitle = "Mixtapes From A Closed Account"
	closedCreatorCollectionSlug  = "mixtapes-from-a-closed-account"
	closedCreatorComment         = "the second side was recorded off the radio"
	closedCreatorTag             = "closed-account-mixtape"
)

// A PUBLIC collection whose creator is closed (deactivated or soft-deleted)
// answers every route a viewer reaches exactly as a collection that does not
// exist: the comment thread, a comment by id, the tag list, the report route,
// the polymorphic subscribe and every slug write; and it leaves the inbox and
// the watching list. Reopening the account serves all of it again.
func TestClosedCreatorCollectionAnswersLikeAMissingOne(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	creator := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	commenter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	artist := testhelpers.CreateArtist(td.DB, "Closed Creator Route Artist")

	closed := testhelpers.CreateCollection(t, td.DB, creator.ID,
		closedCreatorCollectionTitle, closedCreatorCollectionSlug, true)
	if err := td.DB.Model(closed).Update("collaborative", true).Error; err != nil {
		t.Fatalf("make collection collaborative: %v", err)
	}
	seedEntityTag(t, td.DB, "collection", closed.ID, closedCreatorTag, creator.ID)

	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		return doRequest(t, router, method, path, credential, body)
	}
	get := func(t *testing.T, path, credential string) (int, []byte) {
		return getRequest(t, router, path, credential)
	}
	strangerToken := mintToken(t, sc, stranger)

	// The stranger watches the thread while the creator is live, then a comment
	// fans out to them: one inbox row and one watching-list row to withhold.
	subscribePath := func(id uint) string { return fmt.Sprintf("/entities/collection/%d/subscribe", id) }
	if code, body := do(t, http.MethodPost, subscribePath(closed.ID), strangerToken, nil); code != http.StatusOK {
		t.Fatalf("subscribe to the live creator's collection = %d; body: %s", code, body)
	}
	seeder := newCommentSeeder(td.DB)
	commentID := seedEntityComment(t, seeder, commenter.ID, "collection", closed.ID, closedCreatorComment)
	fanOut(t, sc, commentID)

	const absentID = uint(99999999)
	const missingSlug = "a-slug-nobody-has-ever-used"

	assertServed := func(t *testing.T, when string) {
		t.Helper()
		if code, body := get(t, fmt.Sprintf("/entities/collection/%d/comments", closed.ID), strangerToken); code != http.StatusOK ||
			!strings.Contains(string(body), closedCreatorComment) {
			t.Errorf("%s: the thread = %d; body: %s", when, code, body)
		}
		if code, body := get(t, fmt.Sprintf("/comments/%d", commentID), strangerToken); code != http.StatusOK {
			t.Errorf("%s: the comment by id = %d; body: %s", when, code, body)
		}
		if code, body := get(t, fmt.Sprintf("/entities/collection/%d/tags", closed.ID), ""); code != http.StatusOK ||
			!strings.Contains(string(body), closedCreatorTag) {
			t.Errorf("%s: the tag list = %d; body: %s", when, code, body)
		}
		if _, body := get(t, "/me/comment-subscriptions?limit=100", strangerToken); !strings.Contains(string(body), fmt.Sprintf(`"entity_id":%d`, closed.ID)) {
			t.Errorf("%s: the watching list lost the collection; body: %s", when, body)
		}
		assertInbox(t, get, strangerToken, when, 1, 1)
	}

	assertServed(t, "while the creator is live")

	for _, state := range []struct {
		name  string
		close func(t *testing.T)
	}{
		{"deactivated", func(t *testing.T) { testutil.DeactivateAccount(t, td.DB, creator.ID) }},
		{"soft-deleted", func(t *testing.T) { testutil.SoftDeleteAccount(t, td.DB, creator.ID) }},
	} {
		t.Run(state.name, func(t *testing.T) {
			state.close(t)

			for _, credential := range []string{"", strangerToken} {
				code, body := get(t, "/collections/"+closedCreatorCollectionSlug, credential)
				if code != http.StatusNotFound {
					t.Errorf("the detail route = %d, want 404; body: %s", code, body)
				}

				code, body = get(t, fmt.Sprintf("/entities/collection/%d/comments", closed.ID), credential)
				absentCode, absentBody := get(t, fmt.Sprintf("/entities/collection/%d/comments", absentID), credential)
				if code != absentCode || strings.Contains(string(body), closedCreatorComment) {
					t.Errorf("the thread = %d %s; a missing collection's = %d %s", code, body, absentCode, absentBody)
				}

				code, body = get(t, fmt.Sprintf("/comments/%d", commentID), credential)
				absentCode, absentBody = get(t, fmt.Sprintf("/comments/%d", absentID), credential)
				if code != http.StatusNotFound || code != absentCode || !sameErrorMessage(body, absentBody) {
					t.Errorf("the comment by id = %d %s; a missing comment's = %d %s", code, body, absentCode, absentBody)
				}

				code, body = get(t, fmt.Sprintf("/entities/collection/%d/tags", closed.ID), credential)
				absentCode, _ = get(t, fmt.Sprintf("/entities/collection/%d/tags", absentID), credential)
				if code != absentCode || strings.Contains(string(body), closedCreatorTag) {
					t.Errorf("the tag list = %d %s; a missing collection's = %d", code, body, absentCode)
				}
			}

			reportBody := []byte(`{"report_type":"spam"}`)
			code, body := do(t, http.MethodPost, fmt.Sprintf("/collections/%d/report", closed.ID), strangerToken, reportBody)
			absentCode, absentBody := do(t, http.MethodPost, fmt.Sprintf("/collections/%d/report", absentID), strangerToken, reportBody)
			if code != http.StatusNotFound || code != absentCode {
				t.Errorf("the report route = %d %s; a missing collection's = %d %s", code, body, absentCode, absentBody)
			}
			if strings.Contains(string(body), closedCreatorCollectionTitle) {
				t.Errorf("the report route published the title: %s", body)
			}

			code, body = do(t, http.MethodPost, subscribePath(closed.ID), strangerToken, nil)
			absentCode, _ = do(t, http.MethodPost, subscribePath(absentID), strangerToken, nil)
			if code < 400 || code != absentCode {
				t.Errorf("the polymorphic subscribe = %d %s; a missing collection's = %d", code, body, absentCode)
			}

			addBody := []byte(fmt.Sprintf(`{"entity_type":"artist","entity_id":%d}`, artist.ID))
			tagBody := []byte(`{"tag_name":"added-after-closing"}`)
			for _, w := range []struct {
				name   string
				method string
				suffix string
				body   []byte
			}{
				{"clone", http.MethodPost, "/clone", nil},
				{"subscribe", http.MethodPost, "/subscribe", nil},
				{"like", http.MethodPost, "/like", nil},
				{"unlike", http.MethodDelete, "/like", nil},
				{"add item", http.MethodPost, "/items", addBody},
				{"add tag", http.MethodPost, "/tags", tagBody},
			} {
				code, body := do(t, w.method, "/collections/"+closedCreatorCollectionSlug+w.suffix, strangerToken, w.body)
				missingCode, _ := do(t, w.method, "/collections/"+missingSlug+w.suffix, strangerToken, w.body)
				if code != http.StatusNotFound || code != missingCode {
					t.Errorf("%s = %d %s; an unused slug's = %d", w.name, code, body, missingCode)
				}
				if strings.Contains(string(body), closedCreatorCollectionTitle) {
					t.Errorf("%s published the title: %s", w.name, body)
				}
			}

			if _, body := get(t, "/me/comment-subscriptions?limit=100", strangerToken); strings.Contains(string(body), fmt.Sprintf(`"entity_id":%d`, closed.ID)) {
				t.Errorf("the watching list still carries the closed creator's collection; body: %s", body)
			}
			assertInbox(t, get, strangerToken, "while the creator is "+state.name, 0, 0)

			var items, clones int64
			td.DB.Table("collection_items").Where("collection_id = ?", closed.ID).Count(&items)
			td.DB.Table("collections").Where("forked_from_collection_id = ?", closed.ID).Count(&clones)
			if items != 0 || clones != 0 {
				t.Errorf("a write landed while the creator was %s: %d items, %d clones", state.name, items, clones)
			}

			testutil.ReopenAccount(t, td.DB, creator.ID)
			assertServed(t, "after reopening from "+state.name)
		})
	}
}
