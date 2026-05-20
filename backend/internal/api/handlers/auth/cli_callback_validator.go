package auth

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// errInvalidCLICallback is returned when a cli_callback value is not on the
// loopback allowlist. The message is intentionally generic — we do not echo
// the rejected value back to the caller.
var errInvalidCLICallback = errors.New("cli_callback must be a loopback (localhost) http URL")

// allowedCLICallbackHosts is the set of hostnames the CLI OAuth callback may
// target. The CLI runs a transient HTTP listener on the user's own machine, so
// the only legitimate destinations are loopback addresses.
//
// NOTE: the in-repo `cli/` client (token-paste `ph init`) does not currently
// use the OAuth `cli_callback` flow at all, and no custom URI scheme
// (e.g. `psychichomily://`) exists anywhere in the codebase. The allowlist is
// therefore restricted to loopback. If a future CLI build adopts a custom
// scheme, add it here deliberately rather than widening host matching.
var allowedCLICallbackHosts = map[string]struct{}{
	"localhost": {},
	"127.0.0.1": {},
	"::1":       {},
}

// validateCLICallback enforces that a cli_callback redirect target points only
// at the local machine. It defends against open-redirect token exfiltration:
// the OAuth flow appends a 24h JWT to this URL, so an attacker-controlled host
// would receive the victim's credentials.
//
// On success it returns the parsed-and-re-serialized URL (canonical form) so
// callers store a normalized value. On failure it returns errInvalidCLICallback.
//
// Rejected by construction: non-http schemes (javascript:, data:, file://,
// https), non-loopback hosts (evil.com), unicode/punycode-spoofed loopback,
// loopback as a credential or subdomain (http://127.0.0.1@evil.com,
// http://localhost.evil.com), and empty/garbage input.
func validateCLICallback(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errInvalidCLICallback
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", errInvalidCLICallback
	}

	// Only plain http is valid for a local loopback listener. This rejects
	// https, file, data, javascript, and any custom scheme outright.
	if u.Scheme != "http" {
		return "", errInvalidCLICallback
	}

	// Reject embedded credentials (http://127.0.0.1@evil.com parses Host as
	// evil.com, but be explicit and refuse any userinfo component).
	if u.User != nil {
		return "", errInvalidCLICallback
	}

	// u.Hostname() strips any :port and surrounding [] for IPv6, giving the
	// bare host. This is what we match against the allowlist — matching the
	// full Host would let "127.0.0.1.evil.com" or a spoofed port slip through.
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errInvalidCLICallback
	}

	// Defense against unicode/punycode homograph spoofing of "localhost"
	// (e.g. "lоcalhost" with a Cyrillic 'о', or its xn-- punycode form):
	// require the host to be pure ASCII. Loopback names/IPs are all ASCII.
	if !isASCII(host) {
		return "", errInvalidCLICallback
	}

	if _, ok := allowedCLICallbackHosts[host]; !ok {
		// Also accept any IPv4 loopback address (127.0.0.0/8), matching the
		// OS convention that the entire block is loopback.
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return "", errInvalidCLICallback
		}
	}

	return u.String(), nil
}

// isASCII reports whether s contains only ASCII bytes. Used to reject
// unicode-spoofed hostnames before allowlist matching.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}
