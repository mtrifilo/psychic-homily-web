import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { VenueWithShowCount } from '@/features/venues/types'
import { NO_CITY_VENUE_FILTERS, type VenuePinStack } from '../cityView'
import { VenueListSheet } from './VenueListSheet'

function venue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'mohawk-austin-tx',
    name: 'Mohawk',
    address: null,
    city: 'Austin',
    state: 'TX',
    verified: true,
    upcoming_show_count: 14,
    shows_this_week: 3,
    next_show_date: '2026-07-28',
    next_show_artists: ['Gouge Away'],
    dominant_genre: 'punk_hardcore',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  } as VenueWithShowCount
}

const venues = [venue(), venue({ id: 2, slug: 'hotel-vegas', name: 'Hotel Vegas' })]
const stack: VenuePinStack = {
  key: '-97.743100,30.267200',
  lng: -97.7431,
  lat: 30.2672,
  venueIds: [1, 2],
  atCentroid: true,
}

function renderSheet(props: Partial<React.ComponentProps<typeof VenueListSheet>> = {}) {
  return render(
    <VenueListSheet
      principalCity="Austin"
      venues={venues}
      allVenues={venues}
      filters={NO_CITY_VENUE_FILTERS}
      onFiltersChange={vi.fn()}
      selectedVenueId={null}
      onVenueSelect={vi.fn()}
      detent="half"
      onDetentChange={vi.fn()}
      stacks={[stack]}
      inViewCount={2}
      scopedStack={null}
      onClearScope={vi.fn()}
      {...props}
    />,
  )
}

describe('VenueListSheet touch targets', () => {
  it('gives every filter chip a 24px-tall target', () => {
    renderSheet()
    for (const name of ['Next 7 days', 'All-ages shows', 'Record stores']) {
      expect(screen.getByRole('button', { name })).toHaveClass('min-h-6')
    }
    // The genre menu is a transparent native select laid over its chip, so
    // the chip's box is the select's box.
    const genre = screen.getByRole('combobox', { name: 'Filter venues by genre' })
    expect(genre).toHaveClass('absolute', 'inset-0')
    expect(genre.nextElementSibling).toHaveClass('min-h-6')
  })

  it('gives the scope reset a 24px-tall target', () => {
    renderSheet({ scopedStack: stack })
    expect(screen.getByRole('button', { name: 'Show all' })).toHaveClass('min-h-6')
  })
})
