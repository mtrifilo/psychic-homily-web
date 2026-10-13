#!/usr/bin/env node
// Phone chrome perf: what a cold page load downloads, and how fast a bottom
// tab tap paints, on the same phone profile as scripts/atlas-perf.mjs.
//
//   bun run perf:chrome <base-url> --mode page [--path /] [options]
//   bun run perf:chrome <base-url> --mode tap [--path /atlas] [--tab Shows] [options]
//
// Target a production build (a Vercel preview). `next dev` does not prefetch
// and serves unminified bundles, so it is not a valid target.
//
// Modes:
//   page  Cold load of --path. Reports FCP, LCP, the main-thread long-task
//         total before LCP + 2 s, and the script bytes that finished by
//         LCP + 2 s.
//   tap   Cold load of --path; when the path is /atlas, waits for the first
//         rendered map (the window.__atlasMap seam), otherwise for the load
//         event; then waits --settle-ms and taps the bottom tab bar's --tab
//         with a real touch sequence: touchstart, --hold-ms, touchend. Playwright's
//         tap() sends both back to back, which would hide anything that starts
//         on touchstart. Reports, from touchend:
//           - pending paint: the first frame that shows the tapped tab's
//             pending state ([data-tab-pending] inside the tab), or "none"
//             on a build without one;
//           - destination paint: the first frame after the URL changes to the
//             tab's destination, i.e. the destination's first committed paint.
//         Also reports whether the destination's route was requested before
//         touchstart (a prefetch the tap could use).
//
// Options:
//   --mode page|tap     required
//   --runs N            cold runs (default 3); medians are reported
//   --path P            page path (default / for page, /atlas for tap)
//   --tab LABEL         tab link to tap (default Shows)
//   --hold-ms MS        touchstart to touchend (default 100)
//   --settle-ms MS      wait after the map or load before the tap (default 3000)
//   --headed            headed Chromium (uses the machine's GPU on macOS)
//   --json FILE         write every run to FILE
//
// Profile: atlas-perf.mjs's phone profile, from lib/phone-perf-profile.mjs
// (both scripts import it): Playwright's `iPhone 13` descriptor at 390x844
// (`pointer: coarse` checked per run), CPU 4x on the main thread, Fast 4G;
// cache disabled, a fresh browser per run.
//
// Env: VERCEL_PROTECTION_BYPASS, if set, is sent on one bypass-cookie request
// per run, only to an https URL on this project's Vercel preview host, and is
// redacted from anything the script prints.
//
// Exit codes: 0 every run finished, 2 harness error.
import { writeFileSync } from 'node:fs'
import { chromium } from '@playwright/test'
import { EXIT, errorMessage, median } from './lib/atlas-perf-budget.mjs'
import { CHROMIUM_ARGS, CPU_THROTTLE_RATE, FAST_4G, FAST_4G_LABEL, SCENE_LIST_SELECTOR, isBypassTarget, phoneContextOptions, setBypassCookie } from './lib/phone-perf-profile.mjs'

const READY_TIMEOUT_MS = 90_000
const LCP_WINDOW_AFTER_MS = 2000
// How long after the load event a page run keeps observing before it reads
// LCP: LCP candidates stop once the page is idle. The read then also waits
// until LCP + LCP_WINDOW_AFTER_MS has passed on the page's clock.
const PAGE_OBSERVE_MS = 6000
// Default wait between the first map (or the load event) and the tap.
const TAP_SETTLE_MS = 3000
const TAP_TIMEOUT_MS = 30_000

function usage(message) {
  if (message) console.error(`error: ${message}`)
  console.error('usage: node scripts/chrome-perf.mjs <base-url> --mode page|tap [--runs N] [--path P] [--tab LABEL] [--hold-ms MS] [--settle-ms MS] [--headed] [--json FILE]')
  process.exit(EXIT.HARNESS_ERROR)
}

function parseArgs(argv) {
  const opts = { mode: null, runs: 3, path: null, tab: 'Shows', holdMs: 100, settleMs: TAP_SETTLE_MS, headed: false, json: null }
  const positional = []
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const next = () => {
      const value = argv[++i]
      if (value === undefined) usage(`${arg} needs a value`)
      return value
    }
    if (arg === '--mode') opts.mode = next()
    else if (arg === '--runs') opts.runs = Number(next())
    else if (arg === '--path') opts.path = next()
    else if (arg === '--tab') opts.tab = next()
    else if (arg === '--hold-ms') opts.holdMs = Number(next())
    else if (arg === '--settle-ms') opts.settleMs = Number(next())
    else if (arg === '--headed') opts.headed = true
    else if (arg === '--json') opts.json = next()
    else if (arg.startsWith('--')) usage(`unknown option ${arg}`)
    else positional.push(arg)
  }
  if (!['page', 'tap'].includes(opts.mode)) usage('--mode must be page or tap')
  if (positional.length !== 1) usage('exactly one base URL is required')
  opts.path ??= opts.mode === 'tap' ? '/atlas' : '/'
  let url
  try {
    url = new URL(opts.path, positional[0])
  } catch {
    usage(`not a URL: ${positional[0]}`)
  }
  if (!Number.isInteger(opts.runs) || opts.runs < 1) usage('--runs must be a positive integer')
  if (!(opts.holdMs >= 0) || !(opts.settleMs >= 0)) usage('--hold-ms and --settle-ms must be non-negative')
  return { ...opts, url }
}

const kib = (bytes) => (bytes / 1024).toFixed(0)

// Page-side probes, installed before any page script runs. Everything is
// stamped on the page's own clock (ms since navigation start).
const PROBES = `(() => {
  if (window !== window.top) return
  const perf = (window.__chromePerf = { lcp: null, fcp: null, longTasks: [], touchStart: null, touchEnd: null, pendingPaint: null, destPaint: null })
  try {
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) perf.lcp = e.startTime
    }).observe({ type: 'largest-contentful-paint', buffered: true })
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) if (e.name === 'first-contentful-paint') perf.fcp = e.startTime
    }).observe({ type: 'paint', buffered: true })
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) perf.longTasks.push({ start: e.startTime, duration: e.duration })
    }).observe({ type: 'longtask', buffered: true })
  } catch {}
  // Arms the frame watch for one tap: the first frame showing the tab's
  // pending marker, and the first frame after the URL reaches the destination.
  window.__chromePerfWatchTap = (tabSelector, destPathname) => {
    addEventListener('touchstart', () => { perf.touchStart = performance.now() }, { capture: true, once: true })
    addEventListener('touchend', () => {
      perf.touchEnd = performance.now()
      // performance.now(), not the callback's frame timestamp: that marks the
      // start of the frame, which can precede the touchend handler's stamp.
      const frame = () => {
        const now = performance.now()
        const tab = document.querySelector(tabSelector)
        if (perf.pendingPaint === null && tab && tab.querySelector('[data-tab-pending]')) perf.pendingPaint = now
        if (perf.destPaint === null && location.pathname === destPathname) perf.destPaint = now
        if (perf.destPaint === null) requestAnimationFrame(frame)
      }
      requestAnimationFrame(frame)
    }, { capture: true, once: true })
  }
})()`

async function openPage(browser, opts) {
  const context = await browser.newContext(phoneContextOptions())
  await setBypassCookie(context, opts.url)
  await context.addInitScript(PROBES)
  const page = await context.newPage()
  const cdp = await context.newCDPSession(page)
  await cdp.send('Network.enable')
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: true })
  await cdp.send('Network.emulateNetworkConditions', FAST_4G)
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: CPU_THROTTLE_RATE })

  // One entry per resource type + URL, the largest report kept, each stamped
  // with when it started and finished (epoch ms).
  const byKey = new Map()
  const pending = new Set()
  // Every request issued, finished or not, with its start time.
  const started = new Map()
  context.on('request', (request) => started.set(request, Date.now()))
  context.on('requestfinished', async (request) => {
    const url = request.url()
    if (url.startsWith('data:')) return
    const at = Date.now()
    const lookup = request.sizes()
    pending.add(lookup)
    try {
      const sizes = await lookup
      const entry = { url, type: request.resourceType(), bytes: sizes.responseBodySize + sizes.responseHeadersSize, at, startedAt: started.get(request) ?? at }
      const key = `${entry.type} ${url}`
      const previous = byKey.get(key)
      if (!previous || entry.bytes > previous.bytes) byKey.set(key, entry)
    } catch {
      // A request torn down with its page has no sizes to report.
    }
  })
  const requests = async () => {
    await Promise.allSettled([...pending])
    return [...byKey.values()]
  }
  const issued = () => [...started].map(([request, at]) => ({ url: request.url(), at }))

  const response = await page.goto(opts.url.href, { waitUntil: 'commit', timeout: READY_TIMEOUT_MS })
  const landedHost = new URL(page.url()).host
  if (landedHost !== opts.url.host || (response && response.status() >= 400)) {
    const status = response ? response.status() : 'no response'
    await context.close()
    throw new Error(`the page did not load the target (HTTP ${status}, landed on ${landedHost}); a protected deployment needs VERCEL_PROTECTION_BYPASS and a preview host`)
  }
  const coarse = await page.evaluate(() => matchMedia('(pointer: coarse)').matches)
  if (!coarse) {
    await context.close()
    throw new Error('the phone profile does not report (pointer: coarse)')
  }
  return { context, page, cdp, requests, issued }
}

async function pageRun(browser, opts) {
  const { context, page, requests } = await openPage(browser, opts)
  await page.waitForLoadState('load', { timeout: READY_TIMEOUT_MS })
  await page.waitForTimeout(PAGE_OBSERVE_MS)
  const windowPassed = await page
    .waitForFunction(
      (afterMs) => window.__chromePerf.lcp !== null && performance.now() >= window.__chromePerf.lcp + afterMs,
      LCP_WINDOW_AFTER_MS,
      { timeout: READY_TIMEOUT_MS, polling: 100 }
    )
    .then(() => true, () => false)
  const { perf, timeOrigin } = await page.evaluate(() => ({ perf: window.__chromePerf, timeOrigin: performance.timeOrigin }))
  if (!windowPassed) {
    await context.close()
    throw new Error(perf.lcp === null ? 'no largest-contentful-paint entry' : 'LCP + 2 s never passed')
  }
  const cut = perf.lcp + LCP_WINDOW_AFTER_MS
  const finished = await requests()
  const scriptBytes = finished
    .filter((r) => r.type === 'script' && r.at - timeOrigin <= cut)
    .reduce((sum, r) => sum + r.bytes, 0)
  const longTaskMs = perf.longTasks.filter((t) => t.start < cut).reduce((sum, t) => sum + t.duration, 0)
  await context.close()
  return { fcpMs: Math.round(perf.fcp ?? NaN), lcpMs: Math.round(perf.lcp), longTaskMs: Math.round(longTaskMs), scriptBytes }
}

async function tapRun(browser, opts) {
  const { context, page, cdp, requests, issued } = await openPage(browser, opts)
  if (opts.url.pathname === '/atlas') {
    // Resolves 'map' once the map has rendered, or 'scene-list' when the page
    // shows the scene list in its place.
    const ready = await page
      .waitForFunction((sceneListSelector) => {
        const m = window.__atlasMap
        if (!m && document.querySelector(sceneListSelector)) return 'scene-list'
        try {
          return !!m && m.isStyleLoaded() && m.areTilesLoaded() && m.queryRenderedFeatures().length > 0 && 'map'
        } catch {
          return false
        }
      }, SCENE_LIST_SELECTOR, { timeout: READY_TIMEOUT_MS, polling: 50 })
      .then((handle) => handle.jsonValue(), () => null)
    if (ready === 'scene-list') {
      await context.close()
      throw new Error('the page rendered the Atlas scene list, not the map')
    }
    if (ready !== 'map') {
      await context.close()
      throw new Error('the Atlas map never became ready')
    }
  } else {
    await page.waitForLoadState('load', { timeout: READY_TIMEOUT_MS })
  }
  await page.waitForTimeout(opts.settleMs)

  const tab = page.getByRole('navigation', { name: 'Mobile navigation' }).getByRole('link', { name: opts.tab, exact: true })
  const href = await tab.getAttribute('href')
  if (!href) {
    await context.close()
    throw new Error(`no tab link named ${opts.tab}`)
  }
  const destPathname = new URL(href, opts.url).pathname
  if (destPathname === new URL(page.url()).pathname) {
    await context.close()
    throw new Error(`the ${opts.tab} tab links to the page under test (${destPathname}); pick another --tab`)
  }
  const box = await tab.boundingBox()
  const tabSelector = `nav[aria-label="Mobile navigation"] a[href="${href}"]`
  await page.evaluate(([selector, dest]) => window.__chromePerfWatchTap(selector, dest), [tabSelector, destPathname])

  const x = box.x + box.width / 2
  const y = box.y + box.height / 2
  const touchStartAt = Date.now()
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] })
  await page.waitForTimeout(opts.holdMs)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })

  const done = await page
    .waitForFunction(() => window.__chromePerf.destPaint !== null, null, { timeout: TAP_TIMEOUT_MS, polling: 50 })
    .then(() => true, () => false)
  const perf = await page.evaluate(() => window.__chromePerf)
  await context.close()
  if (!done || perf.touchEnd === null) throw new Error(`the tap on ${opts.tab} never reached ${destPathname}`)

  const destRequested = (r) => {
    const u = new URL(r.url)
    return u.host === opts.url.host && u.pathname === destPathname && u.searchParams.has('_rsc')
  }
  const finished = await requests()
  return {
    holdMs: Math.round(perf.touchEnd - perf.touchStart),
    pendingPaintMs: perf.pendingPaint === null ? null : Math.round(perf.pendingPaint - perf.touchEnd),
    destPaintMs: Math.round(perf.destPaint - perf.touchEnd),
    destRequestedBeforeTouch: issued().some((r) => r.at < touchStartAt && destRequested(r)),
    jsBytesBeforeTouch: finished.filter((r) => r.type === 'script' && r.at < touchStartAt).reduce((sum, r) => sum + r.bytes, 0),
    // The requests that started from touchstart on, in ms from touchstart:
    // what the tap itself fetched (the --json report only).
    afterTouch: finished
      .filter((r) => r.startedAt >= touchStartAt)
      .sort((a, b) => a.startedAt - b.startedAt)
      .map((r) => ({ url: r.url, type: r.type, bytes: r.bytes, startMs: r.startedAt - touchStartAt, endMs: r.at - touchStartAt })),
  }
}

function printReport(opts, runs) {
  console.log(`\nChrome perf (${opts.mode}): ${opts.url.href}\n`)
  const { viewport, deviceScaleFactor } = phoneContextOptions()
  console.log(`- profile: iPhone 13 ${viewport.width}x${viewport.height} DPR ${deviceScaleFactor}, pointer coarse, CPU ${CPU_THROTTLE_RATE}x main thread, ${FAST_4G_LABEL}, cache disabled, ${opts.headed ? 'headed' : 'headless'}`)
  if (opts.mode === 'page') {
    console.log(`- runs (FCP / LCP / long tasks before LCP+2s / script by LCP+2s): ${runs.map((r) => `${r.fcpMs} / ${r.lcpMs} / ${r.longTaskMs} ms / ${kib(r.scriptBytes)} KiB`).join(', ')}`)
    console.log(`- median: FCP ${median(runs.map((r) => r.fcpMs))} ms, LCP ${median(runs.map((r) => r.lcpMs))} ms, long tasks ${median(runs.map((r) => r.longTaskMs))} ms, script by LCP+2s ${kib(median(runs.map((r) => r.scriptBytes)))} KiB`)
  } else {
    console.log(`- tap: ${opts.tab}, touchstart to touchend ${opts.holdMs} ms, ${opts.settleMs} ms after ${opts.url.pathname === '/atlas' ? 'first map' : 'load'}`)
    console.log(`- runs (pending paint / destination paint from touchend; destination prefetched before touch; JS before touch): ${runs.map((r) => `${r.pendingPaintMs ?? 'none'} / ${r.destPaintMs} ms; ${r.destRequestedBeforeTouch ? 'yes' : 'no'}; ${kib(r.jsBytesBeforeTouch)} KiB`).join(', ')}`)
    const pendings = runs.map((r) => r.pendingPaintMs).filter((v) => v !== null)
    let pendingMedian = 'none'
    if (pendings.length) pendingMedian = `${median(pendings)} ms`
    if (pendings.length && pendings.length < runs.length) pendingMedian += ` (${pendings.length} of ${runs.length} runs)`
    console.log(`- median: pending paint ${pendingMedian}, destination paint ${median(runs.map((r) => r.destPaintMs))} ms`)
  }
}

const opts = parseArgs(process.argv.slice(2))
const bypass = process.env.VERCEL_PROTECTION_BYPASS
if (bypass && !isBypassTarget(opts.url)) {
  console.error(`note: VERCEL_PROTECTION_BYPASS is set but not sent: ${opts.url.host} is not an https preview host`)
}
const redact = (message) => (bypass ? message.replaceAll(bypass, '[redacted]') : message)
const runs = []
try {
  for (let i = 0; i < opts.runs; i++) {
    // A fresh browser per run, so nothing cached survives between runs.
    const browser = await chromium.launch({
      headless: !opts.headed,
      args: CHROMIUM_ARGS,
    })
    try {
      runs.push(await (opts.mode === 'page' ? pageRun(browser, opts) : tapRun(browser, opts)))
    } finally {
      await browser.close().catch(() => {})
    }
  }
} catch (error) {
  console.error(`harness error: ${redact(errorMessage(error))}`)
  process.exit(EXIT.HARNESS_ERROR)
}
printReport(opts, runs)
if (opts.json) writeFileSync(opts.json, JSON.stringify({ url: opts.url.href, mode: opts.mode, runs }, null, 2))
process.exit(EXIT.PASS)
