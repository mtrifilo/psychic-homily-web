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

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// The 429 line of the per-IP write limiters identifies the bucket by
// fingerprint, never by the connection's address.
func TestRateLimitHandler_LogsFingerprintNotAddress(t *testing.T) {
	const address = "192.0.2.123"
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = address + ":5555"
	req = req.WithContext(logger.NewContext(req.Context(), log))
	rr := httptest.NewRecorder()

	rateLimitHandler(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
	if strings.Contains(buf.String(), address) {
		t.Fatalf("429 log line carries the client address:\n%s", buf.String())
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("429 log line is not one JSON record: %v: %s", err, buf.String())
	}
	if _, ok := line["remote_addr"]; ok {
		t.Errorf("429 log line carries remote_addr: %v", line["remote_addr"])
	}
	if got, want := line["key_fingerprint"], middleware.ClientIPKeyFingerprint(req); got != want {
		t.Errorf("key_fingerprint = %v, want %v", got, want)
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
			if entityFamily == middleware.RateLimitPathFamily("/labels/some-entity-slug") {
				// Collections without an entity family fold into the same value
				// as /labels, so there is nothing to disagree with.
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
