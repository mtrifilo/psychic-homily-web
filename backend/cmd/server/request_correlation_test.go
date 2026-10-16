package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// A request's id is ONE id across everything that reports it: the X-Request-ID
// response header, the access log line, the Huma context logger the handler and
// the JWT middleware write through, the Sentry scope's request_id tag, and a
// group rate limiter's refusal line. The router is the production correlation
// stack (useRequestCorrelation) in front of routes.SetupRoutes, the order
// main() builds them in.
//
// Every one of those log lines goes through logger.Default(), whose handler is
// bound to os.Stdout the first time it is used and cannot be redirected
// afterwards. So the requests are served in a child process running this same
// test, and this process reads the child's stdout: the lines it sees are
// exactly the lines production writes.
func TestRequestCorrelationCarriesOneID(t *testing.T) {
	if os.Getenv(correlationChildEnv) == "1" {
		serveCorrelationCases(t)
		return
	}

	results := parseCorrelationOutput(t, runCorrelationChild(t))

	for _, inbound := range []string{"", correlationInboundID} {
		for _, c := range correlationRoutes {
			name := correlationCaseName(c.name, inbound)
			res, ok := results[name]
			if !ok {
				t.Errorf("%s: the child served no such case", name)
				continue
			}
			assertOneID(t, name, inbound, res, logReachedHandler)
		}

		refusal := firstRefusal(t, results, correlationCaseName("refusal", inbound))
		if refusal == nil {
			continue
		}
		assertOneID(t, correlationCaseName("refusal", inbound), inbound, *refusal, logRefusal)
	}
}

const (
	correlationChildEnv  = "PSY_TEST_REQUEST_CORRELATION_CHILD"
	correlationInboundID = "inbound-correlation-id"
	caseBeginMarker      = "correlation-case-begin "
	caseEndMarker        = "correlation-case-end "
)

// correlationRoutes are one route per family that sits behind a group rate
// limiter, plus the entity-report group as the regression row. Each request
// reaches the code past the limiter: the login handler, or the JWT gate that
// refuses a caller with no session.
var correlationRoutes = []struct {
	name, path, body, ip string
}{
	{"login", "/auth/login", `{"email":"probe@example.com","password":"not-the-password"}`, "192.0.2.10"},
	{"show-create", "/shows", `{}`, "192.0.2.11"},
	{"tag-vote", "/tags/1/entities/artist/1/votes", `{}`, "192.0.2.12"},
	{"entity-report", "/artists/1/report", `{}`, "192.0.2.13"},
}

func correlationCaseName(route, inbound string) string {
	if inbound == "" {
		return route + "/no-inbound-id"
	}
	return route + "/inbound-id"
}

// caseEnd is what the child reports about one request once it is served.
type caseEnd struct {
	Name      string `json:"name"`
	Status    int    `json:"status"`
	Header    string `json:"header"`
	SentryTag string `json:"sentry_tag"`
}

// caseResult is one served request: what the child reported plus every log
// line written between its begin and end markers.
type caseResult struct {
	caseEnd
	lines []string
}

// serveCorrelationCases runs in the child. It serves every case through the
// production-shaped router and brackets each one with markers on stdout, the
// stream logger.Default() writes to.
func serveCorrelationCases(t *testing.T) {
	cfg := &config.Config{
		JWT:   config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24},
		OAuth: config.OAuthConfig{SecretKey: "test-oauth-secret-key-32-chars"},
	}
	sc := services.NewServiceContainer(nil, cfg)

	var hub *sentry.Hub
	router := chi.NewMux()
	useRequestCorrelation(router)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hub = sentry.GetHubFromContext(r.Context())
			next.ServeHTTP(w, r)
		})
	})
	routes.SetupRoutes(router, sc, cfg)

	serve := func(name, path, body, ip, inbound string) int {
		fmt.Println(caseBeginMarker + name)
		hub = nil
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		if inbound != "" {
			req.Header.Set(middleware.RequestIDHeader, inbound)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		end := caseEnd{Name: name, Status: rr.Code, Header: rr.Header().Get(middleware.RequestIDHeader)}
		if hub != nil {
			end.SentryTag = hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil).Tags["request_id"]
		}
		encoded, err := json.Marshal(end)
		if err != nil {
			t.Fatalf("encode case end: %v", err)
		}
		fmt.Println(caseEndMarker + string(encoded))
		return rr.Code
	}

	for i, inbound := range []string{"", correlationInboundID} {
		for _, c := range correlationRoutes {
			serve(correlationCaseName(c.name, inbound), c.path, c.body, c.ip, inbound)
		}
		// A source address of its own, so only this loop spends its budget.
		refusalIP := fmt.Sprintf("192.0.2.%d", 20+i)
		for attempt := 0; attempt < middleware.AuthRequestsPerMinute+5; attempt++ {
			if serve(correlationCaseName("refusal", inbound), "/auth/login", `{}`, refusalIP, inbound) == http.StatusTooManyRequests {
				break
			}
		}
	}
}

// runCorrelationChild re-runs this test binary with only this test selected and
// the child switch set, and returns its combined output. The auth rate-limit
// escape hatch is cleared so the group limiter is real in the child.
func runCorrelationChild(t *testing.T) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRequestCorrelationCarriesOneID$", "-test.count=1")
	env := []string{correlationChildEnv + "=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, routes.DisableAuthRateLimitsEnvVar+"=") && !strings.HasPrefix(kv, correlationChildEnv+"=") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child run failed: %v\n%s", err, out)
	}
	return out
}

// parseCorrelationOutput groups the child's log lines by the case whose markers
// bracket them. Every name appears once except the refusal cases, whose attempts
// share one name; those are kept under name#attempt.
func parseCorrelationOutput(t *testing.T, out []byte) map[string]caseResult {
	t.Helper()
	results := map[string]caseResult{}
	attempts := map[string]int{}
	var current string
	var lines []string

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, caseBeginMarker):
			current = strings.TrimPrefix(line, caseBeginMarker)
			lines = nil
		case strings.HasPrefix(line, caseEndMarker):
			var end caseEnd
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, caseEndMarker)), &end); err != nil {
				t.Fatalf("decode case end %q: %v", line, err)
			}
			if end.Name != current {
				t.Fatalf("case end %q closes case %q", end.Name, current)
			}
			key := end.Name
			if strings.HasPrefix(key, "refusal/") {
				key = fmt.Sprintf("%s#%d", key, attempts[end.Name])
				attempts[end.Name]++
			}
			results[key] = caseResult{caseEnd: end, lines: lines}
			current = ""
		case current != "":
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read child output: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("the child reported no cases:\n%s", out)
	}
	return results
}

// firstRefusal returns the first attempt of the named refusal case that the
// group limiter refused.
func firstRefusal(t *testing.T, results map[string]caseResult, name string) *caseResult {
	t.Helper()
	for i := 0; ; i++ {
		res, ok := results[fmt.Sprintf("%s#%d", name, i)]
		if !ok {
			t.Errorf("%s: the auth group limiter refused none of %d attempts", name, i)
			return nil
		}
		if res.Status == http.StatusTooManyRequests {
			return &res
		}
	}
}

// logKind sorts a case's log lines by who wrote them.
type logKind string

const (
	logAccess         logKind = "access log line"
	logRefusal        logKind = "rate limiter refusal line"
	logReachedHandler logKind = "Huma context logger line"
)

var (
	logMsgPattern       = regexp.MustCompile(`(?:^| )msg=("(?:[^"\\]|\\.)*"|\S+)`)
	logRequestIDPattern = regexp.MustCompile(`(?:^| )request_id=("(?:[^"\\]|\\.)*"|\S*)`)
)

func classifyLogLine(line string) (logKind, bool) {
	m := logMsgPattern.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	switch strings.Trim(m[1], `"`) {
	case "request":
		return logAccess, true
	case "rate limit exceeded":
		return logRefusal, true
	}
	if !logRequestIDPattern.MatchString(line) {
		return "", false
	}
	return logReachedHandler, true
}

// assertOneID checks that the response header, the Sentry tag and every
// request_id on every log line of the case are the same id, that the id is the
// inbound one when the client sent one and a minted UUID otherwise, and that
// the access line and the wanted kind of line are both present, so a stream the
// parent cannot see fails rather than passing with nothing to compare.
func assertOneID(t *testing.T, name, inbound string, res caseResult, want logKind) {
	t.Helper()
	id := res.Header
	if inbound != "" {
		if id != inbound {
			t.Errorf("%s: X-Request-ID = %q, want the inbound %q", name, id, inbound)
		}
	} else if _, err := uuid.Parse(id); err != nil {
		t.Errorf("%s: X-Request-ID = %q, want a minted UUID", name, id)
	}
	if res.SentryTag != id {
		t.Errorf("%s: Sentry request_id tag = %q, X-Request-ID = %q", name, res.SentryTag, id)
	}

	seen := map[logKind]bool{}
	for _, line := range res.lines {
		kind, ok := classifyLogLine(line)
		if !ok {
			continue
		}
		seen[kind] = true
		matches := logRequestIDPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			t.Errorf("%s: %s carries no request_id: %s", name, kind, line)
		}
		for _, m := range matches {
			if got := strings.Trim(m[1], `"`); got != id {
				t.Errorf("%s: %s request_id = %q, X-Request-ID = %q: %s", name, kind, got, id, line)
			}
		}
	}
	for _, kind := range []logKind{logAccess, want} {
		if !seen[kind] {
			t.Errorf("%s: no %s was written; lines seen:\n%s", name, kind, strings.Join(res.lines, "\n"))
		}
	}
}
