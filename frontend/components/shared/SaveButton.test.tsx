import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SaveButton } from './SaveButton'

const mockToggle = vi.fn()
const mockUseSaveShowToggle = vi.fn(() => ({
  isSaved: false,
  isLoading: false,
  toggle: mockToggle,
  error: null,
}))

vi.mock('@/lib/hooks/shows/useSavedShows', () => ({
  useSaveShowToggle: (...args: unknown[]) => mockUseSaveShowToggle(...args),
}))

const mockUseAuthContext = vi.fn(() => ({
  isAuthenticated: true,
  user: { email: 'test@test.com' },
  isLoading: false,
  logout: vi.fn(),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

describe('SaveButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { email: 'test@test.com' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: false,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
  })

  it('renders nothing when not authenticated', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
      isLoading: false,
      logout: vi.fn(),
    })
    const { container } = render(<SaveButton showId={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders save button when authenticated', () => {
    render(<SaveButton showId={1} />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('has "Add to My List" aria-label when not saved', () => {
    render(<SaveButton showId={1} />)
    expect(screen.getByLabelText('Add to My List')).toBeInTheDocument()
  })

  it('has "Remove from My List" aria-label when saved', () => {
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: true,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    render(<SaveButton showId={1} />)
    expect(screen.getByLabelText('Remove from My List')).toBeInTheDocument()
  })

  it('has "Add to My List" title when not saved', () => {
    render(<SaveButton showId={1} />)
    expect(screen.getByTitle('Add to My List')).toBeInTheDocument()
  })

  it('has "Remove from My List" title when saved', () => {
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: true,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    render(<SaveButton showId={1} />)
    expect(screen.getByTitle('Remove from My List')).toBeInTheDocument()
  })

  it('calls toggle on click', async () => {
    const user = userEvent.setup()
    render(<SaveButton showId={1} />)

    await user.click(screen.getByRole('button'))
    expect(mockToggle).toHaveBeenCalledOnce()
  })

  it('disables button when loading', () => {
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: false,
      isLoading: true,
      toggle: mockToggle,
      error: null,
    })
    render(<SaveButton showId={1} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('shows "Save" label when showLabel is true and not saved', () => {
    render(<SaveButton showId={1} showLabel={true} />)
    expect(screen.getByText('Save')).toBeInTheDocument()
  })

  it('shows "Saved" label when showLabel is true and saved', () => {
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: true,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    render(<SaveButton showId={1} showLabel={true} />)
    expect(screen.getByText('Saved')).toBeInTheDocument()
  })

  it('does not show text label when showLabel is false', () => {
    render(<SaveButton showId={1} showLabel={false} />)
    expect(screen.queryByText('Save')).not.toBeInTheDocument()
    expect(screen.queryByText('Saved')).not.toBeInTheDocument()
  })

  it('passes showId and isAuthenticated to useSaveShowToggle', () => {
    render(<SaveButton showId={42} />)
    expect(mockUseSaveShowToggle).toHaveBeenCalledWith(42, true, undefined)
  })

  it('passes batchIsSaved to useSaveShowToggle', () => {
    render(<SaveButton showId={42} isSaved={true} />)
    expect(mockUseSaveShowToggle).toHaveBeenCalledWith(42, true, true)
  })

  it('shows error tooltip when toggle fails', async () => {
    const user = userEvent.setup()
    mockToggle.mockRejectedValueOnce(new Error('Network error'))
    mockUseSaveShowToggle.mockReturnValue({
      isSaved: false,
      isLoading: false,
      toggle: mockToggle,
      error: new Error('Network error'),
    })
    render(<SaveButton showId={1} />)

    await user.click(screen.getByRole('button'))
    expect(await screen.findByText('Failed to save show')).toBeInTheDocument()
  })

  it('stops event propagation on click', async () => {
    const user = userEvent.setup()
    const parentClick = vi.fn()
    render(
      <div onClick={parentClick}>
        <SaveButton showId={1} />
      </div>
    )

    await user.click(screen.getByRole('button'))
    expect(parentClick).not.toHaveBeenCalled()
  })
})
