import { describe, it, expect, vi } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { buildSceneSlice, type SceneSliceData } from '../sceneSlice'
import type { SceneDayResponse } from '../sceneDay'
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

import { SceneCalendar } from './SceneCalendar'

/**
 * The root's calendar slice (PSY-1850).
 *
 * Note what these tests no longer do: mock a fetch, pin a clock, or assert a
 * window length. The slice is a pure function of two day payloads the SERVER
 * resolved, so "which night is tonight" is a fact of the fixture rather than of
 * the machine running the suite — which is the entire point of moving off the
 * forward-only window. The boundary itself is the backend's, covered in
 * `sceneSlice.test.ts` and in the Go `scene_day` suite.
 */

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
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
    venues: [],
    ...overrides,
  }
}

function buildShow(overrides: Partial<SceneShowSummary> = {}): SceneShowSummary {
  return {
    id: 1,
    title: '',
    artist_names: ['Gatecreeper'],
    event_date: '2026-08-08',
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

function buildDay(overrides: Partial<SceneDayResponse> = {}): SceneDayResponse {
  return {
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-08-08',
    timezone: 'America/Phoenix',
    iso_week: '2026-W32',
    prev_date: '2026-08-07',
    next_date: '2026-08-09',
    is_tonight: true,
    is_past_day: false,
    show_count: 0,
    shows: [],
    tracked_venues: [],
    ...overrides,
  }
}

/** Tonight with rows, plus the next full day with rows. */
function buildSlice(
  tonightShows: SceneShowSummary[],
  nextShows: SceneShowSummary[] = []
): SceneSliceData {
  return buildSceneSlice(
    buildDay({ shows: tonightShows }),
    buildDay({ date: '2026-08-09', is_tonight: false, shows: nextShows })
  )!
}

describe('SceneCalendar', () => {
  describe('the window strip', () => {
    // The re-lock in one assertion: the root is not one of these windows, so
    // none of them may be marked current.
    it('renders all four windows as links, none of them active', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )

      const nav = screen.getByRole('navigation', { name: /show windows/i })
      const links = within(nav).getAllByRole('link')

      expect(links.map(a => a.textContent)).toEqual([
        'Tonight',
        'This weekend',
        'This week',
        'Next 4 weeks',
      ])
      expect(within(nav).queryByText(/next 4 weeks/i)?.tagName).toBe('A')
      expect(nav.querySelector('[aria-current]')).toBeNull()
    })

    // The shipped defect this retires: `This weekend` and `This week` pointed
    // at the SAME /week href, because /this-weekend did not exist yet.
    it('gives each window its own path segment', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )

      const nav = screen.getByRole('navigation', { name: /show windows/i })
      const hrefs = within(nav)
        .getAllByRole('link')
        .map(a => a.getAttribute('href'))

      expect(hrefs).toEqual([
        '/scenes/phoenix-az/tonight',
        '/scenes/phoenix-az/this-weekend',
        '/scenes/phoenix-az/week',
        '/scenes/phoenix-az/next-4-weeks',
      ])
      expect(new Set(hrefs).size).toBe(hrefs.length)
    })
  })

  describe('the slice boundary', () => {
    it('renders exactly the two whole dates the payloads answered for', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice([buildShow({ id: 1 })], [buildShow({ id: 2 })])}
        />
      )

      const headings = screen.getAllByRole('heading', { level: 3 })
      expect(headings).toHaveLength(2)
      expect(headings[0]).toHaveTextContent('SATURDAY, AUGUST 8')
      expect(headings[1]).toHaveTextContent('SUNDAY, AUGUST 9')
    })

    // Tonight comes from the backend's `is_tonight`, so a night in progress is
    // tagged correctly at 01:00 — the case the old forward-only window could
    // not see at all.
    it('tags only the night the payload calls tonight', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice([buildShow({ id: 1 })], [buildShow({ id: 2 })])}
        />
      )

      const headings = screen.getAllByRole('heading', { level: 3 })
      expect(headings[0]).toHaveTextContent('TONIGHT')
      expect(headings[1]).not.toHaveTextContent('TONIGHT')
    })

    it('renders one date when there is no next day to show', () => {
      const slice = buildSceneSlice(
        buildDay({ next_date: '', shows: [buildShow()] }),
        null
      )!
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={slice} />)

      expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(1)
    })

    // The root must not grow a third date. A window is what the other four
    // routes are for.
    it('never renders a date beyond the two the slice carries', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice([buildShow({ id: 1 })], [buildShow({ id: 2 })])}
        />
      )
      expect(screen.queryByText(/AUGUST 10/i)).toBeNull()
    })
  })

  describe('the rows', () => {
    it('lists every show of a date, with its time, price and room', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice([
            buildShow({ id: 1, artist_names: ['Gatecreeper'] }),
            buildShow({
              id: 2,
              artist_names: ['Destruction Unit'],
              price: 12,
              starts_at: '2026-08-09T04:30:00Z', // 21:30 Aug 8, Phoenix
            }),
          ])}
        />
      )

      expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
      expect(screen.getByText('Destruction Unit')).toBeInTheDocument()
      expect(screen.getByText('8:00 PM')).toBeInTheDocument()
      expect(screen.getByText('9:30 PM')).toBeInTheDocument()
      expect(screen.getByText('$28.00')).toBeInTheDocument()
      expect(screen.getByText('$12.00')).toBeInTheDocument()
    })

    // The sub-locality is what lets a metro scene read as a region.
    it('prints the room city beside the room', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )
      expect(screen.getByText(/Nile Theater \(Mesa\)/)).toBeInTheDocument()
    })

    it('marks cancelled and sold-out rows', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice([
            buildShow({ id: 1, is_cancelled: true }),
            buildShow({ id: 2, is_sold_out: true }),
          ])}
        />
      )
      expect(screen.getByText('CANCELLED')).toBeInTheDocument()
      expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
    })

    // The payoff of reading the day endpoint: the backend answered for the
    // whole date, so this count is that date's real total rather than however
    // many rows survived a window cap. Nothing has to be suppressed.
    it('states each date total without qualification', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene()}
          slice={buildSlice(
            [buildShow({ id: 1 }), buildShow({ id: 2 })],
            [buildShow({ id: 3 })]
          )}
        />
      )
      expect(screen.getByText('2 shows')).toBeInTheDocument()
      expect(screen.getByText('1 show')).toBeInTheDocument()
    })

    it('says a date is empty rather than hiding it', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()], [])} />
      )
      expect(screen.getByText('0 shows listed')).toBeInTheDocument()
      expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(2)
    })
  })

  describe('the footer', () => {
    it('offers the week and the four weeks, and claims nothing about them', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )

      expect(
        screen.getByRole('link', { name: /full week in phoenix/i })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
      expect(
        screen.getByRole('link', { name: /next 4 weeks →/i })
      ).toHaveAttribute('href', '/scenes/phoenix-az/next-4-weeks')
    })

    // The claims the old windowed footer made, which this page is no longer
    // entitled to make. A section that shows two nights must not describe a
    // month.
    it('makes no "next four weeks" claim about what it rendered', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )
      expect(screen.queryByText(/showing everything we have/i)).toBeNull()
      expect(screen.queryByText(/of 328 upcoming/i)).toBeNull()
      expect(screen.queryByText(/shows \/ next 4 weeks/i)).toBeNull()
    })

    // Share and .ics already sit in the header's action row a screen above.
    it('does not repeat the header actions', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([buildShow()])} />
      )
      expect(screen.queryByRole('button', { name: /share this scene/i })).toBeNull()
    })
  })

  describe('a quiet slice', () => {
    it('states the zero honestly and names the rooms it checked', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([], [])} />
      )

      expect(
        screen.getByText(/nothing on our calendar for the 12 phoenix rooms we track/i)
      ).toBeInTheDocument()
      // Never a claim about the CITY, only about our calendar.
      expect(screen.getByText(/a room may have shows we have not listed/i)).toBeInTheDocument()
    })

    // The old copy said "in the next four weeks" here. Two quiet nights say
    // nothing whatever about the month.
    it('names the two nights it actually checked', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([], [])} />
      )
      expect(screen.getByText(/tonight or Sunday/i)).toBeInTheDocument()
      expect(screen.queryByText(/next four weeks/i)).toBeNull()
    })

    // The case the "tomorrow" spelling got wrong. Between midnight and 06:00 the
    // scene's night boundary puts tonight on YESTERDAY's date, so the second day
    // the slice checked is TODAY — and calling it "tomorrow" would be false in
    // exactly the window this ticket moved to the day endpoint to render right.
    it('names the second night rather than calling it tomorrow', () => {
      const slice = buildSceneSlice(
        buildDay({ date: '2026-08-08', is_tonight: true, next_date: '2026-08-09', shows: [] }),
        buildDay({ date: '2026-08-09', is_tonight: false, shows: [] })
      )!
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={slice} />)

      const copy = screen.getByText(/nothing on our calendar/i)
      expect(copy).toHaveTextContent(/tonight or Sunday/i)
      expect(copy).not.toHaveTextContent(/tomorrow/i)
    })

    it('says only tonight when only tonight was checked', () => {
      const slice = buildSceneSlice(buildDay({ next_date: '', shows: [] }), null)!
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={slice} />)

      const copy = screen.getByText(/nothing on our calendar/i)
      expect(copy).toHaveTextContent(/we track tonight\./i)
      expect(copy).not.toHaveTextContent(/tomorrow/i)
    })

    // Honest zero plus one step wider, and a second door: the scene that is
    // quiet for two nights may well have a quiet week too.
    it('offers a way onward rather than a dead end', () => {
      renderWithProviders(
        <SceneCalendar scene={buildScene()} slice={buildSlice([], [])} />
      )

      expect(screen.getByRole('link', { name: /full week in phoenix/i })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: /next 4 weeks →/i })).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: /all upcoming in phoenix/i })
      ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ')
      expect(screen.getByRole('link', { name: /suggest a venue/i })).toBeInTheDocument()
    })

    it('drops the room count rather than printing a zero one', () => {
      renderWithProviders(
        <SceneCalendar
          scene={buildScene({
            stats: {
              venue_count: 0,
              artist_count: 0,
              upcoming_show_count: 0,
              festival_count: 0,
            },
          })}
          slice={buildSlice([], [])}
        />
      )
      expect(
        screen.getByText(/nothing on our calendar for the phoenix rooms we track/i)
      ).toBeInTheDocument()
    })
  })

  describe('a slice that could not be loaded', () => {
    // A failed request is NOT an empty calendar. Falling through to the
    // honest-zero copy would state, in our own voice and with a room count
    // attached, that nothing is on tonight — a claim we never checked.
    it('says what happened instead of asserting a zero', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={null} />)

      expect(screen.getByText(/could not load this scene's calendar/i)).toBeInTheDocument()
      expect(screen.queryByText(/nothing on our calendar/i)).toBeNull()
      expect(screen.queryByText(/0 shows listed/i)).toBeNull()
    })

    it('still offers the week', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={null} />)
      expect(
        screen.getByRole('link', { name: /the full week in phoenix/i })
      ).toHaveAttribute('href', '/scenes/phoenix-az/week')
    })

    // The strip is the reader's way out of a broken calendar, so it must not
    // depend on the calendar having loaded.
    it('keeps the window strip', () => {
      renderWithProviders(<SceneCalendar scene={buildScene()} slice={null} />)
      const nav = screen.getByRole('navigation', { name: /show windows/i })
      expect(within(nav).getAllByRole('link')).toHaveLength(4)
    })
  })
})
