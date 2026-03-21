import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { useForm } from '@tanstack/react-form'
import { ArtistInput } from './ArtistInput'

// Mock search results
let mockSearchData: { artists: Array<{ id: number; name: string; city?: string; state?: string }> } | undefined

vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({
    data: mockSearchData,
    isLoading: false,
  }),
  getArtistLocation: (artist: { city?: string; state?: string }) =>
    [artist.city, artist.state].filter(Boolean).join(', '),
}))

function TestArtistInput({ onArtistMatch }: { onArtistMatch?: (id: number | undefined) => void }) {
  const form = useForm({
    defaultValues: { 'artists[0]': '' },
  })

  return (
    <form.Field name="artists[0]">
      {field => (
        <ArtistInput
          field={field}
          index={0}
          onArtistMatch={onArtistMatch}
        />
      )}
    </form.Field>
  )
}

describe('ArtistInput ARIA combobox attributes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchData = undefined
  })

  it('has role="combobox" on the input', () => {
    renderWithProviders(<TestArtistInput />)
    const input = screen.getByPlaceholderText('Enter artist name')
    expect(input).toHaveAttribute('role', 'combobox')
  })

  it('has aria-autocomplete="list" on the input', () => {
    renderWithProviders(<TestArtistInput />)
    const input = screen.getByPlaceholderText('Enter artist name')
    expect(input).toHaveAttribute('aria-autocomplete', 'list')
  })

  it('has aria-expanded="false" when dropdown is closed', () => {
    renderWithProviders(<TestArtistInput />)
    const input = screen.getByPlaceholderText('Enter artist name')
    expect(input).toHaveAttribute('aria-expanded', 'false')
  })

  it('has aria-controls pointing to the listbox id', () => {
    renderWithProviders(<TestArtistInput />)
    const input = screen.getByPlaceholderText('Enter artist name')
    expect(input).toHaveAttribute('aria-controls')
    const controlsId = input.getAttribute('aria-controls')
    expect(controlsId).toContain('artist-listbox')
  })

  it('shows listbox with role="listbox" when dropdown is open with results', async () => {
    mockSearchData = {
      artists: [
        { id: 1, name: 'Radiohead', city: 'Oxford', state: '' },
        { id: 2, name: 'Radio Moscow', city: 'Ames', state: 'IA' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestArtistInput />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')

    const listbox = screen.getByRole('listbox')
    expect(listbox).toBeInTheDocument()

    // The listbox id should match aria-controls
    const controlsId = input.getAttribute('aria-controls')
    expect(listbox).toHaveAttribute('id', controlsId)
  })

  it('has role="option" on each dropdown item', async () => {
    mockSearchData = {
      artists: [
        { id: 1, name: 'Radiohead', city: 'Oxford', state: '' },
        { id: 2, name: 'Radio Moscow', city: 'Ames', state: 'IA' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestArtistInput />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')

    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(2)
  })

  it('sets aria-expanded="true" when dropdown is open with results', async () => {
    mockSearchData = {
      artists: [
        { id: 1, name: 'Radiohead', city: 'Oxford', state: '' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestArtistInput />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')

    expect(input).toHaveAttribute('aria-expanded', 'true')
  })
})
