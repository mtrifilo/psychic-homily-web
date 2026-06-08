import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RadioStationList } from './RadioStationList'
import type { RadioStationListItem } from '../types'

function makeStation(overrides: Partial<RadioStationListItem> = {}): RadioStationListItem {
  return {
    id: 1,
    name: 'KEXP',
    slug: 'kexp',
    city: 'Seattle',
    state: 'WA',
    country: 'USA',
    broadcast_type: 'both',
    frequency_mhz: 90.3,
    logo_url: null,
    is_active: true,
    network: null,
    sibling_stations: [],
    show_count: 5,
    ...overrides,
  }
}

const stations = [
  makeStation({ id: 1, name: 'KEXP', slug: 'kexp', city: 'Seattle', state: 'WA' }),
  makeStation({ id: 2, name: 'WFMU', slug: 'wfmu', city: 'Jersey City', state: 'NJ' }),
  makeStation({ id: 3, name: 'NTS', slug: 'nts', city: 'London', state: null, country: 'UK' }),
]

describe('RadioStationList', () => {
  it('renders each station name and its location', () => {
    render(<RadioStationList stations={stations} selectedSlug="kexp" onSelect={() => {}} />)
    expect(screen.getByText('KEXP')).toBeInTheDocument()
    expect(screen.getByText('Seattle, WA')).toBeInTheDocument()
    expect(screen.getByText('WFMU')).toBeInTheDocument()
    expect(screen.getByText('Jersey City, NJ')).toBeInTheDocument()
    expect(screen.getByText('London')).toBeInTheDocument()
  })

  it('exposes stations as a labeled group of buttons with the selected one pressed', () => {
    render(<RadioStationList stations={stations} selectedSlug="wfmu" onSelect={() => {}} />)
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(3)
    expect(screen.getByRole('button', { name: /WFMU/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: /KEXP/ })).toHaveAttribute('aria-pressed', 'false')
  })

  it('calls onSelect with the station slug when a station is clicked', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<RadioStationList stations={stations} selectedSlug="kexp" onSelect={onSelect} />)
    await user.click(screen.getByRole('button', { name: /NTS/ }))
    expect(onSelect).toHaveBeenCalledWith('nts')
  })

  it('is keyboard-operable (Enter selects a focused station)', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<RadioStationList stations={stations} selectedSlug="kexp" onSelect={onSelect} />)
    const wfmu = screen.getByRole('button', { name: /WFMU/ })
    wfmu.focus()
    await user.keyboard('{Enter}')
    expect(onSelect).toHaveBeenCalledWith('wfmu')
  })
})
