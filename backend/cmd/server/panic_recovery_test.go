package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// A handler panic answers 500 with the auth error envelope naming the request,
// is captured by Sentry once with the request's id as its tag, and is logged
// once at ERROR with that id and the scrubbed panic value, for a route
// registered directly on chi and for a Huma operation alike. A panic after the
// response started aborts the connection instead, and a handler's own
// http.ErrAbortHandler passes through unlogged.
//
// The router is the production correlation stack in front of
// routes.SetupRoutes, with the response compressor (the global middleware that
// wraps the writer and closes it while a panic unwinds) between them as in
// main(). Requests go through a real net/http server so an aborted response is
// what a client sees, and the log lines are read from a child process for the
// reason TestRequestCorrelationCarriesOneID gives.
func TestHandlerPanicAnswers500(t *testing.T) {
	if os.Getenv(panicChildEnv) == "1" {
		servePanicCases(t)
		return
	}

	cases := parseChildCases(t, runTestChild(t, "TestHandlerPanicAnswers500", panicChildEnv))
	result := func(name string) (panicCaseEnd, []map[string]any) {
		c, ok := cases[name]
		if !ok {
			t.Fatalf("%s: the child served no such case", name)
		}
		var end panicCaseEnd
		if err := json.Unmarshal(c.end, &end); err != nil {
			t.Fatalf("%s: decode case end: %v", name, err)
		}
		for _, line := range c.lines {
			if strings.Contains(line, panicProbeHeaderValue) {
				t.Errorf("%s: a log line carries a request header value: %s", name, line)
			}
		}
		return end, logObjects(c.lines)
	}

	for _, name := range []string{"raw-route", "huma-operation"} {
		end, records := result(name)
		id := end.RequestIDHeader
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("%s: X-Request-ID = %q, want a minted UUID", name, id)
		}
		if end.Status != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500 (body %q)", name, end.Status, end.Body)
		}
		if end.ContentType != "application/json" {
			t.Errorf("%s: Content-Type = %q, want application/json", name, end.ContentType)
		}
		var body middleware.JWTErrorResponse
		if err := json.Unmarshal([]byte(end.Body), &body); err != nil {
			t.Errorf("%s: body %q is not the error envelope: %v", name, end.Body, err)
		}
		want := middleware.JWTErrorResponse{
			Success:   false,
			Message:   autherrors.ToExternalMessage(autherrors.CodeUnknown),
			ErrorCode: autherrors.CodeUnknown,
			RequestID: id,
		}
		if body != want {
			t.Errorf("%s: body = %+v, want %+v", name, body, want)
		}

		if len(end.Events) != 1 {
			t.Errorf("%s: Sentry captured %d events, want 1", name, len(end.Events))
		}
		for _, ev := range end.Events {
			if ev.RequestIDTag != id {
				t.Errorf("%s: Sentry request_id tag = %q, X-Request-ID = %q", name, ev.RequestIDTag, id)
			}
			if !strings.Contains(ev.Exception, panicProbeMessage) {
				t.Errorf("%s: Sentry exception = %q, want the panic value", name, ev.Exception)
			}
		}

		if n := countRecords(records, "request", id); n != 1 {
			t.Errorf("%s: %d access lines carry the request id, want 1", name, n)
		}
		panics := recordsWithMsg(records, panicLogMsg)
		if len(panics) != 1 {
			t.Errorf("%s: %d %s lines, want 1; lines: %v", name, len(panics), panicLogMsg, records)
			continue
		}
		line := panics[0]
		if line["level"] != "ERROR" || line["request_id"] != id || line["status"] != float64(http.StatusInternalServerError) || line["response_started"] != false {
			t.Errorf("%s: panic line = %v, want level ERROR, request_id %s, status 500, response_started false", name, line, id)
		}
		panicText, _ := line["panic"].(string)
		if !strings.Contains(panicText, panicProbeMessage) || strings.Contains(panicText, panicProbeSecret) {
			t.Errorf("%s: panic line value = %q, want the panic message with its secret scrubbed", name, panicText)
		}
	}

	t.Run("response already started", func(t *testing.T) {
		end, records := result("started")
		if end.Status != http.StatusOK || end.ClientError == "" {
			t.Errorf("status %d, client error %q: want the started 200 cut off with a read error", end.Status, end.ClientError)
		}
		if len(end.Events) != 1 {
			t.Errorf("Sentry captured %d events, want 1", len(end.Events))
		}
		panics := recordsWithMsg(records, panicLogMsg)
		if len(panics) != 1 {
			t.Errorf("%d %s lines, want 1; lines: %v", len(panics), panicLogMsg, records)
			return
		}
		if line := panics[0]; line["response_started"] != true || line["status"] != nil || line["request_id"] != end.RequestIDHeader {
			t.Errorf("panic line = %v, want response_started true, no status, request_id %s", line, end.RequestIDHeader)
		}
	})

	t.Run("handler aborts", func(t *testing.T) {
		end, records := result("abort")
		if end.ClientError == "" {
			t.Errorf("status %d with body %q: want the connection aborted", end.Status, end.Body)
		}
		if n := len(recordsWithMsg(records, panicLogMsg)); n != 0 {
			t.Errorf("%d %s lines for http.ErrAbortHandler, want 0", n, panicLogMsg)
		}
	})
}

const (
	panicChildEnv         = "PSY_TEST_PANIC_RECOVERY_CHILD"
	panicLogMsg           = "handler_panic"
	panicProbeMessage     = "probe panic"
	panicProbeSecret      = "probe-secret-value"
	panicProbeHeaderValue = "probe-header-value"
)

// panicCaseEnd is what the child reports about one request once it is served.
type panicCaseEnd struct {
	Status          int          `json:"status"`
	ContentType     string       `json:"content_type"`
	RequestIDHeader string       `json:"request_id_header"`
	Body            string       `json:"body"`
	ClientError     string       `json:"client_error"`
	Events          []panicEvent `json:"events"`
}

type panicEvent struct {
	RequestIDTag string `json:"request_id_tag"`
	Exception    string `json:"exception"`
}

// servePanicCases runs in the child. Each request gets a hub of its own on the
// test Sentry client, as sentryhttp gives each request a clone of the
// process hub, and the client records every event it would send.
func servePanicCases(t *testing.T) {
	logger.Init(true, false)

	var mu sync.Mutex
	var events []panicEvent
	client, err := sentry.NewClient(sentry.ClientOptions{
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			ev := panicEvent{RequestIDTag: event.Tags["request_id"]}
			if len(event.Exception) > 0 {
				ev.Exception = event.Exception[0].Value
			}
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("sentry test client: %v", err)
	}

	cfg := &config.Config{
		JWT:   config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24},
		OAuth: config.OAuthConfig{SecretKey: "test-oauth-secret-key-32-chars"},
	}
	router := chi.NewMux()
	useRequestCorrelation(router)
	router.Use(middleware.ResponseCompression())
	api := routes.SetupRoutes(router, services.NewServiceContainer(nil, cfg), cfg)

	probe := errors.New(panicProbeMessage + ": token=" + panicProbeSecret)
	router.Get("/test-panic/raw", func(http.ResponseWriter, *http.Request) {
		panic(probe)
	})
	huma.Register(api, huma.Operation{
		OperationID: "test-panic",
		Method:      http.MethodGet,
		Path:        "/test-panic/huma",
	}, func(context.Context, *struct{}) (*struct{}, error) {
		panic(probe)
	})
	router.Get("/test-panic/started", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		_ = http.NewResponseController(w).Flush()
		panic(probe)
	})
	router.Get("/test-panic/abort", func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub := sentry.NewHub(client, sentry.NewScope())
		router.ServeHTTP(w, r.WithContext(sentry.SetHubOnContext(r.Context(), hub)))
	}))
	defer server.Close()

	serve := func(name, path string) {
		fmt.Println(caseBeginMarker + name)
		mu.Lock()
		events = nil
		mu.Unlock()

		var end panicCaseEnd
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatalf("%s: build request: %v", name, err)
		}
		req.Header.Set("Authorization", "Bearer "+panicProbeHeaderValue)
		resp, err := server.Client().Do(req)
		if err != nil {
			end.ClientError = err.Error()
		} else {
			end.Status = resp.StatusCode
			end.ContentType = resp.Header.Get("Content-Type")
			end.RequestIDHeader = resp.Header.Get(middleware.RequestIDHeader)
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			end.Body = string(body)
			if readErr != nil {
				end.ClientError = readErr.Error()
			}
		}

		mu.Lock()
		end.Events = append([]panicEvent(nil), events...)
		mu.Unlock()
		encoded, err := json.Marshal(end)
		if err != nil {
			t.Fatalf("%s: encode case end: %v", name, err)
		}
		fmt.Println(caseEndMarker + string(encoded))
	}

	serve("raw-route", "/test-panic/raw")
	serve("huma-operation", "/test-panic/huma")
	serve("started", "/test-panic/started")
	serve("abort", "/test-panic/abort")
}

// logObjects decodes the lines that are JSON log records.
func logObjects(lines []string) []map[string]any {
	var records []map[string]any
	for _, line := range lines {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil {
			records = append(records, record)
		}
	}
	return records
}

func recordsWithMsg(records []map[string]any, msg string) []map[string]any {
	var matched []map[string]any
	for _, record := range records {
		if record["msg"] == msg {
			matched = append(matched, record)
		}
	}
	return matched
}

func countRecords(records []map[string]any, msg, requestID string) int {
	n := 0
	for _, record := range recordsWithMsg(records, msg) {
		if record["request_id"] == requestID {
			n++
		}
	}
	return n
}
