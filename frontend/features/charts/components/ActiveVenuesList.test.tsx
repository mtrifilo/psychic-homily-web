import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@/test/utils'
import { ActiveVenuesList } from './ActiveVenuesList'
import type { ActiveVenue } from '../types'

function makeVenue(overrides: Partial<ActiveVenue> = {}): ActiveVenue {
  return {
    venue_id: 1,
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    city: 'Phoenix',
    state: 'AZ',
    upcoming_show_count: 18,
    follow_count: 65,
    score: 83,
    ...overrides,
  }
}

describe('ActiveVenuesList', () => {
  it('renders ranked venues in the order provided', () => {
    const venues = [
      makeVenue({ venue_id: 1, name: 'First Venue', slug: 'first-venue' }),
      makeVenue({ venue_id: 2, name: 'Second Venue', slug: 'second-venue' }),
      makeVenue({ venue_id: 3, name: 'Third Venue', slug: 'third-venue' }),
    ]

    render(<ActiveVenuesList venues={venues} />)

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)

    expect(within(items[0]).getByText('1')).toBeInTheDocument()
    expect(within(items[0]).getByText('First Venue')).toBeInTheDocument()
    expect(within(items[1]).getByText('2')).toBeInTheDocument()
    expect(within(items[1]).getByText('Second Venue')).toBeInTheDocument()
    expect(within(items[2]).getByText('3')).toBeInTheDocument()
    expect(within(items[2]).getByText('Third Venue')).toBeInTheDocument()
  })

  it('links each venue to its detail page', () => {
    render(<ActiveVenuesList venues={[makeVenue()]} />)

    const link = screen.getByRole('link', { name: /The Rebel Lounge/ })
    expect(link).toHaveAttribute('href', '/venues/the-rebel-lounge')
  })

  it('renders city with state in full mode', () => {
    render(<ActiveVenuesList venues={[makeVenue({ city: 'Phoenix', state: 'AZ' })]} />)

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('omits the state separator when state is empty', () => {
    render(<ActiveVenuesList venues={[makeVenue({ city: 'Phoenix', state: '' })]} />)

    expect(screen.getByText('Phoenix')).toBeInTheDocument()
    expect(screen.queryByText(/Phoenix,/)).not.toBeInTheDocument()
  })

  it('renders upcoming-show and follower counts in full mode', () => {
    render(<ActiveVenuesList venues={[makeVenue({ upcoming_show_count: 18, follow_count: 65 })]} />)

    expect(screen.getByText('18')).toBeInTheDocument()
    expect(screen.getByText('65')).toBeInTheDocument()
  })

  it('hides city and counts in compact mode', () => {
    render(
      <ActiveVenuesList
        venues={[makeVenue({ city: 'Phoenix', state: 'AZ', upcoming_show_count: 18, follow_count: 65 })]}
        compact
      />
    )

    expect(screen.queryByText('Phoenix, AZ')).not.toBeInTheDocument()
    expect(screen.queryByText('18')).not.toBeInTheDocument()
    expect(screen.queryByText('65')).not.toBeInTheDocument()
    // Name still renders when compact.
    expect(screen.getByText('The Rebel Lounge')).toBeInTheDocument()
  })

  it('renders the empty state when there are no venues', () => {
    render(<ActiveVenuesList venues={[]} />)

    expect(screen.getByText('No active venues right now.')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})
