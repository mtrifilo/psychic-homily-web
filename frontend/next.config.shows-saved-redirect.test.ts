import { describe, it, expect } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import nextConfigForPhase from './next.config'

/**
 * `/shows/saved` has no page and no `RESERVED_SEGMENTS` entry in `proxy.ts`:
 * this rule is the only thing that answers it. Without it the path reaches the
 * proxy as a show slug and is existence-checked like one. The redirect status
 * and the carried query string are asserted end to end in
 * `e2e/pages/not-found.spec.ts`, which runs in the full suite, not the PR smoke
 * subset; this pins the rule itself in PR CI.
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
