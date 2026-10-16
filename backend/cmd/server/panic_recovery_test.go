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

// A handler panic answers 500 with the error envelope naming the request, is
// captured by Sentry once with the request's id as its tag, and is logged once
// at ERROR with that id, for a route registered directly on chi and for a Huma
// operation alike; a panic after the response started cuts the response off.
// middleware.RecoverPanic's own tests pin the envelope, the scrubbing and the
// header handling; this test pins the order the production chain runs them in.
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

	cases := parseChildCases[panicCaseEnd](t, runTestChild(t, "TestHandlerPanicAnswers500", panicChildEnv))
	result := func(name string) (panicCaseEnd, []map[string]any) {
		c, ok := cases[name]
		if !ok {
			t.Fatalf("%s: the child served no such case", name)
		}
		return c.end, logObjects(c.lines)
	}

	for _, name := range []string{"raw-route", "huma-operation"} {
		end, records := result(name)
		id := end.RequestIDHeader
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("%s: X-Request-ID = %q, want a minted UUID", name, id)
		}
		var body middleware.JWTErrorResponse
		_ = json.Unmarshal([]byte(end.Body), &body)
		if end.Status != http.StatusInternalServerError || body.ErrorCode != autherrors.CodeUnknown || body.RequestID != id {
			t.Errorf("%s: status %d body %q, want 500 with the error envelope naming %s", name, end.Status, end.Body, id)
		}

		if len(end.Events) != 1 {
			t.Errorf("%s: Sentry captured %d events, want 1", name, len(end.Events))
		}
		for _, ev := range end.Events {
			if ev.RequestIDTag != id || !strings.Contains(ev.Exception, panicProbeMessage) {
				t.Errorf("%s: Sentry event %+v, want the panic value tagged with %s", name, ev, id)
			}
		}

		access, panics := 0, recordsWithMsg(records, panicLogMsg)
		for _, record := range recordsWithMsg(records, "request") {
			if record["request_id"] == id {
				access++
			}
		}
		if access != 1 {
			t.Errorf("%s: %d access lines carry the request id, want 1", name, access)
		}
		if len(panics) != 1 || panics[0]["level"] != "ERROR" || panics[0]["request_id"] != id || panics[0]["status"] != float64(http.StatusInternalServerError) {
			t.Errorf("%s: %s lines = %v, want one at ERROR with request_id %s and status 500", name, panicLogMsg, panics, id)
		}
		if n := len(recordsAtLevel(records, "ERROR")); n != 1 {
			t.Errorf("%s: %d ERROR lines, want only the %s line", name, n, panicLogMsg)
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
		if len(panics) != 1 || panics[0]["response_started"] != true || panics[0]["request_id"] != end.RequestIDHeader {
			t.Errorf("%s lines = %v, want one with response_started true and request_id %s", panicLogMsg, panics, end.RequestIDHeader)
		}
	})

	// The handler's own abort reaches net/http as the sentinel: the connection
	// closes, nothing is logged, and Sentry, which captures every panic it
	// sees, holds one event.
	t.Run("handler aborts", func(t *testing.T) {
		end, records := result("abort")
		if end.ClientError == "" {
			t.Errorf("status %d with body %q: want the connection aborted", end.Status, end.Body)
		}
		if n := len(recordsAtLevel(records, "ERROR")); n != 0 {
			t.Errorf("%d ERROR lines for http.ErrAbortHandler, want 0", n)
		}
		if len(end.Events) != 1 {
			t.Errorf("Sentry captured %d events, want 1", len(end.Events))
		}
	})
}

const (
	panicChildEnv     = "PSY_TEST_PANIC_RECOVERY_CHILD"
	panicLogMsg       = "handler_panic"
	panicProbeMessage = "probe panic"
)

// panicCaseEnd is what the child reports about one request once it is served.
type panicCaseEnd struct {
	Status          int          `json:"status"`
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

	probe := errors.New(panicProbeMessage)
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
	router.Get("/test-panic/abort", func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})
	router.Get("/test-panic/started", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		_ = http.NewResponseController(w).Flush()
		panic(probe)
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
		resp, err := server.Client().Get(server.URL + path)
		if err != nil {
			end.ClientError = err.Error()
		} else {
			end.Status = resp.StatusCode
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

func recordsAtLevel(records []map[string]any, level string) []map[string]any {
	var matched []map[string]any
	for _, record := range records {
		if record["level"] == level {
			matched = append(matched, record)
		}
	}
	return matched
}
