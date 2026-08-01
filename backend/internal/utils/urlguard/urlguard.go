// Package urlguard decides whether a user-supplied URL may be STORED, by
// classifying where its host actually points.
//
// The value it protects is show/entity image_url: writable by any
// email-verified user, and later fetched server-side by the share-card renderer
// (frontend/lib/og/remoteImage.ts). That renderer runs on the edge, which
// exposes no resolver, so it can only classify hosts that are already IP
// literals. A hostname that RESOLVES to 169.254.169.254 is invisible to it.
// Resolution happens here, at the write boundary, which is the only layer that
// has a resolver — so this is the layer that can close that gap.
//
// The two layers are complementary, not redundant:
//
//	write time (here)  — resolve the host, refuse a name that points inward.
//	fetch time (edge)  — re-check the IP LITERAL on every hop of every fetch,
//	                     which is what covers rows written before this guard
//	                     existed, and what covers a redirect chain this layer
//	                     never sees.
//
// What NEITHER layer covers, stated plainly so nobody has to rediscover it:
// DNS rebinding. An attacker who controls a zone can answer with a public
// address while this guard resolves, then flip to 169.254.169.254 before the
// card renders. The edge cannot catch it either — for a hostname it checks the
// NAME, not the address it will connect to, because it has no resolver. What
// bounds the damage is the far end of the pipe: only a parseable PNG, JPEG or
// GIF is ever drawn, so nothing read from an internal service comes back out
// on the card. What remains is blind request forgery. Closing it needs
// resolve-then-PIN (hand the fetcher the vetted IP, not the name), which is a
// change to the fetch layer, not to this one.
//
// Nothing here fetches. It parses, classifies, and resolves.
package urlguard

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/idna"
)

// Resolver is the DNS surface the guard needs. *net.Resolver satisfies it; a
// map-backed stub (MapResolver) satisfies it in tests, which must not depend on
// the machine's DNS.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// resolveTimeout bounds the lookup a write path waits on. A write blocked on a
// hostile nameserver is a worse outcome than a rejected flyer, and 2s is the
// same budget the edge renderer gives the whole fetch.
const resolveTimeout = 2 * time.Second

// Guard classifies URLs against a resolver.
type Guard struct {
	resolver Resolver
	timeout  time.Duration
}

// New returns a Guard using r, or the process resolver when r is nil.
func New(r Resolver) *Guard {
	if r == nil {
		r = net.DefaultResolver
	}
	return &Guard{resolver: r, timeout: resolveTimeout}
}

// Default is the guard the API write paths use. Tests replace its resolver via
// UseResolver rather than constructing their own, because the validators in
// internal/api/handlers/shared reach it as package state. Because it IS package
// state, a test that swaps it must not call t.Parallel.
var Default = New(nil)

// UseResolver swaps g's resolver and returns a func restoring the previous one,
// so a test reads `defer urlguard.Default.UseResolver(stub)()`.
func (g *Guard) UseResolver(r Resolver) (restore func()) {
	previous := g.resolver
	if r == nil {
		r = net.DefaultResolver
	}
	g.resolver = r
	return func() { g.resolver = previous }
}

// blockedHostNames and blockedHostSuffixes refuse names that denote "this
// machine" or a private namespace WITHOUT waiting on a resolver. Resolution
// below would catch localhost anyway; these are kept because they are free, and
// because they still hold when a split-horizon resolver answers with a public
// address for a name that is internal by construction.
//
// The set matches the edge guard's (frontend/lib/og/remoteImage.ts) so the two
// layers agree on what a "name that is internal on its face" is.
var blockedHostNames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true,
}

var blockedHostSuffixes = []string{".localhost", ".local", ".internal"}

// Validate reports why rawURL must not be stored in fieldName, or nil when the
// URL points somewhere public. Empty input passes so callers keep their
// "clear the field with an empty string" semantics.
//
// It fails CLOSED on anything it can inspect and does not like: an unparseable
// URL, a non-http scheme, a missing host, a host that IS a non-public address,
// a name that is internal on its face, a name that resolves to a non-public
// address. Rejecting an unparseable URL also covers the divergence Go's
// stricter parser creates: a host spelling Go refuses but the WHATWG parser at
// the edge would accept never reaches the column.
//
// It deliberately does NOT reject a name that fails to RESOLVE. That looks like
// a hole and is not worth what closing it costs:
//
//   - It closes nothing. The attack it appears to stop is an attacker whose
//     nameserver answers our resolver differently from the fetcher's. Such an
//     attacker does not need it — plain DNS rebinding (answer public now, flip
//     to 169.254.169.254 before the card renders) reaches the same place, is
//     strictly easier, and no write-time check can see it.
//   - It breaks ordinary editing, certainly and repeatedly. A flyer host that
//     later expires would otherwise make the whole show uneditable: the edit
//     form re-sends the stored image_url with every submit, so a dead flyer
//     host would 422 a title fix. A resolver blip would do the same to every
//     show that has a flyer.
//
// So an unresolvable host is treated as what it is — a URL that will render no
// image — and is left to the fetch layer.
//
// fieldName is the user-facing label interpolated into the message. The message
// names the HOST, never the addresses it resolved to: a submitter needs to know
// which host was refused, and echoing internal addresses back would hand over
// exactly the information an SSRF probe is looking for.
func (g *Guard) Validate(ctx context.Context, rawURL, fieldName string) error {
	host, err := CheckLiteralHost(rawURL, fieldName)
	if err != nil {
		return err
	}
	if host == "" {
		// Empty value, or a literal that already cleared the address check.
		return nil
	}
	return g.hostResolvesPublic(ctx, fieldName, host)
}

// CheckLiteralHost applies every classification that needs no resolver: parse,
// scheme, IDNA normalization, IP-literal address check, and the internal-name
// list. It returns the normalized host still awaiting DNS, or "" when there is
// nothing left to resolve (an empty value, or a public IP literal, which is
// already the address anything will connect to).
//
// It is separate from Validate so a caller that cannot afford a lookup per
// value — the discovery importer takes up to 100 events per request — can still
// refuse the literal forms, which are the ones an attacker actually writes.
func CheckLiteralHost(rawURL, fieldName string) (host string, err error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil
	}
	u, parseErr := url.Parse(trimmed)
	if parseErr != nil {
		return "", fmt.Errorf("%s must be a valid URL", fieldName)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https scheme", fieldName)
	}

	// A trailing dot is a valid absolute FQDN that resolvers honour, so it must
	// come off BEFORE any name comparison — "localhost." would otherwise match
	// nothing below. TrimRight, not TrimSuffix: "localhost.." is equally valid.
	host = strings.ToLower(strings.TrimRight(u.Hostname(), "."))
	if host == "" {
		return "", fmt.Errorf("%s must include a host", fieldName)
	}

	// Address literals are classified on the spelling as written. IDNA below
	// must not touch them: an IPv6 literal is not a domain name and folding one
	// fails, and a numeric IPv4 spelling is already ASCII.
	if ip := ParseHostIP(host); ip != nil {
		if !IsPublicIP(ip) {
			return "", blockedHostError(fieldName, host)
		}
		// A public literal needs no resolution.
		return "", nil
	}

	// Fold the remaining host to its ASCII (punycode) form before anything
	// compares it. Go's url.Parse does no IDNA, but the WHATWG parser the edge
	// uses does, so without this the two layers can look up DIFFERENT names:
	// "ⓁⓄⒸⒶⓁⒽⓄⓈⓉ" is an unrecognised name to Go and "localhost" to the edge.
	// Today the production binary is saved from that only by CGO_ENABLED=0 (the
	// pure Go resolver refuses non-ASCII names outright), which makes a
	// Dockerfile flag load-bearing for a security property. This makes it
	// explicit instead. A name that cannot be folded is refused: nothing can
	// look it up.
	ascii, idnaErr := idna.Lookup.ToASCII(host)
	if idnaErr != nil {
		return "", fmt.Errorf("%s host %q is not a valid hostname", fieldName, host)
	}
	host = ascii

	// Folding can TURN a name into a literal — fullwidth digits map to ASCII
	// ones, so "１２７.0.0.1" only becomes 127.0.0.1 here. Re-classify.
	if ip := ParseHostIP(host); ip != nil {
		if !IsPublicIP(ip) {
			return "", blockedHostError(fieldName, host)
		}
		return "", nil
	}
	if isBlockedHostName(host) {
		return "", blockedHostError(fieldName, host)
	}
	return host, nil
}

// hostResolvesPublic rejects a name when ANY address it resolves to is
// non-public. Any, not all: a name answering with one public and one loopback
// address hands the fetcher a choice we do not control.
//
// A lookup that fails is NOT a rejection — see Validate for why.
func (g *Guard) hostResolvesPublic(ctx context.Context, fieldName, host string) error {
	// A test binary that reaches a live resolver is a test that depends on the
	// network and on whatever DNS answers that day — and, worse, one whose SSRF
	// assertions can pass for the wrong reason. Packages that exercise a guarded
	// handler install a stub in TestMain (urlguard.Default.UseResolver); this
	// turns "forgot to" from a silent flake into a failure that names the fix.
	if testing.Testing() && g.resolver == net.DefaultResolver {
		panic("urlguard: a test reached the real resolver — install a stub in TestMain, e.g. " +
			`defer urlguard.Default.UseResolver(urlguard.MapResolver{"example.com": {"93.184.216.34"}})()`)
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	addrs, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if !IsPublicIP(addr.IP) {
			return blockedHostError(fieldName, host)
		}
	}
	return nil
}

// isBlockedHostName reports whether host is internal on its face. Suffixes are
// matched on a label boundary (the leading dot is part of the suffix), never as
// a substring, so "notlocal.example.com" is not mistaken for a ".local" name.
func isBlockedHostName(host string) bool {
	if blockedHostNames[host] {
		return true
	}
	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func blockedHostError(fieldName, host string) error {
	return fmt.Errorf("%s must not point to a private or internal address (host %q)", fieldName, host)
}

// MapResolver is a fixed resolution table: a Resolver for tests and for any
// caller that must classify a URL without touching DNS. A host absent from the
// map resolves to nothing, which the guard treats as unresolvable.
type MapResolver map[string][]string

// LookupIPAddr implements Resolver.
func (m MapResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	raw, ok := m[strings.ToLower(host)]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	addrs := make([]net.IPAddr, 0, len(raw))
	for _, s := range raw {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("urlguard: MapResolver has invalid address %q for host %q", s, host)
		}
		addrs = append(addrs, net.IPAddr{IP: ip})
	}
	return addrs, nil
}
