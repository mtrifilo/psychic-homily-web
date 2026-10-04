package pipeline

import (
	"context"
	"testing"
)

// TestIsLive_RejectsNonHTTPSchemes confirms the public IsLive entrypoint rejects
// non-network schemes before any transport work.
func TestIsLive_RejectsNonHTTPSchemes(t *testing.T) {
	c := NewSSRFSafeLivenessChecker()
	for _, u := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://host/x", "", "://nohost"} {
		if c.IsLive(context.Background(), u) {
			t.Errorf("IsLive(%q) = true, want false", u)
		}
	}
}
