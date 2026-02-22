import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CityFilters, type CityWithCount, type CityState } from './CityFilters'

const cities: CityWithCount[] = [
  { city: 'Phoenix', state: 'AZ', count: 8 },
  { city: 'Mesa', state: 'AZ', count: 3 },
  { city: 'Tempe', state: 'AZ', count: 2 },
]

describe('CityFilters', () => {
  it('renders "All Cities" chip and city chips with counts', () => {
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} />
    )

    expect(screen.getByText('All Cities')).toBeInTheDocument()
    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
    expect(screen.getByText('(8)')).toBeInTheDocument()
    expect(screen.getByText(/Mesa, AZ/)).toBeInTheDocument()
    expect(screen.getByText(/Tempe, AZ/)).toBeInTheDocument()
  })

  it('uses custom allLabel', () => {
    render(
      <CityFilters
        cities={cities}
        selectedCities={[]}
        onFilterChange={vi.fn()}
        allLabel="All Venues"
      />
    )

    expect(screen.getByText('All Venues')).toBeInTheDocument()
  })

  it('clicking "All Cities" calls onFilterChange with empty array', async () => {
    const onChange = vi.fn()
    render(
      <CityFilters
        cities={cities}
        selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
        onFilterChange={onChange}
      />
    )

    await userEvent.click(screen.getByText('All Cities'))
    expect(onChange).toHaveBeenCalledWith([])
  })

  describe('single click (no shift)', () => {
    it('selects only the clicked city', async () => {
      const onChange = vi.fn()
      render(
        <CityFilters cities={cities} selectedCities={[]} onFilterChange={onChange} />
      )

      await userEvent.click(screen.getByText(/Phoenix, AZ/))
      expect(onChange).toHaveBeenCalledWith([{ city: 'Phoenix', state: 'AZ' }])
    })

    it('replaces current selection with clicked city', async () => {
      const onChange = vi.fn()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          onFilterChange={onChange}
        />
      )

      await userEvent.click(screen.getByText(/Mesa, AZ/))
      expect(onChange).toHaveBeenCalledWith([{ city: 'Mesa', state: 'AZ' }])
    })

    it('deselects to "All Cities" when clicking the sole selected city', async () => {
      const onChange = vi.fn()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          onFilterChange={onChange}
        />
      )

      await userEvent.click(screen.getByText(/Phoenix, AZ/))
      expect(onChange).toHaveBeenCalledWith([])
    })

    it('single-selects when clicking one of multiple selected cities', async () => {
      const onChange = vi.fn()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Mesa', state: 'AZ' },
          ]}
          onFilterChange={onChange}
        />
      )

      await userEvent.click(screen.getByText(/Phoenix, AZ/))
      expect(onChange).toHaveBeenCalledWith([{ city: 'Phoenix', state: 'AZ' }])
    })
  })

  describe('shift+click (multi-select)', () => {
    it('adds a city to the selection', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          onFilterChange={onChange}
        />
      )

      await user.keyboard('{Shift>}')
      await user.click(screen.getByText(/Mesa, AZ/))
      await user.keyboard('{/Shift}')

      expect(onChange).toHaveBeenCalledWith([
        { city: 'Phoenix', state: 'AZ' },
        { city: 'Mesa', state: 'AZ' },
      ])
    })

    it('removes a city from the selection', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Mesa', state: 'AZ' },
          ]}
          onFilterChange={onChange}
        />
      )

      await user.keyboard('{Shift>}')
      await user.click(screen.getByText(/Mesa, AZ/))
      await user.keyboard('{/Shift}')

      expect(onChange).toHaveBeenCalledWith([{ city: 'Phoenix', state: 'AZ' }])
    })

    it('adds a city when nothing is selected', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <CityFilters cities={cities} selectedCities={[]} onFilterChange={onChange} />
      )

      await user.keyboard('{Shift>}')
      await user.click(screen.getByText(/Tempe, AZ/))
      await user.keyboard('{/Shift}')

      expect(onChange).toHaveBeenCalledWith([{ city: 'Tempe', state: 'AZ' }])
    })
  })

  it('renders children', () => {
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()}>
        <span data-testid="child">Extra</span>
      </CityFilters>
    )

    expect(screen.getByTestId('child')).toBeInTheDocument()
  })
})
