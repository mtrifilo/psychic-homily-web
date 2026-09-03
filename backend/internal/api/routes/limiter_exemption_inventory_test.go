package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

// The exemptions and the in-scope path patterns of the global limiters are
// written by hand, and nothing in the router tells them a path was added or
// renamed. These guards walk the BUILT router instead, so a new feed route or a
// renamed entity-request route fails here rather than shipping unmetered.

// Every route the router serves under /feeds/ must be a declared personal-feed
// template, and every template must be served. A new /feeds/ route that is not a
// token-authenticated feed does not belong under that prefix; one that is needs
// its template added here so the exemption can validate its token.
func TestFeedRoutesAndExemptionTemplatesAgree(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	declared := map[string]bool{}
	for _, template := range personalFeedRouteTemplates {
		declared[template] = true
	}

	served := map[string]bool{}
	for _, patterns := range routes {
		for _, pattern := range patterns {
			if strings.HasPrefix(pattern, "/feeds/") {
				served[pattern] = true
			}
		}
	}

	for pattern := range served {
		if !declared[pattern] {
			t.Errorf("router serves %q but personalFeedRouteTemplates does not declare it: "+
				"a feed route absent from the templates never earns the exemption, and a "+
				"non-feed route under /feeds/ does not belong there", pattern)
		}
	}
	for _, template := range personalFeedRouteTemplates {
		if strings.HasPrefix(template, "/feeds/") && !served[template] {
			t.Errorf("personalFeedRouteTemplates declares %q but the router does not serve it: "+
				"the exemption would validate a token for a path nothing answers", template)
		}
	}
}

// The legacy alias's matcher has an EMPTY suffix, so every two-segment path
// under /calendar/ is a token candidate. Any other route the router serves there
// must therefore be a literal that carries no phcal_ segment, or it silently
// becomes exemption-eligible.
func TestOtherCalendarRoutesCannotEarnTheFeedExemption(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))
	alwaysValid := func(string) bool { return true }

	for method, patterns := range routes {
		for _, pattern := range patterns {
			if !strings.HasPrefix(pattern, "/calendar/") || pattern == legacyCalendarFeedRoute {
				continue
			}
			if strings.Contains(pattern, "{") {
				t.Errorf("%s %q takes a path parameter under /calendar/: a caller could spell a "+
					"phcal_ token into it and claim the personal-feed exemption", method, pattern)
				continue
			}
			if validatedFeedToken(alwaysValid, pattern) {
				t.Errorf("%s %q is treated as a personal feed", method, pattern)
			}
		}
	}
}

// The legacy /calendar/{token} alias lives under /calendar/ rather than /feeds/,
// so the prefix sweep above cannot see it. Its registration is checked by name.
//
// What that alias reads as by SHAPE is pinned by TestPersonalFeedTokenFromPath
// and TestPublicReadRateLimiter_NonPrefixedFeedPathDoesNotHitValidateCallback,
// which cover the /calendar/token CRUD path beside it.
func TestLegacyCalendarAliasIsRegistered(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, pattern := range routes[http.MethodGet] {
		if pattern == legacyCalendarFeedRoute {
			return
		}
	}
	t.Errorf("router does not serve %q, but the legacy iCal alias is in the exemption templates",
		legacyCalendarFeedRoute)
}

// EVERY user-facing entity-request write the router serves must be claimed by
// exactly one of the two budgets. Sweeping the router rather than listing paths
// is what makes a NEW write route fail here instead of shipping unmetered; the
// expectations below say which budget each known route belongs to.
//
// Admin routes are excluded: they sit behind the admin gate, and the shared
// budget exempts admins anyway.
func TestEntityRequestRoutesLandOnTheirLimiters(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	// pattern -> which budget claims it. A served pattern absent from this map
	// fails the sweep below.
	expected := map[string]struct{ shared, batch bool }{
		"/entity-requests":               {shared: true},
		EntityRequestBatchPath:           {batch: true},
		"/entity-requests/{id}/withdraw": {shared: true},
	}

	served := map[string]bool{}
	for _, pattern := range routes[http.MethodPost] {
		if strings.HasPrefix(pattern, "/entity-requests") {
			served[pattern] = true
		}
	}

	for pattern := range served {
		want, known := expected[pattern]
		if !known {
			t.Errorf("router serves POST %q and no budget claims it: a new entity-request write "+
				"must be added to one of the limiters and to this table", pattern)
			continue
		}
		// Concretise the pattern the way a request arrives.
		path := strings.ReplaceAll(pattern, "{id}", "42")
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if got := isEngagementMutationRequest(req); got != want.shared {
			t.Errorf("isEngagementMutationRequest(POST %s) = %v, want %v", path, got, want.shared)
		}
		if got := isEntityRequestBatchRequest(req); got != want.batch {
			t.Errorf("isEntityRequestBatchRequest(POST %s) = %v, want %v", path, got, want.batch)
		}
	}
	for pattern := range expected {
		if !served[pattern] {
			t.Errorf("the limiter table claims POST %q but the router does not serve it", pattern)
		}
	}
}

// A percent-encoded separator routes in chi but decodes to extra segments, so a
// limiter matching the DECODED path would miss a request the server serves. Both
// matchers read the escaped path; this pins that they do.
func TestLimiterMatchersReadTheRoutedPath(t *testing.T) {
	cases := []struct {
		target string
		shared bool
		batch  bool
	}{
		{"/entity-requests/a%2Fb/withdraw", true, false},
		{"/entity-requests/42/withdraw", true, false},
		{EntityRequestBatchPath, false, true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.target, nil)
		if got := isEngagementMutationRequest(req); got != tc.shared {
			t.Errorf("isEngagementMutationRequest(POST %s) = %v, want %v", tc.target, got, tc.shared)
		}
		if got := isEntityRequestBatchRequest(req); got != tc.batch {
			t.Errorf("isEntityRequestBatchRequest(POST %s) = %v, want %v", tc.target, got, tc.batch)
		}
	}
}

// The same rule on the read side: an encoded spelling of the phcal_ prefix must
// not earn the exemption, because the router hands the handler the encoded
// segment and the handler rejects it. Exempting it would buy an unmetered 401.
//
// Driven through the mounted limiter rather than through validatedFeedToken
// directly, so it pins the WIRING: reading r.URL.Path there would hand the
// decoded token to a validator that resolves it.
func TestFeedExemptionReadsTheRoutedPath(t *testing.T) {
	const live = "phcal_abc"
	mw := PublicReadRateLimiter(nil, nil, func(tok string) bool { return tok == live }, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	burst := func(path, remote string, n int) (ok, limited int) {
		for i := 0; i < n; i++ {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			switch rr.Code {
			case http.StatusOK:
				ok++
			case http.StatusTooManyRequests:
				limited++
			}
		}
		return ok, limited
	}

	// The plain spelling is exempt well past the anonymous cap.
	if ok, limited := burst("/feeds/"+live+"/saved-shows.ics", "7.7.9.20:100",
		middleware.APIRequestsPerMinute+5); limited != 0 {
		t.Errorf("live feed token: ok=%d limited=%d, want none limited", ok, limited)
	}
	// The percent-encoded spelling of the same token is not.
	if _, limited := burst("/feeds/phcal%5fabc/saved-shows.ics", "7.7.9.21:100",
		middleware.APIRequestsPerMinute+5); limited == 0 {
		t.Error("the percent-encoded spelling was exempted: the routed path carries no phcal_ token")
	}

	// And the predicate itself refuses the encoded token, so nothing downstream
	// has to re-derive the rule.
	alwaysValid := func(string) bool { return true }
	if validatedFeedToken(alwaysValid, "/feeds/phcal%5fabc/saved-shows.ics") {
		t.Error("validatedFeedToken accepted a percent-encoded prefix")
	}
	if !validatedFeedToken(alwaysValid, "/feeds/"+live+"/saved-shows.ics") {
		t.Error("the plain feed path lost its exemption")
	}
}
