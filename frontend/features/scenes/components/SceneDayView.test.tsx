import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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

describe('SceneDayView — a night with shows', () => {
  it('renders the city with its state alongside', () => {
    render(<SceneDayView day={day()} />)
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1).toHaveTextContent('Phoenix')
    // Cold arrivals from a shared link need it: "Columbus" is ambiguous.
    expect(h1).toHaveTextContent('AZ')
  })

  it('states the night and counts the rows it actually renders', () => {
    const shows = [show({ id: 1 }), show({ id: 2 }), show({ id: 3 }), show({ id: 4 })]
    // `show_count` deliberately disagrees: the header counts what is on the
    // page, so it can never advertise a show the reader cannot find.
    render(<SceneDayView day={day({ shows, show_count: 99 })} />)
    expect(screen.getByText(/Tonight/)).toBeInTheDocument()
    expect(screen.getByText(/Friday, July 31, 2026/)).toBeInTheDocument()
    expect(screen.getByText(/4 shows/)).toBeInTheDocument()
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
    expect(screen.getByText('$22.00')).toBeInTheDocument()
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
  // check for themselves.
  it('lists the rooms — their own site when we have one, their page here otherwise', () => {
    render(<SceneDayView day={quiet()} />)
    expect(screen.getByText(/CHECK THE ROOMS DIRECTLY/)).toBeInTheDocument()

    // Named exactly: "Club Congress" also appears inside the next-show pointer
    // above, and the ↗ is what marks this one as leaving the site.
    const external = screen.getByRole('link', { name: 'Club Congress ↗' })
    expect(external).toHaveAttribute('href', 'https://hotelcongress.com/')
    expect(external).toHaveAttribute('target', '_blank')
    expect(external).toHaveAttribute('rel', 'noopener noreferrer nofollow')

    expect(screen.getByRole('link', { name: 'La Rosa' })).toHaveAttribute(
      'href',
      '/venues/la-rosa'
    )
  })

  // Operator-supplied data reaching an href: a stored `javascript:` value must
  // never become a link.
  it('falls back to the venue page for an unsafe website value', () => {
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

  it('names a room with neither a site nor a page, unlinked', () => {
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
})
