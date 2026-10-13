// The phone measurement profile and the preview-bypass rule shared by
// scripts/atlas-perf.mjs and scripts/chrome-perf.mjs, so the two report on the
// same conditions and send the bypass secret to the same hosts.
import { devices } from '@playwright/test'

// Playwright's `iPhone 13` descriptor (mobile UA, touch, DPR 3, coarse
// pointer) at 390x844 unless the caller passes another viewport.
export function phoneContextOptions(viewport = { width: 390, height: 844 }) {
  return { ...devices['iPhone 13'], reducedMotion: 'no-preference', viewport }
}

// Puppeteer's `Fast 4G` preset: 9 Mbps down and 1.5 Mbps up at 90%
// efficiency, 165 ms latency (60 ms RTT x 2.75), for CDP
// Network.emulateNetworkConditions.
export const FAST_4G = {
  offline: false,
  downloadThroughput: ((9 * 1000 * 1000) / 8) * 0.9,
  uploadThroughput: ((1.5 * 1000 * 1000) / 8) * 0.9,
  latency: 60 * 2.75,
}
export const FAST_4G_LABEL = 'Fast 4G (9 Mbps down, 1.5 Mbps up, 165 ms)'

// CDP Emulation.setCPUThrottlingRate on the page's main thread.
export const CPU_THROTTLE_RATE = 4

// Chromium's software WebGL, so a headless run renders the Atlas map.
export const CHROMIUM_ARGS = ['--use-gl=angle', '--enable-webgl', '--ignore-gpu-blocklist', '--enable-unsafe-swiftshader']

// AtlasSceneList's root carries this test id; /atlas renders it in place of
// the map when the map cannot run.
export const SCENE_LIST_SELECTOR = '[data-testid="atlas-scene-list"]'

// This project's Vercel preview hosts: the only hosts the bypass secret goes to.
export const PREVIEW_HOST = /^psychic-homily-[a-z0-9-]+-matts-projects-722d5204\.vercel\.app$/

/** Whether VERCEL_PROTECTION_BYPASS may be sent to `url` (a URL object). */
export function isBypassTarget(url) {
  return url.protocol === 'https:' && PREVIEW_HOST.test(url.hostname)
}

/**
 * When VERCEL_PROTECTION_BYPASS is set and `url` is a preview host, sends it
 * on one request from `context` (a Playwright BrowserContext), which sets the
 * bypass cookie for the context's page requests. The secret rides only that
 * request, never a redirect, and no error this throws carries it.
 */
export async function setBypassCookie(context, url) {
  const bypass = process.env.VERCEL_PROTECTION_BYPASS
  if (!bypass || !isBypassTarget(url)) return
  let status
  try {
    const response = await context.request.get(url.href, {
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
