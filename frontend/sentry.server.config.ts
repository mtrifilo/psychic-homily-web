import * as Sentry from '@sentry/nextjs'

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,

  // Environment for filtering in Sentry dashboard
  environment: process.env.NODE_ENV,

  // Adjust tracing sample rate for production
  // Disable tracing in development to reduce noise
  tracesSampleRate: process.env.NODE_ENV === 'production' ? 0.1 : 0,

  // Debug mode off - too verbose
  debug: false,

  beforeSend: scrubSecretHeaders,
})

/**
 * Strip request headers that carry credentials before an event leaves us.
 *
 * Sentry's requestDataIntegration is on by default and attaches request
 * headers to server events. Its built-in redaction covers `cookie` and the
 * IP headers only, so a custom credential header would be shipped verbatim
 * and then retained, readable by anyone with project access.
 *
 * `x-internal-secret` is the shared INTERNAL_API_SECRET used by
 * /api/internal/revalidate (PSY-1691) and by the Go backend's admin bypass.
 * It rides on requests that report to Sentry on ordinary paths, not just on
 * errors, so this scrub is load-bearing rather than defence in depth.
 * Header names are lowercased on the wire but compared case-insensitively
 * here so a differently-cased key cannot slip past.
 */
function scrubSecretHeaders<T extends { request?: { headers?: Record<string, string> } }>(
  event: T
): T {
  const headers = event.request?.headers
  if (!headers) return event

  for (const name of Object.keys(headers)) {
    if (SECRET_HEADERS.has(name.toLowerCase())) {
      headers[name] = '[Filtered]'
    }
  }
  return event
}

const SECRET_HEADERS = new Set(['x-internal-secret', 'authorization'])
