import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { DayGroupedShowRow } from './DayGroupedShowRow'
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
      { id: 1, name: 'Sunn Amps', is_headliner: true },
      { id: 2, name: 'Low Ceiling', is_headliner: false },
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
      isAdmin={false}
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

    expect(screen.getByText(/w\/ Low Ceiling/)).toBeInTheDocument()
  })

  it('links the venue', () => {
    renderRow()

    expect(
      screen.getByRole('link', { name: 'Valley Bar' })
    ).toHaveAttribute('href', '/venues/valley-bar')
  })

  // A single-metro list would repeat one city on every row: the column's whole
  // width spent on no information.
  it('omits the city until the list spans more than one', () => {
    const { unmount } = renderRow()
    expect(screen.queryAllByText(/· Phoenix/)).toHaveLength(0)
    unmount()

    renderRow({}, { showCity: true })
    expect(screen.getByText(/Phoenix/)).toBeInTheDocument()
  })

  it('renders the price and the age requirement', () => {
    renderRow()

    expect(screen.getByText('$15')).toBeInTheDocument()
    expect(screen.getByText('21+')).toBeInTheDocument()
  })

  // A blank cell reads as "nobody filled this in"; the dash reads as "there is
  // no value", which is what the row knows.
  it('holds the price column open with a dash when no price is recorded', () => {
    renderRow({ price: null, door_price: null } as never)

    expect(screen.getByText('–')).toBeInTheDocument()
  })

  it('carries the sold-out and cancelled badges', () => {
    const { unmount } = renderRow({ is_sold_out: true })
    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
    unmount()

    renderRow({ is_cancelled: true })
    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
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

      expect(screen.queryByText(/w\/ Low Ceiling/)).toBeNull()
      expect(screen.queryByText('21+')).toBeNull()
    })

    it('keeps support and age in comfortable', () => {
      renderRow({}, { density: 'comfortable' })

      expect(screen.getByText(/w\/ Low Ceiling/)).toBeInTheDocument()
      expect(screen.getByText('21+')).toBeInTheDocument()
    })

    // Expanded gives support its own line under the headliner.
    it('moves support onto its own line in expanded', () => {
      renderRow({}, { density: 'expanded' })

      const bill = screen.getByText(/w\/ Low Ceiling/)
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
