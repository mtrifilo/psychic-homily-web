import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { pageBackgroundTokens } from '@/lib/seo/pageBackgroundTokens.testing'
import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'

import manifest from './manifest'

// Width and height from a PNG's IHDR chunk, which starts at byte 16.
function pngSize(path: string): string {
  const bytes = readFileSync(path)
  expect(bytes.subarray(1, 4).toString('ascii')).toBe('PNG')
  return `${bytes.readUInt32BE(16)}x${bytes.readUInt32BE(20)}`
}

describe('manifest', () => {
  const m = manifest()

  it('declares the identity Chrome and Safari read at install time', () => {
    expect(m.id).toBe('/')
    expect(m.name).toBe('Psychic Homily')
    expect(m.short_name).toBe('Psychic Homily')
    expect(m.description).toBe(SITE_DESCRIPTION)
    expect(m.start_url).toBe('/')
    expect(m.display).toBe('standalone')
  })

  it('takes both colours from the dark page background token', () => {
    const { dark } = pageBackgroundTokens()
    expect(m.background_color).toBe(dark)
    expect(m.theme_color).toBe(dark)
    expect(dark).toBe('#0d0805')
  })

  it('lists a 192 and a 512 icon for any purpose plus a maskable 512', () => {
    expect(m.icons).toEqual([
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ])
  })

  it('points every icon at a public file whose pixel size matches its declared sizes', () => {
    for (const icon of m.icons ?? []) {
      expect(pngSize(join(__dirname, '..', 'public', icon.src))).toBe(icon.sizes)
    }
  })
})

describe('app icon files', () => {
  it('keeps the touch icon at the apple-icon convention path, 180x180', () => {
    expect(pngSize(join(__dirname, 'apple-icon.png'))).toBe('180x180')
  })

  it('keeps the rel=icon PNG at 192x192', () => {
    expect(pngSize(join(__dirname, 'icon.png'))).toBe('192x192')
  })
})
