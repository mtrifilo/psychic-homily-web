/**
 * PSY-1645 — single source of truth for where the E2E harness's backend lives.
 *
 * Everything in the harness that needs to reach the backend directly (global
 * setup's spawn + health check, the fixture-reset helper, the OAuth spec's
 * callback URL) resolves it from here. Before this module those were three
 * independent `http://localhost:8080` literals, and any run on a different port
 * left them disagreeing: the browser authenticated against one backend while
 * the reset helper talked to whatever else happened to own :8080. The reset
 * then failed with a 401, was swallowed, and five unrelated-looking `@smoke`
 * specs failed instead (measured in PSY-1592).
 *
 * `BACKEND_URL` is not a new variable — it is the one the Next.js proxy already
 * reads (`app/api/[...path]/route.ts`) and the one `scripts/dispatch/stack-up.sh`
 * writes into `<worktree>/dispatch-stack/.env`. Reusing it is what keeps the
 * harness and the frontend pointed at the SAME backend by construction: there
 * is no combination of env vars that aims them at different ones.
 */

const DEFAULT_BACKEND_URL = 'http://localhost:8080'

/**
 * Loopback only. `resetTestFixtures` sends an admin session cookie to this
 * origin and asks it to DELETE rows — so a misaimed value is destructive, not
 * merely wrong. `BACKEND_URL` legitimately holds a remote origin in deployed
 * environments (the proxy uses it that way), which makes "a shell that happens
 * to have deploy env loaded" a realistic path to pointing a truncating request
 * at a real database. The backend independently refuses the endpoint outside
 * {test,ci,development}, but that is the far end saying no; this is the near
 * end not asking. Both halves are cheap.
 */
const LOOPBACK_HOSTNAMES = new Set(['localhost', '127.0.0.1', '::1'])

function resolveBackendUrl(): URL {
  const raw = process.env.BACKEND_URL ?? DEFAULT_BACKEND_URL

  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    throw new Error(
      `[PSY-1645] BACKEND_URL is not a valid absolute URL: ${JSON.stringify(raw)}. ` +
        `Expected something like "http://localhost:8099".`,
    )
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error(
      `[PSY-1645] BACKEND_URL must be http(s), got ${parsed.protocol} in ${raw}`,
    )
  }

  // `URL` keeps IPv6 hosts bracketed in `hostname`; strip so '::1' matches.
  const hostname = parsed.hostname.replace(/^\[|\]$/g, '')
  if (!LOOPBACK_HOSTNAMES.has(hostname)) {
    throw new Error(
      `[PSY-1645] refusing to run the E2E harness against non-loopback host ` +
        `${JSON.stringify(hostname)} (BACKEND_URL=${raw}). The fixture-reset ` +
        `helper DELETES rows for the seeded users, so it may only target a ` +
        `local test backend. Unset BACKEND_URL to use ${DEFAULT_BACKEND_URL}, ` +
        `or point it at a local stack (see scripts/dispatch/stack-up.sh).`,
    )
  }

  return parsed
}

const resolved = resolveBackendUrl()

/**
 * Origin of the backend this run targets, with no trailing slash — safe to
 * concatenate (`${BACKEND_BASE_URL}/auth/profile`) and to hand to Playwright's
 * `baseURL`.
 */
export const BACKEND_BASE_URL = resolved.origin

/**
 * Port the backend listens on. Global setup uses it for the "is the port free"
 * pre-check and to build the `API_ADDR` it hands the spawned Go server, so a
 * custom `BACKEND_URL` moves the backend rather than leaving it on :8080 with
 * the rest of the harness looking elsewhere.
 */
export const BACKEND_PORT = Number(
  resolved.port || (resolved.protocol === 'https:' ? 443 : 80),
)
