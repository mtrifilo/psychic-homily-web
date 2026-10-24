import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { stackedPlace } from '@/test/stackedTable'

// The control's own states are SaveButton's suite; here it only has to occupy
// its cell.
const saveButtonRenders: number[] = []
vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: ({ showId }: { showId: number }) => {
    saveButtonRenders.push(showId)
    return (
      <button type="button" data-show-id={showId}>
        Save
      </button>
    )
  },
}))

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

describe('VenueShowsTable with a Save column', () => {
  const zone: VenueShowZone = { venueState: 'NY', venueTimezone: null }
  const renderUpcoming = (show = makeShow({ price: 28 })) =>
    render(
      <VenueShowsTable
        shows={[show]}
        zone={zone}
        ariaLabel="Upcoming shows"
        saveColumn={{ counts: undefined }}
      />
    )

  it('declares its table roles, since its display changes below sm', () => {
    renderUpcoming()
    const table = screen.getByRole('table', { name: 'Upcoming shows' })
    expect(table).toHaveAttribute('role', 'table')
    expect(table).toHaveClass('max-sm:block')
    const head = table.querySelector('thead')!
    const body = table.querySelector('tbody')!
    expect(head).toHaveAttribute('role', 'rowgroup')
    expect(head).toHaveClass('max-sm:sr-only')
    expect(body).toHaveAttribute('role', 'rowgroup')
    expect(body).toHaveClass('max-sm:block')
    for (const row of table.querySelectorAll('tr')) {
      expect(row).toHaveAttribute('role', 'row')
    }
    for (const th of table.querySelectorAll('th')) {
      expect(th).toHaveAttribute('role', 'columnheader')
    }
    for (const td of table.querySelectorAll('td')) {
      expect(td).toHaveAttribute('role', 'cell')
    }
  })

  it('re-renders only the row whose save entry changed', () => {
    const shows = [makeShow({ id: 7 }), makeShow({ id: 8, slug: 'show-8' })]
    const zone: VenueShowZone = { venueState: 'NY', venueTimezone: null }
    const before = {
      '7': { save_count: 0, is_saved: false },
      '8': { save_count: 2, is_saved: false },
    }
    let counts: Record<string, { save_count: number; is_saved: boolean }> =
      before
    const { rerender } = render(
      <VenueShowsTable shows={shows} zone={zone} ariaLabel="Upcoming shows" saveColumn={{ counts }} />
    )
    saveButtonRenders.length = 0

    // A toggle copies the map and keeps every other show's entry object.
    counts = { ...before, '7': { save_count: 1, is_saved: true } }
    rerender(
      <VenueShowsTable shows={shows} zone={zone} ariaLabel="Upcoming shows" saveColumn={{ counts }} />
    )

    expect(saveButtonRenders).toEqual([7])
  })

  it('is a table row from sm up and a grid only below it', () => {
    renderUpcoming()
    const row = screen.getAllByRole('row')[1]
    const classes = [...row.classList]
    expect(classes).toContain('max-sm:grid')
    expect(classes.every(name => name.startsWith('max-sm:'))).toBe(true)
  })

  it('stacks the date, price, time and Save, then the bill', () => {
    renderUpcoming()
    const row = screen.getAllByRole('row')[1]
    const [date, bill, price, time, save] = within(row).getAllByRole('cell')
    expect(stackedPlace(date)).toBe('line 1, column 1')
    // The one shrinkable track: the date wraps instead of overflowing.
    expect(date).toHaveClass('max-sm:whitespace-normal')
    expect(stackedPlace(price)).toBe('line 1, column 2')
    expect(stackedPlace(time)).toBe('line 1, column 3')
    expect(stackedPlace(save)).toBe('line 1, column 4')
    expect(within(save).getByRole('button')).toHaveTextContent('Save')
    expect(stackedPlace(bill)).toBe('line 2, full width')
    // Price and time share the line; the dot between them is visual only.
    const dot = time.firstElementChild!
    expect(dot).toHaveTextContent('·')
    expect(dot).toHaveAttribute('aria-hidden', 'true')
    expect(dot).toHaveClass('sm:hidden')
    expect(time).toHaveTextContent('11:00 PM')
  })

  it('prints no dot before a time it withholds', () => {
    render(
      <VenueShowsTable
        shows={[makeShow()]}
        zone={{ venueState: 'England', venueTimezone: null }}
        ariaLabel="Upcoming shows"
        saveColumn={{ counts: undefined }}
      />
    )
    expect(timeCell()).toBeEmptyDOMElement()
  })
})

describe('VenueShowsTable without a Save column', () => {
  it('renders the plain table at every width', () => {
    render(
      <VenueShowsTable
        shows={[makeShow(), makeShow({ id: 2, slug: 'show-2' })]}
        zone={{ venueState: 'NY', venueTimezone: null }}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
    )
    const table = screen.getByRole('table', { name: 'Past shows' })
    expect(table.outerHTML).not.toMatch(/max-sm:|role=|class=""/)
    expect(within(table).getAllByRole('columnheader')).toHaveLength(4)
    expect(within(table).queryByRole('button')).not.toBeInTheDocument()
  })
})
