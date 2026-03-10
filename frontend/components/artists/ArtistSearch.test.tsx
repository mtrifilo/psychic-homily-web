import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Mock the search hook
const mockSearchResults = vi.fn()
vi.mock('@/lib/hooks/artists/useArtistSearch', () => ({
  useArtistSearch: ({ query }: { query: string }) => ({
    data: mockSearchResults(query),
  }),
}))

import { ArtistSearch } from './ArtistSearch'

describe('ArtistSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchResults.mockReturnValue(undefined)
  })

  it('renders search input with placeholder', () => {
    renderWithProviders(<ArtistSearch />)

    expect(screen.getByPlaceholderText('Search artists...')).toBeInTheDocument()
  })

  it('shows dropdown when results are available', async () => {
    mockSearchResults.mockReturnValue({
      artists: [
        { id: 1, slug: 'radiohead', name: 'Radiohead', city: null, state: null, social: {} },
      ],
      count: 1,
    })

    renderWithProviders(<ArtistSearch />)

    const input = screen.getByPlaceholderText('Search artists...')
    await userEvent.type(input, 'radio')

    expect(screen.getByText('Radiohead')).toBeInTheDocument()
  })

  it('navigates to artist page on selection', async () => {
    mockSearchResults.mockReturnValue({
      artists: [
        { id: 1, slug: 'radiohead', name: 'Radiohead', city: null, state: null, social: {} },
      ],
      count: 1,
    })

    renderWithProviders(<ArtistSearch />)

    const input = screen.getByPlaceholderText('Search artists...')
    await userEvent.type(input, 'radio')

    // mouseDown triggers navigation (not click, due to blur handling)
    fireEvent.mouseDown(screen.getByText('Radiohead'))

    expect(mockPush).toHaveBeenCalledWith('/artists/radiohead')
  })

  it('shows location for artists with city data', async () => {
    mockSearchResults.mockReturnValue({
      artists: [
        { id: 1, slug: 'local-band', name: 'Local Band', city: 'Phoenix', state: 'AZ', social: {} },
      ],
      count: 1,
    })

    renderWithProviders(<ArtistSearch />)

    const input = screen.getByPlaceholderText('Search artists...')
    await userEvent.type(input, 'local')

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('clears input after selection', async () => {
    mockSearchResults.mockReturnValue({
      artists: [
        { id: 1, slug: 'radiohead', name: 'Radiohead', city: null, state: null, social: {} },
      ],
      count: 1,
    })

    renderWithProviders(<ArtistSearch />)

    const input = screen.getByPlaceholderText('Search artists...') as HTMLInputElement
    await userEvent.type(input, 'radio')
    fireEvent.mouseDown(screen.getByText('Radiohead'))

    expect(input.value).toBe('')
  })
})
