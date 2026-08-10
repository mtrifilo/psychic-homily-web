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

function mockShowsError() {
  mockUseSceneShows.mockReturnValue({
    data: undefined,
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
    it('asks for four weeks and the endpoint’s full row cap', () => {
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
      const nav = screen.getByRole('navigation', { name: 'Show window' })
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
    // at a URL this site 404s.
    it('points "This weekend" at the nearest existing route', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show window' })
      expect(
        within(nav).getByRole('link', { name: 'This weekend' })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    it('marks the page’s own window active rather than linking it', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show window' })
      expect(
        within(nav).queryByRole('link', { name: 'Next 4 weeks' })
      ).not.toBeInTheDocument()
      expect(within(nav).getByText('Next 4 weeks')).toHaveAttribute(
        'aria-current',
        'page'
      )
    })

    // The nav NEVER degrades: a window that is empty is still an answer, so a
    // one-show scene gets the same strip a 328-show one does.
    it('renders the full strip on a scene with nothing in the window', () => {
      mockShows([])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show window' })
      expect(within(nav).getAllByText(/Tonight|This weekend|This week|Next 4 weeks/))
        .toHaveLength(4)
    })

    it('builds every href from the canonical scene slug', () => {
      // A metro member spelling resolves to its principal city, so the payload's
      // slug is the only one that must appear in a link.
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const nav = screen.getByRole('navigation', { name: 'Show window' })
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

    // The endpoint renders `event_date` in UTC, so a 20:00 Phoenix show arrives
    // labelled with the FOLLOWING day. Grouping off that field would file every
    // evening show under tomorrow.
    it('groups by the scene-local date, not the payload’s UTC event_date', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      expect(screen.queryByText(/SUN · AUG 9/)).not.toBeInTheDocument()
    })

    it('tags tonight’s group and leaves later dates untagged', () => {
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

  describe('the honest zero', () => {
    // The sparse frame's defining move: the tonight bucket renders even when
    // nothing is on it, so a reader arriving before doors gets the answer to the
    // question they came with instead of the next date that happens to have a row.
    it('renders a zero tonight bucket when the window starts later', () => {
      mockShows([buildShow({ id: 1, starts_at: '2026-08-23T03:00:00Z' })])
      renderWithProviders(<SceneCalendar scene={buildScene({ city: 'Portland', state: 'OR', slug: 'portland-or', stats: { venue_count: 3, artist_count: 9, upcoming_show_count: 1, festival_count: 0 } })} />)

      expect(screen.getByText(/SAT · AUG 8/)).toBeInTheDocument()
      expect(screen.getByText('0 shows')).toBeInTheDocument()
      expect(
        screen.getByText(
          'Nothing on our calendar for the 3 Portland rooms we track tonight. A room may have shows we have not listed.'
        )
      ).toBeInTheDocument()
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

    // The endpoint caps at 20 rows, so a dense scene really is truncated and the
    // line has to say which number is which rather than implying 20 is the total.
    it('names both numbers when the endpoint’s cap truncated the window', () => {
      mockShows(
        Array.from({ length: 20 }, (_, i) =>
          buildShow({ id: i + 1, starts_at: '2026-08-09T03:00:00Z' })
        )
      )
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByText('Showing 20 of 328 upcoming')).toBeInTheDocument()
    })

    // The mock draws the week link at BOTH ends of the list, which is what a
    // twenty-row window needs: the reader who skims and the reader who reaches
    // the bottom are different people.
    it('points at the week from the head and the foot of the list', () => {
      mockShows([buildShow()])
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      const weekLinks = screen.getAllByRole('link', { name: 'Full week in Phoenix' })
      expect(weekLinks).toHaveLength(2)
      for (const link of weekLinks) {
        expect(link).toHaveAttribute('href', '/scenes/phoenix-az/week')
      }
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
    beforeEach(() => {
      mockShowsError()
    })

    it('never claims the zero it did not verify', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.queryByText(/Nothing on our calendar/)).not.toBeInTheDocument()
      expect(screen.queryByText(/^Showing /)).not.toBeInTheDocument()
    })

    it('says what happened and still offers the week', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByText(/could not load this scene's calendar/i)
      ).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: /the full week in Phoenix/i })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    it('keeps the window nav intact', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(
        screen.getByRole('navigation', { name: 'Show window' })
      ).toBeInTheDocument()
    })
  })

  describe('while the window is loading', () => {
    it('renders the nav but no rows and no zero claim', () => {
      mockShows([], true)
      renderWithProviders(<SceneCalendar scene={buildScene()} />)
      expect(screen.getByRole('navigation', { name: 'Show window' })).toBeInTheDocument()
      expect(screen.queryByText(/Nothing on our calendar/)).not.toBeInTheDocument()
      expect(screen.queryByText(/^Showing /)).not.toBeInTheDocument()
    })
  })
})
