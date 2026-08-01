package pipeline

import (
	"context"
	"testing"
)

// The IP classifier itself (urlguard.IsPublicIP) is exercised in
// internal/utils/urlguard, which owns it; this file covers only the dial hook
// that applies it.

// TestSSRFDialControl confirms the dial hook refuses non-public addresses and
// permits public ones.
func TestSSRFDialControl(t *testing.T) {
	if err := ssrfDialControl("tcp", "169.254.169.254:80", nil); err == nil {
		t.Errorf("ssrfDialControl must refuse the cloud-metadata address")
	}
	if err := ssrfDialControl("tcp", "127.0.0.1:443", nil); err == nil {
		t.Errorf("ssrfDialControl must refuse loopback")
	}
	if err := ssrfDialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("ssrfDialControl must permit a public address, got %v", err)
	}
	if err := ssrfDialControl("tcp", "garbage", nil); err == nil {
		t.Errorf("ssrfDialControl must reject a malformed address")
	}
}

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
