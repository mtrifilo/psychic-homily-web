package notification

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingHub is a hub over a client that keeps every event and sends none.
func recordingHub(t *testing.T) (*sentry.Hub, *[]*sentry.Event) {
	t.Helper()
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			events = append(events, event)
			return nil
		},
	})
	require.NoError(t, err)
	return sentry.NewHub(client, sentry.NewScope()), &events
}

// A provider error is reported on the hub the context carries, with the
// email capture's own tags over that hub's scope.
func TestSend_ProviderErrorReportedOnTheContextHub(t *testing.T) {
	for _, tc := range []struct {
		kind string
		send func(*EmailService, context.Context) error
	}{
		{"magic_link", func(s *EmailService, ctx context.Context) error {
			return s.SendMagicLinkEmailContext(ctx, "user@test.com", "token")
		}},
		{"verification", func(s *EmailService, ctx context.Context) error {
			return s.SendVerificationEmailContext(ctx, "user@test.com", "token")
		}},
		{"account_recovery", func(s *EmailService, ctx context.Context) error {
			return s.SendAccountRecoveryEmailContext(ctx, "user@test.com", "token", 12)
		}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			svc := setupEmailTestError(t)
			hub, events := recordingHub(t)
			hub.ConfigureScope(func(scope *sentry.Scope) { scope.SetTag("service", "auth_email_outbox") })

			err := tc.send(svc, sentry.SetHubOnContext(context.Background(), hub))

			require.Error(t, err)
			require.Len(t, *events, 1)
			assert.Equal(t, "email", (*events)[0].Tags["service"])
			assert.Equal(t, tc.kind, (*events)[0].Tags["email_type"])

			hub.CaptureMessage("after")
			require.Len(t, *events, 2)
			assert.Equal(t, "auth_email_outbox", (*events)[1].Tags["service"],
				"the capture's tags do not stay on the caller's hub")
			assert.NotContains(t, (*events)[1].Tags, "email_type")
		})
	}
}

// A context without a hub reports through the current client, on a scope of
// its own: a scope another goroutine has pushed onto the global hub, with its
// tags, does not reach the event.
func TestSend_ProviderErrorWithoutAContextHub(t *testing.T) {
	svc := setupEmailTestError(t)
	recorder, events := recordingHub(t)
	global := sentry.CurrentHub()
	pushed := global.PushScope()
	t.Cleanup(global.PopScope)
	global.BindClient(recorder.Client())
	pushed.SetTag("pushed_elsewhere", "yes")
	pushed.SetTag("email_type", "someone_elses")

	err := svc.SendMagicLinkEmail("user@test.com", "token")

	assert.ErrorContains(t, err, "failed to send magic link email")
	require.Len(t, *events, 1, "the provider error is reported through the current client")
	assert.Equal(t, "email", (*events)[0].Tags["service"])
	assert.Equal(t, "magic_link", (*events)[0].Tags["email_type"])
	assert.NotContains(t, (*events)[0].Tags, "pushed_elsewhere", "no scope from the global hub")
}
