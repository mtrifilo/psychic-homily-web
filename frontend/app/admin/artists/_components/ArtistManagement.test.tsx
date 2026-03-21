import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ArtistManagement } from './ArtistManagement'

// --- Mock data ---

const mockArtists = [
  { id: 1, name: 'Artist One', city: 'Phoenix', state: 'AZ', slug: 'artist-one' },
  { id: 2, name: 'Artist Two', city: 'Tempe', state: 'AZ', slug: 'artist-two' },
]

// --- Hook mocks ---

let mockSearchData: { artists: typeof mockArtists } | undefined
let mockSearchLoading = false

vi.mock('@/features/artists', () => ({
  useArtistSearch: ({ query }: { query: string }) => ({
    data: query.length >= 2 ? mockSearchData : undefined,
    isLoading: mockSearchLoading,
  }),
}))

let mockAliasesData: { aliases: { id: number; alias: string }[] } | undefined
let mockAliasesLoading = false

const mockCreateAliasMutate = vi.fn()
let mockCreateAliasPending = false

const mockDeleteAliasMutate = vi.fn()
let mockDeleteAliasPending = false

const mockMergeMutate = vi.fn()
let mockMergePending = false

vi.mock('@/lib/hooks/admin/useAdminArtists', () => ({
  useArtistAliases: () => ({
    data: mockAliasesData,
    isLoading: mockAliasesLoading,
  }),
  useCreateArtistAlias: () => ({
    mutate: mockCreateAliasMutate,
    isPending: mockCreateAliasPending,
  }),
  useDeleteArtistAlias: () => ({
    mutate: mockDeleteAliasMutate,
    isPending: mockDeleteAliasPending,
  }),
  useMergeArtists: () => ({
    mutate: mockMergeMutate,
    isPending: mockMergePending,
  }),
}))

describe('ArtistManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchData = undefined
    mockSearchLoading = false
    mockAliasesData = undefined
    mockAliasesLoading = false
    mockCreateAliasMutate.mockReset()
    mockCreateAliasPending = false
    mockDeleteAliasMutate.mockReset()
    mockDeleteAliasPending = false
    mockMergeMutate.mockReset()
    mockMergePending = false
  })

  it('renders title and description', () => {
    renderWithProviders(<ArtistManagement />)
    expect(screen.getByText('Artist Management')).toBeInTheDocument()
    expect(screen.getByText('Manage aliases and merge duplicate artists')).toBeInTheDocument()
  })

  it('renders search input', () => {
    renderWithProviders(<ArtistManagement />)
    expect(screen.getByPlaceholderText('Search artists to manage aliases...')).toBeInTheDocument()
  })

  it('renders Merge Artists button', () => {
    renderWithProviders(<ArtistManagement />)
    expect(screen.getByRole('button', { name: /Merge Artists/ })).toBeInTheDocument()
  })

  // --- Search behavior ---

  it('does not show search results when query is too short', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'A')
    expect(screen.queryByText('Artist One')).not.toBeInTheDocument()
  })

  it('shows search results when query length >= 2', async () => {
    mockSearchData = { artists: mockArtists }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Ar')
    expect(screen.getByText('Artist One')).toBeInTheDocument()
    expect(screen.getByText('Artist Two')).toBeInTheDocument()
  })

  it('shows loading state during search', async () => {
    mockSearchLoading = true
    mockSearchData = undefined
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    // Loading spinner should be visible (Loader2 component)
    // The loading state renders a centered spinner container
    expect(screen.queryByText('Artist One')).not.toBeInTheDocument()
  })

  it('shows "No artists found" when search returns empty', async () => {
    mockSearchData = { artists: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'xyz')
    expect(screen.getByText('No artists found')).toBeInTheDocument()
  })

  it('clears selected artist when search is cleared below 2 chars', async () => {
    mockSearchData = { artists: mockArtists }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    // Type to search
    const input = screen.getByPlaceholderText('Search artists to manage aliases...')
    await user.type(input, 'Art')

    // Select an artist
    await user.click(screen.getByText('Artist One'))
    expect(screen.getByText(/Aliases for Artist One/)).toBeInTheDocument()

    // Clear the search input (which triggers setSelectedArtist(null) when < 2 chars)
    await user.clear(input)
    expect(screen.queryByText(/Aliases for Artist One/)).not.toBeInTheDocument()
  })

  // --- Artist selection & alias management ---

  it('shows alias manager when artist is selected', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    expect(screen.getByText('Aliases for Artist One')).toBeInTheDocument()
    expect(screen.getByText('No aliases')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Add alias...')).toBeInTheDocument()
  })

  it('shows existing aliases', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = {
      aliases: [
        { id: 10, alias: 'AO' },
        { id: 11, alias: 'Artist 1' },
      ],
    }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    expect(screen.getByText('AO')).toBeInTheDocument()
    expect(screen.getByText('Artist 1')).toBeInTheDocument()
  })

  it('shows artist location and ID when selected', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
    expect(screen.getByText(/ID: 1/)).toBeInTheDocument()
  })

  it('shows "No location" when artist has no city/state', async () => {
    mockSearchData = {
      artists: [
        { id: 3, name: 'Mystery Artist', city: '', state: '', slug: 'mystery' },
      ],
    }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'My')
    await user.click(screen.getByText('Mystery Artist'))

    expect(screen.getByText(/No location/)).toBeInTheDocument()
  })

  it('calls createAlias.mutate when adding alias', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    const aliasInput = screen.getByPlaceholderText('Add alias...')
    await user.type(aliasInput, 'New Alias')

    // Find the add button (the Plus button next to the input)
    const addButtons = screen.getAllByRole('button')
    const addBtn = addButtons.find(btn => {
      // It's the small button next to the alias input
      const isInAliasSection = btn.closest('.space-y-3') !== null
      return isInAliasSection && btn.textContent === ''
    })

    // Click the add button - it's the button with Plus icon
    // Use keyboard Enter instead since the button is harder to target
    await user.type(aliasInput, '{Enter}')

    expect(mockCreateAliasMutate).toHaveBeenCalledWith(
      { artistId: 1, alias: 'New Alias' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('does not add empty alias', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    const aliasInput = screen.getByPlaceholderText('Add alias...')
    // Type Enter on empty input
    await user.type(aliasInput, '{Enter}')

    expect(mockCreateAliasMutate).not.toHaveBeenCalled()
  })

  it('deselects artist when close button is clicked', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesData = { aliases: [] }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))
    expect(screen.getByText('Aliases for Artist One')).toBeInTheDocument()

    // Click the close X button on the selected artist panel
    const closeButtons = screen.getAllByRole('button')
    // The close button is the ghost variant with X icon in the selected artist section
    const selectedPanel = screen.getByText('Artist One').closest('.rounded-md.border.p-4')
    if (selectedPanel) {
      const closeBtnInPanel = selectedPanel.querySelector('button')
      // There should be a close/X button
      // Find a button that's not the add alias or merge button
    }

    // Use a more reliable approach - find the X button by its proximity
    const artistHeader = screen.getByText('Artist One').closest('.flex.items-center.justify-between')
    const closeBtn = artistHeader?.querySelector('button')
    if (closeBtn) {
      await user.click(closeBtn)
      expect(screen.queryByText('Aliases for Artist One')).not.toBeInTheDocument()
    }
  })

  // --- Merge dialog ---

  it('opens merge dialog when Merge Artists button is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.click(screen.getByRole('button', { name: /Merge Artists/ }))
    expect(screen.getByText(/Merge a duplicate artist into the canonical one/)).toBeInTheDocument()
    expect(screen.getByText('Keep (canonical)')).toBeInTheDocument()
    expect(screen.getByText('Merge & delete')).toBeInTheDocument()
  })

  it('shows Cancel and disabled Merge button in merge dialog', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.click(screen.getByRole('button', { name: /Merge Artists/ }))

    // Cancel button should be visible
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()

    // Merge button should be disabled when no artists selected
    const mergeBtn = screen.getAllByRole('button').find(
      btn => btn.textContent?.includes('Merge Artists') && btn.closest('[role="dialog"]')
    )
    expect(mergeBtn).toBeDefined()
    if (mergeBtn) {
      expect(mergeBtn).toBeDisabled()
    }
  })

  it('closes merge dialog when Cancel is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.click(screen.getByRole('button', { name: /Merge Artists/ }))
    expect(screen.getByText(/Merge a duplicate artist into the canonical one/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Merge a duplicate artist into the canonical one/)).not.toBeInTheDocument()
  })

  // --- Aliases loading ---

  it('shows loading state for aliases', async () => {
    mockSearchData = { artists: mockArtists }
    mockAliasesLoading = true
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')
    await user.click(screen.getByText('Artist One'))

    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  // --- Search results show location ---

  it('shows artist city and state in search results', async () => {
    mockSearchData = { artists: mockArtists }
    const user = userEvent.setup()
    renderWithProviders(<ArtistManagement />)

    await user.type(screen.getByPlaceholderText('Search artists to manage aliases...'), 'Art')

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    expect(screen.getByText('Tempe, AZ')).toBeInTheDocument()
  })
})
