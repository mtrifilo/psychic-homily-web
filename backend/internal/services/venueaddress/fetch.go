// Package venueaddress finds a venue's street address on the venue's own web
// pages: it fetches a page politely, reads any schema.org address the page
// publishes, and falls back to the AI extraction path for an address printed
// only as text.
package venueaddress

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html/charset"

	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/utils/urlguard"
)

const (
	fetchTimeout      = 15 * time.Second
	fetchMaxRedirects = 5
	// fetchMaxBody bounds what one page may cost in memory; a venue homepage
	// is far smaller, and anything larger is not a page worth parsing.
	fetchMaxBody = 3 << 20
	// hostMinInterval spaces requests to the same host, robots.txt included,
	// so a run that visits several venues sharing one site (a multi-room
	// promoter, a ticket vendor) never bursts it.
	hostMinInterval = 2 * time.Second
	// robotsProductToken is the User-agent token a site's robots.txt may name
	// to address this fetcher specifically.
	robotsProductToken = "psychichomily"
)

// ErrUnavailable marks a definitive answer that the page cannot be read:
// asking again tomorrow is expected to give the same answer, so the caller may
// record the attempt as a miss. Definitive: a 4xx status (429 aside), a
// non-HTML body, a robots.txt rule, a login-walled host, a malformed or
// over-long redirect chain, a host name that does not resolve, a host that
// resolves to a non-public address, and a TLS certificate that fails
// verification. Anything else (a timeout, a refused connection, a 5xx, a 429)
// is transient.
var ErrUnavailable = errors.New("page unavailable")

// loginWalledHosts serve their pages only behind a login or a consent wall, so
// they are never fetched, whether named directly or reached by a redirect.
var loginWalledHosts = []string{
	"instagram.com", "facebook.com", "fb.com", "fb.me", "twitter.com", "x.com",
	"tiktok.com", "threads.net",
}

// LoginWalled reports whether host (or a parent domain of it) is login-walled.
func LoginWalled(host string) bool {
	host = strings.ToLower(strings.TrimPrefix(strings.ToLower(host), "www."))
	for _, h := range loginWalledHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// Page is a fetched HTML document.
type Page struct {
	URL  string // the final URL after redirects
	HTML []byte // UTF-8
}

// Fetcher reads public web pages for the backfill. Every connection goes
// through a dialer that refuses non-public addresses, because the URLs come
// from contributor-editable venue fields; robots.txt is honoured per host; and
// requests to one host are spaced by hostMinInterval.
type Fetcher struct {
	client    *http.Client
	userAgent string

	mu       sync.Mutex
	robots   map[string]*robotsRules // host -> parsed rules; nil entry = allow all
	lastCall map[string]time.Time    // host -> last request start
	interval time.Duration
}

// NewFetcher builds the production fetcher.
func NewFetcher() *Fetcher {
	dialer := &net.Dialer{Timeout: fetchTimeout, Control: urlguard.DialControl}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		// Keep-alive lets robots.txt and the page after it share one
		// connection; the dial guard ran when that connection was opened.
		IdleConnTimeout: 10 * time.Second,
	}
	return newFetcher(&http.Client{Timeout: fetchTimeout, Transport: transport}, hostMinInterval)
}

// newFetcher wraps client so that it never follows a redirect itself: Fetch
// follows each hop by hand so every hop passes the same checks as the first
// URL.
func newFetcher(client *http.Client, interval time.Duration) *Fetcher {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Fetcher{
		client:    &c,
		userAgent: fmt.Sprintf("PsychicHomily/1.0 (venue-address-backfill; %s)", geo.ContactChannel()),
		robots:    map[string]*robotsRules{},
		lastCall:  map[string]time.Time{},
		interval:  interval,
	}
}

// Fetch returns the page at rawURL, following up to fetchMaxRedirects
// redirects. Every hop, the first included, must be http(s), must not be a
// login-walled host, must be allowed by its host's robots.txt, and waits out
// its host's spacing interval. A returned error wrapping ErrUnavailable is
// definitive; any other error is transient.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Page, error) {
	target := strings.TrimSpace(rawURL)
	for hop := 0; ; hop++ {
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return nil, fmt.Errorf("%w: not an http(s) URL", ErrUnavailable)
		}
		if LoginWalled(u.Hostname()) {
			return nil, fmt.Errorf("%w: %s is login-walled", ErrUnavailable, u.Hostname())
		}
		allowed, err := f.robotsAllow(ctx, u)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, fmt.Errorf("%w: robots.txt disallows this path", ErrUnavailable)
		}

		resp, err := f.get(ctx, u)
		if err != nil {
			return nil, err
		}
		if next, isRedirect := redirectTarget(resp, u); isRedirect {
			_ = resp.Body.Close()
			if next == "" {
				return nil, fmt.Errorf("%w: redirect without a usable Location", ErrUnavailable)
			}
			if hop >= fetchMaxRedirects {
				return nil, fmt.Errorf("%w: more than %d redirects", ErrUnavailable, fetchMaxRedirects)
			}
			target = next
			continue
		}
		return readPage(resp, u)
	}
}

// redirectTarget reports whether resp is a redirect and, if so, the absolute
// URL its Location names ("" when Location is missing or unparseable).
func redirectTarget(resp *http.Response, from *url.URL) (string, bool) {
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		return "", false
	}
	loc := strings.TrimSpace(resp.Header.Get("Location"))
	next, err := from.Parse(loc)
	if loc == "" || err != nil {
		return "", true
	}
	return next.String(), true
}

// readPage turns a final response into a Page, classifying a failed status.
func readPage(resp *http.Response, u *url.URL) (*Page, error) {
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	default:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "html") {
		return nil, fmt.Errorf("%w: content type %q is not HTML", ErrUnavailable, ct)
	}
	body, err := readUTF8(resp.Body, ct)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &Page{URL: u.String(), HTML: body}, nil
}

// get issues one GET (no redirect following) after waiting out the host's
// spacing interval.
func (f *Fetcher) get(ctx context.Context, u *url.URL) (*http.Response, error) {
	if err := f.waitHost(ctx, strings.ToLower(u.Hostname())); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := f.client.Do(req)
	if err != nil {
		if definitiveTransportError(err) {
			return nil, fmt.Errorf("%w: %w", ErrUnavailable, stripURL(err))
		}
		return nil, fmt.Errorf("request failed: %w", stripURL(err))
	}
	return resp, nil
}

// definitiveTransportError reports whether a transport failure will repeat on
// every attempt: the name does not resolve, the dial guard refused the
// address, or the certificate does not verify.
func definitiveTransportError(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}
	if errors.Is(err, urlguard.ErrNonPublicAddress) {
		return true
	}
	var verifyErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	return errors.As(err, &verifyErr) || errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostname) || errors.As(err, &invalid)
}

// waitHost blocks until hostMinInterval has passed since the last request to
// host, then records this one.
func (f *Fetcher) waitHost(ctx context.Context, host string) error {
	f.mu.Lock()
	wait := f.interval - time.Since(f.lastCall[host])
	if wait < 0 {
		wait = 0
	}
	f.lastCall[host] = time.Now().Add(wait)
	f.mu.Unlock()
	if wait == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// robotsAllow reports whether robots.txt on u's host permits fetching u's
// path, fetching and caching the file on first use. A missing robots.txt (any
// 4xx) allows everything; a 5xx or transport failure is transient, because
// the site has not said yes.
func (f *Fetcher) robotsAllow(ctx context.Context, u *url.URL) (bool, error) {
	host := strings.ToLower(u.Host)
	f.mu.Lock()
	rules, cached := f.robots[host]
	f.mu.Unlock()
	if !cached {
		var err error
		rules, err = f.loadRobots(ctx, u.Scheme+"://"+u.Host+"/robots.txt")
		if err != nil {
			return false, err
		}
		f.mu.Lock()
		f.robots[host] = rules
		f.mu.Unlock()
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return rules.allows(path), nil
}

// loadRobots fetches and parses robots.txt, following up to fetchMaxRedirects
// redirects (a site commonly redirects it to https or to www).
func (f *Fetcher) loadRobots(ctx context.Context, robotsURL string) (*robotsRules, error) {
	u, err := url.Parse(robotsURL)
	if err != nil {
		return nil, fmt.Errorf("robots.txt: %w", err)
	}
	for hop := 0; ; hop++ {
		resp, err := f.get(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("robots.txt: %w", err)
		}
		if next, isRedirect := redirectTarget(resp, u); isRedirect {
			_ = resp.Body.Close()
			nu, perr := url.Parse(next)
			if next == "" || perr != nil || hop >= fetchMaxRedirects || (nu.Scheme != "http" && nu.Scheme != "https") {
				return nil, nil // an unusable robots.txt redirect says nothing; allow
			}
			u = nu
			continue
		}
		defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
			if err != nil {
				return nil, fmt.Errorf("robots.txt: read body: %w", err)
			}
			return parseRobots(string(body), robotsProductToken), nil
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			return nil, nil
		default:
			return nil, fmt.Errorf("robots.txt: status %d", resp.StatusCode)
		}
	}
}

// readUTF8 reads at most fetchMaxBody bytes and converts them to UTF-8 from
// the charset the Content-Type header or the document declares.
// A body whose declared charset is unknown is returned as read.
func readUTF8(r io.Reader, contentType string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, fetchMaxBody))
	if err != nil {
		return nil, err
	}
	decoded, err := charset.NewReader(bytes.NewReader(raw), contentType)
	if err != nil {
		return raw, nil
	}
	out, err := io.ReadAll(decoded)
	if err != nil {
		return raw, nil
	}
	return out, nil
}

// stripURL drops the request URL net/http embeds in a *url.Error, keeping the
// operation and the cause, so a wrapped error names what failed without
// repeating a URL the caller already reports beside it.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}
