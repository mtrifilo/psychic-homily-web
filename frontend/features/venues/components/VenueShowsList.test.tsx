import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { VenueShow, VenueShowYearCount } from '../types'
import type { TimeFilter } from '../hooks/useVenues'

// ── Query stubs ────────────────────────────────────────────────────────────
// Two parallel useVenueShows results so upcoming + past fixtures can be set
// independently, plus the year histogram behind the past section's strip.
const upcomingResult = {
  data: undefined as { shows: VenueShow[]; total: number } | undefined,
  isPending: false,
  isFetching: false,
  isPlaceholderData: false,
  isError: false,
  dataUpdatedAt: 1,
  error: null as Error | null,
}
const pastResult = { ...upcomingResult }
const yearsResult = {
  data: undefined as { years: VenueShowYearCount[] } | undefined,
  isSuccess: false,
  isPending: false,
  isError: false,
}

// The past section asks for a page by offset; record what it asked for so the
// URL-to-request wiring can be asserted directly rather than inferred from
// which rows happened to render.
const pastRequests: Array<{ offset?: number; year?: number; limit?: number }> = []

vi.mock('../hooks/useVenues', () => ({
  useVenueShows: ({
    timeFilter,
    offset,
    year,
    limit,
  }: {
    timeFilter: TimeFilter
    offset?: number
    year?: number
    limit?: number
  }) => {
    if (timeFilter !== 'past') return upcomingResult
    pastRequests.push({ offset, year, limit })
    return pastResult
  },
  useVenueShowYears: () => yearsResult,
}))

// nuqs throws without a NuqsAdapter, and the adapter would need a real router.
// The component only READS these params (every write is an <a href>), so a
// plain value stub is the whole contract.
let queryYear: number | null = null
let queryPage = 1
vi.mock('nuqs', async importOriginal => ({
  // Partial: the shared filter parsers elsewhere in this import graph build on
  // nuqs's real `createParser`, so only the hook is swapped out.
  ...(await importOriginal<typeof import('nuqs')>()),
  useQueryState: (key: string) =>
    key === 'year' ? [queryYear, vi.fn()] : [queryPage, vi.fn()],
}))

// ShowForm pulls in a lot of form/mutation plumbing the suite doesn't need.
// Render a thin stub so we can assert open/close + submit/cancel handlers.
vi.mock('@/features/shows', () => ({
  ShowForm: ({
    onCancel,
    onSuccess,
    prefilledVenue,
  }: {
    onCancel: () => void
    onSuccess: () => void
    prefilledVenue: { name: string }
  }) => (
    <div data-testid="show-form">
      Form for {prefilledVenue.name}
      <button onClick={onCancel}>cancel</button>
      <button onClick={onSuccess}>save</button>
    </div>
  ),
}))

const mockAuthIsAuthenticated = { value: false }
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockAuthIsAuthenticated.value,
    user: mockAuthIsAuthenticated.value ? { id: 1 } : null,
    isLoading: false,
  }),
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="notify-me-button">Notify me about {entityName}</button>
  ),
}))

// `@/components/shared` is deliberately NOT mocked: Pagination, YearStrip and
// DenseTable are the behaviour under test here, not incidental chrome.

import { VenueShowsList } from './VenueShowsList'

// ── Fixtures ───────────────────────────────────────────────────────────────

function makeShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    price: 15,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    artists: [
      {
        id: 42,
        slug: 'main-artist',
        name: 'Main Artist',
        set_type: 'headliner',
        position: 1,
        is_headliner: true,
        socials: {},
      },
      {
        id: 99,
        slug: 'opener',
        name: 'The Opener',
        set_type: 'direct_support',
        position: 2,
        is_headliner: false,
        socials: {},
      },
    ],
    ...overrides,
  }
}

type QueryState = {
  isPending?: boolean
  isFetching?: boolean
  isPlaceholderData?: boolean
  error?: Error | null
}

function applyState(
  target: typeof upcomingResult,
  data: { shows: VenueShow[]; total?: number } | null,
  opts?: QueryState
) {
  target.data = data
    ? { shows: data.shows, total: data.total ?? data.shows.length }
    : undefined
  target.isPending = opts?.isPending ?? false
  target.isFetching = opts?.isFetching ?? false
  target.isPlaceholderData = opts?.isPlaceholderData ?? false
  target.error = opts?.error ?? null
  target.isError = Boolean(opts?.error)
  target.dataUpdatedAt = data ? 1 : 0
}

const setUpcoming = (
  data: { shows: VenueShow[]; total?: number } | null,
  opts?: QueryState
) => applyState(upcomingResult, data, opts)

const setPast = (
  data: { shows: VenueShow[]; total?: number } | null,
  opts?: QueryState
) => applyState(pastResult, data, opts)

function setYears(years: VenueShowYearCount[] | null) {
  yearsResult.data = years ? { years } : undefined
  yearsResult.isSuccess = years !== null
  yearsResult.isPending = years === null
  yearsResult.isError = false
}

function renderList(overrides?: Partial<Parameters<typeof VenueShowsList>[0]>) {
  return renderWithProviders(
    <VenueShowsList
      venueId={7}
      venueSlug="the-venue"
      venueName="The Venue"
      venueCity="Phoenix"
      venueState="AZ"
      venueTimezone="America/Phoenix"
      {...overrides}
    />
  )
}

/** The past section's `<section>`, scoped so upcoming rows never leak in. */
const pastSection = () =>
  within(document.getElementById('venue-past-shows') as HTMLElement)

beforeEach(() => {
  setUpcoming(null, { isPending: true })
  setPast(null, { isPending: true })
  setYears(null)
  mockAuthIsAuthenticated.value = false
  pastRequests.length = 0
  queryYear = null
  queryPage = 1
})

afterEach(() => {
  document.title = ''
})

// ── Upcoming ───────────────────────────────────────────────────────────────

describe('VenueShowsList — upcoming section', () => {
  it('renders the Upcoming shows heading always', () => {
    renderList()
    expect(
      screen.getByRole('heading', { name: 'Upcoming shows' })
    ).toBeInTheDocument()
  })

  it('shows a loader while upcoming shows are loading', () => {
    renderList()
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows an error message when the upcoming fetch fails', () => {
    setUpcoming(null, { error: new Error('boom') })
    renderList()
    expect(screen.getByText(/Failed to load shows/i)).toBeInTheDocument()
  })

  it('renders an inline [Notify me] affordance when there are no upcoming shows', () => {
    setUpcoming({ shows: [] })
    renderList({ venueName: 'Rebel Lounge' })
    expect(screen.getByText(/No upcoming shows yet/i)).toBeInTheDocument()
    expect(screen.getByTestId('notify-me-button')).toHaveTextContent(
      'Rebel Lounge'
    )
  })

  it('renders upcoming shows as a table with the total beside the heading', () => {
    setUpcoming({ shows: [makeShow()], total: 83 })
    renderList()
    const table = screen.getByRole('table', { name: 'Upcoming shows' })
    expect(within(table).getByText('Main Artist')).toBeInTheDocument()
    expect(within(table).getByText('The Opener')).toBeInTheDocument()
    expect(screen.getByText('83')).toBeInTheDocument()
  })

  it('does not group upcoming shows into month headings', () => {
    setUpcoming({ shows: [makeShow()] })
    renderList()
    const table = screen.getByRole('table', { name: 'Upcoming shows' })
    expect(
      within(table).queryByRole('rowheader', { name: /2025/ })
    ).not.toBeInTheDocument()
  })

  it('discloses truncation only when the backend cap actually bit', () => {
    setUpcoming({ shows: [makeShow()], total: 1 })
    const { unmount } = renderList()
    expect(screen.queryByText(/Showing the next/)).not.toBeInTheDocument()
    unmount()

    setUpcoming({ shows: [makeShow()], total: 240 })
    renderList()
    expect(
      screen.getByText(/Showing the next 1 of 240 announced shows/)
    ).toBeInTheDocument()
  })
})

// ── Past archive: presence ─────────────────────────────────────────────────

describe('VenuePastShows — presence', () => {
  it('omits the past section entirely for a venue with no past shows', () => {
    setUpcoming({ shows: [makeShow()] })
    setPast({ shows: [] })
    setYears([])
    renderList()
    expect(document.getElementById('venue-past-shows')).toBeNull()
  })

  it('renders the past archive expanded, with no [Show] toggle', () => {
    setUpcoming({ shows: [] })
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    setYears([{ year: 2025, count: 1 }])
    renderList()
    expect(
      screen.getByRole('table', { name: 'Past shows' })
    ).toBeInTheDocument()
    expect(screen.queryByText('[Show]')).not.toBeInTheDocument()
  })

  it('keeps the section for an empty hand-typed year and offers the way out', () => {
    queryYear = 1999
    setUpcoming({ shows: [] })
    setPast({ shows: [], total: 0 })
    setYears([{ year: 2025, count: 60 }])
    renderList()
    expect(screen.getByText(/No past shows in 1999/)).toBeInTheDocument()
    expect(
      pastSection().getByRole('link', { name: 'Show every year' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
  })

  it('says so rather than redirecting when the page is past the end', () => {
    queryPage = 40
    setUpcoming({ shows: [] })
    setPast({ shows: [], total: 60 })
    setYears([{ year: 2025, count: 60 }])
    renderList()
    expect(screen.getByText(/past the end of this archive/)).toBeInTheDocument()
    expect(
      pastSection().getByRole('link', { name: 'Back to the first page' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
  })
})

// ── Past archive: rows ─────────────────────────────────────────────────────

describe('VenuePastShows — rows', () => {
  beforeEach(() => {
    setUpcoming({ shows: [] })
    setYears([{ year: 2025, count: 3 }])
  })

  it('links the date to the show slug, falling back to the id only without one', () => {
    setPast({
      shows: [
        makeShow({ id: 5, slug: 'ripe-at-valley-bar' }),
        makeShow({ id: 6, slug: '', event_date: '2025-06-14T20:00:00Z' }),
      ],
      total: 2,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: /Jun 15/ })
    ).toHaveAttribute('href', '/shows/ripe-at-valley-bar')
    expect(
      within(table).getByRole('link', { name: /Jun 14/ })
    ).toHaveAttribute('href', '/shows/6')
  })

  it('emphasizes the headliner and reads the support acts as "w/"', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: 'Main Artist' }).closest('.font-medium')
    ).not.toBeNull()
    expect(within(table).getByText(/w\//)).toBeInTheDocument()
    expect(
      within(table).getByRole('link', { name: 'The Opener' }).closest('.font-medium')
    ).toBeNull()
  })

  it('leads with the flagged headliner even when it is not listed first', () => {
    setPast({
      shows: [
        makeShow({
          id: 5,
          artists: [
            {
              id: 1,
              slug: 'opener',
              name: 'Opener',
              set_type: 'direct_support',
              position: 1,
              is_headliner: false,
              socials: {},
            },
            {
              id: 2,
              slug: 'top',
              name: 'Top Billing',
              set_type: 'headliner',
              position: 2,
              is_headliner: true,
              socials: {},
            },
          ],
        }),
      ],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: 'Top Billing' }).closest('.font-medium')
    ).not.toBeNull()
    expect(
      within(table).getByRole('link', { name: 'Opener' }).closest('.font-medium')
    ).toBeNull()
  })

  it('leads with the curated set_type when only that names the headliner', () => {
    // `set_type` is authoritative over the older `is_headliner` flag, and shows
    // written before the roles existed carry only the flag. Both have to count,
    // or the same show headlines differently here and on a show card.
    setPast({
      shows: [
        makeShow({
          id: 5,
          artists: [
            {
              id: 1,
              slug: 'opener',
              name: 'Opener',
              set_type: 'opener',
              position: 1,
              is_headliner: false,
              socials: {},
            },
            {
              id: 2,
              slug: 'curated',
              name: 'Curated Lead',
              set_type: 'headliner',
              position: 2,
              is_headliner: false,
              socials: {},
            },
          ],
        }),
      ],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: 'Curated Lead' }).closest('.font-medium')
    ).not.toBeNull()
  })

  it('badges cancelled shows, and suppresses sold-out on a cancelled one', () => {
    setPast({
      shows: [
        makeShow({ id: 5, is_cancelled: true, is_sold_out: true }),
        makeShow({
          id: 6,
          is_sold_out: true,
          event_date: '2025-06-14T20:00:00Z',
        }),
      ],
      total: 2,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getAllByText('CANCELLED')).toHaveLength(1)
    expect(within(table).getAllByText('SOLD OUT')).toHaveLength(1)
  })

  it('renders a price, and an en dash (never an em dash) when there is none', () => {
    setPast({
      shows: [
        makeShow({ id: 5, price: 22 }),
        makeShow({ id: 6, price: null, event_date: '2025-06-14T20:00:00Z' }),
      ],
      total: 2,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getByText('$22.00')).toBeInTheDocument()
    expect(within(table).getAllByText('–').length).toBeGreaterThan(0)
    expect(within(table).queryByText('—')).not.toBeInTheDocument()
  })

  it('groups past rows under month headings, skipping months with no shows', () => {
    setPast({
      shows: [
        makeShow({ id: 5, event_date: '2025-09-20T03:00:00Z' }),
        makeShow({ id: 6, event_date: '2025-09-06T03:00:00Z' }),
        makeShow({ id: 7, event_date: '2025-06-06T03:00:00Z' }),
      ],
      total: 3,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    const headings = within(table)
      .getAllByRole('rowheader')
      .map(cell => cell.textContent)
    expect(headings).toEqual(['Sep 2025', 'Jun 2025'])
  })
})

// ── Past archive: URL state ────────────────────────────────────────────────

describe('VenuePastShows — year and page state', () => {
  const threeYears: VenueShowYearCount[] = [
    { year: 2026, count: 34 },
    { year: 2025, count: 161 },
    { year: 2024, count: 58 },
  ]

  beforeEach(() => {
    setUpcoming({ shows: [] })
    setPast({ shows: [makeShow({ id: 5 })], total: 253 })
    setYears(threeYears)
  })

  it('renders a year strip whose links are bare, page-free and anchored', () => {
    renderList()
    const strip = screen.getByRole('navigation', {
      name: 'Filter past shows by year',
    })
    expect(within(strip).getByRole('link', { name: /2025/ })).toHaveAttribute(
      'href',
      '/venues/the-venue?year=2025#venue-past-shows'
    )
    expect(
      within(strip).getByRole('link', { name: 'All years' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
  })

  it('never emits ?page=1: page 1 links are bare', () => {
    queryPage = 2
    renderList()
    const pagers = screen.getAllByRole('navigation', { name: /pagination/i })
    expect(within(pagers[0]).getByRole('link', { name: 'Page 1' })).toHaveAttribute(
      'href',
      '/venues/the-venue#venue-past-shows'
    )
  })

  it('carries the active year into every page link', () => {
    queryYear = 2025
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(within(pager).getByRole('link', { name: /^Page 2/ })).toHaveAttribute(
      'href',
      '/venues/the-venue?year=2025&page=2#venue-past-shows'
    )
  })

  it('translates the page param into an offset request', () => {
    queryPage = 3
    queryYear = 2025
    renderList()
    expect(pastRequests[0]).toMatchObject({ offset: 100, year: 2025, limit: 50 })
  })

  it('reads a hand-edited nonsense year as the unfiltered archive', () => {
    queryYear = 1_759_000_000
    renderList()
    expect(pastRequests[0].year).toBeUndefined()
    expect(
      screen.getByRole('heading', { name: 'Past shows' })
    ).toBeInTheDocument()
  })

  it('bounds a hand-edited runaway page instead of forwarding the offset', () => {
    queryPage = 999_999
    renderList()
    expect(pastRequests[0].offset).toBe(999 * 50)
  })

  it('renders a pager above and below the table, with distinct names', () => {
    renderList()
    const pagers = screen.getAllByRole('navigation', { name: /pagination/i })
    expect(pagers).toHaveLength(2)
    expect(
      new Set(pagers.map(nav => nav.getAttribute('aria-label'))).size
    ).toBe(2)
  })

  it('hides the pagers when the whole archive fits on one page', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 3 })
    setYears([{ year: 2025, count: 3 }])
    renderList()
    expect(
      screen.queryByRole('navigation', { name: /pagination/i })
    ).not.toBeInTheDocument()
  })

  it('scrolls a cold #venue-past-shows deep link onto the archive', () => {
    // The browser resolves the fragment before this section exists, so the
    // component has to honour it once its own rows are on screen.
    const scrollIntoView = vi.fn()
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    window.location.hash = '#venue-past-shows'
    try {
      renderList()
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
    } finally {
      Element.prototype.scrollIntoView = original
      window.location.hash = ''
    }
  })

  it('leaves the scroll position alone without our fragment', () => {
    const scrollIntoView = vi.fn()
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    window.location.hash = '#venue-shows'
    try {
      renderList()
      expect(scrollIntoView).not.toHaveBeenCalled()
    } finally {
      Element.prototype.scrollIntoView = original
      window.location.hash = ''
    }
  })

  it('moves focus to the past-shows heading on a client-side page change', () => {
    renderList()
    const heading = screen.getByRole('heading', { name: /Past shows/ })
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    fireEvent.click(within(pager).getByRole('link', { name: /^Page 2/ }))
    expect(heading).toHaveFocus()
  })
})

// ── Past archive: filter reflection ────────────────────────────────────────

describe('VenuePastShows — filter reflection', () => {
  const years: VenueShowYearCount[] = [
    { year: 2026, count: 34 },
    { year: 2025, count: 161 },
    { year: 2024, count: 217 },
  ]

  beforeEach(() => {
    setUpcoming({ shows: [] })
    setPast({ shows: [makeShow({ id: 5 })], total: 412 })
    setYears(years)
    document.title = 'The Venue | Psychic Homily'
  })

  it('heads the unfiltered archive with the all-time total', () => {
    renderList()
    expect(
      screen.getByRole('heading', { name: 'Past shows' })
    ).toBeInTheDocument()
    expect(pastSection().getByText('412')).toBeInTheDocument()
  })

  it('rescopes the heading and count to the active year', () => {
    queryYear = 2025
    renderList()
    expect(
      screen.getByRole('heading', { name: 'Past shows in 2025' })
    ).toBeInTheDocument()
    expect(pastSection().getByText('161 of 412 all-time')).toBeInTheDocument()
  })

  it('leaves the document title alone on the default view', () => {
    renderList()
    expect(document.title).toBe('The Venue | Psychic Homily')
  })

  it('carries the year and page in the document title', () => {
    queryYear = 2025
    queryPage = 2
    renderList()
    expect(document.title).toBe(
      'The Venue shows in 2025 (page 2 of 4) | Psychic Homily'
    )
  })

  it('restores the route title when the archive unmounts', () => {
    queryYear = 2025
    const { unmount } = renderList()
    expect(document.title).toContain('shows in 2025')
    unmount()
    expect(document.title).toBe('The Venue | Psychic Homily')
  })

  it('dims the rows only while they answer a different question', () => {
    setPast(
      { shows: [makeShow({ id: 5 })], total: 412 },
      { isFetching: true, isPlaceholderData: true }
    )
    const { unmount } = renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(table.closest('.opacity-60')).not.toBeNull()
    unmount()

    // A same-key background revalidation must NOT fade a list that is not
    // changing (the ShowList.tsx gate, not raw isFetching).
    setPast(
      { shows: [makeShow({ id: 5 })], total: 412 },
      { isFetching: true, isPlaceholderData: false }
    )
    renderList()
    expect(
      screen.getByRole('table', { name: 'Past shows' }).closest('.opacity-60')
    ).toBeNull()
  })
})

// ── Add-show affordance ────────────────────────────────────────────────────

describe('VenueShowsList — add-show affordance', () => {
  beforeEach(() => {
    setUpcoming({ shows: [makeShow()] })
    setPast({ shows: [] })
    setYears([])
  })

  it('does not render the add-show button for unauthenticated users', () => {
    mockAuthIsAuthenticated.value = false
    renderList()
    expect(
      screen.queryByRole('button', { name: /Add a show/i })
    ).not.toBeInTheDocument()
  })

  it('renders the add-show button for authenticated users', () => {
    mockAuthIsAuthenticated.value = true
    renderList({ venueName: 'Rebel Lounge' })
    expect(
      screen.getByRole('button', { name: /Add a show at Rebel Lounge/i })
    ).toBeInTheDocument()
  })

  it('toggles the ShowForm open when the add-show button is clicked', async () => {
    const user = userEvent.setup()
    mockAuthIsAuthenticated.value = true
    renderList()
    await user.click(screen.getByRole('button', { name: /Add a show/i }))
    expect(screen.getByTestId('show-form')).toBeInTheDocument()
  })

  it('closes the ShowForm and calls onShowAdded on successful submit', async () => {
    const user = userEvent.setup()
    const onShowAdded = vi.fn()
    mockAuthIsAuthenticated.value = true
    renderList({ onShowAdded })
    await user.click(screen.getByRole('button', { name: /Add a show/i }))
    await user.click(screen.getByRole('button', { name: 'save' }))
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    expect(onShowAdded).toHaveBeenCalled()
  })
})
