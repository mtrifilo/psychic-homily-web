import type { NextConfig } from "next";
import { withSentryConfig } from "@sentry/nextjs";

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
};

export default withSentryConfig(nextConfig, {
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
