package routes

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

const (
	closedProfileUsername        = "closed-profile-target"
	closedProfileUnknownUsername = "a-username-nobody-holds"
	closedProfileSectionTitle    = "Records From A Closed Account"
	closedProfileCollectionTitle = "Crates From A Closed Account"
)

// closedProfileCollectionPages are the two paths of the username-addressed
// collections page, which answers an unknown username with an empty page.
var closedProfileCollectionPages = []string{"/collections", "/crates"}

// closedProfileReads are the username-addressed reads a logged-out visitor and
// a signed-in stranger reach.
var closedProfileReads = []string{
	"/users/%s",
	"/users/%s/contributions",
	"/users/%s/sections",
	"/users/%s/activity-heatmap",
	"/users/%s/rankings",
	"/users/%s/following",
	"/users/%s/field-notes",
	"/users/%s/followers",
}

// A deactivated or soft-deleted account answers every /users/{username} route
// exactly as a username nobody holds: each read is the unknown username's 404
// for a logged-out visitor and for a signed-in stranger, the collections page
// is the unknown username's empty page, and a follow is refused with the same
// 404 and writes nothing. A follower can still remove their own follow, which
// a stranger's unfollow cannot use to learn the account exists. Reopening the
// account serves every route again.
func TestClosedAccountAnswersEveryUsernameRouteLikeAnUnknownOne(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	target := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	if err := td.DB.Model(target).Update("username", closedProfileUsername).Error; err != nil {
		t.Fatalf("set username: %v", err)
	}
	follower := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	// Percentile rankings need ten live accounts, so the live target's
	// rankings answer 200 rather than "Rankings not available".
	for i := 0; i < 10; i++ {
		testhelpers.CreateUserWithTier(td.DB, "new_user")
	}
	testhelpers.CreateCollection(t, td.DB, target.ID, closedProfileCollectionTitle, "crates-from-a-closed-account", true)

	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		return doRequest(t, router, method, path, credential, body)
	}
	targetToken := mintToken(t, sc, target)
	followerToken := mintToken(t, sc, follower)
	strangerToken := mintToken(t, sc, stranger)

	sectionBody := []byte(fmt.Sprintf(`{"title":%q,"content":"side one","position":0}`, closedProfileSectionTitle))
	if code, body := do(t, http.MethodPost, "/auth/profile/sections", targetToken, sectionBody); code != http.StatusOK {
		t.Fatalf("create the target's section = %d; body: %s", code, body)
	}

	followPath := func(username string) string { return "/users/" + username + "/follow" }
	followRows := func(t *testing.T, userID uint) int64 {
		t.Helper()
		var n int64
		if err := td.DB.Model(&engagementm.UserBookmark{}).
			Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
				userID, "user", target.ID, engagementm.BookmarkActionFollow).
			Count(&n).Error; err != nil {
			t.Fatalf("count follow rows: %v", err)
		}
		return n
	}

	assertServed := func(t *testing.T, when string) {
		t.Helper()
		for _, credential := range []string{"", strangerToken} {
			for _, route := range closedProfileReads {
				path := fmt.Sprintf(route, closedProfileUsername)
				if code, body := do(t, http.MethodGet, path, credential, nil); code != http.StatusOK {
					t.Errorf("%s: GET %s = %d, want 200; body: %s", when, path, code, body)
				}
			}
			if _, body := do(t, http.MethodGet, "/users/"+closedProfileUsername+"/sections", credential, nil); !strings.Contains(string(body), closedProfileSectionTitle) {
				t.Errorf("%s: the sections lost the section; body: %s", when, body)
			}
			for _, page := range closedProfileCollectionPages {
				if _, body := do(t, http.MethodGet, "/users/"+closedProfileUsername+page, credential, nil); !strings.Contains(string(body), closedProfileCollectionTitle) {
					t.Errorf("%s: %s lost the collection; body: %s", when, page, body)
				}
			}
		}
	}

	if code, body := do(t, http.MethodPost, followPath(closedProfileUsername), followerToken, nil); code != http.StatusOK {
		t.Fatalf("follow the live target = %d; body: %s", code, body)
	}
	assertServed(t, "while the account is live")

	for _, state := range []struct {
		name  string
		close func(t *testing.T)
	}{
		{"deactivated", func(t *testing.T) { testutil.DeactivateAccount(t, td.DB, target.ID) }},
		{"soft-deleted", func(t *testing.T) { testutil.SoftDeleteAccount(t, td.DB, target.ID) }},
	} {
		t.Run(state.name, func(t *testing.T) {
			if followRows(t, follower.ID) != 1 {
				t.Fatal("the follower's row is missing before the account closes")
			}
			state.close(t)

			for _, credential := range []string{"", strangerToken} {
				for _, route := range closedProfileReads {
					path := fmt.Sprintf(route, closedProfileUsername)
					code, body := do(t, http.MethodGet, path, credential, nil)
					unknownCode, unknownBody := do(t, http.MethodGet, fmt.Sprintf(route, closedProfileUnknownUsername), credential, nil)
					if code != http.StatusNotFound || code != unknownCode || !sameErrorMessage(body, unknownBody) {
						t.Errorf("GET %s = %d %s; an unknown username's = %d %s", path, code, body, unknownCode, unknownBody)
					}
				}
				for _, page := range closedProfileCollectionPages {
					code, body := do(t, http.MethodGet, "/users/"+closedProfileUsername+page, credential, nil)
					unknownCode, unknownBody := do(t, http.MethodGet, "/users/"+closedProfileUnknownUsername+page, credential, nil)
					if code != unknownCode || !bytes.Equal(body, unknownBody) {
						t.Errorf("%s = %d %s; an unknown username's = %d %s", page, code, body, unknownCode, unknownBody)
					}
				}
			}

			code, body := do(t, http.MethodPost, followPath(closedProfileUsername), strangerToken, nil)
			unknownCode, unknownBody := do(t, http.MethodPost, followPath(closedProfileUnknownUsername), strangerToken, nil)
			if code != http.StatusNotFound || code != unknownCode || !sameErrorMessage(body, unknownBody) {
				t.Errorf("a follow = %d %s; an unknown username's = %d %s", code, body, unknownCode, unknownBody)
			}
			if n := followRows(t, stranger.ID); n != 0 {
				t.Errorf("the refused follow wrote %d rows", n)
			}

			code, body = do(t, http.MethodDelete, followPath(closedProfileUsername), strangerToken, nil)
			unknownCode, unknownBody = do(t, http.MethodDelete, followPath(closedProfileUnknownUsername), strangerToken, nil)
			if code != http.StatusNotFound || code != unknownCode || !sameErrorMessage(body, unknownBody) {
				t.Errorf("a stranger's unfollow = %d %s; an unknown username's = %d %s", code, body, unknownCode, unknownBody)
			}

			if code, body := do(t, http.MethodDelete, followPath(closedProfileUsername), followerToken, nil); code != http.StatusOK {
				t.Errorf("the follower's unfollow = %d, want 200; body: %s", code, body)
			}
			if n := followRows(t, follower.ID); n != 0 {
				t.Errorf("the follower's unfollow left %d rows", n)
			}

			testutil.ReopenAccount(t, td.DB, target.ID)
			if code, body := do(t, http.MethodPost, followPath(closedProfileUsername), followerToken, nil); code != http.StatusOK {
				t.Fatalf("follow the reopened target = %d; body: %s", code, body)
			}
			assertServed(t, "after the account is reopened")
		})
	}
}

// A username Postgres rejects as text answers every username-addressed read,
// the follow and the unfollow with the unknown username's 404, never a failed
// lookup's 500.
func TestRejectedTextUsernameAnswersEveryUsernameRouteLikeAnUnknownOne(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	caller := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	callerToken := mintToken(t, sc, caller)

	for name, username := range map[string]string{
		"NUL":           "no%00body",
		"invalid UTF-8": "no%FFbody",
	} {
		t.Run(name, func(t *testing.T) {
			type request struct{ method, route string }
			requests := []request{{http.MethodPost, "/users/%s/follow"}, {http.MethodDelete, "/users/%s/follow"}}
			for _, route := range closedProfileReads {
				requests = append(requests, request{http.MethodGet, route})
			}
			for _, r := range requests {
				code, body := doRequest(t, router, r.method, fmt.Sprintf(r.route, username), callerToken, nil)
				unknownCode, unknownBody := doRequest(t, router, r.method, fmt.Sprintf(r.route, closedProfileUnknownUsername), callerToken, nil)
				if code != http.StatusNotFound || code != unknownCode || !sameErrorMessage(body, unknownBody) {
					t.Errorf("%s %s = %d %s; an unknown username's = %d %s", r.method, fmt.Sprintf(r.route, username), code, body, unknownCode, unknownBody)
				}
			}
			for _, page := range closedProfileCollectionPages {
				code, body := doRequest(t, router, http.MethodGet, "/users/"+username+page, callerToken, nil)
				unknownCode, unknownBody := doRequest(t, router, http.MethodGet, "/users/"+closedProfileUnknownUsername+page, callerToken, nil)
				if code != unknownCode || !bytes.Equal(body, unknownBody) {
					t.Errorf("%s = %d %s; an unknown username's = %d %s", page, code, body, unknownCode, unknownBody)
				}
			}
		})
	}
}

// A follower of an account that turns its profile private can still remove
// their own follow, while a stranger's unfollow gets the unknown username's
// 404 and the profile's owner still reads it.
func TestPrivateProfileFollowerCanStillUnfollow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	target := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	if err := td.DB.Model(target).Update("username", closedProfileUsername).Error; err != nil {
		t.Fatalf("set username: %v", err)
	}
	follower := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	targetToken := mintToken(t, sc, target)
	followerToken := mintToken(t, sc, follower)
	strangerToken := mintToken(t, sc, stranger)
	followPath := "/users/" + closedProfileUsername + "/follow"
	unknownFollowPath := "/users/" + closedProfileUnknownUsername + "/follow"

	if code, body := doRequest(t, router, http.MethodPost, followPath, followerToken, nil); code != http.StatusOK {
		t.Fatalf("follow the public target = %d; body: %s", code, body)
	}
	testutil.SetProfileVisibility(t, td.DB, target.ID, "private")

	if code, body := getRequest(t, router, "/users/"+closedProfileUsername+"/followers", targetToken); code != http.StatusOK {
		t.Errorf("the owner's followers read = %d, want 200; body: %s", code, body)
	}

	code, body := doRequest(t, router, http.MethodDelete, followPath, strangerToken, nil)
	unknownCode, unknownBody := doRequest(t, router, http.MethodDelete, unknownFollowPath, strangerToken, nil)
	if code != http.StatusNotFound || code != unknownCode || !sameErrorMessage(body, unknownBody) {
		t.Errorf("a stranger's unfollow = %d %s; an unknown username's = %d %s", code, body, unknownCode, unknownBody)
	}

	if code, body := doRequest(t, router, http.MethodDelete, followPath, followerToken, nil); code != http.StatusOK {
		t.Errorf("the follower's unfollow = %d, want 200; body: %s", code, body)
	}
	var rows int64
	if err := td.DB.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			follower.ID, "user", target.ID, engagementm.BookmarkActionFollow).
		Count(&rows).Error; err != nil {
		t.Fatalf("count follow rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("the follower's unfollow left %d rows", rows)
	}
}
