import type { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HydrationBoundary, hashKey } from '@tanstack/react-query'
import {
  boundaryStateOf as stateOf,
  entriesFor,
  registeredQueryHashes,
  renderOnNavigation,
  serverRenderFirstScreen as serverRender,
  warmClient,
} from '@/test/firstScreenRender'
import { describeCachedListFallback } from '@/test/cachedListFallbackSuite'

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

import RadioPage, { HydratedRadioDial } from './page'
import RadioHub, {
  RadioDial,
  RadioDialLoader,
} from './_components/RadioHub'
import { CachedListFallback } from '@/components/shared/CachedListFallback'
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

/** The Suspense element the page puts in the hub's dial slot. */
const pageDialSlot = () =>
  (RadioPage().props as { dial: ReactElement<{ fallback: ReactElement }> }).dial

describe('/radio server-seeded dial', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockResolvedValue(stationsPayload)
  })

  it('reads the paired stations URL at request time', async () => {
    await HydratedRadioDial()

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
    const state = stateOf(await HydratedRadioDial())

    expect(state.queries.map(q => q.queryKey)).toEqual([
      RADIO_DIAL_FIRST_SCREEN_KEY,
    ])
    expect(state.queries[0].state.dataUpdatedAt).toBe(0)
  })

  // The real dial and strips over the seed: a key the hook does not register
  // would leave the dial on its loader.
  it('server-renders a strip per dial station, each linking its station page', async () => {
    const html = serverRender(await HydratedRadioDial())

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

  it('renders the dial with its loader when the read fails, rather than throwing', async () => {
    fetchListPayload.mockResolvedValue(null)

    const boundary = await HydratedRadioDial()

    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
    expect(serverRender(boundary)).toContain(DIAL_LOADER)
  })

  // On a request the page's fallback and the seeded boundary render through
  // ONE query client, fallback first. Built from the page's own fallback, so a
  // fallback that registers the stations key fails here.
  it("serves the seeded dial after the page's own fallback rendered on the same client", async () => {
    const fallback = pageDialSlot().props.fallback
    const html = serverRender(
      <>
        {fallback}
        {await HydratedRadioDial()}
      </>
    )

    expect(html).toContain('href="/radio/kexp"')
    expect(html.match(/<article/g)).toHaveLength(2)
  })

  it('renders the page head and the rest of the hub around the dial fallback', () => {
    const html = serverRender(RadioPage())

    expect(html).toContain('>Radio</h1>')
    expect(html).toContain('Latest playlists')
    expect(html).toContain(DIAL_LOADER)
  })
})

describe('/radio dial fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  })

  it('is the shared cached fallback over the dial, and registers no query on a cold cache', () => {
    const fallback = pageDialSlot().props.fallback as ReactElement<{
      loader: ReactElement
      children: ReactElement
    }>

    expect(fallback.type).toBe(CachedListFallback)
    expect(fallback.props.loader.type).toBe(RadioDialLoader)
    expect(fallback.props.children.type).toBe(RadioDial)
    expect(registeredQueryHashes(fallback)).toEqual([])
    expect(apiRequest).not.toHaveBeenCalled()
  })

  it('on a cold cache, is the markup the dial renders before its reads answer', () => {
    const fallback = pageDialSlot().props.fallback

    expect(serverRender(fallback)).toBe(serverRender(<RadioDial />))
    expect(serverRender(<RadioDialLoader />)).toBe(serverRender(<RadioDial />))
  })

  // A client-side navigation back to the hub renders the fallback on the
  // browser's warm client: it shows the cached dial instead of the loader.
  // The dial's own hooks run: they reuse the cached stations entry and create
  // the guide and per-strip entries, none of which the route seeds.
  it('draws the cached stations from a warm client, creating no seeded entry', () => {
    const client = warmClient([[RADIO_DIAL_FIRST_SCREEN_KEY, stationsPayload]])
    const [stationsEntry] = entriesFor(client, [RADIO_DIAL_FIRST_SCREEN_KEY])

    const { container } = renderOnNavigation(pageDialSlot().props.fallback, client)

    expect(container.innerHTML).toContain('href="/radio/kexp"')
    expect(container.querySelectorAll('article')).toHaveLength(2)
    expect(container.innerHTML).not.toContain(DIAL_LOADER)
    expect(entriesFor(client, [RADIO_DIAL_FIRST_SCREEN_KEY])).toEqual([stationsEntry])
    const created = client
      .getQueryCache()
      .getAll()
      .map(q => q.queryHash)
      .filter(hash => hash !== hashKey(RADIO_DIAL_FIRST_SCREEN_KEY))
    expect(created).toContain(hashKey(['radio', 'guide']))
  })

  // A failed guide refetch keeps its old data; the dial shows no guide then.
  it.each([
    ['shows a cached guide', 'success', true],
    ['hides a cached guide whose last refetch failed', 'error', false],
  ] as const)('%s', (_name, status, shown) => {
    const client = warmClient([
      [RADIO_DIAL_FIRST_SCREEN_KEY, stationsPayload],
      [
        ['radio', 'guide'],
        {
          on_now: [
            {
              station: { slug: 'wfmu', name: 'WFMU' },
              show: { id: 1, slug: 'wake', name: 'Wake', host_name: null },
              starts_at: new Date().toISOString(),
              ends_at: new Date(Date.now() + 2 * 3600e3).toISOString(),
              station_timezone: 'America/New_York',
            },
          ],
          up_next: [],
          generated_at: new Date().toISOString(),
        },
      ],
    ])
    if (status === 'error') {
      client
        .getQueryCache()
        .find({ queryKey: ['radio', 'guide'], exact: true })!
        .setState({ status: 'error', error: new Error('refetch failed') })
    }

    const { container } = renderOnNavigation(pageDialSlot().props.fallback, client)

    expect(container.innerHTML.includes('aria-label="Program guide"')).toBe(shown)
  })

  // The seeded key must be registered by nothing that renders before the
  // boundary: the hub around the slot included.
  it('the hub outside the dial slot does not register the stations key', () => {
    const hashes = registeredQueryHashes(<RadioHub dial={null} />)

    expect(hashes.length).toBeGreaterThan(0)
    expect(hashes).not.toContain(hashKey(RADIO_DIAL_FIRST_SCREEN_KEY))
  })
})

/**
 * The client half of the pair: mounted on a cold cache, the real dial requests
 * the URL the server reads and registers the key the server seeds.
 */
describe('/radio dial request pairing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  })

  it('RadioDial requests the stations URL and keys on the stations key', () => {
    const hashes = registeredQueryHashes(<RadioDial />)

    expect(hashes).toContain(hashKey(RADIO_DIAL_FIRST_SCREEN_KEY))
    expect(apiRequest.mock.calls.map(([url]) => url)).toContain(
      RADIO_DIAL_FIRST_SCREEN_URL
    )
  })
})

describeCachedListFallback({
  route: '/radio',
  fallback: () => pageDialSlot().props.fallback,
  list: RadioDial,
  seeded: [[RADIO_DIAL_FIRST_SCREEN_KEY, stationsPayload]],
  defeatingParams: [],
  seededRow: 'href="/radio/kexp"',
  loaders: [DIAL_LOADER],
  // The hub reads no URL params: there is no other filter, sort or page.
  setSearch: () => {},
  reset: () => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  },
})
