package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// Every request served by the production chain writes exactly one access line
// at INFO, carrying the status the client received, the body bytes it
// received, the duration and the request id of the response header; the line
// written before the handler is Debug only, so production (debug off) carries
// none. The router is the production correlation stack in front of
// routes.SetupRoutes with the response compressor between them, served by a
// real net/http server, and the lines are read from a child process for the
// reason TestRequestCorrelationCarriesOneID gives.
func TestAccessLogCompletionLine(t *testing.T) {
	if os.Getenv(accessLogChildEnv) == "1" {
		serveAccessLogCases(t)
		return
	}

	cases := parseChildCases[accessLogCaseEnd](t, runTestChild(t, "TestAccessLogCompletionLine", accessLogChildEnv))
	for _, want := range accessLogCases {
		c, ok := cases[want.name]
		if !ok {
			t.Errorf("%s: the child served no such case", want.name)
			continue
		}
		end, records := c.end, logObjects(c.lines)
		if end.Status != want.status {
			t.Errorf("%s: status %d, want %d", want.name, end.Status, want.status)
		}
		if want.keepsInbound {
			if end.RequestIDHeader != want.inbound {
				t.Errorf("%s: X-Request-ID = %q, want the inbound %q", want.name, end.RequestIDHeader, want.inbound)
			}
		} else if _, err := uuid.Parse(end.RequestIDHeader); err != nil {
			t.Errorf("%s: X-Request-ID = %q, want a minted UUID", want.name, end.RequestIDHeader)
		}

		if started := recordsWithMsg(records, accessLogStartMsg); len(started) != 0 {
			t.Errorf("%s: %d %s lines with debug off, want 0: %v", want.name, len(started), accessLogStartMsg, started)
		}
		lines := recordsWithMsg(records, accessLogMsg)
		if len(lines) != 1 {
			t.Errorf("%s: %d access lines, want 1: %v", want.name, len(lines), lines)
			continue
		}
		line := lines[0]
		if line["level"] != "INFO" {
			t.Errorf("%s: access line level %v, want INFO", want.name, line["level"])
		}
		if line["request_id"] != end.RequestIDHeader {
			t.Errorf("%s: access line request_id %v, X-Request-ID = %q", want.name, line["request_id"], end.RequestIDHeader)
		}
		if line["method"] != http.MethodGet || line["path"] != want.path {
			t.Errorf("%s: access line method %v path %v, want GET %s", want.name, line["method"], line["path"], want.path)
		}
		if line["status"] != float64(end.Status) {
			t.Errorf("%s: access line status %v, response status %d", want.name, line["status"], end.Status)
		}
		if line["bytes"] != float64(end.BodyBytes) {
			t.Errorf("%s: access line bytes %v, response body %d bytes", want.name, line["bytes"], end.BodyBytes)
		}
		if ms, ok := line["duration_ms"].(float64); !ok || ms < 0 {
			t.Errorf("%s: access line duration_ms %v, want a non-negative number", want.name, line["duration_ms"])
		}
		if _, ok := line["aborted"]; ok {
			t.Errorf("%s: access line carries aborted for a response that completed: %v", want.name, line)
		}
	}
}

const accessLogChildEnv = "PSY_TEST_ACCESS_LOG_CHILD"

// accessLogCases are served by the child in order.
var accessLogCases = []struct {
	name, path, inbound string
	keepsInbound        bool
	status              int
}{
	{name: "ok", path: "/openapi.json", status: http.StatusOK},
	{name: "not-found", path: "/nope", status: http.StatusNotFound},
	{name: "panic", path: "/test-access-log/panic", status: http.StatusInternalServerError},
	{name: "inbound-id", path: "/nope", inbound: "client-request-42", keepsInbound: true, status: http.StatusNotFound},
	{name: "oversized-inbound-id", path: "/nope", inbound: strings.Repeat("x", 1000), status: http.StatusNotFound},
}

// accessLogCaseEnd is what the child reports about one request once it is
// served.
type accessLogCaseEnd struct {
	Status          int    `json:"status"`
	RequestIDHeader string `json:"request_id_header"`
	BodyBytes       int    `json:"body_bytes"`
}

// serveAccessLogCases runs in the child. The client asks for no compression,
// so the body it reads is the body the server wrote.
func serveAccessLogCases(t *testing.T) {
	logger.Init(true, false)

	cfg := &config.Config{
		JWT:   config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24},
		OAuth: config.OAuthConfig{SecretKey: "test-oauth-secret-key-32-chars"},
	}
	router := chi.NewMux()
	useRequestCorrelation(router)
	router.Use(middleware.ResponseCompression())
	routes.SetupRoutes(router, services.NewServiceContainer(nil, cfg), cfg)
	router.Get("/test-access-log/panic", func(http.ResponseWriter, *http.Request) {
		panic(errors.New("access log probe panic"))
	})

	server := httptest.NewServer(router)
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	for _, c := range accessLogCases {
		fmt.Println(caseBeginMarker + c.name)
		req, err := http.NewRequest(http.MethodGet, server.URL+c.path, nil)
		if err != nil {
			t.Fatalf("%s: build request: %v", c.name, err)
		}
		if c.inbound != "" {
			req.Header.Set(middleware.RequestIDHeader, c.inbound)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: request: %v", c.name, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("%s: read body: %v", c.name, err)
		}
		encoded, err := json.Marshal(accessLogCaseEnd{
			Status:          resp.StatusCode,
			RequestIDHeader: resp.Header.Get(middleware.RequestIDHeader),
			BodyBytes:       len(body),
		})
		if err != nil {
			t.Fatalf("%s: encode case end: %v", c.name, err)
		}
		fmt.Println(caseEndMarker + string(encoded))
	}
}

// accessLogRecords serves one request through RequestIDMiddleware, accessLog
// and RecoverPanic, in the order useRequestCorrelation registers them, with
// the access log written at level, and returns the access log records.
func accessLogRecords(t *testing.T, level slog.Level, handler http.HandlerFunc) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: level}))

	chain := middleware.RequestIDMiddleware(
		accessLog(func() *slog.Logger { return log })(
			middleware.RecoverPanic(handler)))
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set(middleware.RequestIDHeader, "probe-request-id")

	func() {
		// A panic RecoverPanic hands on reaches the test as it would net/http.
		defer func() { _ = recover() }()
		chain.ServeHTTP(httptest.NewRecorder(), req)
	}()
	return logObjects(strings.Split(strings.TrimSpace(out.String()), "\n"))
}

func TestAccessLogLines(t *testing.T) {
	writeOK := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }

	for _, tc := range []struct {
		name        string
		handler     http.HandlerFunc
		wantStatus  float64
		wantAborted bool
	}{
		{name: "200 with a body", handler: writeOK, wantStatus: 200},
		{name: "nothing written is the 200 net/http sends", handler: func(http.ResponseWriter, *http.Request) {}, wantStatus: 200},
		{name: "404", handler: http.NotFound, wantStatus: 404},
		{name: "panic answered 500 by RecoverPanic", handler: func(http.ResponseWriter, *http.Request) {
			panic("probe")
		}, wantStatus: 500},
		{name: "panic after the response started", handler: func(w http.ResponseWriter, r *http.Request) {
			writeOK(w, r)
			panic("probe")
		}, wantStatus: 200, wantAborted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := accessLogRecords(t, slog.LevelInfo, tc.handler)
			if len(records) != 1 {
				t.Fatalf("%d lines at INFO, want the one completion line: %v", len(records), records)
			}
			line := records[0]
			if line["msg"] != accessLogMsg || line["level"] != "INFO" {
				t.Fatalf("line %v, want msg %q at INFO", line, accessLogMsg)
			}
			for key, want := range map[string]any{
				"method":     http.MethodGet,
				"path":       "/probe",
				"origin":     "https://example.test",
				"request_id": "probe-request-id",
				"status":     tc.wantStatus,
			} {
				if line[key] != want {
					t.Errorf("%s = %v, want %v", key, line[key], want)
				}
			}
			if _, ok := line["duration_ms"].(float64); !ok {
				t.Errorf("duration_ms = %v, want a number", line["duration_ms"])
			}
			if _, ok := line["bytes"].(float64); !ok {
				t.Errorf("bytes = %v, want a number", line["bytes"])
			}
			if aborted, _ := line["aborted"].(bool); aborted != tc.wantAborted {
				t.Errorf("aborted = %v, want %v", line["aborted"], tc.wantAborted)
			}
		})
	}

	t.Run("bytes counts the body written", func(t *testing.T) {
		records := accessLogRecords(t, slog.LevelInfo, writeOK)
		if len(records) != 1 || records[0]["bytes"] != float64(len("ok")) {
			t.Fatalf("records %v, want one line with bytes %d", records, len("ok"))
		}
	})

	t.Run("the start line is written at DEBUG before the completion line", func(t *testing.T) {
		records := accessLogRecords(t, slog.LevelDebug, writeOK)
		if len(records) != 2 {
			t.Fatalf("%d lines at DEBUG, want the start and completion lines: %v", len(records), records)
		}
		start, done := records[0], records[1]
		if start["msg"] != accessLogStartMsg || start["level"] != "DEBUG" {
			t.Errorf("first line %v, want msg %q at DEBUG", start, accessLogStartMsg)
		}
		if start["request_id"] != "probe-request-id" || start["path"] != "/probe" {
			t.Errorf("start line %v, want the request id and path", start)
		}
		if _, ok := start["status"]; ok {
			t.Errorf("start line %v carries a status before the handler ran", start)
		}
		if done["msg"] != accessLogMsg || done["level"] != "INFO" {
			t.Errorf("second line %v, want msg %q at INFO", done, accessLogMsg)
		}
	})
}
