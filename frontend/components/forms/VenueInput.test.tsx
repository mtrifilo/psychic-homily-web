import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VenueInput } from './VenueInput'

// --- Mocks ---

const mockVenues = [
  { id: 1, name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ', slug: 'the-rebel-lounge', address: '2303 E Indian School Rd', verified: true, created_at: '', updated_at: '' },
  { id: 2, name: 'Valley Bar', city: 'Phoenix', state: 'AZ', slug: 'valley-bar', address: '130 N Central Ave', verified: true, created_at: '', updated_at: '' },
]

const mockSearchResults = { venues: mockVenues }

const mockUseVenueSearch = vi.fn(() => ({
  data: null as typeof mockSearchResults | null,
}))

vi.mock('@/features/venues', () => ({
  useVenueSearch: (...args: unknown[]) => mockUseVenueSearch(...args),
  getVenueLocation: (venue: { city?: string; state?: string }) => {
    if (venue.city && venue.state) return `${venue.city}, ${venue.state}`
    return venue.city || venue.state || ''
  },
}))

vi.mock('./FormField', () => ({
  FieldInfo: ({ field }: { field: { state: { meta: { errors: unknown[]; isTouched: boolean } } } }) => {
    if (field.state.meta.isTouched && field.state.meta.errors.length > 0) {
      return <p role="alert">{String(field.state.meta.errors[0])}</p>
    }
    return null
  },
}))

function makeMockField(overrides: Partial<{
  value: string
  errors: unknown[]
  isTouched: boolean
}> = {}) {
  return {
    name: 'venue',
    state: {
      value: overrides.value ?? '',
      meta: {
        errors: overrides.errors ?? [],
        isTouched: overrides.isTouched ?? false,
        isValidating: false,
      },
    },
    handleChange: vi.fn(),
    handleBlur: vi.fn(),
  } as any // eslint-disable-line @typescript-eslint/no-explicit-any
}

describe('VenueInput', () => {
  let field: ReturnType<typeof makeMockField>

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    field = makeMockField()
    mockUseVenueSearch.mockReturnValue({ data: null })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders label "Venue"', () => {
    render(<VenueInput field={field} />)
    expect(screen.getByText('Venue')).toBeInTheDocument()
  })

  it('renders input with placeholder', () => {
    render(<VenueInput field={field} />)
    expect(screen.getByPlaceholderText('Enter venue name')).toBeInTheDocument()
  })

  it('renders input with current field value', () => {
    field = makeMockField({ value: 'The Rebel Lounge' })
    render(<VenueInput field={field} />)
    expect(screen.getByDisplayValue('The Rebel Lounge')).toBeInTheDocument()
  })

  it('has autoComplete="off"', () => {
    render(<VenueInput field={field} />)
    expect(screen.getByPlaceholderText('Enter venue name')).toHaveAttribute('autoComplete', 'off')
  })

  it('calls field.handleChange when user types', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'T')
    expect(field.handleChange).toHaveBeenCalledWith('T')
  })

  it('calls onVenueSelect(null) when user types (clears selection)', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueSelect = vi.fn()
    render(<VenueInput field={field} onVenueSelect={onVenueSelect} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'T')
    expect(onVenueSelect).toHaveBeenCalledWith(null)
  })

  it('calls onVenueNameChange when user types', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueNameChange = vi.fn()
    render(<VenueInput field={field} onVenueNameChange={onVenueNameChange} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'T')
    expect(onVenueNameChange).toHaveBeenCalledWith('T')
  })

  it('shows dropdown with matching venues when search returns results', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'the')

    expect(screen.getByText('Existing Venues')).toBeInTheDocument()
    expect(screen.getByText('The Rebel Lounge')).toBeInTheDocument()
    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
  })

  it('shows "Add New Venue" option when input does not exactly match', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    field = makeMockField({ value: 'New Place' })
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'x')
    expect(screen.getByText('Add New Venue')).toBeInTheDocument()
  })

  it('selects venue from dropdown on mouseDown', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueSelect = vi.fn()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} onVenueSelect={onVenueSelect} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'Rebel')

    const venueButton = screen.getByText('The Rebel Lounge').closest('button')!
    await user.pointer({ keys: '[MouseLeft>]', target: venueButton })

    expect(field.handleChange).toHaveBeenCalledWith('The Rebel Lounge')
    expect(onVenueSelect).toHaveBeenCalledWith(mockVenues[0])
  })

  it('closes dropdown after selecting a venue', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'Bar')
    expect(screen.getByText('Existing Venues')).toBeInTheDocument()

    const venueButton = screen.getByText('Valley Bar').closest('button')!
    await user.pointer({ keys: '[MouseLeft>]', target: venueButton })

    expect(screen.queryByText('Existing Venues')).not.toBeInTheDocument()
  })

  it('closes dropdown on Escape key', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'Bar')
    expect(screen.getByText('Existing Venues')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('Existing Venues')).not.toBeInTheDocument()
  })

  it('confirms input on Enter key with exact match', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueSelect = vi.fn()
    field = makeMockField({ value: 'valley bar' })
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} onVenueSelect={onVenueSelect} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), '{Enter}')

    // Should correct casing and pass the matched venue
    expect(field.handleChange).toHaveBeenCalledWith('Valley Bar')
    expect(onVenueSelect).toHaveBeenCalledWith(mockVenues[1])
  })

  it('confirms input on Enter key with no match passes null', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueSelect = vi.fn()
    field = makeMockField({ value: 'Unknown Venue' })
    mockUseVenueSearch.mockReturnValue({ data: { venues: [] } })
    render(<VenueInput field={field} onVenueSelect={onVenueSelect} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), '{Enter}')
    expect(onVenueSelect).toHaveBeenCalledWith(null)
  })

  it('prevents default on Enter key (no form submission)', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const formSubmit = vi.fn()
    render(
      <form onSubmit={formSubmit}>
        <VenueInput field={field} />
      </form>
    )

    await user.type(screen.getByPlaceholderText('Enter venue name'), '{Enter}')
    expect(formSubmit).not.toHaveBeenCalled()
  })

  it('calls field.handleBlur after blur delay', async () => {
    render(<VenueInput field={field} />)

    const input = screen.getByPlaceholderText('Enter venue name')
    input.focus()
    input.blur()

    await vi.advanceTimersByTimeAsync(200)
    expect(field.handleBlur).toHaveBeenCalled()
  })

  it('does not call onVenueSelect(null) on blur after selecting from dropdown', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onVenueSelect = vi.fn()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} onVenueSelect={onVenueSelect} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'Rebel')

    // Select venue from dropdown
    const venueButton = screen.getByText('The Rebel Lounge').closest('button')!
    await user.pointer({ keys: '[MouseLeft>]', target: venueButton })

    // The justSelectedRef should prevent handleConfirm from calling onVenueSelect(null)
    const selectCalls = onVenueSelect.mock.calls
    const lastCall = selectCalls[selectCalls.length - 1]
    // The last call should be the venue selection, not null
    expect(lastCall[0]).toEqual(mockVenues[0])
  })

  it('sets aria-invalid when field has errors', () => {
    field = makeMockField({ errors: ['Required'], isTouched: true })
    render(<VenueInput field={field} />)
    expect(screen.getByPlaceholderText('Enter venue name')).toHaveAttribute('aria-invalid', 'true')
  })

  it('does not set aria-invalid when field has no errors', () => {
    render(<VenueInput field={field} />)
    expect(screen.getByPlaceholderText('Enter venue name')).toHaveAttribute('aria-invalid', 'false')
  })

  it('displays venue locations in dropdown', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    await user.type(screen.getByPlaceholderText('Enter venue name'), 'the')

    expect(screen.getAllByText('Phoenix, AZ')).toHaveLength(2)
  })

  it('does not open dropdown when typing empty string (value.length === 0)', () => {
    // The dropdown is only opened when value.length > 0
    // If the value is empty, isOpen stays false
    mockUseVenueSearch.mockReturnValue({ data: mockSearchResults })
    render(<VenueInput field={field} />)

    // Without typing anything, the dropdown should not be shown
    expect(screen.queryByText('Existing Venues')).not.toBeInTheDocument()
  })
})
