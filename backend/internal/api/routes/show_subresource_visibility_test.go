package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1939: a show's SUB-RESOURCES mirror the show detail route's visibility
// rule, and the per-user surfaces that publish a show's identity mirror it too.
//
// PSY-1715 closed revision history. Everything else a show owns stayed open:
// field notes carrying the address and running order, discussion, tags,
// collection membership, and the contributions timeline that publishes a gated
// show's id and title beside its submitter's name. The detail route's 404 cost a
// reader one extra request.
//
// This file is the only place the whole rule exists end to end. Below the auth
// boundary a service is handed a viewer struct somebody filled in; only here
// does a real credential travel through the real middleware onto the real
// router, which is also what proves each read was left on an optional-auth group
// rather than silently downgraded to anonymous for everybody.
//
// All four tiers are present on every route: anonymous, an authenticated
// stranger, the submitter, and an admin. The stranger is the tier no service
// test can assert, because a service sees a user id and cannot tell an id that
// owns nothing from no id at all.

const (
	// The strings the gated show must not publish. Single-sourced so a fixture
	// and its assertion cannot drift onto different values and pass vacuously,
	// and INDEPENDENT of each other so no assertion is subsumed by another.
	gatedShowTitle    = "House Show At 118 Rosewood Lane"
	gatedNoteBody     = "the back door code is on the fridge magnet"
	gatedCommentBody  = "park on the street behind the alley, not the driveway"
	gatedTagName      = "rosewood-basement"
	gatedCollectionT  = "Rosewood Lane Nights"
	openShowTitleSub  = "Public Show At The Trunk Space"
	openNoteBody      = "the monitors were loud but the mix held up"
	openCommentBody   = "great opener, worth showing up early for"
	openTagName       = "trunk-space-noise"
	openCollectionTtl = "Trunk Space Nights"
)

// subResourceCaller is one of the four tiers, and whether the detail route
// grants it the gated show.
type subResourceCaller struct {
	name    string
	token   string
	mayRead bool
}

func TestShowSubResourcesMirrorTheDetailRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	admin := testhelpers.CreateAdminUser(td.DB)
	// Tiered authors: a new user's comments and field notes land in
	// pending_review and never reach a public listing, so an untiered fixture
	// would make every read below empty and every gate look correct.
	submitter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateTestUser(td.DB)
	// A separate author for the discussion comment. Comment creation is rate
	// limited per user per entity, and a field note is a comment row: one author
	// cannot seed both on the same show inside the window.
	commenter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	// Tagging a new tag onto an entity is gated on contributor tier, so the
	// tagger is tiered. Which user tagged is irrelevant to the rule under test.
	tagger := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	submitterName := giveUsername(t, td.DB, submitter, "rosewood-submitter")

	// Both shows are dated in the past, because a field note may only be written
	// about a show that has happened.
	gated := testhelpers.CreatePastApprovedShow(td.DB, submitter.ID, gatedShowTitle, 3)
	open := testhelpers.CreatePastApprovedShow(td.DB, submitter.ID, openShowTitleSub, 3)

	// Content is attached while both shows are still approved, which is the
	// reported repro: submit a show, discuss it, then take it private. A gate
	// that only refused NEW content would leave every existing note readable.
	seed := func(show *catalogm.Show, note, comment, tag, collection string) {
		t.Helper()
		if _, err := sc.Comment.CreateFieldNote(submitter.ID, &contracts.CreateFieldNoteRequest{
			ShowID: show.ID,
			Body:   note,
		}); err != nil {
			t.Fatalf("seed field note on show %d: %v", show.ID, err)
		}
		if _, err := sc.Comment.CreateComment(commenter.ID, &contracts.CreateCommentRequest{
			EntityType: "show",
			EntityID:   show.ID,
			Body:       comment,
		}); err != nil {
			t.Fatalf("seed comment on show %d: %v", show.ID, err)
		}
		if _, err := sc.Tag.AddTagToEntity(0, tag, "show", show.ID, tagger.ID, "other"); err != nil {
			t.Fatalf("seed tag on show %d: %v", show.ID, err)
		}
		created, err := sc.Collection.CreateCollection(submitter.ID, &contracts.CreateCollectionRequest{
			Title:    collection,
			IsPublic: true,
		})
		if err != nil {
			t.Fatalf("seed collection for show %d: %v", show.ID, err)
		}
		if _, _, err := sc.Collection.AddItem(created.Slug, submitter.ID, &contracts.AddCollectionItemRequest{
			EntityType: "show",
			EntityID:   show.ID,
		}); err != nil {
			t.Fatalf("seed collection item for show %d: %v", show.ID, err)
		}
	}
	seed(gated, gatedNoteBody, gatedCommentBody, gatedTagName, gatedCollectionT)
	seed(open, openNoteBody, openCommentBody, openTagName, openCollectionTtl)

	// Now take the subject private, and read the status back rather than trust
	// the update: a fixture left approved would make every assertion below pass
	// for the wrong reason.
	if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
		Update("status", catalogm.ShowStatusPrivate).Error; err != nil {
		t.Fatalf("make show private: %v", err)
	}
	assertShowStatus(t, td.DB, gated.ID, catalogm.ShowStatusPrivate)
	assertShowStatus(t, td.DB, open.ID, catalogm.ShowStatusApproved)

	gatedCommentID := commentIDOnShow(t, td.DB, gated.ID)

	token := func(u *authm.User) string {
		t.Helper()
		tok, err := sc.JWT.CreateToken(u)
		if err != nil {
			t.Fatalf("mint token for user %d: %v", u.ID, err)
		}
		return tok
	}

	// Credentials ride the cookie, which is the carrier the product uses:
	// apiRequest sends credentials:'include' and never an Authorization header.
	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		t.Helper()
		var req *http.Request
		if body == nil {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: credential})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}
	get := func(t *testing.T, path, credential string) (int, []byte) {
		t.Helper()
		return do(t, http.MethodGet, path, credential, nil)
	}

	callers := []subResourceCaller{
		{"anonymous", "", false},
		{"an authenticated stranger", token(stranger), false},
		{"the submitter", token(submitter), true},
		{"an admin", token(admin), true},
	}

	// The route every gate below mirrors. Pinned rather than assumed, so a change
	// to the detail route's rule fails here instead of leaving the two out of
	// step.
	t.Run("the detail route it mirrors", func(t *testing.T) {
		for _, c := range callers {
			want := http.StatusNotFound
			if c.mayRead {
				want = http.StatusOK
			}
			if code, body := get(t, fmt.Sprintf("/shows/%d", gated.ID), c.token); code != want {
				t.Errorf("GET the gated show as %s = %d, want %d; body: %s", c.name, code, want, body)
			}
		}
	})

	// The empty-list routes. A gated show answers exactly like a show with no
	// content, which is also how a show id that never existed answers, so there
	// is no id to sweep for.
	//
	// Each case names the secret its own route would leak, and the raw body is
	// searched for it rather than a decoded field: a field-by-field check passes
	// if a future writer echoes the value under another key or into an error.
	emptyListRoutes := []struct {
		name       string
		path       func(showID uint) string
		gatedValue string
		openValue  string
	}{
		{
			"field notes",
			func(id uint) string { return fmt.Sprintf("/shows/%d/field-notes", id) },
			gatedNoteBody, openNoteBody,
		},
		{
			"comments",
			func(id uint) string { return fmt.Sprintf("/entities/show/%d/comments", id) },
			gatedCommentBody, openCommentBody,
		},
		{
			"tags",
			func(id uint) string { return fmt.Sprintf("/entities/show/%d/tags", id) },
			gatedTagName, openTagName,
		},
		{
			"collections",
			func(id uint) string { return fmt.Sprintf("/collections/entity/show/%d", id) },
			gatedCollectionT, openCollectionTtl,
		},
	}

	for _, r := range emptyListRoutes {
		r := r
		t.Run("the "+r.name+" route", func(t *testing.T) {
			for _, c := range callers {
				code, body := get(t, r.path(gated.ID), c.token)
				if code != http.StatusOK {
					t.Fatalf("GET the gated show's %s as %s = %d, want 200; body: %s",
						r.name, c.name, code, body)
				}
				if c.mayRead {
					if !strings.Contains(string(body), r.gatedValue) {
						t.Errorf("the gated show's %s withheld %q from %s, who may read it; body: %s",
							r.name, r.gatedValue, c.name, body)
					}
					continue
				}
				if strings.Contains(string(body), r.gatedValue) {
					t.Errorf("the gated show's %s published %q to %s; body: %s",
						r.name, r.gatedValue, c.name, body)
				}

				// Indistinguishable from a show id nobody has ever used. A
				// difference here, even in a total, restores the oracle.
				_, absentBody := get(t, r.path(99999999), c.token)
				if !sameJSON(body, absentBody) {
					t.Errorf("the gated show's %s answers %s differently from a nonexistent show: %s vs %s",
						r.name, c.name, body, absentBody)
				}
			}

			// The control. A gate that simply broke the route could not pass this.
			code, body := get(t, r.path(open.ID), "")
			if code != http.StatusOK || !strings.Contains(string(body), r.openValue) {
				t.Errorf("GET the approved show's %s anonymously = %d without %q; body: %s",
					r.name, code, r.openValue, body)
			}
		})
	}

	// Comment ids are dense and sequential, so refusing the listing is not enough
	// on its own: a caller can walk them until one lands on the gated show.
	t.Run("the single comment and thread routes", func(t *testing.T) {
		for _, path := range []string{
			fmt.Sprintf("/comments/%d", gatedCommentID),
			fmt.Sprintf("/comments/%d/thread", gatedCommentID),
		} {
			for _, c := range callers {
				code, body := get(t, path, c.token)
				if c.mayRead {
					if code != http.StatusOK {
						t.Errorf("GET %s as %s = %d, want 200; body: %s", path, c.name, code, body)
					}
					continue
				}
				if code != http.StatusNotFound {
					t.Errorf("GET %s as %s = %d, want 404; body: %s", path, c.name, code, body)
				}
				if strings.Contains(string(body), gatedCommentBody) {
					t.Errorf("GET %s published the comment body to %s; body: %s", path, c.name, body)
				}
			}
		}
	})

	// The WRITE gates. Without them a caller who cannot read the show still
	// learns its id is real, from the difference between "not found" and a
	// successful post, and can attach content to it.
	t.Run("the write routes refuse and answer like a missing show", func(t *testing.T) {
		writes := []struct {
			name string
			path func(showID uint) string
			body []byte
		}{
			{
				"a field note",
				func(id uint) string { return fmt.Sprintf("/shows/%d/field-notes", id) },
				[]byte(`{"body":"probing whether this show exists"}`),
			},
			{
				"a comment",
				func(id uint) string { return fmt.Sprintf("/entities/show/%d/comments", id) },
				[]byte(`{"body":"probing whether this show exists"}`),
			},
		}
		for _, w := range writes {
			gatedCode, gatedBody := do(t, http.MethodPost, w.path(gated.ID), token(stranger), w.body)
			if gatedCode < 400 {
				t.Errorf("POST %s to the gated show as a stranger = %d, want a refusal; body: %s",
					w.name, gatedCode, gatedBody)
			}
			const absentID = uint(99999999)
			absentCode, absentBody := do(t, http.MethodPost, w.path(absentID), token(stranger), w.body)

			// The id the caller supplied is echoed back in one of these
			// messages, and echoing a caller's own input discloses nothing. It
			// is normalised out so the comparison is about everything else: any
			// OTHER difference in status, title or detail would let a caller
			// sort real gated shows from ids that were never used.
			gatedNorm := strings.ReplaceAll(string(gatedBody), fmt.Sprint(gated.ID), "<id>")
			absentNorm := strings.ReplaceAll(string(absentBody), fmt.Sprint(absentID), "<id>")
			if gatedCode != absentCode || !sameErrorMessage([]byte(gatedNorm), []byte(absentNorm)) {
				t.Errorf("POST %s answers %d/%s for a gated show but %d/%s for a nonexistent one — the difference is the leak",
					w.name, gatedCode, gatedNorm, absentCode, absentNorm)
			}
		}
	})

	// The per-user surfaces. These are not addressed by show id at all, which is
	// why the gate on the show's own routes does not reach them: they publish the
	// gated show's title and id beside the submitter's name.
	t.Run("the contributions timeline", func(t *testing.T) {
		path := fmt.Sprintf("/users/%s/contributions", submitterName)
		for _, c := range callers {
			code, body := get(t, path, c.token)
			if code != http.StatusOK {
				t.Fatalf("GET %s as %s = %d, want 200; body: %s", path, c.name, code, body)
			}
			if c.mayRead {
				if !strings.Contains(string(body), gatedShowTitle) {
					t.Errorf("the timeline withheld the gated show from %s, who may read it; body: %s", c.name, body)
				}
				continue
			}
			if strings.Contains(string(body), gatedShowTitle) {
				t.Errorf("the timeline published %q to %s; body: %s", gatedShowTitle, c.name, body)
			}
			// The ID alone is the whole of what an enumeration oracle needs, so
			// the row must be absent rather than merely unnamed.
			if hasContributionForShow(t, body, gated.ID) {
				t.Errorf("the timeline published the gated show's id to %s; body: %s", c.name, body)
			}
			if !hasContributionForShow(t, body, open.ID) {
				t.Errorf("the timeline dropped the APPROVED show for %s; body: %s", c.name, body)
			}
		}
	})

	// The counts. A public number differenced against a filtered sibling is the
	// same disclosure stated as arithmetic, which is why these move with the
	// listings rather than staying whole.
	t.Run("the profile counts", func(t *testing.T) {
		path := "/users/" + submitterName
		for _, c := range callers {
			code, body := get(t, path, c.token)
			if code != http.StatusOK {
				t.Fatalf("GET %s as %s = %d, want 200; body: %s", path, c.name, code, body)
			}
			var parsed struct {
				Stats *struct {
					ShowsSubmitted int64 `json:"shows_submitted"`
				} `json:"stats"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("parse profile: %v; body: %s", err, body)
			}
			if parsed.Stats == nil {
				t.Fatalf("profile carried no stats for %s; body: %s", c.name, body)
			}
			want := int64(1)
			if c.mayRead {
				want = 2
			}
			if parsed.Stats.ShowsSubmitted != want {
				t.Errorf("shows_submitted for %s = %d, want %d — the count must not name shows the caller cannot see",
					c.name, parsed.Stats.ShowsSubmitted, want)
			}
		}
	})

	// The author's own field-note listing. A per-author listing, not the show's
	// page, so it reads the PUBLIC tier for everybody including the author: the
	// show's own page is where a submitter reads their own notes.
	t.Run("the authored field notes listing", func(t *testing.T) {
		path := fmt.Sprintf("/users/%s/field-notes", submitterName)
		for _, c := range callers {
			code, body := get(t, path, c.token)
			if code != http.StatusOK {
				t.Fatalf("GET %s as %s = %d, want 200; body: %s", path, c.name, code, body)
			}
			if strings.Contains(string(body), gatedNoteBody) || strings.Contains(string(body), gatedShowTitle) {
				t.Errorf("the authored notes listing published the gated show to %s; body: %s", c.name, body)
			}
			if !strings.Contains(string(body), openNoteBody) {
				t.Errorf("the authored notes listing dropped the APPROVED show's note for %s; body: %s", c.name, body)
			}
		}
	})

	// Suppression, not scrubbing. Re-approving must restore everything, because
	// the gate is evaluated at read time against the show's current status.
	t.Run("re-approving restores every sub-resource", func(t *testing.T) {
		if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
			Update("status", catalogm.ShowStatusApproved).Error; err != nil {
			t.Fatalf("re-approve show: %v", err)
		}
		for _, r := range emptyListRoutes {
			code, body := get(t, r.path(gated.ID), "")
			if code != http.StatusOK || !strings.Contains(string(body), r.gatedValue) {
				t.Errorf("after re-approval the %s route = %d without %q; body: %s",
					r.name, code, r.gatedValue, body)
			}
		}
	})
}

// giveUsername sets a username on a user so the /users/{username} family can
// address them, and returns it. CreateTestUser leaves the column null, and every
// route in that family resolves the user by it.
func giveUsername(t *testing.T, db *gorm.DB, u *authm.User, username string) string {
	t.Helper()
	if err := db.Model(&authm.User{}).Where("id = ?", u.ID).
		Update("username", username).Error; err != nil {
		t.Fatalf("set username on user %d: %v", u.ID, err)
	}
	return username
}

// hasContributionForShow reports whether the timeline names the given show id on
// a show-typed row.
func hasContributionForShow(t *testing.T, body []byte, showID uint) bool {
	t.Helper()
	var parsed struct {
		Contributions []struct {
			EntityType string `json:"entity_type"`
			EntityID   uint   `json:"entity_id"`
		} `json:"contributions"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse contributions: %v; body: %s", err, body)
	}
	for _, c := range parsed.Contributions {
		if c.EntityID == showID && strings.HasPrefix(c.EntityType, "show") {
			return true
		}
	}
	return false
}

// commentIDOnShow reads back the id of the discussion comment seeded on a show,
// so the single-comment route can be addressed by the id a sweeping caller would
// otherwise guess.
func commentIDOnShow(t *testing.T, db *gorm.DB, showID uint) uint {
	t.Helper()
	var comment engagementm.Comment
	if err := db.Where("entity_type = ? AND entity_id = ? AND kind = ?",
		engagementm.CommentEntityShow, showID, engagementm.CommentKindComment).
		First(&comment).Error; err != nil {
		t.Fatalf("read back the comment on show %d: %v", showID, err)
	}
	return comment.ID
}

// sameJSON compares two response bodies as decoded JSON, so key order and
// whitespace do not decide whether a gated show is distinguishable from an
// absent one.
func sameJSON(a, b []byte) bool {
	var av, bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return bytes.Equal(ab, bb)
}
