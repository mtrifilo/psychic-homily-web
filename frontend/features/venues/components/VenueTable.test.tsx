import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { VenueTable } from './VenueTable'
import type { VenueWithShowCount } from '../types'

const LONG_ADDRESS = '2303 East Indian School Road, Suite 400'

function makeVenue(
  overrides: Partial<VenueWithShowCount> = {}
): VenueWithShowCount {
  return {
    id: 1,
    slug: 'a-room',
    name: 'A Room',
    address: '1 Main St',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    verified: true,
    upcoming_show_count: 3,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderTable(venues: VenueWithShowCount[], showCity = false) {
  return render(
    <VenueTable
      venues={venues}
      sort="upcoming"
      onSortChange={vi.fn()}
      showCity={showCity}
    />
  )
}

/**
 * The column widths at table sizes. `DenseTable` is `table-layout: auto`, where
 * a cell's `max-width` is a hint the algorithm may ignore, so the bound and the
 * clip both have to sit on a block child. Without them the longest street
 * address takes the width the room name needs and the name wraps, which is what
 * the mini Atlas pane made visible by narrowing the table to 688px.
 */
describe('VenueTable column widths', () => {
  it('clips a long address and keeps the whole of it available', () => {
    renderTable([makeVenue({ address: LONG_ADDRESS })])

    const address = screen.getByTitle(LONG_ADDRESS)
    expect(address).toHaveTextContent(LONG_ADDRESS)
    // The clip and its bound are on the span, not on the cell.
    expect(address.className).toContain('truncate')
    expect(address.className).toContain('max-w-')
    expect(address.tagName).toBe('SPAN')
  })

  it('reads the address whole to assistive tech, ellipsis notwithstanding', () => {
    // `text-overflow` is presentational: the full string stays in the DOM, so a
    // screen reader gets the address the sighted reader has to hover for.
    renderTable([makeVenue({ address: LONG_ADDRESS })])

    expect(screen.getByText(LONG_ADDRESS)).toBeInTheDocument()
  })

  it('carries the city ahead of the street when the list is not one city', () => {
    renderTable([makeVenue({ address: LONG_ADDRESS })], true)

    const expected = `Phoenix, AZ · ${LONG_ADDRESS}`
    expect(screen.getByTitle(expected)).toHaveTextContent(expected)
  })

  it('has no title to offer when the room has no address', () => {
    renderTable([makeVenue({ address: null })])

    expect(screen.queryByTitle('')).not.toBeInTheDocument()
  })

  it('keeps a long room name on one line at table widths', () => {
    renderTable([
      makeVenue({ id: 1, name: 'Arizona Financial Theatre', verified: true }),
    ])

    const cell = screen.getByText('Arizona Financial Theatre').closest('td')
    expect(cell?.className).toContain('sm:whitespace-nowrap')
  })
})
