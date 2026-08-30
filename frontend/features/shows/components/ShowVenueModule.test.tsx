import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ShowResponse, VenueResponse } from '../types'

// Follow / notify affordances need a QueryClientProvider these render-only
// tests don't mount; mocked at the component boundary (VenueDetail.test
// convention). Their own behaviour lives in their own test files. The mocks
// surface the props this module is responsible for getting right — the
// follow ENTITY TYPE especially, which must be the PLURAL route segment.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({
    entityType,
    bracketLabel,
  }: {
    entityType: string
    bracketLabel?: string
  }) => (
    <button data-testid="follow-venue" data-entity-type={entityType}>
      [{bracketLabel ?? 'Follow'}]
    </button>
  ),
}))

import { ShowVenueModule } from './ShowVenueModule'
import { venueFactSegments } from './showVenueFacts'

function makeVenue(overrides: Partial<VenueResponse> = {}): VenueResponse {
  return {
    id: 1,
    slug: 'salt-shed',
    name: 'Salt Shed',
    address: '1357 N Elston Ave',
    city: 'Chicago',
    state: 'IL',
    timezone: 'America/Chicago',
    verified: true,
    ...overrides,
  }
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-08-13T01:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [makeVenue()],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}

/** A show whose only venue is `venue` — the shape the module reads. */
function showAt(
  venue: VenueResponse,
  overrides: Partial<ShowResponse> = {}
): ShowResponse {
  return makeShow({ venues: [venue], ...overrides })
}

describe('venueFactSegments', () => {
  it('renders capacity with a grouping comma and the ~ that marks a community estimate', () => {
    const venue = makeVenue({ capacity: 3600 })
    expect(venueFactSegments(showAt(venue), venue)).toContain('CAP ~3,600')
  })

  it('omits capacity when unknown or zero', () => {
    const unknown = makeVenue({ capacity: null })
    expect(venueFactSegments(showAt(unknown), unknown)).toEqual([])
    const zero = makeVenue({ capacity: 0 })
    expect(venueFactSegments(showAt(zero), zero)).toEqual([])
  })

  it('marks the event age as an override over the house default when they differ', () => {
    const venue = makeVenue({ age_policy: 'all ages' })
    expect(
      venueFactSegments(showAt(venue, { age_requirement: '17+' }), venue)
    ).toContain('17+ (event override; house default all ages)')
  })

  it('collapses the age fragment when override and house default agree', () => {
    const venue = makeVenue({ age_policy: '21+' })
    const segments = venueFactSegments(
      showAt(venue, { age_requirement: '21+' }),
      venue
    )
    expect(segments).toContain('21+')
    expect(segments.join(' ')).not.toContain('override')
  })

  it('renders the house default alone when the show has no override', () => {
    const venue = makeVenue({ age_policy: 'all ages' })
    expect(venueFactSegments(showAt(venue), venue)).toContain('all ages')
  })

  // Same copy rules as the status stripe: venue-local clock register, and the
  // line hangs off doors — see doorsMusicFactSegment.
  it('renders announced doors and music venue-local in the stripe register', () => {
    const venue = makeVenue()
    const segments = venueFactSegments(
      showAt(venue, {
        // 7 PM / 8 PM Aug 12 in Chicago.
        doors_at: '2026-08-13T00:00:00Z',
        music_at: '2026-08-13T01:00:00Z',
      }),
      venue
    )
    expect(segments).toContain('DOORS 7PM / MUSIC 8PM')
  })

  it('renders doors alone when only doors is announced', () => {
    const venue = makeVenue()
    expect(
      venueFactSegments(showAt(venue, { doors_at: '2026-08-13T00:00:00Z' }), venue)
    ).toContain('DOORS 7PM')
  })

  it('drops a lone music time; half a schedule is not a statement', () => {
    const venue = makeVenue()
    expect(
      venueFactSegments(showAt(venue, { music_at: '2026-08-13T01:00:00Z' }), venue)
    ).toEqual([])
  })

  it('prints no clock when the venue timezone is a guess', () => {
    // No resolvable zone: no venue timezone and a state outside the US map.
    const venue = makeVenue({ timezone: null, state: 'Berlin' })
    expect(
      venueFactSegments(showAt(venue, { doors_at: '2026-08-13T00:00:00Z' }), venue)
    ).toEqual([])
  })
})

describe('ShowVenueModule', () => {
  it('renders nothing for a venue-less show', () => {
    const { container } = render(
      <ShowVenueModule show={makeShow({ venues: [] })} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the venue name linked, with the street address and city inline', () => {
    render(<ShowVenueModule show={makeShow()} />)

    expect(screen.getByRole('link', { name: 'Salt Shed' })).toHaveAttribute(
      'href',
      '/venues/salt-shed'
    )
    expect(screen.getByTestId('venue-address')).toHaveTextContent(
      '1357 N Elston Ave'
    )
    expect(screen.getByTestId('venue-location')).toHaveTextContent(
      '1357 N Elston Ave, Chicago, IL'
    )
  })

  // An unverified venue's street address arrives redacted server-side; the
  // module says less rather than leaving a stray comma.
  it('degrades to city and state when the address is absent', () => {
    render(<ShowVenueModule show={showAt(makeVenue({ address: null }))} />)

    expect(screen.queryByTestId('venue-address')).not.toBeInTheDocument()
    expect(screen.getByTestId('venue-location').textContent?.trim()).toBe(
      'Chicago, IL'
    )
  })

  // formatLocation's stand-alone placeholder must not be spliced into the
  // middle of an address line.
  it('drops the location segment rather than printing Location Unknown mid-line', () => {
    render(
      <ShowVenueModule show={showAt(makeVenue({ city: '', state: '' }))} />
    )

    expect(screen.getByTestId('venue-location')).toHaveTextContent(
      '1357 N Elston Ave'
    )
    expect(screen.getByTestId('venue-location').textContent).not.toContain(
      'Location Unknown'
    )
    expect(
      screen.getByTestId('venue-location').textContent?.trim().endsWith(',')
    ).toBe(false)
  })

  it('hides the facts line entirely when no fact survives', () => {
    render(<ShowVenueModule show={makeShow()} />)
    expect(screen.queryByTestId('venue-facts')).not.toBeInTheDocument()
  })

  it('separates surviving facts with middots', () => {
    render(
      <ShowVenueModule
        show={showAt(makeVenue({ capacity: 3600 }), {
          age_requirement: '17+',
        })}
      />
    )
    expect(screen.getByTestId('venue-facts')).toHaveTextContent(
      'CAP ~3,600 · 17+'
    )
  })

  it('links Directions to a Google Maps search in a new tab', () => {
    render(<ShowVenueModule show={makeShow()} />)

    // Names the venue AND the destination app: the component-owned suffix says
    // a new tab opens, but only the call site can say where it lands.
    const directions = screen.getByRole('link', {
      name: /^Directions to Salt Shed on Google Maps\b/,
    })
    expect(directions).toHaveAttribute('target', '_blank')
    expect(directions).toHaveAttribute('rel', 'noopener noreferrer')
    expect(
      directions.getAttribute('aria-label')?.match(/opens in a new tab/g)
    ).toHaveLength(1)
    expect(directions.getAttribute('href')).toBe(
      `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(
        'Salt Shed, 1357 N Elston Ave, Chicago, IL'
      )}`
    )
  })

  // The venue page refuses to map an unverified venue (a name + city search
  // narrows a house show to a door); the show page must apply the same rule.
  it('offers no Directions for an unverified venue', () => {
    render(
      <ShowVenueModule show={showAt(makeVenue({ verified: false, address: null }))} />
    )
    expect(
      screen.queryByRole('link', { name: /Directions/ })
    ).not.toBeInTheDocument()
  })

  // Blank venue names are a modeled case; a Directions bracket pointing at
  // an EMPTY maps query is a dead verb, not an affordance.
  it('offers no Directions when there is nothing to search for', () => {
    render(
      <ShowVenueModule
        show={showAt(
          makeVenue({ name: '  ', address: null, city: '', state: '' })
        )}
      />
    )
    expect(
      screen.queryByRole('link', { name: /Directions/ })
    ).not.toBeInTheDocument()
  })

  // The follow ROUTE segment is plural. Getting it wrong 400s every click,
  // which is the exact bug this pins.
  it('follows through the plural route segment', () => {
    render(<ShowVenueModule show={makeShow()} />)

    expect(screen.getByTestId('follow-venue')).toHaveAttribute(
      'data-entity-type',
      'venues'
    )
  })

  it('labels the follow affordance with the venue, not the show', () => {
    render(<ShowVenueModule show={makeShow()} />)
    expect(screen.getByTestId('follow-venue')).toHaveTextContent(
      '[Follow venue]'
    )
  })

  // PSY-1905: this module used to pair [Follow venue] with [Notify me], two
  // brackets that did different things. Following a venue now subscribes on
  // its own, so the second bracket is gone and must not come back.
  it('offers one subscribe bracket, not a Follow/Notify pair', () => {
    render(<ShowVenueModule show={makeShow()} />)
    expect(screen.queryByTestId('notify-me')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /notify/i })
    ).not.toBeInTheDocument()
  })

  it('renders the more-at link only when the venue has a slug', () => {
    const { rerender } = render(<ShowVenueModule show={makeShow()} />)
    expect(
      screen.getByRole('link', { name: /More at Salt Shed/ })
    ).toHaveAttribute('href', '/venues/salt-shed')

    rerender(<ShowVenueModule show={showAt(makeVenue({ slug: '' }))} />)
    expect(
      screen.queryByRole('link', { name: /More at Salt Shed/ })
    ).not.toBeInTheDocument()
  })
})
