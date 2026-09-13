import { describe, it, expect, vi, afterEach } from 'vitest'
import { cleanup, render, screen, within, waitFor } from '@testing-library/react'
import { fireEvent } from '@testing-library/dom'
import { SceneWeekView } from './SceneWeekView'
import type { SceneWeekResponse, SceneWeekShow } from '../sceneWeek'
import type { SceneTrackedVenue } from '../sceneDay'

const show = (over: Partial<SceneWeekShow> = {}): SceneWeekShow => ({
  id: 1,
  title: '',
  event_date: '2026-07-27',
  starts_at: '2026-07-28T01:00:00Z',
  venue_name: 'Empty Bottle',
  artist_names: ['Ovlov', 'Cusp'],
  is_sold_out: false,
  is_cancelled: false,
  ...over,
})

const room = (over: Partial<SceneTrackedVenue> = {}): SceneTrackedVenue =>
  ({ name: 'Empty Bottle', slug: 'empty-bottle', website: '', ...over }) as SceneTrackedVenue

const week = (over: Partial<SceneWeekResponse> = {}): SceneWeekResponse =>
  ({
    slug: 'chicago-il',
    scene_name: 'Chicago, IL',
    city: 'Chicago',
    state: 'IL',
    iso_week: '2026-W31',
    start_date: '2026-07-27',
    end_date: '2026-08-02',
    timezone: 'America/Chicago',
    show_count: 1,
    prev_week: '2026-W30',
    next_week: '2026-W32',
    is_current_week: true,
    is_past_week: false,
    days: [{ date: '2026-07-27', shows: [show()] }],
    tracked_venues: [room(), room({ name: 'Thalia Hall', slug: 'thalia-hall' })],
    ...over,
  }) as SceneWeekResponse

/** Every href on the page that addresses a dated week permalink, in order. */
const weekLinks = (container: HTMLElement): string[] =>
  [...container.querySelectorAll('a[href]')]
    .map(a => a.getAttribute('href') ?? '')
    .filter(href => /^\/scenes\/[^/]+\/\d{4}-W\d{2}$/i.test(href))

/**
 * The label of each direction in the adjacent-week row, in order.
 *
 * Reads the ROW rather than filtering hrefs by shape: a direction the guard
 * should have suppressed carries a malformed href, which an href-shape filter
 * cannot see, so an assertion built on one passes whether or not the guard
 * exists. The muted edges have no href at all and are only visible this way.
 */
const stepLabels = (): string[] =>
  [...screen.getByRole('navigation', { name: 'Adjacent weeks' }).children].map(el =>
    (el.textContent ?? '').trim()
  )

describe('SceneWeekView — share affordance', () => {
  afterEach(() => {
    Reflect.deleteProperty(navigator, 'clipboard')
    vi.restoreAllMocks()
  })

  it('shares the DATED permalink, never the rolling /week URL', async () => {
    // This page is reachable at both `/scenes/{slug}/{iso-week}` and the
    // rolling `/scenes/{slug}/week`. Sharing the rolling URL would hand a
    // friend a page whose contents change next Monday, so the control must
    // emit the archived permalink regardless of which route rendered it.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })

    render(<SceneWeekView week={week()} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Share this week' })
    )

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        'https://psychichomily.com/scenes/chicago-il/2026-W31'
      )
    )
  })

  it('renders no share control when the browser cannot share or copy', async () => {
    render(<SceneWeekView week={week()} />)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Share this week' })
      ).not.toBeInTheDocument()
    )
  })
})

describe('SceneWeekView', () => {
  // The title rule. An archived week names its own Monday; only the current
  // week may call itself "this week".
  it('titles the current week by the window and an archived one by its date', () => {
    render(<SceneWeekView week={week()} />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'This week in Chicago'
    )

    cleanup()
    render(<SceneWeekView week={week({ is_current_week: false, is_past_week: true })} />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'Week of Jul 27 in Chicago'
    )
  })

  it('states the range and count', () => {
    render(<SceneWeekView week={week({ show_count: 32 })} />)
    expect(screen.getByText(/Mon, Jul 27 – Sun, Aug 2, 2026/)).toBeInTheDocument()
    expect(screen.getByText(/32 shows/)).toBeInTheDocument()
  })

  it('singularises a one-show week', () => {
    render(<SceneWeekView week={week({ show_count: 1 })} />)
    expect(screen.getByText(/1 show(?!s)/)).toBeInTheDocument()
  })

  // Load-bearing, not decoration: coverage is a curated slice, and a page that
  // implied full city coverage would be false. Slugged rooms link to
  // /venues/{slug}; rooms without a slug stay plain text.
  it('always discloses that coverage is partial', () => {
    render(<SceneWeekView week={week()} />)
    expect(screen.getByText(/Not a complete city listing/)).toBeInTheDocument()
    const footer = screen.getByText(/ROOMS WE TRACK IN CHICAGO/).closest('footer')
    expect(footer).not.toBeNull()
    expect(
      within(footer as HTMLElement).getByRole('link', { name: 'Empty Bottle' })
    ).toHaveAttribute('href', '/venues/empty-bottle')
    expect(
      within(footer as HTMLElement).getByRole('link', { name: 'Thalia Hall' })
    ).toHaveAttribute('href', '/venues/thalia-hall')
    const footerLinks = within(footer as HTMLElement)
      .getAllByRole('link')
      .filter(a => a.getAttribute('href')?.startsWith('/venues/'))
    expect(footerLinks).toHaveLength(2)
  })

  it('names a tracked room without a slug, unlinked, in the listing footer', () => {
    render(
      <SceneWeekView
        week={week({ tracked_venues: [room({ name: 'DIY Basement', slug: '' })] })}
      />
    )
    const footer = screen.getByText(/ROOMS WE TRACK IN CHICAGO/).closest('footer')
    expect(footer).not.toBeNull()
    expect(within(footer as HTMLElement).getByText('DIY Basement')).toBeInTheDocument()
    expect(
      within(footer as HTMLElement).queryByRole('link', { name: 'DIY Basement' })
    ).not.toBeInTheDocument()
  })

  it('treats a whitespace-only slug as missing, not a broken /venues/ URL', () => {
    render(
      <SceneWeekView
        week={week({ tracked_venues: [room({ name: 'Whitespace Room', slug: '   ' })] })}
      />
    )
    const footer = screen.getByText(/ROOMS WE TRACK IN CHICAGO/).closest('footer')
    expect(footer).not.toBeNull()
    expect(
      within(footer as HTMLElement).queryByRole('link', { name: 'Whitespace Room' })
    ).not.toBeInTheDocument()
  })

  it('links each show and shows its venue', () => {
    render(<SceneWeekView week={week()} />)
    const link = screen.getByRole('link', { name: /Ovlov, Cusp/ })
    expect(link).toHaveAttribute('href', '/shows/1')
    expect(within(link).getByText('Empty Bottle')).toBeInTheDocument()
  })

  it('badges a sold-out show', () => {
    render(<SceneWeekView week={week({ days: [{ date: '2026-07-27', shows: [show({ is_sold_out: true })] }] })} />)
    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  // Cancelled outranks sold out — a cancelled show that reads "SOLD OUT" would
  // actively mislead someone deciding whether to go.
  it('badges a cancelled show and suppresses the sold-out badge', () => {
    render(
      <SceneWeekView
        week={week({
          days: [{ date: '2026-07-27', shows: [show({ is_cancelled: true, is_sold_out: true })] }],
        })}
      />
    )
    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
    expect(screen.queryByText('SOLD OUT')).not.toBeInTheDocument()
  })

  // The decision was an empty STATE, not a 404 — a real city having a quiet
  // week is a fact, and 404ing would break an already-shared permalink.
  it('renders an empty week with a way forward', () => {
    render(<SceneWeekView week={week({ show_count: 0, days: [], tracked_venues: [room()] })} />)
    expect(screen.getByText(/No shows at the Chicago rooms we track this week/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Try next week/ })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/2026-W32'
    )
  })

  // "This week" describes a week that ended months ago on a permalink, and
  // "try next week" points at a week that is also over. An archived empty week
  // names itself and points at what is on NOW.
  it('names an archived empty week by its date and points at the current week', () => {
    render(
      <SceneWeekView
        week={week({
          show_count: 0,
          days: [],
          tracked_venues: [room()],
          is_current_week: false,
          is_past_week: true,
        })}
      />
    )

    expect(
      screen.getByText(/No shows at the Chicago rooms we track the week of Jul 27\./)
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Try next week/ })).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /See this week in Chicago/ })
    ).toHaveAttribute('href', '/scenes/chicago-il/week')
  })

  // The control names what it shares. "Share this week" on a permalink to last
  // March is the same false claim the body copy above closes.
  it('names an archived week in the share control', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })

    render(<SceneWeekView week={week({ is_current_week: false, is_past_week: true })} />)
    expect(
      await screen.findByRole('button', { name: 'Share the week of Jul 27' })
    ).toBeInTheDocument()
    Reflect.deleteProperty(navigator, 'clipboard')
  })

  it('offers adjacent-week navigation, relative on the current week', () => {
    render(<SceneWeekView week={week()} />)
    const adjacent = screen.getByRole('navigation', { name: 'Adjacent weeks' })
    expect(within(adjacent).getByRole('link', { name: /Last week/ })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/2026-W30'
    )
    expect(within(adjacent).getByRole('link', { name: /Next week/ })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/2026-W32'
    )
  })

  // "Last week" and "next week" are claims about now, so only the current week
  // may make them. An archived week names its neighbours by their own dates.
  it('names the neighbours of an archived week by date', () => {
    render(<SceneWeekView week={week({ is_current_week: false, is_past_week: true })} />)
    const adjacent = screen.getByRole('navigation', { name: 'Adjacent weeks' })
    expect(
      within(adjacent).getByRole('link', { name: /Week of Jul 20/ })
    ).toHaveAttribute('href', '/scenes/chicago-il/2026-W30')
    expect(
      within(adjacent).getByRole('link', { name: /Week of Aug 3/ })
    ).toHaveAttribute('href', '/scenes/chicago-il/2026-W32')
  })

  // A key this site cannot serve is not navigation. An absent one renders the
  // word `undefined` into both the label and the href; one outside the week
  // route's year bounds renders a chip whose page 404s.
  it.each([
    ['absent', undefined],
    ['blank', '   '],
    ['not a week key', 'next'],
    ['before the servable years', '2014-W52'],
  ])('mutes the next direction when the key is %s', (_label, next_week) => {
    const { container } = render(<SceneWeekView week={week({ next_week })} />)

    // Asserted on the ROW, not on hrefs matching the week shape: a filter that
    // only admits well-formed week links cannot see the malformed one the
    // guard exists to suppress, so it would pass with the guard deleted.
    expect(stepLabels()).toEqual(['← Last week', 'End of listings'])
    expect(weekLinks(container)).toEqual(['/scenes/chicago-il/2026-W30'])
  })

  it('mutes the previous direction when the key is absent', () => {
    const { container } = render(<SceneWeekView week={week({ prev_week: undefined })} />)

    expect(stepLabels()).toEqual(['Start of listings', 'Next week →'])
    expect(weekLinks(container)).toEqual(['/scenes/chicago-il/2026-W32'])
  })

  // The pointer onward goes with the link, trailing stop included: a bare "."
  // after the sentence would read as a typo.
  it('drops the way forward on an empty current week with no next week to offer', () => {
    render(
      <SceneWeekView
        week={week({ show_count: 0, days: [], tracked_venues: [room()], next_week: undefined })}
      />
    )

    expect(screen.queryByRole('link', { name: /Try next week/ })).not.toBeInTheDocument()
    expect(
      screen.getByText(/No shows at the Chicago rooms we track this week\.$/)
    ).toBeInTheDocument()
  })

  // Load-bearing: the two pages are read as a pair, and this link is the half
  // of that pairing a restyle of the strip would drop silently.
  it('links out to tonight, the reciprocal of the day view week chip', () => {
    render(<SceneWeekView week={week()} />)
    expect(screen.getByRole('link', { name: 'Tonight' })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/tonight'
    )
  })

  // Current on the week this page IS, and only then. An archived permalink
  // marking "This week" current would claim a week that ended months ago is the
  // current one, and would leave the reader no link to the week that is.
  it('marks the week window current only while the week is current', () => {
    const { container } = render(<SceneWeekView week={week()} />)
    expect(container.querySelector('[aria-current="page"]')).toHaveTextContent(
      'This week'
    )
    expect(
      within(screen.getByRole('navigation', { name: 'Show windows' })).queryByRole(
        'link',
        { name: 'This week' }
      )
    ).toBeNull()

    cleanup()
    const archived = render(
      <SceneWeekView week={week({ is_current_week: false, is_past_week: true })} />
    )
    expect(archived.container.querySelector('[aria-current="page"]')).toBeNull()
    expect(
      within(screen.getByRole('navigation', { name: 'Show windows' })).getByRole('link', {
        name: 'This week',
      })
    ).toHaveAttribute('href', '/scenes/chicago-il/week')
  })

  // Kept on archived weeks too: a reader who lands on last March still wants
  // the way back to what is on now.
  it('keeps the tonight link on an archived week', () => {
    render(<SceneWeekView week={week({ is_current_week: false, is_past_week: true })} />)
    expect(screen.getByRole('link', { name: 'Tonight' })).toBeInTheDocument()
  })

  // The generator types these nullable even though the API always emits arrays;
  // a null must not take the page down.
  it('survives null days and tracked_venues', () => {
    render(<SceneWeekView week={week({ days: null, tracked_venues: null, show_count: 0 })} />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Chicago')
    expect(screen.queryByText(/ROOMS WE TRACK/)).not.toBeInTheDocument()
  })

  it('renders a quiet day as a heading and count only', () => {
    render(
      <SceneWeekView
        week={week({
          show_count: 1,
          days: [
            { date: '2026-07-27', shows: [show()] },
            { date: '2026-07-28', shows: [] },
          ],
        })}
      />
    )
    expect(screen.getByText('TUE JUL 28')).toBeInTheDocument()
    // The `0` in the heading says it; a sentence per quiet day would be most of
    // the page in a sparse scene.
    expect(screen.queryByText(/No shows at the rooms we track\./)).not.toBeInTheDocument()
  })
})
