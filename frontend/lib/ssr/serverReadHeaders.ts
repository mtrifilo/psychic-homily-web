/**
 * Request headers a SERVER-SIDE read of the backend carries: the shared
 * server-read secret, which lets a GET or HEAD from a server render, a proxy
 * probe, a metadata or sitemap fetch skip the backend's anonymous per-IP
 * public-read limiter (`ServerReadExemption` in
 * backend/internal/api/middleware/server_read.go). It grants no identity and
 * nothing beyond that limiter.
 *
 * The secret lives in `SERVER_READ_SECRET`, read from the server's environment
 * at call time. It is deliberately NOT a `NEXT_PUBLIC_` variable: Next inlines
 * only `NEXT_PUBLIC_` values into a client bundle, so in a browser the variable
 * reads as undefined and this returns no header at all.
 *
 * Never spread this into a request whose headers come from a visitor (the dev
 * `/api` relay in `app/api/[...path]/route.ts`): that would hand the exemption
 * to whoever sends the request.
 *
 * Unset, blank, or a value that is not a valid header value sends nothing, and
 * the request is metered as anonymous. An invalid value is dropped rather than
 * sent because `fetch` throws on one, which would fail the read itself instead
 * of only losing the exemption.
 *
 * The header joins the Data Cache key (Next keys an entry on URL, method,
 * headers and body), so setting or rotating the secret starts every cached
 * fetch cold once.
 */

/** Must match `ServerReadSecretHeader` in the backend. */
export const SERVER_READ_SECRET_HEADER = 'X-Server-Read-Secret'

/** Visible ASCII only: what a header value may carry without `fetch` throwing. */
const HEADER_VALUE_SAFE = /^[\x21-\x7e]+$/

export function serverReadHeaders(): Record<string, string> {
  // Must match `ServerReadSecretEnvVar` in the backend. Read by its literal
  // name: a computed `process.env` lookup is not inlined on every runtime.
  const secret = process.env.SERVER_READ_SECRET?.trim()
  if (!secret || !HEADER_VALUE_SAFE.test(secret)) return {}
  return { [SERVER_READ_SECRET_HEADER]: secret }
}
