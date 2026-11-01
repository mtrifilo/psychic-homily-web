package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/testlog"
)

// bootRefusalLines serves one refusal into a JSON handler at INFO, the
// production level, and returns the records written.
func bootRefusalLines(t *testing.T, hub *sentry.Hub, capture bool) []map[string]any {
	t.Helper()
	log, out := testlog.NewJSONCapture(slog.LevelInfo)
	reportBootRefusal(log, hub, bootStageSchemaAssertion, errors.New("probe refusal"), capture)
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
		assertOneBootRefusalLine(t, bootRefusalLines(t, sentry.NewHub(nil, sentry.NewScope()), true))
	})

	t.Run("logs at ERROR with no hub", func(t *testing.T) {
		assertOneBootRefusalLine(t, bootRefusalLines(t, nil, true))
	})

	t.Run("captures one fatal event tagged with the stage", func(t *testing.T) {
		hub, events := recordingSentryHub(t)
		assertOneBootRefusalLine(t, bootRefusalLines(t, hub, true))

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
		assertOneBootRefusalLine(t, bootRefusalLines(t, hub, false))
		if n := len(events()); n != 0 {
			t.Errorf("Sentry captured %d events, want 0", n)
		}
	})
}

const bootRefusalChildEnv = "PSY_TEST_BOOT_REFUSAL_CHILD"

// refuseBoot exits the process with status 1 after writing its ERROR line
// through the logger production initializes, with no Sentry client in the
// process. It runs in a child process, since it exits.
func TestRefuseBootExitsOne(t *testing.T) {
	if os.Getenv(bootRefusalChildEnv) == "1" {
		logger.Init(true, false)
		refuseBoot(bootStageConfig, errors.New("probe refusal"))
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
