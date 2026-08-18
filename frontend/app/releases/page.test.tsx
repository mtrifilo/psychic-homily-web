import { describe, expect, it, vi } from 'vitest'

vi.mock('@/features/releases/components', () => ({ ReleaseList: () => null }))
vi.mock('@/components/shared', () => ({ LoadingSpinner: () => null }))

import { metadata } from './page'

/**
 * A regression guard on a decision, not on a string. PSY-1767 settled that
 * every query variant of `/releases` canonicalizes to `/releases`, so the pager
 * and the filters are crawled without minting competing documents. Removing or
 * loosening this canonical is a policy change, and it should have to break a
 * test to happen. Reasoning: `listRootCanonical` in `lib/seo/siteMetadata`.
 */
describe('releases route canonical (pagination indexing policy)', () => {
  it('pins the list root for every page and filter variant', () => {
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/releases'
    )
  })
})
