/**
 * PSY-1645 — the single place the E2E suite decides which backend it is
 * talking to.
 *
 * Most of the suite reaches the backend through the frontend's own `/api`
 * proxy and never needs this. A few places have to address the backend
 * directly — the test-fixtures reset helper (the proxy strips the
 * `X-Test-Fixtures` header) and the OAuth callback leg (the browser must land
 * on the backend's real redirect endpoint). Those used to each hardcode
 * `http://localhost:8080`.
 *
 * A hardcoded origin does not fail when it is wrong; it silently addresses
 * whatever else owns :8080 — another session's stack, or nothing. That
 * converted one configuration error into five unrelated-looking spec failures.
 *
 * `BACKEND_URL` is the same variable the Next proxy reads
 * (app/api/[...path]/route.ts) and the same one
 * scripts/dispatch/stack-up.sh writes for an isolated stack, so honouring it
 * keeps the direct-to-backend calls on the backend the rest of the run is
 * already using. The default is unchanged, so `bun run test:e2e` — where
 * global-setup.ts owns and pins :8080 — behaves exactly as before.
 */
export const E2E_BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

/**
 * How to describe the backend target in an error message: the origin plus
 * where it came from. When a direct-to-backend call fails, which backend it
 * addressed is the fact that diagnoses it — and the fact that was missing.
 */
export const E2E_BACKEND_TARGET = process.env.BACKEND_URL
  ? `${E2E_BACKEND_URL} (from BACKEND_URL)`
  : `${E2E_BACKEND_URL} (default; set BACKEND_URL to target another backend)`
