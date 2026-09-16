import { describe, it, expect, vi, beforeAll, afterEach } from 'vitest'
import { useRef } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  CityFilters,
  type CityFiltersControl,
  type CityWithCount,
} from './CityFilters'
import { SOFT_KEYBOARD_VIEWPORT_QUERY } from '@/lib/hooks/common/useSoftKeyboardViewport'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'

// jsdom does not implement scrollIntoView (required by cmdk)
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

const cities: CityWithCount[] = [
  { city: 'Phoenix', state: 'AZ', count: 8 },
  { city: 'Mesa', state: 'AZ', count: 3 },
  { city: 'Tempe', state: 'AZ', count: 2 },
]

// Large city list for popular cities tests
const manyCities: CityWithCount[] = [
  { city: 'Phoenix', state: 'AZ', count: 15 },
  { city: 'Denver', state: 'CO', count: 5 },
  { city: 'Chicago', state: 'IL', count: 4 },
  { city: 'Mesa', state: 'AZ', count: 3 },
  { city: 'Tempe', state: 'AZ', count: 2 },
  { city: 'Flagstaff', state: 'AZ', count: 1 },
]

describe('CityFilters', () => {
  it('renders the combobox trigger', () => {
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    expect(screen.getByTestId('city-filter-combobox')).toBeInTheDocument()
    expect(screen.getByText('Filter by city...')).toBeInTheDocument()
  })

  // The trigger reaches server HTML on /shows and /venues, so it is clickable
  // for the whole pre-hydration window and a click there is dropped unless it
  // is a replay root. `replayOnHydrate` has two halves and both are needed: the
  // marker attribute below, and the ref that consumes the buffered click. The
  // component composes that ref with one of its own, so each half is pinned
  // separately - either one lost alone leaves a control that looks adopted.
  it('marks the combobox trigger as a click-replay root', () => {
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    expect(screen.getByTestId('city-filter-combobox')).toHaveAttribute(
      'data-replay-on-hydrate'
    )
  })

  it('hands the trigger node to the click-replay ref', () => {
    const replayRef = vi.spyOn(
      replayOnHydrate as { ref: (node: HTMLElement | null) => void },
      'ref'
    )

    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    expect(replayRef).toHaveBeenCalledWith(
      screen.getByTestId('city-filter-combobox')
    )
    replayRef.mockRestore()
  })

  it('opens the dropdown when combobox is clicked', async () => {
    const user = userEvent.setup()
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))

    // City options should be visible in the dropdown
    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    expect(screen.getByText('Mesa, AZ')).toBeInTheDocument()
    expect(screen.getByText('Tempe, AZ')).toBeInTheDocument()
  })

  it('shows cities sorted by count descending in dropdown', async () => {
    const user = userEvent.setup()
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))

    const items = screen.getAllByRole('option')
    expect(items[0]).toHaveTextContent('Phoenix, AZ')
    expect(items[0]).toHaveTextContent('(8)')
    expect(items[1]).toHaveTextContent('Mesa, AZ')
    expect(items[1]).toHaveTextContent('(3)')
    expect(items[2]).toHaveTextContent('Tempe, AZ')
    expect(items[2]).toHaveTextContent('(2)')
  })

  it('selecting a city from dropdown calls onFilterChange', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={onChange} resultNoun={{ singular: 'venue', plural: 'venues' }} />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))
    await user.click(screen.getByText('Phoenix, AZ'))

    expect(onChange).toHaveBeenCalledWith([{ city: 'Phoenix', state: 'AZ' }])
  })

  it('selecting an already-selected city removes it', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <CityFilters
        cities={cities}
        selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
        onFilterChange={onChange}
        resultNoun={{ singular: 'venue', plural: 'venues' }}
      />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))
    // Use testid to target the dropdown option (not the chip)
    await user.click(screen.getByTestId('city-option-phoenix-az'))

    expect(onChange).toHaveBeenCalledWith([])
  })

  it('multi-select: adds a second city', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <CityFilters
        cities={cities}
        selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
        onFilterChange={onChange}
        resultNoun={{ singular: 'venue', plural: 'venues' }}
      />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))
    await user.click(screen.getByText('Mesa, AZ'))

    expect(onChange).toHaveBeenCalledWith([
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Mesa', state: 'AZ' },
    ])
  })

  describe('active filter chips', () => {
    it('shows dismissible chips for selected cities', () => {
      render(
        <CityFilters
          cities={cities}
          selectedCities={[
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Mesa', state: 'AZ' },
          ]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      expect(screen.getByTestId('city-chip-phoenix-az')).toBeInTheDocument()
      expect(screen.getByTestId('city-chip-mesa-az')).toBeInTheDocument()
    })

    it('removes a city when chip dismiss button is clicked', async () => {
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
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      await user.click(screen.getByTestId('city-chip-remove-phoenix-az'))

      expect(onChange).toHaveBeenCalledWith([{ city: 'Mesa', state: 'AZ' }])
    })
  })

  describe('clear all / all cities button', () => {
    it('shows "All Cities" when one city is selected', () => {
      render(
        <CityFilters
          cities={cities}
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      expect(screen.getByTestId('city-filter-all')).toHaveTextContent('All Cities')
    })

    it('shows "Clear all" when 2+ cities are selected', () => {
      render(
        <CityFilters
          cities={cities}
          selectedCities={[
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Mesa', state: 'AZ' },
          ]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      expect(screen.getByTestId('city-filter-all')).toHaveTextContent('Clear all')
    })

    it('clears all filters when clicked', async () => {
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
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      await user.click(screen.getByTestId('city-filter-all'))
      expect(onChange).toHaveBeenCalledWith([])
    })

    it('does not show clear button when no cities are selected', () => {
      render(
        <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
      )

      expect(screen.queryByTestId('city-filter-all')).not.toBeInTheDocument()
    })
  })

  describe('popular cities row', () => {
    it('shows popular cities when none are selected', () => {
      render(
        <CityFilters cities={manyCities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
      )

      expect(screen.getByTestId('popular-cities')).toBeInTheDocument()
      expect(screen.getByText('Popular:')).toBeInTheDocument()
      // Top 5 with count >= 2
      expect(screen.getByTestId('popular-city-phoenix-az')).toBeInTheDocument()
      expect(screen.getByTestId('popular-city-denver-co')).toBeInTheDocument()
      expect(screen.getByTestId('popular-city-chicago-il')).toBeInTheDocument()
      expect(screen.getByTestId('popular-city-mesa-az')).toBeInTheDocument()
      expect(screen.getByTestId('popular-city-tempe-az')).toBeInTheDocument()
    })

    it('hides popular cities row when cities are selected', () => {
      render(
        <CityFilters
          cities={manyCities}
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      expect(screen.queryByTestId('popular-cities')).not.toBeInTheDocument()
    })

    it('clicking a popular city selects it', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <CityFilters cities={manyCities} selectedCities={[]} onFilterChange={onChange} resultNoun={{ singular: 'venue', plural: 'venues' }} />
      )

      await user.click(screen.getByTestId('popular-city-denver-co'))
      expect(onChange).toHaveBeenCalledWith([{ city: 'Denver', state: 'CO' }])
    })

    it('does not show popular cities when fewer than 3 cities have 2+ count', () => {
      const fewCities: CityWithCount[] = [
        { city: 'Phoenix', state: 'AZ', count: 5 },
        { city: 'Mesa', state: 'AZ', count: 2 },
        { city: 'Tempe', state: 'AZ', count: 1 },
      ]

      render(
        <CityFilters cities={fewCities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }} />
      )

      expect(screen.queryByTestId('popular-cities')).not.toBeInTheDocument()
    })

    it('renders no row at all when the surface opts out', () => {
      render(
        <CityFilters
          cities={manyCities}
          selectedCities={[]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
          showPopularCities={false}
        />
      )

      expect(screen.queryByTestId('popular-cities')).not.toBeInTheDocument()
    })
  })

  describe('imperative control', () => {
    it('opens the picker when a control elsewhere on the page asks it to', async () => {
      const user = userEvent.setup()

      function Harness() {
        const ref = useRef<CityFiltersControl | null>(null)
        return (
          <>
            <button data-testid="outside-change" onClick={() => ref.current?.open()}>
              change
            </button>
            <CityFilters
              cities={cities}
              selectedCities={[]}
              onFilterChange={vi.fn()}
              resultNoun={{ singular: 'venue', plural: 'venues' }}
              controlRef={ref}
            />
          </>
        )
      }

      render(<Harness />)
      expect(screen.queryByPlaceholderText('Search cities...')).toBeNull()

      await user.click(screen.getByTestId('outside-change'))

      expect(screen.getByPlaceholderText('Search cities...')).toBeInTheDocument()
      expect(screen.getByTestId('city-filter-combobox')).toHaveAttribute(
        'aria-expanded',
        'true'
      )
    })
  })

  it('shows checkmark for selected cities in dropdown', async () => {
    const user = userEvent.setup()
    render(
      <CityFilters
        cities={cities}
        selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
        onFilterChange={vi.fn()}
        resultNoun={{ singular: 'venue', plural: 'venues' }}
      />
    )

    await user.click(screen.getByTestId('city-filter-combobox'))

    // The Phoenix option should have a visible checkmark
    const phoenixOption = screen.getByTestId('city-option-phoenix-az')
    const checkIcon = phoenixOption.querySelector('svg')
    expect(checkIcon).not.toHaveClass('opacity-0')

    // Mesa should have an invisible checkmark
    const mesaOption = screen.getByTestId('city-option-mesa-az')
    const mesaCheckIcon = mesaOption.querySelector('svg')
    expect(mesaCheckIcon).toHaveClass('opacity-0')
  })

  it('renders children', () => {
    render(
      <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} resultNoun={{ singular: 'venue', plural: 'venues' }}>
        <span data-testid="child">Extra</span>
      </CityFilters>
    )

    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('uses custom allLabel', () => {
    render(
      <CityFilters
        cities={cities}
        selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
        onFilterChange={vi.fn()}
        resultNoun={{ singular: 'venue', plural: 'venues' }}
        allLabel="All Venues"
      />
    )

    expect(screen.getByTestId('city-filter-all')).toHaveTextContent('All Venues')
  })

  describe('overlay choice', () => {
    // jsdom has no layout: it can show WHICH overlay a viewport gets and what
    // contract the trigger advertises, and nothing about the geometry that
    // results. The sheet's own behaviour is CityFilterSheet.test.tsx and its
    // geometry is e2e/pages/city-filter-mobile.spec.ts.
    const originalMatchMedia = window.matchMedia

    function mockSoftKeyboardViewport(matches: boolean) {
      window.matchMedia = vi.fn(
        (query: string) =>
          ({
            matches: matches && query === SOFT_KEYBOARD_VIEWPORT_QUERY,
            media: query,
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
          }) as unknown as MediaQueryList
      )
    }

    afterEach(() => {
      window.matchMedia = originalMatchMedia
    })

    it('opens the bottom sheet on a soft-keyboard viewport', async () => {
      mockSoftKeyboardViewport(true)
      const user = userEvent.setup()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      const trigger = screen.getByTestId('city-filter-combobox')
      expect(trigger).toHaveAttribute('aria-haspopup', 'dialog')
      expect(trigger).not.toHaveAttribute('role', 'combobox')
      // The popover content is never mounted here, so the trigger must not
      // still be advertising the id Radix's PopoverTrigger hands it.
      expect(trigger).not.toHaveAttribute('aria-controls')

      await user.click(trigger)

      expect(screen.getByTestId('city-filter-sheet')).toBeInTheDocument()
      // The popover root is permanently closed here, so the trigger has to
      // report the sheet's state rather than the popover's.
      expect(trigger).toHaveAttribute('aria-expanded', 'true')
      // The only search field on the page is the sheet's own, so the combobox
      // popover is not mounted alongside it.
      expect(screen.getByPlaceholderText('Search cities...')).toBe(
        screen.getByTestId('city-filter-sheet-search')
      )
    })

    it('opens the combobox popover on a pointer viewport', async () => {
      mockSoftKeyboardViewport(false)
      const user = userEvent.setup()
      render(
        <CityFilters
          cities={cities}
          selectedCities={[]}
          onFilterChange={vi.fn()}
          resultNoun={{ singular: 'venue', plural: 'venues' }}
        />
      )

      const trigger = screen.getByTestId('city-filter-combobox')
      expect(trigger).toHaveAttribute('role', 'combobox')

      await user.click(trigger)

      expect(screen.queryByTestId('city-filter-sheet')).toBeNull()
      expect(screen.getByPlaceholderText('Search cities...')).toHaveFocus()
    })
  })
})
