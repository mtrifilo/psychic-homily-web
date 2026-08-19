import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ShowResponse } from '../types'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: undefined }),
}))

// The save bracket reads the router for its login redirect; there is no app
// router mounted in a render-only test.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/shows/test-show',
}))

vi.mock('../hooks/useSavedShows', () => ({
  useSaveShow: () => ({ mutate: vi.fn(), isPending: false }),
  useShowSaveCount: () => ({ data: undefined }),
  useSaveShowToggle: () => ({
    isLoading: false,
    toggle: vi.fn(),
    error: null,
  }),
}))

// The collection affordance needs a QueryClientProvider this render-only
// test doesn't mount, and the share affordance self-hides in jsdom (no
// clipboard/share capability); both behaviours are covered in their own
// test files.
vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
  AddToCollectionButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="add-to-collection" data-entity-name={entityName}>
      [Add to collection]
    </button>
  ),
  ShareButton: ({ path }: { path: string | null }) => (
    <button data-testid="share-button" data-path={path ?? ''}>
      [Share]
    </button>
  ),
}))

import { ShowTicketRow } from './ShowTicketRow'
import { ticketLineSegments } from './showTicketLine'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: '',
    // 8 PM Aug 12 in Chicago.
    event_date: '2026-08-13T01:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 1,
        slug: 'salt-shed',
        name: 'Salt Shed',
        city: 'Chicago',
        state: 'IL',
        timezone: 'America/Chicago',
        verified: true,
      },
    ],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}

describe('ticketLineSegments', () => {
  // The start time leads (user decision): for the common show with no
  // announced doors/music this line is the only clock on the page. It is
  // rendered in the STRIPE's register ("8PM"), so one page never spells the
  // same clock two ways.
  it('leads with the venue-local start time in the stripe register', () => {
    expect(ticketLineSegments(makeShow())[0]).toBe('8PM')
  })

  // Same refusal rule as the stripe and the venue facts line: a venue whose
  // timezone is a guess gets no confidently-wrong hour.
  it('prints no clock when the venue timezone is a guess', () => {
    const segments = ticketLineSegments(
      makeShow({
        venues: [
          {
            id: 1,
            slug: 'berlin-venue',
            name: 'Berlin Venue',
            city: 'Berlin',
            state: 'Berlin',
            timezone: null,
            verified: true,
          },
        ],
      })
    )
    expect(segments.some(segment => /\dPM|\dAM/.test(segment))).toBe(false)
  })

  it('claims ON SALE only when there is somewhere to buy', () => {
    expect(ticketLineSegments(makeShow())).not.toContain('ON SALE')
    expect(
      ticketLineSegments(makeShow({ ticket_url: 'https://tix.example/1' }))
    ).toContain('ON SALE')
  })

  it('swaps the sale state for SOLD OUT', () => {
    const segments = ticketLineSegments(
      makeShow({ ticket_url: 'https://tix.example/1', is_sold_out: true })
    )
    expect(segments).toContain('SOLD OUT')
    expect(segments).not.toContain('ON SALE')
  })

  // The stripe says CANCELLED at the top of the page; this line must not
  // argue with it.
  it('never claims ON SALE for a cancelled show', () => {
    expect(
      ticketLineSegments(
        makeShow({ ticket_url: 'https://tix.example/1', is_cancelled: true })
      )
    ).not.toContain('ON SALE')
  })

  it('renders the single price and says nothing about a door split', () => {
    expect(ticketLineSegments(makeShow({ price: 35 }))).toContain('$35.00')
  })

  it('renders a zero price as Free', () => {
    expect(ticketLineSegments(makeShow({ price: 0 }))).toContain('Free')
  })

  it('omits the price segment when no price is known', () => {
    const segments = ticketLineSegments(makeShow({ price: null }))
    expect(segments.join(' ')).not.toContain('$')
  })

  // The venue facts line owns the age fact, but a venue-less show never
  // mounts that module — the line falls back so "21+" cannot vanish from
  // the page.
  it('carries the age requirement only for a venue-less show', () => {
    expect(
      ticketLineSegments(makeShow({ venues: [], age_requirement: '21+' }))
    ).toContain('21+')
    expect(
      ticketLineSegments(makeShow({ age_requirement: '21+' }))
    ).not.toContain('21+')
  })
})

describe('ShowTicketRow', () => {
  it('renders Buy Tickets as an outbound bracket in a new tab', () => {
    render(
      <ShowTicketRow show={makeShow({ ticket_url: 'https://tix.example/1' })} />
    )

    const buy = screen.getByRole('link', { name: /Buy tickets/i })
    expect(buy).toHaveAttribute('href', 'https://tix.example/1')
    expect(buy).toHaveAttribute('target', '_blank')
    expect(buy).toHaveAttribute('rel', 'noopener noreferrer')
  })

  // The backend stores ticket urls as typed; the repair is scheme-anchored
  // and case-insensitive, not a bare prefix test.
  it('repairs a scheme-less ticket url to https', () => {
    render(<ShowTicketRow show={makeShow({ ticket_url: 'tix.example/1' })} />)

    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://tix.example/1'
    )
  })

  it('leaves an uppercase scheme alone and repairs protocol-relative urls', () => {
    const { rerender } = render(
      <ShowTicketRow show={makeShow({ ticket_url: 'HTTPS://tix.example/1' })} />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'HTTPS://tix.example/1'
    )

    rerender(
      <ShowTicketRow show={makeShow({ ticket_url: '//tix.example/1' })} />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://tix.example/1'
    )
  })

  // A prefix-passing non-scheme would otherwise ship as a RELATIVE href
  // resolving under /shows/.
  it('prefixes a value that merely starts with the letters http', () => {
    render(
      <ShowTicketRow show={makeShow({ ticket_url: 'httpfoo.example/1' })} />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://httpfoo.example/1'
    )
  })

  it('renders no Buy Tickets bracket without a ticket url', () => {
    render(<ShowTicketRow show={makeShow()} />)
    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // PSY-1666 coupling: the calendar affordance (which saves as a side
  // effect) and the save bracket share this row and the same query key.
  it('renders the full mock action row: calendar, save, collection, share', () => {
    render(<ShowTicketRow show={makeShow()} />)

    expect(screen.getByText('Add to calendar')).toBeInTheDocument()
    expect(screen.getByText('Save')).toBeInTheDocument()
    expect(screen.getByTestId('add-to-collection')).toBeInTheDocument()
    expect(screen.getByTestId('share-button')).toHaveAttribute(
      'data-path',
      '/shows/test-show'
    )
  })

  it('names the collection entry from the bill when the show has no title', () => {
    render(
      <ShowTicketRow
        show={makeShow({
          title: '',
          artists: [
            {
              id: 1,
              slug: 'modest-mouse',
              name: 'Modest Mouse',
              set_type: 'headliner',
              position: 0,
              socials: {},
            },
          ],
        })}
      />
    )

    expect(screen.getByTestId('add-to-collection')).toHaveAttribute(
      'data-entity-name',
      'Modest Mouse'
    )
  })
})
