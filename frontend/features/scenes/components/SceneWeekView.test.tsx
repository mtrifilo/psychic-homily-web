import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
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
  it('renders the city with its state alongside', () => {
    render(<SceneWeekView week={week()} />)
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1).toHaveTextContent('Chicago')
    // The state has to be present for cold arrivals from a shared link —
    // "Columbus" and "Portland" are ambiguous without it.
    expect(h1).toHaveTextContent('IL')
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

  it('offers adjacent-week navigation', () => {
    render(<SceneWeekView week={week()} />)
    expect(screen.getByRole('link', { name: /2026-W30/ })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/2026-W30'
    )
    expect(screen.getByRole('link', { name: /2026-W32/ })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/2026-W32'
    )
  })

  // Load-bearing: the two pages are read as a pair, and this chip is the half
  // of that pairing a restyle of the three-chip row would drop silently.
  it('links out to tonight, the reciprocal of the day view Full week chip', () => {
    render(<SceneWeekView week={week()} />)
    expect(screen.getByRole('link', { name: 'Tonight' })).toHaveAttribute(
      'href',
      '/scenes/chicago-il/tonight'
    )
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
