package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"psychic-homily-backend/internal/utils"
)

// PSY-1190: resolve a Bandcamp PROFILE root (band.bandcamp.com) to an embeddable
// /album|/track URL.
//
// Discovered Bandcamp links (MusicBrainz url-rels) and many existing
// social.bandcamp values are profile roots, not the album/track URLs the
// artist-page player needs in artists.bandcamp_embed_url. A profile root renders
// a discography grid (and a bare root 30x's to /music, which renders the same
// grid); the FIRST grid item is the band's featured/latest release. This service
// fetches the root and extracts that item's /album|/track URL.
//
// It is the Go counterpart of frontend/lib/bandcamp.ts. That FE resolver embeds a
// known album/track URL; this one resolves the PROFILE → a featured album URL,
// which the FE resolver (or the iframe) can then embed.

// bandcampFetchTimeout bounds a single profile fetch. The resolver runs in the
// artist write path; without a bound a slow/hung Bandcamp connection would tie up
// the request. Mirrors the FE BANDCAMP_FETCH_TIMEOUT_MS (8s).
const bandcampFetchTimeout = 8 * time.Second

// bandcampUserAgent identifies the resolver to Bandcamp; mirrors the FE header.
const bandcampUserAgent = "Mozilla/5.0 (compatible; MusicEmbed/1.0)"

// bandcampResolverMaxBytes caps how much of a profile page is read into memory.
// A real Bandcamp root is ~150KB; 2MB is a generous ceiling that defends against
// a hostile/oversized body without truncating a legitimate page.
const bandcampResolverMaxBytes = 2 << 20 // 2 MiB

// musicGridBlockRe isolates the discography grid (<ol id="music-grid"> … </ol>).
// The featured/latest release is the first grid item; anchoring on the grid keeps
// nav-bar and "featured"-link hrefs (which also point at /album|/track) from
// being mistaken for the discography order. (?s) lets . span newlines; the lazy
// .*? stops at the first </ol>.
var musicGridBlockRe = regexp.MustCompile(`(?s)<ol[^>]*id="music-grid"[^>]*>.*?</ol>`)

// gridItemHrefRe pulls the first /album/<slug> or /track/<slug> path from an
// anchor href. The path is taken up to the first #, ?, or closing quote so a
// query/fragment doesn't leak into the stored URL.
var gridItemHrefRe = regexp.MustCompile(`<a\s+[^>]*href="(/(?:album|track)/[^"#?]+)`)

// BandcampProfileResolver fetches a Bandcamp profile root and extracts a
// featured/latest album/track URL. The HTTP client is injectable so tests can
// point it at an httptest server; production uses newBandcampResolverClient().
type BandcampProfileResolver struct {
	httpClient *http.Client
}

// NewBandcampProfileResolver builds a resolver with an SSRF-hardened HTTP client
// (host-anchored redirects, https-only, bounded timeout).
func NewBandcampProfileResolver() *BandcampProfileResolver {
	return &BandcampProfileResolver{httpClient: newBandcampResolverClient()}
}

// NewBandcampProfileResolverWithClient injects an HTTP client (tests).
func NewBandcampProfileResolverWithClient(client *http.Client) *BandcampProfileResolver {
	return &BandcampProfileResolver{httpClient: client}
}

// newBandcampResolverClient builds the production HTTP client. The CheckRedirect
// hook re-validates EVERY redirect hop's target against the *.bandcamp.com
// allowlist (and https-only), so a profile that 30x's to an internal host is
// refused before the second request is issued — the SSRF defense can't be
// bypassed via redirect. A bare root legitimately 30x's to /music on the SAME
// host, so this allows that hop while refusing a cross-host one.
func newBandcampResolverClient() *http.Client {
	return &http.Client{
		Timeout: bandcampFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if !isAllowedBandcampFetchURL(req.URL) {
				return fmt.Errorf("redirect to disallowed host: %s", req.URL.Hostname())
			}
			return nil
		},
	}
}

// isAllowedBandcampFetchURL is the SSRF host-anchor for a fetched URL: https-only,
// no userinfo, and a host that is exactly bandcamp.com or a *.bandcamp.com
// subdomain. Mirrors the FE isAllowedBandcampUrl. A substring check would pass
// hostile values like https://bandcamp.com.evil.test/ or
// https://169.254.169.254/?x=bandcamp.com — parsing + an exact (sub)domain match
// rejects them. https-only (not http) because every legit Bandcamp page is https,
// and http would widen the SSRF surface to cleartext internal services.
func isAllowedBandcampFetchURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	// Reject embedded credentials (https://user:pass@host/ or https://x@host/):
	// the userinfo can smuggle an alternate authority past a naive host read.
	if u.User != nil {
		return false
	}
	// Accept any bandcamp host for the FETCH allowlist — an artist subdomain OR
	// the bare apex (FE isAllowedBandcampUrl parity / defense in depth). The
	// profile-root CLASSIFIER (isBandcampProfileRoot) is what additionally
	// excludes the apex, since the apex isn't an artist profile to resolve.
	host := strings.ToLower(u.Hostname())
	return host == "bandcamp.com" || utils.IsBandcampArtistHost(host)
}

// ResolveProfileEmbed fetches profileURL (a *.bandcamp.com profile root) and
// returns the featured/latest /album|/track URL to embed, or "" with ok=false
// when the root can't be fetched or carries no embeddable release.
//
// SSRF: the URL is host-anchored here AND every redirect hop is re-anchored by
// the client's CheckRedirect, so neither the input nor a redirect can target a
// non-bandcamp host. An empty/odd profile (no discography grid, no album/track
// link) returns ("", false) — never an error or a panic — so a caller fills
// nothing rather than failing the triggering write.
func (r *BandcampProfileResolver) ResolveProfileEmbed(ctx context.Context, profileURL string) (string, bool) {
	trimmed := strings.TrimSpace(profileURL)
	u, err := url.Parse(trimmed)
	if err != nil || !isAllowedBandcampFetchURL(u) {
		return "", false
	}

	html, finalURL, ok := r.fetch(ctx, u.String())
	if !ok {
		return "", false
	}

	path, ok := extractFeaturedReleasePath(html)
	if !ok {
		return "", false
	}

	// Build the embed on the host of the page the path was EXTRACTED FROM, not the
	// input host. A profile root may 30x to another *.bandcamp.com subdomain (all
	// on-allowlist; the CheckRedirect re-anchor proved every hop stayed on
	// bandcamp), in which case the discography — and the site-relative /album path
	// — belongs to the FINAL subdomain. finalURL is re-anchored here (defense in
	// depth) before its host is trusted. The path is site-relative (/album/<slug>),
	// so resolving it against the final origin yields the strict /album|/track URL
	// artists.bandcamp_embed_url expects.
	if !isAllowedBandcampFetchURL(finalURL) {
		return "", false
	}
	embed := (&url.URL{Scheme: finalURL.Scheme, Host: finalURL.Host, Path: path}).String()
	return embed, true
}

// fetch GETs a Bandcamp profile page, returning its body, the FINAL request URL
// (after any redirects), and ok=true on a 200. Any non-200, transport error, or
// read error returns ok=false — the resolver treats an unfetchable profile as
// "nothing to fill", not an error. The final URL is reported so the caller can
// anchor the extracted path on the subdomain the page actually came from.
func (r *BandcampProfileResolver) fetch(ctx context.Context, fetchURL string) (string, *url.URL, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", nil, false
	}
	req.Header.Set("User-Agent", bandcampUserAgent)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", nil, false
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return "", nil, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, bandcampResolverMaxBytes))
	if err != nil {
		return "", nil, false
	}
	// resp.Request is the request that produced this response — its URL reflects
	// the final hop after redirects (Go updates it on each follow).
	return string(body), resp.Request.URL, true
}

// extractFeaturedReleasePath returns the site-relative /album|/track path of the
// profile's featured/latest release, or ok=false when none is present.
//
// Signal: the FIRST anchor inside the <ol id="music-grid"> discography grid.
// Bandcamp orders that grid featured/newest-first, so its first item is the
// release to embed. Anchoring on the grid is deliberate and the ONLY signal used:
// a nav-bar / "featured-grid" link elsewhere on the page also points at
// /album|/track, so a whole-page scan would mistake that decoy for the
// discography's first item. When the grid block is absent (an unrecognized layout
// we can't reason about), the resolver fills NOTHING rather than risk storing the
// wrong release — fill-when-empty makes a miss harmless and human-correctable,
// whereas a wrong embed is a silent defect.
func extractFeaturedReleasePath(html string) (string, bool) {
	grid := musicGridBlockRe.FindString(html)
	if grid == "" {
		return "", false
	}
	if m := gridItemHrefRe.FindStringSubmatch(grid); m != nil {
		return m[1], true
	}
	return "", false
}
