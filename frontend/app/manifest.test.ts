import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { readImageHeader } from '@/lib/og/imageSize'
import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import { PAGE_BACKGROUND } from '@/lib/seo/viewport'

import manifest from './manifest'

// The file a manifest icon URL is served from: /icon.png is the app/icon.png
// metadata route, everything else is a public/ file.
function iconFile(src: string): string {
  return src === '/icon.png' ? 'app/icon.png' : `public${src}`
}

function pngSize(path: string): string | null {
  const header = readImageHeader(readFileSync(resolve(process.cwd(), path)))
  return header?.mime === 'image/png' ? `${header.width}x${header.height}` : null
}

describe('manifest', () => {
  const m = manifest()

  it('declares the identity Chrome and Safari read at install time', () => {
    expect(m).toMatchObject({
      id: '/',
      name: 'Psychic Homily',
      short_name: 'Psychic Homily',
      description: SITE_DESCRIPTION,
      start_url: '/',
      display: 'standalone',
    })
  })

  it('uses the dark page background for both colours', () => {
    expect(m.background_color).toBe(PAGE_BACKGROUND.dark)
    expect(m.theme_color).toBe(PAGE_BACKGROUND.dark)
  })

  it('lists a 192 and a 512 icon for any purpose plus a maskable 512', () => {
    expect(m.icons?.map(icon => `${icon.sizes} ${icon.purpose}`)).toEqual([
      '192x192 any',
      '512x512 any',
      '512x512 maskable',
    ])
  })

  it('points every icon at a PNG whose pixel size matches its declared sizes', () => {
    for (const icon of m.icons ?? []) {
      expect(pngSize(iconFile(icon.src)), icon.src).toBe(icon.sizes)
    }
  })

  // Launchers crop a maskable icon to their own shape; only the centred circle
  // of radius 40% is guaranteed to survive. Checked on the committed bytes, so
  // a hand-replaced file fails here even if scripts/app-icons.mjs never ran.
  it('keeps every non-black pixel of each maskable icon inside the safe zone', async () => {
    const { default: sharp } = await import('sharp')
    const maskable = (m.icons ?? []).filter(icon => icon.purpose === 'maskable')
    expect(maskable.length).toBeGreaterThan(0)
    for (const icon of maskable) {
      const { data, info } = await sharp(resolve(process.cwd(), iconFile(icon.src)))
        .flatten({ background: '#000000' })
        .raw()
        .toBuffer({ resolveWithObject: true })
      const centre = info.width / 2
      let farthest = 0
      for (let y = 0; y < info.height; y++) {
        for (let x = 0; x < info.width; x++) {
          const i = (y * info.width + x) * info.channels
          if (data[i] || data[i + 1] || data[i + 2]) {
            farthest = Math.max(farthest, Math.hypot(x + 0.5 - centre, y + 0.5 - centre))
          }
        }
      }
      expect(farthest / info.width, icon.src).toBeLessThanOrEqual(0.4)
    }
  })
})

describe('app icon files', () => {
  it('keeps the touch icon at the apple-icon convention path, 180x180', () => {
    expect(pngSize('app/apple-icon.png')).toBe('180x180')
  })
})
