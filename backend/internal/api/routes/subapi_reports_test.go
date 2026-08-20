package routes

import (
	"fmt"
	"net/http"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

// PSY-1598 step 4, the last one: the entity-report submit group moves off its own
// humachi.New onto the main API.
//
// This is the group the whole problem was named after. Its instance was titled
// "Psychic Homily Entity Reports", and because chi keeps the LAST registration of
// a duplicate method+path, its /openapi.json won — production served those 8
// report routes as if they were the entire API (PSY-1554). Suppressing its doc
// routes stopped the shadowing but left the operations undocumented; this moves
// them into the one document, which is the actual fix.
//
// The group's properties differ from the ones converted before it, and each
// difference is pinned below rather than assumed:
//
//   - EIGHT operations shared ONE chi group, so they shared ONE counter. This is
//     the only converted group where budget-sharing spans more than two paths, and
//     the failure mode is quiet: give each operation its own limiter and an abuser
//     rotates entity types for 8x the budget while every single-path test passes.
//   - NO bypass. Show-create has a phk_ hatch and tag-create has SkipRateLimitForAdmin;
//     this group has neither and must not acquire one, because the endpoint's
//     output is the admin moderation queue's inbox.
//   - The limiter must stay OUTER of the JWT gate (PSY-1397 ordering). As chi
//     middleware it ran before Huma saw the request; unauthenticated floods were
//     rejected without touching the auth path.

// entityReportPaths are the eight operations that moved. They share one limiter,
// so a test that needs a fresh budget must use a fresh source address.
var entityReportPaths = []string{
	"/artists/{entity_id}/report",
	"/venues/{entity_id}/report",
	"/festivals/{entity_id}/report",
	"/shows/{entity_id}/entity-report",
	"/comments/{entity_id}/report",
	"/collections/{entity_id}/report",
	"/releases/{entity_id}/report",
	"/labels/{entity_id}/report",
}

// entityReportRequestPaths are the same operations as concrete request paths.
var entityReportRequestPaths = []string{
	"/artists/1/report",
	"/venues/1/report",
	"/festivals/1/report",
	"/shows/1/entity-report",
	"/comments/1/report",
	"/collections/1/report",
	"/releases/1/report",
	"/labels/1/report",
}

// The point of the change: eight operations that were reachable but undocumented
// now appear in the one published document.
func TestEntityReportOperationsAreInMainSpec(t *testing.T) {
	paths := servedSpecPaths(t, newTestRouter(t))

	for _, p := range entityReportPaths {
		item, ok := paths[p]
		if !ok {
			t.Errorf("%s is missing from the served spec — the entity-report group is still undocumented", p)
			continue
		}
		if _, ok := item["post"]; !ok {
			t.Errorf("expected a documented POST operation for %s", p)
		}
	}
}

// Siblings that already lived on the main API share a path prefix with the moved
// POSTs; registering the POSTs must not displace them. /shows/{show_id}/report in
// particular is a DIFFERENT operation from /shows/{entity_id}/entity-report, and
// both must survive.
func TestEntityReportSiblingsSurviveTheMove(t *testing.T) {
	paths := servedSpecPaths(t, newTestRouter(t))

	for _, want := range []struct{ path, method string }{
		{"/artists/{entity_id}/my-report", "get"},
		{"/shows/{show_id}/report", "post"},
		{"/admin/entity-reports", "get"},
	} {
		item, ok := paths[want.path]
		if !ok {
			t.Errorf("%s disappeared from the spec after the move", want.path)
			continue
		}
		if _, ok := item[want.method]; !ok {
			t.Errorf("%s lost its %s operation after the move", want.path, want.method)
		}
	}
}

func TestEntityReportStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.61:1234"
	const path = "/artists/1/report"
	limit := middleware.ReportRequestsPerMinute

	burstUnderCap(t, router, "POST", path, ip, limit)
	if code := send(t, router, "POST", path, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the entity-report limit did NOT survive the move to a Huma group",
			limit+1, code)
	}
}

// The eight operations shared one chi group and therefore one counter. Rotating
// entity types must not buy more budget.
func TestEntityReportRateLimitIsSharedAcrossTheGroup(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.62:1234"
	limit := middleware.ReportRequestsPerMinute

	// Spend the budget across DIFFERENT paths, one request each.
	for i := 0; i < limit; i++ {
		p := entityReportRequestPaths[i%len(entityReportRequestPaths)]
		if code := send(t, router, "POST", p, ip, nil); code == http.StatusTooManyRequests {
			t.Fatalf("%s (request %d/%d) was limited early", p, i+1, limit)
		}
	}

	// A path not yet touched must already be out of budget.
	next := entityReportRequestPaths[limit%len(entityReportRequestPaths)]
	if code := send(t, router, "POST", next, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("%s returned %d after the group budget was spent, want 429 — the eight operations "+
			"no longer share one counter, so rotating entity types multiplies the budget", next, code)
	}
}

// The two report submit surfaces were separate chi/Huma groups with separate
// limiters and stay that way. Merging them would silently halve the effective
// budget for a user who legitimately reports a show and then an artist.
func TestEntityReportAndShowReportDoNotShareABudget(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.63:1234"
	limit := middleware.ReportRequestsPerMinute

	for i := 0; i <= limit; i++ {
		send(t, router, "POST", "/artists/1/report", ip, nil)
	}
	if code := send(t, router, "POST", "/artists/1/report", ip, nil); code != http.StatusTooManyRequests {
		t.Fatalf("entity-report budget should be exhausted, got %d", code)
	}
	if code := send(t, router, "POST", "/shows/1/report", ip, nil); code == http.StatusTooManyRequests {
		t.Error("POST /shows/{show_id}/report was limited by the entity-report budget — the two groups " +
			"share a counter they never shared before")
	}
}

func TestEntityReportRateLimitIsPerIP(t *testing.T) {
	router := newTestRouter(t)
	const path = "/venues/1/report"

	for i := 0; i <= middleware.ReportRequestsPerMinute; i++ {
		send(t, router, "POST", path, "198.51.100.61:5000", nil)
	}
	if code := send(t, router, "POST", path, "198.51.100.61:5000", nil); code != http.StatusTooManyRequests {
		t.Fatalf("first client should be limited, got %d", code)
	}
	if code := send(t, router, "POST", path, "198.51.100.62:5000", nil); code == http.StatusTooManyRequests {
		t.Error("a different IP was limited by the first client's budget; KeyByClientIP did not survive the move")
	}
}

// This group has never had an API-token or admin hatch, and must not gain one:
// reports feed the moderation queue, so an unthrottled path here is a way to bury
// real reports under noise. Pinned explicitly because the two groups converted
// before this one DO have bypasses, which makes adding one here look like
// consistency rather than a policy change.
func TestEntityReportHasNoAPITokenBypass(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.64:1234"
	const path = "/artists/1/report"
	hdrs := map[string]string{"Authorization": "Bearer phk_deadbeef"}

	for i := 0; i < middleware.ReportRequestsPerMinute; i++ {
		send(t, router, "POST", path, ip, hdrs)
	}
	if code := send(t, router, "POST", path, ip, hdrs); code != http.StatusTooManyRequests {
		t.Errorf("phk_ request past the cap returned %d, want 429 — entity reports must NOT have a token bypass", code)
	}
}

// PSY-1397 ordering, pinned by observable behaviour rather than by reading the
// registration order.
//
// The limiter has to run OUTSIDE the JWT gate. Every request below is
// unauthenticated: if the limiter is outer it counts them and the (limit+1)th is
// 429, which is what unauthenticated flood protection means. If the ordering ever
// inverted, the JWT gate would reject all of them with 401 before the limiter saw
// anything, no request would ever be counted, and no 429 would appear — an
// endpoint that can be hammered for free.
func TestEntityReportLimiterRunsBeforeTheAuthGate(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.65:1234"
	const path = "/labels/1/report"
	limit := middleware.ReportRequestsPerMinute

	for i := 0; i < limit; i++ {
		code := send(t, router, "POST", path, ip, nil)
		if code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was limited early", i+1, limit)
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated POST %s = %d, want 401 — the JWT gate must still run on this group", path, code)
		}
	}

	if code := send(t, router, "POST", path, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — unauthenticated requests are not being counted, "+
			"which means the JWT middleware now runs OUTSIDE the limiter (PSY-1397 ordering inverted)", limit+1, code)
	}
}

// The JWT gate itself must survive: the sub-API carried HumaJWTMiddleware, and
// dropping it while moving would turn eight authenticated endpoints anonymous.
// Checked on every path, because the middleware is attached to the group once and
// a path registered on the wrong group is the way it would leak.
func TestEntityReportGroupStillRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t)

	for i, path := range entityReportRequestPaths {
		// Fresh source per path: the eight operations share ONE 5/min budget, so
		// reusing an address would start returning 429 on the sixth path and hide
		// whatever the auth gate would have said.
		ip := fmt.Sprintf("198.51.100.%d:5000", 100+i)
		code, body := sendWithBody(t, router, "POST", path, ip, nil)
		if code != http.StatusUnauthorized {
			t.Errorf("unauthenticated POST %s = %d, want 401 — the JWT middleware did not survive the move; body: %s",
				path, code, body)
		}
		assertReachedHandler(t, code, body, path)
	}
}
