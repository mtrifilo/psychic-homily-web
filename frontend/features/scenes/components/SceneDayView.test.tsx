import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
import { fireEvent } from '@testing-library/dom'
import { SceneDayView } from './SceneDayView'
import type { SceneDayResponse, SceneDayShow, SceneTrackedVenue } from '../sceneDay'

const show = (over: Partial<SceneDayShow> = {}): SceneDayShow =>
  ({
    id: 1,
    title: '',
    event_date: '2026-07-31',
    // 20:00 Phoenix on the 31st.
    starts_at: '2026-08-01T03:00:00Z',
    venue_name: 'Valley Bar',
    venue_state: 'AZ',
    venue_timezone: 'America/Phoenix',
    artist_names: ['Smooth Hands', 'Tournament'],
    is_sold_out: false,
    is_cancelled: false,
    ...over,
  }) as SceneDayShow

const room = (over: Partial<SceneTrackedVenue> = {}): SceneTrackedVenue =>
  ({ name: 'Valley Bar', slug: 'valley-bar', website: '', ...over }) as SceneTrackedVenue

const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
  ({
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-07-31',
    timezone: 'America/Phoenix',
    iso_week: '2026-W31',
    show_count: 1,
    prev_date: '2026-07-30',
    next_date: '2026-08-01',
    is_tonight: true,
    is_past_day: false,
    shows: [show()],
    tracked_venues: [room(), room({ name: 'Crescent Ballroom', slug: 'crescent-ballroom' })],
    ...over,
  }) as SceneDayResponse

describe('SceneDayView — share affordance', () => {
  afterEach(() => {
    Reflect.deleteProperty(navigator, 'clipboard')
    vi.restoreAllMocks()
  })

  it('shares the DATED permalink, never the rolling /tonight URL', async () => {
    // This page is reachable at both `/scenes/{slug}/{date}` and the rolling
    // `/scenes/{slug}/tonight`. Sharing the rolling URL would hand a friend a
    // page whose contents change tomorrow, so the control must emit the dated
    // permalink regardless of which route rendered it.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })

    render(<SceneDayView day={day()} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Share this night' })
    )

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
      )
    )
  })

  // An archived night is the most shareable page in the family, so the control
  // is not gated on the night being tonight.
  it('offers the same control on a night that has already happened', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })

    render(
      <SceneDayView
        day={day({ date: '2024-03-15', is_tonight: false, is_past_day: true })}
      />
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Share this night' })
    )

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        'https://psychichomily.com/scenes/phoenix-az/2024-03-15'
      )
    )
  })

  // Both halves in one test on purpose. jsdom exposes neither `navigator.share`
  // nor `navigator.clipboard`, so the absence alone would also pass with the
  // control deleted outright — the presence half is what makes the absence mean
  // "no mechanism" rather than "no component".
  it('renders no share control when the browser can neither share nor copy', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
      writable: true,
    })
    const withClipboard = render(<SceneDayView day={day()} />)
    expect(
      await screen.findByRole('button', { name: 'Share this night' })
    ).toBeInTheDocument()
    withClipboard.unmount()

    Reflect.deleteProperty(navigator, 'clipboard')
    render(<SceneDayView day={day()} />)
    expect(
      screen.queryByRole('button', { name: 'Share this night' })
    ).not.toBeInTheDocument()
  })
})

describe('SceneDayView — a night with shows', () => {
  it('renders the city with its state alongside', () => {
    render(<SceneDayView day={day()} />)
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1).toHaveTextContent('Phoenix')
    // Cold arrivals from a shared link need it: "Columbus" is ambiguous.
    expect(h1).toHaveTextContent('AZ')
  })

  // The WHOLE line, exactly. Three separate substring assertions all resolve to
  // this same <p>, so they would pass with the segments reordered, the em dash
  // swapped for a comma, or the separator changed — and this string is locked.
  it('states the night as one locked line, counting the rows it renders', () => {
    const shows = [show({ id: 1 }), show({ id: 2 }), show({ id: 3 }), show({ id: 4 })]
    // `show_count` deliberately disagrees: the header counts what is on the
    // page, so it can never advertise a show the reader cannot find.
    render(<SceneDayView day={day({ shows, show_count: 99 })} />)
    expect(
      screen.getByText('Tonight — Friday, July 31, 2026 · 4 shows')
    ).toBeInTheDocument()
  })

  it('drops only the Tonight prefix on a dated permalink', () => {
    render(<SceneDayView day={day({ is_tonight: false })} />)
    expect(screen.getByText('Friday, July 31, 2026 · 1 show')).toBeInTheDocument()
  })

  it('singularises a one-show night', () => {
    render(<SceneDayView day={day()} />)
    expect(screen.getByText(/1 show(?!s)/)).toBeInTheDocument()
  })

  // The dated permalink is a permanent URL; calling an archived Tuesday
  // "tonight" would be false the day after it was written.
  it('drops the tonight framing on a dated permalink that is not tonight', () => {
    render(<SceneDayView day={day({ is_tonight: false })} />)
    expect(screen.queryByText(/Tonight/)).not.toBeInTheDocument()
    expect(screen.getByText(/Friday, July 31, 2026/)).toBeInTheDocument()
  })

  // The acceptance criterion in full: artist, venue AND time, in the markup.
  it('lists each show with its bill, venue and venue-local start time', () => {
    render(<SceneDayView day={day()} />)
    const link = screen.getByRole('link', { name: /Smooth Hands, Tournament/ })
    expect(link).toHaveAttribute('href', '/shows/1')
    expect(within(link).getByText('Valley Bar')).toBeInTheDocument()
    // 03:00Z is 20:00 in Phoenix — a UTC render would say 3:00 AM.
    expect(within(link).getByText('8:00 PM')).toBeInTheDocument()
  })

  it('renders a price when the show has one', () => {
    render(<SceneDayView day={day({ shows: [show({ price: 22 })] })} />)
    expect(screen.getByText('$22')).toBeInTheDocument()
  })

  it('renders no price when the show has none recorded', () => {
    render(<SceneDayView day={day()} />)
    expect(screen.queryByText(/^\$/)).not.toBeInTheDocument()
  })

  it('badges a sold-out show', () => {
    render(<SceneDayView day={day({ shows: [show({ is_sold_out: true })] })} />)
    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  // Cancelled outranks sold out — a cancelled show reading "SOLD OUT" would
  // actively mislead someone deciding whether to go.
  it('badges a cancelled show and suppresses the sold-out badge', () => {
    render(
      <SceneDayView day={day({ shows: [show({ is_cancelled: true, is_sold_out: true })] })} />
    )
    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
    expect(screen.queryByText('SOLD OUT')).not.toBeInTheDocument()
  })

  it('always discloses that coverage is partial', () => {
    render(<SceneDayView day={day()} />)
    expect(screen.getByText(/Not a complete city listing/)).toBeInTheDocument()
    expect(screen.getByText(/ROOMS WE TRACK IN PHOENIX/)).toBeInTheDocument()
  })

  // PSY-1733: listing nights keep the slug data and link each room to its
  // page here — not an external website.
  it('links tracked rooms in the listing footer to their venue pages', () => {
    render(<SceneDayView day={day()} />)
    // Scope to the footer: show rows also mention venue names inside the
    // whole-row show link, and a bare name query would match those too.
    const footer = screen.getByText(/ROOMS WE TRACK IN PHOENIX/).closest('footer')
    expect(footer).not.toBeNull()
    expect(within(footer as HTMLElement).getByRole('link', { name: 'Valley Bar' })).toHaveAttribute(
      'href',
      '/venues/valley-bar'
    )
    expect(
      within(footer as HTMLElement).getByRole('link', { name: 'Crescent Ballroom' })
    ).toHaveAttribute('href', '/venues/crescent-ballroom')
    const footerLinks = within(footer as HTMLElement)
      .getAllByRole('link')
      .filter(a => a.getAttribute('href')?.startsWith('/venues/'))
    expect(footerLinks).toHaveLength(2)
    expect(
      within(footer as HTMLElement)
        .getAllByRole('link')
        .every(a => {
          const href = a.getAttribute('href') ?? ''
          return href.startsWith('/venues/') || href === '/contribute'
        })
    ).toBe(true)
  })

  it('names a tracked room without a slug, unlinked, in the listing footer', () => {
    render(
      <SceneDayView
        day={day({
          tracked_venues: [room({ name: 'DIY Basement', slug: '' })],
        })}
      />
    )
    const footer = screen.getByText(/ROOMS WE TRACK IN PHOENIX/).closest('footer')
    expect(footer).not.toBeNull()
    expect(within(footer as HTMLElement).getByText('DIY Basement')).toBeInTheDocument()
    expect(
      within(footer as HTMLElement).queryByRole('link', { name: 'DIY Basement' })
    ).not.toBeInTheDocument()
  })

  it('treats a whitespace-only slug as missing, not a broken /venues/ URL', () => {
    render(
      <SceneDayView
        day={day({
          tracked_venues: [room({ name: 'Whitespace Room', slug: '   ' })],
        })}
      />
    )
    const footer = screen.getByText(/ROOMS WE TRACK IN PHOENIX/).closest('footer')
    expect(footer).not.toBeNull()
    expect(
      within(footer as HTMLElement).queryByRole('link', { name: 'Whitespace Room' })
    ).not.toBeInTheDocument()
  })

  it('offers adjacent-day navigation and a way to the week', () => {
    render(<SceneDayView day={day()} />)
    expect(screen.getByRole('link', { name: /Thu Jul 30/ })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/2026-07-30'
    )
    expect(screen.getByRole('link', { name: /Sat Aug 1/ })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/2026-08-01'
    )
    expect(screen.getByRole('link', { name: 'Full week' })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/week'
    )
  })

  // A page about a night two months ago must not link to whatever week it
  // happens to be now.
  it('links a past night to its OWN week, not the rolling one', () => {
    render(<SceneDayView day={day({ is_tonight: false })} />)
    expect(screen.getByRole('link', { name: 'Full week' })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/2026-W31'
    )
  })

  // The generator types these nullable even though the API always emits arrays.
  it('survives null shows and tracked_venues', () => {
    render(<SceneDayView day={day({ shows: null, tracked_venues: null, show_count: 0 })} />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Phoenix')
    expect(screen.queryByText(/ROOMS WE TRACK/)).not.toBeInTheDocument()
  })
})

describe('SceneDayView — a quiet night', () => {
  const quiet = (over: Partial<SceneDayResponse> = {}) =>
    day({
      city: 'Tucson',
      slug: 'tucson-az',
      scene_name: 'Tucson, AZ',
      date: '2026-07-30',
      show_count: 0,
      shows: [],
      tracked_venues: [
        room({ name: 'Club Congress', slug: 'club-congress', website: 'https://hotelcongress.com' }),
        room({ name: 'La Rosa', slug: 'la-rosa', website: '' }),
      ],
      next_show: show({
        id: 9,
        event_date: '2026-07-31',
        artist_names: ['In Lessons'],
        venue_name: 'Club Congress',
      }),
      ...over,
    })

  // The whole point of the amendment: never assert that no show EXISTS, only
  // that none is on our calendar.
  it('never claims the city is empty, only that our calendar is', () => {
    render(<SceneDayView day={quiet()} />)
    expect(
      screen.getByText(
        /Nothing on our calendar for the Tucson rooms we track tonight\. A room may have a show we haven't listed\./
      )
    ).toBeInTheDocument()
    expect(screen.getByText(/0 shows listed/)).toBeInTheDocument()
  })

  it('points at the next show on our calendar', () => {
    render(<SceneDayView day={quiet()} />)
    const link = screen.getByRole('link', { name: /Next on our calendar/ })
    expect(link).toHaveTextContent('Next on our calendar: Friday, In Lessons at Club Congress')
    expect(link).toHaveAttribute('href', '/shows/9')
  })

  it('offers the week', () => {
    render(<SceneDayView day={quiet()} />)
    expect(screen.getByRole('link', { name: /Full week in Tucson/ })).toHaveAttribute(
      'href',
      '/scenes/tucson-az/week'
    )
  })

  // Being told we have nothing is exactly when a reader needs the means to
  // check for themselves — on OUR venue pages (PSY-1733), not off-site.
  it('lists the rooms as links to their venue pages here', () => {
    render(<SceneDayView day={quiet()} />)
    expect(screen.getByText(/CHECK THE ROOMS DIRECTLY/)).toBeInTheDocument()

    expect(screen.getByRole('link', { name: 'Club Congress' })).toHaveAttribute(
      'href',
      '/venues/club-congress'
    )
    expect(screen.getByRole('link', { name: 'La Rosa' })).toHaveAttribute(
      'href',
      '/venues/la-rosa'
    )
    // External website on the payload must not become the href.
    expect(screen.queryByRole('link', { name: 'Club Congress' })).not.toHaveAttribute(
      'href',
      'https://hotelcongress.com/'
    )
  })

  it('ignores an unsafe website value and still links via slug', () => {
    render(
      <SceneDayView
        day={quiet({
          tracked_venues: [
            room({ name: 'La Rosa', slug: 'la-rosa', website: 'javascript:alert(1)' }),
          ],
        })}
      />
    )
    expect(screen.getByRole('link', { name: 'La Rosa' })).toHaveAttribute(
      'href',
      '/venues/la-rosa'
    )
  })

  it('names a room with no slug, unlinked', () => {
    render(
      <SceneDayView
        day={quiet({ tracked_venues: [room({ name: 'RV Phone Home', slug: '', website: '' })] })}
      />
    )
    expect(screen.getByText('RV Phone Home')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'RV Phone Home' })).not.toBeInTheDocument()
  })

  it('says "on {date}" rather than "tonight" for a dated quiet night', () => {
    render(<SceneDayView day={quiet({ is_tonight: false })} />)
    expect(
      screen.getByText(/rooms we track on Thursday, July 30, 2026\./)
    ).toBeInTheDocument()
  })
})

describe('SceneDayView — a dead-quiet scene', () => {
  const dead = day({
    city: 'Cleveland',
    slug: 'cleveland-oh',
    scene_name: 'Cleveland, OH',
    state: 'OH',
    show_count: 0,
    shows: [],
    next_show: undefined,
    tracked_venues: [room({ name: 'Grog Shop', slug: 'grog-shop', website: '' })],
  })

  it('extends the copy to the weeks ahead, and still does not speak for the city', () => {
    render(<SceneDayView day={dead} />)
    expect(
      screen.getByText(
        /Nothing on our calendar for the Cleveland rooms we track tonight, or in the next few weeks\. A room may have shows we haven't listed\./
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/Next on our calendar/)).not.toBeInTheDocument()
  })

  // A scene with nothing ahead of it is the one most likely to be missing a
  // room, so this is where the ask belongs.
  it('asks for the room we are missing', () => {
    render(<SceneDayView day={dead} />)
    expect(screen.getByRole('link', { name: /Suggest a venue/ })).toHaveAttribute(
      'href',
      '/contribute'
    )
  })

  // A scene whose room list came back empty is likelier still to be missing
  // one, which is exactly when nesting the ask inside the rooms block hid it.
  it('still asks when there are no rooms to list', () => {
    render(<SceneDayView day={day({ ...dead, tracked_venues: [] })} />)
    expect(screen.queryByText(/CHECK THE ROOMS DIRECTLY/)).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Suggest a venue/ })).toBeInTheDocument()
  })
})

describe('SceneDayView — a night that has already happened', () => {
  const past = day({
    date: '2020-01-15',
    is_tonight: false,
    is_past_day: true,
    show_count: 0,
    shows: [],
    next_show: undefined,
  })

  // "or in the next few weeks" is a claim about NOW. The look-ahead behind it
  // starts at the day being viewed, so on a 2020 page that window closed six
  // years ago and was never re-checked — the page must not assert it.
  it('does not claim anything about the weeks ahead', () => {
    render(<SceneDayView day={past} />)
    expect(screen.queryByText(/in the next few weeks/)).not.toBeInTheDocument()
    expect(
      screen.getByText(
        /Nothing on our calendar for the Phoenix rooms we track on Wednesday, January 15, 2020\./
      )
    ).toBeInTheDocument()
  })

  // The pointer is a live-night affordance. On an archived page "next" could
  // only name a show that is itself long over.
  it('offers no next-show pointer', () => {
    render(<SceneDayView day={past} />)
    expect(screen.queryByText(/Next on our calendar/)).not.toBeInTheDocument()
  })

  // The ask belongs to the DEAD-QUIET state — a scene with nothing ahead of it.
  // An archived empty Tuesday says nothing about the scene's current calendar,
  // and the server never sends a pointer for a past date, so keying the ask on
  // the pointer's absence alone would solicit on every archived day.
  it('does not solicit venues on the strength of an empty archived day', () => {
    render(<SceneDayView day={past} />)
    expect(screen.queryByRole('link', { name: /Suggest a venue/ })).not.toBeInTheDocument()
  })
})

describe('SceneDayView — the edges of the servable window', () => {
  // The server sends an empty adjacent date when there is no servable day that
  // way. Rendering the chip anyway would advertise a link the site 404s.
  it('renders no chip for an adjacent day the server did not offer', () => {
    const { container } = render(<SceneDayView day={day({ prev_date: '', next_date: '' })} />)

    const datedLinks = [...container.querySelectorAll('a[href]')].filter(a =>
      /^\/scenes\/phoenix-az\/\d{4}-\d{2}-\d{2}$/.test(a.getAttribute('href') ?? '')
    )
    expect(datedLinks).toHaveLength(0)
    // The way out is still there.
    expect(screen.getByRole('link', { name: 'Full week' })).toBeInTheDocument()
  })

  it('renders both chips when the server offers both dates', () => {
    const { container } = render(<SceneDayView day={day()} />)

    const datedLinks = [...container.querySelectorAll('a[href]')]
      .map(a => a.getAttribute('href'))
      .filter(href => /^\/scenes\/phoenix-az\/\d{4}-\d{2}-\d{2}$/.test(href ?? ''))
    expect(datedLinks).toEqual([
      '/scenes/phoenix-az/2026-07-30',
      '/scenes/phoenix-az/2026-08-01',
    ])
  })
})
