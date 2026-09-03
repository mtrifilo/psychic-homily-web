package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// The entity-request limiters match on concrete request paths, so the patterns
// and the router have to agree about what those paths are. Concretising each
// registered pattern and asserting which limiter claims it is what catches a
// rename that leaves a write route unmetered.
func TestEntityRequestRoutesLandOnTheirLimiters(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	registered := map[string]bool{}
	for _, pattern := range routes[http.MethodPost] {
		registered[pattern] = true
	}

	cases := []struct {
		pattern string
		path    string
		shared  bool // metered by the shared engagement budget
		batch   bool // metered by the batch budget
	}{
		{"/entity-requests", "/entity-requests", true, false},
		{EntityRequestBatchPath, EntityRequestBatchPath, false, true},
		{"/entity-requests/{id}/withdraw", "/entity-requests/42/withdraw", true, false},
	}
	for _, tc := range cases {
		if !registered[tc.pattern] {
			t.Errorf("POST %s is not registered: the limiter patterns name a path the router does not serve", tc.pattern)
			continue
		}
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		if got := isEngagementMutationRequest(req); got != tc.shared {
			t.Errorf("isEngagementMutationRequest(POST %s) = %v, want %v", tc.path, got, tc.shared)
		}
		if got := isEntityRequestBatchRequest(req); got != tc.batch {
			t.Errorf("isEntityRequestBatchRequest(POST %s) = %v, want %v", tc.path, got, tc.batch)
		}
	}
}
