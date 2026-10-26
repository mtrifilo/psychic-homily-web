import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { HydrationBoundary, hashKey } from '@tanstack/react-query'
import {
  LOADER,
  boundaryStateOf as stateOf,
  RecordingSearchParams,
  registeredQueryHashes,
  serverRenderFirstScreen as serverRender,
} from '@/test/firstScreenRender'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// The browser request a mounted list issues on a cold cache. It never
// answers, so the list stays on its first request.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

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

// The URL a mounted list reads. Reset per test; a test may swap in params.
const nav = vi.hoisted(() => ({ params: new URLSearchParams() }))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => nav.params,
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
import { ReleaseList } from '@/features/releases/components/ReleaseList'
import {
  RELEASE_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  RELEASE_LIST_FIRST_SCREEN_KEY,
  RELEASE_LIST_FIRST_SCREEN_URL,
} from '@/features/releases/listFirstScreen'


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

/**
 * The client half of the pair: mounted on a cold cache, the real list requests
 * the URL the server reads and registers the key the server seeds. A drift in
 * either leaves the seed unread or seeds the wrong payload.
 */
describe('/releases first-screen request pairing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
    nav.params = new URLSearchParams()
  })

  it('ReleaseList requests the first-screen URL and keys on the first-screen key', () => {
    const hashes = registeredQueryHashes(<ReleaseList />)

    expect(hashes).toContain(hashKey(RELEASE_LIST_FIRST_SCREEN_KEY))
    expect(apiRequest.mock.calls.map(([url]) => url)).toContain(
      RELEASE_LIST_FIRST_SCREEN_URL
    )
  })

  // A URL param the list reads but the seed gate does not skip on would seed a
  // key the list never asks for on that URL.
  it('ReleaseList reads no URL param its seed gate does not skip on', () => {
    const params = new RecordingSearchParams()
    nav.params = params

    registeredQueryHashes(<ReleaseList />)

    expect(params.read.size).toBeGreaterThan(0)
    expect([...params.read].filter(
      name => !(RELEASE_LIST_FIRST_SCREEN_DEFEATING_PARAMS as readonly string[]).includes(name)
    )).toEqual([])
  })

  // The seed gate reads an empty value as absent, so the list has to as well.
  it.each([['type'], ['sort']])(
    'ReleaseList keys an empty ?%s= on the first-screen key',
    key => {
      nav.params = new URLSearchParams({ [key]: '' })

      expect(registeredQueryHashes(<ReleaseList />)).toContain(
        hashKey(RELEASE_LIST_FIRST_SCREEN_KEY)
      )
    }
  )
})
