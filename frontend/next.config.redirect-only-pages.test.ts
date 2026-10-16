import { describe, it, expect } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import nextConfigForPhase from './next.config'
import { ALERTS_HREF } from './components/shared/followAlertChoices'

/**
 * `/explore` and `/settings/notifications` have no page: these rules are the
 * only thing that answers them. The status, the `Location` header and the
 * landing are asserted in `e2e/pages/graph.spec.ts` and
 * `e2e/pages/settings-hub.spec.ts`, which are outside the PR smoke subset.
 */
async function rulesFor(source: string) {
  const redirects = await nextConfigForPhase(
    PHASE_DEVELOPMENT_SERVER
  ).redirects!()
  return redirects.filter(entry => entry.source === source)
}

describe('the /explore redirect', () => {
  it('is a permanent config redirect to /graph', async () => {
    expect(await rulesFor('/explore')).toEqual([
      { source: '/explore', destination: '/graph', permanent: true },
    ])
  })
})

describe('the /settings/notifications redirect', () => {
  // Every artist and venue show-alert email links this path as its "manage"
  // CTA, so where it lands is a product contract.
  it('is a temporary config redirect to the Alerts card', async () => {
    expect(await rulesFor('/settings/notifications')).toEqual([
      {
        source: '/settings/notifications',
        destination: ALERTS_HREF,
        permanent: false,
      },
    ])
  })
})
