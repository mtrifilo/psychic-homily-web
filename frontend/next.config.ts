import type { NextConfig } from "next";
import { withSentryConfig } from "@sentry/nextjs";
import withBundleAnalyzer from "@next/bundle-analyzer";

const nextConfig: NextConfig = {
  experimental: {
    // Optimize barrel imports for common libraries
    // Only list packages that are actually installed
    optimizePackageImports: [
      'lucide-react',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-tabs',
      '@radix-ui/react-slot',
      '@radix-ui/react-label',
      '@tanstack/react-query',
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
              "script-src 'self' 'unsafe-inline' https://vercel.live https://us-assets.i.posthog.com",
              "style-src 'self' 'unsafe-inline'",
              "img-src 'self' data: blob: https://vercel.com https://vercel.live",
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
