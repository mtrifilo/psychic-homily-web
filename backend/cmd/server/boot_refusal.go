package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// Boot stages, the stage attribute of a boot refusal's log line and Sentry
// tag. Each names the check or setup step whose error refused the boot.
const (
	bootStageConfig                       = "config"
	bootStageTestFixturesEnvironment      = "test_fixtures_environment"
	bootStageAuthRateLimitEnvironment     = "auth_rate_limit_environment"
	bootStageOAuthTestProviderEnvironment = "oauth_test_provider_environment"
	bootStageDatabaseConnect              = "database_connect"
	bootStageSchemaAssertion              = "schema_assertion"
	bootStageInvalidIndexCheck            = "invalid_index_check"
	bootStageGothSetup                    = "goth_setup"
	bootStageListen                       = "listen"
)

const (
	// bootRefusalMsg is the msg of the ERROR line a boot refusal writes.
	bootRefusalMsg = "boot refused"
	// bootRefusalSentrySource is the "source" tag of a boot refusal's Sentry
	// event.
	bootRefusalSentrySource = "boot_refusal"
	// bootRefusalFlushTimeout bounds how long a refusal waits for Sentry to
	// send its events before the process exits.
	bootRefusalFlushTimeout = 2 * time.Second
)

// refuseBoot writes one ERROR line naming the stage and the error, captures
// the error in Sentry at fatal level when the current hub has a client, waits
// up to bootRefusalFlushTimeout for Sentry to send, and exits with status 1.
// os.Exit skips deferred calls, so the flush here is the only one that runs.
func refuseBoot(stage string, err error) {
	reportBootRefusal(slog.Default(), sentry.CurrentHub(), stage, err, true)
	os.Exit(1)
}

// refuseBootCaptured is refuseBoot for a stage whose check has already
// captured its own Sentry event for this error: it logs, flushes that event
// and exits 1 without capturing a second one.
func refuseBootCaptured(stage string, err error) {
	reportBootRefusal(slog.Default(), sentry.CurrentHub(), stage, err, false)
	os.Exit(1)
}

// reportBootRefusal is everything a refusal does short of exiting. The ERROR
// line is written whatever the hub; a hub with no client (Sentry not yet
// initialised, or SENTRY_DSN unset) is neither captured to nor flushed.
func reportBootRefusal(log *slog.Logger, hub *sentry.Hub, stage string, err error, capture bool) {
	log.Error(bootRefusalMsg, "stage", stage, "error", err)
	if hub == nil || hub.Client() == nil {
		return
	}
	if capture {
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
			scope.SetTag("source", bootRefusalSentrySource)
			scope.SetTag("boot_stage", stage)
			hub.CaptureException(err)
		})
	}
	hub.Flush(bootRefusalFlushTimeout)
}
