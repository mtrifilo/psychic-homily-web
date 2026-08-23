package notification

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRenderEmailArtifact writes the exact HTML a send would hand Resend to the
// path in RENDER_OUT, and skips when that variable is unset.
//
// Assertions can only pin strings; whether the frame, rhythm and palette
// actually look right has to be judged by eye in a browser. This captures the
// real payload for that, rather than a hand-copied approximation that can drift
// away from what ships:
//
//	RENDER_OUT=/tmp/verify.html go test ./internal/services/notification/ \
//	  -run TestRenderEmailArtifact
func TestRenderEmailArtifact(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("set RENDER_OUT to a file path to capture the rendered email")
	}

	svc, emails, _ := setupEmailTest(t)
	svc.frontendURL = "https://psychichomily.com"

	// A realistic verification JWT, so the plain-link fallback is exercised at
	// the length recipients will actually see.
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJzdWIiOiI0MjEiLCJwdXJwb3NlIjoiZW1haWxfdmVyaWZpY2F0aW9uIiwiZXhwIjoxNzcyMDAwMDAwfQ." +
		"qN3sQ7bXk1r8mZ0vD2wLpYtH5cA9fJgU6eR4iO1nS8k"

	require.NoError(t, svc.SendVerificationEmail("someone@example.com", token))
	require.NoError(t, os.WriteFile(out, []byte((<-emails).Html), 0o600))
	t.Logf("wrote %s", out)
}
