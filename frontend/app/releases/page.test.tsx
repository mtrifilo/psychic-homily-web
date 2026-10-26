import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
} from '@tanstack/react-query'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// `seedFirstScreen` awaits `connection()`, which marks a request-time render and
// has nothing to do outside Next.
vi.mock('next/server', () => ({ connection: async () => {} }))

// On the server `getQueryClient()` mints a fresh client per call. Under jsdom it
// would hand back the browser singleton, so the server behaviour is restored.
vi.mock('@/lib/queryClient', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/queryClient')>()
  const { QueryClient } = await import('@tanstack/react-query')
  return { ...actual, getQueryClient: () => new QueryClient() }
})

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/releases',
}))

// The route's HTML is served to every viewer as the signed-out render, and the
// save control reads the viewer.
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  const { ANONYMOUS_VIEWER } = await import('@/test/saveControlHydration')
  return { useAuthContext: () => deriveMockAuthSignals(ANONYMOUS_VIEWER) }
})

import ReleasesPage, { HydratedReleaseList, metadata } from './page'
import {
  RELEASE_LIST_FIRST_SCREEN_KEY,
  RELEASE_LIST_FIRST_SCREEN_URL,
} from '@/features/releases/listFirstScreen'

const LOADER = 'animate-spin'

const release = (id: number, title: string, slug: string) => ({
  id,
  title,
  slug,
  release_type: 'lp',
  release_year: 2007,
  cover_art_url: null,
  artist_count: 1,
  artists: [{ id, name: `Artist ${id}`, slug: `artist-${id}` }],
  label_name: null,
  label_slug: null,
})

const releasesPayload = {
  releases: [
    release(1, 'In Rainbows', 'in-rainbows'),
    release(2, 'Loveless', 'loveless'),
  ],
  total: 2,
  limit: 50,
  offset: 0,
}

const renderBoundary = (
  params: Record<string, string | string[] | undefined> = {}
) => HydratedReleaseList({ searchParams: Promise.resolve(params) })

/**
 * The route's HTML for the list: no client fetch runs inside `renderToString`,
 * so whatever this prints came from the seed.
 */
function serverRender(boundary: ReactElement): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderToString(
    <QueryClientProvider client={client}>{boundary}</QueryClientProvider>
  )
}

const stateOf = (boundary: ReactElement) =>
  (boundary.props as { state: DehydratedState }).state

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

describe('/releases server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockResolvedValue(releasesPayload)
  })

  it('reads the paired first-screen URL', async () => {
    await renderBoundary()

    expect(fetchListPayload).toHaveBeenCalledTimes(1)
    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: RELEASE_LIST_FIRST_SCREEN_URL,
        collection: 'releases',
      })
    )
  })

  it('seeds exactly the first-screen key', async () => {
    const state = stateOf(await renderBoundary())

    expect(state.queries.map(q => q.queryKey)).toEqual([
      RELEASE_LIST_FIRST_SCREEN_KEY,
    ])
  })

  // The real list over the seed: a key the hook does not register would leave
  // this on the loader.
  it('server-renders the first page as links, undimmed, with no loader', async () => {
    const html = serverRender(await renderBoundary())

    expect(html).toContain('href="/releases/in-rainbows"')
    expect(html).toContain('href="/releases/loveless"')
    expect(html).not.toContain(LOADER)
    expect(html).not.toContain('opacity-60')
  })

  it('renders the list with its loader when the read fails, rather than throwing', async () => {
    fetchListPayload.mockResolvedValue(null)

    const boundary = await renderBoundary()

    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
    expect(serverRender(boundary)).toContain(LOADER)
  })

  it.each([
    ['page', '2'],
    ['type', 'lp'],
    ['year', '2007'],
    ['search', 'rainbows'],
    ['sort', 'oldest'],
    ['label_id', '3'],
    ['tags', 'shoegaze'],
    ['tag_match', 'any'],
  ])('skips the read when ?%s= is present', async (key, value) => {
    const boundary = await renderBoundary({ [key]: value })

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
  })

  it('renders the heading and the boundary fallback in the page body', () => {
    const html = renderToString(
      ReleasesPage({ searchParams: Promise.resolve({}) })
    )

    expect(html).toContain('>Releases</h1>')
    expect(html).toContain(LOADER)
  })
})
