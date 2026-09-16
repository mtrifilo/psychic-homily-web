import { describe, it, expect, vi, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'

import { renderWithProviders, screen, waitFor } from '@/test/utils'
import { SOFT_KEYBOARD_VIEWPORT_QUERY } from '@/lib/hooks/common/useSoftKeyboardViewport'
import { CityFilters, type CityWithCount } from './CityFilters'

const cities: CityWithCount[] = [
  { city: 'Phoenix', state: 'AZ', count: 8 },
  { city: 'Chicago', state: 'IL', count: 42 },
  { city: 'Mesa', state: 'AZ', count: 1 },
]

const originalMatchMedia = window.matchMedia

function mockSoftKeyboardViewport() {
  window.matchMedia = vi.fn(
    (query: string) =>
      ({
        matches: query === SOFT_KEYBOARD_VIEWPORT_QUERY,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }) as unknown as MediaQueryList
  )
}

afterEach(() => {
  window.matchMedia = originalMatchMedia
})

function renderFilters(
  props: Partial<React.ComponentProps<typeof CityFilters>> = {}
) {
  mockSoftKeyboardViewport()
  return renderWithProviders(
    <CityFilters
      cities={cities}
      selectedCities={[]}
      onFilterChange={vi.fn()}
      resultNoun={{ singular: 'venue', plural: 'venues' }}
      {...props}
    />
  )
}

async function openSheet(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByTestId('city-filter-combobox'))
  return screen.findByRole('dialog')
}

describe('CityFilterSheet', () => {
  it('names itself and keeps the search field above the scrolling list', async () => {
    const user = userEvent.setup()
    renderFilters()

    const sheet = await openSheet(user)

    expect(sheet).toHaveAccessibleName('Filter by city')
    expect(sheet).toHaveAttribute('aria-describedby')
    const search = screen.getByTestId('city-filter-sheet-search')
    const list = screen.getByTestId('city-filter-sheet-list')
    // The field is a sibling ABOVE the scroll container, not a row inside it,
    // which is what keeps it on screen at any scroll position.
    expect(list.contains(search)).toBe(false)
    expect(search.compareDocumentPosition(list)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING
    )
    expect(list.className).toContain('overflow-y-auto')
  })

  it('counts every listed city until cities are ticked', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)

    const apply = screen.getByTestId('city-filter-sheet-apply')
    expect(apply).toHaveTextContent('Show 51 venues')

    await user.click(screen.getByTestId('city-sheet-option-phoenix-az'))
    expect(apply).toHaveTextContent('Show 8 venues')

    await user.click(screen.getByTestId('city-sheet-option-chicago-il'))
    expect(apply).toHaveTextContent('Show 50 venues')
  })

  it('uses the singular noun for a single result', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.click(screen.getByTestId('city-sheet-option-mesa-az'))

    expect(screen.getByTestId('city-filter-sheet-apply')).toHaveTextContent(
      'Show 1 venue'
    )
  })

  it('applies the edited selection once, on the button', async () => {
    const user = userEvent.setup()
    const onFilterChange = vi.fn()
    renderFilters({ onFilterChange })

    await openSheet(user)
    await user.click(screen.getByTestId('city-sheet-option-phoenix-az'))
    await user.click(screen.getByTestId('city-sheet-option-chicago-il'))

    // Ticking is not applying: nothing is written until the button.
    expect(onFilterChange).not.toHaveBeenCalled()

    await user.click(screen.getByTestId('city-filter-sheet-apply'))

    expect(onFilterChange).toHaveBeenCalledTimes(1)
    expect(onFilterChange).toHaveBeenCalledWith([
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Chicago', state: 'IL' },
    ])
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('discards an edit that was never applied', async () => {
    const user = userEvent.setup()
    const onFilterChange = vi.fn()
    renderFilters({
      onFilterChange,
      selectedCities: [{ city: 'Phoenix', state: 'AZ' }],
    })

    await openSheet(user)
    await user.click(screen.getByTestId('city-sheet-option-chicago-il'))
    await user.keyboard('{Escape}')
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })

    expect(onFilterChange).not.toHaveBeenCalled()

    // Reopening starts from the applied selection again, not the abandoned edit.
    await openSheet(user)
    expect(screen.getByTestId('city-filter-sheet-apply')).toHaveTextContent(
      'Show 8 venues'
    )
  })

  it('filters the list by the search field', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.type(screen.getByTestId('city-filter-sheet-search'), 'chi')

    expect(screen.getByTestId('city-sheet-option-chicago-il')).toBeInTheDocument()
    expect(screen.queryByTestId('city-sheet-option-phoenix-az')).toBeNull()

    await user.clear(screen.getByTestId('city-filter-sheet-search'))
    await user.type(screen.getByTestId('city-filter-sheet-search'), 'zzz')
    expect(screen.getByText('No cities found.')).toBeInTheDocument()
  })

  it('marks each city with its own checked state and count', async () => {
    const user = userEvent.setup()
    renderFilters({ selectedCities: [{ city: 'Phoenix', state: 'AZ' }] })

    await openSheet(user)

    expect(screen.getByLabelText('Phoenix, AZ, 8 venues')).toBeChecked()
    expect(screen.getByLabelText('Chicago, IL, 42 venues')).not.toBeChecked()
    expect(screen.getByLabelText('Mesa, AZ, 1 venue')).not.toBeChecked()
  })

  it('traps focus while open', async () => {
    const user = userEvent.setup()
    renderFilters()

    const sheet = await openSheet(user)
    await waitFor(() => {
      expect(sheet.contains(document.activeElement)).toBe(true)
    })

    for (let i = 0; i < 8; i++) {
      await user.tab()
      expect(sheet.contains(document.activeElement)).toBe(true)
    }
  })

  it('returns focus to the trigger when the handle closes it', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.click(screen.getByTestId('city-filter-sheet-handle'))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(screen.getByTestId('city-filter-combobox')).toHaveFocus()
  })

  it('returns focus to the trigger when the header Close closes it', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.click(screen.getByTestId('city-filter-sheet-close'))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(screen.getByTestId('city-filter-combobox')).toHaveFocus()
  })

  // The sheet carries its own handle and Close, so the shared corner X would be
  // a third dismiss control landing on top of the grab handle.
  it('suppresses the shared sheet close button', async () => {
    const user = userEvent.setup()
    renderFilters()

    const sheet = await openSheet(user)

    expect(sheet.querySelector('.lucide-x')).toBeNull()
    expect(screen.getByTestId('city-filter-sheet-handle')).toBeInTheDocument()
    expect(screen.getByTestId('city-filter-sheet-close')).toBeInTheDocument()
  })

  it('returns focus to the trigger after Escape', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(screen.getByTestId('city-filter-combobox')).toHaveFocus()
  })

  it('returns focus to the trigger after applying', async () => {
    const user = userEvent.setup()
    renderFilters()

    await openSheet(user)
    await user.click(screen.getByTestId('city-filter-sheet-apply'))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(screen.getByTestId('city-filter-combobox')).toHaveFocus()
  })
})
