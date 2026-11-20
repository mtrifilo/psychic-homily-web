import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { HydrationBoundary, hashKey } from '@tanstack/react-query'
import {
  LOADER,
  boundaryStateOf as stateOf,
  RecordingSearchParams,
  registeredQueryHashes,
  serverRenderFirstScreen as serverRender,
  suspenseFallbackOf,
} from '@/test/firstScreenRender'
import { describeCachedListFallback } from '@/test/cachedListFallbackSuite'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// The browser request a mounted list issues on a cold cache. It never
// answers, so the list stays on its first request.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

// The seed path awaits `connection()`, which marks a request-time render and
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
  usePathname: () => '/festivals',
}))

import FestivalsPage, { HydratedFestivalList } from './page'
import { FestivalList } from '@/features/festivals/components'
import {
  FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  FESTIVAL_LIST_FIRST_SCREEN_KEY,
  FESTIVAL_LIST_FIRST_SCREEN_URL,
} from '@/features/festivals/listFirstScreen'
import { festivalQueryKeys } from '@/features/festivals/api'

const festivalsPayload = {
  festivals: [
    {
      id: 1,
      name: 'FORM Arcosanti',
      slug: 'form-arcosanti-2026',
      series_slug: 'form-arcosanti',
      edition_year: 2026,
      city: 'Mayer',
      state: 'AZ',
      start_date: '2026-05-08',
      end_date: '2026-05-10',
      status: 'confirmed',
      artist_count: 30,
      venue_count: 1,
    },
    {
      id: 2,
      name: 'Desert Daze',
      slug: 'desert-daze-2026',
      series_slug: 'desert-daze',
      edition_year: 2026,
      city: 'Lake Perris',
      state: 'CA',
      start_date: '2026-10-09',
      end_date: '2026-10-11',
      status: 'announced',
      artist_count: 60,
      venue_count: 1,
    },
  ],
  count: 2,
}

const renderBoundary = (
  params: Record<string, string | string[] | undefined> = {}
) => HydratedFestivalList({ searchParams: Promise.resolve(params) })

describe('/festivals server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockResolvedValue(festivalsPayload)
  })

  it('reads the paired first-screen URL', async () => {
    await renderBoundary()

    expect(fetchListPayload).toHaveBeenCalledTimes(1)
    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: FESTIVAL_LIST_FIRST_SCREEN_URL,
        collection: 'festivals',
      })
    )
  })

  it('seeds exactly the first-screen key, stamped stale', async () => {
    const state = stateOf(await renderBoundary())

    expect(state.queries.map(q => q.queryKey)).toEqual([
      FESTIVAL_LIST_FIRST_SCREEN_KEY,
    ])
    expect(state.queries[0].state.dataUpdatedAt).toBe(0)
  })

  // The real list over the seed: a key the hook does not register would leave
  // this on the loader.
  it('server-renders the festivals as links, undimmed, with no loader', async () => {
    const html = serverRender(await renderBoundary())

    expect(html).toContain('href="/festivals/form-arcosanti-2026"')
    expect(html).toContain('href="/festivals/desert-daze-2026"')
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
    ['status', 'confirmed'],
    ['year', '2026'],
    ['city', 'Phoenix'],
    ['tags', 'psych'],
    ['tag_match', 'any'],
  ])('skips the read when ?%s= is present', async (key, value) => {
    const boundary = await renderBoundary({ [key]: value })

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
  })

  it('renders the heading and the boundary fallback in the page body', () => {
    const html = renderToString(
      FestivalsPage({ searchParams: Promise.resolve({}) })
    )

    expect(html).toContain('>Festivals</h1>')
    expect(html).toContain(LOADER)
  })
})

/**
 * The client half of the pair: mounted on a cold cache, the real list requests
 * the URL the server reads and registers the key the server seeds. A drift in
 * either leaves the seed unread or seeds the wrong payload.
 */
describe('/festivals first-screen request pairing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
    nav.params = new URLSearchParams()
  })

  it('FestivalList requests the first-screen URL and keys on the first-screen key', () => {
    const hashes = registeredQueryHashes(<FestivalList />)

    expect(hashes).toContain(hashKey(FESTIVAL_LIST_FIRST_SCREEN_KEY))
    expect(apiRequest.mock.calls.map(([url]) => url)).toContain(
      FESTIVAL_LIST_FIRST_SCREEN_URL
    )
  })

  // A URL param the list reads but the seed gate does not skip on would seed a
  // key the list never asks for on that URL.
  it('FestivalList reads no URL param its seed gate does not skip on', () => {
    const params = new RecordingSearchParams()
    nav.params = params

    registeredQueryHashes(<FestivalList />)

    expect(params.read.size).toBeGreaterThan(0)
    expect([...params.read].filter(
      name => !(FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS as readonly string[]).includes(name)
    )).toEqual([])
  })
})

describeCachedListFallback({
  route: '/festivals',
  fallback: () =>
    suspenseFallbackOf(FestivalsPage({ searchParams: Promise.resolve({}) })),
  seededBoundary: () => {
    fetchListPayload.mockResolvedValue(festivalsPayload)
    return renderBoundary()
  },
  list: FestivalList,
  seeded: [[FESTIVAL_LIST_FIRST_SCREEN_KEY, festivalsPayload]],
  defeatingParams: FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  seededRow: 'href="/festivals/form-arcosanti-2026"',
  loaders: [LOADER],
  otherQuery: {
    entry: [
      festivalQueryKeys.list({
        status: 'cancelled',
        year: undefined,
        city: undefined,
        tags: undefined,
        tagMatch: 'all',
      }),
      {
        festivals: [
          {
            ...festivalsPayload.festivals[0],
            id: 9,
            name: 'Called Off Fest',
            slug: 'called-off-fest-2026',
            status: 'cancelled',
          },
        ],
        count: 1,
      },
    ],
    row: 'href="/festivals/called-off-fest-2026"',
    search: 'status=cancelled',
  },
  nav,
  apiRequest,
})
