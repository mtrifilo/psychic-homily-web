package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1715: revision history mirrors the show detail route's visibility rule.
//
// GET /shows/{id} answers 404 for a show whose status is not approved unless the
// caller is an admin or the show's own submitter. Before this test, the same
// show's edit history was world-readable on three other routes, so unpublishing
// a show cost a reader one extra request rather than hiding anything.
//
// This is the only place the whole rule exists end to end. The service tests
// below the auth boundary are handed a viewer struct somebody filled in; only
// here does a real credential travel through the real middleware onto the real
// router, which is also what proves the reads were left on the optional-auth
// group. Registering them anywhere else would leave every handler and service
// test green while every submitter silently lost their own show's history.
//
// The four tiers of the acceptance criterion are all present: anonymous, an
// authenticated stranger, the submitter, and an admin. The stranger is the one
// that cannot be asserted anywhere below this file, because the service sees a
// user id and cannot tell an id that owns nothing from no id at all.

const (
	// The values the private show's history must not publish. Single-sourced so
	// a fixture and its assertion cannot drift onto different strings and pass
	// vacuously, and INDEPENDENT of each other. A summary built as
	// "moved it to " + hiddenShowTitle would contain the title, so the prose
	// assertion could never fail on its own and a response that echoed only the
	// summary would slip through as "the title check already covers it".
	hiddenShowTitle   = "House Show At My Actual Address"
	hiddenShowSummary = "rescheduled the doors and fixed the cover charge"
	openShowTitle     = "Public Show At The Rebel Lounge"
)

func TestShowRevisionVisibilityMirrorsTheDetailRoute(t *testing.T) {
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
	submitter := testhelpers.CreateTestUser(td.DB)
	stranger := testhelpers.CreateTestUser(td.DB)

	// The subject: a show its owner unpublished. Read back rather than trusted,
	// because an approved fixture would make every tier below look correct.
	hidden := testhelpers.CreateApprovedShow(td.DB, submitter.ID, hiddenShowTitle)
	if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", hidden.ID).
		Update("status", catalogm.ShowStatusPrivate).Error; err != nil {
		t.Fatalf("unpublish show: %v", err)
	}
	assertShowStatus(t, td.DB, hidden.ID, catalogm.ShowStatusPrivate)

	// The control: an approved show by the same submitter, edited by the same
	// author. Every assertion about the hidden show is paired against this one,
	// so a gate that simply broke the routes could not pass.
	open := testhelpers.CreateApprovedShow(td.DB, submitter.ID, openShowTitle)
	assertShowStatus(t, td.DB, open.ID, catalogm.ShowStatusApproved)

	record := func(show *catalogm.Show, title, summary string) {
		t.Helper()
		changes := []adminm.FieldChange{
			{Field: "title", OldValue: "Untitled", NewValue: title},
			{Field: "city", OldValue: "Tempe", NewValue: "Phoenix"},
		}
		if err := sc.Revision.RecordRevision("show", show.ID, submitter.ID, changes, summary); err != nil {
			t.Fatalf("record revision for show %d: %v", show.ID, err)
		}
	}
	record(hidden, hiddenShowTitle, hiddenShowSummary)
	// The control's prose must share no substring with the hidden show's, or the
	// raw-body assertion below would fire on the row that is supposed to survive.
	record(open, openShowTitle, "renamed it and corrected the venue")

	var hiddenRevision adminm.Revision
	if err := td.DB.Where("entity_type = ? AND entity_id = ?", "show", hidden.ID).
		First(&hiddenRevision).Error; err != nil {
		t.Fatalf("read back the hidden show's revision: %v", err)
	}

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
	// The header path is covered by revision_viewer_tier_test.go.
	get := func(t *testing.T, path, credential string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: credential})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}

	adminToken := token(admin)
	submitterToken := token(submitter)
	strangerToken := token(stranger)

	// callers names the four tiers once. Only the last two may read the hidden
	// show, which is exactly what GET /shows/{id} grants.
	callers := []struct {
		name    string
		token   string
		mayRead bool
	}{
		{"anonymous", "", false},
		{"an authenticated stranger", strangerToken, false},
		{"the submitter", submitterToken, true},
		{"an admin", adminToken, true},
	}

	hiddenHistory := fmt.Sprintf("/revisions/show/%d", hidden.ID)
	openHistory := fmt.Sprintf("/revisions/show/%d", open.ID)

	// The gate is only worth asserting if the route it mirrors behaves the way
	// this test claims. Pinned rather than assumed, so a change to the detail
	// route's rule fails here instead of silently leaving the two out of step.
	t.Run("the detail route it mirrors", func(t *testing.T) {
		for _, c := range callers {
			want := http.StatusNotFound
			if c.mayRead {
				want = http.StatusOK
			}
			if code, body := get(t, fmt.Sprintf("/shows/%d", hidden.ID), c.token); code != want {
				t.Errorf("GET the hidden show as %s = %d, want %d; body: %s", c.name, code, want, body)
			}
		}
	})

	t.Run("the entity history route", func(t *testing.T) {
		for _, c := range callers {
			code, body := get(t, hiddenHistory, c.token)

			if !c.mayRead {
				if code != http.StatusNotFound {
					t.Errorf("GET the hidden show's history as %s = %d, want 404; body: %s", c.name, code, body)
				}
				assertWithholdsShowSecrets(t, c.name, body)
				continue
			}
			if code != http.StatusOK {
				t.Errorf("GET the hidden show's history as %s = %d, want 200; body: %s", c.name, code, body)
				continue
			}
			if items := decodeHistory(t, body); len(items) != 1 {
				t.Errorf("%s got %d revisions, want 1; body: %s", c.name, len(items), body)
			} else if items[0].Summary != hiddenShowSummary {
				t.Errorf("%s got summary %q, want %q", c.name, items[0].Summary, hiddenShowSummary)
			}
		}
	})

	// An id nobody has ever used must answer exactly the way a hidden show does.
	// A 200 with an empty list here would turn the gate into an oracle: a caller
	// could sweep the id space and learn which unpublished shows exist.
	t.Run("a show id that does not exist is indistinguishable", func(t *testing.T) {
		code, body := get(t, "/revisions/show/99999999", "")
		if code != http.StatusNotFound {
			t.Errorf("GET a nonexistent show's history = %d, want 404; body: %s", code, body)
		}
		hiddenCode, hiddenBody := get(t, hiddenHistory, "")
		if code != hiddenCode || !sameErrorMessage(body, hiddenBody) {
			t.Errorf("a nonexistent show answers %d/%s but a hidden one answers %d/%s — the difference is the leak",
				code, body, hiddenCode, hiddenBody)
		}
	})

	t.Run("an approved show is unaffected", func(t *testing.T) {
		for _, c := range callers {
			code, body := get(t, openHistory, c.token)
			if code != http.StatusOK {
				t.Errorf("GET the approved show's history as %s = %d, want 200; body: %s", c.name, code, body)
				continue
			}
			if items := decodeHistory(t, body); len(items) != 1 {
				t.Errorf("%s got %d revisions on the approved show, want 1; body: %s", c.name, len(items), body)
			}
		}
	})

	// The contributions listing. Both revisions were authored by the submitter,
	// so this is also the case where authorship and visibility disagree: a caller
	// who cannot see the hidden show sees one row, not two, and the TOTAL says
	// one as well. A total that still said two would announce how many rows were
	// withheld.
	t.Run("the user revisions listing", func(t *testing.T) {
		path := fmt.Sprintf("/users/%d/revisions", submitter.ID)
		for _, c := range callers {
			code, body := get(t, path, c.token)
			if code != http.StatusOK {
				t.Fatalf("GET %s as %s = %d, want 200; body: %s", path, c.name, code, body)
			}

			var parsed adminh.GetUserRevisionsResponse
			if err := json.Unmarshal(body, &parsed.Body); err != nil {
				t.Fatalf("parse response: %v; body: %s", err, body)
			}

			want := 1
			if c.mayRead {
				want = 2
			}
			if len(parsed.Body.Revisions) != want {
				t.Errorf("%s got %d revisions, want %d; body: %s", c.name, len(parsed.Body.Revisions), want, body)
			}
			if parsed.Body.Total != int64(want) {
				t.Errorf("%s got total=%d, want %d — the total must count the rows the page contains",
					c.name, parsed.Body.Total, want)
			}
			if !c.mayRead {
				assertWithholdsShowSecrets(t, c.name, body)
			}
		}
	})

	// The route the acceptance criterion does not name. It takes an opaque
	// revision id rather than a show id, so leaving it open would leave the gap
	// intact for anyone willing to guess a number.
	t.Run("the single revision route", func(t *testing.T) {
		path := fmt.Sprintf("/revisions/%d", hiddenRevision.ID)
		for _, c := range callers {
			code, body := get(t, path, c.token)

			if !c.mayRead {
				if code != http.StatusNotFound {
					t.Errorf("GET the hidden show's revision as %s = %d, want 404; body: %s", c.name, code, body)
				}
				assertWithholdsShowSecrets(t, c.name, body)
				continue
			}
			if code != http.StatusOK {
				t.Errorf("GET the hidden show's revision as %s = %d, want 200; body: %s", c.name, code, body)
			}
		}
	})

	// Suppression, not scrubbing. Rollback restores stored values, so a read
	// that rewrote the row would break the moderation action the history exists
	// to support. Asserted last, after every tier has read it.
	t.Run("no read rewrote the stored row", func(t *testing.T) {
		var after adminm.Revision
		if err := td.DB.First(&after, hiddenRevision.ID).Error; err != nil {
			t.Fatalf("re-read revision: %v", err)
		}
		if !strings.Contains(string(*after.FieldChanges), hiddenShowTitle) {
			t.Errorf("stored field_changes lost the real title: %s", *after.FieldChanges)
		}
		if after.Summary == nil || *after.Summary != hiddenShowSummary {
			t.Errorf("stored summary was rewritten by a read: %v", after.Summary)
		}
	})
}

// assertWithholdsShowSecrets checks the RAW BODY, not a decoded field. A
// field-by-field check would pass if a future writer echoed the value under
// another key, or into an error message.
func assertWithholdsShowSecrets(t *testing.T, caller string, body []byte) {
	t.Helper()
	for _, secret := range []string{hiddenShowTitle, hiddenShowSummary} {
		if strings.Contains(string(body), secret) {
			t.Errorf("response published %q to %s; body: %s", secret, caller, body)
		}
	}
}

func decodeHistory(t *testing.T, body []byte) []adminh.RevisionResponseItem {
	t.Helper()
	var parsed adminh.GetEntityHistoryResponse
	if err := json.Unmarshal(body, &parsed.Body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, body)
	}
	return parsed.Body.Revisions
}

// sameErrorMessage compares two huma error bodies by their human-readable
// fields. Ids and instance paths differ per request and are not part of what a
// caller could learn from.
func sameErrorMessage(a, b []byte) bool {
	read := func(raw []byte) string {
		var e struct {
			Title  string `json:"title"`
			Status int    `json:"status"`
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return string(raw)
		}
		return fmt.Sprintf("%d/%s/%s", e.Status, e.Title, e.Detail)
	}
	return read(a) == read(b)
}

// assertShowStatus reads the fixture's status back out of the database rather
// than trusting the struct that created it. A fixture seeded into the wrong
// state would make the gate under test never engage, and every assertion in
// this file would pass for the wrong reason.
func assertShowStatus(t *testing.T, db *gorm.DB, showID uint, want catalogm.ShowStatus) {
	t.Helper()
	var got struct{ Status catalogm.ShowStatus }
	if err := db.Model(&catalogm.Show{}).Select("status").Where("id = ?", showID).
		Scan(&got).Error; err != nil {
		t.Fatalf("read back show %d status: %v", showID, err)
	}
	if got.Status != want {
		t.Fatalf("show %d status = %q, want %q", showID, got.Status, want)
	}
}
