package main

import (
	"testing"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/api/middleware"
)

// A sampled transaction carries the request's headers, and sentry-go hands
// transactions to BeforeSendTransaction, not BeforeSend. Both hooks must scrub,
// or a credential header ships verbatim on every sampled request.
func TestSentryClientOptions_ScrubsSecretHeadersOnTransactionsAndErrors(t *testing.T) {
	opts := sentryClientOptions("", "test", false)
	hooks := map[string]func(*sentry.Event, *sentry.EventHint) *sentry.Event{
		"BeforeSend":            opts.BeforeSend,
		"BeforeSendTransaction": opts.BeforeSendTransaction,
	}
	for name, hook := range hooks {
		t.Run(name, func(t *testing.T) {
			if hook == nil {
				t.Fatalf("%s is not set", name)
			}
			const secret = "server-read-secret-value-0123456789"
			event := &sentry.Event{
				Type: "transaction",
				Request: &sentry.Request{Headers: map[string]string{
					middleware.ServerReadSecretHeader: secret,
					"X-Internal-Secret":               secret,
				}},
			}
			out := hook(event, nil)
			for header, value := range out.Request.Headers {
				if value == secret {
					t.Errorf("%s left %s = %q", name, header, value)
				}
			}
		})
	}
}
