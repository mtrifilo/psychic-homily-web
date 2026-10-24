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

import { ArtistShowsTable } from './ArtistShowsTable'
import { ENTITY_LINK_CLASS } from '@/components/shared/entityLink'
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

// The bill column is a dense table, so its names take the restrained tier:
// colour and weight from the surrounding text, accent and underline on hover.
describe('ArtistShowsTable bill links', () => {
  it('gives headliner and support names the restrained entity-link treatment', () => {
    render(
      <ArtistShowsTable
        shows={[
          {
            ...makeShow(1, makeVenue()),
            artists: [
              { id: 10, slug: 'lead', name: 'Lead Act' },
              { id: 11, slug: 'opener', name: 'Opening Act' },
            ],
          },
        ]}
        ariaLabel="Shows"
      />
    )
    for (const name of ['Lead Act', 'Opening Act']) {
      const link = screen.getByRole('link', { name })
      expect(link.className).toBe(ENTITY_LINK_CLASS.restrained)
    }
    // The headliner keeps its weight from the bill, the support its muted run.
    expect(
      screen.getByRole('link', { name: 'Lead Act' }).closest('.font-medium')
    ).not.toBeNull()
    expect(
      screen.getByRole('link', { name: 'Opening Act' }).closest('.font-medium')
    ).toBeNull()
  })
})

describe('ArtistShowsTable with a Save column', () => {
  const show = {
    ...makeShow(1, makeVenue({ timezone: 'America/Phoenix' })),
    artists: [{ id: 10, slug: 'lead', name: 'Lead Act' }],
  }
  const renderUpcoming = () =>
    render(
      <ArtistShowsTable
        shows={[show]}
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
    // Hidden from sight on a phone, kept for assistive tech.
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
    const shows = [makeShow(7, makeVenue()), makeShow(8, makeVenue())]
    const before = {
      '7': { save_count: 0, is_saved: false },
      '8': { save_count: 2, is_saved: false },
    }
    let counts: Record<string, { save_count: number; is_saved: boolean }> =
      before
    const { rerender } = render(
      <ArtistShowsTable shows={shows} ariaLabel="Upcoming shows" saveColumn={{ counts }} />
    )
    saveButtonRenders.length = 0

    // A toggle copies the map and keeps every other show's entry object.
    counts = { ...before, '7': { save_count: 1, is_saved: true } }
    rerender(
      <ArtistShowsTable shows={shows} ariaLabel="Upcoming shows" saveColumn={{ counts }} />
    )

    expect(saveButtonRenders).toEqual([7])
  })

  it('is a table row from sm up and a grid only below it', () => {
    renderUpcoming()
    const row = screen.getAllByRole('row')[1]
    const classes = [...row.classList]
    expect(classes).toContain('max-sm:grid')
    // Every layout class on the row is scoped below `sm`, so from `sm` up the
    // row keeps the table's own display.
    expect(classes.every(name => name.startsWith('max-sm:'))).toBe(true)
  })

  it('stacks the date, time and Save, then the bill, then the place', () => {
    renderUpcoming()
    const row = screen.getAllByRole('row')[1]
    const [date, bill, price, time, save] = within(row).getAllByRole('cell')
    expect(stackedPlace(date)).toBe('line 1, column 1')
    expect(stackedPlace(time)).toBe('line 1, column 2')
    expect(stackedPlace(save)).toBe('line 1, column 3')
    expect(within(save).getByRole('button')).toHaveTextContent('Save')
    expect(stackedPlace(bill)).toBe('line 2, full width')
    expect(stackedPlace(price)).toBe('no line')
    // The place leaves the bill's run and ends the row on its own line.
    const place = within(bill)
      .getByRole('link', { name: 'The Rebel Lounge' })
      .closest('.text-muted-foreground')!
    expect(place).toHaveClass('max-sm:order-last', 'max-sm:basis-full')
    expect(place.firstElementChild).toHaveClass('max-sm:hidden')
    expect(place.firstElementChild).toHaveTextContent('·')
  })
  it('puts "Venue TBA" on its own last line too', () => {
    render(
      <ArtistShowsTable
        shows={[{ ...makeShow(1, null), artists: show.artists }]}
        ariaLabel="Upcoming shows"
        saveColumn={{ counts: undefined }}
      />
    )
    const place = screen.getByText(/Venue TBA/)
    expect(place).toHaveClass('max-sm:order-last', 'max-sm:basis-full')
    expect(place.firstElementChild).toHaveClass('max-sm:hidden')
  })

  it('moves the place after a status badge in the same flex row', () => {
    render(
      <ArtistShowsTable
        shows={[{ ...show, is_cancelled: true }]}
        ariaLabel="Upcoming shows"
        saveColumn={{ counts: undefined }}
      />
    )
    const place = screen
      .getByRole('link', { name: 'The Rebel Lounge' })
      .closest('.text-muted-foreground')!
    const badge = screen.getByText('CANCELLED')
    // Same flex row, so `order-last` puts the place after the badge.
    expect(place.parentElement).toBe(badge.parentElement)
    expect(place).toHaveClass('max-sm:order-last')
    expect([...badge.classList].some(name => /(^|:)order-/.test(name))).toBe(
      false
    )
  })
})

describe('ArtistShowsTable without a Save column', () => {
  it('renders the plain table at every width', () => {
    render(
      <ArtistShowsTable
        shows={[makeShow(1, makeVenue()), makeShow(2, null)]}
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
