/**
 * PSY-1645 — the single place the E2E suite decides which backend it is
 * talking to.
 *
 * Most of the suite reaches the backend through the frontend's own `/api`
 * proxy and never needs this. A few places have to address the backend
 * directly — the test-fixtures reset helper (the proxy forwards only
 * content-type and the auth cookie, so a custom `X-Test-Fixtures` header would
 * be dropped) and the OAuth callback leg (the browser must land on the
 * backend's real redirect endpoint). Those used to each hardcode
 * `http://localhost:8080`.
 *
 * A hardcoded origin does not fail when it is wrong; it silently addresses
 * whatever else owns :8080 — another session's stack, or nothing. That
 * converted one configuration error into five unrelated-looking spec failures.
 *
 * `BACKEND_URL` is the variable `scripts/dispatch/stack-up.sh` writes for an
 * isolated stack and the one the Next proxy reads, so honouring it points the
 * direct-to-backend calls at the same stack the rest of an externally-managed
 * run uses. The default is unchanged, so `bun run test:e2e` behaves exactly as
 * before.
 *
 * Two things this variable is NOT, both worth knowing before reaching for it:
 *
 * - It is only meaningful for a run whose backend is managed OUTSIDE the
 *   Playwright harness. `global-setup.ts` provisions its own backend and
 *   refuses to start if `BACKEND_URL` disagrees with it — see the guard there.
 *   The supported way to use this variable is therefore
 *   `bun run test:e2e:external` (`e2e/playwright.external.config.ts`), which
 *   runs the same suite with no globalSetup. Without that config this variable
 *   would have no reachable caller at all.
 * - The Playwright process does not read `frontend/.env.local`; Next does. So
 *   setting `BACKEND_URL` there moves the proxy but not these calls. Set it in
 *   the environment of the `playwright test` command.
 */

const DEFAULT_BACKEND_URL = 'http://localhost:8080'

/**
 * Parse, constrain and normalize. This value addresses a credential-bearing,
 * row-deleting endpoint, so it is a trust boundary: validate it here rather
 * than letting a malformed or surprising value surface later as an opaque
 * request error in an unrelated spec — the exact diagnosability gap this file
 * exists to close.
 */
function resolveBackendOrigin(value: string): string {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    throw new Error(
      `BACKEND_URL is not a valid absolute URL: ${JSON.stringify(value)}. ` +
        `Expected an origin like ${DEFAULT_BACKEND_URL}.`,
    )
  }

  // Not a stylistic preference — a hard requirement of how the suite
  // authenticates. Every stored auth cookie is written with `domain=localhost`
  // (global-setup.ts captures them from a browser session on
  // http://localhost:3000), and cookie domains match by host, ignoring port.
  // Against any other host — `127.0.0.1` included — Playwright attaches no
  // cookie at all and the request goes out anonymous, which surfaces as a
  // baffling 401 rather than as the configuration error it is.
  if (parsed.hostname !== 'localhost') {
    throw new Error(
      `BACKEND_URL must be a localhost origin, got ${JSON.stringify(value)}. ` +
        `The E2E auth cookies are stored with domain=localhost and are not ` +
        `sent to any other host (127.0.0.1 included), so a non-localhost ` +
        `backend would receive unauthenticated requests.`,
    )
  }

  // Playwright resolves a root-absolute path against `baseURL` with `new URL`,
  // which DISCARDS a path prefix — unlike the Next proxy, which concatenates.
  // `http://localhost:8080/api` would therefore silently send the admin-authed
  // reset to `/admin/test-fixtures/reset`, outside the intended mount. Refuse
  // rather than quietly diverge.
  if (parsed.pathname !== '/' || parsed.search || parsed.hash) {
    throw new Error(
      `BACKEND_URL must be a bare origin with no path, query or fragment, ` +
        `got ${JSON.stringify(value)}. Playwright resolves request paths ` +
        `against it with new URL(), which would drop the extra parts.`,
    )
  }

  // `.origin` also strips any userinfo, so a credential embedded in the URL
  // cannot reach the error messages below (and from there a CI log artifact).
  return parsed.origin
}

export const E2E_BACKEND_URL = resolveBackendOrigin(
  process.env.BACKEND_URL || DEFAULT_BACKEND_URL,
)

/**
 * How to describe the backend target in an error message: the origin plus
 * where it came from. When a direct-to-backend call fails, which backend it
 * addressed is the fact that diagnoses it — and the fact that was missing.
 */
export const E2E_BACKEND_TARGET = process.env.BACKEND_URL
  ? `${E2E_BACKEND_URL} (from BACKEND_URL)`
  : `${E2E_BACKEND_URL} (default; set BACKEND_URL to target another backend)`
