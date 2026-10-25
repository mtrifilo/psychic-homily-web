import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import { render, waitFor } from '@testing-library/react'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  hashKey,
  type DehydratedState,
} from '@tanstack/react-query'
import { errorResponse, jsonBodyResponse } from '@/lib/seo/test-helpers'
import { findElements } from '@/test/elementTree'

// `seedFirstScreen` awaits `connection()`, which marks a request-time render and
// has nothing to do outside Next.
vi.mock('next/server', () => ({
  connection: async () => {},
}))

// On the server `getQueryClient()` mints a fresh client per call; under jsdom
// it would hand back the browser singleton instead.
vi.mock('@/lib/queryClient', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/queryClient')>()
  const { QueryClient } = await import('@tanstack/react-query')
  return { ...actual, getQueryClient: () => new QueryClient() }
})

const mockResolveHomeViewer = vi.fn()
const mockGetAuthenticatedHomeLayout = vi.fn()
const mockPrefetchHomeSavedShows = vi.fn()
vi.mock('@/lib/auth-hydration', () => ({
  resolveHomeViewer: () => mockResolveHomeViewer(),
  getAuthenticatedHomeLayout: () => mockGetAuthenticatedHomeLayout(),
  prefetchHomeSavedShows: () => mockPrefetchHomeSavedShows(),
}))

// The variants are stood in by NAMED functions, so a test can find the element
// the slot renders. The list itself is rendered for real below, under the
// boundaries the slot hands back.
vi.mock('@/features/home/components/AnonymousHome', () => ({
  AnonymousHome: function AnonymousHome(): null {
    return null
  },
}))
vi.mock('@/features/home/components/SignedInHome', () => ({
  SignedInHome: function SignedInHome(): null {
    return null
  },
}))
vi.mock('@/features/home/components/HomeVariantSwitch', () => ({
  HomeVariantSwitch: function HomeVariantSwitch(): null {
    return null
  },
}))

// What the real list needs around it: a signed-out reader with no profile, and
// no idle route prefetching.
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ authStatus: 'anonymous' as const, user: null }),
  }
})
vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: undefined }),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))
vi.mock('@/lib/hooks/common/usePrefetchRoutes', () => ({
  usePrefetchRoutes: vi.fn(),
}))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}))

import { HomeContentSlot } from './HomeContentSlot'
import { AnonymousHome } from '@/features/home/components/AnonymousHome'
import { SignedInHome } from '@/features/home/components/SignedInHome'
import { HomeVariantSwitch } from '@/features/home/components/HomeVariantSwitch'
import { HomeShowList } from '@/features/shows/components/HomeShowList'
import {
  HOME_UPCOMING_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_KEY,
} from '@/features/shows/api'

function show(id: number, slug: string, headliner: string) {
  return {
    id,
    slug,
    title: '',
    event_date: '2099-09-12T03:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    status: 'approved',
    is_cancelled: false,
    is_sold_out: false,
    venues: [
      { id: 7, slug: 'the-rebel-lounge', name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' },
    ],
    artists: [{ id: id + 100, slug: `${slug}-act`, name: headliner, is_headliner: true }],
  }
}

const SEEDED_SHOWS = [
  show(1, 'first-show', 'First Act'),
  show(2, 'second-show', 'Second Act'),
]

function upcomingResponse(shows: unknown[]) {
  return jsonBodyResponse({
    shows,
    timezone: 'UTC',
    total: shows.length,
    pagination: { next_cursor: null, has_more: false, limit: 5 },
  })
}

const citiesResponse = () =>
  jsonBodyResponse({
    cities: [
      { city: 'Phoenix', state: 'AZ', show_count: 31 },
      { city: 'Mesa', state: 'AZ', show_count: 18 },
      { city: 'Tucson', state: 'AZ', show_count: 18 },
    ],
  })

const upcomingFetch = vi.fn()
const citiesFetch = vi.fn()
const otherFetch = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  mockResolveHomeViewer.mockResolvedValue('anonymous')
  upcomingFetch.mockImplementation(async () => upcomingResponse(SEEDED_SHOWS))
  citiesFetch.mockImplementation(async () => citiesResponse())
  otherFetch.mockImplementation(async () => jsonBodyResponse({}))
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    const path = new URL(String(url), 'http://test.local').pathname
    if (path.endsWith('/shows/upcoming')) return upcomingFetch(url, init)
    if (path.endsWith('/shows/cities')) return citiesFetch(url, init)
    return otherFetch(url, init)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function renderSlot() {
  return (await HomeContentSlot()) as ReactElement
}

/** Every dehydrated state the slot hands a `HydrationBoundary`, outermost first. */
function boundaryStatesOf(tree: ReactElement): DehydratedState[] {
  return findElements(tree, node => node.type === HydrationBoundary)
    .map(node => (node.props as { state?: DehydratedState }).state)
    .filter((state): state is DehydratedState => state !== undefined)
}

function seededQueryHashes(tree: ReactElement): string[] {
  return boundaryStatesOf(tree).flatMap(state => state.queries.map(q => q.queryHash))
}

const testClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } })

/** The real home list under the slot's own boundaries, nested as the slot nests them. */
function listUnder(states: DehydratedState[], client: QueryClient) {
  return (
    <QueryClientProvider client={client}>
      {states.reduceRight<ReactElement>(
        (children, state) => <HydrationBoundary state={state}>{children}</HydrationBoundary>,
        <HomeShowList />,
      )}
    </QueryClientProvider>
  )
}

/**
 * Server-render the list the way the route's HTML is produced: no client fetch
 * runs inside `renderToString`, so whatever this prints came from the seed.
 */
function serverRenderList(tree: ReactElement): string {
  return renderToString(listUnder(boundaryStatesOf(tree), testClient()))
}

const LOADER = 'animate-spin'
// The literal spelling is pinned in useShowsFirstScreen.test.tsx, beside the
// hook that registers it.
const UPCOMING_KEY_HASH = hashKey(HOME_UPCOMING_FIRST_SCREEN_KEY)
const CITIES_KEY_HASH = hashKey(SHOW_CITIES_FIRST_SCREEN_KEY)

describe('HomeContentSlot, anonymous viewer', () => {
  it('renders the anonymous page inside the seeded boundary', async () => {
    const tree = await renderSlot()

    expect(findElements(tree, node => node.type === AnonymousHome)).toHaveLength(1)
    expect(boundaryStatesOf(tree)).toHaveLength(1)
  })

  it('asks for the first page the home list asks for, and the city facet', async () => {
    await renderSlot()

    expect(upcomingFetch).toHaveBeenCalledTimes(1)
    const url = new URL(String(upcomingFetch.mock.calls[0][0]))
    expect([...url.searchParams.entries()]).toEqual([['limit', '5']])
    expect(citiesFetch).toHaveBeenCalledTimes(1)
    expect(new URL(String(citiesFetch.mock.calls[0][0])).search).toBe('')
  })

  it('seeds the rows and the facet under the hooks keys, stamped stale', async () => {
    const tree = await renderSlot()
    const queries = boundaryStatesOf(tree).flatMap(state => state.queries)

    expect(queries.map(q => q.queryHash).sort()).toEqual(
      [UPCOMING_KEY_HASH, CITIES_KEY_HASH].sort(),
    )
    const rows = queries.find(q => q.queryHash === UPCOMING_KEY_HASH)
    expect(rows?.state.data).toMatchObject({ shows: SEEDED_SHOWS })
    expect(rows?.state.dataUpdatedAt).toBe(0)
  })

  it('puts the first page of rows and the city chips in the server render, with no loader', async () => {
    const html = serverRenderList(await renderSlot())

    expect(html).toContain('href="/shows/first-show"')
    expect(html).toContain('href="/shows/second-show"')
    expect(html).toContain('First Act')
    // React's server renderer separates adjacent text nodes with comments.
    expect(html.replaceAll('<!-- -->', '')).toContain('Popular:')
    expect(html.replaceAll('<!-- -->', '')).toContain('Phoenix, AZ (31)')
    expect(html).not.toContain(LOADER)
    // A seeded entry revalidates on its first commit; that must not dim it.
    expect(html).not.toContain('opacity-60')
  })

  it('server-renders the empty sentence for an empty catalogue, not the loader', async () => {
    upcomingFetch.mockImplementation(async () => upcomingResponse([]))

    const html = serverRenderList(await renderSlot())

    expect(html).toContain('No upcoming shows at this time.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the page, with the loader, when the rows read fails', async () => {
    upcomingFetch.mockImplementation(async () => errorResponse(500))

    const tree = await renderSlot()

    expect(findElements(tree, node => node.type === AnonymousHome)).toHaveLength(1)
    expect(seededQueryHashes(tree)).toEqual([CITIES_KEY_HASH])
    const html = serverRenderList(tree)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows')
  })

  it('still seeds the rows when only the facet read fails', async () => {
    citiesFetch.mockImplementation(async () => errorResponse(503))

    const tree = await renderSlot()

    expect(seededQueryHashes(tree)).toEqual([UPCOMING_KEY_HASH])
    const html = serverRenderList(tree)
    expect(html).toContain('href="/shows/first-show"')
    expect(html).not.toContain(LOADER)
  })

  it('renders the page unseeded, with the loader, when both reads fail', async () => {
    upcomingFetch.mockImplementation(async () => {
      throw new TypeError('fetch failed')
    })
    citiesFetch.mockImplementation(async () => errorResponse(500))

    const tree = await renderSlot()

    expect(findElements(tree, node => node.type === AnonymousHome)).toHaveLength(1)
    expect(boundaryStatesOf(tree)).toEqual([])
    expect(serverRenderList(tree)).toContain(LOADER)
  })

  it('replaces the seeded rows with the live read once mounted, at full strength meanwhile', async () => {
    const states = boundaryStatesOf(await renderSlot())
    upcomingFetch.mockImplementation(async () =>
      upcomingResponse([show(3, 'late-addition', 'Late Act')]),
    )

    const { container } = render(listUnder(states, testClient()))

    expect(container.innerHTML).toContain('href="/shows/first-show"')
    expect(container.innerHTML).not.toContain('opacity-60')
    await waitFor(() =>
      expect(container.innerHTML).toContain('href="/shows/late-addition"'),
    )
    // The browser asked the question the server asked.
    const [serverUrl, browserUrl] = upcomingFetch.mock.calls.map(([url]) => String(url))
    expect(new URL(browserUrl).search).toBe(new URL(serverUrl).search)
  })

  it('keeps the seeded rows when the mount-time refresh fails', async () => {
    const states = boundaryStatesOf(await renderSlot())
    upcomingFetch.mockImplementation(async () => errorResponse(500))

    const { container } = render(listUnder(states, testClient()))

    await waitFor(() => expect(upcomingFetch).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(container.innerHTML).not.toContain('opacity-60'))
    expect(container.innerHTML).toContain('href="/shows/first-show"')
    expect(container.innerHTML).not.toContain('Unable to load shows.')
  })
})

describe('HomeContentSlot, other viewers', () => {
  it('makes no upcoming-shows read for a signed-in viewer', async () => {
    mockResolveHomeViewer.mockResolvedValue('authenticated')
    mockGetAuthenticatedHomeLayout.mockResolvedValue(null)
    mockPrefetchHomeSavedShows.mockResolvedValue({ mutations: [], queries: [] })

    const tree = await renderSlot()

    expect(findElements(tree, node => node.type === SignedInHome)).toHaveLength(1)
    expect(upcomingFetch).not.toHaveBeenCalled()
    expect(citiesFetch).not.toHaveBeenCalled()
  })

  it('makes no upcoming-shows read when the viewer could not be resolved', async () => {
    mockResolveHomeViewer.mockResolvedValue('indeterminate')

    const tree = await renderSlot()

    expect(tree.type).toBe(HomeVariantSwitch)
    expect(upcomingFetch).not.toHaveBeenCalled()
    expect(citiesFetch).not.toHaveBeenCalled()
  })
})
