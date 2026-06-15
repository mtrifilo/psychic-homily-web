import type { NextConfig } from "next";
import { withSentryConfig } from "@sentry/nextjs";
import withBundleAnalyzer from "@next/bundle-analyzer";

const nextConfig: NextConfig = {
  // Cache Components is Next 16's successor to `experimental.ppr` and
  // the supported way to enable Partial Prerendering. The legacy
  // `experimental.ppr` config (boolean OR `'incremental'`) is a
  // hard-deprecated config error in Next 16; `cacheComponents` is the
  // only switch and enables PPR globally. Every route's static shell
  // prerenders; anything wrapped in `<Suspense>` (here, `<AuthHydrator>`
  // in the root layout) streams dynamically. Per-route ISR
  // (`next: { revalidate: 3600 }` on the hydrated entity routes) is
  // preserved because the route body — the part with the `fetch` cache
  // hint — stays in the static shell.
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
      // Hugo shows used /shows/YYYY/MM/slug/ — flatten to /shows/slug
      {
        source: '/shows/:year(\\d{4})/:month(\\d{2})/:slug',
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
      // "my-shows" and "following" consolidated into Library
      {
        source: '/my-shows',
        destination: '/library',
        permanent: false,
      },
      {
        source: '/following',
        destination: '/library',
        permanent: false,
      },
      // User "collection" page merged into Library (PSY-275)
      {
        source: '/collection',
        destination: '/library',
        permanent: false,
      },
    ]
  },
  async headers() {
    return [
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
              "connect-src 'self' https://api.psychichomily.com https://stage.api.psychichomily.com https://app.posthog.com https://us.i.posthog.com https://us-assets.i.posthog.com",
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

export default process.env.ANALYZE === 'true'
  ? withBundleAnalyzer({ enabled: true })(sentryConfig)
  : sentryConfig;
