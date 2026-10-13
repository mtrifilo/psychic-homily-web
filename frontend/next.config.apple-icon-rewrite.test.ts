import { describe, it, expect } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import nextConfigForPhase from './next.config'

/**
 * The conventional root touch-icon paths resolve to the file Next serves from
 * app/apple-icon.png. That the rewrite answers 200 image/png over HTTP is
 * asserted by e2e/pages/app-icons.spec.ts; this pins the config entries.
 */
describe('the apple touch icon rewrites', () => {
  it('maps both conventional root paths to /apple-icon.png', async () => {
    const rewrites = await nextConfigForPhase(PHASE_DEVELOPMENT_SERVER).rewrites!()
    expect(Array.isArray(rewrites)).toBe(true)
    const touchIconRules = (rewrites as { source: string; destination: string }[]).filter(rule =>
      rule.source.startsWith('/apple-touch-icon')
    )
    expect(touchIconRules).toEqual([
      { source: '/apple-touch-icon.png', destination: '/apple-icon.png' },
      { source: '/apple-touch-icon-precomposed.png', destination: '/apple-icon.png' },
    ])
  })
})
