package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1717: an unverified venue's address history is masked for the public and
// served whole to an admin.
//
// This is the only place the three tiers of the acceptance criterion exist
// together. The service takes one bool and cannot tell an anonymous caller from
// a logged-in contributor; the handler resolves that bool but is handed a
// context a test wrote by hand. Only here does a real JWT travel through the
// real middleware chain on the real router, which is also the only way to prove
// the routes were registered on a group that reads credentials at all — moving
// them back to the bare rc.API would leave every handler and service test
// passing while every admin silently dropped to the public tier.
//
// Assertions are on the SERVED RESPONSE BODY, decoded through each route's OWN
// exported response type, so the test moves with the wire contract rather than
// with a local copy of it — and so a route whose shape drifts from its siblings
// stops compiling here rather than quietly losing coverage.
//
// Both credential carriers are exercised. The middleware parses the
// Authorization header and the auth_token cookie in separate code, and the
// cookie is the only one the product uses.

// Moving the three reads onto a Huma group is the operation that has previously
// dropped operations out of the published document (PSY-1554, though by way of a
// separate humachi instance rather than a group). huma.NewGroup shares the one
// API and its document, so the spec is expected to be unchanged — which is
// exactly the kind of expectation worth a cheap assertion, since a regression
// here is invisible to every functional test in this file.
//
// It also pins single registration BY SHAPE, via matching(), not by pattern
// string. chi keys on shape and silently last-wins a duplicate, so a second
// registration under a different parameter name (/revisions/{id}) is the
// collision that matters and the one an exact-string count would miss.
func TestRevisionReadRoutesStayInTheSpecExactlyOnce(t *testing.T) {
	router := newTestRouter(t)
	paths := servedSpecPaths(t, router)
	routes := chiRoutes(t, router)

	for _, want := range []struct{ shape, pattern string }{
		{"/revisions/{}/{}", "/revisions/{entity_type}/{entity_id}"},
		{"/revisions/{}", "/revisions/{revision_id}"},
		{"/users/{}/revisions", "/users/{user_id}/revisions"},
	} {
		item, ok := paths[want.pattern]
		if !ok {
			t.Errorf("%s is missing from the served spec — the optional-auth group dropped it", want.pattern)
			continue
		}
		if _, ok := item["get"]; !ok {
			t.Errorf("expected a documented GET operation for %s", want.pattern)
		}

		got := matching(routes, http.MethodGet, want.shape)
		if len(got) != 1 {
			t.Errorf("GET %s resolved to %d registrations (%v), want exactly 1 — a leftover "+
				"registration on the bare API group would serve some requests with no "+
				"credential in context", want.shape, len(got), got)
			continue
		}
		if got[0] != want.pattern {
			t.Errorf("GET %s resolved to %q, want %q — something else claimed the shape",
				want.shape, got[0], want.pattern)
		}
	}
}

// The literals a DIY venue's history is not allowed to publish, and the prose
// that repeats one of them. Single-sourced so a fixture and its assertion cannot
// drift onto different strings and pass vacuously.
// credentialCarrier names how a credential reaches the server.
// OptionalHumaJWTMiddleware reads both, from separate code, and the product only
// ever uses the cookie.
type credentialCarrier int

const (
	viaHeader credentialCarrier = iota
	viaCookie
)

const (
	tierSecretAddress = "1234 Secret St"
	tierOldAddress    = "1 Old St"
	tierOldZip        = "85003"
	tierNewZip        = "85004"
	tierSummary       = "corrected the address to " + tierSecretAddress
)

func TestRevisionHistoryViewerTiersEndToEnd(t *testing.T) {
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
	contributor := testhelpers.CreateTestUser(td.DB)

	// The subject: a venue nobody has verified, which is routinely somebody's
	// house. Read back rather than trusted, because a verified fixture would
	// make every tier below look correct.
	venue := testhelpers.CreateUnverifiedVenue(td.DB, "Unverified Room", "Phoenix", "AZ")
	var seeded struct{ Verified bool }
	if err := td.DB.Table("venues").Select("verified").Where("id = ?", venue.ID).
		Scan(&seeded).Error; err != nil {
		t.Fatalf("read back venue: %v", err)
	}
	if seeded.Verified {
		t.Fatalf("fixture venue is verified — the gate under test would not engage")
	}
	// Same for the admin fixture: an admin seeded as a contributor by accident
	// would make the whole test pass for the wrong reason.
	var seededAdmin struct{ IsAdmin bool }
	if err := td.DB.Table("users").Select("is_admin").Where("id = ?", admin.ID).
		Scan(&seededAdmin).Error; err != nil {
		t.Fatalf("read back admin: %v", err)
	}
	if !seededAdmin.IsAdmin {
		t.Fatalf("fixture admin is not an admin — the tier under test would never engage")
	}

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old Room", NewValue: "The Basement"},
		{Field: "address", OldValue: tierOldAddress, NewValue: tierSecretAddress},
		{Field: "zipcode", OldValue: tierOldZip, NewValue: tierNewZip},
	}
	if err := sc.Revision.RecordRevision("venue", venue.ID, contributor.ID, changes, tierSummary); err != nil {
		t.Fatalf("record revision: %v", err)
	}

	var stored adminm.Revision
	if err := td.DB.First(&stored).Error; err != nil {
		t.Fatalf("read back revision: %v", err)
	}

	token := func(u *authm.User) string {
		t.Helper()
		tok, err := sc.JWT.CreateToken(u)
		if err != nil {
			t.Fatalf("mint token for user %d: %v", u.ID, err)
		}
		return tok
	}

	adminToken := token(admin)
	contributorToken := token(contributor)

	// send issues the request with the credential carried the way `carrier` says.
	// Both carriers matter and they are NOT interchangeable: the middleware has
	// its own copy of the cookie parse, and the cookie is the only credential the
	// product actually uses (apiRequest sends credentials:'include' and never an
	// Authorization header), so header-only coverage would leave the production
	// path untested.
	send := func(t *testing.T, path, credential string, carrier credentialCarrier) []byte {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if credential != "" {
			switch carrier {
			case viaHeader:
				req.Header.Set("Authorization", "Bearer "+credential)
			case viaCookie:
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: credential})
			}
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200; body: %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	get := func(t *testing.T, path, bearer string) []byte {
		t.Helper()
		return send(t, path, bearer, viaHeader)
	}

	// readList decodes an entity-history response. It is NOT used for
	// /users/{id}/revisions: that route answers with its own response type, and
	// decoding it through this one would silently stop covering it the moment
	// the two shapes diverge. See readUserList.
	readList := func(t *testing.T, path, bearer string) ([]byte, adminh.RevisionResponseItem) {
		t.Helper()
		body := get(t, path, bearer)
		var parsed adminh.GetEntityHistoryResponse
		if err := json.Unmarshal(body, &parsed.Body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, body)
		}
		if len(parsed.Body.Revisions) != 1 {
			t.Fatalf("got %d revisions, want 1; body: %s", len(parsed.Body.Revisions), body)
		}
		return body, parsed.Body.Revisions[0]
	}

	readUserList := func(t *testing.T, path, bearer string) ([]byte, adminh.RevisionResponseItem) {
		t.Helper()
		body := get(t, path, bearer)
		var parsed adminh.GetUserRevisionsResponse
		if err := json.Unmarshal(body, &parsed.Body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, body)
		}
		if len(parsed.Body.Revisions) != 1 {
			t.Fatalf("got %d revisions, want 1; body: %s", len(parsed.Body.Revisions), body)
		}
		return body, parsed.Body.Revisions[0]
	}

	readOne := func(t *testing.T, path, bearer string) ([]byte, adminh.RevisionResponseItem) {
		t.Helper()
		body := get(t, path, bearer)
		var item adminh.RevisionResponseItem
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, body)
		}
		return body, item
	}

	// assertMasked is the acceptance criterion for the public tier, checked
	// against the RAW BODY first: neither address may appear anywhere in the
	// payload, through the diff or through the prose. A field-by-field check
	// alone would pass if a future writer duplicated the value under another key.
	assertMasked := func(t *testing.T, body []byte, item adminh.RevisionResponseItem) {
		t.Helper()
		for _, secret := range []string{tierSecretAddress, tierOldAddress, tierOldZip, tierNewZip} {
			if strings.Contains(string(body), secret) {
				t.Errorf("response published %q to a public caller; body: %s", secret, body)
			}
		}
		byField := map[string]adminm.FieldChange{}
		for _, c := range item.Changes {
			byField[c.Field] = c
		}
		for _, f := range []string{"address", "zipcode"} {
			if byField[f].OldValue != revisiondiff.RedactedValue || byField[f].NewValue != revisiondiff.RedactedValue {
				t.Errorf("%s = %v -> %v, want both %q", f, byField[f].OldValue, byField[f].NewValue, revisiondiff.RedactedValue)
			}
		}
		// Redaction, not deletion: the edit itself stays auditable.
		if byField["name"].NewValue != "The Basement" {
			t.Errorf("the non-private field was altered: %v", byField["name"])
		}
		if item.Summary != "" {
			t.Errorf("summary = %q, want it withheld from a public caller", item.Summary)
		}
	}

	// assertUnmasked is the other half. It asserts the exact stored values, so a
	// tier that returned an empty or partially-rebuilt diff could not pass by
	// merely failing to contain the mask string.
	assertUnmasked := func(t *testing.T, item adminh.RevisionResponseItem) {
		t.Helper()
		byField := map[string]adminm.FieldChange{}
		for _, c := range item.Changes {
			byField[c.Field] = c
		}
		if byField["address"].OldValue != tierOldAddress || byField["address"].NewValue != tierSecretAddress {
			t.Errorf("address = %v -> %v, want %q -> %q",
				byField["address"].OldValue, byField["address"].NewValue, tierOldAddress, tierSecretAddress)
		}
		if byField["zipcode"].OldValue != tierOldZip || byField["zipcode"].NewValue != tierNewZip {
			t.Errorf("zipcode = %v -> %v, want %s -> %s",
				byField["zipcode"].OldValue, byField["zipcode"].NewValue, tierOldZip, tierNewZip)
		}
		if item.Summary != tierSummary {
			t.Errorf("summary = %q, want %q", item.Summary, tierSummary)
		}
	}

	historyPath := fmt.Sprintf("/revisions/venue/%d", venue.ID)

	t.Run("anonymous sees the masked history", func(t *testing.T) {
		body, item := readList(t, historyPath, "")
		assertMasked(t, body, item)
	})

	// The tier that only exists once real auth is in the chain. A logged-in
	// contributor is authenticated and still public — including on their OWN
	// edit, which this is.
	t.Run("authenticated non-admin sees the masked history", func(t *testing.T) {
		body, item := readList(t, historyPath, contributorToken)
		assertMasked(t, body, item)
	})

	t.Run("admin sees the stored history", func(t *testing.T) {
		_, item := readList(t, historyPath, adminToken)
		assertUnmasked(t, item)
	})

	// The carrier the PRODUCT uses. Browsers never send an Authorization header
	// to these routes — apiRequest sets credentials:'include' and the session
	// rides the HTTP-only auth_token cookie — and the middleware parses the
	// cookie in its own code, separate from the header branch. Without this,
	// every subtest above could pass while real admins sat on the masked tier.
	t.Run("an admin authenticated by cookie sees the stored history", func(t *testing.T) {
		body := send(t, historyPath, adminToken, viaCookie)
		var parsed adminh.GetEntityHistoryResponse
		if err := json.Unmarshal(body, &parsed.Body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, body)
		}
		if len(parsed.Body.Revisions) != 1 {
			t.Fatalf("got %d revisions, want 1; body: %s", len(parsed.Body.Revisions), body)
		}
		assertUnmasked(t, parsed.Body.Revisions[0])
	})

	// The negative half of the same carrier: a cookie must not be a free pass.
	t.Run("a garbage cookie gets the public tier", func(t *testing.T) {
		body := send(t, historyPath, "not.a.real.token", viaCookie)
		var parsed adminh.GetEntityHistoryResponse
		if err := json.Unmarshal(body, &parsed.Body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, body)
		}
		if len(parsed.Body.Revisions) != 1 {
			t.Fatalf("got %d revisions, want 1; body: %s", len(parsed.Body.Revisions), body)
		}
		assertMasked(t, body, parsed.Body.Revisions[0])
	})

	// A forged or unparseable credential must not buy the admin tier. The
	// optional middleware swallows the failure and proceeds anonymously, which is
	// the behaviour that makes "no credential" and "bad credential" identical
	// here — both land on the public tier rather than on an error.
	t.Run("a garbage bearer token gets the public tier, not a 401", func(t *testing.T) {
		body, item := readList(t, historyPath, "not.a.real.token")
		assertMasked(t, body, item)
	})

	// Same admin credential, minus one character of signature. Proves the tier
	// rides on signature verification rather than on the mere shape of a JWT.
	t.Run("an admin token with a broken signature gets the public tier", func(t *testing.T) {
		body, item := readList(t, historyPath, adminToken[:len(adminToken)-1])
		assertMasked(t, body, item)
	})

	// Deactivating the account revokes the tier without reissuing anything:
	// ValidateToken re-reads the user row on every request, so IsAdmin and
	// IsActive cannot go stale inside a still-valid token.
	t.Run("a deactivated admin's live token drops to the public tier", func(t *testing.T) {
		if err := td.DB.Model(&authm.User{}).Where("id = ?", admin.ID).
			Update("is_active", false).Error; err != nil {
			t.Fatalf("deactivate admin: %v", err)
		}
		defer func() {
			if err := td.DB.Model(&authm.User{}).Where("id = ?", admin.ID).
				Update("is_active", true).Error; err != nil {
				t.Fatalf("reactivate admin: %v", err)
			}
		}()

		body, item := readList(t, historyPath, adminToken)
		assertMasked(t, body, item)
	})

	// The other two read routes carry the same policy. They are covered here
	// rather than in their own test because the fixture is the expensive part and
	// the risk being pinned is per-ROUTE wiring: a handler that forgot to forward
	// the tier, or a route left on the bare API group.
	t.Run("the single-revision route carries the tier", func(t *testing.T) {
		path := fmt.Sprintf("/revisions/%d", stored.ID)

		body, public := readOne(t, path, contributorToken)
		assertMasked(t, body, public)

		_, asAdmin := readOne(t, path, adminToken)
		assertUnmasked(t, asAdmin)
	})

	t.Run("the user-revisions route carries the tier", func(t *testing.T) {
		path := fmt.Sprintf("/users/%d/revisions", contributor.ID)

		body, public := readUserList(t, path, "")
		assertMasked(t, body, public)

		// The author reading their own contributions page is still the public
		// tier. Authorship is not a credential.
		body, own := readUserList(t, path, contributorToken)
		assertMasked(t, body, own)

		_, asAdmin := readUserList(t, path, adminToken)
		assertUnmasked(t, asAdmin)
	})

	// The stored row is the truth Rollback restores, and no read may rewrite it.
	// Asserted last, after every tier has read the row, so it also rules out a
	// redaction that mutated the shared underlying JSON in place.
	t.Run("no read rewrote the stored row", func(t *testing.T) {
		var after adminm.Revision
		if err := td.DB.First(&after, stored.ID).Error; err != nil {
			t.Fatalf("re-read revision: %v", err)
		}
		if !strings.Contains(string(*after.FieldChanges), tierSecretAddress) {
			t.Errorf("stored field_changes lost the real address: %s", *after.FieldChanges)
		}
		if after.Summary == nil || *after.Summary != tierSummary {
			t.Errorf("stored summary was rewritten by a read: %v", after.Summary)
		}
	})
}
