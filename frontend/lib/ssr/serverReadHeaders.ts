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
 * Unset or blank sends nothing, and the request is metered as anonymous. So
 * does a value the backend refuses at start: shorter than 32 characters, or
 * holding a character outside visible ASCII or a comma (a comma separates the
 * two secrets of a rotation there). Sending one could never match. That case
 * also logs one error per process, because each side would otherwise look
 * configured while no read is exempt.
 *
 * The header joins the Data Cache key (Next keys an entry on URL, method,
 * headers and body), so setting or rotating the secret starts every cached
 * fetch cold once.
 */

/** Must match `ServerReadSecretHeader` in the backend. */
export const SERVER_READ_SECRET_HEADER = 'X-Server-Read-Secret'

/**
 * What the backend accepts as one secret: at least 32 characters of visible
 * ASCII, no comma. Must match `minServerReadSecretLength` and
 * `isVisibleASCII` in the backend.
 */
const USABLE_SECRET = /^[\x21-\x2b\x2d-\x7e]{32,}$/

let reportedUnusableSecret = false

export function serverReadHeaders(): Record<string, string> {
  // Must match `ServerReadSecretEnvVar` in the backend. Read by its literal
  // name: a computed `process.env` lookup is not inlined on every runtime.
  const secret = process.env.SERVER_READ_SECRET?.trim()
  if (!secret) return {}
  if (!USABLE_SECRET.test(secret)) {
    if (!reportedUnusableSecret) {
      reportedUnusableSecret = true
      console.error(
        'SERVER_READ_SECRET is a value the backend refuses (under 32 characters, outside visible ASCII, or with a comma); server reads are sent without it'
      )
    }
    return {}
  }
  return { [SERVER_READ_SECRET_HEADER]: secret }
}
