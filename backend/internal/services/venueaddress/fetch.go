// Package venueaddress finds a venue's street address on the venue's own web
// pages: it fetches a page politely, reads any schema.org address the page
// publishes, and falls back to the AI extraction path for an address printed
// only as text.
package venueaddress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
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

// ErrUnavailable marks a definitive answer that the page cannot be read: a 4xx
// status, a non-HTML body, or a robots.txt rule. Unlike a transport failure or
// a 5xx, asking again tomorrow is expected to give the same answer, so the
// caller may record the attempt as a miss.
var ErrUnavailable = errors.New("page unavailable")

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
	dialer := &net.Dialer{Timeout: fetchTimeout, Control: publicOnlyDialControl}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		DisableKeepAlives:     true,
	}
	return newFetcher(&http.Client{
		Timeout:   fetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= fetchMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", fetchMaxRedirects)
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}, hostMinInterval)
}

func newFetcher(client *http.Client, interval time.Duration) *Fetcher {
	return &Fetcher{
		client:    client,
		userAgent: fmt.Sprintf("PsychicHomily/1.0 (venue-address-backfill; %s)", geo.ContactChannel()),
		robots:    map[string]*robotsRules{},
		lastCall:  map[string]time.Time{},
		interval:  interval,
	}
}

// Fetch returns the page at rawURL. A returned error wrapping ErrUnavailable
// is definitive; any other error is transient.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Page, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: not an http(s) URL", ErrUnavailable)
	}

	allowed, err := f.robotsAllow(ctx, u)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: robots.txt disallows this path", ErrUnavailable)
	}

	resp, err := f.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
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
	return &Page{URL: resp.Request.URL.String(), HTML: body}, nil
}

// get issues one GET after waiting out the host's spacing interval.
func (f *Fetcher) get(ctx context.Context, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err := f.waitHost(ctx, strings.ToLower(u.Hostname())); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", stripURL(err))
	}
	return resp, nil
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

func (f *Fetcher) loadRobots(ctx context.Context, robotsURL string) (*robotsRules, error) {
	resp, err := f.get(ctx, robotsURL)
	if err != nil {
		return nil, fmt.Errorf("robots.txt: %w", err)
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

// publicOnlyDialControl refuses any connection whose resolved target is not a
// routable public address. It runs after DNS resolution for every dial,
// including redirect hops, so a hostname that resolves to a private range,
// loopback, or a cloud metadata address is refused before a byte is sent.
func publicOnlyDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("dial guard: malformed address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("dial guard: non-IP dial host %q", host)
	}
	if !urlguard.IsPublicIP(ip) {
		return fmt.Errorf("dial guard: refusing non-public address %s", ip)
	}
	return nil
}
