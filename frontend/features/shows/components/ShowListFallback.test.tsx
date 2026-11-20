import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { act, cleanup, renderHook } from '@testing-library/react'
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

type Viewer = import('@/test/saveControlHydration').ViewerState
const viewer = vi.hoisted(() => ({
  current: { authStatus: 'anonymous', user: null } as Viewer,
  favoriteCities: [] as Array<{ city: string; state: string }>,
  // Whose profile the profile read has answered with: the viewer's own, none
  // yet, or a payload retained from another account.
  profile: 'own' as 'own' | 'unanswered' | 'other',
}))
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ ...viewer.current, logout: vi.fn() }),
  }
})
vi.mock('@/features/auth', () => ({
  useProfile: () => {
    const { user } = viewer.current
    if (!user || viewer.profile === 'unanswered') return { data: undefined }
    return {
      data: {
        success: true,
        user: {
          id: viewer.profile === 'own' ? Number(user.id) : Number(user.id) + 1,
          preferences: { favorite_cities: viewer.favoriteCities },
        },
      },
    }
  },
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
import {
  entriesFor,
  registeredQueryHashes,
  renderOnNavigation,
  warmClient,
} from '@/test/firstScreenRender'
import {
  ANONYMOUS_VIEWER,
  hydrateCollectingErrors,
} from '@/test/saveControlHydration'
import { showsFirstScreenSeeds } from '../firstScreen'
import type { ShowsCalendarResponse } from '../types'
import { showsCalendarQueryKey } from '../hooks/useShows'
import { useShowListCitySelection } from '../hooks/useShowListCitySelection'
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
    days: 0,
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

const skeletonHtml = renderToString(<ShowListSkeleton />)

/** The fallback mounted the way a client-side navigation mounts it. */
const navigate = (client: QueryClient) => renderOnNavigation(<ShowListFallback />, client)

const geoRequests = (spy: { mock: { calls: unknown[][] } }) =>
  spy.mock.calls.filter(([input]) => String(input).includes('/api/geo'))

const entryFor = (client: QueryClient, key: QueryKey) => entriesFor(client, [key])[0]

function setViewer(next: Viewer, favoriteCities: typeof viewer.favoriteCities = []) {
  viewer.current = next
  viewer.favoriteCities = favoriteCities
  viewer.profile = 'own'
}

const SIGNED_IN: Viewer = { authStatus: 'authenticated', user: { id: '7' } }

let fetchSpy: { mock: { calls: unknown[][] }; mockRestore: () => void }

beforeEach(() => {
  vi.clearAllMocks()
  apiRequest.mockReturnValue(new Promise(() => {}))
  fetchSpy = vi.spyOn(globalThis, 'fetch').mockReturnValue(new Promise(() => {}))
  nav.params = new URLSearchParams()
  setViewer(ANONYMOUS_VIEWER)
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
    const { root, errors } = await hydrateCollectingErrors(
      container,
      <QueryClientProvider client={warmClient(ALL_CITIES_ENTRIES)}>
        <ShowListFallback />
      </QueryClientProvider>
    )

    expect(errors).toEqual([])
    expect(container.querySelector('[inert]')?.textContent).toContain('All cities row')

    act(() => root.unmount())
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

  it('draws the city the legacy ?city=&state= names', () => {
    nav.params = new URLSearchParams('city=Tucson&state=AZ')
    const client = warmClient(entriesForSelection([TUCSON], TUCSON_ROWS))

    const { container } = navigate(client)

    expectDrawn(container, 'Tucson row')
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  // This list's geo default is for anonymous viewers only, so a signed-in
  // viewer with no favourites lists all cities whatever the session cache holds.
  it('draws all cities for a signed-in viewer with no favourites and no cached location answer', () => {
    setViewer(SIGNED_IN, [])
    const client = warmClient(ALL_CITIES_ENTRIES)

    const { container } = navigate(client)

    expectDrawn(container, 'All cities row')
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

  // A cached city the facet does not list draws nothing either: a facet
  // refetch that adds it would make it the default under the drawn copy.
  it('is the skeleton for a cached location the facet does not list', () => {
    cacheGeoAnswer({ city: 'Flagstaff', state: 'AZ' })
    const client = warmClient(ALL_CITIES_ENTRIES)

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it("is the skeleton while a signed-in viewer's own profile has not answered", () => {
    setViewer(SIGNED_IN, [TUCSON])
    viewer.profile = 'unanswered'
    const client = warmClient([
      ...ALL_CITIES_ENTRIES,
      ...entriesForSelection([TUCSON], TUCSON_ROWS),
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it("is the skeleton while the cached profile is another account's", () => {
    setViewer(SIGNED_IN, [TUCSON])
    viewer.profile = 'other'
    const client = warmClient([
      ...ALL_CITIES_ENTRIES,
      ...entriesForSelection([TUCSON], TUCSON_ROWS),
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
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

  // The cache the home page's idle prefetch leaves: the rows and the facet,
  // not the month histogram. Drawing the list there would create the seeded
  // histogram entry before the seed lands.
  it('is the skeleton, and creates no histogram entry, when the month histogram is not cached', () => {
    cacheGeoAnswer(null)
    const client = warmClient([
      [SHOW_CITIES_FIRST_SCREEN_KEY, facet],
      [SHOWS_CALENDAR_FIRST_SCREEN_KEY, ALL_ROWS],
    ])

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(entryFor(client, SHOWS_MONTHS_FIRST_SCREEN_KEY)).toBeUndefined()
  })

  it('is the skeleton and creates nothing on a cold cache', () => {
    cacheGeoAnswer(null)
    const client = new QueryClient()

    const { container } = navigate(client)

    expectSkeleton(container)
    expect(client.getQueryCache().getAll()).toHaveLength(0)
  })

  it.each(['tags=noise', 'tag_match=any', 'page=2', 'tags=', 'utm_source=mail', 'cities=Tucson%2CAZ&page=2'])(
    'is the skeleton on a URL with %s, which carries a param other than the city selection',
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
    [
      'the legacy ?city=&state=',
      () => {
        nav.params = new URLSearchParams('city=Tucson&state=AZ')
      },
      [TUCSON],
    ],
  ]

  it.each(states)('names the entries the list reads for %s', (_name, arrange, selection) => {
    arrange()

    const registered = registeredQueryHashes(<ShowList />)
    for (const key of showListFirstPaintKeys(selection)) {
      expect(registered).toContain(hashKey(key))
    }
  })

  // Every key the page seeds is required before the all-cities list is drawn,
  // so drawing it can create none of them.
  it("covers every key the page's seed writes, for the all-cities selection", () => {
    const seeds = showsFirstScreenSeeds({
      // The fixture carries only the fields the list reads.
      shows: ALL_ROWS as unknown as ShowsCalendarResponse,
      cities: facet,
      months,
      calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY,
      citiesKey: SHOW_CITIES_FIRST_SCREEN_KEY,
    })

    expect(new Set(showListFirstPaintKeys([]).map(hashKey))).toEqual(
      new Set(seeds?.map(seed => hashKey(seed.queryKey)))
    )
  })
})

// What the selection hook reports, read directly, in both of its modes.
describe('useShowListCitySelection', () => {
  const read = (geoSource: 'request' | 'cache') =>
    renderHook(() => useShowListCitySelection({ citiesData: facet, geoSource }), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
      ),
    }).result.current

  it('in cache mode, reads an uncached location answer as resolving, and the default as possible', () => {
    const selection = read('cache')

    expect(selection.isResolving).toBe(true)
    expect(selection.geoDefaultPossible).toBe(true)
    expect(geoRequests(fetchSpy)).toHaveLength(0)
  })

  it('in cache mode, reads a cached "no city" as settled, with no default possible', () => {
    cacheGeoAnswer(null)
    const selection = read('cache')

    expect(selection.isResolving).toBe(false)
    expect(selection.geoDefaultPossible).toBe(false)
    expect(selection.selectedCities).toEqual([])
  })

  it('in cache mode, applies a cached location on the first render', () => {
    cacheGeoAnswer(MESA)
    const selection = read('cache')

    expect(selection.appliedGeoDefault).toEqual(MESA)
    expect(selection.geoDefaultPossible).toBe(true)
  })

  it('in request mode, counts the default as possible for an eligible viewer', () => {
    cacheGeoAnswer(null)

    expect(read('request').geoDefaultPossible).toBe(true)
  })

  it('counts no default as possible when favourites choose the cities', () => {
    setViewer(SIGNED_IN, [TUCSON])

    for (const geoSource of ['request', 'cache'] as const) {
      const selection = read(geoSource)
      expect(selection.geoDefaultPossible).toBe(false)
      expect(selection.isResolving).toBe(false)
      expect(selection.selectedCities).toEqual([TUCSON])
    }
  })
})
