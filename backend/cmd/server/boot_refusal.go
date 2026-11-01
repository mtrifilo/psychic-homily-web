package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// Boot stages, the stage attribute of a boot refusal's log line and the
// boot_stage tag of the Sentry event it captures. Each names the check or
// setup step whose error refused the boot.
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
	// bootRefusalSentrySource is the "source" tag of the Sentry event a boot
	// refusal captures.
	bootRefusalSentrySource = "boot_refusal"
	// bootRefusalFlushTimeout bounds how long a refusal waits for Sentry to
	// send its events before the process exits.
	bootRefusalFlushTimeout = 2 * time.Second
)

// bootRefusalReport is what a boot refusal captures in Sentry.
type bootRefusalReport int

const (
	// reportBootError captures the refusal's error.
	reportBootError bootRefusalReport = iota
	// reportBootStageOnly captures an error naming only the stage, for a stage
	// whose error text can carry a credential the Sentry scrub does not
	// recognise: the driver echoes a DATABASE_URL it cannot parse, password
	// included. The full error stays on the log line.
	reportBootStageOnly
	// reportBootNothing captures nothing, for a stage whose check has already
	// captured its own event for this error.
	reportBootNothing
)

// refuseBoot writes one ERROR line naming the stage and the error, captures
// what report says in Sentry at fatal level when the current hub has a client,
// waits up to bootRefusalFlushTimeout for Sentry to send, and exits with
// status 1. os.Exit skips deferred calls, so the flush here is the only one
// that runs.
func refuseBoot(stage string, err error, report bootRefusalReport) {
	reportBootRefusal(slog.Default(), sentry.CurrentHub(), stage, err, report)
	os.Exit(1)
}

// reportBootRefusal is everything a refusal does short of exiting. The ERROR
// line is written whatever the hub; a hub with no client (Sentry not yet
// initialised, or SENTRY_DSN unset) is neither captured to nor flushed.
func reportBootRefusal(log *slog.Logger, hub *sentry.Hub, stage string, err error, report bootRefusalReport) {
	log.Error(bootRefusalMsg, "stage", stage, "error", err)
	if hub == nil || hub.Client() == nil {
		return
	}

	var captured error
	switch report {
	case reportBootError:
		captured = err
	case reportBootStageOnly:
		captured = fmt.Errorf("boot refused at stage %s; the error is on the %q log line", stage, bootRefusalMsg)
	}
	if captured != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
			scope.SetTag("source", bootRefusalSentrySource)
			scope.SetTag("boot_stage", stage)
			hub.CaptureException(captured)
		})
	}
	hub.Flush(bootRefusalFlushTimeout)
}
