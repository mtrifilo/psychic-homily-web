import { describe, expect, it } from 'vitest'

import { GET } from './route'
import { FAMILY_SHARD_IDS, PAGES_SHARD_ID } from '../sitemap-shards'

/**
 * The index is the document robots.txt points crawlers at, and its body is
 * derived entirely from compile-time constants. That makes it cheap to pin —
 * and worth pinning, because a shard id added to `sitemap-shards.ts` without
 * appearing here would be a family silently absent from the index.
 */
describe('sitemap-index', () => {
  it('lists every shard, pages first', async () => {
    const body = await (await GET()).text()

    const locs = [...body.matchAll(/<loc>([^<]+)<\/loc>/g)].map(m => m[1])

    expect(locs).toEqual([
      `https://psychichomily.com/sitemap/${PAGES_SHARD_ID}.xml`,
      ...FAMILY_SHARD_IDS.map(
        id => `https://psychichomily.com/sitemap/${id}.xml`
      ),
    ])
  })

  it('serves parseable sitemapindex XML', async () => {
    const res = await GET()
    const body = await res.text()

    expect(res.headers.get('Content-Type')).toBe(
      'application/xml; charset=utf-8'
    )
    expect(body.startsWith('<?xml version="1.0" encoding="UTF-8"?>')).toBe(true)
    expect(body).toContain(
      '<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'
    )
  })

  it('bounds how long a CDN may serve a stale shard list', async () => {
    const cacheControl = (await GET()).headers.get('Cache-Control')

    // Deliberately NOT tied to the shards' revalidate window — this bounds how
    // long a crawler may miss a newly deployed shard, nothing more.
    expect(cacheControl).toBe('public, max-age=0, s-maxage=3600')
  })
})
