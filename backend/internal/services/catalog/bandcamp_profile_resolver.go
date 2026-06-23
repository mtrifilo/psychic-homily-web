package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
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

// musicGridOpenTagRe isolates JUST the <ol id="music-grid" …> OPENING tag, where
// the data-client-items JSON discography attribute lives. (?s) lets the attribute
// list span newlines (Bandcamp pretty-prints the tag); [^>]* stops at the tag's
// closing '>'. Used to scope the JSON extraction to the grid's own tag — not some
// other data-client-items attribute elsewhere on the page.
var musicGridOpenTagRe = regexp.MustCompile(`(?s)<ol[^>]*id="music-grid"[^>]*>`)

// clientItemsAttrRe pulls the (HTML-entity-escaped) JSON array out of the grid
// tag's data-client-items attribute. Bandcamp emits the full discography here even
// when the inline <li> children are lazily/partially rendered, so it is the
// resilient same-page fallback when the inline anchors yield nothing.
var clientItemsAttrRe = regexp.MustCompile(`data-client-items="([^"]*)"`)

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
// Modern Bandcamp roots render the discography several ways, so extraction is
// LAYERED with deterministic, first-match-wins fallbacks:
//
//	(a) inline <ol id="music-grid"> <a href="/album|/track"> children — the common
//	    layout (boris, nope, maletears);
//	(b) the grid tag's data-client-items JSON — same featured-first order, present
//	    even when (a)'s inline anchors are lazily/partially rendered;
//	(c) the artist's /music sub-page — when the ROOT 30x's to a single album page
//	    (PSY-1198 AJJ repro: ajjtheband.bandcamp.com → /album/… with no grid), the
//	    full discography grid still lives at /music; (a)/(b) then run on /music.
//
// Layers (a) and (b) run on the already-fetched page (extractFeaturedRelease); only
// (c) costs a second fetch, and only when the first page yields nothing — so the
// common path is still a single request.
//
// NOTE on the rejected "last-resort whole-page /album scan": the ticket proposed a
// 4th fallback — the first same-origin /album|/track href ANYWHERE on the page. It
// is deliberately NOT implemented: on the real AJJ album-landing page that very scan
// picks a TRACKLIST track (/track/body-terror-song-demo), not the featured release —
// the exact "wrong embed is a silent defect" the grid-anchoring was built to avoid.
// /music (layer c) already resolves every observed redirect-to-album case with the
// correct featured release. When neither the page nor /music has a grid, the artist
// has no resolvable discography and the resolver correctly fills NOTHING (the
// documented fill-when-empty invariant; the no_grid fixture pins it).
//
// SSRF: the input URL AND the /music URL are host-anchored here, and every redirect
// hop is re-anchored by the client's CheckRedirect, so neither input, redirect, nor
// the /music fetch can target a non-bandcamp host. An empty/odd profile (no grid,
// no JSON) returns ("", false) — never an error or a panic — so a caller fills
// nothing rather than failing the triggering write.
func (r *BandcampProfileResolver) ResolveProfileEmbed(ctx context.Context, profileURL string) (string, bool) {
	trimmed := strings.TrimSpace(profileURL)
	u, err := url.Parse(trimmed)
	if err != nil || !isAllowedBandcampFetchURL(u) {
		return "", false
	}

	// Layers (a)/(b): fetch the root (which may 30x to /music or to an album page)
	// and extract from whatever page we land on.
	body, finalURL, ok := r.fetch(ctx, u.String())
	if ok {
		if embed, ok := buildEmbed(body, finalURL); ok {
			return embed, true
		}
	}

	// Layer (c): the root yielded nothing (e.g. it 30x'd to a single album page with
	// no discography grid). The full discography grid still lives at the artist's
	// /music sub-page. Build /music on the INPUT origin (an artist subdomain we
	// already host-anchored) and fetch it through the SAME SSRF-guarded client; the
	// extraction then anchors on /music's own final URL (re-anchored in buildEmbed).
	musicURL := *u
	musicURL.Path = "/music"
	musicURL.RawQuery = ""
	musicURL.Fragment = ""
	if !isAllowedBandcampFetchURL(&musicURL) {
		return "", false
	}
	musicBody, musicFinalURL, ok := r.fetch(ctx, musicURL.String())
	if !ok {
		return "", false
	}
	return buildEmbed(musicBody, musicFinalURL)
}

// buildEmbed extracts a featured /album|/track path from one fetched page's body
// and resolves it into an absolute embed URL anchored on that page's FINAL host.
// Returns ("", false) when the page carries no embeddable release or the final host
// fails the SSRF re-anchor (defense in depth — the host is only trusted after this
// check).
func buildEmbed(body string, finalURL *url.URL) (string, bool) {
	path, ok := extractFeaturedRelease(body)
	if !ok {
		return "", false
	}

	// Build the embed on the host of the page the path was EXTRACTED FROM, not the
	// input host. A profile root may 30x to another *.bandcamp.com subdomain (all
	// on-allowlist; the CheckRedirect re-anchor proved every hop stayed on
	// bandcamp), in which case the discography — and the site-relative /album path
	// — belongs to the FINAL subdomain. finalURL is re-anchored here (defense in
	// depth) before its host is trusted.
	if !isAllowedBandcampFetchURL(finalURL) {
		return "", false
	}
	// Resolve the extracted href as a relative reference against the final origin.
	// finalURL.Parse preserves the href's EXISTING percent-encoding (a slug like
	// /track/with%20x round-trips intact), unlike constructing url.URL{Path: path}
	// which would treat the already-encoded path as decoded and double-encode the
	// '%'. The host is overwritten to the validated final host so a (regex-rejected
	// but defensively) absolute/host-bearing reference can't redirect the origin.
	ref, err := finalURL.Parse(path)
	if err != nil {
		return "", false
	}
	ref.Scheme = finalURL.Scheme
	ref.Host = finalURL.Host
	return ref.String(), true
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
	// the final hop after redirects (Go updates it on each follow). http.Client.Do
	// always sets it on a successful round-trip; guard defensively so a custom
	// transport that violates the contract can't nil-panic the caller.
	if resp.Request == nil || resp.Request.URL == nil {
		return "", nil, false
	}
	return string(body), resp.Request.URL, true
}

// extractFeaturedRelease returns the site-relative /album|/track path of a page's
// featured/latest release, trying deterministic SAME-PAGE fallbacks in order (first
// match wins) so the resolver handles every observed grid layout:
//
//	(a) extractFromMusicGridAnchors — the FIRST inline anchor in the
//	    <ol id="music-grid"> discography grid. The canonical, highest-confidence
//	    signal: Bandcamp orders the grid featured/newest-first.
//	(b) extractFromClientItemsJSON — the grid tag's data-client-items JSON, in the
//	    same featured-first order. Present even when the inline anchors are lazily
//	    or partially rendered, so it backstops (a) on the SAME page.
//
// (Layer (c), the /music sub-page, is a second FETCH handled by ResolveProfileEmbed,
// not a same-page extraction layer; it re-runs (a)/(b) on /music.) When NO layer
// yields a release the resolver fills NOTHING rather than risk storing the wrong
// release — fill-when-empty makes a miss harmless and human-correctable, whereas a
// wrong embed is a silent defect. Both layers anchor on the music-grid: a nav-bar /
// "featured-grid" decoy link elsewhere on the page is never picked.
func extractFeaturedRelease(html string) (string, bool) {
	if path, ok := extractFromMusicGridAnchors(html); ok {
		return path, true
	}
	return extractFromClientItemsJSON(html)
}

// extractFromMusicGridAnchors returns the first /album|/track href among the inline
// <a> children of the discography grid (layer a). Anchoring on the grid is
// deliberate: a nav-bar / "featured-grid" link elsewhere on the page also points at
// /album|/track, so a whole-page scan would mistake that decoy for the
// discography's first item.
//
// Iterate ALL music-grid blocks, not just the first: a layout/AB-test that emits a
// leading EMPTY <ol id="music-grid"> (e.g. a "featured" placeholder) before the
// populated one would otherwise make the resolver a silent no-op for that cohort.
// The first grid that actually contains an /album|/track anchor wins.
func extractFromMusicGridAnchors(html string) (string, bool) {
	for _, grid := range musicGridBlockRe.FindAllString(html, -1) {
		if m := gridItemHrefRe.FindStringSubmatch(grid); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// clientItem is one entry of the music-grid data-client-items JSON discography.
// Only page_url is needed; the array is already featured/newest-first, so the first
// album|track entry is the release to embed.
type clientItem struct {
	PageURL string `json:"page_url"`
	Type    string `json:"type"`
}

// extractFromClientItemsJSON returns the first /album|/track page_url from the grid
// tag's data-client-items JSON attribute (layer b). Bandcamp emits the full
// discography here even when the inline <li> anchors are lazily/partially rendered,
// so it is the resilient same-page backstop for layer (a).
//
// Scoped to the music-grid's OWN opening tag (musicGridOpenTagRe) so an unrelated
// data-client-items attribute elsewhere on the page can't feed it. The attribute is
// HTML-entity-escaped in the markup (&quot;, &#39;, …); it is unescaped before JSON
// parsing. A malformed/empty attribute returns ok=false (no panic).
func extractFromClientItemsJSON(htmlBody string) (string, bool) {
	for _, tag := range musicGridOpenTagRe.FindAllString(htmlBody, -1) {
		m := clientItemsAttrRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		var items []clientItem
		if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &items); err != nil {
			continue
		}
		for _, it := range items {
			if isReleasePath(it.PageURL) {
				return it.PageURL, true
			}
		}
	}
	return "", false
}

// isReleasePath reports whether a (site-relative) page_url is an /album or /track
// release path — used to skip non-release client-items entries (defensive; observed
// data is album-only).
func isReleasePath(p string) bool {
	return strings.HasPrefix(p, "/album/") || strings.HasPrefix(p, "/track/")
}
