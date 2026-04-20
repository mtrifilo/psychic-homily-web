import { execSync, spawn, type ChildProcess } from 'child_process'
import { chromium, type FullConfig } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import * as net from 'net'
import * as http from 'http'

const BACKEND_DIR = path.resolve(__dirname, '../../backend')
const PID_FILE = path.resolve(__dirname, '.backend-pid')
const AUTH_DIR = path.resolve(__dirname, '.auth')

const E2E_DB_URL =
  'postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable'

// Shared password for all seeded test users (see setup-db.sh).
const TEST_PASSWORD = 'e2e-test-password-123'

// PSY-431: seed N regular users so each Playwright worker gets its own
// authenticated user (worker index → user N), avoiding cross-worker races
// on shared user state (saved_shows, favorite_venues, submissions, etc.).
// Must match the seeded user count in setup-db.sh and the local workers
// cap in playwright.config.ts.
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
  execSync('docker compose -p e2e -f docker-compose.e2e.yml up -d', {
    cwd: BACKEND_DIR,
    stdio: 'inherit',
  })
  // Wait for DB to be healthy
  log('Waiting for PostgreSQL to be ready...')
  for (let i = 0; i < 30; i++) {
    try {
      execSync(
        'docker compose -p e2e -f docker-compose.e2e.yml exec -T db pg_isready -U e2euser -d e2edb',
        { cwd: BACKEND_DIR, stdio: 'pipe' }
      )
      log('PostgreSQL is ready.')
      return
    } catch {
      await new Promise((r) => setTimeout(r, 1000))
    }
  }
  throw new Error('Timeout waiting for PostgreSQL to be ready')
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
  log('Starting backend on port 8080...')
  const proc = spawn('go', ['run', './cmd/server'], {
    cwd: BACKEND_DIR,
    env: {
      ...process.env,
      DATABASE_URL: E2E_DB_URL,
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
      DISABLE_SCHEDULER: '1',
      DISABLE_CLEANUP: '1',
      DISABLE_REMINDERS: '1',
      DISABLE_RELATIONSHIP_DERIVATION: '1',
      // PSY-432: enable the /admin/test-fixtures/reset endpoint. Guarded by
      // a default-deny ENVIRONMENT check on the backend — the server
      // refuses to boot if ENABLE_TEST_FIXTURES=1 and ENVIRONMENT is not
      // one of {test, ci, development}.
      ENABLE_TEST_FIXTURES: '1',
      ENVIRONMENT: 'test',
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

async function captureAuthState() {
  log(`Capturing auth state for ${USER_COUNT} regular users + 1 admin...`)
  fs.mkdirSync(AUTH_DIR, { recursive: true })

  const browser = await chromium.launch()

  // Capture all users in parallel so global-setup overhead stays within budget
  // (each login is ~1s; serial would scale linearly with user count).
  const captureTasks: Promise<void>[] = []

  for (let i = 0; i < USER_COUNT; i++) {
    captureTasks.push((async () => {
      const context = await browser.newContext()
      const page = await context.newPage()
      await loginAs(page, userEmailForWorker(i), TEST_PASSWORD)
      await context.storageState({
        path: path.join(AUTH_DIR, userAuthFileForWorker(i)),
      })
      await context.close()
    })())
  }

  captureTasks.push((async () => {
    const adminContext = await browser.newContext()
    const adminPage = await adminContext.newPage()
    await loginAs(adminPage, TEST_ADMIN.email, TEST_ADMIN.password)
    await adminContext.storageState({
      path: path.join(AUTH_DIR, 'admin.json'),
    })
    await adminContext.close()
  })())

  await Promise.all(captureTasks)
  await browser.close()
  log('Auth state captured.')
}

async function loginAs(
  page: Awaited<ReturnType<Awaited<ReturnType<typeof chromium.launch>>['newPage']>>,
  email: string,
  password: string
) {
  await page.goto('http://localhost:3000/auth')

  // Wait for login form to render (handles dev compilation + React hydration + auth check)
  await page.locator('#email').waitFor({ state: 'visible', timeout: 60_000 })

  // Fill login form — use ID selectors for reliability during setup
  await page.locator('#email').fill(email)
  await page.locator('#password').fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()

  // Wait for redirect away from /auth (successful login)
  await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
    timeout: 15_000,
  })
}

export default async function globalSetup(_config: FullConfig) {
  log('Starting E2E global setup...')

  // 1. Start database
  await startDatabase()

  // 2. Seed data
  await seedDatabase()

  // 3. Check port 8080 is free, then start backend
  if (await isPortInUse(8080)) {
    throw new Error(
      'Port 8080 is already in use. Stop the dev backend before running E2E tests.'
    )
  }
  startBackend()

  // 4. Wait for backend health
  log('Waiting for backend health check...')
  await waitForUrl('http://localhost:8080/health', 60_000)
  log('Backend is healthy.')

  // 5. Wait for frontend (started by Playwright webServer config)
  log('Waiting for frontend...')
  await waitForUrl('http://localhost:3000', 60_000)
  log('Frontend is ready.')

  // 6. Capture auth state
  await captureAuthState()

  log('Global setup complete!')
}
