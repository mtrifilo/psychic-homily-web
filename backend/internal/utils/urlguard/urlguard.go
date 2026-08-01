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
//	fetch time (edge)  — re-check the literal on every hop of every fetch,
//	                     which is what still covers rows written before this
//	                     guard existed and a DNS answer that changes after the
//	                     write (rebinding).
//
// Nothing here fetches. It parses, classifies, and resolves.
package urlguard

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
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
// It fails CLOSED on anything it cannot classify — an unparseable URL, a host
// that will not resolve, a resolver error. That is deliberate: a value this
// layer cannot vouch for is a value the fetcher may still reach, and a name
// that does not resolve produces no image anyway, so rejecting it costs a
// submitter nothing but a corrected URL. It also covers the case Go's stricter
// URL parser creates: a host spelling Go refuses but the WHATWG parser at the
// edge would accept is rejected here rather than waved through.
//
// fieldName is the user-facing label interpolated into the message. The message
// names the HOST, never the addresses it resolved to: a submitter needs to know
// which host was refused, and echoing internal addresses back would hand over
// exactly the information an SSRF probe is looking for.
func (g *Guard) Validate(ctx context.Context, rawURL, fieldName string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL", fieldName)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme", fieldName)
	}

	// A trailing dot is a valid absolute FQDN that resolvers honour, so it must
	// come off BEFORE any name comparison — "localhost." would otherwise match
	// nothing below.
	host := strings.ToLower(strings.TrimRight(u.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("%s must include a host", fieldName)
	}

	if ip := ParseHostIP(host); ip != nil {
		if !IsPublicIP(ip) {
			return blockedHostError(fieldName, host)
		}
		// A literal that IS public needs no resolution: it is already the
		// address anything will connect to.
		return nil
	}

	if isBlockedHostName(host) {
		return blockedHostError(fieldName, host)
	}
	return g.hostResolvesPublic(ctx, fieldName, host)
}

// hostResolvesPublic rejects a name unless EVERY address it resolves to is
// public. Every, not any: a name with one public and one loopback answer hands
// the fetcher a choice we do not control.
func (g *Guard) hostResolvesPublic(ctx context.Context, fieldName, host string) error {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	addrs, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("%s host %q could not be resolved", fieldName, host)
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
