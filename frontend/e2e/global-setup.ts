import { execSync, spawn, type ChildProcess } from 'child_process'
import {
  chromium,
  type Browser,
  type FullConfig,
  type Page,
} from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import * as net from 'net'
import * as http from 'http'
import pLimit from 'p-limit'
import { BACKEND_BASE_URL, BACKEND_PORT } from './backend-url'
// PSY-1663: shared with the seed-restore helper the admin specs use, so the
// database this seeds and the one they restore rows in cannot drift apart.
import { E2E_DATABASE_URL as E2E_DB_URL } from './e2e-db'

const BACKEND_DIR = path.resolve(__dirname, '../../backend')
const PID_FILE = path.resolve(__dirname, '.backend-pid')
const AUTH_DIR = path.resolve(__dirname, '.auth')

// Shared password for all seeded test users (see setup-db.sh).
const TEST_PASSWORD = 'e2e-test-password-123'

// PSY-431: seed N regular users so each Playwright worker gets its own
// authenticated user (worker index → user N), avoiding cross-worker races
// on shared user state (saved_shows, favorite_venues, submissions, etc.).
// Must match the seeded user count in setup-db.sh. It is a ceiling on the
// worker count, not a target: the local cap in playwright.config.ts is 2 for
// dev-server-contention reasons, and the fixture maps workerIndex % USER_COUNT,
// so seeding more users than workers is harmless. It also sets how much work
// captureAuthState has to push through AUTH_CAPTURE_CONCURRENCY below —
// raising it adds batches, and so adds setup wall time. Check that constant
// too.
const USER_COUNT = 5

function userEmailForWorker(workerIndex: number): string {
  return workerIndex === 0
    ? 'e2e-user@test.local'
    : `e2e-user-${workerIndex}@test.local`
}

function userAuthFileForWorker(workerIndex: number): string {
  return workerIndex === 0 ? 'user.json' : `user-${workerIndex}.json`
}

// Exported so the fixture can reuse the same mapping.
export { USER_COUNT, userEmailForWorker, userAuthFileForWorker }

const TEST_ADMIN = {
  email: 'e2e-admin@test.local',
  password: TEST_PASSWORD,
}

function log(msg: string) {
  console.log(`[e2e-setup] ${msg}`)
}

function isPortInUse(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const server = net.createServer()
    server.once('error', () => resolve(true))
    server.once('listening', () => {
      server.close()
      resolve(false)
    })
    server.listen(port, '127.0.0.1')
  })
}

function waitForUrl(
  url: string,
  timeoutMs: number = 30_000
): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now()
    const check = () => {
      http
        .get(url, (res) => {
          if (res.statusCode === 200) {
            resolve()
          } else if (Date.now() - start > timeoutMs) {
            reject(new Error(`Timeout waiting for ${url} (last status: ${res.statusCode})`))
          } else {
            setTimeout(check, 500)
          }
        })
        .on('error', () => {
          if (Date.now() - start > timeoutMs) {
            reject(new Error(`Timeout waiting for ${url}`))
          } else {
            setTimeout(check, 500)
          }
        })
    }
    check()
  })
}

async function startDatabase() {
  log('Starting ephemeral PostgreSQL on port 5433...')
  // Don't use --wait: the migrate container is a one-shot that exits with 0,
  // which docker compose --wait treats as failure.
  //
  // PSY-1006: `up -d` pulls the postgres + migrate images from Docker Hub. A
  // transient registry timeout ("context deadline exceeded" against
  // registry-1.docker.io) was failing the E2E Smoke gate before any test ran.
  // Retry the bring-up a few times with backoff so a flaky registry pull can't
  // fail the whole run. The happy path is unchanged (first attempt succeeds).
  const upCmd = 'docker compose -p e2e -f docker-compose.e2e.yml up -d'
  const MAX_ATTEMPTS = 3
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      execSync(upCmd, { cwd: BACKEND_DIR, stdio: 'inherit' })
      break
    } catch (err) {
      if (attempt === MAX_ATTEMPTS) {
        throw new Error(
          `docker compose up failed after ${MAX_ATTEMPTS} attempts ` +
            `(likely a Docker Hub registry timeout): ${(err as Error).message}`
        )
      }
      log(
        `docker compose up failed (attempt ${attempt}/${MAX_ATTEMPTS}); ` +
          `retrying in ${attempt * 5}s...`
      )
      await new Promise((r) => setTimeout(r, attempt * 5000))
    }
  }
  // Wait for the DB to be healthy by READING the healthcheck docker already
  // runs, rather than re-implementing the probe here. There is exactly one
  // definition of "the E2E database is ready" and it lives in
  // docker-compose.e2e.yml; a second copy in TypeScript would drift from it
  // (and a naive `pg_isready` copy would reintroduce the initdb temp-server
  // race the compose healthcheck exists to close).
  //
  // Strictly this gate is redundant — migrate is `depends_on: db:
  // service_healthy`, so setup-db.sh waiting for migrate already implies a
  // healthy db, which is why scripts/dispatch/stack-up.sh has no equivalent
  // loop. It is kept because it fails with a DB-specific message before the
  // seed script starts, instead of surfacing as a migrate timeout.
  //
  // Wall-clock deadline, not an iteration count: each poll costs a docker
  // round trip, so counting iterations would silently overshoot the budget it
  // claims under exactly the host load this guards against.
  const DB_READY_TIMEOUT_MS = 120_000
  log('Waiting for PostgreSQL to be ready...')
  const containerId = execSync(
    'docker compose -p e2e -f docker-compose.e2e.yml ps -q db',
    { cwd: BACKEND_DIR, encoding: 'utf8' }
  ).trim()
  if (!containerId) {
    throw new Error('E2E db container is not running after docker compose up')
  }
  const deadline = Date.now() + DB_READY_TIMEOUT_MS
  let health = 'unknown'
  while (Date.now() < deadline) {
    try {
      health = execSync(
        `docker inspect -f '{{.State.Health.Status}}' ${containerId}`,
        { encoding: 'utf8', stdio: 'pipe' }
      ).trim()
    } catch (err) {
      // A loaded Docker daemon intermittently answers "Error response from
      // daemon" for a container that is in fact fine. Treat a failed query as
      // "not healthy yet" and let the deadline decide, rather than aborting
      // the whole run on one flaky call.
      health = `unreadable (${(err as Error).message.split('\n')[0]})`
    }
    if (health === 'healthy') {
      log('PostgreSQL is ready.')
      return
    }
    await new Promise((r) => setTimeout(r, 1000))
  }
  throw new Error(
    `Timeout waiting for PostgreSQL to be ready after ` +
      `${DB_READY_TIMEOUT_MS / 1000}s (last health status: ${health})`
  )
}

async function seedDatabase() {
  log('Seeding database...')
  execSync('bash ../frontend/e2e/setup-db.sh', {
    cwd: BACKEND_DIR,
    stdio: 'inherit',
    env: { ...process.env, DATABASE_URL: E2E_DB_URL },
  })
}

function startBackend(): ChildProcess {
  log(`Starting backend on port ${BACKEND_PORT}...`)
  const proc = spawn('go', ['run', './cmd/server'], {
    cwd: BACKEND_DIR,
    env: {
      ...process.env,
      DATABASE_URL: E2E_DB_URL,
      // PSY-1645: bind where BACKEND_URL says, so the spawned backend, the
      // health check, the Next proxy and the fixture-reset helper all agree.
      // Without this the server took its default :8080 while everything else
      // followed BACKEND_URL elsewhere.
      API_ADDR: `localhost:${BACKEND_PORT}`,
      JWT_SECRET_KEY: 'e2e-jwt-secret-key-for-testing-only',
      OAUTH_SECRET_KEY: 'e2e-oauth-secret-key-for-testing-only',
      CORS_ALLOWED_ORIGINS: 'http://localhost:3000',
      SESSION_SECURE: 'false',
      SESSION_SAME_SITE: 'lax',
      DISCORD_NOTIFICATIONS_ENABLED: 'false',
      // Disable all scheduled background services for E2E (see PSY-433).
      // These cause log spam, nondeterministic DB state, and wasted CPU during
      // tests. Defaults (flag unset) still start everything for local dev.
      DISABLE_RADIO_FETCH: '1',
      DISABLE_AUTO_PROMOTION: '1',
      DISABLE_ENRICHMENT_WORKER: '1',
      DISABLE_COLLECTION_DIGEST: '1',
      DISABLE_CLEANUP: '1',
      DISABLE_REMINDERS: '1',
      DISABLE_RELATIONSHIP_DERIVATION: '1',
      DISABLE_STREET_GEOCODE_SWEEP: '1',
      // PSY-1612. Default-ON like the sweeps above, and it runs at boot, so
      // without this the E2E backend would start a loop the harness's
      // "no scheduled tickers" contract says it doesn't.
      DISABLE_SWEEP_HEALTH_CHECK: '1',
      // PSY-1894. Also default-ON, and its enqueue rides the show-create
      // transaction, so without this an E2E test that creates a show leaves a
      // queue row that the poller drains into notification_log rows a later
      // assertion can see. Exactly the nondeterministic DB state this block exists
      // to prevent.
      DISABLE_SHOW_NOTIFY_OUTBOX: '1',
      // PSY-432: enable the /admin/test-fixtures/reset endpoint. Guarded by
      // a default-deny ENVIRONMENT check on the backend — the server
      // refuses to boot if ENABLE_TEST_FIXTURES=1 and ENVIRONMENT is not
      // one of {test, ci, development}.
      ENABLE_TEST_FIXTURES: '1',
      ENVIRONMENT: 'test',
      // PSY-475: replace the IP-scoped auth (10/min) + passkey (20/min)
      // rate limiters with no-op middleware for E2E. All parallel workers
      // on a CI shard share 127.0.0.1, so the limits got tripped on shard
      // 3 and caused intermittent failures in register.spec.ts and
      // magic-link.spec.ts. Same default-deny ENVIRONMENT guard as
      // ENABLE_TEST_FIXTURES — the server refuses to boot if the flag is
      // set in anything other than test/ci/development.
      DISABLE_AUTH_RATE_LIMITS: '1',
      // PSY-914: register the faux "google" OAuth provider so
      // oauth-google.spec.ts can exercise the real login -> callback ->
      // session flow without a live Google IdP. Same default-deny ENVIRONMENT
      // guard as the two flags above — the server refuses to boot if this is
      // set outside {test, ci, development}, so the fake provider can never
      // reach production.
      ENABLE_OAUTH_TEST_PROVIDER: '1',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true,
  })

  // Forward backend stdout/stderr so failures are visible
  proc.stdout?.on('data', (data: Buffer) => {
    const line = data.toString().trim()
    if (line) console.log(`[backend] ${line}`)
  })
  proc.stderr?.on('data', (data: Buffer) => {
    const line = data.toString().trim()
    if (line) console.error(`[backend] ${line}`)
  })

  // Save PID for teardown
  if (proc.pid) {
    fs.writeFileSync(PID_FILE, String(proc.pid))
    // Detach so backend keeps running after this process
    proc.unref()
  }

  return proc
}

// PSY-1557: how many logins may load /auth at the same time.
//
// The original code ran all USER_COUNT + 1 captures through a single
// Promise.all. Six simultaneous page loads do NOT cost the same as one against
// a Next.js dev server: measured here, a lone login's navigation took ~2.1s,
// while the same navigation inside the six-way burst took 8-28s depending on
// machine load. The dev server serves each page as hundreds of separate module
// requests, so concurrent loads contend for it instead of overlapping. Since
// page.goto's default navigation timeout is 30s, a loaded machine (several
// dispatch stacks running at once) pushed the burst past that limit and failed
// global-setup outright.
//
// Warming /auth first was measured too and is NOT sufficient on its own — it
// removes only the one-time compile (~1.6-2.9s), not the per-request
// contention. Bounding concurrency is what keeps the tail in check.
//
// Numbers above measured 2026-07-26 (PSY-1557) on a 10-core machine, cold
// .next, with a sibling dev server compiling — re-measure before trusting
// them across a Next.js or Playwright upgrade.
const AUTH_CAPTURE_CONCURRENCY = 2

// Budget for the two waits that load /auth itself — the navigation and the
// form render. They cover one continuous stretch of dev-server compilation
// under contention, so they must move together; splitting them just moves the
// failure from one line to the next.
const AUTH_PAGE_TIMEOUT_MS = 60_000

// The post-login redirect gets its own, smaller budget on purpose. It lands on
// `/`, which globalSetup has already warmed via waitForUrl before any capture
// starts, so it does NOT carry the cold-compile risk that /auth does — and the
// measurements bear that out: worst observed was 9.4s (under the old unbounded
// 6-way burst) and 1.5s once concurrency was capped. 30s is ~3x the worst
// value ever recorded here, while keeping the per-login ceiling low enough to
// stay useful: the whole phase's worst case scales as
// ceil(logins / AUTH_CAPTURE_CONCURRENCY) x (2 x AUTH_PAGE_TIMEOUT_MS +
// POST_LOGIN_REDIRECT_TIMEOUT_MS), which has to stay diagnosable inside the
// e2e-smoke job budget rather than being killed by it.
const POST_LOGIN_REDIRECT_TIMEOUT_MS = 30_000

// Readiness budget for a server process to answer at all. Deliberately NOT
// AUTH_PAGE_TIMEOUT_MS: that one covers page compilation under contention,
// this one covers process boot. They happen to be equal today; keep them
// separate so tuning one doesn't silently retune the other.
const SERVER_READY_TIMEOUT_MS = 60_000

type SeededLogin = {
  email: string
  password: string
  authFile: string
}

async function captureStorageState(
  browser: Browser,
  login: SeededLogin
): Promise<void> {
  const context = await browser.newContext()
  try {
    const page = await context.newPage()
    await loginAs(page, login.email, login.password)
    await context.storageState({ path: path.join(AUTH_DIR, login.authFile) })
    // Per-login progress: when a login is the thing that hangs, this is what
    // tells you which one, and how far the batch got, instead of leaving an
    // outer job timeout with nothing to go on.
    log(`  captured ${login.authFile}`)
  } finally {
    // Never let a teardown failure replace the real error: if one capture
    // rejects, the shared browser is closed while siblings are still in
    // flight, so their context.close() can fail for reasons that have nothing
    // to do with why the run actually failed. Log rather than discard, so a
    // close failure that ISN'T that race still leaves a trace.
    await context
      .close()
      .catch((err) => log(`  warn: close failed for ${login.authFile}: ${err}`))
  }
}

async function captureAuthState() {
  log(`Capturing auth state for ${USER_COUNT} regular users + 1 admin...`)
  fs.mkdirSync(AUTH_DIR, { recursive: true })

  const logins: SeededLogin[] = [
    ...Array.from({ length: USER_COUNT }, (_, i) => ({
      email: userEmailForWorker(i),
      password: TEST_PASSWORD,
      authFile: userAuthFileForWorker(i),
    })),
    {
      email: TEST_ADMIN.email,
      password: TEST_ADMIN.password,
      authFile: 'admin.json',
    },
  ]

  const browser = await chromium.launch()

  // Still parallel — global-setup overhead has to stay within budget, and
  // serial would scale linearly with user count — but capped (see
  // AUTH_CAPTURE_CONCURRENCY).
  const limit = pLimit(AUTH_CAPTURE_CONCURRENCY)

  try {
    await Promise.all(
      logins.map((login) => limit(() => captureStorageState(browser, login)))
    )
  } finally {
    await browser.close()
  }
  log('Auth state captured.')
}

async function loginAs(page: Page, email: string, password: string) {
  // PSY-1557: navigating here can be far slower than Playwright's 30s default
  // — dev-server compilation plus contention from the other captures. Without
  // an explicit budget a loaded machine turns a merely-slow setup into a hard
  // global-setup failure.
  await page.goto('http://localhost:3000/auth', {
    timeout: AUTH_PAGE_TIMEOUT_MS,
  })

  // Wait for login form to render (handles dev compilation + React hydration + auth check)
  await page
    .locator('#email')
    .waitFor({ state: 'visible', timeout: AUTH_PAGE_TIMEOUT_MS })

  // Fill login form — use ID selectors for reliability during setup
  await page.locator('#email').fill(email)
  await page.locator('#password').fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()

  // Wait for redirect away from /auth (successful login). See
  // POST_LOGIN_REDIRECT_TIMEOUT_MS for why this one is deliberately smaller
  // than the two waits above.
  await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
    timeout: POST_LOGIN_REDIRECT_TIMEOUT_MS,
  })
}

export default async function globalSetup(_config: FullConfig) {
  log('Starting E2E global setup...')

  // 1. Start database
  await startDatabase()

  // 2. Seed data
  await seedDatabase()

  // 3. Check the backend port is free, then start backend
  if (await isPortInUse(BACKEND_PORT)) {
    throw new Error(
      `Port ${BACKEND_PORT} is already in use. Either stop the dev backend, ` +
        `or point the harness at a FREE port with BACKEND_URL ` +
        `(e.g. BACKEND_URL=http://localhost:8099 bun run test:e2e) — note ` +
        `that provisions a NEW backend there; it does not talk to the one ` +
        `already running. Testing against an already-running stack is ` +
        `\`bun run test:e2e:external\`, but only for a stack configured with ` +
        `ENABLE_TEST_FIXTURES=1 and the same JWT_SECRET_KEY the e2e/.auth ` +
        `state was captured under — a plain run-dev.sh backend has neither. ` +
        `See e2e/playwright.external.config.ts.`
    )
  }
  startBackend()

  // 4. Wait for backend health
  log('Waiting for backend health check...')
  await waitForUrl(`${BACKEND_BASE_URL}/health`, SERVER_READY_TIMEOUT_MS)
  log('Backend is healthy.')

  // 5. Wait for frontend (started by Playwright webServer config)
  log('Waiting for frontend...')
  await waitForUrl('http://localhost:3000', SERVER_READY_TIMEOUT_MS)
  log('Frontend is ready.')

  // 6. Capture auth state
  await captureAuthState()

  log('Global setup complete!')
}
