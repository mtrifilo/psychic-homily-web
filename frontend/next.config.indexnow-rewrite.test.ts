import { describe, it, expect } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import nextConfigForPhase from './next.config'

/**
 * The IndexNow key file's root address is rewritten to app/indexnow/[key].
 * That the rewrite serves the key over HTTP, and that robots.txt still wins
 * over it, is the PR's manual repro; this pins the config entry.
 */
describe('the IndexNow key file rewrite', () => {
  it('maps /{key}.txt at the root to /indexnow/{key}', async () => {
    const rewrites = await nextConfigForPhase(PHASE_DEVELOPMENT_SERVER).rewrites!()
    expect(Array.isArray(rewrites)).toBe(true)
    const rules = (rewrites as { source: string; destination: string }[]).filter(
      rule => rule.destination.startsWith('/indexnow/')
    )
    expect(rules).toEqual([
      { source: '/:key([A-Za-z0-9-]{8,128}).txt', destination: '/indexnow/:key' },
    ])
  })
})
