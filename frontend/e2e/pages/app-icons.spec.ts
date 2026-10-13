import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { test, expect } from '@playwright/test'

/**
 * The install surfaces over HTTP: the touch icon (at its convention path and
 * through the root-path rewrites), the manifest, and the icons the manifest
 * lists. Request-only, so no page renders and no backend data is read.
 */
const APPLE_ICON_BYTES = readFileSync(join(__dirname, '..', '..', 'app', 'apple-icon.png'))

test.describe('app icons and manifest', () => {
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
    expect(manifest.icons.length).toBeGreaterThan(0)
    for (const icon of manifest.icons) {
      const iconResponse = await request.get(icon.src)
      expect(iconResponse.status(), icon.src).toBe(200)
      expect(iconResponse.headers()['content-type'], icon.src).toBe('image/png')
    }
  })

  test('the home page links the manifest and the touch icon', async ({ request }) => {
    const html = await (await request.get('/')).text()
    expect(html).toMatch(/<link rel="manifest" href="\/manifest\.webmanifest"/)
    expect(html).toMatch(/<link rel="apple-touch-icon" href="\/apple-icon\.png[?"]/)
  })
})
