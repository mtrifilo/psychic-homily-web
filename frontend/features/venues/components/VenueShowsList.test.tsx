import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type {
  VenueShow,
  VenueShowMonthCount,
  VenueShowYearCount,
} from '../types'
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
// The month histogram the pager labels its page links from (PSY-1769). Separate
// from the year histogram because it answers a different question: years say
// which pages exist, months say what is behind each one.
const monthsResult = {
  data: undefined as { months: VenueShowMonthCount[] } | undefined,
  isSuccess: false,
  isPending: false,
  isError: false,
}

// The past section asks for a page by offset; record what it asked for so the
// URL-to-request wiring can be asserted directly rather than inferred from
// which rows happened to render.
const pastRequests: Array<{
  offset?: number
  year?: number
  limit?: number
  enabled?: boolean
}> = []

/**
 * Whether the month histogram was actually asked for. It is gated on the pager
 * existing at all (PSY-1769), and a gate is invisible in the rendered output —
 * the labels are absent either way.
 */
const monthsRequests: Array<{ enabled?: boolean }> = []

vi.mock('../hooks/useVenues', () => ({
  useVenueShows: ({
    timeFilter,
    offset,
    year,
    limit,
    enabled,
  }: {
    timeFilter: TimeFilter
    offset?: number
    year?: number
    limit?: number
    enabled?: boolean
  }) => {
    if (timeFilter !== 'past') return upcomingResult
    pastRequests.push({ offset, year, limit, enabled })
    return pastResult
  },
  useVenueShowYears: () => yearsResult,
  useVenueShowMonths: ({ enabled }: { enabled?: boolean }) => {
    monthsRequests.push({ enabled })
    return monthsResult
  },
}))

// nuqs throws without a NuqsAdapter, and the adapter would need a real router.
// The component only READS `?page=` (every write is an <a href>), so a plain
// value stub is the whole contract. There is no `?year=` any more: the year is
// a path segment, so it arrives as a prop (PSY-1756).
let queryPage = 1
vi.mock('nuqs', async importOriginal => ({
  // Partial: the shared filter parsers elsewhere in this import graph build on
  // nuqs's real `createParser`, so only the hook is swapped out.
  ...(await importOriginal<typeof import('nuqs')>()),
  useQueryState: () => [queryPage, vi.fn()],
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
import { VenuePastShows } from './VenuePastShows'

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

function setMonths(months: VenueShowMonthCount[] | null) {
  monthsResult.data = months ? { months } : undefined
  monthsResult.isSuccess = months !== null
  monthsResult.isPending = months === null
  monthsResult.isError = false
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

/**
 * The archive on its own, scoped to one year — what the
 * `/venues/{slug}/shows/{year}` route renders (PSY-1756).
 *
 * The venue page has no year scope any more, so a year-scoped case cannot go
 * through `renderList`: it would be asserting a state that surface can no longer
 * be in. Same component either way, which is the point.
 */
function renderArchive(
  activeYear: number,
  overrides?: Partial<Parameters<typeof VenuePastShows>[0]>
) {
  return renderWithProviders(
    <VenuePastShows
      venueId={7}
      venueSlug="the-venue"
      venueName="The Venue"
      venueState="AZ"
      venueTimezone="America/Phoenix"
      activeYear={activeYear}
      {...overrides}
    />
  )
}

/**
 * Let a pending animation frame run.
 *
 * The archive's re-align window coalesces its scrolls through one
 * `requestAnimationFrame` so a burst of resizes costs one forced layout rather
 * than one each — which means a resize fired in a test has not done anything yet
 * when `fireResize` returns.
 */
const flushFrame = () =>
  act(async () => {
    await new Promise(resolve => setTimeout(resolve, 32))
  })

/** The past section's `<section>`, scoped so upcoming rows never leak in. */
const pastSection = () =>
  within(document.getElementById('venue-past-shows') as HTMLElement)

beforeEach(() => {
  setUpcoming(null, { isPending: true })
  setPast(null, { isPending: true })
  setYears(null)
  setMonths(null)
  mockAuthIsAuthenticated.value = false
  pastRequests.length = 0
  monthsRequests.length = 0
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

  it('keeps the section for an empty year and offers the way out', () => {
    setUpcoming({ shows: [] })
    setPast({ shows: [], total: 0 })
    setYears([{ year: 2025, count: 60 }])
    // The route 404s a year the histogram does not carry, so this state is not
    // reachable by hand-editing the URL any more. It survives as a component
    // contract because the histogram and the page can still disagree across a
    // revalidation boundary, and the section must say what is empty rather than
    // render a bare table with nothing in it.
    renderArchive(1999)
    expect(screen.getByText(/No past shows in 1999/)).toBeInTheDocument()
    expect(
      pastSection().getByRole('link', { name: 'Show every year' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
  })

  it('says so rather than redirecting when the page is past the end', () => {
    queryPage = 40
    setUpcoming({ shows: [] })
    setPast(null, { isPending: true })
    setYears([{ year: 2025, count: 60 }])
    renderList()
    expect(screen.getByText(/past the end of this archive/)).toBeInTheDocument()
    expect(
      pastSection().getByRole('link', { name: 'Back to the first page' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
    // And the histogram already knew, so no 50,000-row offset scan was spent
    // proving it. A spinner must never be the terminal state here.
    expect(pastRequests[0].enabled).toBe(false)
    expect(document.querySelector('.animate-spin')).toBeNull()
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

  it('still badges a cancelled show that has no bill at all', () => {
    // The backend's minimum-one-artist validation is inert and its artist
    // resolution skips ids it cannot resolve, so an empty bill reaches this
    // table — and a cancelled show with no bill is the one row where the
    // status is the only thing the row has to say.
    setPast({
      shows: [makeShow({ id: 5, artists: [], is_cancelled: true })],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getByText('CANCELLED')).toBeInTheDocument()
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
    // A year is a PATH, not a query variant of the venue page: it is a document
    // of its own, with its own canonical and title (PSY-1756).
    expect(within(strip).getByRole('link', { name: /2025/ })).toHaveAttribute(
      'href',
      '/venues/the-venue/shows/2025'
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

  it('pages within a year on the year path, never back onto ?year=', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    renderArchive(2025)
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(within(pager).getByRole('link', { name: /^Page 2/ })).toHaveAttribute(
      'href',
      '/venues/the-venue/shows/2025?page=2'
    )
  })

  it('translates the page param into an offset request', () => {
    queryPage = 3
    renderArchive(2025)
    expect(pastRequests[0]).toMatchObject({ offset: 100, year: 2025, limit: 50 })
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

  // PSY-1769. The upcoming list above this section and (on mobile) the whole
  // sidebar resolve after the archive does, pushing it down again — so landing
  // on it once is landing on it before most of the page exists.
  it('re-aligns the archive while the page above it is still settling', async () => {
    const scrollIntoView = vi.fn()
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    const resizeObserver = installImmediateResizeObserver()
    window.location.hash = '#venue-past-shows'
    try {
      renderList()
      await flushFrame()
      const afterLanding = scrollIntoView.mock.calls.length
      resizeObserver.fireResize(1024)
      await flushFrame()
      expect(scrollIntoView.mock.calls.length).toBeGreaterThan(afterLanding)
    } finally {
      resizeObserver.restore()
      Element.prototype.scrollIntoView = originalScroll
      window.location.hash = ''
    }
  })

  // The restraint that makes the above safe: re-aligning a page the reader has
  // started moving is the worse failure, and is why this used to be a one-shot.
  it('stops re-aligning the moment the reader takes over the scroll', async () => {
    const scrollIntoView = vi.fn()
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    const resizeObserver = installImmediateResizeObserver()
    window.location.hash = '#venue-past-shows'
    try {
      renderList()
      fireEvent.wheel(window)
      await flushFrame()
      const afterHandOver = scrollIntoView.mock.calls.length
      resizeObserver.fireResize(1024)
      await flushFrame()
      expect(scrollIntoView).toHaveBeenCalledTimes(afterHandOver)
    } finally {
      resizeObserver.restore()
      Element.prototype.scrollIntoView = originalScroll
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

  // PSY-1769. Before the month histogram, a label could only be derived from a
  // page's own rows, so pages the reader had not visited rendered bare numerals
  // — which is most of the strip on first paint.
  it('labels every page in the strip, including ones never fetched', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    setMonths([
      // A month outside the active year, first in the list: an unfiltered walk
      // would start page 1 here and shift every label in the strip.
      { year: 2026, month: 3, count: 34 },
      { year: 2025, month: 12, count: 30 },
      { year: 2025, month: 11, count: 30 },
      { year: 2025, month: 10, count: 30 },
      { year: 2025, month: 9, count: 30 },
      { year: 2025, month: 8, count: 30 },
      { year: 2025, month: 7, count: 11 },
    ])
    renderArchive(2025)
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    for (const name of [
      'Page 1, Dec–Nov',
      'Page 2, Nov–Sep',
      'Page 3, Sep–Aug',
      'Page 4, Jul',
    ]) {
      expect(within(pager).getByRole('link', { name })).toBeInTheDocument()
    }
  })

  // The all-years pager has no year in context — the strip above it has nothing
  // selected — so a label that elided the year would give two pages years apart
  // the same accessible name.
  it('keeps the year on every label of the all-years pager', () => {
    setYears([
      { year: 2025, count: 60 },
      { year: 2024, count: 40 },
    ])
    setPast({ shows: [makeShow({ id: 5 })], total: 100 })
    setMonths([
      { year: 2025, month: 1, count: 60 },
      { year: 2024, month: 12, count: 40 },
    ])
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(
      within(pager).getByRole('link', { name: 'Page 1, Jan 2025' })
    ).toBeInTheDocument()
    expect(
      within(pager).getByRole('link', { name: 'Page 2, Jan 2025–Dec 2024' })
    ).toBeInTheDocument()
  })

  it('distinguishes two same-month pages years apart in the VISIBLE strip', () => {
    setYears([
      { year: 2025, count: 50 },
      { year: 2023, count: 50 },
    ])
    setPast({ shows: [makeShow({ id: 5 })], total: 100 })
    // The same month, two years apart — indistinguishable if the year is elided.
    setMonths([
      { year: 2025, month: 8, count: 50 },
      { year: 2023, month: 8, count: 50 },
    ])
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    // The rendered text, not the accessible name: the name is always prefixed
    // "Page N" and so can never collide. It is the strip a sighted reader
    // chooses from that would show "Aug" twice.
    const visible = within(pager)
      .getAllByRole('link')
      .map(link => link.textContent ?? '')
      .filter(text => /Aug/.test(text))
    expect(visible).toHaveLength(2)
    expect(new Set(visible).size).toBe(2)
  })

  // A failed histogram must not strip the label from EVERY page link — the
  // shape this replaced always labelled at least the page being read, and below
  // `sm` the pager renders no page links at all, so the current page's label is
  // the only one there is.
  it('falls back to the rows on screen when the histogram fails', () => {
    queryPage = 2
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    setMonths(null)
    monthsResult.isError = true
    renderArchive(2025)
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    // makeShow's fixture date is June 2025, read on the venue's calendar.
    expect(
      within(pager).getByRole('link', { name: 'Page 2, Jun' })
    ).toBeInTheDocument()
    // ...and the pages it cannot know about stay bare rather than guessing.
    expect(
      within(pager).getByRole('link', { name: 'Page 3' })
    ).toBeInTheDocument()
  })

  it('does not ask for the histogram when the archive fits on one page', () => {
    // Labels are pager chrome, and a one-page archive renders no pager. The
    // same gate covers a venue with no past shows at all, whose `totalPages`
    // floors at 1.
    setPast({ shows: [makeShow({ id: 5 })], total: 3 })
    setYears([{ year: 2025, count: 3 }])
    renderList()
    expect(monthsRequests.length).toBeGreaterThan(0)
    expect(monthsRequests.every(request => request.enabled === false)).toBe(true)

    // ...and it IS asked for once there is a pager to label.
    monthsRequests.length = 0
    setPast({ shows: [makeShow({ id: 5 })], total: 253 })
    setYears(threeYears)
    renderList()
    expect(monthsRequests.some(request => request.enabled === true)).toBe(true)
  })

  it('falls back to bare numerals while the histogram is still loading', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    setMonths(null)
    renderArchive(2025)
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(
      within(pager).getByRole('link', { name: 'Page 3' })
    ).toBeInTheDocument()
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
    renderArchive(2025)
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
    queryPage = 2
    renderArchive(2025)
    expect(document.title).toBe(
      'The Venue shows in 2025 (page 2 of 4) | Psychic Homily'
    )
  })

  it('leaves a title another writer has replaced alone on unmount', () => {
    // On a soft navigation the next route's <title> is committed before this
    // effect's cleanup runs, so an unconditional restore would relabel the page
    // the reader just opened.
    queryPage = 2
    const { unmount } = renderArchive(2025)
    expect(document.title).toContain('shows in 2025')
    document.title = 'Some Other Page | Psychic Homily'
    unmount()
    expect(document.title).toBe('Some Other Page | Psychic Homily')
  })

  it('restores the route title when the archive unmounts', () => {
    queryPage = 2
    const { unmount } = renderArchive(2025)
    expect(document.title).toContain('shows in 2025')
    unmount()
    expect(document.title).toBe('The Venue | Psychic Homily')
  })

  it('states no row range while the rows belong to the previous page', () => {
    // `keepPreviousData` holds the outgoing page on screen. A caption is an
    // exact claim ("Showing 51-100 of 412") and would be WRONG, not merely
    // stale, over rows 1-50 — and the pager latches its announcement on that
    // first render, so a label taken from them is never corrected.
    queryPage = 2
    setPast(
      { shows: [makeShow({ id: 5 })], total: 412 },
      { isFetching: true, isPlaceholderData: true }
    )
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(within(pager).getAllByText(/Page 2 of/).length).toBeGreaterThan(0)
    expect(within(pager).queryByText(/Showing/)).not.toBeInTheDocument()
    // No month-span label for the current page either, for the same reason:
    // the accessible name stays a bare "Page 2".
    expect(
      within(pager).getByRole('link', { name: 'Page 2' })
    ).toBeInTheDocument()
  })

  it('keeps a way out of a failed page', () => {
    // A venue with one year renders no year strip, so without these the only
    // escape from a failed page 2 is hand-editing the URL.
    queryPage = 2
    setYears([{ year: 2025, count: 412 }])
    setPast(null, { error: new Error('boom') })
    renderList()
    expect(
      pastSection().getByRole('button', { name: /Try again/i })
    ).toBeInTheDocument()
    expect(
      pastSection().getByRole('link', { name: 'Back to the first page' })
    ).toHaveAttribute('href', '/venues/the-venue#venue-past-shows')
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
