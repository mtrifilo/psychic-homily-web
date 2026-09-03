import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { ArtistShowsTable } from './ArtistShowsTable'
import type { ArtistShow, ArtistShowVenue } from '../types'

/**
 * The artist table's TIME column, and specifically what it does when a row's
 * venue timezone is a guess.
 *
 * An artist's rows span rooms, so unlike the venue table this one can hold a
 * withheld clock and a printed one at once. Both cases are asserted in the same
 * render below, because a refusal that also blanked its neighbours would look
 * correct in a single-row fixture.
 */

// 03:00Z is the previous evening in the fallback zone (UTC-7) and the same
// evening in New York, so a row read on the wrong one lands on a different day
// as well as a different hour.
const EVENT_DATE = '2026-09-10T03:00:00Z'

function makeVenue(overrides: Partial<ArtistShowVenue> = {}): ArtistShowVenue {
  return {
    id: 1,
    slug: 'the-rebel-lounge',
    name: 'The Rebel Lounge',
    city: 'Phoenix',
    state: 'AZ',
    timezone: null,
    ...overrides,
  }
}

function makeShow(
  id: number,
  venue: ArtistShowVenue | null
): ArtistShow {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: EVENT_DATE,
    price: null,
    door_price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    venue,
    artists: [],
  }
}

function timeCellOfRow(index: number): HTMLElement {
  // Row 0 is the header.
  const row = screen.getAllByRole('row')[index + 1]
  return within(row).getAllByRole('cell')[3]
}

describe('ArtistShowsTable time column', () => {
  it('prints the hour for a venue with a resolved zone and withholds it for one without, in the same table', () => {
    render(
      <ArtistShowsTable
        shows={[
          makeShow(1, makeVenue()),
          makeShow(
            2,
            makeVenue({ id: 2, slug: 'hall', name: 'Hall', state: '' })
          ),
        ]}
        ariaLabel="Shows"
      />
    )
    expect(timeCellOfRow(0)).toHaveTextContent('8:00 PM')
    expect(timeCellOfRow(1)).toBeEmptyDOMElement()
  })

  it('prints the hour for a non-US venue that carries its own IANA zone', () => {
    render(
      <ArtistShowsTable
        shows={[
          makeShow(
            1,
            makeVenue({ state: '', timezone: 'Europe/Berlin' })
          ),
        ]}
        ariaLabel="Shows"
      />
    )
    expect(timeCellOfRow(0)).toHaveTextContent('5:00 AM')
  })

  it('withholds the hour for a state outside the US map', () => {
    render(
      <ArtistShowsTable
        shows={[makeShow(1, makeVenue({ state: 'England' }))]}
        ariaLabel="Shows"
      />
    )
    expect(timeCellOfRow(0)).toBeEmptyDOMElement()
  })

  it('withholds the hour for a row with no venue at all', () => {
    render(
      <ArtistShowsTable shows={[makeShow(1, null)]} ariaLabel="Shows" />
    )
    expect(timeCellOfRow(0)).toBeEmptyDOMElement()
  })

  it('keeps the date in a row whose time it withheld', () => {
    render(
      <ArtistShowsTable
        shows={[makeShow(1, makeVenue({ state: 'England' }))]}
        ariaLabel="Shows"
      />
    )
    const row = screen.getAllByRole('row')[1]
    expect(within(row).getAllByRole('cell')[0]).toHaveTextContent('Sep 9')
  })
})
