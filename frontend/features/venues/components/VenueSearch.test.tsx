import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Mock the search hook
const mockSearchResults = vi.fn()
vi.mock('../hooks/useVenueSearch', () => ({
  useVenueSearch: ({ query }: { query: string }) => ({
    data: mockSearchResults(query),
  }),
}))

import { VenueSearch } from './VenueSearch'

describe('VenueSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchResults.mockReturnValue(undefined)
  })

  it('renders search input with placeholder', () => {
    renderWithProviders(<VenueSearch />)

    expect(screen.getByPlaceholderText('Search venues...')).toBeInTheDocument()
  })

  it('shows dropdown when results are available', async () => {
    mockSearchResults.mockReturnValue({
      venues: [
        { id: 1, slug: 'valley-bar', name: 'Valley Bar', city: 'Phoenix', state: 'AZ', verified: true },
      ],
      count: 1,
    })

    renderWithProviders(<VenueSearch />)

    const input = screen.getByPlaceholderText('Search venues...')
    await userEvent.type(input, 'valley')

    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
  })

  it('navigates to venue page on selection', async () => {
    mockSearchResults.mockReturnValue({
      venues: [
        { id: 1, slug: 'valley-bar', name: 'Valley Bar', city: 'Phoenix', state: 'AZ', verified: true },
      ],
      count: 1,
    })

    renderWithProviders(<VenueSearch />)

    const input = screen.getByPlaceholderText('Search venues...')
    await userEvent.type(input, 'valley')

    // mouseDown triggers navigation (not click, due to blur handling)
    fireEvent.mouseDown(screen.getByText('Valley Bar'))

    expect(mockPush).toHaveBeenCalledWith('/venues/valley-bar')
  })

  it('shows location for venues', async () => {
    mockSearchResults.mockReturnValue({
      venues: [
        { id: 1, slug: 'valley-bar', name: 'Valley Bar', city: 'Phoenix', state: 'AZ', verified: true },
      ],
      count: 1,
    })

    renderWithProviders(<VenueSearch />)

    const input = screen.getByPlaceholderText('Search venues...')
    await userEvent.type(input, 'valley')

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('clears input after selection', async () => {
    mockSearchResults.mockReturnValue({
      venues: [
        { id: 1, slug: 'valley-bar', name: 'Valley Bar', city: 'Phoenix', state: 'AZ', verified: true },
      ],
      count: 1,
    })

    renderWithProviders(<VenueSearch />)

    const input = screen.getByPlaceholderText('Search venues...') as HTMLInputElement
    await userEvent.type(input, 'valley')
    fireEvent.mouseDown(screen.getByText('Valley Bar'))

    expect(input.value).toBe('')
  })
})
