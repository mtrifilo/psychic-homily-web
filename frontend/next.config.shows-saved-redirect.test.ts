import { describe, it, expect } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import nextConfigForPhase from './next.config'

/**
 * `/shows/saved` has no page and no `RESERVED_SEGMENTS` entry: this rule is the
 * only thing that answers it. The 308 and the carried query string are asserted
 * in `e2e/pages/not-found.spec.ts`, which is outside the PR smoke subset.
 */
describe('the /shows/saved redirect', () => {
  it('is a permanent config redirect to /library', async () => {
    const redirects = await nextConfigForPhase(
      PHASE_DEVELOPMENT_SERVER
    ).redirects!()

    expect(redirects).toContainEqual({
      source: '/shows/saved',
      destination: '/library',
      permanent: true,
    })
  })
})
