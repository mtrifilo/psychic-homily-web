#!/usr/bin/env node
// Atlas mobile perf gate: first rendered map and entry bytes on a phone
// profile, checked against the owner budget.
//
//   bun run perf:atlas <base-url> [options]
//   node scripts/atlas-perf.mjs https://<preview>.vercel.app --runs 3
//
// Target a production build (a Vercel preview, or `next build && next start`).
// `next dev` serves unminified, uncompressed bundles and is not a valid target.
// AtlasGlobe renders a scene list instead of the map in the conditions
// atlasRendersSceneList (features/scenes/atlasViewport.ts) names; the script
// sets up none of them, and if the page shows the list anyway it stops at once
// with exit code 2.
//
// Options:
//   --runs N            cold runs; the budget is checked on the medians (default 3)
//   --path P            page path (default /atlas)
//   --city LNG,LAT      city-view jump target (default Chicago -87.6298,41.8781)
//   --device NAME       iphone13 (default) | desktop
//   --viewport WxH      override the viewport (default 390x844 for iphone13)
//   --headed            headed Chromium (uses the machine's GPU on macOS)
//   --json FILE         write the full report ({ url, device, verdict, runs },
//                       every request included) to FILE
//   --no-budget         report only; exit 0 once every run finishes (2 on a
//                       harness error)
//   --budget-ms MS      first rendered map limit in ms (default 3500)
//   --budget-bytes N    entry bytes limit (default 1572864, 1.5 MiB)
//   --first-map-informational NOTE
//                       report first map against its limit without gating
//                       on it; NOTE replaces its Result cell
//
// Env: VERCEL_PROTECTION_BYPASS, if set, is sent to get past preview SSO, and
// only when the target is an https URL on one of this project's Vercel preview
// hosts (PREVIEW_HOST). It rides one bypass-cookie request, never page
// requests, and is redacted from anything the script prints.
//
// Budget (DEFAULT_BUDGET in scripts/lib/atlas-perf-budget.mjs): first
// rendered map at most 3.5 s and entry bytes at most 1.5 MiB, both medians,
// plus no raster request on a compact viewport. The 2.5 s first-map target is
// printed beside the budget with its delta and never changes the exit code.
//
// Exit codes: 0 budget met (or --no-budget), 1 budget missed, 2 harness error
// (bad arguments, a scene list instead of a map, or a map that never passed
// the readiness gate).
//
// Profile (the owner budget's stated conditions):
//   - Device: Playwright's `iPhone 13` descriptor (mobile UA, touch, DPR 3) at
//     390x844, the phone boards' frame. The descriptor's own viewport is
//     390x664 (Safari's visible area with toolbars); pass --viewport to use it.
//   - CPU: CDP Emulation.setCPUThrottlingRate 4, Lighthouse's default mobile
//     multiplier (https://github.com/GoogleChrome/lighthouse/blob/main/docs/throttling.md),
//     on the page's MAIN THREAD ONLY. Chromium refuses the call on dedicated
//     worker targets ("Operation is only supported for pages, not workers"),
//     so MapLibre's worker work (tile decode, GeoJSON parsing) runs at full
//     speed, and so does the GPU. First rendered map is therefore a lower
//     bound for a real phone.
//   - Network: Puppeteer's `Fast 4G` preset, 9 Mbps down and 1.5 Mbps up at
//     90% efficiency, 165 ms latency (60 ms RTT x 2.75), via CDP
//     Network.emulateNetworkConditions
//     (https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/cdp/PredefinedNetworkConditions.ts).
//   - Cold: fresh browser per run, HTTP cache disabled.
//
// Measures:
//   - First rendered map: ms from navigation start until
//     `isStyleLoaded() && areTilesLoaded()` and `queryRenderedFeatures()`
//     returns at least one feature, polled in the page every 50 ms on the
//     `window.__atlasMap` seam. MapLibre's `idle` event never fires on /atlas
//     (the pulse rings repaint every frame), so it is not used.
//   - Compact viewports (narrower than 1024px) must request no night-earth
//     raster tile at any point in the run (entry or city view, finished or
//     not): one raster request fails the run.
//   - Entry bytes (gated): every request that finished between navigation
//     and first rendered map, on the wire (encoded body + headers, from
//     Playwright's request.sizes()), each URL counted once per resource type:
//     what a phone downloads to see the map.
//   - Bytes in the first AFTER_MAP_WINDOW_MS after first map: the requests
//     that finished in that window, counted the same way. Reported, never
//     gated. Playwright under-reports dedicated-worker module scripts (the
//     vendored maplibre-gl-worker.mjs, about 6 KiB on the wire, reports under
//     1 KiB). Requests to
//     Vercel's preview toolbar are listed but excluded from the total: they
//     do not exist in production.
//   - City view: after the after-map window, jump the camera to --city at
//     z12.5 and measure time until the map is ready again, plus the bytes
//     that finished in the following AFTER_MAP_WINDOW_MS. Reported, not
//     budgeted.
import { writeFileSync } from 'node:fs'
import { chromium, devices } from '@playwright/test'
import { AFTER_MAP_WINDOW_MS, EXIT, budgetTable, errorMessage, median, resolveBudget, runBudgetCheck, seconds, targetLine } from './lib/atlas-perf-budget.mjs'

const FAST_4G = {
  offline: false,
  downloadThroughput: ((9 * 1000 * 1000) / 8) * 0.9,
  uploadThroughput: ((1.5 * 1000 * 1000) / 8) * 0.9,
  latency: 60 * 2.75,
}
const CPU_THROTTLE_RATE = 4
const READY_TIMEOUT_MS = 90_000
const CITY_ZOOM = 12.5
const PREVIEW_ONLY_HOSTS = ['vercel.live']
const PREVIEW_HOST = /^psychic-homily-[a-z0-9-]+-matts-projects-722d5204\.vercel\.app$/
// Tailwind's `lg` at the default root size; below it the Atlas draws the
// light globe, which requests no raster.
const COMPACT_MAX_WIDTH = 1023
// AtlasSceneList's root carries this test id.
const SCENE_LIST_SELECTOR = '[data-testid="atlas-scene-list"]'

function usage(message) {
  if (message) console.error(`error: ${message}`)
  console.error('usage: node scripts/atlas-perf.mjs <base-url> [--runs N] [--path P] [--city LNG,LAT] [--device iphone13|desktop] [--viewport WxH] [--headed] [--json FILE] [--no-budget] [--budget-ms MS] [--budget-bytes N] [--first-map-informational NOTE]')
  process.exit(EXIT.HARNESS_ERROR)
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
    enforce: true,
    firstMapNote: null,
  }
  const positional = []
  const overrides = {}
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
    else if (arg === '--no-budget') opts.enforce = false
    else if (arg === '--budget-ms') overrides.firstMapMs = next()
    else if (arg === '--budget-bytes') overrides.entryBytes = next()
    else if (arg === '--first-map-informational') opts.firstMapNote = next()
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
  if (opts.firstMapNote !== null && !opts.firstMapNote.trim()) usage('--first-map-informational needs a non-empty note')
  const { budget, error } = resolveBudget(overrides)
  if (error) usage(error)
  return { ...opts, url, budget }
}

function contextOptions(opts) {
  if (opts.device === 'desktop') {
    return { ...devices['Desktop Chrome'], reducedMotion: 'no-preference', viewport: opts.viewport ?? { width: 1440, height: 900 } }
  }
  return { ...devices['iPhone 13'], reducedMotion: 'no-preference', viewport: opts.viewport ?? { width: 390, height: 844 } }
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

// The readiness gate, defined once in the page and used for both the entry
// and the city view. The entry poll runs in the top frame every 50 ms, so the
// timestamp is the page's own clock (ms since navigation start), and reports
// once through an exposed binding (or reports the scene list, when the build
// renders that instead of a map). The poll itself is a small cost inside the
// measured window: until the map exists it is one querySelector, and the
// cheap map checks short-circuit queryRenderedFeatures until style and tiles
// load.
const READINESS_PROBE = `(() => {
  if (window !== window.top) return
  const sceneListShown = () => !!document.querySelector(${JSON.stringify(SCENE_LIST_SELECTOR)})
  window.__atlasPerfReady = () => {
    const m = window.__atlasMap
    if (!m) return false
    try {
      return m.isStyleLoaded() && m.areTilesLoaded() && m.queryRenderedFeatures().length > 0
    } catch {
      return false
    }
  }
  const poll = () => {
    if (window.__atlasPerfReady()) window.__atlasPerfFirstMap(performance.now())
    else if (!window.__atlasMap && sceneListShown()) window.__atlasPerfFirstMap('scene-list')
    else setTimeout(poll, 50)
  }
  poll()
})()`

const timeout = (ms) => new Promise((resolve) => setTimeout(() => resolve(null), ms))

async function oneRun(browser, opts) {
  const context = await browser.newContext(contextOptions(opts))
  const bypass = process.env.VERCEL_PROTECTION_BYPASS
  if (bypass && opts.url.protocol === 'https:' && PREVIEW_HOST.test(opts.url.hostname)) {
    let status
    try {
      const response = await context.request.get(opts.url.href, {
        headers: { 'x-vercel-protection-bypass': bypass, 'x-vercel-set-bypass-cookie': 'samesitenone' },
        maxRedirects: 0,
      })
      status = response.status()
    } catch (error) {
      // Playwright's error message carries the request's call log, headers
      // included, so it must not be rethrown or printed.
      throw new Error(`bypass request failed (${error?.code ?? error?.name ?? 'network error'})`)
    }
    if (status >= 400) throw new Error(`bypass request returned HTTP ${status}`)
  }
  let reportFirstMap
  // When the page reported first map, on the same clock as the request
  // timestamps below: entry bytes are the requests that finished by then.
  let firstMapAt = Infinity
  const firstMapReported = new Promise((resolve) => {
    reportFirstMap = resolve
  })
  await context.exposeFunction('__atlasPerfFirstMap', (ms) => {
    firstMapAt = Date.now()
    reportFirstMap(ms)
  })
  await context.addInitScript(READINESS_PROBE)
  const page = await context.newPage()
  const cdp = await context.newCDPSession(page)
  await cdp.send('Network.enable')
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: true })
  await cdp.send('Network.emulateNetworkConditions', FAST_4G)
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: CPU_THROTTLE_RATE })

  // One entry per resource type + URL, keeping the largest report. Keyed by
  // type because the streamed HTML document and an RSC fetch can share a URL.
  // Largest because duplicates report tiny bodies: a preloaded font refetched
  // with the cache off, and each of MapLibre's workers importing the same
  // module scripts, where only one import reports the full transfer.
  const byKey = new Map()
  context.on('requestfinished', async (request) => {
    const url = request.url()
    if (url.startsWith('data:')) return
    // Stamped before the sizes lookup, which resolves later than the finish.
    const at = Date.now()
    try {
      const sizes = await request.sizes()
      const entry = { url, type: request.resourceType(), bytes: sizes.responseBodySize + sizes.responseHeadersSize, at }
      entry.category = category(entry)
      const key = `${entry.type} ${url}`
      const previous = byKey.get(key)
      if (!previous || entry.bytes > previous.bytes) byKey.set(key, entry)
    } catch {
      // A request torn down with its page has no sizes to report.
    }
  })
  const finished = () => [...byKey.values()]
  // Raster requests are counted when ISSUED, for the whole run: a tile request
  // aborted by a late hide, or failed by an unreachable host, never reaches
  // requestfinished but still left the page.
  const rasterRequested = new Set()
  context.on('request', (request) => {
    if (category({ url: request.url(), type: request.resourceType() }) === 'raster') {
      rasterRequested.add(request.url())
    }
  })

  const response = await page.goto(opts.url.href, { waitUntil: 'commit', timeout: READY_TIMEOUT_MS })
  const landedHost = new URL(page.url()).host
  if (landedHost !== opts.url.host || (response && response.status() >= 400)) {
    const status = response ? response.status() : 'no response'
    await context.close()
    throw new Error(`the page did not load the target (HTTP ${status}, landed on ${landedHost}); a protected deployment needs VERCEL_PROTECTION_BYPASS and a PREVIEW_HOST match`)
  }
  const firstMapMs = await Promise.race([firstMapReported, timeout(READY_TIMEOUT_MS)])
  if (firstMapMs === 'scene-list') {
    await context.close()
    throw new Error('the page rendered the Atlas scene list, not the map: a condition atlasRendersSceneList names held, including a map that failed to start (see the header)')
  }
  if (firstMapMs === null) {
    await context.close()
    throw new Error(`the map never passed the readiness gate within ${READY_TIMEOUT_MS} ms`)
  }
  await page.waitForTimeout(AFTER_MAP_WINDOW_MS)
  const entryCut = Date.now()
  const entry = finished().filter((r) => r.at <= firstMapAt)
  const afterMap = finished().filter((r) => r.at > firstMapAt && r.at <= entryCut)
  // The map canvas's own context: getContext returns the existing one, so
  // the probe creates no extra GL context inside the measured session.
  const renderer = await page.evaluate(() => {
    const gl = window.__atlasMap.getCanvas().getContext('webgl2')
    const ext = gl?.getExtension('WEBGL_debug_renderer_info')
    return ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : gl ? gl.getParameter(gl.RENDERER) : 'unknown'
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
        if (window.__atlasPerfReady()) resolve(performance.now() - t0)
        else setTimeout(poll, 50)
      }
      poll()
    })
  }), [...opts.city, CITY_ZOOM, READY_TIMEOUT_MS])
  if (cityMs === null) {
    await context.close()
    throw new Error(`the city view never passed the readiness gate within ${READY_TIMEOUT_MS} ms`)
  }
  await page.waitForTimeout(AFTER_MAP_WINDOW_MS)
  const city = finished().filter((r) => r.at > entryCut)
  await context.close()
  return {
    firstMapMs: Math.round(firstMapMs),
    entry: summarize(entry),
    afterMap: summarize(afterMap),
    city: { readyMs: Math.round(cityMs), ...summarize(city) },
    renderer,
    rasterRequests: rasterRequested.size,
    requests: { entry, afterMap, city },
  }
}

const kib = (bytes) => (bytes / 1024).toFixed(0)

function printReport(opts, runs, verdict) {
  const ctx = contextOptions(opts)
  // Markdown, so the report reads the same in a terminal and a CI job summary.
  console.log(`\nAtlas perf: ${opts.url.href}\n`)
  console.log(`- profile: ${opts.device} ${ctx.viewport.width}x${ctx.viewport.height} DPR ${ctx.deviceScaleFactor}, CPU ${CPU_THROTTLE_RATE}x main thread only (workers and GPU unthrottled), Fast 4G (9 Mbps down, 1.5 Mbps up, 165 ms), cache disabled, ${opts.headed ? 'headed' : 'headless'}`)
  console.log(`- WebGL renderer: ${runs[0].renderer}`)
  console.log(`- runs: ${runs.map((r) => `${r.firstMapMs} ms / ${kib(r.entry.totalBytes)} KiB to first map + ${kib(r.afterMap.totalBytes)} KiB after / ${r.rasterRequests} raster requests`).join(', ')}\n`)

  const window = `${AFTER_MAP_WINDOW_MS / 1000} s after map`
  const categories = CATEGORY_ORDER.filter((c) => runs.some((r) => r.entry.byCategory[c] || r.afterMap.byCategory[c] || r.city.byCategory[c]))
  console.log(`| Category | Entry KiB, to first map (median) | Entry requests | ${window} KiB (median) | City view KiB (median) |`)
  console.log('|---|---:|---:|---:|---:|')
  for (const c of categories) {
    const entryBytes = median(runs.map((r) => r.entry.byCategory[c]?.bytes ?? 0))
    const entryCount = median(runs.map((r) => r.entry.byCategory[c]?.requests ?? 0))
    const afterBytes = median(runs.map((r) => r.afterMap.byCategory[c]?.bytes ?? 0))
    const cityBytes = median(runs.map((r) => r.city.byCategory[c]?.bytes ?? 0))
    const label = c === 'preview-only' ? 'preview-only (excluded)' : c
    console.log(`| ${label} | ${kib(entryBytes)} | ${entryCount} | ${kib(afterBytes)} | ${kib(cityBytes)} |`)
  }
  console.log(`| **Total** | **${kib(verdict.entryBytes)}** | | **${kib(verdict.afterMapBytes)}** | **${kib(verdict.cityBytes)}** |\n`)

  console.log(budgetTable(verdict).join('\n'))
  console.log(`\n${targetLine(verdict.target)}`)
  console.log(`\nCity view ready after jump: ${seconds(verdict.cityMs)} s (median, not budgeted)`)
}

const opts = parseArgs(process.argv.slice(2))
if (process.env.VERCEL_PROTECTION_BYPASS && !(opts.url.protocol === 'https:' && PREVIEW_HOST.test(opts.url.hostname))) {
  console.error(`note: VERCEL_PROTECTION_BYPASS is set but not sent: ${opts.url.host} is not an https PREVIEW_HOST`)
}
const bypass = process.env.VERCEL_PROTECTION_BYPASS
const exitCode = await runBudgetCheck({
  count: opts.runs,
  runOnce: async () => {
    // A fresh browser per run, not only a fresh context: a dedicated worker's
    // module fetch is not covered by the page's cache-disabled flag, and a
    // browser-level cache shared across contexts would undercount later runs.
    const browser = await chromium.launch({
      headless: !opts.headed,
      args: ['--use-gl=angle', '--enable-webgl', '--ignore-gpu-blocklist', '--enable-unsafe-swiftshader'],
    })
    try {
      return await oneRun(browser, opts)
    } finally {
      // A failing close must not replace the run's own result or error.
      await browser.close().catch(() => {})
    }
  },
  budget: opts.budget,
  compact: contextOptions(opts).viewport.width <= COMPACT_MAX_WIDTH,
  firstMapNote: opts.firstMapNote,
  enforce: opts.enforce,
  report: (runs, verdict) => {
    printReport(opts, runs, verdict)
    if (opts.json) {
      writeFileSync(opts.json, JSON.stringify({ url: opts.url.href, device: opts.device, verdict, runs }, null, 2))
    }
  },
  onError: (failure) => {
    const message = errorMessage(failure)
    console.error(`harness error: ${bypass ? message.replaceAll(bypass, '[redacted]') : message}`)
  },
})
process.exit(exitCode)
