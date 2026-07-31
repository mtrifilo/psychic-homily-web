import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Mock the search hook — the component calls it with { query }. We echo a
// canned result keyed off the latest query so the dropdown can open.
const mockSearchResults = vi.fn()
vi.mock('../hooks/useLabelSearch', () => ({
  useLabelSearch: ({ query }: { query: string }) => ({
    data: mockSearchResults(query),
  }),
}))

import { LabelSearch } from './LabelSearch'

const SUB_POP = {
  id: 1,
  slug: 'sub-pop',
  name: 'Sub Pop',
  city: 'Seattle',
  state: 'WA',
}
const MERGE = {
  id: 2,
  slug: 'merge-records',
  name: 'Merge Records',
  city: 'Durham',
  state: 'NC',
}

describe('LabelSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchResults.mockReturnValue(undefined)
  })

  it('renders the search input with placeholder', () => {
    renderWithProviders(<LabelSearch />)
    expect(screen.getByPlaceholderText('Search labels...')).toBeInTheDocument()
  })

  it('opens the dropdown with results once the user types', async () => {
    mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

    renderWithProviders(<LabelSearch />)
    await userEvent.type(screen.getByPlaceholderText('Search labels...'), 'sub')

    expect(screen.getByText('Sub Pop')).toBeInTheDocument()
  })

  it('shows the location next to each result', async () => {
    mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

    renderWithProviders(<LabelSearch />)
    await userEvent.type(screen.getByPlaceholderText('Search labels...'), 'sub')

    expect(screen.getByText('Seattle, WA')).toBeInTheDocument()
  })

  it('does not open the dropdown when the input is empty', async () => {
    mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

    renderWithProviders(<LabelSearch />)
    // No typing — isOpen stays false even though the hook would return data.
    expect(screen.queryByText('Sub Pop')).not.toBeInTheDocument()
  })

  it('navigates to the label page on mousedown selection', async () => {
    mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

    renderWithProviders(<LabelSearch />)
    await userEvent.type(screen.getByPlaceholderText('Search labels...'), 'sub')

    // mousedown (not click) drives selection — blur is delayed to allow it.
    fireEvent.mouseDown(screen.getByText('Sub Pop'))

    expect(mockPush).toHaveBeenCalledWith('/labels/sub-pop')
  })

  it('clears the input after a selection', async () => {
    mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

    renderWithProviders(<LabelSearch />)
    const input = screen.getByPlaceholderText(
      'Search labels...'
    ) as HTMLInputElement
    await userEvent.type(input, 'sub')
    fireEvent.mouseDown(screen.getByText('Sub Pop'))

    expect(input.value).toBe('')
  })

  describe('keyboard navigation', () => {
    it('selects the active result on ArrowDown + Enter', async () => {
      const user = userEvent.setup()
      mockSearchResults.mockReturnValue({
        labels: [SUB_POP, MERGE],
        count: 2,
      })

      renderWithProviders(<LabelSearch />)
      const input = screen.getByPlaceholderText('Search labels...')
      await user.type(input, 'records')

      // First ArrowDown highlights index 0 (Sub Pop), then Enter selects it.
      await user.keyboard('{ArrowDown}{Enter}')

      expect(mockPush).toHaveBeenCalledWith('/labels/sub-pop')
    })

    it('wraps to the second result with two ArrowDowns', async () => {
      const user = userEvent.setup()
      mockSearchResults.mockReturnValue({
        labels: [SUB_POP, MERGE],
        count: 2,
      })

      renderWithProviders(<LabelSearch />)
      const input = screen.getByPlaceholderText('Search labels...')
      await user.type(input, 'records')

      await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')

      expect(mockPush).toHaveBeenCalledWith('/labels/merge-records')
    })

    it('closes the dropdown on Escape without navigating', async () => {
      const user = userEvent.setup()
      mockSearchResults.mockReturnValue({ labels: [SUB_POP], count: 1 })

      renderWithProviders(<LabelSearch />)
      const input = screen.getByPlaceholderText('Search labels...')
      await user.type(input, 'sub')
      expect(screen.getByText('Sub Pop')).toBeInTheDocument()

      await user.keyboard('{Escape}')

      expect(screen.queryByText('Sub Pop')).not.toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })
  })

  // The blur delay used to be an untracked `setTimeout`, so it still fired
  // ~150ms after unmount and called `setState` into a React DOM with no
  // document. Under vitest that lands after jsdom teardown and throws
  // `ReferenceError: window is not defined`, which fails the whole run with
  // every test passing — a race against teardown, so it only showed on a
  // loaded CI runner. Asserting no timer survives unmount is the invariant;
  // the symptom itself is not reproducible from inside a test.
  it('leaves no pending blur timer behind on unmount', () => {
    vi.useFakeTimers()
    try {
      const { unmount } = renderWithProviders(<LabelSearch />)
      fireEvent.blur(screen.getByPlaceholderText('Search labels...'))
      expect(vi.getTimerCount()).toBeGreaterThan(0)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })
})
