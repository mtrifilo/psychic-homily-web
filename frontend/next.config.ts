import type { NextConfig } from "next";
import { withSentryConfig } from "@sentry/nextjs";
import withBundleAnalyzer from "@next/bundle-analyzer";
import { PHASE_PRODUCTION_BUILD } from "next/constants";

// This file imports packages only. Next compiles it without a parent module
// path, so a relative import resolves against the process cwd and fails when a
// command is given this directory from elsewhere (`next dev frontend`).

/**
 * Refuses a production build that would prerender against the production API
 * without anyone choosing it.
 *
 * With `NEXT_PUBLIC_API_URL` unset, `getApiBaseUrl` in lib/api-base.ts resolves
 * a production build to the production API origin, and prerender fetches
 * through it. Only `PHASE_PRODUCTION_BUILD` is checked: `next dev` resolves to
 * localhost and `next start` serves an existing build. An empty value counts as
 * unset, which is how `getApiBaseUrl` reads it too.
 *
 * A build with `VERCEL` set is exempt, so a missing project variable never
 * blocks a deploy. On Vercel the API origin comes from the project's
 * environment settings, and an unset value there still resolves to the
 * production API.
 */
export function assertBuildApiOriginConfigured(
  phase: string,
  env: Readonly<Record<string, string | undefined>>,
): void {
  if (phase !== PHASE_PRODUCTION_BUILD) return;
  if (env.VERCEL) return;
  if (env.NEXT_PUBLIC_API_URL) return;
  throw new Error(
    "NEXT_PUBLIC_API_URL is not set, so this production build would prerender " +
      "against the production API. Set it to https://stage.api.psychichomily.com " +
      "or a local backend before building. Builds with VERCEL set skip this check.",
  );
}

/**
 * One entity URL segment that addresses a slug, not a numeric id: lowercase
 * letters, digits, `-` and `.`, with at least one letter or `-`. Config route
 * matching is case-insensitive, so uppercase also matches and is substituted
 * as requested. A segment of digits alone gets no header: `/shows/{id}`
 * declares the show's slug URL as canonical, and the other families read such
 * a segment as an id alias.
 */
const ENTITY_SLUG_SEGMENT = '[0-9]*[a-z-][a-z0-9.-]*'

/**
 * A scene slug segment: `ENTITY_SLUG_SEGMENT` plus `'`, which a city name can
 * carry. It stays as it is in a URL path and in the header value.
 */
const SCENE_SLUG_SEGMENT = "[0-9]*[a-z-][a-z0-9.'-]*"

/**
 * A query key or cookie whose presence, with any non-empty value, withholds the
 * header.
 */
type MissingCondition = { type: 'query' | 'cookie'; key: string }

/**
 * A self-canonical route: `path` is the route with `:name` parameters, which is
 * what the `Link` header names, and `source` is the same route with each
 * parameter constrained to `ENTITY_SLUG_SEGMENT` unless `paramPatterns` gives
 * it another pattern. `missing` lists query keys or cookies the header requires
 * absent.
 */
function selfCanonicalRoute(
  path: string,
  paramPatterns: Readonly<Record<string, string>> = {},
  missing: MissingCondition[] = [],
): { source: string; path: string; missing: MissingCondition[] } {
  const source = path.replace(
    /:(\w+)/g,
    (_, name: string) => `:${name}(${paramPatterns[name] ?? ENTITY_SLUG_SEGMENT})`,
  )
  return { source, path, missing }
}

/**
 * Routes that get a canonical `Link` header. Each one holds both of these for
 * every request its `source` matches:
 *
 * - The page declares the request's own path on the apex host as canonical.
 * - A request whose entity the proxy's existence check reports missing, or a
 *   calendar window outside the addressable span, gets a real HTTP 404 from
 *   `proxy.ts` before anything streams.
 *
 * A not-found body can still go out at HTTP 200 under the header: when the
 * proxy's existence probe cannot answer and the request is passed through,
 * when the proxy reuses a positive verdict for a few minutes after the entity
 * is gone, and when a page treats any failed read of its entity as not found
 * (the venue page does) after a successful probe. Each names the request's own
 * URL, so the header points nowhere else. A scene URL the proxy would redirect
 * renders without the redirect in the same circumstances (the probe cannot
 * answer, or cannot name the scene's slug) and names itself the same way, in
 * the header and in the page.
 *
 * `next.config.canonical-link.test.ts` checks the rules against the pages and
 * the proxy, and records why every other dynamic page route is left out.
 */
export const SELF_CANONICAL_ROUTES = [
  selfCanonicalRoute('/artists/:slug'),
  selfCanonicalRoute('/releases/:slug'),
  selfCanonicalRoute('/venues/:slug'),
  selfCanonicalRoute('/labels/:slug'),
  selfCanonicalRoute('/festivals/:slug'),
  selfCanonicalRoute('/tags/:slug'),
  // `submit` is a static page beside the show route and `saved` a config
  // redirect; neither is a show.
  selfCanonicalRoute('/shows/:slug', {
    slug: `(?!(?:submit|saved)(?:/|$))${ENTITY_SLUG_SEGMENT}`,
  }),
  // The calendar's own segment grammar: a year in 2000-2100, a two-digit month
  // and day. Every request it admits is spelled the way the page writes its
  // canonical.
  selfCanonicalRoute('/shows/:year/:month', {
    year: '20\\d{2}|2100',
    month: '0[1-9]|1[0-2]',
  }),
  // `?days=` turns a day into a run: a valid one canonicalizes to the day root
  // and is noindexed, and a refused one is a not-found body at HTTP 200, which
  // the proxy cannot see because it does not read the query. An empty `days=`
  // names no run; Next's own router keeps the header for it and Vercel's
  // withholds it, and either leaves that URL consistent with its page.
  selfCanonicalRoute(
    '/shows/:year/:month/:day',
    { year: '20\\d{2}|2100', month: '0[1-9]|1[0-2]', day: '0[1-9]|[12]\\d|3[01]' },
    [{ type: 'query', key: 'days' }],
  ),
  // A year without a leading zero, the only spelling the archive writes.
  selfCanonicalRoute('/venues/:slug/shows/:year', { year: '[1-9]\\d{3}' }),
  // A show under the station that carries it; the proxy 404s it under any
  // other station segment.
  selfCanonicalRoute('/radio/:station/:show'),
  // `channel` there is the channel route, a station rather than a show.
  selfCanonicalRoute('/radio/:station/:show/:date', {
    show: `(?!channel(?:/|$))${ENTITY_SLUG_SEGMENT}`,
    date: '\\d{4}-\\d{2}-\\d{2}',
  }),
  // The proxy probes only a request without a session: one with a session gets
  // the page's own answer, a not-found body at HTTP 200 for a collection it may
  // not see.
  selfCanonicalRoute('/collections/:slug', {}, [{ type: 'cookie', key: 'auth_token' }]),
  // Every scene page names its request path. The proxy redirects a metro
  // member city, a case variant of the slug and a lower-case week key to the
  // scene's canonical URL, so the request path a scene page renders at is
  // canonical unless the proxy's probe could not answer.
  selfCanonicalRoute('/scenes/:slug', { slug: SCENE_SLUG_SEGMENT }),
  selfCanonicalRoute('/scenes/:slug/tonight', { slug: SCENE_SLUG_SEGMENT }),
  selfCanonicalRoute('/scenes/:slug/this-weekend', { slug: SCENE_SLUG_SEGMENT }),
  selfCanonicalRoute('/scenes/:slug/week', { slug: SCENE_SLUG_SEGMENT }),
  selfCanonicalRoute('/scenes/:slug/next-4-weeks', { slug: SCENE_SLUG_SEGMENT }),
  // An ISO week permalink or a day permalink, the two keys `[period]` serves.
  selfCanonicalRoute('/scenes/:slug/:period', {
    slug: SCENE_SLUG_SEGMENT,
    period: '\\d{4}-W\\d{2}|\\d{4}-\\d{2}-\\d{2}',
  }),
]

/**
 * Declares each self-canonical route's canonical in an HTTP `Link` header. On
 * these routes Next streams `<link rel="canonical">` after `</head>`
 * to Googlebot, and Google accepts that element only inside `<head>`; the
 * header carries the same URL. The render's own `Link` preload entries arrive
 * as a separate `Link` field line.
 */
const CANONICAL_LINK_HEADER_RULES = SELF_CANONICAL_ROUTES.map(({ source, path, missing }) => ({
  source,
  ...(missing.length > 0 ? { missing } : {}),
  headers: [{ key: 'Link', value: `<https://psychichomily.com${path}>; rel="canonical"` }],
}))

// `htmlLimitedBots` stays unset; next.config.canonical-link.test.ts says why.
const nextConfig: NextConfig = {
  // Cache Components is Next 16's successor to `experimental.ppr` and
  // the supported way to enable Partial Prerendering. The legacy
  // `experimental.ppr` config (boolean OR `'incremental'`) is a
  // hard-deprecated config error in Next 16; `cacheComponents` is the
  // only switch and enables PPR globally. Every route's static shell
  // prerenders; anything wrapped in `<Suspense>` (here, `<AuthHydrator>`
  // in the root layout) streams dynamically.
  //
  // ISR under cacheComponents — measured PSY-1641 (Next 16.1.4), do not
  // restate from intuition:
  //
  //   • PPR list pages that bind a window at build (`/artists`, `/shows`,
  //     `/venues` — route table `◐ … 1h 1y`) DO inherit that window from
  //     per-fetch `next: { revalidate: 3600 }` alone (prerender-manifest
  //     `initialRevalidateSeconds: 3600`). They are still `◐`, not `○`.
  //
  //   • Dynamic entity `[slug]` pages DO time-revalidate. Their
  //     `fallbackRevalidate: false` governs only the FALLBACK SHELL —
  //     one slug-independent file with no entity data, because these
  //     routes have no `generateStaticParams`. Entity data is in the
  //     postponed dynamic resume (`x-nextjs-postponed: 1`,
  //     `Cache-Control: no-store`), bounded by the per-fetch
  //     `next: { revalidate }` Data Cache. Measured PSY-1650 at RUNTIME:
  //     a mutation appeared 68s after a 60s window on `next start`.
  //     Production `x-vercel-cache: HIT` with a large Age is the shell's
  //     age, NOT stale content — the tell is many different slugs sharing
  //     one identical cache `Date`, which per-slug entries cannot do.
  //     PSY-1641 read that as staleness; PSY-1650 falsified it.
  //
  //   • `export const revalidate` is a build error on page routes under
  //     `cacheComponents` ("Route segment config revalidate is not
  //     compatible"). `"use cache"` + `cacheLife` is NOT a replacement
  //     for it here: measured PSY-1650, it does not move
  //     `fallbackRevalidate` in either shape (on the fetch helper, or on
  //     the page component), because the cached scope is never entered at
  //     build — `await params` hangs for a fallback route param. Metadata
  //     routes like `sitemap.ts` are a different compiler path; do not
  //     conflate (PSY-1621 / PSY-1644 / PSY-1652).
  //
  //   • A deploy does NOT refresh entity pages. The Data Cache survives a
  //     rebuild via `.next/cache/fetch-cache` (measured PSY-1650). Only
  //     the timer or an on-demand `revalidatePath` busts it (see
  //     `lib/proxy-revalidation.ts`, which works with no `cacheTag`).
  cacheComponents: true,
  experimental: {
    // Optimize barrel imports for common libraries
    // Only list packages that are actually installed
    optimizePackageImports: [
      'lucide-react',
      // The `radix-ui` meta-package re-exports every primitive from a
      // single barrel. components/ui/{select,popover,hover-card,switch}
      // import named primitives from it; without this entry the whole
      // barrel is pulled in. optimizePackageImports rewrites those named
      // imports to the underlying `@radix-ui/react-*` sub-paths so only
      // the used primitives are bundled (PSY-1101).
      'radix-ui',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-tabs',
      '@radix-ui/react-slot',
      '@radix-ui/react-label',
      '@tanstack/react-query',
    ],
  },
  // `next/image` allowlist for external image hosts (PSY-783).
  //
  // Policy: SPECIFIC hosts, not a wildcard `**`. Fails closed for
  // unexpected hosts — a stricter posture than the document-level
  // `img-src 'self' data: blob: https:` CSP, because the optimizer
  // proxies upstream bytes through `/_next/image` and we don't want
  // arbitrary third-party origins riding our CDN budget.
  //
  // Trade-off: requires a config update (and PR) when a new provider
  // or admin-supplied CDN host surfaces. Acceptable — same posture as
  // the connect-src/frame-src CSP allowlists above.
  //
  // Sourcing (radio surface, the first allowlist consumer):
  //   • `www.kexp.org` — confirmed via `radio_shows.image_url` query
  //     against the live dev DB (28/28 rows; path
  //     `/media/filer_public/<uuid>/<file>.jpg`).
  //   • `media.nts.live` — from NTS provider test fixtures
  //     (`radio_provider_nts_test.go` lines 27/33/50), the canonical
  //     CDN for `media.picture_large` / `media.background_large`.
  //   • `wfmu.org` / `www.wfmu.org` — WFMU HTML scraper does not
  //     extract per-show artwork today, but admins may attach show
  //     images / station logos pointing at the station's own host.
  //
  // Station logos (`radio_stations.logo_url`) are admin-entered; none
  // are seeded yet. Provider hosts double as the most likely logo
  // hosts; expand this list as new hosts surface in the 502 logs.
  images: {
    remotePatterns: [
      { protocol: 'https', hostname: 'www.kexp.org', pathname: '/**' },
      { protocol: 'https', hostname: 'media.nts.live', pathname: '/**' },
      { protocol: 'https', hostname: 'wfmu.org', pathname: '/**' },
      { protocol: 'https', hostname: 'www.wfmu.org', pathname: '/**' },
    ],
  },
  async redirects() {
    return [
      // Flat /sitemap.xml predated generateSitemaps() sharding (PSY-1622). The
      // index now lives at /sitemap-index; keep the old URL resolving so
      // crawlers that still have it land on the index rather than a 404.
      {
        source: '/sitemap.xml',
        destination: '/sitemap-index',
        permanent: true,
      },
      // Hugo shows used /shows/YYYY/MM/slug/, flatten to /shows/slug.
      //
      // The final segment must NOT be day-shaped. `/shows/{yyyy}/{mm}/{dd}` is
      // the day list route, and this rule is evaluated BEFORE the proxy and
      // before the router, so without the exclusion it claims every day URL and
      // 308s it to `/shows/{dd}`. The two rules are made disjoint here rather
      // than by reordering, because a redirect cannot decline a match.
      //
      // The exclusion is exactly the day shape, so this rule claims a final
      // segment if and only if the day route does not. A legacy slug that is
      // itself two digits in 01-31 therefore no longer flattens.
      // `next.config.shows-redirect.test.ts` asserts the partition.
      //
      // The lookahead ends on a SEGMENT boundary rather than a path end. The
      // compiled rule carries an optional trailing group of its own, so a bare
      // `$` inside the lookahead stops applying as soon as anything follows the
      // day, and the rule reclaims `/shows/2026/11/14/`.
      {
        source: '/shows/:year(\\d{4})/:month(\\d{2})/:slug((?!(?:0[1-9]|[12]\\d|3[01])(?:[/#?]|$))[^/]+)',
        destination: '/shows/:slug',
        permanent: true,
      },
      // "bands" taxonomy renamed to "artists"
      {
        source: '/bands/:slug',
        destination: '/artists/:slug',
        permanent: true,
      },
      // "mixes" section renamed to "dj-sets"
      {
        source: '/mixes/:slug',
        destination: '/dj-sets/:slug',
        permanent: true,
      },
      // Old about page
      {
        source: '/about',
        destination: '/',
        permanent: true,
      },
      // "crates" renamed back to "collections" (PSY-275)
      {
        source: '/crates',
        destination: '/collections',
        permanent: true,
      },
      {
        source: '/crates/:slug',
        destination: '/collections/:slug',
        permanent: true,
      },
      // "my-shows" and "following" consolidated into Library. The /my-shows page
      // is gone, which is exactly why this redirect must stay: stale bookmarks,
      // emails and search-engine entries still point at it.
      {
        source: '/my-shows',
        destination: '/library',
        permanent: false,
      },
      {
        source: '/following',
        destination: '/library?tab=artists',
        permanent: false,
      },
      // Saved shows live in Library. A config redirect rather than a page, so
      // the old URL answers 308 before anything streams; Next appends the
      // incoming query string to the destination.
      {
        source: '/shows/saved',
        destination: '/library',
        permanent: true,
      },
      // User "collection" page merged into Library (PSY-275)
      {
        source: '/collection',
        destination: '/library',
        permanent: false,
      },
      // Discovery lives at /graph. A config redirect rather than a page, so
      // /explore answers 308 before anything streams.
      {
        source: '/explore',
        destination: '/graph',
        permanent: true,
      },
      // The "manage" link of every artist and venue show-alert email. Temporary
      // because the target follows whichever surface owns alert preferences.
      // The destination is `ALERTS_HREF` (components/shared/followAlertChoices)
      // spelled out, since this file imports packages only;
      // next.config.redirect-only-pages.test.ts fails if the two drift.
      {
        source: '/settings/notifications',
        destination: '/profile?tab=settings#alerts',
        permanent: false,
      },
    ]
  },
  async rewrites() {
    return [
      // Next serves the touch icon only at /apple-icon.png (the app/apple-icon
      // file convention). Clients that skip the <link rel="apple-touch-icon">
      // tag and probe these conventional root paths instead get the same file.
      { source: '/apple-touch-icon.png', destination: '/apple-icon.png' },
      { source: '/apple-touch-icon-precomposed.png', destination: '/apple-icon.png' },
      // The IndexNow key file must sit at the site root to vouch for every
      // page. The pattern is the protocol's key format, and app/indexnow/[key]
      // answers only the configured key. Rewrites run after the filesystem
      // routes, so robots.txt and every other real root file still win.
      { source: '/:key([A-Za-z0-9-]{8,128}).txt', destination: '/indexnow/:key' },
    ]
  },
  async headers() {
    return [
      ...CANONICAL_LINK_HEADER_RULES,
      // The TopBar logo is `priority`-preloaded on every route, so it is worth
      // pinning (PSY-1771). Files under `public/` are served
      // `max-age=0, must-revalidate` by default, which is why the logo used to
      // be re-fetched on every cold load.
      //
      // This rule is what makes the logo cacheable at all: `/_next/image` does
      // NOT inherit it. Measured both ways: an `immutable` upstream still
      // comes back out of the optimizer as `max-age=0, must-revalidate`, and
      // prod serves the same for `/_next/image?url=%2Fog-image.jpg`. That is
      // why TopBar renders this one `unoptimized`, so the browser requests the
      // file below directly and actually gets these headers.
      //
      // Versioning the filename is what makes freezing it safe: a new logo
      // ships as `-v2` rather than mutating a URL clients were told to hold for
      // a year. The rule matches the whole `-vN` family on purpose: pinning
      // `-v1` literally would mean a future bump silently drops back to
      // `max-age=0` if whoever renames the file misses this line.
      {
        source: '/psychic-homily-logo-:version(v\\d+).png',
        headers: [
          { key: 'Cache-Control', value: 'public, max-age=31536000, immutable' },
        ],
      },
      {
        source: '/(.*)',
        headers: [
          // Prevent clickjacking — page cannot be embedded in frames
          { key: 'X-Frame-Options', value: 'DENY' },
          // Prevent MIME type sniffing — browser must respect Content-Type
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          // Limit referrer info sent to other origins
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          // Disable browser features the app doesn't use
          { key: 'Permissions-Policy', value: 'geolocation=(), microphone=(), camera=(), payment=(), usb=()' },
          // Prevent Adobe cross-domain policy requests
          { key: 'X-Permitted-Cross-Domain-Policies', value: 'none' },
          // CSP: Next.js requires 'unsafe-inline' for scripts without nonce middleware.
          // Still provides value via frame-ancestors, base-uri, form-action, and connect-src restrictions.
          {
            key: 'Content-Security-Policy',
            value: [
              "default-src 'self'",
              "script-src 'self' 'unsafe-inline' https://vercel.live https://va.vercel-scripts.com https://us-assets.i.posthog.com",
              "style-src 'self' 'unsafe-inline'",
              // img-src permits any HTTPS source so release cover art, station
              // logos, venue photos, and other user-contributed URLs render
              // without rebuilding an allowlist each time we add a source.
              // Widened from a specific domain list in PSY-333. Trade-offs:
              //   • Any HTTPS host can serve images, so a compromised CDN
              //     could surface unexpected content — acceptable given we
              //     link out to community-editable URLs anyway.
              //   • Referrer leakage to third-party CDNs is mitigated at the
              //     document level by Referrer-Policy:
              //     strict-origin-when-cross-origin (set above), which only
              //     sends the origin — not the specific page — cross-origin.
              //   • A per-image referrerpolicy="no-referrer" sweep would
              //     tighten this further but is a code-wide change; revisit
              //     if a specific leakage surface warrants it.
              "img-src 'self' data: blob: https:",
              "font-src 'self'",
              "worker-src 'self' blob:",
              // gibs.earthdata.nasa.gov: the Atlas globe's night-earth raster
              // tiles (NASA GIBS Black Marble). MapLibre fetches tiles via
              // fetch(), NOT <img>, so the tile host belongs in connect-src —
              // the img-src https: wildcard above does not cover it
              // (verified in the MapLibre spike).
              // tiles.openfreemap.org: the Atlas street basemap (PSY-1543) —
              // TileJSON, vector tiles, and glyph PBFs all come from this one
              // host, and all via fetch() (glyphs too), so connect-src again.
              "connect-src 'self' https://api.psychichomily.com https://stage.api.psychichomily.com https://app.posthog.com https://us.i.posthog.com https://us-assets.i.posthog.com https://gibs.earthdata.nasa.gov https://tiles.openfreemap.org",
              "frame-src https://open.spotify.com https://bandcamp.com https://w.soundcloud.com https://vercel.live https://maps.google.com https://www.google.com",
              "frame-ancestors 'none'",
              "base-uri 'self'",
              "form-action 'self'",
            ].join('; '),
          },
        ],
      },
    ];
  },
};

const sentryConfig = withSentryConfig(nextConfig, {
  // Sentry organization and project slugs
  org: process.env.SENTRY_ORG,
  project: process.env.SENTRY_PROJECT,

  // Auth token for source map uploads (set in CI/CD or .env.sentry-build-plugin)
  authToken: process.env.SENTRY_AUTH_TOKEN,

  // Suppress logs except in CI
  silent: !process.env.CI,

  // Route to tunnel Sentry events through your server (bypasses ad blockers)
  tunnelRoute: "/monitoring",

  // Source map configuration
  sourcemaps: {
    // Delete source maps after upload (don't expose in production)
    deleteSourcemapsAfterUpload: true,
  },

  // Disable Sentry telemetry
  telemetry: false,
});

const resolvedConfig = process.env.ANALYZE === 'true'
  ? withBundleAnalyzer({ enabled: true })(sentryConfig)
  : sentryConfig;

// `next dev`, `next build` and `next start` all evaluate this module; the phase
// argument is what tells a build apart, so the guard runs here, before Next
// compiles or prerenders anything.
export default function nextConfigForPhase(phase: string): NextConfig {
  assertBuildApiOriginConfigured(phase, process.env);
  return resolvedConfig;
}
