#!/usr/bin/env node
// Atlas mobile perf gate: first rendered map and entry bytes on a phone
// profile, checked against the owner budget.
//
//   bun run perf:atlas <base-url> [options]
//   node scripts/atlas-perf.mjs https://<preview>.vercel.app --runs 3
//
// Target a production build (a Vercel preview, or `next build && next start`).
// `next dev` serves unminified, uncompressed bundles and is not a valid target.
//
// Options:
//   --runs N            cold runs; the budget is checked on the medians (default 3)
//   --path P            page path (default /atlas)
//   --city LNG,LAT      city-view jump target (default Chicago -87.6298,41.8781)
//   --device NAME       iphone13 (default) | desktop
//   --viewport WxH      override the viewport (default 390x844 for iphone13)
//   --headed            headed Chromium (uses the machine's GPU on macOS)
//   --json FILE         write the full report, including every request, to FILE
//   --no-budget         report only; always exit 0 once a map renders
//
// Env: VERCEL_PROTECTION_BYPASS, if set, is sent only to *.vercel.app hosts to
// get past preview SSO (as a one-time bypass cookie request, never on page
// requests to other hosts).
//
// Exit codes: 0 budget met (or --no-budget), 1 budget missed, 2 harness error
// (bad arguments, or the page never rendered a map; below 640px wide the
// Atlas renders a scene list instead of the map).
//
// Profile (the owner budget's stated conditions):
//   - Device: Playwright's `iPhone 13` descriptor (mobile UA, touch, DPR 3) at
//     390x844, the phone boards' frame. The descriptor's own viewport is
//     390x664 (Safari's visible area with toolbars); pass --viewport to use it.
//   - CPU: CDP Emulation.setCPUThrottlingRate 4, Lighthouse's default mobile
//     multiplier (https://github.com/GoogleChrome/lighthouse/blob/main/docs/throttling.md).
//     JavaScript only: GPU work is NOT throttled, so render-bound timings are
//     a lower bound for a real phone.
//   - Network: Puppeteer's `Fast 4G` preset, 9 Mbps down and 1.5 Mbps up at
//     90% efficiency, 165 ms latency (60 ms RTT x 2.75), via CDP
//     Network.emulateNetworkConditions
//     (https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/cdp/PredefinedNetworkConditions.ts).
//   - Cold: fresh browser context per run, HTTP cache disabled.
//
// Measures:
//   - First rendered map: ms from navigation start until
//     `isStyleLoaded() && areTilesLoaded()` and `queryRenderedFeatures()`
//     returns at least one feature, polled in the page every 50 ms on the
//     `window.__atlasMap` seam. MapLibre's `idle` event never fires on /atlas
//     (the pulse rings repaint every frame), so it is not used.
//   - Entry bytes: every request that finished between navigation and
//     first rendered map + SETTLE_MS, on the wire (encoded body + headers,
//     from Playwright's request.sizes()), each URL counted once per resource
//     type (cache-disabled runs refetch preloaded fonts). Requests to
//     Vercel's preview toolbar are listed but excluded from the total: they
//     do not exist in production.
//   - City view: after the entry window, jump the camera to --city at z12.5
//     and measure time until the map is ready again, plus the bytes that
//     finished in the following SETTLE_MS. Reported, not budgeted.
import { writeFileSync } from 'node:fs'
import { chromium, devices } from '@playwright/test'

const BUDGET = {
  firstMapMs: 2500,
  entryBytes: 1.5 * 1024 * 1024,
}
const FAST_4G = {
  offline: false,
  downloadThroughput: ((9 * 1000 * 1000) / 8) * 0.9,
  uploadThroughput: ((1.5 * 1000 * 1000) / 8) * 0.9,
  latency: 60 * 2.75,
}
const CPU_THROTTLE_RATE = 4
const SETTLE_MS = 3000
const READY_TIMEOUT_MS = 90_000
const CITY_ZOOM = 12.5
const PREVIEW_ONLY_HOSTS = ['vercel.live']

function usage(message) {
  if (message) console.error(`error: ${message}`)
  console.error('usage: node scripts/atlas-perf.mjs <base-url> [--runs N] [--path P] [--city LNG,LAT] [--device iphone13|desktop] [--viewport WxH] [--headed] [--json FILE] [--no-budget]')
  process.exit(2)
}

function parseArgs(argv) {
  const opts = {
    runs: 3,
    path: '/atlas',
    city: [-87.6298, 41.8781],
    device: 'iphone13',
    viewport: null,
    headed: false,
    json: null,
    budget: true,
  }
  const positional = []
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const next = () => {
      const value = argv[++i]
      if (value === undefined) usage(`${arg} needs a value`)
      return value
    }
    if (arg === '--runs') opts.runs = Number(next())
    else if (arg === '--path') opts.path = next()
    else if (arg === '--city') opts.city = next().split(',').map(Number)
    else if (arg === '--device') opts.device = next()
    else if (arg === '--viewport') {
      const [width, height] = next().split('x').map(Number)
      opts.viewport = { width, height }
    } else if (arg === '--headed') opts.headed = true
    else if (arg === '--json') opts.json = next()
    else if (arg === '--no-budget') opts.budget = false
    else if (arg.startsWith('--')) usage(`unknown option ${arg}`)
    else positional.push(arg)
  }
  if (positional.length !== 1) usage('exactly one base URL is required')
  let url
  try {
    url = new URL(opts.path, positional[0])
  } catch {
    usage(`not a URL: ${positional[0]}`)
  }
  if (!Number.isInteger(opts.runs) || opts.runs < 1) usage('--runs must be a positive integer')
  if (opts.city.length !== 2 || opts.city.some((n) => !Number.isFinite(n))) usage('--city must be LNG,LAT')
  if (!['iphone13', 'desktop'].includes(opts.device)) usage('--device must be iphone13 or desktop')
  if (opts.viewport && !(opts.viewport.width > 0 && opts.viewport.height > 0)) usage('--viewport must be WxH')
  return { ...opts, url }
}

function contextOptions(opts) {
  if (opts.device === 'desktop') {
    return { ...devices['Desktop Chrome'], viewport: opts.viewport ?? { width: 1440, height: 900 } }
  }
  return { ...devices['iPhone 13'], viewport: opts.viewport ?? { width: 390, height: 844 } }
}

function category(request) {
  const url = new URL(request.url)
  const host = url.host
  if (PREVIEW_ONLY_HOSTS.some((h) => host === h || host.endsWith(`.${h}`))) return 'preview-only'
  if (host === 'gibs.earthdata.nasa.gov') return 'raster'
  if (host === 'tiles.openfreemap.org') {
    return url.pathname.startsWith('/fonts/') ? 'glyphs' : 'tiles'
  }
  if (url.pathname.startsWith('/atlas/') && url.pathname.endsWith('.geojson')) return 'geodata'
  if (/\.woff2?$/.test(url.pathname) || request.type === 'font') return 'fonts'
  if (url.pathname.startsWith('/api/') || /(^|\.)api\.psychichomily\.com$/.test(host)) return 'api'
  if (request.type === 'script' || /\.m?js$/.test(url.pathname)) return 'js'
  if (request.type === 'stylesheet' || url.pathname.endsWith('.css')) return 'css'
  if (request.type === 'document') return 'document'
  if (url.searchParams.has('_rsc')) return 'rsc'
  if (request.type === 'image') return 'images'
  return 'other'
}

const CATEGORY_ORDER = ['document', 'js', 'css', 'fonts', 'api', 'rsc', 'tiles', 'glyphs', 'raster', 'geodata', 'images', 'other', 'preview-only']

function summarize(requests) {
  const byCategory = {}
  let budgetedBytes = 0
  for (const r of requests) {
    const c = (byCategory[r.category] ??= { requests: 0, bytes: 0 })
    c.requests++
    c.bytes += r.bytes
    if (r.category !== 'preview-only') budgetedBytes += r.bytes
  }
  return { byCategory, totalBytes: budgetedBytes }
}

// Polls the readiness gate inside the page so the timestamp is the page's
// own clock (ms since navigation start), unaffected by harness round trips.
const READINESS_PROBE = `(() => {
  const ready = () => {
    const m = window.__atlasMap
    if (!m) return false
    try {
      return m.isStyleLoaded() && m.areTilesLoaded() && m.queryRenderedFeatures().length > 0
    } catch {
      return false
    }
  }
  const poll = () => {
    if (ready()) {
      window.__atlasPerfFirstMapMs = performance.now()
      return
    }
    setTimeout(poll, 50)
  }
  poll()
})()`

async function waitFor(page, predicate, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const value = await page.evaluate(predicate).catch(() => null)
    if (value !== null && value !== undefined && value !== false) return value
    await page.waitForTimeout(100)
  }
  return null
}

async function oneRun(browser, opts) {
  const context = await browser.newContext(contextOptions(opts))
  const bypass = process.env.VERCEL_PROTECTION_BYPASS
  if (bypass && opts.url.hostname.endsWith('.vercel.app')) {
    const response = await context.request.get(opts.url.href, {
      headers: { 'x-vercel-protection-bypass': bypass, 'x-vercel-set-bypass-cookie': 'samesitenone' },
      maxRedirects: 0,
    })
    if (response.status() >= 400) throw new Error(`bypass request returned HTTP ${response.status()}`)
  }
  await context.addInitScript(READINESS_PROBE)
  const page = await context.newPage()
  const cdp = await context.newCDPSession(page)
  await cdp.send('Network.enable')
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: true })
  await cdp.send('Network.emulateNetworkConditions', FAST_4G)
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: CPU_THROTTLE_RATE })

  const finished = []
  const seen = new Set()
  context.on('requestfinished', async (request) => {
    const url = request.url()
    // Keyed by type too: the streamed HTML document and an RSC fetch can share
    // a URL, while a preloaded font refetched with the cache off shares both.
    const key = `${request.resourceType()} ${url}`
    if (url.startsWith('data:') || seen.has(key)) return
    seen.add(key)
    try {
      const sizes = await request.sizes()
      const entry = { url, type: request.resourceType(), bytes: sizes.responseBodySize + sizes.responseHeadersSize, at: Date.now() }
      entry.category = category(entry)
      finished.push(entry)
    } catch {
      // A request torn down with its page has no sizes to report.
    }
  })

  await page.goto(opts.url.href, { waitUntil: 'commit', timeout: READY_TIMEOUT_MS })
  const firstMapMs = await waitFor(page, () => window.__atlasPerfFirstMapMs ?? null, READY_TIMEOUT_MS)
  if (firstMapMs === null) {
    const hasCanvas = await page.evaluate(() => !!document.querySelector('.maplibregl-canvas')).catch(() => false)
    await context.close()
    throw new Error(hasCanvas
      ? `the map never passed the readiness gate within ${READY_TIMEOUT_MS} ms`
      : 'no map canvas rendered (below 640px wide the Atlas renders a scene list instead)')
  }
  await page.waitForTimeout(SETTLE_MS)
  const entryCut = Date.now()
  const entry = finished.filter((r) => r.at <= entryCut)
  const renderer = await page.evaluate(() => {
    const gl = document.createElement('canvas').getContext('webgl2')
    const ext = gl?.getExtension('WEBGL_debug_renderer_info')
    return ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : gl ? gl.getParameter(gl.RENDERER) : 'no WebGL2'
  })

  const cityMs = await page.evaluate(([lng, lat, zoom, timeoutMs]) => new Promise((resolve) => {
    const map = window.__atlasMap
    const t0 = performance.now()
    setTimeout(() => resolve(null), timeoutMs)
    map.jumpTo({ center: [lng, lat], zoom })
    // One render pass first: tile requests for the new camera are only
    // issued during render, so areTilesLoaded() is stale until then.
    map.once('render', () => {
      const poll = () => {
        let ready = false
        try {
          ready = map.isStyleLoaded() && map.areTilesLoaded() && map.queryRenderedFeatures().length > 0
        } catch {
          ready = false
        }
        if (ready) resolve(performance.now() - t0)
        else setTimeout(poll, 50)
      }
      poll()
    })
  }), [...opts.city, CITY_ZOOM, READY_TIMEOUT_MS])
  if (cityMs === null) {
    await context.close()
    throw new Error(`the city view never passed the readiness gate within ${READY_TIMEOUT_MS} ms`)
  }
  await page.waitForTimeout(SETTLE_MS)
  const city = finished.filter((r) => r.at > entryCut)
  await context.close()
  return {
    firstMapMs: Math.round(firstMapMs),
    entry: summarize(entry),
    city: { readyMs: Math.round(cityMs), ...summarize(city) },
    renderer,
    requests: { entry, city },
  }
}

const median = (values) => {
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}
const kib = (bytes) => (bytes / 1024).toFixed(0)
const mib = (bytes) => (bytes / 1024 / 1024).toFixed(2)

function printReport(opts, runs, verdict) {
  const ctx = contextOptions(opts)
  console.log(`\nAtlas perf: ${opts.url.href}`)
  console.log(`profile: ${opts.device} ${ctx.viewport.width}x${ctx.viewport.height} DPR ${ctx.deviceScaleFactor}, CPU ${CPU_THROTTLE_RATE}x, Fast 4G (9 Mbps down, 1.5 Mbps up, 165 ms), cache disabled, ${opts.headed ? 'headed' : 'headless'}`)
  console.log(`WebGL renderer: ${runs[0].renderer} (GPU not throttled)`)
  console.log(`runs: ${runs.map((r) => `${r.firstMapMs} ms / ${kib(r.entry.totalBytes)} KiB`).join(', ')}\n`)

  const categories = CATEGORY_ORDER.filter((c) => runs.some((r) => r.entry.byCategory[c] || r.city.byCategory[c]))
  console.log('| Category | Entry KiB (median) | Entry requests | City view KiB (median) |')
  console.log('|---|---:|---:|---:|')
  for (const c of categories) {
    const entryBytes = median(runs.map((r) => r.entry.byCategory[c]?.bytes ?? 0))
    const entryCount = median(runs.map((r) => r.entry.byCategory[c]?.requests ?? 0))
    const cityBytes = median(runs.map((r) => r.city.byCategory[c]?.bytes ?? 0))
    const label = c === 'preview-only' ? 'preview-only (excluded)' : c
    console.log(`| ${label} | ${kib(entryBytes)} | ${entryCount} | ${kib(cityBytes)} |`)
  }
  console.log(`| **Total** | **${kib(verdict.entryBytes)}** | | **${kib(verdict.cityBytes)}** |\n`)

  const mark = (ok) => (ok ? 'PASS' : 'FAIL')
  console.log('| Budget | Median | Limit | Result |')
  console.log('|---|---:|---:|---|')
  console.log(`| First rendered map | ${(verdict.firstMapMs / 1000).toFixed(2)} s | ${BUDGET.firstMapMs / 1000} s | ${mark(verdict.firstMapOk)} |`)
  console.log(`| Entry bytes | ${mib(verdict.entryBytes)} MiB | ${mib(BUDGET.entryBytes)} MiB | ${mark(verdict.entryOk)} |`)
  console.log(`\nCity view ready after jump: ${(verdict.cityMs / 1000).toFixed(2)} s (median, not budgeted)`)
}

const opts = parseArgs(process.argv.slice(2))
const browser = await chromium.launch({
  headless: !opts.headed,
  args: ['--use-gl=angle', '--enable-webgl', '--ignore-gpu-blocklist', '--enable-unsafe-swiftshader'],
})
const runs = []
try {
  for (let i = 0; i < opts.runs; i++) runs.push(await oneRun(browser, opts))
} catch (error) {
  console.error(`harness error: ${error.message}`)
  await browser.close()
  process.exit(2)
}
await browser.close()

const verdict = {
  firstMapMs: median(runs.map((r) => r.firstMapMs)),
  entryBytes: median(runs.map((r) => r.entry.totalBytes)),
  cityMs: median(runs.map((r) => r.city.readyMs)),
  cityBytes: median(runs.map((r) => r.city.totalBytes)),
}
verdict.firstMapOk = verdict.firstMapMs <= BUDGET.firstMapMs
verdict.entryOk = verdict.entryBytes <= BUDGET.entryBytes
printReport(opts, runs, verdict)
if (opts.json) {
  writeFileSync(opts.json, JSON.stringify({ url: opts.url.href, device: opts.device, budget: BUDGET, verdict, runs }, null, 2))
}
process.exit(!opts.budget || (verdict.firstMapOk && verdict.entryOk) ? 0 : 1)
