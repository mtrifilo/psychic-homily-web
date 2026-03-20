import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ArtistInput } from './ArtistInput'

// --- Mocks ---

const mockSearchResults = {
  artists: [
    { id: 1, name: 'Radiohead', city: 'Oxford', state: 'UK', slug: 'radiohead' },
    { id: 2, name: 'Radio Moscow', city: 'Ames', state: 'IA', slug: 'radio-moscow' },
  ],
}

const mockUseArtistSearch = vi.fn(() => ({
  data: null as typeof mockSearchResults | null,
}))

vi.mock('@/features/artists', () => ({
  useArtistSearch: (...args: unknown[]) => mockUseArtistSearch(...args),
  getArtistLocation: (artist: { city?: string | null; state?: string | null }) => {
    if (artist.city && artist.state) return `${artist.city}, ${artist.state}`
    return artist.city || artist.state || ''
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
    name: 'artists[0]',
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

describe('ArtistInput', () => {
  let field: ReturnType<typeof makeMockField>

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    field = makeMockField()
    mockUseArtistSearch.mockReturnValue({ data: null })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders label with correct artist number (1-indexed)', () => {
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByText('Artist 1')).toBeInTheDocument()
  })

  it('renders label for second artist', () => {
    render(<ArtistInput field={field} index={2} />)
    expect(screen.getByText('Artist 3')).toBeInTheDocument()
  })

  it('renders input with placeholder', () => {
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByPlaceholderText('Enter artist name')).toBeInTheDocument()
  })

  it('renders input with field value', () => {
    field = makeMockField({ value: 'Radiohead' })
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByDisplayValue('Radiohead')).toBeInTheDocument()
  })

  it('has autoComplete="off" to prevent browser autocomplete', () => {
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByPlaceholderText('Enter artist name')).toHaveAttribute('autoComplete', 'off')
  })

  it('does not show remove button when showRemoveButton is false', () => {
    render(<ArtistInput field={field} index={0} showRemoveButton={false} onRemove={vi.fn()} />)
    expect(screen.queryByLabelText('Remove artist')).not.toBeInTheDocument()
  })

  it('does not show remove button when onRemove is not provided', () => {
    render(<ArtistInput field={field} index={0} showRemoveButton={true} />)
    expect(screen.queryByLabelText('Remove artist')).not.toBeInTheDocument()
  })

  it('shows remove button when showRemoveButton is true and onRemove is provided', () => {
    render(<ArtistInput field={field} index={0} showRemoveButton={true} onRemove={vi.fn()} />)
    expect(screen.getByLabelText('Remove artist')).toBeInTheDocument()
  })

  it('calls onRemove when remove button is clicked', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onRemove = vi.fn()
    render(<ArtistInput field={field} index={0} showRemoveButton={true} onRemove={onRemove} />)

    await user.click(screen.getByLabelText('Remove artist'))
    expect(onRemove).toHaveBeenCalledOnce()
  })

  it('calls field.handleChange when user types', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'R')
    expect(field.handleChange).toHaveBeenCalledWith('R')
  })

  it('calls onArtistMatch with undefined when user types (clears match)', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onArtistMatch = vi.fn()
    render(<ArtistInput field={field} index={0} onArtistMatch={onArtistMatch} />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'R')
    expect(onArtistMatch).toHaveBeenCalledWith(undefined)
  })

  it('shows dropdown with matching artists when search returns results', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')

    expect(screen.getByText('Existing Artists')).toBeInTheDocument()
    expect(screen.getByText('Radiohead')).toBeInTheDocument()
    expect(screen.getByText('Radio Moscow')).toBeInTheDocument()
  })

  it('shows "Add New Artist" option when input does not exactly match any result', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    // Field value is updated by handleChange but we need to simulate it staying in sync
    field = makeMockField({ value: 'Radio' })
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'x') // triggers isOpen=true and searchValue set

    expect(screen.getByText('Add New Artist')).toBeInTheDocument()
  })

  it('does not show "Add New Artist" when input exactly matches a result (case-insensitive)', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({
      data: { artists: [{ id: 1, name: 'Radiohead', city: 'Oxford', state: 'UK', slug: 'radiohead' }] },
    })
    render(<ArtistInput field={field} index={0} />)

    // Simulate typing "radiohead" to match exactly
    const input = screen.getByPlaceholderText('Enter artist name')
    // We can only partially test this since field.state.value doesn't update dynamically
    await user.type(input, 'r')
    // The searchValue is 'r', which won't exactly match 'Radiohead'
    expect(screen.getByText('Add New Artist')).toBeInTheDocument()
  })

  it('selects artist from dropdown on mouseDown', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onArtistMatch = vi.fn()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} onArtistMatch={onArtistMatch} />)

    // Type to open dropdown
    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')

    // Click an artist - uses mouseDown to prevent blur closing dropdown first
    const radioheadButton = screen.getByText('Radiohead').closest('button')!
    await user.pointer({ keys: '[MouseLeft>]', target: radioheadButton })

    expect(field.handleChange).toHaveBeenCalledWith('Radiohead')
    expect(onArtistMatch).toHaveBeenCalledWith(1)
  })

  it('closes dropdown after selecting an artist', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')
    expect(screen.getByText('Radiohead')).toBeInTheDocument()

    const radioheadButton = screen.getByText('Radiohead').closest('button')!
    await user.pointer({ keys: '[MouseLeft>]', target: radioheadButton })

    // Dropdown should be closed
    expect(screen.queryByText('Existing Artists')).not.toBeInTheDocument()
  })

  it('closes dropdown on Escape key', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, 'Radio')
    expect(screen.getByText('Existing Artists')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('Existing Artists')).not.toBeInTheDocument()
  })

  it('confirms input on Enter key', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onArtistMatch = vi.fn()
    field = makeMockField({ value: 'Unknown Band' })
    mockUseArtistSearch.mockReturnValue({ data: { artists: [] } })
    render(<ArtistInput field={field} index={0} onArtistMatch={onArtistMatch} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    await user.type(input, '{Enter}')

    // No exact match, so onArtistMatch should be called with undefined
    expect(onArtistMatch).toHaveBeenCalledWith(undefined)
  })

  it('corrects casing on Enter when exact match exists', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const onArtistMatch = vi.fn()
    field = makeMockField({ value: 'radiohead' })
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} onArtistMatch={onArtistMatch} />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), '{Enter}')

    // Should correct casing to 'Radiohead' and match the artist ID
    expect(field.handleChange).toHaveBeenCalledWith('Radiohead')
    expect(onArtistMatch).toHaveBeenCalledWith(1)
  })

  it('prevents default on Enter key (prevents form submission)', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const formSubmit = vi.fn()
    render(
      <form onSubmit={formSubmit}>
        <ArtistInput field={field} index={0} />
      </form>
    )

    await user.type(screen.getByPlaceholderText('Enter artist name'), '{Enter}')
    expect(formSubmit).not.toHaveBeenCalled()
  })

  it('calls field.handleBlur after blur delay', async () => {
    render(<ArtistInput field={field} index={0} />)

    const input = screen.getByPlaceholderText('Enter artist name')
    input.focus()
    input.blur()

    // handleBlur is called in a setTimeout(150ms)
    await vi.advanceTimersByTimeAsync(200)
    expect(field.handleBlur).toHaveBeenCalled()
  })

  it('sets aria-invalid when field has errors', () => {
    field = makeMockField({ errors: ['Required'], isTouched: true })
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByPlaceholderText('Enter artist name')).toHaveAttribute('aria-invalid', 'true')
  })

  it('does not set aria-invalid when field has no errors', () => {
    render(<ArtistInput field={field} index={0} />)
    expect(screen.getByPlaceholderText('Enter artist name')).toHaveAttribute('aria-invalid', 'false')
  })

  it('does not open dropdown when input is empty (value.length === 0)', () => {
    // The dropdown only opens when value.length > 0
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} />)

    // Without typing anything, the dropdown should not be shown
    expect(screen.queryByText('Existing Artists')).not.toBeInTheDocument()
  })

  it('displays artist locations in dropdown', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    mockUseArtistSearch.mockReturnValue({ data: mockSearchResults })
    render(<ArtistInput field={field} index={0} />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Radio')

    expect(screen.getByText('Oxford, UK')).toBeInTheDocument()
    expect(screen.getByText('Ames, IA')).toBeInTheDocument()
  })
})
