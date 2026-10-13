import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { test, expect } from '@playwright/test'

/**
 * The install surfaces over HTTP, with no browser page: the touch icon (at its
 * convention path and through the root-path rewrites), the manifest, the icons
 * it lists, and the head links on a static page.
 */
const APPLE_ICON_BYTES = readFileSync(join(__dirname, '..', '..', 'app', 'apple-icon.png'))

// @smoke: request-only and fast, and these are the only HTTP-level checks on
// the touch-icon rewrites, so they run on every PR rather than after merge.
test.describe('app icons and manifest', { tag: '@smoke' }, () => {
  for (const path of ['/apple-icon.png', '/apple-touch-icon.png', '/apple-touch-icon-precomposed.png']) {
    test(`${path} answers 200 image/png with the apple-icon artwork`, async ({ request }) => {
      const response = await request.get(path, { maxRedirects: 0 })
      expect(response.status()).toBe(200)
      expect(response.headers()['content-type']).toBe('image/png')
      expect(Buffer.compare(await response.body(), APPLE_ICON_BYTES)).toBe(0)
    })
  }

  test('the manifest is served and every icon it lists answers 200 image/png', async ({ request }) => {
    const response = await request.get('/manifest.webmanifest')
    expect(response.status()).toBe(200)
    expect(response.headers()['content-type']).toContain('application/manifest+json')
    const manifest = (await response.json()) as { icons: { src: string }[] }
    const iconPaths = [...new Set(manifest.icons.map(icon => icon.src))]
    expect(iconPaths.length).toBeGreaterThan(0)
    const iconResponses = await Promise.all(iconPaths.map(src => request.get(src)))
    iconResponses.forEach((iconResponse, index) => {
      expect(iconResponse.status(), iconPaths[index]).toBe(200)
      expect(iconResponse.headers()['content-type'], iconPaths[index]).toBe('image/png')
    })
  })

  test('the root layout links the manifest and the touch icon', async ({ request }) => {
    const html = await (await request.get('/privacy')).text()
    expect(html).toMatch(/<link rel="manifest" href="\/manifest\.webmanifest"/)
    expect(html).toMatch(/<link rel="apple-touch-icon" href="\/apple-icon\.png[?"]/)
  })
})
