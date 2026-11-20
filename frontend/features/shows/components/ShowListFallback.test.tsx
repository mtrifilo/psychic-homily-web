import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import { hydrateRoot } from 'react-dom/client'
import { act, cleanup, render } from '@testing-library/react'
import {
  QueryClient,
  QueryClientProvider,
  hashKey,
  type QueryKey,
} from '@tanstack/react-query'

// Every browser request the list makes never answers, so a test sees only what
// the cache held.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

const nav = vi.hoisted(() => ({ params: new URLSearchParams() }))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => nav.params,
  usePathname: () => '/shows',
}))

// nuqs reads the same URL as `useSearchParams`; the real parsers stay in the loop.
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (key: string, parser: { parse: (v: string) => unknown }) => {
      const raw = nav.params.get(key)
      return [raw != null ? parser.parse(raw) : null, vi.fn()]
    },
  }
})

type Viewer = {
  authStatus: 'pending' | 'anonymous' | 'authenticated'
  user: { id: string } | null
}
const viewer = vi.hoisted(() => ({
  current: { authStatus: 'anonymous', user: null } as Viewer,
  favoriteCities: [] as Array<{ city: string; state: string }>,
}))
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ ...viewer.current, logout: vi.fn() }),
  }
})
vi.mock('@/features/auth', () => ({
  useProfile: () => ({
    data:
      viewer.current.authStatus === 'authenticated'
        ? { user: { preferences: { favorite_cities: viewer.favoriteCities } } }
        : null,
  }),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))

vi.mock('../hooks/useSavedShows', async importOriginal => ({
  ...(await importOriginal<typeof import('../hooks/useSavedShows')>()),
  useShowSaveCountBatch: () => ({ data: {} }),
}))

vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div />,
  TagFacetSheet: () => <div />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (s: string[]) => s.join(','),
}))

vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: () => ({ density: 'comfortable', setDensity: vi.fn() }),
}))

vi.mock('./DayGroupedShowRow', () => ({
  DayGroupedShowRow: ({ show }: { show: { id: number; title: string } }) => (
    <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  ),
  DayGroupedShowListHeader: () => <div />,
}))

import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOWS_CALENDAR_FIRST_SCREEN_KEY,
  SHOWS_MONTHS_FIRST_SCREEN_KEY,
} from '@/features/shows/api'
import { GEO_CACHE_KEY } from '@/lib/geo-client'
import { showsCalendarQueryKey } from '../hooks/useShows'
import { ShowList } from './ShowList'
import { ShowListFallback, showListFirstPaintKeys } from './ShowListFallback'
import { ShowListSkeleton } from './ShowListSkeleton'

const TUCSON = { city: 'Tucson', state: 'AZ' }
const PHOENIX = { city: 'Phoenix', state: 'AZ' }
const MESA = { city: 'Mesa', state: 'AZ' }

const facet = {
  cities: [
    { city: 'Phoenix', state: 'AZ', show_count: 3, latitude: 33.45, longitude: -112.07 },
    { city: 'Mesa', state: 'AZ', show_count: 2, latitude: 33.42, longitude: -111.82 },
    { city: 'Tucson', state: 'AZ', show_count: 2, latitude: 32.22, longitude: -110.93 },
  ],
}

function calendar(id: number, title: string) {
  return {
    shows: [
      { id, title, event_date: '2026-12-01T03:00:00Z', state: 'AZ', venues: [], artists: [] },
    ],
    total: 1,
    limit: 50,
    offset: 0,
    year: 0,
    month: 0,
    day: 0,
  }
}
const months = { months: [{ year: 2026, month: 11, count: 1 }], total: 1 }

const ALL_ROWS = calendar(1, 'All cities row')
const TUCSON_ROWS = calendar(2, 'Tucson row')
const MESA_ROWS = calendar(3, 'Mesa row')
const PHOENIX_ROWS = calendar(4, 'Phoenix row')

type Entry = readonly [QueryKey, unknown]

/** The facet plus a selection's rows and histogram, under the fallback's keys. */
function entriesForSelection(
  selection: Array<{ city: string; state: string }>,
  rows: unknown
): Entry[] {
  const [facetKey, calendarKey, monthsKey] = showListFirstPaintKeys(selection)
  return [
    [facetKey, facet],
    [calendarKey, rows],
    [monthsKey, months],
  ]
}

const ALL_CITIES_ENTRIES: Entry[] = [
  [SHOW_CITIES_FIRST_SCREEN_KEY, facet],
  [SHOWS_CALENDAR_FIRST_SCREEN_KEY, ALL_ROWS],
  [SHOWS_MONTHS_FIRST_SCREEN_KEY, months],
]

function warmClient(entries: readonly Entry[]): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  for (const [key, data] of entries) client.setQueryData(key, data, { updatedAt: 0 })
  return client
}

const skeletonHtml = renderToString(<ShowListSkeleton />)

/** A fallback mounted the way a client-side navigation mounts it. */
function navigate(client: QueryClient, element: ReactElement = <ShowListFallback />) {
  return render(<QueryClientProvider client={client}>{element}</QueryClientProvider>)
}

const geoRequests = (spy: { mock: { calls: unknown[][] } }) =>
  spy.mock.calls.filter(([input]) => String(input).includes('/api/geo'))

const entryFor = (client: QueryClient, key: QueryKey) =>
  client.getQueryCache().find({ queryKey: key, exact: true })

function setViewer(next: Viewer, favoriteCities: typeof viewer.favoriteCities = []) {
  viewer.current = next
  viewer.favoriteCities = favoriteCities
}

const ANONYMOUS: Viewer = { authStatus: 'anonymous', user: null }
const SIGNED_IN: Viewer = { authStatus: 'authenticated', user: { id: '7' } }

let fetchSpy: { mock: { calls: unknown[][] }; mockRestore: () => void }

beforeEach(() => {
  vi.clearAllMocks()
  apiRequest.mockReturnValue(new Promise(() => {}))
  fetchSpy = vi.spyOn(globalThis, 'fetch').mockReturnValue(new Promise(() => {}))
  nav.params = new URLSearchParams()
  setViewer(ANONYMOUS)
  window.sessionStorage.clear()
})

afterEach(() => {
  cleanup()
  fetchSpy.mockRestore()
  window.sessionStorage.clear()
})

const cacheGeoAnswer = (geo: { city: string; state: string } | null) =>
  window.sessionStorage.setItem(GEO_CACHE_KEY, JSON.stringify({ geo }))

describe('ShowListFallback in server HTML', () => {
  it('server-renders exactly the skeleton, whatever the cache holds', () => {
    cacheGeoAnswer(null)
    const html = renderToString(
      <QueryClientProvider client={warmClient(ALL_CITIES_ENTRIES)}>
        <ShowListFallback />
      </QueryClientProvider>
    )

    expect(html).toBe(skeletonHtml)
  })

  it('hydrates the skeleton over a warm cache with no mismatch, then draws the list', async () => {
    cacheGeoAnswer(null)
    const container = document.createElement('div')
    document.body.appendChild(container)
    container.innerHTML = renderToString(
      <QueryClientProvider client={new QueryClient()}>
        <ShowListFallback />
      </QueryClientProvider>
    )
    const recoverable = vi.fn()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    let root: ReturnType<typeof hydrateRoot> | undefined
    await act(async () => {
      root = hydrateRoot(
        container,
        <QueryClientProvider client={warmClient(ALL_CITIES_ENTRIES)}>
          <ShowListFallback />
        </QueryClientProvider>,
        { onRecoverableError: recoverable }
      )
    })

    expect(recoverable).not.toHaveBeenCalled()
    expect(consoleError).not.toHaveBeenCalled()
    expect(container.querySelector('[inert]')?.textContent).toContain('All cities row')

    consoleError.mockRestore()
    act(() => root?.unmount())
    container.remove()
  })
})

describe('ShowListFallback on a client-side navigation, warm for the viewer', () => {
  function expectDrawn(container: HTMLElement, title: string) {
    const copy = container.querySelector('[inert]')
    expect(copy?.textContent).toContain(title)
    expect(
      [...container.querySelectorAll('[role="status"]')].some(s => !s.closest('[inert]'))
    ).toBe(true)
  }

  it('draws the all-cities rows for an anonymous viewer whose location answer is "no city"', () => {
    cacheGeoAnswer(null)
    const client = warmClient(ALL_CITIES_ENTRIES)
    const before = ALL_CITIES_ENTRIES.map(([key]) => entryFor(client, key))

    const { container } = navigate(client)

    expectDrawn(container, 'All cities row')
    // The list ran over the seeded entries and replaced none of them.
    expect(ALL_CITIES_ENTRIES.map(([key]) => entryFor(client, key))).toEqual(before)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it("draws a signed-in viewer's favourite city, creating no all-cities entry and no geo request", () => {
    setViewer(SIGNED_IN, [TUCSON])
    const client = warmClient(entriesForSelection([TUCSON], TUCSON_ROWS))

    const { container } = navigate(client)

    expectDrawn(container, 'Tucson row')
    expect(container.textContent).not.toContain('All cities row')
    expect(entryFor(client, SHOWS_CALENDAR_FIRST_SCREEN_KEY)).toBeUndefined()
    expect(entryFor(client, SHOWS_MONTHS_FIRST_SCREEN_KEY)).toBeUndefined()
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it('draws the city an explicit ?cities= names, with no cached location answer', () => {
    nav.params = new URLSearchParams('cities=Tucson%2CAZ')
    const client = warmClient(entriesForSelection([TUCSON], TUCSON_ROWS))

    const { container } = navigate(client)

    expectDrawn(container, 'Tucson row')
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })
})

describe('ShowListFallback on a client-side navigation, skeleton', () => {
  function expectSkeleton(container: HTMLElement) {
    expect(container.innerHTML).toBe(skeletonHtml)
  }

  it('is the skeleton for a location-default selection even when its city and all cities are cached', () => {
    cacheGeoAnswer(MESA)
    const client = warmClient([
      ...ALL_CITIES_ENTRIES,
      ...entriesForSelection([MESA], MESA_ROWS),
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it('is the skeleton while the selection is still resolving', () => {
    setViewer({ authStatus: 'pending', user: null })
    cacheGeoAnswer(null)
    const client = warmClient(ALL_CITIES_ENTRIES)

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it('is the skeleton, and requests nothing, for an anonymous viewer with no cached location answer', () => {
    const client = warmClient(ALL_CITIES_ENTRIES)

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it("is the skeleton when the cache holds another city's rows", () => {
    setViewer(SIGNED_IN, [TUCSON])
    const client = warmClient([
      ...ALL_CITIES_ENTRIES,
      ...entriesForSelection([PHOENIX], PHOENIX_ROWS),
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(entryFor(client, showListFirstPaintKeys([TUCSON])[1])).toBeUndefined()
  })

  it("is the skeleton when the cache holds a month page's rows, not the list's", () => {
    cacheGeoAnswer(null)
    const client = warmClient([
      [SHOW_CITIES_FIRST_SCREEN_KEY, facet],
      [showsCalendarQueryKey({ limit: 50, window: { year: 2026, month: 11 } }), ALL_ROWS],
      [SHOWS_MONTHS_FIRST_SCREEN_KEY, months],
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(entryFor(client, SHOWS_CALENDAR_FIRST_SCREEN_KEY)).toBeUndefined()
  })

  it('is the skeleton and creates nothing on a cold cache', () => {
    cacheGeoAnswer(null)
    const client = new QueryClient()

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(client.getQueryCache().getAll()).toHaveLength(0)
  })

  it.each(['tags=noise', 'tag_match=any', 'page=2'])(
    'is the skeleton on a URL with %s, whose rows key by more than the city',
    search => {
      cacheGeoAnswer(null)
      nav.params = new URLSearchParams(search)
      const { container } = navigate(warmClient(ALL_CITIES_ENTRIES))

      expectSkeleton(container)
    }
  )
})

// The fallback names entries by building keys itself; the list builds them in
// its hooks. They have to be the same entries, for every viewer it draws for.
describe('showListFirstPaintKeys', () => {
  const states: Array<[string, () => void, Array<{ city: string; state: string }>]> = [
    ['anonymous, no location answer', () => cacheGeoAnswer(null), []],
    ['signed in with a favourite', () => setViewer(SIGNED_IN, [TUCSON]), [TUCSON]],
    [
      'an explicit ?cities=',
      () => {
        nav.params = new URLSearchParams('cities=Tucson%2CAZ')
      },
      [TUCSON],
    ],
  ]

  it.each(states)('names the entries the list reads for %s', (_name, arrange, selection) => {
    arrange()
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    navigate(client, <ShowList />)

    const registered = client.getQueryCache().getAll().map(q => q.queryHash)
    for (const key of showListFirstPaintKeys(selection)) {
      expect(registered).toContain(hashKey(key))
    }
  })

  it('names the seeded entries for the all-cities selection', () => {
    expect(showListFirstPaintKeys([]).map(hashKey)).toEqual(
      [
        SHOW_CITIES_FIRST_SCREEN_KEY,
        SHOWS_CALENDAR_FIRST_SCREEN_KEY,
        SHOWS_MONTHS_FIRST_SCREEN_KEY,
      ].map(hashKey)
    )
  })
})
