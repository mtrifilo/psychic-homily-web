package auth

import "testing"

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
