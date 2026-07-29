/**
 * Proof harness for the pre-hydration click capture-and-replay primitive.
 *
 * PSY-1610 built an equivalent harness, proved the bug with it, and then threw
 * it away — so PSY-1615 had to build it a second time. It is committed this
 * time. It does not run in the normal E2E suite (own directory, own config,
 * no `webServer`, no `globalSetup`) because it needs a **production build** and
 * a dedicated stack; dev-server timings are not representative.
 *
 * ## Running it
 *
 * 1. Postgres + seed, on ports that do not collide with the standard E2E stack:
 *
 *        cd backend
 *        docker compose -p e2e-psy1615 -f docker-compose.e2e.yml \
 *          -f <override mapping 5455:5432> up -d
 *        DATABASE_URL='postgres://e2euser:e2epassword@localhost:5455/e2edb?sslmode=disable' \
 *          COMPOSE_PROJECT=e2e-psy1615 bash ../frontend/e2e/setup-db.sh
 *
 * 2. Backend on :8099 with the same env `e2e/global-setup.ts` uses, plus
 *    `API_ADDR=localhost:8099` and `DATABASE_URL` pointing at 5455.
 *
 * 3. Production frontend. **`NEXT_PUBLIC_API_URL` must be set at BUILD time** —
 *    `NEXT_PUBLIC_*` is inlined into the client bundle, so setting it only on
 *    `start` leaves the production fallback (`https://api.psychichomily.com`)
 *    compiled in and every browser call leaves the machine. It cost an hour
 *    here, and it fails in a way that looks like the feature is broken:
 *    the mutation fires, is blocked by CORS against the live API, and the local
 *    database stays empty.
 *
 *    It must also be ABSOLUTE. `/api` works in the browser but not for the
 *    server render, which has no origin to resolve it against — the page then
 *    renders without its data and the control never appears at all.
 *
 *        NEXT_PUBLIC_API_URL=http://localhost:3099/api bun run build
 *        BACKEND_URL=http://localhost:8099 bun run start --port 3099
 *
 * 4. `bunx playwright test --config=e2e-hydration/playwright.hydration.config.ts`
 *
 * ## What it measures
 *
 * React stamps a `__reactProps$…` key on a host DOM node at the instant it
 * hydrates that node. An init script polls the target for that key and records
 * `performance.now()`, and a capture-phase listener registered before any page
 * script records each click together with whether the node was hydrated at that
 * moment. That is how a click is *proven* to be pre-hydration rather than
 * assumed to be.
 *
 * Three traps PSY-1610 documented, each of which produced a confident wrong
 * answer first, and how this harness avoids them:
 *
 * 1. **Fixed coordinates miss.** Layout shifts as images load, so the target's
 *    box is re-measured immediately before the click and `elementFromPoint` is
 *    asserted to be inside the target.
 * 2. **"State changed" is not "the click worked."** Hydration itself rewrites
 *    `aria-label`s and mounts portals, so the mutation is verified against the
 *    network log and the database, never against rendered text.
 * 3. **Leaked database state.** `user_bookmarks` is truncated before every
 *    trial. Without that, earlier runs' writes made pre-hydration saves *look*
 *    like they had succeeded.
 */
import { test, expect, type Page } from '@playwright/test'
import { execFileSync } from 'child_process'

const SHOW_SLUG = '2026-07-30-e2e-test-show-1'
const USER_EMAIL = 'e2e-user@test.local'
const USER_PASSWORD = 'e2e-test-password-123'

const DB = {
  host: 'localhost',
  port: process.env.HYDRATION_DB_PORT ?? '5455',
  user: 'e2euser',
  password: 'e2epassword',
  name: 'e2edb',
}

function psql(sql: string): string {
  return execFileSync(
    'psql',
    ['-h', DB.host, '-p', DB.port, '-U', DB.user, '-d', DB.name, '-Atc', sql],
    { env: { ...process.env, PGPASSWORD: DB.password }, encoding: 'utf8' }
  ).trim()
}

function resetBookmarks() {
  psql('DELETE FROM user_bookmarks')
}

function bookmarkCount(): number {
  return Number(psql(`SELECT count(*) FROM user_bookmarks`))
}

/**
 * Registered before any page script. Times hydration per-node and tags every
 * click with the hydration state of its target at the instant it landed.
 */
const INSTRUMENTATION = `
window.__probe = { clicks: [], hydratedAt: null, fcp: null, domAt: null };

new PerformanceObserver(list => {
  for (const entry of list.getEntries()) {
    if (entry.name === 'first-contentful-paint' && window.__probe.fcp === null) {
      window.__probe.fcp = entry.startTime;
    }
  }
}).observe({ type: 'paint', buffered: true });

function reactKey(node) {
  for (const key in node) {
    if (key.startsWith('__reactProps$')) return true;
  }
  return false;
}

// Target the save control by its accessible name, NOT by a generic
// [data-replay-on-hydrate] lookup: on an authenticated page the TopBar's bell
// and user menu are replay roots too and come first in DOM order. The label
// gains a "(N saved)" suffix once the count query resolves, so match on the
// prefix (trap 2).
function target() {
  return document.querySelector(
    'button[aria-label^="Add to My List"], button[aria-label^="Remove from My List"]'
  );
}

// 1 ms poll: React attaching its props key is the moment the node goes live.
const poll = setInterval(() => {
  const node = target();
  if (!node) return;
  if (window.__probe.domAt === null) window.__probe.domAt = performance.now();
  if (reactKey(node)) {
    window.__probe.hydratedAt = performance.now();
    clearInterval(poll);
  }
}, 1);

document.addEventListener('click', event => {
  const node = target();
  window.__probe.clicks.push({
    at: performance.now(),
    trusted: event.isTrusted,
    onTarget: !!node && node.contains(event.target),
    hydratedNow: !!node && reactKey(node),
  });
}, true);
`

interface Probe {
  clicks: Array<{
    at: number
    trusted: boolean
    onTarget: boolean
    hydratedNow: boolean
  }>
  hydratedAt: number | null
  fcp: number | null
  domAt: number | null
}

async function login(page: Page) {
  const response = await page.request.post('/api/auth/login', {
    data: { email: USER_EMAIL, password: USER_PASSWORD },
  })
  expect(response.ok(), `login failed: ${response.status()}`).toBe(true)
}

/**
 * Click the save control the instant it is painted — the behaviour of a user
 * who knows where the button is. Re-measures the box immediately before firing
 * and asserts the point is really over the target (trap 1).
 */
async function clickAsSoonAsPainted(page: Page, timeoutMs = 15_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const box = await page.evaluate(() => {
      const node = document.querySelector(
        'button[aria-label^="Add to My List"], button[aria-label^="Remove from My List"]'
      )
      if (!node) return null
      const rect = node.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) return null
      const x = rect.left + rect.width / 2
      const y = rect.top + rect.height / 2
      const hit = document.elementFromPoint(x, y)
      return { x, y, over: !!hit && node.contains(hit) }
    })
    if (box?.over) {
      await page.mouse.click(box.x, box.y)
      return true
    }
    await page.waitForTimeout(4)
  }
  return false
}

test.describe('pre-hydration clicks on a mutation control', () => {
  test.beforeEach(() => {
    resetBookmarks()
  })

  test('a click before hydration results in exactly one save', async ({
    page,
  }) => {
    await login(page)
    await page.addInitScript(INSTRUMENTATION)

    // Every request the page makes to the save endpoint, counted at the
    // network layer rather than inferred from the UI (trap 2).
    const saveRequests: string[] = []
    page.on('request', request => {
      const url = request.url()
      if (/\/saved-shows\/\d+/.test(url)) {
        saveRequests.push(`${request.method()} ${new URL(url).pathname}`)
      }
    })
    // Status matters as much as the count: a rejected POST still shows up as a
    // request, and would otherwise read as a successful replay.
    const saveResponses: string[] = []
    page.on('response', response => {
      if (/\/saved-shows\/\d+/.test(response.url())) {
        saveResponses.push(`${response.request().method()} ${response.status()}`)
      }
    })
    const failures: string[] = []
    page.on('requestfailed', request => {
      if (/\/saved-shows\/\d+/.test(request.url())) {
        failures.push(`${request.method()} ${request.failure()?.errorText}`)
      }
    })
    const navigations: string[] = []
    page.on('framenavigated', frame => {
      if (frame === page.mainFrame()) navigations.push(frame.url())
    })
    const consoleErrors: string[] = []
    page.on('console', msg => {
      if (msg.type() === 'error') consoleErrors.push(msg.text().slice(0, 200))
    })
    page.on('pageerror', err => consoleErrors.push(`pageerror: ${err.message.slice(0, 200)}`))

    // Throttle hard enough to open a window a human could act in. PSY-1610
    // measured ~4.6s at 6x + slow 4G.
    const cdp = await page.context().newCDPSession(page)
    await cdp.send('Emulation.setCPUThrottlingRate', { rate: 6 })
    await cdp.send('Network.emulateNetworkConditions', {
      offline: false,
      latency: 150,
      downloadThroughput: (1.6 * 1024 * 1024) / 8,
      uploadThroughput: (750 * 1024) / 8,
    })

    await page.goto(`/shows/${SHOW_SLUG}`, { waitUntil: 'commit' })

    expect(await clickAsSoonAsPainted(page), 'never got a click on target').toBe(
      true
    )

    // Give hydration, the replay, and the mutation round trip time to finish.
    await page.waitForTimeout(12_000)

    const probe = (await page.evaluate(() => window.__probe)) as Probe
    const onTarget = probe.clicks.filter(c => c.onTarget && c.trusted)

    // The whole experiment is void unless the click really did land before the
    // node was live — otherwise this proves nothing about replay.
    expect(onTarget.length, 'no trusted on-target click recorded').toBeGreaterThan(0)
    expect(
      onTarget[0].hydratedNow,
      `click landed AFTER hydration (window too small: fcp=${probe.fcp}, hydratedAt=${probe.hydratedAt}) — raise the throttle`
    ).toBe(false)

    console.log(
      `[harness] fcp=${probe.fcp?.toFixed(0)}ms domAt=${probe.domAt?.toFixed(0)}ms ` +
        `hydratedAt=${probe.hydratedAt?.toFixed(0)}ms ` +
        `dead window=${((probe.hydratedAt ?? 0) - (probe.fcp ?? 0)).toFixed(0)}ms\n` +
        `[harness] save requests: ${JSON.stringify(saveRequests)} ` +
        `responses: ${JSON.stringify(saveResponses)} ` +
        `failures: ${JSON.stringify(failures)}\n` +
        `[harness] navigations: ${JSON.stringify(navigations)}\n` +
        `[harness] console errors: ${JSON.stringify(consoleErrors)}`
    )

    // The point of the ticket: the click is honoured...
    expect(bookmarkCount(), 'the save did not happen').toBe(1)
    // ...and exactly once. A double-fired Save is worse than the bug.
    expect(
      saveRequests.filter(r => r.startsWith('POST')),
      'expected exactly one POST to the save endpoint'
    ).toHaveLength(1)
  })

  test('a pre-hydration click opens a Radix pointerdown trigger', async ({
    page,
  }) => {
    // The TopBar user menu is a Radix DropdownMenu, which opens on
    // `onPointerDown` rather than `onClick`. It is the reason the primitive
    // replays the whole pointer sequence instead of a lone click, so it is
    // worth proving end-to-end rather than trusting the unit test.
    await login(page)
    // At the default 1280px the TopBar cluster overflows: the trigger's right
    // edge lands past the viewport, so its centre point has nothing to hit and
    // no click can be aimed at it. Widen so the control is fully on-screen.
    await page.setViewportSize({ width: 1600, height: 900 })

    const cdp = await page.context().newCDPSession(page)
    await cdp.send('Emulation.setCPUThrottlingRate', { rate: 6 })
    await cdp.send('Network.emulateNetworkConditions', {
      offline: false,
      latency: 150,
      downloadThroughput: (1.6 * 1024 * 1024) / 8,
      uploadThroughput: (750 * 1024) / 8,
    })

    await page.goto(`/shows/${SHOW_SLUG}`, { waitUntil: 'commit' })

    const trigger = 'button[aria-label="User menu"]'
    const deadline = Date.now() + 15_000
    let clicked = false
    while (Date.now() < deadline && !clicked) {
      const box = await page.evaluate(sel => {
        const node = document.querySelector(sel)
        if (!node) return null
        const rect = node.getBoundingClientRect()
        if (rect.width === 0) return null
        const x = rect.left + rect.width / 2
        const y = rect.top + rect.height / 2
        const hit = document.elementFromPoint(x, y)
        const hydrated = Object.keys(node).some(k => k.startsWith('__reactProps$'))
        return { x, y, over: !!hit && node.contains(hit), hydrated }
      }, trigger)
      if (box?.over) {
        expect(
          box.hydrated,
          'trigger hydrated before the click landed — raise the throttle'
        ).toBe(false)
        await page.mouse.click(box.x, box.y)
        clicked = true
      } else {
        await page.waitForTimeout(4)
      }
    }
    expect(clicked, 'never got a click on the user menu trigger').toBe(true)

    // Radix sets data-state="open" on the trigger and mounts a [role=menu].
    await expect(page.locator('[role="menu"]')).toBeVisible({ timeout: 20_000 })
  })

  test('a click after hydration still saves exactly once', async ({ page }) => {
    // The control arm: replay must not perturb the ordinary path.
    await login(page)
    await page.addInitScript(INSTRUMENTATION)

    const saveRequests: string[] = []
    page.on('request', request => {
      if (/\/saved-shows\/\d+/.test(request.url())) {
        saveRequests.push(request.method())
      }
    })

    await page.goto(`/shows/${SHOW_SLUG}`)
    await page.waitForFunction(() => window.__probe?.hydratedAt !== null, null, {
      timeout: 30_000,
    })

    expect(await clickAsSoonAsPainted(page)).toBe(true)
    await page.waitForTimeout(3_000)

    expect(bookmarkCount()).toBe(1)
    expect(saveRequests.filter(m => m === 'POST')).toHaveLength(1)
  })
})

declare global {
  interface Window {
    __probe?: Probe
  }
}
