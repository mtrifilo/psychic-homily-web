import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneDetail, SceneShowSummary } from '../types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseSceneShows = vi.fn()
vi.mock('../hooks', () => ({
  useSceneShows: (slug: string, options?: { days?: number; limit?: number }) =>
    mockUseSceneShows(slug, options),
}))

import { SceneCalendar } from './SceneCalendar'

// 20:00 Saturday Aug 8 2026 in Phoenix. Every fixture below is anchored to this
// instant so "tonight" is a fact of the test, not of the machine running it.
const SATURDAY_EVENING = new Date('2026-08-09T03:00:00Z')

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    stats: {
      venue_count: 12,
      artist_count: 17,
      upcoming_show_count: 328,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 0,
      shows_prev_month: 0,
      shows_trend: 0,
      new_artists_30d: 0,
      active_venues_this_month: 0,
      shows_by_month: [],
    },
    // The calendar reads rows from its own endpoint, never from this list
    // (PSY-1780 added it; rendering the rooms leaderboard is Wave 1B). Present
    // so the fixture is a real `SceneDetail` rather than a cast.
    venues: [],
    ...overrides,
  }
}

function buildShow(overrides: Partial<SceneShowSummary> = {}): SceneShowSummary {
  return {
    id: 1,
    title: '',
    artist_names: ['Gatecreeper'],
    event_date: '2026-08-09',
    starts_at: '2026-08-09T03:00:00Z', // 20:00 Aug 8, Phoenix
    price: 28,
    is_cancelled: false,
    is_sold_out: false,
    venue_name: 'Nile Theater',
    venue_city: 'Mesa',
    venue_state: 'AZ',
    venue_timezone: 'America/Phoenix',
    venue_slug: 'nile-theater',
    ...overrides,
  }
}

function mockShows(shows: SceneShowSummary[], isLoading = false) {
  mockUseSceneShows.mockReturnValue({ data: { shows }, isLoading, isError: false })
}

function mockShowsError(shows: SceneShowSummary[] = []) {
  mockUseSceneShows.mockReturnValue({
    data: shows.length > 0 ? { shows } : undefined,
    isLoading: false,
    isError: true,
  })
}

describe('SceneCalendar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(SATURDAY_EVENING)
    mockShows([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('the window request', () => {
    it("asks for four weeks and the endpoint's full row cap", () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(mockUseSceneShows).toHaveBeenCalledWith('phoenix-az', {
        days: 28,
        limit: 20,
      })
    })
  })

  describe('window nav strip', () => {
    it('links the windows that already have routes', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show windows' })
      expect(within(nav).getByRole('link', { name: 'Tonight' })).toHaveAttribute(
        'href',
        '/scenes/phoenix-az/tonight'
      )
      expect(within(nav).getByRole('link', { name: 'This week' })).toHaveAttribute(
        'href',
        '/scenes/phoenix-az/week'
      )
    })

    // `/scenes/{slug}/this-weekend` is Wave 2 work. Until it exists the chip
    // points at the nearest existing page that CONTAINS the weekend rather than
    // at a URL this site 404s. The ticket asks for exactly this.
    it('points "This weekend" at the nearest existing route', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show windows' })
      expect(
        within(nav).getByRole('link', { name: 'This weekend' })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    it("marks the page's own window current rather than linking it", () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show windows' })
      expect(
        within(nav).queryByRole('link', { name: 'Next 4 weeks' })
      ).not.toBeInTheDocument()
      expect(within(nav).getByText('Next 4 weeks')).toHaveAttribute(
        'aria-current',
        'true'
      )
    })

    // The nav NEVER degrades: a window that is empty is still an answer, so a
    // one-show scene gets the same strip a 328-show one does.
    it('renders the full strip on a scene with nothing in the window', () => {
      mockShows([])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show windows' })
      expect(within(nav).getAllByText(/Tonight|This weekend|This week|Next 4 weeks/))
        .toHaveLength(4)
    })

    it('builds every href from the canonical scene slug', () => {
      // A metro member spelling resolves to its principal city, so the payload's
      // slug is the only one that must appear in a link.
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show windows' })
      for (const link of within(nav).getAllByRole('link')) {
        expect(link.getAttribute('href')).toMatch(/^\/scenes\/phoenix-az\//)
      }
    })
  })

  describe('date-grouped rows', () => {
    it('renders a heading, a count and one row per show', () => {
      mockShows([
        buildShow({ id: 1, artist_names: ['Gatecreeper', 'Enforced'] }),
        buildShow({
          id: 2,
          artist_names: ['Playboy Manbaby'],
          starts_at: '2026-08-09T04:00:00Z', // 21:00 Aug 8
          venue_name: 'The Rebel Lounge',
          venue_city: 'Phoenix',
          price: 18,
        }),
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)

      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      expect(screen.getByText('2 shows')).toBeInTheDocument()
      expect(screen.getByText('Gatecreeper, Enforced')).toBeInTheDocument()
      expect(screen.getByText('Playboy Manbaby')).toBeInTheDocument()
    })

    it('carries time, price and the room with its sub-locality on the row', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)

      const row = screen.getByText('Gatecreeper').closest('a')!
      expect(row).toHaveAttribute('href', '/shows/1')
      expect(within(row).getByText('8:00 PM')).toBeInTheDocument()
      expect(within(row).getByText('$28.00')).toBeInTheDocument()
      // The sub-locality is what lets a metro scene read as a region.
      expect(within(row).getByText('Nile Theater (Mesa)')).toBeInTheDocument()
    })

    // ONE outer link per row. The mock draws the room as its own link, but a
    // nested anchor is invalid HTML and a row with two targets is a row the
    // reader has to aim at.
    it('is a single anchor, not a row of competing targets', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const row = screen.getByText('Gatecreeper').closest('li')!
      expect(within(row).getAllByRole('link')).toHaveLength(1)
    })

    // The endpoint renders `event_date` in UTC, so a 20:00 Phoenix show arrives
    // labelled with the FOLLOWING day. Grouping off that field would file every
    // evening show under tomorrow.
    it("groups by the scene-local date, not the payload's UTC event_date", () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      expect(screen.queryByText(/SUN · AUG 9/)).not.toBeInTheDocument()
    })

    // A row with no zone of its own is read in the scene's resolved zone, so a
    // heading and the start time beside it can never disagree about the day.
    it("falls back to the scene's majority zone for a row with no zone", () => {
      mockShows([
        buildShow({ id: 1 }),
        buildShow({ id: 2 }),
        buildShow({
          id: 3,
          artist_names: ['Sundressed'],
          venue_timezone: undefined,
          venue_state: undefined,
          venue_city: undefined,
          starts_at: '2026-08-09T04:30:00Z', // 21:30 Aug 8 in Phoenix
        }),
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText('3 shows')).toBeInTheDocument()
      expect(screen.queryByText(/SUN · AUG 9/)).not.toBeInTheDocument()
    })

    it("tags tonight's group and leaves later dates untagged", () => {
      mockShows([
        buildShow({ id: 1 }),
        buildShow({ id: 2, starts_at: '2026-08-23T03:00:00Z' }), // Aug 22
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)

      const tonight = screen.getByText(/SAT · AUG 8/)
      expect(within(tonight).getByText('· TONIGHT')).toBeInTheDocument()
      const later = screen.getByText(/SAT · AUG 22/)
      expect(within(later).queryByText('· TONIGHT')).not.toBeInTheDocument()
    })

    // A same-day claim needs a zone. `Intl` treats an absent zone as the
    // READER's, so tagging on a guess would tell someone in Tokyo that a
    // different night was tonight in Phoenix.
    it('tags no night at all when no row carries a resolvable zone', () => {
      mockShows([
        buildShow({
          venue_timezone: undefined,
          venue_state: undefined,
          venue_city: undefined,
        }),
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.queryByText('· TONIGHT')).not.toBeInTheDocument()
      expect(screen.queryByText('0 shows listed')).not.toBeInTheDocument()
    })

    it('marks a sold-out and a cancelled row', () => {
      mockShows([
        buildShow({ id: 1, is_sold_out: true }),
        buildShow({ id: 2, is_cancelled: true, artist_names: ['Diners'] }),
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
      expect(screen.getByText('CANCELLED')).toBeInTheDocument()
    })
  })

  // The page's only honesty disclosure. Coverage is a curated slice of each
  // city's rooms, and a page that let a reader assume otherwise would be lying
  // by omission.
  describe('the accuracy caveat', () => {
    it('speaks in a human voice above the rows', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByText(
          'Shows change all the time. We do our best to keep up, but check with the room before you head out.'
        )
      ).toBeInTheDocument()
    })
  })

  describe('the honest zero', () => {
    // The sparse frame's defining move: the tonight bucket renders even when
    // nothing is on it, so a reader arriving before doors gets the answer to the
    // question they came with instead of the next date that happens to have a row.
    it('renders a zero tonight bucket when the window starts later', () => {
      mockShows([
        buildShow({
          id: 1,
          starts_at: '2026-08-23T03:00:00Z',
          venue_timezone: 'America/Los_Angeles',
          venue_state: 'OR',
          venue_city: 'Portland',
          venue_name: 'Mississippi Studios',
        }),
      ])
      renderWithProviders(
        <SceneCalendar
          scene={buildScene({
            city: 'Portland',
            state: 'OR',
            slug: 'portland-or',
            stats: {
              venue_count: 3,
              artist_count: 9,
              upcoming_show_count: 1,
              festival_count: 0,
            },
          })}
        />
      )

      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      // "listed", not a bare "0 shows": the nightly page's phrase, because a
      // bare zero would assert nothing is happening in the city.
      expect(screen.getByText('0 shows listed')).toBeInTheDocument()
      expect(
        screen.getByText(
          'Nothing on our calendar for the 3 Portland rooms we track tonight. A room may have shows we have not listed.'
        )
      ).toBeInTheDocument()
    })

    // The window footer beneath the list already carries both destinations, so
    // the tonight bucket does not repeat them.
    it('does not repeat the footer links inside a zero tonight bucket', () => {
      mockShows([
        buildShow({ id: 1, starts_at: '2026-08-23T03:00:00Z' }),
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getAllByRole('link', { name: 'All upcoming in Phoenix' })
      ).toHaveLength(1)
      expect(
        screen.queryByRole('link', { name: 'Suggest a venue' })
      ).not.toBeInTheDocument()
    })

    it('states the zero and offers exactly one step wider when the whole window is empty', () => {
      mockShows([])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)

      expect(
        screen.getByText(
          'Nothing on our calendar for the 12 Phoenix rooms we track in the next four weeks. A room may have shows we have not listed.'
        )
      ).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: 'All upcoming in Phoenix' })
      ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ')
      expect(
        screen.getByRole('link', { name: 'Suggest a venue' })
      ).toHaveAttribute('href', '/contribute')
    })

    // No scaffolding under an empty window: the footer counts rows, and there
    // are none to count.
    it('renders no window footer when there is nothing in the window', () => {
      mockShows([])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.queryByText(/^Showing /)).not.toBeInTheDocument()
    })
  })

  describe('the window footer', () => {
    it('says so in words when the whole window fits', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByText('Showing everything we have in the next four weeks')
      ).toBeInTheDocument()
    })

    // NO `n of upcoming_show_count`: that field counts every future show with
    // no upper bound, so it would be a denominator from a different window than
    // its numerator.
    it('names no total it cannot check', () => {
      mockShows(
        Array.from({ length: 20 }, (_, i) =>
          buildShow({ id: i + 1, starts_at: '2026-08-09T03:00:00Z' })
        )
      )
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByText('Showing the first 20 in the next four weeks')
      ).toBeInTheDocument()
      expect(screen.queryByText(/328/)).not.toBeInTheDocument()
    })

    // The endpoint stops at its cap wherever that falls, usually part-way
    // through a date. That date's count would be a per-day total nobody
    // checked, so the partial group is dropped rather than qualified.
    it('drops the date the row cap cut in half', () => {
      mockShows([
        ...Array.from({ length: 19 }, (_, i) =>
          buildShow({ id: i + 1, starts_at: '2026-08-09T03:00:00Z' })
        ),
        buildShow({ id: 20, starts_at: '2026-08-10T03:00:00Z' }), // Aug 9
      ])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)

      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      expect(screen.queryByText(/SUN · AUG 9/)).not.toBeInTheDocument()
      expect(
        screen.getByText('Showing the first 19 in the next four weeks')
      ).toBeInTheDocument()
    })

    // The mock draws the week link at both ends of the list. The nav strip
    // already carries a THIS WEEK chip above the header, so only the footer
    // copy ships and there is exactly one link with this name.
    it('points at the week exactly once', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const weekLinks = screen.getAllByRole('link', { name: 'Full week in Phoenix' })
      expect(weekLinks).toHaveLength(1)
      expect(weekLinks[0]).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    it('points at everything upcoming', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByRole('link', { name: 'All upcoming in Phoenix' })
      ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ')
    })
  })

  // A failed request is not an empty calendar. Claiming the honest zero here
  // would state, in our own voice and with a room count attached, that nothing
  // is on tonight, a claim the page never actually checked.
  describe('when the window request fails', () => {
    it('never claims the zero it did not verify', () => {
      mockShowsError()
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.queryByText(/Nothing on our calendar/)).not.toBeInTheDocument()
      expect(screen.queryByText(/^Showing /)).not.toBeInTheDocument()
    })

    it('says what happened and still offers the week', () => {
      mockShowsError()
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByText(/could not load this scene's calendar/i)
      ).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: /the full week in Phoenix/i })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    it('keeps the window nav intact', () => {
      mockShowsError()
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByRole('navigation', { name: 'Show windows' })
      ).toBeInTheDocument()
    })

    // A background refetch that fails keeps the rows it already had. Replacing
    // four correct weeks with an apology because a reconnect blipped would be a
    // worse answer than the rows.
    it('keeps rows it already has when a refetch fails', () => {
      mockShowsError([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
      expect(
        screen.queryByText(/could not load this scene's calendar/i)
      ).not.toBeInTheDocument()
    })
  })

  describe('while the window is loading', () => {
    it('renders the nav but no rows and no zero claim', () => {
      mockShows([], true)
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByRole('navigation', { name: 'Show windows' })
      ).toBeInTheDocument()
      expect(screen.queryByText(/Nothing on our calendar/)).not.toBeInTheDocument()
      expect(screen.queryByText(/^Showing /)).not.toBeInTheDocument()
    })
  })
})
