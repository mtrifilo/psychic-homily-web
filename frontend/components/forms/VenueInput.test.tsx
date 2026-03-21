import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { useForm } from '@tanstack/react-form'
import { VenueInput } from './VenueInput'

// Mock search results
let mockSearchData: { venues: Array<{ id: number; name: string; slug: string; city?: string; state?: string }> } | undefined

vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({
    data: mockSearchData,
    isLoading: false,
  }),
  getVenueLocation: (venue: { city?: string; state?: string }) =>
    [venue.city, venue.state].filter(Boolean).join(', '),
}))

function TestVenueInput() {
  const form = useForm({
    defaultValues: { venue: '' },
  })

  return (
    <form.Field name="venue">
      {field => (
        <VenueInput field={field} />
      )}
    </form.Field>
  )
}

describe('VenueInput ARIA combobox attributes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchData = undefined
  })

  it('has role="combobox" on the input', () => {
    renderWithProviders(<TestVenueInput />)
    const input = screen.getByPlaceholderText('Enter venue name')
    expect(input).toHaveAttribute('role', 'combobox')
  })

  it('has aria-autocomplete="list" on the input', () => {
    renderWithProviders(<TestVenueInput />)
    const input = screen.getByPlaceholderText('Enter venue name')
    expect(input).toHaveAttribute('aria-autocomplete', 'list')
  })

  it('has aria-expanded="false" when dropdown is closed', () => {
    renderWithProviders(<TestVenueInput />)
    const input = screen.getByPlaceholderText('Enter venue name')
    expect(input).toHaveAttribute('aria-expanded', 'false')
  })

  it('has aria-controls pointing to the listbox id', () => {
    renderWithProviders(<TestVenueInput />)
    const input = screen.getByPlaceholderText('Enter venue name')
    expect(input).toHaveAttribute('aria-controls')
    const controlsId = input.getAttribute('aria-controls')
    expect(controlsId).toContain('venue-listbox')
  })

  it('shows listbox with role="listbox" when dropdown is open with results', async () => {
    mockSearchData = {
      venues: [
        { id: 1, name: 'The Rebel Lounge', slug: 'the-rebel-lounge', city: 'Phoenix', state: 'AZ' },
        { id: 2, name: 'Rebel Room', slug: 'rebel-room', city: 'Tempe', state: 'AZ' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestVenueInput />)

    const input = screen.getByPlaceholderText('Enter venue name')
    await user.type(input, 'Rebel')

    const listbox = screen.getByRole('listbox')
    expect(listbox).toBeInTheDocument()

    // The listbox id should match aria-controls
    const controlsId = input.getAttribute('aria-controls')
    expect(listbox).toHaveAttribute('id', controlsId)
  })

  it('has role="option" on each dropdown item', async () => {
    mockSearchData = {
      venues: [
        { id: 1, name: 'The Rebel Lounge', slug: 'the-rebel-lounge', city: 'Phoenix', state: 'AZ' },
        { id: 2, name: 'Rebel Room', slug: 'rebel-room', city: 'Tempe', state: 'AZ' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestVenueInput />)

    const input = screen.getByPlaceholderText('Enter venue name')
    await user.type(input, 'Rebel')

    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(2)
  })

  it('sets aria-expanded="true" when dropdown is open with results', async () => {
    mockSearchData = {
      venues: [
        { id: 1, name: 'The Rebel Lounge', slug: 'the-rebel-lounge', city: 'Phoenix', state: 'AZ' },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(<TestVenueInput />)

    const input = screen.getByPlaceholderText('Enter venue name')
    await user.type(input, 'Rebel')

    expect(input).toHaveAttribute('aria-expanded', 'true')
  })
})
