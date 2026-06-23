package catalog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"psychic-homily-backend/internal/utils"
)

// loadFixture reads a testdata HTML fixture, failing the test if missing.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// =============================================================================
// extractFeaturedReleasePath — extraction from recorded HTML fixtures
// =============================================================================

func TestExtractFeaturedReleasePath(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		wantPath string
		wantOK   bool
	}{
		{
			// Real boris.bandcamp.com root: grid's first item is an album. A decoy
			// /album/ nav link BEFORE the grid must NOT win — grid-anchoring picks
			// the discography order, not the first album href on the page.
			name:     "album-first grid, decoy nav link ignored",
			fixture:  "bandcamp_profile_album_first.html",
			wantPath: "/album/fangsanalsatan-vol-25-in-gifu",
			wantOK:   true,
		},
		{
			// Real nope.bandcamp.com root: grid's first item is a standalone track.
			name:     "track-first grid",
			fixture:  "bandcamp_profile_track_first.html",
			wantPath: "/track/youdo-betterbymyself",
			wantOK:   true,
		},
		{
			// No music-grid block: the resolver fills NOTHING rather than risk
			// picking a nav/featured decoy link from elsewhere on the page. The
			// fixture has a release-shaped href (/album/lone-release) OUTSIDE any
			// grid — it must NOT be returned.
			name:    "no music-grid yields no path (grid is the only signal)",
			fixture: "bandcamp_profile_no_grid.html",
			wantOK:  false,
		},
		{
			// Single-release root: a one-item grid still resolves (the spec named
			// single-release + many-release validation; the others are 2-item).
			name:     "single-release grid",
			fixture:  "bandcamp_profile_single_release.html",
			wantPath: "/album/the-only-record",
			wantOK:   true,
		},
		{
			// A leading EMPTY <ol id="music-grid"> (a layout/AB-test placeholder)
			// must NOT shadow the populated grid that follows — iterate all grids
			// and take the first with a release. The decoy nav /album link before
			// both grids must also be ignored.
			name:     "leading empty music-grid does not shadow the populated one",
			fixture:  "bandcamp_profile_leading_empty_grid.html",
			wantPath: "/album/the-real-first-release",
			wantOK:   true,
		},
		{
			// Empty profile (no releases): nothing to extract.
			name:    "empty profile yields no path",
			fixture: "bandcamp_profile_empty.html",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := loadFixture(t, tt.fixture)
			got, ok := extractFeaturedReleasePath(html)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (path=%q)", ok, tt.wantOK, got)
			}
			if ok && got != tt.wantPath {
				t.Fatalf("path = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

// A percent-encoded slug in the extracted href must round-trip into the embed
// URL intact — not get its '%' double-encoded into a 404 path.
func TestResolveProfileEmbed_PercentEncodedSlugRoundTrips(t *testing.T) {
	const body = `<ol id="music-grid"><li class="music-grid-item"><a href="/track/with%20space">x</a></li></ol>`
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer cleanup()

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://enc.bandcamp.com")
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	want := "https://enc.bandcamp.com/track/with%20space"
	if embed != want {
		t.Fatalf("embed = %q, want %q (slug must not be double-encoded)", embed, want)
	}
}

// Defensive: a malformed/garbage body must not panic and must report no match.
func TestExtractFeaturedReleasePath_NoPanicOnGarbage(t *testing.T) {
	for _, body := range []string{"", "<<<<", "<html", strings.Repeat("<a href=", 1000)} {
		if _, ok := extractFeaturedReleasePath(body); ok {
			t.Fatalf("garbage %q unexpectedly extracted a path", body)
		}
	}
}

// =============================================================================
// isAllowedBandcampFetchURL — SSRF host anchor
// =============================================================================

func TestIsAllowedBandcampFetchURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://boris.bandcamp.com", true},
		{"https://boris.bandcamp.com/", true},
		{"https://bandcamp.com", true},
		{"https://sub.deep.bandcamp.com/music", true},
		// http rejected — https-only narrows the SSRF surface.
		{"http://boris.bandcamp.com", false},
		// substring-of-host attacks
		{"https://bandcamp.com.evil.test/", false},
		{"https://evilbandcamp.com/", false},
		{"https://notbandcamp.com/", false},
		// userinfo smuggling an alternate authority
		{"https://boris.bandcamp.com@evil.test/", false},
		{"https://user:pass@evil.test/?x=bandcamp.com", false},
		// raw IP / metadata endpoint
		{"https://169.254.169.254/?x=bandcamp.com", false},
		{"https://127.0.0.1/album/x", false},
		// non-http schemes
		{"file:///etc/passwd", false},
		{"ftp://boris.bandcamp.com/", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				if tt.want {
					t.Fatalf("parse %q failed but expected allowed: %v", tt.raw, err)
				}
				return
			}
			if got := isAllowedBandcampFetchURL(u); got != tt.want {
				t.Fatalf("isAllowedBandcampFetchURL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// =============================================================================
// ResolveProfileEmbed — end-to-end against an httptest server
// =============================================================================

type errStr string

func (e errStr) Error() string { return string(e) }

// rewriteHostRoundTripper sends every request to targetURL's host:port but leaves
// the request's URL (and thus the SSRF gate) keyed on the original *.bandcamp.com
// host. This lets a test serve fixtures for a real-looking bandcamp URL.
type rewriteHostRoundTripper struct {
	target *url.URL
	rt     http.RoundTripper
}

func (h *rewriteHostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = h.target.Scheme
	clone.URL.Host = h.target.Host
	resp, err := h.rt.RoundTrip(clone)
	if resp != nil {
		// Restore the ORIGINAL *.bandcamp.com request URL on the response so the
		// resolver's final-URL host anchor (ResolveProfileEmbed) sees the
		// bandcamp host it dialed logically, not the 127.0.0.1 test-server host we
		// physically routed to.
		resp.Request = req
	}
	return resp, err
}

// cannedResponse is a fixed reply for one logical bandcamp host.
type cannedResponse struct {
	status   int
	location string // when set, a redirect Location (an absolute https://*.bandcamp.com URL)
	body     string
}

// cannedRoundTripper answers each request from a per-logical-host table WITHOUT a
// real network, so a test can drive the real http.Client through a multi-hop
// cross-host redirect chain and observe production CheckRedirect + resp.Request
// behavior faithfully. Keyed on the request URL's host, so the resolver's
// final-URL host anchor sees the REAL logical host of the hop that produced the
// response (not a rewritten 127.0.0.1).
type cannedRoundTripper struct {
	byHost map[string]cannedResponse
	hits   map[string]int // host → times fetched (assert SSRF targets are never hit)
}

func (c *cannedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if c.hits == nil {
		c.hits = map[string]int{}
	}
	c.hits[host]++
	cr, ok := c.byHost[host]
	if !ok {
		// Unmapped host (an SSRF target the test expects never to be reached):
		// return a sentinel 500 with no body so a leak is visible via hits[].
		cr = cannedResponse{status: http.StatusInternalServerError}
	}
	h := http.Header{}
	if cr.location != "" {
		h.Set("Location", cr.location)
	}
	return &http.Response{
		StatusCode: cr.status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(cr.body)),
		Request:    req, // the hop that produced this response
	}, nil
}

// newCannedResolver builds a resolver wired to the canned table, using the
// PRODUCTION redirect policy (re-anchor every hop) so redirect handling is the
// real one.
func newCannedResolver(rt *cannedRoundTripper) *BandcampProfileResolver {
	client := &http.Client{
		Transport: rt,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errStr("too many redirects")
			}
			if !isAllowedBandcampFetchURL(req.URL) {
				return errStr("redirect to disallowed host: " + req.URL.Hostname())
			}
			return nil
		},
	}
	return NewBandcampProfileResolverWithClient(client)
}

// HIGH (adversarial): a profile root that 30x's to a DIFFERENT *.bandcamp.com
// subdomain must anchor the embed on the FINAL subdomain — the page (and its
// site-relative /album path) belongs to that host, not the input host. This is
// the most-documented branch in the resolver and was previously untested.
func TestResolveProfileEmbed_CrossSubdomainRedirectAnchorsOnFinalHost(t *testing.T) {
	rt := &cannedRoundTripper{byHost: map[string]cannedResponse{
		"a.bandcamp.com": {status: http.StatusFound, location: "https://b.bandcamp.com/music"},
		"b.bandcamp.com": {status: http.StatusOK, body: `<ol id="music-grid"><li class="music-grid-item"><a href="/album/on-b">x</a></li></ol>`},
	}}
	resolver := newCannedResolver(rt)

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://a.bandcamp.com")
	if !ok {
		t.Fatal("expected cross-subdomain redirect to resolve")
	}
	// The embed MUST be on b.bandcamp.com (the final host), NOT a.bandcamp.com.
	want := "https://b.bandcamp.com/album/on-b"
	if embed != want {
		t.Fatalf("embed = %q, want %q (must anchor on the FINAL subdomain)", embed, want)
	}
}

// A profile root whose FINAL hop is the bare apex (bandcamp.com) producing an
// /album path passes the fetch allowlist (apex-OK) but the built embed fails the
// strict IsValidBandcampEmbedURL gate (apex excluded) — so resolveProfileEmbedForArtist
// skips it. Here we assert the resolver itself returns the apex-hosted embed and
// that the strict validator rejects it (the artist.go fill then logs+skips).
func TestResolveProfileEmbed_ApexFinalHostFailsStrictEmbedGate(t *testing.T) {
	rt := &cannedRoundTripper{byHost: map[string]cannedResponse{
		"x.bandcamp.com": {status: http.StatusFound, location: "https://bandcamp.com/music"},
		"bandcamp.com":   {status: http.StatusOK, body: `<ol id="music-grid"><li class="music-grid-item"><a href="/album/apex">x</a></li></ol>`},
	}}
	resolver := newCannedResolver(rt)

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://x.bandcamp.com")
	if !ok {
		t.Fatal("resolver should still build an apex-hosted embed (apex is on the fetch allowlist)")
	}
	if embed != "https://bandcamp.com/album/apex" {
		t.Fatalf("embed = %q, want apex-hosted", embed)
	}
	// The downstream fill gate (utils.IsValidBandcampEmbedURL) rejects the apex →
	// resolveProfileEmbedForArtist logs WARN + skips, so no apex URL is stored.
	if utils.IsValidBandcampEmbedURL(embed) {
		t.Fatal("apex-hosted embed must FAIL the strict embed gate so the fill skips it")
	}
}

// resolverServingFixture builds a resolver whose client serves `body` for any
// *.bandcamp.com URL (rewriting the dial target to the httptest server) while the
// SSRF allowlist still sees the bandcamp.com host. Returns the resolver and a
// cleanup.
func resolverServingFixture(t *testing.T, handler http.Handler) (*BandcampProfileResolver, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{
		Transport: &rewriteHostRoundTripper{target: target, rt: http.DefaultTransport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !isAllowedBandcampFetchURL(req.URL) {
				return errStr("redirect to disallowed host: " + req.URL.Hostname())
			}
			return nil
		},
	}
	return NewBandcampProfileResolverWithClient(client), srv.Close
}

func TestResolveProfileEmbed_AlbumFirst(t *testing.T) {
	html := loadFixture(t, "bandcamp_profile_album_first.html")
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer cleanup()

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://boris.bandcamp.com")
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	want := "https://boris.bandcamp.com/album/fangsanalsatan-vol-25-in-gifu"
	if embed != want {
		t.Fatalf("embed = %q, want %q", embed, want)
	}
}

func TestResolveProfileEmbed_TrackFirst(t *testing.T) {
	html := loadFixture(t, "bandcamp_profile_track_first.html")
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer cleanup()

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://nope.bandcamp.com")
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	want := "https://nope.bandcamp.com/track/youdo-betterbymyself"
	if embed != want {
		t.Fatalf("embed = %q, want %q", embed, want)
	}
}

func TestResolveProfileEmbed_EmptyProfileNoEmbed(t *testing.T) {
	html := loadFixture(t, "bandcamp_profile_empty.html")
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer cleanup()

	if _, ok := resolver.ResolveProfileEmbed(context.Background(), "https://empty.bandcamp.com"); ok {
		t.Fatal("expected no embed for an empty profile")
	}
}

// SSRF: a non-bandcamp input host is rejected BEFORE any fetch (the server would
// fail the test if it were ever hit).
func TestResolveProfileEmbed_RejectsNonBandcampHost(t *testing.T) {
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server must not be hit for a disallowed host; got %s", r.URL)
	}))
	defer cleanup()

	for _, bad := range []string{
		"http://boris.bandcamp.com",         // http
		"https://bandcamp.com.evil.test/",   // substring host
		"https://169.254.169.254/?bandcamp.com",
		"https://user@evil.test/",           // userinfo
		"not a url",
	} {
		if _, ok := resolver.ResolveProfileEmbed(context.Background(), bad); ok {
			t.Fatalf("disallowed input %q unexpectedly resolved", bad)
		}
	}
}

// SSRF: a profile that 30x's to a NON-bandcamp host must be refused by the
// redirect re-anchor — the resolver returns no embed and never reaches the
// off-allowlist target.
func TestResolveProfileEmbed_RejectsRedirectToOtherHost(t *testing.T) {
	// Build two servers: the "bandcamp" one 302s to an "internal" one. The
	// internal one must never be reached.
	internalHit := false
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHit = true
		_, _ = w.Write([]byte(`<ol id="music-grid"><a href="/album/leaked">x</a></ol>`))
	}))
	defer internal.Close()

	bandcamp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a host that is NOT *.bandcamp.com (the internal server's
		// real http://127.0.0.1:port URL).
		http.Redirect(w, r, internal.URL+"/album/leaked", http.StatusFound)
	}))
	defer bandcamp.Close()

	bcTarget, _ := url.Parse(bandcamp.URL)
	client := &http.Client{
		Transport: &rewriteHostRoundTripper{target: bcTarget, rt: http.DefaultTransport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Production policy: re-anchor every hop. The Location points at the
			// internal server's 127.0.0.1 host → rejected.
			if !isAllowedBandcampFetchURL(req.URL) {
				return errStr("redirect to disallowed host: " + req.URL.Hostname())
			}
			return nil
		},
	}
	resolver := NewBandcampProfileResolverWithClient(client)

	embed, ok := resolver.ResolveProfileEmbed(context.Background(), "https://boris.bandcamp.com")
	if ok {
		t.Fatalf("expected redirect-to-other-host to be refused, got embed %q", embed)
	}
	if internalHit {
		t.Fatal("SSRF: the internal (non-bandcamp) redirect target was fetched")
	}
}

// A non-200 from the profile (404/500) yields no embed, not an error.
func TestResolveProfileEmbed_Non200NoEmbed(t *testing.T) {
	resolver, cleanup := resolverServingFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer cleanup()

	if _, ok := resolver.ResolveProfileEmbed(context.Background(), "https://gone.bandcamp.com"); ok {
		t.Fatal("expected no embed on a 404 profile")
	}
}

// isBandcampProfileRoot classification (the wiring gate).
func TestIsBandcampProfileRoot(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://boris.bandcamp.com", true},
		{"https://boris.bandcamp.com/", true},
		{"http://boris.bandcamp.com", true}, // gate accepts http; the FETCH is https-only
		{"https://boris.bandcamp.com/music", true},
		// already an embeddable release URL → not a profile to resolve
		{"https://boris.bandcamp.com/album/x", false},
		{"https://boris.bandcamp.com/track/y", false},
		// bare apex is not an artist profile
		{"https://bandcamp.com", false},
		{"https://bandcamp.com/discover", false},
		// foreign host
		{"https://evil.test/", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := isBandcampProfileRoot(tt.raw); got != tt.want {
				t.Fatalf("isBandcampProfileRoot(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
