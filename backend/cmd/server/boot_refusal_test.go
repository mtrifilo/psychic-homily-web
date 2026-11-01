package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/testlog"
)

// bootRefusalLines serves one refusal into a JSON handler at INFO, the
// production level, and returns the records written.
func bootRefusalLines(t *testing.T, hub *sentry.Hub, report bootRefusalReport) []map[string]any {
	t.Helper()
	log, out := testlog.NewJSONCapture(slog.LevelInfo)
	reportBootRefusal(log, hub, bootStageSchemaAssertion, errors.New("probe refusal"), report)
	return logObjectsIn(out.String())
}

func assertOneBootRefusalLine(t *testing.T, records []map[string]any) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("%d lines, want the one refusal line: %v", len(records), records)
	}
	line := records[0]
	if line["level"] != "ERROR" || line["msg"] != bootRefusalMsg ||
		line["stage"] != bootStageSchemaAssertion || line["error"] != "probe refusal" {
		t.Fatalf("line %v, want msg %q at ERROR with stage %q and the error", line, bootRefusalMsg, bootStageSchemaAssertion)
	}
}

func assertDatabaseConnectRefusalLine(t *testing.T, records []map[string]any) {
	t.Helper()
	if len(records) != 1 || records[0]["level"] != "ERROR" || records[0]["msg"] != bootRefusalMsg || records[0]["stage"] != bootStageDatabaseConnect {
		t.Errorf("lines %v, want one %q line at ERROR with stage %q", records, bootRefusalMsg, bootStageDatabaseConnect)
	}
}

// recordingSentryHub returns a hub on a client that records every event it
// would send instead of sending it.
func recordingSentryHub(t *testing.T) (*sentry.Hub, func() []*sentry.Event) {
	t.Helper()
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			events = append(events, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("sentry test client: %v", err)
	}
	return sentry.NewHub(client, sentry.NewScope()), func() []*sentry.Event { return events }
}

func TestBootRefusal(t *testing.T) {
	t.Run("logs at ERROR with no Sentry client", func(t *testing.T) {
		assertOneBootRefusalLine(t, bootRefusalLines(t, sentry.NewHub(nil, sentry.NewScope()), reportBootError))
	})

	t.Run("logs at ERROR with no hub", func(t *testing.T) {
		assertOneBootRefusalLine(t, bootRefusalLines(t, nil, reportBootError))
	})

	t.Run("captures one fatal event tagged with the stage", func(t *testing.T) {
		hub, events := recordingSentryHub(t)
		assertOneBootRefusalLine(t, bootRefusalLines(t, hub, reportBootError))

		got := events()
		if len(got) != 1 {
			t.Fatalf("Sentry captured %d events, want 1", len(got))
		}
		ev := got[0]
		if ev.Level != sentry.LevelFatal || ev.Tags["source"] != bootRefusalSentrySource || ev.Tags["boot_stage"] != bootStageSchemaAssertion {
			t.Errorf("event level %q tags %v, want fatal with source %q and boot_stage %q", ev.Level, ev.Tags, bootRefusalSentrySource, bootStageSchemaAssertion)
		}
		if len(ev.Exception) == 0 || ev.Exception[0].Value != "probe refusal" {
			t.Errorf("event exception %v, want the refusal error", ev.Exception)
		}
	})

	t.Run("a stage that captured its own event is not captured again", func(t *testing.T) {
		hub, events := recordingSentryHub(t)
		assertOneBootRefusalLine(t, bootRefusalLines(t, hub, reportBootNothing))
		if n := len(events()); n != 0 {
			t.Errorf("Sentry captured %d events, want 0", n)
		}
	})

	// A DATABASE_URL the driver cannot parse comes back in its error with the
	// password in it, in shapes the Sentry scrub does not match. The database
	// connect stage captures only its stage, so neither shape reaches Sentry;
	// the refusal is still one ERROR line.
	for _, tc := range []struct{ name, dsn, password string }{
		{name: "space in the password", dsn: "postgres://app:pa ss-probe-secret@db.internal:5432/ph", password: "ss-probe-secret"},
		{name: "hash in the password", dsn: "postgres://app:probesecret#tail@db.internal:5432/ph", password: "probesecret"},
	} {
		t.Run("database connect captures only the stage: "+tc.name, func(t *testing.T) {
			connectErr := db.Connect(&config.Config{Database: config.DatabaseConfig{URL: tc.dsn}})
			if connectErr == nil || !strings.Contains(connectErr.Error(), tc.password) {
				t.Fatalf("connect error %v: want one that carries the password, the shape this guards", connectErr)
			}

			hub, events := recordingSentryHub(t)
			log, out := testlog.NewJSONCapture(slog.LevelInfo)
			reportBootRefusal(log, hub, bootStageDatabaseConnect, connectErr, reportBootStageOnly)

			got := events()
			if len(got) != 1 || got[0].Tags["boot_stage"] != bootStageDatabaseConnect {
				t.Fatalf("Sentry captured %v, want one event tagged with the stage", got)
			}
			for _, ex := range got[0].Exception {
				if strings.Contains(ex.Value, tc.password) {
					t.Errorf("Sentry exception %q carries the password", ex.Value)
				}
			}
			assertDatabaseConnectRefusalLine(t, logObjectsIn(out.String()))
		})
	}
}

const bootRefusalChildEnv = "PSY_TEST_BOOT_REFUSAL_CHILD"

// refuseBoot exits the process with status 1 after writing its ERROR line
// through the logger production initializes, with no Sentry client in the
// process. It runs in a child process, since it exits.
func TestRefuseBootExitsOne(t *testing.T) {
	if os.Getenv(bootRefusalChildEnv) == "1" {
		logger.Init(true, false)
		refuseBoot(bootStageConfig, errors.New("probe refusal"), reportBootError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRefuseBootExitsOne$", "-test.count=1")
	cmd.Env = append(os.Environ(), bootRefusalChildEnv+"=1", "SENTRY_DSN=")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("child exit = %v, want status 1; stdout:\n%s", err, stdout.String())
	}

	refusals := recordsWithMsg(logObjectsIn(stdout.String()), bootRefusalMsg)
	if len(refusals) != 1 || refusals[0]["level"] != "ERROR" || refusals[0]["stage"] != bootStageConfig || refusals[0]["error"] != "probe refusal" {
		t.Fatalf("refusal lines %v, want one at ERROR with stage %q and the error; stdout:\n%s", refusals, bootStageConfig, stdout.String())
	}
}

const backendOriginChildEnv = "PSY_TEST_BACKEND_ORIGIN_CHILD"

// A production config, loaded from the environment, whose FRONTEND_URL the
// backend-origin resolver does not recognise and whose BACKEND_PUBLIC_URL is
// unset refuses boot at stage frontend_url with status 1. The deployed and
// local shapes that must keep booting return.
func TestRefuseUnresolvableBackendOrigin(t *testing.T) {
	if os.Getenv(backendOriginChildEnv) == "1" {
		logger.Init(true, false)
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("config.Load: %v", err)
		}
		refuseUnresolvableBackendOrigin(cfg, os.Getenv(config.EnvEnvironment))
		return
	}

	for _, shape := range []struct {
		environment string
		cfg         *config.Config
	}{
		{"production", &config.Config{Email: config.EmailConfig{FrontendURL: "https://www.psychichomily.com"}}},
		{"production", &config.Config{Email: config.EmailConfig{FrontendURL: "https://psychichomily.com"}}},
		{"stage", &config.Config{Email: config.EmailConfig{FrontendURL: "https://stage.psychichomily.com"}}},
		{"development", &config.Config{Email: config.EmailConfig{FrontendURL: "http://localhost:3000"}}},
		{"test", &config.Config{
			Server: config.ServerConfig{BackendPublicURL: "http://localhost:61001"},
			Email:  config.EmailConfig{FrontendURL: "http://localhost:61000"},
		}},
		{"production", &config.Config{
			Server: config.ServerConfig{BackendPublicURL: "https://api.example.invalid"},
			Email:  config.EmailConfig{FrontendURL: "https://example.invalid"},
		}},
	} {
		refuseUnresolvableBackendOrigin(shape.cfg, shape.environment)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRefuseUnresolvableBackendOrigin$", "-test.count=1")
	cmd.Env = append(os.Environ(), backendOriginChildEnv+"=1", "SENTRY_DSN=",
		"ENVIRONMENT=production",
		"FRONTEND_URL=https://example.invalid",
		"BACKEND_PUBLIC_URL=",
		"DATABASE_URL=postgres://u:p@db.example.test:5432/d",
		"JWT_SECRET_KEY=boot-refusal-jwt-secret",
		"OAUTH_SECRET_KEY=boot-refusal-oauth-secret",
		"INTERNAL_API_SECRET="+strings.Repeat("x", 32),
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("child exit = %v, want status 1; stdout:\n%s", err, stdout.String())
	}
	refusals := recordsWithMsg(logObjectsIn(stdout.String()), bootRefusalMsg)
	if len(refusals) != 1 || refusals[0]["level"] != "ERROR" || refusals[0]["stage"] != bootStageFrontendURL {
		t.Fatalf("refusal lines %v, want one at ERROR with stage %q; stdout:\n%s", refusals, bootStageFrontendURL, stdout.String())
	}
	if msg, _ := refusals[0]["error"].(string); !strings.Contains(msg, `(host "example.invalid")`) {
		t.Errorf("refusal error %q does not name the frontend host", msg)
	}
}
