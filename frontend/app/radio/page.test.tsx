import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HydrationBoundary, hashKey } from '@tanstack/react-query'
import {
  boundaryStateOf as stateOf,
  registeredQueryHashes,
  serverRenderFirstScreen as serverRender,
} from '@/test/firstScreenRender'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// The browser requests a mounted hub issues on a cold cache. They never
// answer, so every query stays on its first request.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

// `connection()` marks a request-time render and has nothing to do outside
// Next; the spy records that the page asks for one.
const { connection } = vi.hoisted(() => ({
  connection: vi.fn(async () => {}),
}))
vi.mock('next/server', () => ({ connection }))

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
  usePathname: () => '/radio',
}))

import RadioPage, { HydratedRadioHub } from './page'
import RadioHub from './_components/RadioHub'
import {
  RADIO_DIAL_FIRST_SCREEN_KEY,
  RADIO_DIAL_FIRST_SCREEN_URL,
} from '@/features/radio/dialFirstScreen'

/** The hub's own dial loader, as distinct from each strip's on-air loader. */
const DIAL_LOADER = 'Loading stations'

const stationsPayload = {
  stations: [
    {
      id: 1,
      name: 'KEXP',
      slug: 'kexp',
      city: 'Seattle',
      state: 'WA',
      country: 'US',
      broadcast_type: 'both',
      frequency_mhz: 90.3,
      logo_url: null,
      is_active: true,
      network: null,
      sibling_stations: [],
      show_count: 6,
    },
    {
      id: 4,
      name: 'Give the Drummer Radio',
      slug: 'wfmu-drummer',
      city: 'Jersey City',
      state: 'NJ',
      country: 'US',
      broadcast_type: 'internet',
      frequency_mhz: null,
      logo_url: null,
      is_active: true,
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
      sibling_stations: [
        {
          id: 2,
          slug: 'wfmu',
          name: 'WFMU',
          broadcast_type: 'both',
          frequency_mhz: 91.1,
          is_flagship: true,
        },
      ],
      show_count: 1,
    },
    {
      id: 2,
      name: 'WFMU',
      slug: 'wfmu',
      city: 'Jersey City',
      state: 'NJ',
      country: 'US',
      broadcast_type: 'both',
      frequency_mhz: 91.1,
      logo_url: null,
      is_active: true,
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
      sibling_stations: [
        {
          id: 4,
          slug: 'wfmu-drummer',
          name: 'Give the Drummer Radio',
          broadcast_type: 'internet',
          frequency_mhz: null,
          is_flagship: false,
        },
      ],
      show_count: 4,
    },
  ],
  count: 3,
}

describe('/radio server-seeded dial', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockResolvedValue(stationsPayload)
  })

  it('reads the paired stations URL at request time', async () => {
    await HydratedRadioHub()

    expect(connection).toHaveBeenCalled()
    expect(fetchListPayload).toHaveBeenCalledTimes(1)
    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: RADIO_DIAL_FIRST_SCREEN_URL,
        collection: 'stations',
      })
    )
    expect(connection.mock.invocationCallOrder[0]).toBeLessThan(
      fetchListPayload.mock.invocationCallOrder[0]
    )
  })

  it('seeds exactly the stations key, stamped stale', async () => {
    const state = stateOf(await HydratedRadioHub())

    expect(state.queries.map(q => q.queryKey)).toEqual([
      RADIO_DIAL_FIRST_SCREEN_KEY,
    ])
    expect(state.queries[0].state.dataUpdatedAt).toBe(0)
  })

  // The real hub and strips over the seed: a key the hook does not register
  // would leave the dial on its loader.
  it('server-renders a strip per dial station, each linking its station page', async () => {
    const html = serverRender(await HydratedRadioHub())

    expect(html).not.toContain(DIAL_LOADER)
    expect(html).toContain('href="/radio/wfmu"')
    expect(html).toContain('href="/radio/kexp"')
    // A network channel is a sub-row under its flagship, not a strip.
    expect(html).toContain('href="/radio/wfmu/channel/drummer"')
    expect(html.match(/<article/g)).toHaveLength(2)
    // The strips' own reads are not seeded: each waits on its on-air loader.
    expect(html.match(/Loading on-air info/g)).toHaveLength(2)
    expect(html.indexOf('href="/radio/wfmu"')).toBeLessThan(
      html.indexOf('href="/radio/kexp"')
    )
  })

  it('renders the hub with its dial loader when the read fails, rather than throwing', async () => {
    fetchListPayload.mockResolvedValue(null)

    const boundary = await HydratedRadioHub()

    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
    const html = serverRender(boundary)
    expect(html).toContain(DIAL_LOADER)
    expect(html).toContain('>Radio</h1>')
  })

  it('renders the unseeded hub as the boundary fallback', () => {
    const html = serverRender(RadioPage())

    expect(html).toContain('>Radio</h1>')
    expect(html).toContain(DIAL_LOADER)
    expect(fetchListPayload).not.toHaveBeenCalled()
  })
})

/**
 * The client half of the pair: mounted on a cold cache, the real hub requests
 * the URL the server reads and registers the key the server seeds.
 */
describe('/radio dial request pairing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  })

  it('RadioHub requests the stations URL and keys on the stations key', () => {
    const hashes = registeredQueryHashes(<RadioHub />)

    expect(hashes).toContain(hashKey(RADIO_DIAL_FIRST_SCREEN_KEY))
    expect(apiRequest.mock.calls.map(([url]) => url)).toContain(
      RADIO_DIAL_FIRST_SCREEN_URL
    )
  })
})
