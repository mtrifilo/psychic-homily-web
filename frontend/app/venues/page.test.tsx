import { describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'

// The page's only fetch sits in the Suspense subtree, which this suite never
// renders; the stubs keep importing the page free of network and client state.
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/query-hydration', () => ({ seedFirstScreen: vi.fn() }))
vi.mock('@/features/venues', () => ({ VenueList: () => null }))

import VenuesPage from './page'

/** Every JSON-LD payload the page element tree carries, in order. */
function jsonLdPayloads(tree: ReactElement): Array<Record<string, unknown>> {
  const found: Array<Record<string, unknown>> = []
  const walk = (node: unknown): void => {
    if (Array.isArray(node)) {
      node.forEach(walk)
      return
    }
    if (!node || typeof node !== 'object') return
    const props = (node as { props?: Record<string, unknown> }).props
    if (props && typeof props === 'object') {
      if ('data' in props && props.data && typeof props.data === 'object') {
        found.push(props.data as Record<string, unknown>)
      }
      if ('children' in props) walk(props.children)
    }
  }
  walk(tree)
  return found
}

// The directory announces no venue list to crawlers: venue URLs are the
// `venues` sitemap family's job. The breadcrumb stays.
describe('/venues JSON-LD', () => {
  it('emits the breadcrumb and no ItemList', () => {
    const types = jsonLdPayloads(VenuesPage()).map(data => data['@type'])
    expect(types).toEqual(['BreadcrumbList'])
  })
})
