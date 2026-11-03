import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NearbyCityControl } from './NearbyCityControl'
import type { HomeShowCitySelection } from '@/features/shows/hooks/useHomeShowCitySelection'

const phoenix = { city: 'Phoenix', state: 'AZ' }
const mesa = { city: 'Mesa', state: 'AZ' }
const flagstaff = { city: 'Flagstaff', state: 'AZ' }

function selection(
  overrides: Partial<HomeShowCitySelection> = {}
): HomeShowCitySelection {
  return {
    cities: [
      { ...mesa, count: 4 },
      { ...phoenix, count: 30 },
    ],
    favoriteCities: [],
    effectiveCities: [phoenix],
    source: 'favorites',
    isResolving: false,
    geoAffordanceCity: null,
    selectionDiffersFromFavorites: false,
    onFilterChange: vi.fn(),
    ...overrides,
  }
}

describe('NearbyCityControl', () => {
  it('names the selected city and opens the picker on it', async () => {
    const user = userEvent.setup()
    render(<NearbyCityControl selection={selection()} />)

    await user.click(
      screen.getByRole('button', { name: 'Change city, now Phoenix, AZ' })
    )

    expect(screen.getByPlaceholderText('Search cities...')).toBeInTheDocument()
    // Busiest first, as the filter bar's picker orders them.
    const options = screen.getAllByTestId(/^city-option-/)
    expect(options.map(o => o.dataset.testid)).toEqual([
      'city-option-phoenix-az',
      'city-option-mesa-az',
    ])
  })

  // Phone-only control: a focused search field would raise the soft keyboard
  // over the list on every open.
  it('opens with focus on the panel, not the search field', async () => {
    const user = userEvent.setup()
    render(<NearbyCityControl selection={selection()} />)

    await user.click(screen.getByRole('button', { name: /Change city/ }))

    const search = screen.getByPlaceholderText('Search cities...')
    expect(document.activeElement).not.toBe(search)
    expect(document.activeElement?.contains(search)).toBe(true)
  })

  it('toggles a city into the selection the rows read', async () => {
    const user = userEvent.setup()
    const current = selection()
    render(<NearbyCityControl selection={current} />)

    await user.click(screen.getByRole('button', { name: /Change city/ }))
    await user.click(screen.getByTestId('city-option-mesa-az'))

    expect(current.onFilterChange).toHaveBeenCalledWith([phoenix, mesa])
  })

  it('lists a selected city the count list dropped, so it can be unticked', async () => {
    const user = userEvent.setup()
    const current = selection({ effectiveCities: [flagstaff] })
    render(<NearbyCityControl selection={current} />)

    await user.click(screen.getByRole('button', { name: /Change city/ }))
    await user.click(screen.getByTestId('city-option-flagstaff-az'))

    expect(current.onFilterChange).toHaveBeenCalledWith([])
  })

  it('reads All Cities when the viewer cleared the selection', () => {
    render(
      <NearbyCityControl
        selection={selection({ effectiveCities: [], source: 'user' })}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Change city, now All Cities' })
    ).toBeInTheDocument()
  })

  it('renders nothing with no cities', () => {
    const { container } = render(
      <NearbyCityControl selection={selection({ cities: [] })} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('joins several selected cities in its label', () => {
    render(
      <NearbyCityControl
        selection={selection({ effectiveCities: [phoenix, mesa] })}
      />
    )

    expect(
      screen.getByRole('button', {
        name: 'Change city, now Phoenix, AZ · Mesa, AZ',
      })
    ).toBeInTheDocument()
  })
})
