import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import robots from './robots'

// public/llms.txt is a static file served at /llms.txt. It names only what the
// site's robots.txt already lets crawlers fetch: the public page families and
// the sitemap index. The API host disallows every agent, so it is never named.
const llmsTxt = readFileSync(resolve(process.cwd(), 'public/llms.txt'), 'utf8')

const urls = [...llmsTxt.matchAll(/https?:\/\/[^\s)]+/g)].map((m) => m[0])

describe('public/llms.txt', () => {
  it('never references the API host', () => {
    expect(llmsTxt).not.toMatch(/api\.psychichomily\.com/i)
    for (const url of urls) {
      expect(new URL(url).hostname).toBe('psychichomily.com')
    }
  })

  it('points at the sitemap index robots.txt advertises', () => {
    const { sitemap } = robots()
    expect(typeof sitemap).toBe('string')
    expect(urls).toContain(sitemap as string)
  })

  it('lists one URL pattern per public page family, each an existing route', () => {
    const families = urls
      .map((url) => new URL(url.replace('{slug}', 'slug')).pathname)
      .filter((path) => path.endsWith('/slug'))
      .map((path) => path.split('/')[1])

    expect(families).toEqual([
      'shows',
      'artists',
      'venues',
      'releases',
      'labels',
      'scenes',
      'festivals',
      'tags',
    ])
    for (const family of families) {
      expect(
        existsSync(resolve(process.cwd(), `app/${family}/[slug]/page.tsx`)),
        `app/${family}/[slug]/page.tsx`,
      ).toBe(true)
    }
  })
})
