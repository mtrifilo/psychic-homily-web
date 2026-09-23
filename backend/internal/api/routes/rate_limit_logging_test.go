package routes

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// The per-IP limiters built in this package log the same ratelimit_rejected
// line as middleware's limiters, named and with their real window, and never
// the connection's address or raw path; their response keeps Retry-After: 60.
func TestIPRateLimiter_LogsNamedRejectionWithoutAddress(t *testing.T) {
	const address = "192.0.2.123"
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	limited := ipRateLimiter(middleware.LimiterTagCreate, 1, time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var rr *httptest.ResponseRecorder
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/entities/artist/secret-slug-xyz/tags", nil)
		req.RemoteAddr = address + ":5555"
		rr = httptest.NewRecorder()
		limited.ServeHTTP(rr, req.WithContext(logger.NewContext(req.Context(), log)))
	}

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
	for _, raw := range []string{address, "secret-slug-xyz"} {
		if strings.Contains(buf.String(), raw) {
			t.Fatalf("429 log line carries %q:\n%s", raw, buf.String())
		}
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("429 log line is not one JSON record: %v: %s", err, buf.String())
	}
	for field, want := range map[string]any{
		"event":          "ratelimit_rejected",
		"limiter":        string(middleware.LimiterTagCreate),
		"window_seconds": float64(3600),
		"path_family":    middleware.PathFamilyOther,
		"method":         http.MethodPost,
	} {
		if line[field] != want {
			t.Errorf("%s = %v, want %v", field, line[field], want)
		}
	}
	for _, field := range []string{"remote_addr", "path", "key_fingerprint", "fingerprint_epoch"} {
		_, present := line[field]
		if wantPresent := field == "key_fingerprint" || field == "fingerprint_epoch"; present != wantPresent {
			t.Errorf("429 line %q present = %v, want %v", field, present, wantPresent)
		}
	}
}

// TestRateLimitPathFamilyAgreesWithRouter walks every registered route under a
// collection that has an entity path family and checks the family against how
// chi resolves the path: a parameter in the second segment is an entity, so its
// paths belong to the entity family; a static second segment is a
// collection-level route, so its paths must not. A new static route that the
// family's segment list does not know fails here instead of being counted as an
// entity in rate-limit logs.
func TestRateLimitPathFamilyAgreesWithRouter(t *testing.T) {
	param := regexp.MustCompile(`\{[^}]*\}`)
	checked := 0

	for method, patterns := range chiRoutes(t, newTestRouter(t)) {
		for _, pattern := range patterns {
			segments := strings.Split(strings.Trim(pattern, "/"), "/")
			if len(segments) < 2 {
				continue
			}
			entityFamily := middleware.RateLimitPathFamily("/" + segments[0] + "/some-entity-slug")
			if entityFamily == middleware.PathFamilyOther {
				// A collection without an entity family has nothing to disagree
				// with.
				continue
			}

			path := param.ReplaceAllString(pattern, "some-entity-slug")
			got := middleware.RateLimitPathFamily(path)
			checked++

			if param.MatchString(segments[1]) {
				if got != entityFamily {
					t.Errorf("%s %s: path %s has family %q, want the entity family %q",
						method, pattern, path, got, entityFamily)
				}
				continue
			}
			if got == entityFamily {
				t.Errorf("%s %s: static segment %q is counted as the entity family %q; "+
					"add it to the collection's route segments",
					method, pattern, segments[1], entityFamily)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no entity-collection routes were checked; the router walk found nothing")
	}
}
