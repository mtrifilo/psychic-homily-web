package auth

import (
	"strings"
	"testing"
)

func TestValidateCLICallback_Allowed(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"loopback ipv4 with port", "http://127.0.0.1:8888/callback"},
		{"loopback ipv4 no port", "http://127.0.0.1/callback"},
		{"loopback ipv4 block", "http://127.0.0.5:53682/cb"},
		{"loopback ipv6 with port", "http://[::1]:8888/callback"},
		{"loopback ipv6 no port", "http://[::1]/callback"},
		{"localhost with port", "http://localhost:8888/cli-cb"},
		{"localhost uppercase", "http://LOCALHOST:8888/cb"},
		{"localhost with query", "http://localhost:8888/cb?state=abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateCLICallback(tc.raw)
			if err != nil {
				t.Fatalf("expected %q to be allowed, got error: %v", tc.raw, err)
			}
			if got == "" {
				t.Errorf("expected non-empty canonical URL for %q", tc.raw)
			}
		})
	}
}

func TestValidateCLICallback_Rejected(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"external host", "https://evil.com/steal"},
		{"external host http", "http://evil.com/steal"},
		{"javascript scheme", "javascript:alert(document.cookie)"},
		{"data scheme", "data:text/html,<script>alert(1)</script>"},
		{"file scheme", "file:///etc/passwd"},
		{"https loopback", "https://127.0.0.1:8888/cb"},  // loopback but wrong scheme
		{"https localhost", "https://localhost:8888/cb"}, // loopback but wrong scheme
		{"custom scheme loopback", "psychichomily://127.0.0.1/cb"},
		{"loopback as subdomain", "http://127.0.0.1.evil.com/cb"},
		{"localhost as subdomain", "http://localhost.evil.com/cb"},
		{"loopback as credential", "http://127.0.0.1@evil.com/cb"},
		{"localhost as credential", "http://localhost@evil.com/cb"},
		{"unicode spoofed localhost", "http://lоcalhost:8888/cb"}, // Cyrillic 'о'
		{"punycode spoofed localhost", "http://xn--lcalhost-9zd:8888/cb"},
		{"protocol-relative", "//evil.com/cb"},
		{"bare host no scheme", "evil.com/cb"},
		{"public ip", "http://93.184.216.34/cb"},
		{"private non-loopback ip", "http://192.168.1.1/cb"},
		{"link-local ip", "http://169.254.169.254/cb"}, // cloud metadata SSRF target
		{"garbage", "http://%zz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateCLICallback(tc.raw)
			if err == nil {
				t.Fatalf("expected %q to be rejected, got allowed value %q", tc.raw, got)
			}
		})
	}
}

// callbackOfLength builds a loopback cli_callback exactly n bytes long, the
// path padded with pad.
func callbackOfLength(t *testing.T, n int, pad string) string {
	t.Helper()
	const prefix = "http://localhost:8888/"
	raw := prefix + strings.Repeat(pad, (n-len(prefix))/len(pad))
	if len(raw) != n {
		t.Fatalf("built a %d-byte callback, want %d", len(raw), n)
	}
	return raw
}

func TestValidateCLICallback_LengthCap(t *testing.T) {
	atCap := callbackOfLength(t, cliCallbackMaxLength, "a")
	got, err := validateCLICallback(atCap)
	if err != nil {
		t.Fatalf("expected a %d-byte callback to be allowed, got %v", len(atCap), err)
	}
	if got != atCap {
		t.Errorf("expected the at-cap callback unchanged, got %d bytes", len(got))
	}

	for _, n := range []int{cliCallbackMaxLength + 1, 10000} {
		if got, err := validateCLICallback(callbackOfLength(t, n, "a")); err == nil {
			t.Errorf("expected a %d-byte callback to be rejected, got %d bytes back", n, len(got))
		}
	}

	// Within the cap as given, over it once canonicalized: each space in the
	// path becomes "%20". The store holds the canonical form and the callback
	// re-validates it, so the start has to refuse it.
	if _, err := validateCLICallback(callbackOfLength(t, 100, " a")); err != nil {
		t.Fatalf("expected a short callback with spaces in its path to be allowed, got %v", err)
	}
	expands := callbackOfLength(t, cliCallbackMaxLength-10, " a")
	if got, err := validateCLICallback(expands); err == nil {
		t.Errorf("expected a callback whose canonical form is over the cap to be rejected, got %d bytes back", len(got))
	}
}
