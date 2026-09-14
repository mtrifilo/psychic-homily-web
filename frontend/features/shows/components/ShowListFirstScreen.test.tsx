import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

const mockApiRequest = vi.fn()
// Spread the real module: other transitive importers read `API_BASE_URL` from
// it, and replacing the whole thing breaks them at import time.
vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiRequest: (...args: unknown[]) => mockApiRequest(...args) }
})

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: vi.fn(),
  }),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}))

vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return { ...actual, useQueryState: () => [null, vi.fn()] }
})

vi.mock('../hooks/useSavedShows', () => ({
  useShowSaveCountBatch: () => ({ data: {} }),
}))

vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: null }),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))

vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div />,
  TagFacetSheet: () => <div />,
  parseTagsParam: () => [],
  buildTagsParam: (s: string[]) => s.join(','),
}))

vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: () => ({ density: 'comfortable', setDensity: vi.fn() }),
}))

vi.mock('@/components/filters/useGeoDefaultCity', () => ({
  useGeoDefaultCity: () => ({
    appliedGeoDefault: null,
    notifyUserInteracted: vi.fn(),
  }),
  shouldShowGeoAffordance: () => false,
}))

vi.mock('./DayGroupedShowRow', () => ({
  DayGroupedShowRow: ({ show }: { show: { id: number; title: string } }) => (
    <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  ),
  DayGroupedShowListHeader: () => <div data-testid="show-list-header" />,
}))

vi.mock('./ShowListSkeleton', () => ({
  ShowListSkeleton: () => <div data-testid="show-skeleton">Loading...</div>,
}))

import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOWS_CALENDAR_FIRST_SCREEN_KEY,
} from '@/features/shows/api'
import { ShowList } from './ShowList'

const seededShows = {
  shows: [
    { id: 1, title: 'Bright Eyes', event_date: '2026-08-01T02:00:00Z', state: 'AZ', venues: [], artists: [] },
    { id: 2, title: 'Cursive', event_date: '2026-08-02T02:00:00Z', state: 'AZ', venues: [], artists: [] },
  ],
  // A total past one page on purpose: production always has more than one page,
  // so the pager is the branch that actually ships, and a single-page fixture
  // would leave it untested (`Pagination` renders null at one page).
  total: 65,
  limit: 50,
  offset: 0,
  year: 0,
  month: 0,
  day: 0,
}

const seededCities = {
  cities: [{ city: 'Phoenix', state: 'AZ', show_count: 2 }],
}

/**
 * The composition the ticket actually delivers, as opposed to its parts.
 *
 * The pieces each have their own tests: the key/URL pairing, and the mechanics
 * of `seedFirstScreen`. None of them notice if the assembled page stops
 * server-rendering, and the ways that can happen are ordinary edits: widening
 * the `isLoading && !data` gate, passing a `limit` from `ShowList`, adding a
 * query the component blocks on. Every one of those leaves this file's
 * siblings green.
 *
 * `updatedAt: 0` mirrors what `seedFirstScreen` writes, so what is rendered
 * here is what a hydrating browser renders.
 */
function renderSeeded() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 5 * 60 * 1000 } },
  })
  queryClient.setQueryData(SHOWS_CALENDAR_FIRST_SCREEN_KEY, seededShows, {
    updatedAt: 0,
  })
  queryClient.setQueryData(SHOW_CITIES_FIRST_SCREEN_KEY, seededCities, {
    updatedAt: 0,
  })

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )

  return render(<ShowList />, { wrapper })
}

describe('ShowList reading a server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockResolvedValue(seededShows)
  })

  it('renders the seeded rows on the first commit, with no skeleton', () => {
    renderSeeded()

    expect(screen.getByTestId('show-card-1')).toBeInTheDocument()
    expect(screen.getByTestId('show-card-2')).toBeInTheDocument()
    expect(screen.queryByTestId('show-skeleton')).not.toBeInTheDocument()
  })

  it('does not dim the rows it just rendered', () => {
    // The seed is stale by construction, so `isFetching` is true on this very
    // first commit. Wiring the dimming affordance to raw `isFetching` — as it
    // was before PSY-1624 — faded the server-rendered list to 60% the instant
    // it hydrated, which is the opposite of the point of rendering it.
    const { container } = renderSeeded()

    expect(container.querySelector('.opacity-60')).toBeNull()
  })

  it('serves page 2 as a real link while the seeded entry revalidates', () => {
    // The seed is stale by construction, so `isFetching` is true on the first
    // commit and in the server render. That used to matter a great deal: the
    // Load More control it replaced had to ship DISABLED, because a painted
    // button that React has not attached to yet swallows the click. A page link
    // has no such window, being an `<a href>` the browser follows with no
    // JavaScript at all, so the whole hazard is gone rather than managed.
    renderSeeded()

    // Two pagers, top and bottom, so both instances are checked rather than
    // one arbitrarily picked.
    const nextPageLinks = screen.getAllByRole('link', { name: /page 2/i })
    expect(nextPageLinks).toHaveLength(2)
    for (const link of nextPageLinks) {
      expect(link).toHaveAttribute('href', '/shows?page=2')
    }
  })

  it('serves no Load More control at all', () => {
    renderSeeded()

    expect(screen.queryByRole('button', { name: /load more/i })).toBeNull()
  })

  it('keeps the rows when a background revalidation fails', async () => {
    // `error` becomes truthy while `data` is still the server payload. An
    // unguarded `if (error)` would throw away a fully rendered first screen
    // over a refetch the reader never asked for.
    mockApiRequest.mockRejectedValue(new Error('network'))
    renderSeeded()

    // Wait for the revalidation to have actually FAILED before asserting —
    // otherwise this passes while the request is still in flight and would
    // stay green against the unguarded `if (error)` it exists to catch.
    await vi.waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalled()
    })
    await vi.waitFor(() => {
      expect(screen.queryByTestId('show-card-1')).toBeInTheDocument()
      expect(screen.queryByText(/Failed to load shows/)).not.toBeInTheDocument()
    })
    // ...and again after the rejection has settled through the query cache.
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(screen.getByTestId('show-card-1')).toBeInTheDocument()
    expect(screen.queryByText(/Failed to load shows/)).not.toBeInTheDocument()
  })

  it('reports the failure when the rows belong to a DIFFERENT query', async () => {
    // The other half of that guard. `keepPreviousData` carries `data` across a
    // key change, so a filter change whose request fails leaves the previous
    // city's shows on screen. Keeping quiet there would present them as the
    // new filter's answer — the "confident, wrong answer about the catalogue"
    // this whole change exists to avoid. Simulated by seeding an entry the
    // component does NOT ask for, so its own query starts empty and fails.
    mockApiRequest.mockRejectedValue(new Error('network'))

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 5 * 60 * 1000 } },
    })
    queryClient.setQueryData(SHOW_CITIES_FIRST_SCREEN_KEY, seededCities, {
      updatedAt: 0,
    })

    render(<ShowList />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    })

    await vi.waitFor(() => {
      expect(screen.getByText(/Failed to load shows/)).toBeInTheDocument()
    })
  })
})
