import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import {
  DayGroupedShowListHeader,
  DayGroupedShowRow,
} from './DayGroupedShowRow'
import type { ShowResponse } from '../types'

vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: () => (
    <button type="button" aria-label="Add to My List">
      save
    </button>
  ),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'desert-doom',
    title: 'Desert Doom at Valley Bar',
    event_date: '2026-09-12T02:00:00Z',
    status: 'approved',
    city: 'Phoenix',
    state: 'AZ',
    price: 15,
    age_requirement: '21+',
    venues: [
      { id: 7, name: 'Valley Bar', slug: 'valley-bar', timezone: 'America/Phoenix' },
    ] as never,
    artists: [
      { id: 1, name: 'Sunn Amps', slug: 'sunn-amps', is_headliner: true },
      { id: 2, name: 'Low Ceiling', slug: 'low-ceiling', is_headliner: false },
    ] as never,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  } as ShowResponse
}

function renderRow(overrides: Partial<ShowResponse> = {}, props = {}) {
  return render(
    <DayGroupedShowRow
      show={makeShow(overrides)}
      density="comfortable"
      index={0}
      showCity={false}
      {...props}
    />
  )
}

describe('DayGroupedShowRow', () => {
  // The E2E save and list-action specs address a specific seeded show with
  // `getByRole('article', { name })`, so the accessible name is a contract and
  // not decoration.
  it('exposes the show title as the row s accessible name', () => {
    renderRow()

    expect(
      screen.getByRole('article', { name: 'Desert Doom at Valley Bar' })
    ).toBeInTheDocument()
  })

  it('leads the bill with the headliner, linked to the show', () => {
    renderRow()

    const row = screen.getByRole('article')
    expect(
      within(row).getByRole('link', { name: 'Sunn Amps' })
    ).toHaveAttribute('href', '/shows/desert-doom')
  })

  it('carries the support billing beside the headliner', () => {
    renderRow()

    expect(screen.getByTestId('row-support')).toHaveTextContent('w/ Low Ceiling')
  })

  it('links the venue', () => {
    renderRow()

    expect(
      screen.getByRole('link', { name: 'Valley Bar' })
    ).toHaveAttribute('href', '/venues/valley-bar')
  })

  // A single-metro list would repeat one city on every row: the column's whole
  // width spent on no information. Asserted on the CITY TEXT rather than on the
  // separator, so changing the separator cannot turn this green while the city
  // renders on every row.
  it('omits the city until the list spans more than one', () => {
    const { unmount } = renderRow()
    expect(screen.queryByText(/Phoenix/)).toBeNull()
    unmount()

    renderRow({}, { showCity: true })
    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
  })

  // Two metros can share a name across state lines, so the city alone names
  // nothing on a list that spans them.
  it('names the state alongside the city', () => {
    renderRow({ city: 'Kansas City', state: 'KS' }, { showCity: true })

    expect(screen.getByText(/Kansas City, KS/)).toBeInTheDocument()
  })

  // The venue column truncates, which hides the tail with no other way to read
  // it.
  it('titles the venue cell with its full text', () => {
    renderRow({}, { showCity: true })

    expect(
      screen.getByTitle('Valley Bar, Phoenix, AZ')
    ).toBeInTheDocument()
  })

  // The site's whole shape is Shows to Artists to Releases. Every billed act
  // that has a page is reachable from the row.
  it('links every billed act to its artist page', () => {
    renderRow()

    expect(screen.getByRole('link', { name: 'Low Ceiling' })).toHaveAttribute(
      'href',
      '/artists/low-ceiling'
    )
  })

  it('renders an act with no page as plain text', () => {
    renderRow({
      artists: [
        { id: 1, name: 'Sunn Amps', slug: 'sunn-amps', is_headliner: true },
        { id: 2, name: 'No Page', is_headliner: false },
      ] as never,
    })

    expect(screen.queryByRole('link', { name: 'No Page' })).toBeNull()
    expect(screen.getByTestId('row-support')).toHaveTextContent('No Page')
  })

  it('renders the price and the age requirement', () => {
    renderRow()

    expect(screen.getByText('$15')).toBeInTheDocument()
    expect(screen.getByText('21+')).toBeInTheDocument()
  })

  // A blank cell reads as "nobody filled this in"; the dash reads as "there is
  // no value", which is what the row knows.
  // The dash is `aria-hidden`: a column that looks continuous to a sighted
  // reader and is simply absent to a screen reader, rather than an unlabelled
  // "en dash" announced between the bill and the price.
  it('holds the price column open with a dash hidden from assistive tech', () => {
    renderRow({ price: null, door_price: null } as never)

    const dash = screen.getByText('–')
    expect(dash).toHaveAttribute('aria-hidden', 'true')
  })

  it('carries the sold-out badge', () => {
    renderRow({ is_sold_out: true })

    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  // PRIMARY on this surface, not the card's destructive red: among muted mono
  // columns a red block reads as an error state rather than as a status.
  it('paints the cancelled badge in primary', () => {
    renderRow({ is_cancelled: true })

    const badge = screen.getByText('CANCELLED')
    expect(badge.className).toContain('bg-primary')
    expect(badge.className).not.toContain('bg-destructive')
  })

  it('carries the save and outbound controls', () => {
    renderRow()

    const row = screen.getByRole('article')
    expect(
      within(row).getByRole('button', { name: 'Add to My List' })
    ).toBeInTheDocument()
    expect(
      within(row).getByRole('link', { name: 'View show details' })
    ).toHaveAttribute('href', '/shows/desert-doom')
  })

  // The row carries no date of its own: the day heading above it states the
  // day, and a date column would print the same value on every row of a group.
  it('prints no date', () => {
    renderRow()

    expect(screen.queryByText(/SEP 11|SEP 12/)).toBeNull()
  })

  describe('the alternating fill', () => {
    it('fills even rows and leaves odd ones bare', () => {
      const { container, unmount } = renderRow({}, { index: 0 })
      expect(container.querySelector('.bg-muted\\/20')).not.toBeNull()
      unmount()

      const second = renderRow({}, { index: 1 })
      expect(second.container.querySelector('.bg-muted\\/20')).toBeNull()
    })
  })

  describe('density', () => {
    // The frame collapses support and age first.
    it('drops support and age in compact', () => {
      renderRow({}, { density: 'compact' })

      expect(screen.queryByTestId('row-support')).toBeNull()
      expect(screen.queryByText('21+')).toBeNull()
    })

    it('keeps support and age in comfortable', () => {
      renderRow({}, { density: 'comfortable' })

      expect(screen.getByTestId('row-support')).toHaveTextContent(
        'w/ Low Ceiling'
      )
      expect(screen.getByText('21+')).toBeInTheDocument()
    })

    // Expanded gives support its own line under the headliner.
    it('moves support onto its own line in expanded', () => {
      renderRow({}, { density: 'expanded' })

      const bill = screen.getByTestId('row-support')
      const headliner = screen.getByRole('link', { name: 'Sunn Amps' })
      expect(bill.parentElement).not.toBe(headliner.parentElement)
    })

    it('keeps every column in all three densities', () => {
      for (const density of ['compact', 'comfortable', 'expanded'] as const) {
        const { unmount } = renderRow({}, { density })
        const row = screen.getByRole('article')
        expect(
          within(row).getByRole('link', { name: 'Sunn Amps' })
        ).toBeInTheDocument()
        expect(
          within(row).getByRole('link', { name: 'Valley Bar' })
        ).toBeInTheDocument()
        expect(within(row).getByText('$15')).toBeInTheDocument()
        unmount()
      }
    })
  })
})

describe('DayGroupedShowListHeader', () => {
  it('labels every column the row renders', () => {
    render(<DayGroupedShowListHeader density="comfortable" />)

    for (const label of ['Time', 'Bill', 'Venue', 'Price', 'Age']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  // Compact renders no age, and a label over fifty empty cells is a column that
  // is not there.
  it('drops the Age label in compact, where the column is empty', () => {
    render(<DayGroupedShowListHeader density="compact" />)

    expect(screen.queryByText('Age')).toBeNull()
    expect(screen.getByText('Time')).toBeInTheDocument()
  })

  // These rows are articles, not a table: with no header-to-cell association
  // the labels would arrive as five orphan words before the list.
  it('is hidden from assistive tech', () => {
    render(<DayGroupedShowListHeader density="comfortable" />)

    expect(screen.getByTestId('show-list-header')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })
})
