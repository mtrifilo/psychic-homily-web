/**
 * API Base URL Configuration
 *
 * Extracted to its own module to break circular imports between
 * lib/api.ts and feature module api.ts files.
 *
 * Feature modules (features/artists/api.ts, etc.) import API_BASE_URL from here.
 * lib/api.ts re-exports it for backward compatibility.
 *
 * TWO bases live here, because the frontend reaches the backend in two
 * incompatible ways and one value cannot serve both:
 *
 *   API_BASE_URL       The DATA path (XHR/fetch). In the browser during
 *                      development this is the same-origin `/api` proxy,
 *                      because `auth_token` is SameSite=Lax and a
 *                      cross-origin request would not carry it.
 *
 *   OAUTH_BACKEND_URL  The OAUTH path. The Google button performs a full-page
 *                      navigation to the backend's `/auth/login/google`; that
 *                      redirect chain does not survive the Next proxy, so it
 *                      needs a DIRECT backend origin, never `/api`.
 *
 * PSY-1649: those two requirements were both being read off
 * `NEXT_PUBLIC_API_URL`. On a stack whose backend is not on :8080 — which is
 * what `scripts/dispatch/stack-up.sh --mode=isolated` allocates by design —
 * no single value of that variable satisfies both: pointing it at the proxy
 * (`http://localhost:<fe>/api`) fixes the data specs and breaks OAuth, and
 * pointing it at the backend origin does the reverse (measured in PSY-1645).
 * `NEXT_PUBLIC_OAUTH_BACKEND_URL` is the escape hatch, and it falls back to
 * `NEXT_PUBLIC_API_URL` so deployed environments that already set only the
 * latter are unaffected.
 *
 * A third face of the same overload lived in the SSR branch below: it
 * hardcoded `:8080` rather than following `BACKEND_URL`, so server rendering
 * silently missed a relocated backend. See getApiBaseUrl.
 *
 * Which variable does what, in development:
 *
 *   BACKEND_URL                    Where the backend actually listens. Read by
 *                                  the /api proxy and, now, by SSR.
 *   NEXT_PUBLIC_API_URL            Overrides the data base outright. Unset is
 *                                  the norm locally.
 *   NEXT_PUBLIC_OAUTH_BACKEND_URL  Overrides the OAuth base only.
 */

/**
 * Where a backend lives when nothing is configured. Only ever reachable in a
 * non-production build (see getOAuthBackendUrl) — before PSY-1649 the OAuth
 * readers each carried their own `|| 'http://localhost:8080'`, which was a
 * localhost literal compiled into the production bundle and inert only
 * because `NEXT_PUBLIC_API_URL` happens to always be set on Vercel. That
 * invariant is now enforced by construction rather than assumed.
 */
const DEVELOPMENT_BACKEND_ORIGIN = 'http://localhost:8080'
const PRODUCTION_BACKEND_ORIGIN = 'https://api.psychichomily.com'

/**
 * A backend origin has to be an absolute http(s) URL: `new URL()` throws on a
 * relative one, and a non-http scheme in a `window.location.href` assignment
 * is a navigation we never want to make. Every value checked here comes from
 * build/deploy configuration, not from a request, so this is a
 * misconfiguration check rather than a defence against untrusted input — but
 * the OAuth entry point is the wrong place to discover a typo at runtime.
 */
const isAbsoluteHttpUrl = (value: string): boolean => {
  try {
    const { protocol } = new URL(value)
    return protocol === 'http:' || protocol === 'https:'
  } catch {
    return false
  }
}

// Get the API base URL
const getApiBaseUrl = (): string => {
  // Check for environment-specific API URL first
  if (process.env.NEXT_PUBLIC_API_URL) {
    return process.env.NEXT_PUBLIC_API_URL
  }

  // In browser during development, use Next.js API proxy
  // This handles same-origin cookie requirements
  if (typeof window !== 'undefined' && process.env.NODE_ENV === 'development') {
    return '/api'
  }

  // Server-side in development. The browser returned above, so this branch
  // only ever runs on the server — which means BACKEND_URL is readable here.
  // It is not a NEXT_PUBLIC_ var, so Next never inlines it into the client
  // bundle; the `typeof window` guard states that rather than relying on it.
  //
  // PSY-1649: following BACKEND_URL is what makes a backend on a non-default
  // port work end to end. Browser traffic goes through the same-origin /api
  // proxy, which already forwards to BACKEND_URL — but SSR bypasses the proxy
  // and used to hardcode :8080, so with the backend on :8099 every
  // server-rendered fetch hit a dead port. That surfaced as pages rendering
  // logged-out or empty, and it failed the authenticated E2E specs on any
  // non-default port while the same specs passed on :8080 (measured here).
  if (process.env.NODE_ENV === 'development') {
    if (typeof window === 'undefined') {
      const backendUrl = process.env.BACKEND_URL
      if (backendUrl && isAbsoluteHttpUrl(backendUrl)) {
        return backendUrl
      }
    }
    return DEVELOPMENT_BACKEND_ORIGIN
  }

  // Production fallback
  return PRODUCTION_BACKEND_ORIGIN
}

/**
 * A backend origin mounted under a path (`http://localhost:3001/api`) is the
 * signature of a proxy mount, and proxying is precisely what the OAuth
 * redirect cannot tolerate. Warn instead of throwing: a bad value here should
 * be loud during development, not a blank auth page.
 */
const warnIfOAuthBaseLooksProxied = (base: string, source: string): void => {
  if (process.env.NODE_ENV === 'production') return
  let pathname: string
  try {
    pathname = new URL(base).pathname
  } catch {
    return
  }
  if (pathname === '/' || pathname === '') return
  console.warn(
    `[api-base] OAuth requests will go to ${base}, resolved from ${source}, ` +
      `which is mounted under a path and so is probably the Next.js /api ` +
      `proxy. OAuth is a full-page redirect and does not survive the proxy. ` +
      `Set NEXT_PUBLIC_OAUTH_BACKEND_URL to the backend's own origin ` +
      `(e.g. http://localhost:8080).`,
  )
}

/**
 * Direct backend origin for OAuth full-page redirects. Never the `/api` proxy.
 *
 * Resolution order:
 *   1. NEXT_PUBLIC_OAUTH_BACKEND_URL — the dedicated OAuth origin.
 *   2. NEXT_PUBLIC_API_URL — so an environment that sets only the data base
 *      (every deployed environment today) keeps its current behaviour.
 *   3. localhost in non-production builds; the production API origin
 *      otherwise, so a production bundle can never carry a localhost auth
 *      endpoint.
 */
const getOAuthBackendUrl = (): string => {
  const oauthOverride = process.env.NEXT_PUBLIC_OAUTH_BACKEND_URL
  if (oauthOverride) {
    if (isAbsoluteHttpUrl(oauthOverride)) {
      warnIfOAuthBaseLooksProxied(
        oauthOverride,
        'NEXT_PUBLIC_OAUTH_BACKEND_URL',
      )
      return oauthOverride
    }
    if (process.env.NODE_ENV !== 'production') {
      console.warn(
        `[api-base] Ignoring NEXT_PUBLIC_OAUTH_BACKEND_URL=` +
          `${JSON.stringify(oauthOverride)}: it must be an absolute http(s) ` +
          `origin (e.g. http://localhost:8080).`,
      )
    }
  }

  const dataBase = process.env.NEXT_PUBLIC_API_URL
  if (dataBase && isAbsoluteHttpUrl(dataBase)) {
    warnIfOAuthBaseLooksProxied(dataBase, 'NEXT_PUBLIC_API_URL')
    return dataBase
  }

  return process.env.NODE_ENV === 'production'
    ? PRODUCTION_BACKEND_ORIGIN
    : DEVELOPMENT_BACKEND_ORIGIN
}

// Export the configured API base URL
export const API_BASE_URL = getApiBaseUrl()

/**
 * Direct backend origin for OAuth full-page redirects — see
 * getOAuthBackendUrl. Import this, not API_BASE_URL, anywhere the browser is
 * about to leave the app for the backend's `/auth/login/...` endpoints.
 */
export const OAUTH_BACKEND_URL = getOAuthBackendUrl()
