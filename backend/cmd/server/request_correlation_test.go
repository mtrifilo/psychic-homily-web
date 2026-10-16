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
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// A request's id is ONE id across everything that reports it: the X-Request-ID
// response header, the request_id in an error response body, the access log
// line, the Huma context logger the handler and
// the JWT middleware write through, the Sentry scope's request_id tag, and a
// group rate limiter's refusal line. The router is the production correlation
// stack (useRequestCorrelation) in front of routes.SetupRoutes, the order
// main() builds them in.
//
// Every one of those log lines goes through logger.Default(), whose handler is
// bound to os.Stdout the first time it is used and cannot be redirected
// afterwards. So the requests are served in a child process running this same
// test with the logger production initializes (JSON, debug off), and this
// process reads the child's stdout: the lines it sees are the lines production
// writes.
func TestRequestCorrelationCarriesOneID(t *testing.T) {
	if os.Getenv(correlationChildEnv) == "1" {
		serveCorrelationCases(t)
		return
	}

	results := parseChildCases[caseEnd](t, runTestChild(t, "TestRequestCorrelationCarriesOneID", correlationChildEnv))

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

		// The child stops at the first refusal, so the last attempt it reports
		// under this name is that refusal.
		name := correlationCaseName("refusal", inbound)
		if res := results[name]; res.end.Status != http.StatusTooManyRequests {
			t.Errorf("%s: the auth group limiter refused none of the attempts; last status %d", name, res.end.Status)
			continue
		}
		assertOneID(t, name, inbound, results[name], logRefusal)
	}
}

const (
	correlationChildEnv  = "PSY_TEST_REQUEST_CORRELATION_CHILD"
	correlationInboundID = "inbound-correlation-id"
	caseBeginMarker      = "correlation-case-begin "
	caseEndMarker        = "correlation-case-end "
)

// correlationRoutes sample the groups that sit behind a group rate limiter (an
// auth route, show creation, a tag route) plus the entity-report group as the
// regression row. Each request reaches the code past the limiter: the login
// handler, or the JWT gate that refuses a caller with no session.
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
	Status        int    `json:"status"`
	Header        string `json:"header"`
	BodyRequestID string `json:"body_request_id"`
	SentryTag     string `json:"sentry_tag"`
}

// serveCorrelationCases runs in the child. It serves every case through the
// production-shaped router and brackets each one with markers on stdout, the
// stream logger.Default() writes to.
func serveCorrelationCases(t *testing.T) {
	logger.Init(true, false)

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

		end := caseEnd{Status: rr.Code, Header: rr.Header().Get(middleware.RequestIDHeader)}
		var errorBody struct {
			RequestID string `json:"request_id"`
		}
		if json.Unmarshal(rr.Body.Bytes(), &errorBody) == nil {
			end.BodyRequestID = errorBody.RequestID
		}
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

// runTestChild re-runs this test binary with only testName selected and the
// childEnv switch set, and returns its combined output. The auth rate-limit
// escape hatch is cleared so the group limiter is real in the child.
func runTestChild(t *testing.T, testName, childEnv string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	env := []string{childEnv + "=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, routes.DisableAuthRateLimitsEnvVar+"=") && !strings.HasPrefix(kv, childEnv+"=") {
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

// childCase is one case a child served: what it reported at the case end
// marker and every line written between the case's markers.
type childCase[T any] struct {
	end   T
	lines []string
}

// parseChildCases groups a child's output lines by the case whose markers
// bracket them. A name served more than once keeps its last attempt.
func parseChildCases[T any](t *testing.T, out []byte) map[string]childCase[T] {
	t.Helper()
	results := map[string]childCase[T]{}
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
			if current == "" {
				t.Fatalf("case end with no open case: %q", line)
			}
			var end T
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, caseEndMarker)), &end); err != nil {
				t.Fatalf("decode case end %q: %v", line, err)
			}
			results[current] = childCase[T]{end: end, lines: lines}
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

// logKind sorts a case's log lines by who wrote them.
type logKind string

const (
	logAccess  logKind = "access log line"
	logRefusal logKind = "rate limiter refusal line"
	// logReachedHandler is a line written past the limiter through the Huma
	// context logger.
	logReachedHandler logKind = "Huma context logger line"
)

// processScopedLogMsgs are lines that describe the process rather than a
// request, so they carry no request_id wherever they land.
var processScopedLogMsgs = map[string]bool{
	"ratelimit_proxy_trust": true,
}

// logRecord reads one JSON log line: its message and every request_id field on
// it, in order. The logger.Auth* helpers write request_id twice (the context
// logger's and their own), and both must be the request's id. A line that is
// not a JSON object is not a log record.
func logRecord(line string) (msg string, requestIDs []string, ok bool) {
	dec := json.NewDecoder(strings.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return "", nil, false
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return "", nil, false
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return "", nil, false
		}
		var text string
		switch keyTok {
		case "msg":
			_ = json.Unmarshal(value, &text)
			msg = text
		case "request_id":
			_ = json.Unmarshal(value, &text)
			requestIDs = append(requestIDs, text)
		}
	}
	return msg, requestIDs, true
}

func classifyLogLine(msg string) logKind {
	switch msg {
	case "request":
		return logAccess
	case "rate limit exceeded":
		return logRefusal
	}
	return logReachedHandler
}

// assertOneID checks that the response header, the Sentry tag and every
// request_id on every log line of the case are the same id; that every line of
// the case except a process-scoped one carries a request_id; that the id is the
// inbound one when the client sent one and a minted UUID otherwise; and that
// the access line and the wanted kind of line are both present, so a stream the
// parent cannot see fails rather than passing with nothing to compare.
//
// A Huma context logger line counts as present only when it carries request_id
// twice: the logger.Auth* helpers add the context id to a logger that already
// carries it, so a line with one copy means the context logger lost the id.
func assertOneID(t *testing.T, name, inbound string, res childCase[caseEnd], want logKind) {
	t.Helper()
	id := res.end.Header
	if inbound != "" {
		if id != inbound {
			t.Errorf("%s: X-Request-ID = %q, want the inbound %q", name, id, inbound)
		}
	} else if _, err := uuid.Parse(id); err != nil {
		t.Errorf("%s: X-Request-ID = %q, want a minted UUID", name, id)
	}
	if res.end.SentryTag != id {
		t.Errorf("%s: Sentry request_id tag = %q, X-Request-ID = %q", name, res.end.SentryTag, id)
	}
	// Every response past the limiter is an error body that names the request
	// (the login handler's or the JWT gate's); the limiter's 429 body names none.
	if want == logReachedHandler || res.end.BodyRequestID != "" {
		if res.end.BodyRequestID != id {
			t.Errorf("%s: response body request_id = %q, X-Request-ID = %q", name, res.end.BodyRequestID, id)
		}
	}

	seen := map[logKind]bool{}
	for _, line := range res.lines {
		msg, requestIDs, ok := logRecord(line)
		if !ok {
			continue
		}
		if processScopedLogMsgs[msg] {
			continue
		}
		kind := classifyLogLine(msg)
		if kind != logReachedHandler || len(requestIDs) >= 2 {
			seen[kind] = true
		}
		if len(requestIDs) == 0 {
			t.Errorf("%s: %s carries no request_id: %s", name, kind, line)
		}
		for _, got := range requestIDs {
			if got != id {
				t.Errorf("%s: %s request_id = %q, X-Request-ID = %q: %s", name, kind, got, id, line)
			}
		}
	}
	for _, kind := range []logKind{logAccess, want} {
		if !seen[kind] {
			t.Errorf("%s: no %s carrying the request id was written; lines seen:\n%s", name, kind, strings.Join(res.lines, "\n"))
		}
	}
}
