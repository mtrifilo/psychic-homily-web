import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { VenueShowsTable } from './VenueShowsTable'
import type { VenueShow, VenueShowZone } from '../types'

/**
 * The venue table's TIME column, and specifically what it does when the venue's
 * timezone is a guess.
 *
 * A venue page is the surface where this is most visible: every row shares one
 * zone, so a guessed one is wrong for the whole table at once.
 */

// 03:00Z is the previous evening in the fallback zone (UTC-7) and the same
// evening in New York, so a row read on the wrong one lands on a different day
// as well as a different hour.
const EVENT_DATE = '2026-09-10T03:00:00Z'

function makeShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 1,
    slug: 'show-1',
    title: 'Show 1',
    event_date: EVENT_DATE,
    city: 'Berlin',
    state: null,
    price: null,
    door_price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    artists: [],
    ...overrides,
  }
}

function renderTable(zone: VenueShowZone, show = makeShow()) {
  return render(
    <VenueShowsTable shows={[show]} zone={zone} ariaLabel="Shows" />
  )
}

function timeCell(): HTMLElement {
  const row = screen.getAllByRole('row')[1]
  return within(row).getAllByRole('cell')[3]
}

describe('VenueShowsTable time column', () => {
  it('prints the venue-local hour when the venue carries its own zone', () => {
    renderTable({ venueState: '', venueTimezone: 'Europe/Berlin' })
    expect(timeCell()).toHaveTextContent('5:00 AM')
  })

  it('prints the hour for a US venue the state map knows', () => {
    renderTable({ venueState: 'NY', venueTimezone: null })
    expect(timeCell()).toHaveTextContent('11:00 PM')
  })

  it('renders an empty time cell when the venue state is blank', () => {
    renderTable({ venueState: '', venueTimezone: null })
    expect(timeCell()).toBeEmptyDOMElement()
  })

  it('leaves the cell empty for a state outside the US map, with no en-dash placeholder', () => {
    renderTable({ venueState: 'England', venueTimezone: null })
    expect(timeCell()).toBeEmptyDOMElement()
    expect(timeCell().textContent).toBe('')
  })

  it('keeps the date in the same row it withheld the time from', () => {
    // The refusal is about the HOUR. Dropping the day with it would lose the
    // row's only ordering cue in a table read as a chronology.
    renderTable({ venueState: 'England', venueTimezone: null })
    const row = screen.getAllByRole('row')[1]
    expect(within(row).getAllByRole('cell')[0]).toHaveTextContent('Sep 9')
  })

  it('reads a row that carries its own state on that state, not the venue', () => {
    renderTable(
      { venueState: 'England', venueTimezone: null },
      makeShow({ state: 'NY' })
    )
    expect(timeCell()).toHaveTextContent('11:00 PM')
  })
})
