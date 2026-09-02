import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type {
  ArtistShow,
  ArtistShowYearCount,
  ArtistTimeFilter,
} from '../types'

// ── Query stubs ────────────────────────────────────────────────────────────
// Two parallel useArtistShows results so upcoming + past fixtures can be set
// independently, plus the year histogram behind the past section's strip.
const upcomingResult = {
  data: undefined as { shows: ArtistShow[]; total: number } | undefined,
  isPending: false,
  isFetching: false,
  isPlaceholderData: false,
  isError: false,
  isSuccess: false,
  error: null as Error | null,
}
const pastResult = { ...upcomingResult }
const yearsResult = {
  data: undefined as { years: ArtistShowYearCount[] } | undefined,
  isSuccess: false,
  isPending: false,
  isError: false,
}
// The month histogram behind the pager's range labels (PSY-1842). Empty by
// default: this suite is about the archive's rows, URLs and chrome, and the
// label derivation itself is pinned in the shared component's own suite. The
// current page still gets a label from its rows, which is the fallback path.
const monthsResult = {
  data: undefined as
    | { months: Array<{ year: number; month: number; count: number }> }
    | undefined,
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

vi.mock('../hooks/useArtists', () => ({
  useArtistShows: ({
    timeFilter,
    offset,
    year,
    limit,
    enabled,
  }: {
    timeFilter: ArtistTimeFilter
    offset?: number
    year?: number
    limit?: number
    enabled?: boolean
  }) => {
    if (timeFilter !== 'past') return upcomingResult
    pastRequests.push({ offset, year, limit, enabled })
    return pastResult
  },
  useArtistShowYears: () => yearsResult,
  useArtistShowMonths: () => monthsResult,
}))

// nuqs throws without a NuqsAdapter, and the adapter would need a real router.
// The component only READS these params (every write is an <a href>), so a
// plain value stub is the whole contract.
let queryYear: number | null = null
let queryPage = 1
// The archive builds its hrefs from the params already on the URL, so the test
// controls that set directly. Kept separate from the nuqs stub above because it
// is a different question: nuqs says what the archive IS showing, this says what
// ELSE is on the URL that the archive has to carry through.
let otherParams = ''
vi.mock('next/navigation', async importOriginal => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useSearchParams: () => new URLSearchParams(otherParams),
}))
vi.mock('nuqs', async importOriginal => ({
  // Partial: the shared filter parsers elsewhere in this import graph build on
  // nuqs's real `createParser`, so only the hook is swapped out.
  ...(await importOriginal<typeof import('nuqs')>()),
  useQueryState: (key: string) =>
    key === 'year' ? [queryYear, vi.fn()] : [queryPage, vi.fn()],
}))

// `@/components/shared` is deliberately NOT mocked: Pagination, YearStrip and
// DenseTable are the behaviour under test here, not incidental chrome.

import { ArtistShowsList } from './ArtistShowsList'

// ── Fixtures ───────────────────────────────────────────────────────────────

function makeShow(overrides: Partial<ArtistShow> = {}): ArtistShow {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    price: 15,
    door_price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    venue: {
      id: 1,
      slug: 'empty-bottle',
      name: 'Empty Bottle',
      city: 'Chicago',
      state: 'IL',
      timezone: 'America/Chicago',
    },
    artists: [
      { id: 42, slug: 'main-artist', name: 'Main Artist' },
      { id: 99, slug: 'opener', name: 'The Opener' },
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
  data: { shows: ArtistShow[]; total?: number } | null,
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
  // What react-query means by it: the request answered. Read by the archive to
  // decide whether the month histogram is worth fetching before the year
  // histogram has landed.
  target.isSuccess = data !== null && !target.isError && !target.isPending
}

const setUpcoming = (
  data: { shows: ArtistShow[]; total?: number } | null,
  opts?: QueryState
) => applyState(upcomingResult, data, opts)

const setPast = (
  data: { shows: ArtistShow[]; total?: number } | null,
  opts?: QueryState
) => applyState(pastResult, data, opts)

function setYears(years: ArtistShowYearCount[] | null) {
  yearsResult.data = years ? { years } : undefined
  yearsResult.isSuccess = years !== null
  yearsResult.isPending = years === null
  yearsResult.isError = false
}

function renderList(
  overrides?: Partial<Parameters<typeof ArtistShowsList>[0]>
) {
  return renderWithProviders(
    <ArtistShowsList
      artistId={42}
      artistSlug="turnstile"
      artistName="Turnstile"
      {...overrides}
    />
  )
}

/** The past section's `<section>`, scoped so upcoming rows never leak in. */
const pastSection = () =>
  within(document.getElementById('artist-past-shows') as HTMLElement)

beforeEach(() => {
  setUpcoming(null, { isPending: true })
  setPast(null, { isPending: true })
  setYears(null)
  pastRequests.length = 0
  queryYear = null
  queryPage = 1
  otherParams = ''
})

afterEach(() => {
  document.title = ''
})

// ── Upcoming ───────────────────────────────────────────────────────────────

describe('ArtistShowsList — upcoming section', () => {
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

  // PSY-1905: the [Notify me] bracket that used to sit beside this sentence
  // duplicated the header's Follow control, which now subscribes on its own.
  it('states the empty case without a second subscribe control', () => {
    setUpcoming({ shows: [] })
    renderList({ artistName: 'Just Mustard' })
    expect(screen.getByText(/No upcoming shows yet/i)).toBeInTheDocument()
    expect(screen.queryByTestId('notify-me-button')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /notify/i })
    ).not.toBeInTheDocument()
  })

  it('renders upcoming shows as a table with the total beside the heading', () => {
    setUpcoming({ shows: [makeShow()], total: 83 })
    renderList()
    const table = screen.getByRole('table', { name: 'Upcoming shows' })
    expect(within(table).getByText('Main Artist')).toBeInTheDocument()
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

describe('ArtistPastShows — presence', () => {
  it('omits the past section entirely for an artist with no past shows', () => {
    setUpcoming({ shows: [makeShow()] })
    setPast({ shows: [] })
    setYears([])
    renderList()
    expect(document.getElementById('artist-past-shows')).toBeNull()
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
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
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
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
    // And the histogram already knew, so no 50,000-row offset scan was spent
    // proving it. A spinner must never be the terminal state here.
    expect(pastRequests[0].enabled).toBe(false)
    expect(document.querySelector('.animate-spin')).toBeNull()
  })
})

// ── Past archive: rows ─────────────────────────────────────────────────────

describe('ArtistPastShows — rows', () => {
  beforeEach(() => {
    setUpcoming({ shows: [] })
    setYears([{ year: 2025, count: 3 }])
  })

  it('links the date to the show slug, falling back to the id only without one', () => {
    setPast({
      shows: [
        makeShow({ id: 5, slug: 'ripe-at-empty-bottle' }),
        makeShow({ id: 6, slug: '', event_date: '2025-06-14T20:00:00Z' }),
      ],
      total: 2,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getByRole('link', { name: /Jun 15/ })).toHaveAttribute(
      'href',
      '/shows/ripe-at-empty-bottle'
    )
    expect(within(table).getByRole('link', { name: /Jun 14/ })).toHaveAttribute(
      'href',
      '/shows/6'
    )
  })

  it('names the venue and its location after the bill, linked to the venue page', () => {
    // The one deliberate divergence from the venue archive: an artist's rows
    // span venues, so the place is part of the row. Asserted as the full
    // string, not a loose /Chicago/ — the STATE is the point. An artist's rows
    // span metros, so "Portland" alone does not say which Portland, and the
    // PSY-558/780 rule is what decides that.
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: 'Empty Bottle' })
    ).toHaveAttribute('href', '/venues/empty-bottle')
    // Asserted on the cell, because the location is a text node sitting beside
    // the venue link rather than an element of its own.
    expect(within(table).getAllByRole('cell')[1]).toHaveTextContent(
      'Empty Bottle, Chicago, IL'
    )
  })

  it('drops the location entirely when the venue has none to place', () => {
    // `formatLocation` returns its "Location Unknown" placeholder for an
    // unplaceable venue. That is a FIELD placeholder; printed mid-line after a
    // venue name it would read as part of the sentence.
    setPast({
      shows: [
        makeShow({
          id: 5,
          venue: {
            id: 9,
            slug: 'nowhere',
            name: 'Nowhere Room',
            city: '',
            state: '',
            timezone: null,
          },
        }),
      ],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    const cell = within(table).getAllByRole('cell')[1]
    expect(cell).toHaveTextContent('Nowhere Room')
    expect(cell).not.toHaveTextContent('Location Unknown')
  })

  it('leaves a venue with no slug unlinked rather than linking to /venues/', () => {
    setPast({
      shows: [
        makeShow({
          id: 5,
          venue: {
            id: 9,
            slug: '',
            name: 'Unslugged Room',
            city: 'Tempe',
            state: 'AZ',
            timezone: null,
          },
        }),
      ],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getByText(/Unslugged Room/)).toBeInTheDocument()
    expect(
      within(table).queryByRole('link', { name: 'Unslugged Room' })
    ).not.toBeInTheDocument()
  })

  it('says Venue TBA for a show with no venue at all', () => {
    setPast({ shows: [makeShow({ id: 5, venue: null })], total: 1 })
    renderList()
    expect(
      within(screen.getByRole('table', { name: 'Past shows' })).getByText(
        /Venue TBA/
      )
    ).toBeInTheDocument()
  })

  it('emphasizes the headliner and reads the support acts as "w/"', () => {
    // The artist-shows payload carries no set_type/is_headliner, so `splitBill`
    // reads the API's own bill-position order: first act leads.
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table)
        .getByRole('link', { name: 'Main Artist' })
        .closest('.font-medium')
    ).not.toBeNull()
    expect(within(table).getByText(/w\//)).toBeInTheDocument()
    expect(
      within(table).getByRole('link', { name: 'The Opener' }).closest('.font-medium')
    ).toBeNull()
  })

  it('keeps the page artist on the bill instead of filtering them out', () => {
    // A deliberate reversal of the pre-PSY-1754 list, which showed only "w/ …".
    // Pinned rather than merely unpinned so the next reader can tell this was
    // chosen: on a support slot the lead is the thing the reader came for, and
    // filtering it out left the row starting with "w/" and no one to open for.
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    renderList({ artistName: 'Main Artist' })
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(
      within(table).getByRole('link', { name: 'Main Artist' })
    ).toHaveAttribute('href', '/artists/main-artist')
  })

  it('leaves a bill artist with no slug unlinked rather than linking to /artists/', () => {
    // `/artists/` is not a 404 — it is the artists INDEX, so an unguarded link
    // would quietly take the reader off the page instead of failing visibly.
    setPast({
      shows: [
        makeShow({
          id: 5,
          artists: [{ id: 42, slug: '', name: 'Unslugged Band' }],
        }),
      ],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(within(table).getByText('Unslugged Band')).toBeInTheDocument()
    expect(
      within(table).queryByRole('link', { name: 'Unslugged Band' })
    ).not.toBeInTheDocument()
  })

  it('heads the bill column for what the cell actually holds', () => {
    // The cell carries the venue as well as the bill, and a screen reader
    // announces the column header with every cell under it.
    setPast({ shows: [makeShow({ id: 5 })], total: 1 })
    renderList()
    expect(
      within(screen.getByRole('table', { name: 'Past shows' })).getByRole(
        'columnheader',
        { name: 'Bill · Venue' }
      )
    ).toBeInTheDocument()
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
    setPast({
      shows: [makeShow({ id: 5, artists: [], is_cancelled: true })],
      total: 1,
    })
    renderList()
    expect(
      within(screen.getByRole('table', { name: 'Past shows' })).getByText(
        'CANCELLED'
      )
    ).toBeInTheDocument()
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
    expect(within(table).getByText('$22')).toBeInTheDocument()
    expect(within(table).getAllByText('–').length).toBeGreaterThan(0)
    expect(within(table).queryByText('—')).not.toBeInTheDocument()
  })

  // The artist archive is the venue archive's twin and moves with it, so the
  // two cannot end up spelling one show's price differently (PSY-1962).
  it('renders a split price as the pair', () => {
    setPast({
      shows: [makeShow({ id: 7, price: 35, door_price: 40 })],
      total: 1,
    })
    renderList()
    const table = screen.getByRole('table', { name: 'Past shows' })
    // The spelling a screen reader reaches, not an attribute: `aria-label` on
    // a bare span is ARIA-prohibited and silently ignored.
    expect(within(table).getByText('$35/$40')).toBeInTheDocument()
    expect(
      within(table).getByText('$35 advance, $40 at the door')
    ).toBeInTheDocument()
  })

  it('groups past rows under month headings in each venue-local month', () => {
    setPast({
      shows: [
        makeShow({ id: 5, event_date: '2025-09-20T03:00:00Z' }),
        makeShow({ id: 6, event_date: '2025-09-06T03:00:00Z' }),
        makeShow({ id: 7, event_date: '2025-06-06T03:00:00Z' }),
      ],
      total: 3,
    })
    renderList()
    const headings = within(screen.getByRole('table', { name: 'Past shows' }))
      .getAllByRole('rowheader')
      .map(cell => cell.textContent)
    expect(headings).toEqual(['Sep 2025', 'Jun 2025'])
  })

  it('can repeat a month heading when a page straddles a boundary', () => {
    // The API orders on the absolute instant; each row is LABELLED in its own
    // venue's zone. Inside the ~1-day band around a month boundary those two
    // axes disagree, so the honest rendering repeats a heading rather than
    // silently reordering rows under a merged one. Pinned so the next reader
    // does not file it as a sorting bug.
    const london = {
      id: 2,
      slug: 'the-lexington',
      name: 'The Lexington',
      city: 'London',
      state: '',
      timezone: 'Europe/London',
    }
    setPast({
      shows: [
        // 02:00Z on Nov 1 is still Oct 31 in Chicago...
        makeShow({ id: 5, event_date: '2025-11-01T02:00:00Z' }),
        // ...while 01:00Z, an hour EARLIER, is already Nov 1 in London.
        makeShow({ id: 6, event_date: '2025-11-01T01:00:00Z', venue: london }),
        makeShow({ id: 7, event_date: '2025-10-31T20:00:00Z' }),
      ],
      total: 3,
    })
    renderList()
    const headings = within(screen.getByRole('table', { name: 'Past shows' }))
      .getAllByRole('rowheader')
      .map(cell => cell.textContent)
    expect(headings).toEqual(['Oct 2025', 'Nov 2025', 'Oct 2025'])
  })

  it('places each row in its OWN venue timezone, not one zone for the table', () => {
    // 2025-01-01T04:00:00Z is still Dec 31 in Chicago and already Jan 1 in
    // London. A single-zone table would file both rows under one month.
    setPast({
      shows: [
        makeShow({
          id: 5,
          event_date: '2025-01-01T04:00:00Z',
          venue: {
            id: 2,
            slug: 'the-lexington',
            name: 'The Lexington',
            city: 'London',
            state: '',
            timezone: 'Europe/London',
          },
        }),
        makeShow({ id: 6, event_date: '2025-01-01T04:00:00Z' }),
      ],
      total: 2,
    })
    renderList()
    const headings = within(screen.getByRole('table', { name: 'Past shows' }))
      .getAllByRole('rowheader')
      .map(cell => cell.textContent)
    expect(headings).toEqual(['Jan 2025', 'Dec 2024'])
  })
})

// ── Past archive: URL state ────────────────────────────────────────────────

describe('ArtistPastShows — year and page state', () => {
  const threeYears: ArtistShowYearCount[] = [
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
      '/artists/turnstile?year=2025#artist-past-shows'
    )
    expect(
      within(strip).getByRole('link', { name: 'All years' })
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
  })

  it('falls back to the artist id when the artist has no slug', () => {
    // `/artists/` is the artists INDEX, not a 404, so an unguarded empty slug
    // would make every link in the archive silently eject the reader.
    renderList({ artistSlug: '' })
    const strip = screen.getByRole('navigation', {
      name: 'Filter past shows by year',
    })
    expect(within(strip).getByRole('link', { name: /2025/ })).toHaveAttribute(
      'href',
      '/artists/42?year=2025#artist-past-shows'
    )
  })

  it('never emits ?page=1: page 1 links are bare', () => {
    queryPage = 2
    renderList()
    const pagers = screen.getAllByRole('navigation', { name: /pagination/i })
    expect(
      within(pagers[0]).getByRole('link', { name: 'Page 1' })
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
  })

  it('carries the active year into every page link', () => {
    queryYear = 2025
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(
      within(pager).getByRole('link', { name: /^Page 2/ })
    ).toHaveAttribute('href', '/artists/turnstile?year=2025&page=2#artist-past-shows')
  })

  it('carries an unrelated query param through every link it builds', () => {
    // The connections graph writes `?center=` onto this same URL and leaves it
    // there, and IT preserves year/page. Building hrefs from a fresh param set
    // would make that courtesy one-way and silently drop the reader's graph
    // center the moment they paged the archive.
    otherParams = 'center=some-other-artist'
    renderList()
    const pager = screen.getAllByRole('navigation', { name: /pagination/i })[0]
    expect(
      within(pager).getByRole('link', { name: /^Page 2/ })
    ).toHaveAttribute(
      'href',
      '/artists/turnstile?center=some-other-artist&page=2#artist-past-shows'
    )
    const strip = screen.getByRole('navigation', {
      name: 'Filter past shows by year',
    })
    expect(
      within(strip).getByRole('link', { name: 'All years' })
    ).toHaveAttribute(
      'href',
      '/artists/turnstile?center=some-other-artist#artist-past-shows'
    )
  })

  it('still drops its OWN params from the canonical page-1, all-years link', () => {
    // Carrying other people's params through must not turn "page 1 and all
    // years are bare" into "whatever was on the URL stays on it".
    otherParams = 'year=2024&page=7'
    renderList()
    const strip = screen.getByRole('navigation', {
      name: 'Filter past shows by year',
    })
    expect(
      within(strip).getByRole('link', { name: 'All years' })
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
  })

  it('offers a way out of a year filter when the histogram request fails', () => {
    // The year strip is the only exit from `?year=`, and it is built from a
    // SEPARATE request that can fail while the page request succeeds. Without
    // this the reader is left with correct rows and no control that clears the
    // filter — the zero-rows "Show every year" link does not run, because there
    // are rows.
    queryYear = 2025
    setYears(null)
    yearsResult.isError = true
    yearsResult.isPending = false
    setPast({ shows: [makeShow({ id: 5 })], total: 161 })
    renderList()
    expect(
      pastSection().getByRole('link', { name: 'Show every year' })
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
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
    expect(new Set(pagers.map(nav => nav.getAttribute('aria-label'))).size).toBe(
      2
    )
  })

  it('hides the pagers when the whole archive fits on one page', () => {
    setPast({ shows: [makeShow({ id: 5 })], total: 3 })
    setYears([{ year: 2025, count: 3 }])
    renderList()
    expect(
      screen.queryByRole('navigation', { name: /pagination/i })
    ).not.toBeInTheDocument()
  })

  it('scrolls a cold #artist-past-shows deep link onto the archive', () => {
    // The browser resolves the fragment before this section exists, so the
    // component has to honour it once its own rows are on screen.
    const scrollIntoView = vi.fn()
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    window.location.hash = '#artist-past-shows'
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
    window.location.hash = '#venue-past-shows'
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

describe('ArtistPastShows — filter reflection', () => {
  const years: ArtistShowYearCount[] = [
    { year: 2026, count: 34 },
    { year: 2025, count: 161 },
    { year: 2024, count: 217 },
  ]

  beforeEach(() => {
    setUpcoming({ shows: [] })
    setPast({ shows: [makeShow({ id: 5 })], total: 412 })
    setYears(years)
    document.title = 'Turnstile | Psychic Homily'
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
    expect(document.title).toBe('Turnstile | Psychic Homily')
  })

  it('carries the year and page in the document title', () => {
    queryYear = 2025
    queryPage = 2
    renderList()
    expect(document.title).toBe(
      'Turnstile shows in 2025 (page 2 of 4) | Psychic Homily'
    )
  })

  it('leaves a title another writer has replaced alone on unmount', () => {
    // On a soft navigation the next route's <title> is committed before this
    // effect's cleanup runs, so an unconditional restore would relabel the page
    // the reader just opened.
    queryYear = 2025
    const { unmount } = renderList()
    expect(document.title).toContain('shows in 2025')
    document.title = 'Some Other Page | Psychic Homily'
    unmount()
    expect(document.title).toBe('Some Other Page | Psychic Homily')
  })

  it('restores the route title when the archive unmounts', () => {
    queryYear = 2025
    const { unmount } = renderList()
    expect(document.title).toContain('shows in 2025')
    unmount()
    expect(document.title).toBe('Turnstile | Psychic Homily')
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
    // An artist with one year renders no year strip, so without these the only
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
    ).toHaveAttribute('href', '/artists/turnstile#artist-past-shows')
  })

  it('dims the rows only while they answer a different question', () => {
    setPast(
      { shows: [makeShow({ id: 5 })], total: 412 },
      { isFetching: true, isPlaceholderData: true }
    )
    const { unmount } = renderList()
    expect(
      screen.getByRole('table', { name: 'Past shows' }).closest('.opacity-60')
    ).not.toBeNull()
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
