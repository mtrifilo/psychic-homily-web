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
  useSearchParams: () => ({ get: () => null }),
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

vi.mock('./ShowCard', () => ({
  ShowCard: ({ show }: { show: { id: number; title: string } }) => (
    <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  ),
}))

vi.mock('./ShowListSkeleton', () => ({
  ShowListSkeleton: () => <div data-testid="show-skeleton">Loading...</div>,
}))

import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
} from '@/features/shows/api'
import { ShowList } from './ShowList'

const seededShows = {
  shows: [
    { id: 1, title: 'Bright Eyes', event_date: '2026-08-01T02:00:00Z', state: 'AZ', venues: [], artists: [] },
    { id: 2, title: 'Cursive', event_date: '2026-08-02T02:00:00Z', state: 'AZ', venues: [], artists: [] },
  ],
  // `has_more: true` on purpose: production always has more than one page, so
  // the Load More branch is the one that actually ships, and it is the branch
  // a `has_more: false` fixture would silently leave untested.
  pagination: { has_more: true, next_cursor: 'abc123' },
  total: 65,
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
  queryClient.setQueryData(UPCOMING_SHOWS_FIRST_SCREEN_KEY, seededShows, {
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

  it('holds Load More disabled while the seeded entry revalidates', () => {
    // Deliberate, and the opposite of what it looks like. The seed is stale by
    // construction, so `isFetching` is true on the first commit and in the
    // server render, which means this control ships disabled and labelled
    // "Loading...". An adversarial reviewer flagged that as a dead control in
    // server HTML, and gating it on `isPlaceholderData` instead does make it
    // live. It also makes it CLICKABLE BEFORE REACT ATTACHES, and that click is
    // dropped: `e2e/pages/shows.spec.ts` "pagination loads more shows" fails
    // exactly then, clicking a painted button and timing out waiting for rows
    // that were never requested. The `replayOnHydrate` root below does not
    // rescue it, which is measured rather than assumed (bisected: reverting
    // only this gate turns that spec green again).
    //
    // So the honest state is "not ready yet" rather than a live control that
    // eats the interaction. The cost is a JS-less fetcher seeing a disabled
    // pagination button, which costs it nothing: every row it can reach without
    // paginating is already in the HTML. Revisit alongside the replay
    // primitive, not on its own.
    renderSeeded()

    // Queried by the "Loading..." label rather than "Load More", because that
    // relabelling is part of the state being pinned here.
    const button = screen.getByRole('button', { name: /loading/i })
    expect(button).toBeDisabled()
    // Kept regardless: the moment the gate above changes, this is what stops
    // the pre-hydration click being silently swallowed.
    expect(button).toHaveAttribute('data-replay-on-hydrate')
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
